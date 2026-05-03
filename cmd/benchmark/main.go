package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/ingestion"
	"phantom/pkg/shared"
	"phantom/pkg/signalmatrix"
)

type benchmarkResult struct {
	name     string
	passed   bool
	expected string
	actual   string
	detail   string
}

func main() {
	rustBin := flag.String("rust-bin", "", "path to compute_server binary (default: debug then release)")
	outputFmt := flag.String("output", "text", "output format: text or json")
	skipHalving := flag.Bool("skip-halving", false, "skip halving benchmark")
	insecure := flag.Bool("insecure", false, "skip TLS verification (needed if system clock is skewed)")
	useFixtures := flag.Bool("fixtures", false, "use synthetic fixture data instead of live APIs")
	outputDir := flag.String("output-dir", "benchmarks", "directory to save JSON result history")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	label := "Live Data"
	if *useFixtures {
		label = "Fixture"
	}
	fmt.Fprintf(os.Stderr, "=== %s Benchmark Report ===\n", label)

	binPath := *rustBin
	if binPath == "" {
		debugPath := "rust/compute_server/target/debug/compute_server"
		releasePath := "rust/compute_server/target/release/compute_server"
		if _, err := os.Stat(debugPath); err == nil {
			binPath = debugPath
		} else if _, err := os.Stat(releasePath); err == nil {
			binPath = releasePath
		} else {
			fmt.Fprintf(os.Stderr, "FATAL: compute_server binary not found — run 'make build-rust' first\n")
			os.Exit(1)
		}
	}

	computeCmd := exec.CommandContext(ctx, binPath)
	if err := computeCmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: start compute_server: %v — run 'make build-rust' first\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Rust compute_server: binary=%s pid=%d\n", binPath, computeCmd.Process.Pid)
	fmt.Fprintf(os.Stderr, "Rust compute_server: pid=%d\n", computeCmd.Process.Pid)
	time.Sleep(500 * time.Millisecond)

	bridge, err := signalmatrix.NewRustBridge("[::1]:50051")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: NewRustBridge: %v\n", err)
		computeCmd.Process.Kill()
		os.Exit(1)
	}
	defer computeCmd.Process.Kill()

	var results []benchmarkResult

	if *useFixtures {
		if !*skipHalving {
			results = append(results, runHalvingFixtureBenchmark(ctx, bridge))
		}
	} else {
		if !*skipHalving {
			results = append(results, runHalvingBenchmark(ctx, bridge, *insecure))
		}
	}

	switch *outputFmt {
	case "json":
		printJSON(results)
	default:
		printText(results)
	}

	if err := saveJSON(results, *outputDir, *useFixtures); err != nil {
		fmt.Fprintf(os.Stderr, "save results: %v\n", err)
	}

	passed := 0
	for _, r := range results {
		if r.passed {
			passed++
		}
	}
	fmt.Fprintf(os.Stderr, "\n%d/%d benchmarks passed\n", passed, len(results))
	if passed < len(results) {
		os.Exit(1)
	}
}

func printText(results []benchmarkResult) {
	for _, r := range results {
		status := "PASS"
		if !r.passed {
			status = "FAIL"
		}
		fmt.Printf("\n %s\n", r.name)
		fmt.Printf("   Expected: %s\n", r.expected)
		fmt.Printf("   Actual:   %s\n", r.actual)
		fmt.Printf("   Result:   %s\n", status)
		if r.detail != "" {
			fmt.Printf("   Detail:   %s\n", r.detail)
		}
	}
}

func printJSON(results []benchmarkResult) {
	// minimal JSON output
	fmt.Println("[")
	for i, r := range results {
		comma := ","
		if i == len(results)-1 {
			comma = ""
		}
		fmt.Printf("  {\"name\":%q,\"passed\":%v,\"expected\":%q,\"actual\":%q,\"detail\":%q}%s\n",
			r.name, r.passed, r.expected, r.actual, r.detail, comma)
	}
	fmt.Println("]")
}

// resultRecord is the persisted JSON shape.
type resultRecord struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Detail   string `json:"detail,omitempty"`
}

type resultFile struct {
	Timestamp string         `json:"timestamp"`
	Mode      string         `json:"mode"`
	Results   []resultRecord `json:"results"`
	Summary   string         `json:"summary"`
}

func saveJSON(results []benchmarkResult, dir string, fixtures bool) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	mode := "live"
	if fixtures {
		mode = "fixture"
	}

	passed := 0
	for _, r := range results {
		if r.passed {
			passed++
		}
	}

	recs := make([]resultRecord, len(results))
	for i, r := range results {
		recs[i] = resultRecord{
			Name: r.name, Passed: r.passed,
			Expected: r.expected, Actual: r.actual, Detail: r.detail,
		}
	}

	now := time.Now().UTC()
	rf := resultFile{
		Timestamp: now.Format(time.RFC3339),
		Mode:      mode,
		Results:   recs,
		Summary:   fmt.Sprintf("%d/%d passed", passed, len(results)),
	}

	data, err := json.MarshalIndent(rf, "", "  ")
	if err != nil {
		return err
	}

	fname := fmt.Sprintf("benchmark_%s.json", now.Format("20060102T150405"))
	path := filepath.Join(dir, fname)
	return os.WriteFile(path, data, 0644)
}

// --- BTC halving CAR benchmark ---

var halvingEvents = []shared.Event{
	{ID: "btc-halving-2012", Type: "btc_halving", Timestamp: time.Date(2012, 11, 28, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2016", Type: "btc_halving", Timestamp: time.Date(2016, 7, 9, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2020", Type: "btc_halving", Timestamp: time.Date(2020, 5, 11, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2024", Type: "btc_halving", Timestamp: time.Date(2024, 4, 19, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
}

func runHalvingBenchmark(ctx context.Context, bridge *signalmatrix.RustBridge, insecureTLS bool) benchmarkResult {
	r := benchmarkResult{name: "BTC Halving CAR"}
	r.expected = "positive mean CAR(0,+30) across 4 events"

	var allCARs []float64
	windowLen := 31 // T0 to T0+30 inclusive = 31 days
	var details []string

	httpClient := http.DefaultClient
	if insecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	for _, evt := range halvingEvents {
		from := evt.Timestamp.AddDate(0, 0, -365)
		to := evt.Timestamp.AddDate(0, 0, windowLen+30) // buffer after

		fetcher := &ingestion.BinanceFetcher{Interval: "1d", HTTPClient: httpClient}
		store := ingestion.NewMemStore()
		pipe := &ingestion.Pipeline{Fetcher: fetcher, Deduper: &ingestion.MemDeduper{}, Store: store}
		if err := pipe.Run(ctx, evt.Asset, shared.TimeRange{From: from, To: to}); err != nil {
			details = append(details, fmt.Sprintf("%s: fetch failed: %v", evt.ID, err))
			continue
		}

		prices, err := store.Get(ctx, evt.Asset, shared.TimeRange{From: from, To: to})
		if err != nil || len(prices) < windowLen+250 {
			details = append(details, fmt.Sprintf("%s: insufficient data (%d pts)", evt.ID, len(prices)))
			continue
		}
		fmt.Fprintf(os.Stderr, "  halving %s: %d price points\n", evt.ID, len(prices))

		wb := signalmatrix.NewWindowBuilderImpl(prices)
		pw, err := wb.BuildWindows(evt)
		if err != nil {
			details = append(details, fmt.Sprintf("%s: BuildWindows: %v", evt.ID, err))
			continue
		}

		lr := func(pp []shared.PricePoint) []float64 {
			return shared.PriceWindow{}.LogReturns(pp)
		}
		riL2 := lr(pw.Event)
		car := 0.0
		for j := 10; j < len(riL2) && j < 10+windowLen; j++ {
			car += riL2[j]
		}
		allCARs = append(allCARs, car)
		details = append(details, fmt.Sprintf("%s: CAR=%.4f", evt.ID, car))
	}

	if len(allCARs) == 0 {
		r.passed = false
		r.actual = "0 events had sufficient data"
		r.detail = fmt.Sprintf("fallbacks: %s", fmt.Sprint(details))
		return r
	}

	meanCAR := aggregation.MeanCAR(allCARs)
	positive := 0
	for _, c := range allCARs {
		if c > 0 {
			positive++
		}

	}
	r.passed = meanCAR > 0
	r.actual = fmt.Sprintf("mean CAR = %.4f (%d/%d positive)", meanCAR, positive, len(allCARs))
	r.detail = fmt.Sprintf("CARs: %v", allCARs)
	return r
}



// ---- Fixture (synthetic data) benchmarks ----
// These generate controlled data matching known research parameters.
// No external APIs needed. Runs anywhere the Rust compute_server is available.

func makeFixturePrices(nDays int, base time.Time, marketChg func(i int) float64, stockBeta float64) ([]shared.PricePoint, []shared.PricePoint) {
	mkT := make([]shared.PricePoint, nDays)
	stkT := make([]shared.PricePoint, nDays)
	mkT[0] = shared.PricePoint{AssetID: "SPY", Timestamp: base, Close: 100, Volume: 1}
	stkT[0] = shared.PricePoint{AssetID: "STK", Timestamp: base, Close: 100, Volume: 1}
	for i := 1; i < nDays; i++ {
		rm := marketChg(i)
		mkT[i] = shared.PricePoint{AssetID: "SPY", Timestamp: base.AddDate(0, 0, i), Close: mkT[i-1].Close * math.Exp(rm), Volume: 1}
		re := 0.0001 + stockBeta*rm + 0.005*rand.NormFloat64()
		stkT[i] = shared.PricePoint{AssetID: "STK", Timestamp: base.AddDate(0, 0, i), Close: stkT[i-1].Close * math.Exp(re), Volume: 1}
	}
	return mkT, stkT
}

func runHalvingFixtureBenchmark(ctx context.Context, bridge *signalmatrix.RustBridge) benchmarkResult {
	r := benchmarkResult{name: "BTC Halving CAR (fixture)"}
	r.expected = "positive mean CAR(0,+30) across 4 events"

	nDays := 500
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	drift := []float64{0.003, 0.004, 0.002, 0.005}

	var allCARs []float64
	var details []string

	for i, hdrift := range drift {
		mkPrices, stkPrices := makeFixturePrices(nDays, base, func(i int) float64 {
			return 0.0002 + 0.005*(float64(i%7)/7.0-0.5)
		}, 1.0)

		t0Idx := 200 + i*50

		// Inject post-halving drift (no noise — fixture data)
		for j := t0Idx + 1; j < nDays && j <= t0Idx+31; j++ {
			prevClose := stkPrices[j-1].Close
			rm := math.Log(mkPrices[j].Close / mkPrices[j-1].Close)
			re := 0.0001 + 1.0*rm + hdrift
			stkPrices[j] = shared.PricePoint{
				AssetID: "STK", Timestamp: base.AddDate(0, 0, j),
				Close: prevClose * math.Exp(re), Volume: 1,
			}
		}

		store := ingestion.NewMemStore()
		if err := store.Put(ctx, mkPrices); err != nil {
			details = append(details, fmt.Sprintf("event%d: store failed", i))
			continue
		}
		if err := store.Put(ctx, stkPrices); err != nil {
			details = append(details, fmt.Sprintf("event%d: store failed", i))
			continue
		}

		evt := shared.Event{
			ID:        fmt.Sprintf("halving-fixture-%d", i),
			Type:      "halving_fixture",
			Timestamp: stkPrices[t0Idx].Timestamp,
			Asset:     "STK",
		}
		wb := signalmatrix.NewWindowBuilderImpl(stkPrices)
		pw, err := wb.BuildWindows(evt)
		if err != nil {
			details = append(details, fmt.Sprintf("event%d: BuildWindows: %v", i, err))
			continue
		}

		lr := func(pp []shared.PricePoint) []float64 {
			return shared.PriceWindow{}.LogReturns(pp)
		}
		riL2 := lr(pw.Event)
		car := 0.0
		for j := 10; j < len(riL2) && j < 41; j++ {
			car += riL2[j]
		}
		allCARs = append(allCARs, car)
		details = append(details, fmt.Sprintf("event%d: drift=%.3f CAR=%.4f", i, hdrift, car))
	}

	if len(allCARs) == 0 {
		r.passed = false
		r.actual = "0 events"
		return r
	}

	meanCAR := aggregation.MeanCAR(allCARs)
	positive := 0
	for _, c := range allCARs {
		if c > 0 {
			positive++
		}
	}
	r.passed = meanCAR > 0
	r.actual = fmt.Sprintf("mean CAR=%.4f (%d/%d positive)", meanCAR, positive, len(allCARs))
	r.detail = fmt.Sprintf("CARs: %v", allCARs)
	return r
}
