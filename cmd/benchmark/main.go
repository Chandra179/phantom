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
	"sort"
	"strings"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/ingestion"
	"phantom/pkg/shared"
	"phantom/pkg/signalmatrix"
)

// --- report types -----------------------------------------------------------

type DateInfo struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type EventInfo struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Asset     string `json:"asset"`
}

type DataInfo struct {
	FetchRange DateInfo `json:"fetch_range"`
	PriceCount int      `json:"price_count"`
	DateRange  DateInfo `json:"date_range"`
}

type WindowInfo struct {
	NStart int    `json:"n_start"`
	NEnd   int    `json:"n_end"`
	NObs   int    `json:"n_obs"`
	Start  string `json:"start"`
	End    string `json:"end"`
}

type MarketModel struct {
	Alpha    float64 `json:"alpha"`
	Beta     float64 `json:"beta"`
	SigmaEps float64 `json:"sigma_eps"`
	RSquared float64 `json:"r_squared"`
	NObs     int     `json:"n_obs"`
}

type EventResult struct {
	ID        string       `json:"id"`
	Event     EventInfo    `json:"event"`
	Data      DataInfo     `json:"data"`
	L1Window  *WindowInfo  `json:"l1,omitempty"`
	L2Window  *WindowInfo  `json:"l2"`
	MarketModel *MarketModel `json:"market_model,omitempty"`
	AR        []float64    `json:"ar,omitempty"`
	CAR       float64      `json:"car"`
	Status    string       `json:"status"`
	Error     string       `json:"error,omitempty"`
}

type CrossSectionalStats struct {
	NEvents       int       `json:"n_events"`
	CARS          []float64 `json:"cars"`
	MeanCAR       float64   `json:"mean_car"`
	StdCAR        float64   `json:"std_car"`
	MinCAR        float64   `json:"min_car"`
	MaxCAR        float64   `json:"max_car"`
	MedianCAR     float64   `json:"median_car"`
	PositiveCount int       `json:"positive_count"`
	PositiveRatio float64   `json:"positive_ratio"`
	TStat         float64   `json:"t_stat"`
	BMPTStat      *float64  `json:"bmp_t_stat,omitempty"`
	KPTStat       *float64  `json:"kp_t_stat,omitempty"`
	RBar          *float64  `json:"r_bar,omitempty"`
}

type VerdictInfo struct {
	Passed   bool   `json:"passed"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

type ConfigInfo struct {
	EstimationWindowDays  int `json:"estimation_window_days"`
	EstimationMinObs      int `json:"estimation_min_obs"`
	CARWindowDays         int `json:"car_window_days"`
	PostEventDays         int `json:"post_event_days"`
}

type BenchmarkReport struct {
	Benchmark    string              `json:"benchmark"`
	Mode         string              `json:"mode"`
	Timestamp    string              `json:"timestamp"`
	Config       ConfigInfo          `json:"config"`
	Events       []EventResult       `json:"events"`
	CrossSectional CrossSectionalStats `json:"cross_sectional"`
	Verdict      VerdictInfo         `json:"verdict"`
	Summary      string              `json:"summary"`
}

// --- main -------------------------------------------------------------------

func main() {
	rustBin := flag.String("rust-bin", "", "path to compute_server binary (default: debug then release)")
	outputFmt := flag.String("output", "text", "output format: text or json")
	skipHalving := flag.Bool("skip-halving", false, "skip halving benchmark")
	insecure := flag.Bool("insecure", false, "skip TLS verification (needed if system clock is skewed)")
	useFixtures := flag.Bool("fixtures", false, "use synthetic fixture data instead of live APIs")
	outputDir := flag.String("output-dir", "benchmarks", "directory to save JSON result history")
	geopolitics := flag.Bool("geopolitics", false, "run geopolitics event-study benchmark (wikipedia events)")
	eventType := flag.String("event-type", "war", "geopolitics event type: war, election, sanction")
	assets := flag.String("assets", "SPY,GLD,USO", "comma-separated assets for geopolitics benchmark")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	mode := "live"
	label := "Live Data"
	if *useFixtures {
		mode = "fixture"
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
	fmt.Fprintf(os.Stderr, "Rust compute_server: pid=%d\n", computeCmd.Process.Pid)
	time.Sleep(500 * time.Millisecond)

	bridge, err := signalmatrix.NewRustBridge("[::1]:50051")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: NewRustBridge: %v\n", err)
		computeCmd.Process.Kill()
		os.Exit(1)
	}
	defer computeCmd.Process.Kill()

	cfg := ConfigInfo{
		EstimationWindowDays: 250,
		EstimationMinObs:     200,
		CARWindowDays:        31,
		PostEventDays:        30,
	}

	var report BenchmarkReport

	if *geopolitics {
		assetList := parseAssets(*assets)
		report = runGeopoliticsBenchmark(ctx, bridge, cfg, mode, shared.EventType(*eventType), assetList, *insecure)
	} else if *useFixtures {
		if !*skipHalving {
			report = runHalvingFixtureBenchmark(ctx, bridge, cfg, mode)
		}
	} else {
		if !*skipHalving {
			report = runHalvingBenchmark(ctx, bridge, cfg, mode, *insecure)
		}
	}

	switch *outputFmt {
	case "json":
		printJSON(report)
	default:
		printText(report)
	}

	if err := saveJSON(report, *outputDir); err != nil {
		fmt.Fprintf(os.Stderr, "save results: %v\n", err)
	}

	if report.Verdict.Passed {
		fmt.Fprintf(os.Stderr, "\n1/1 benchmarks passed\n")
	} else {
		fmt.Fprintf(os.Stderr, "\n0/1 benchmarks passed\n")
		os.Exit(1)
	}
}

// --- output helpers ---------------------------------------------------------

func printText(r BenchmarkReport) {
	fmt.Printf("\n %s\n", r.Benchmark)
	fmt.Printf("   Mode:     %s\n", r.Mode)
	fmt.Printf("   Expected: %s\n", r.Verdict.Expected)
	fmt.Printf("   Actual:   %s\n", r.Verdict.Actual)
	fmt.Printf("   Result:   %s\n", passFail(r.Verdict.Passed))

	cs := r.CrossSectional
	fmt.Printf("   Events:   %d (%d positive, %.0f%%)\n", cs.NEvents, cs.PositiveCount, cs.PositiveRatio*100)
	fmt.Printf("   Mean CAR: %.6f (std=%.6f)\n", cs.MeanCAR, cs.StdCAR)
	fmt.Printf("   CARs:     %v\n", cs.CARS)
	fmt.Printf("   t-stat:   %.4f\n", cs.TStat)
	if cs.BMPTStat != nil {
		fmt.Printf("   BMP t:    %.4f\n", *cs.BMPTStat)
	}
	if cs.KPTStat != nil {
		fmt.Printf("   KP t:     %.4f (r̄=%.4f)\n", *cs.KPTStat, *cs.RBar)
	}

	for _, ev := range r.Events {
		fmt.Fprintf(os.Stderr, "  %s: CAR=%.6f [%s]\n", ev.ID, ev.CAR, ev.Status)
	}
}

func passFail(p bool) string {
	if p {
		return "PASS"
	}
	return "FAIL"
}

func printJSON(r BenchmarkReport) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

func saveJSON(r BenchmarkReport, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	r.Timestamp = time.Now().UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	fname := fmt.Sprintf("benchmark_%s.json", time.Now().UTC().Format("20060102T150405"))
	return os.WriteFile(filepath.Join(dir, fname), data, 0644)
}

// --- report builder ---------------------------------------------------------

func buildReport(name string, cfg ConfigInfo, mode string, events []EventResult) BenchmarkReport {
	cs := computeCrossSectional(events)
	verdict := VerdictInfo{
		Passed:   cs.MeanCAR > 0,
		Expected: "positive mean CAR(0,+30) across events",
		Actual:   fmt.Sprintf("mean CAR = %.6f (%d/%d positive)", cs.MeanCAR, cs.PositiveCount, cs.NEvents),
	}
	summary := generateSummary(name, mode, cfg, events, cs, verdict)
	return BenchmarkReport{
		Benchmark: name,
		Mode:      mode,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Config:    cfg,
		Events:    events,
		CrossSectional: cs,
		Verdict:   verdict,
		Summary:   summary,
	}
}

func generateSummary(name, mode string, cfg ConfigInfo, events []EventResult, cs CrossSectionalStats, v VerdictInfo) string {
	hasMM := false
	for _, ev := range events {
		if ev.MarketModel != nil {
			hasMM = true
			break
		}
	}

	dataSrc := "live market data (Binance)"
	if mode == "fixture" {
		dataSrc = "synthetic fixture data"
	}

	nSucc := 0
	var skippedIDs []string
	for _, ev := range events {
		if ev.Status == "success" {
			nSucc++
		} else {
			skippedIDs = append(skippedIDs, ev.ID)
		}
	}

	carPct := cs.MeanCAR * 100
	lines := []string{
		fmt.Sprintf("Benchmark of %s using %s.", name, dataSrc),
		fmt.Sprintf("Measured cumulative abnormal return (CAR) over the %d trading days following each event (CAR(0,+%d)).",
			cfg.PostEventDays, cfg.PostEventDays),
	}

	if nSucc < len(events) {
		lines = append(lines, fmt.Sprintf("Analyzed %d of %d events; %d skipped (%s).",
			nSucc, len(events), len(events)-nSucc, fmt.Sprint(skippedIDs)))
	} else {
		lines = append(lines, fmt.Sprintf("All %d events analyzed successfully.", nSucc))
	}

	if hasMM {
		modelBeta := 0.0
		modelN := 0
		for _, ev := range events {
			if ev.MarketModel != nil {
				modelBeta += ev.MarketModel.Beta
				modelN++
			}
		}
		avgBeta := modelBeta / float64(modelN)
		lines = append(lines,
			fmt.Sprintf("Used OLS market model (mean beta=%.2f) on L1 estimation window [T0-%d, T0-11] (%d days, min %d obs) to compute expected returns, then derived abnormal returns (AR) on L2. CAR is sum of daily AR over [T0, T0+%d].",
				avgBeta, cfg.EstimationWindowDays, cfg.EstimationWindowDays, cfg.EstimationMinObs, cfg.PostEventDays))
	} else {
		lines = append(lines,
			fmt.Sprintf("Computed raw log-returns (no market model — single asset, no market index). CAR is sum of daily log-returns over [T0, T0+%d], representing total cumulative return post-event.",
				cfg.PostEventDays))
	}

	sig := ""
	if cs.NEvents >= 2 {
		tMag := math.Abs(cs.TStat)
		switch {
		case tMag < 1.0:
			sig = "not statistically distinguishable from zero"
		case tMag < 2.0:
			sig = "weakly suggestive but below conventional significance (|t| < 2.0)"
		case tMag < 3.0:
			sig = "statistically significant at the 5% level (|t| between 2.0 and 3.0)"
		default:
			sig = "highly statistically significant (|t| > 3.0, p < 0.003)"
		}

		bmpNote := ""
		if cs.BMPTStat != nil {
			bmpNote = fmt.Sprintf(" BMP test (robust to event-induced heteroskedasticity) gives t=%.2f.", *cs.BMPTStat)
		}
		kpNote := ""
		if cs.KPTStat != nil && cs.RBar != nil {
			kpNote = fmt.Sprintf(" Kolari-Pynnönen correction (cross-sectional correlation r̅=%.3f) adjusts to t=%.2f.", *cs.RBar, *cs.KPTStat)
		}

		dir := "positive"
		if cs.MeanCAR < 0 {
			dir = "negative"
		}

		lines = append(lines,
			fmt.Sprintf("Cross-sectional mean CAR=%.4f (%.2f%%), median=%.4f (%.2f%%), std=%.4f, range [%.4f, %.4f].",
				cs.MeanCAR, carPct, cs.MedianCAR, cs.MedianCAR*100, cs.StdCAR, cs.MinCAR, cs.MaxCAR))
		lines = append(lines,
			fmt.Sprintf("Direction: %s — %d of %d events (%.0f%%) show %s CAR.",
				dir, cs.PositiveCount, cs.NEvents, cs.PositiveRatio*100, dir))
		lines = append(lines,
			fmt.Sprintf("Naive t-test: t=%.2f (%s).%s%s",
				cs.TStat, sig, bmpNote, kpNote))
	}

	if v.Passed {
		lines = append(lines,
			fmt.Sprintf("Result: PASS — mean CAR is positive (%.2f%%), consistent with hypothesis that Bitcoin halving events are followed by net positive cumulative returns over the subsequent %d trading days.",
				carPct, cfg.PostEventDays))
	} else {
		lines = append(lines,
			fmt.Sprintf("Result: FAIL — mean CAR is negative (%.2f%%), inconsistent with positive-sign hypothesis. This does not rule out a real effect; limited sample (%d events) reduces power.",
				carPct, cs.NEvents))
	}

	if !hasMM {
		lines = append(lines,
			"Caveat: raw cumulative return reflects both event-driven movement and general market trends. Without a market model, attribution to the event itself is uncertain. For crypto single-asset studies, a crypto-market index or sector index would be needed for risk adjustment.")
	}

	if cs.NEvents < 10 {
		lines = append(lines,
			fmt.Sprintf("Caveat: small sample (%d events). Cross-sectional t-tests assume normality and independence; with N<30, results are illustrative but not definitive. As more halving events occur, power increases.", cs.NEvents))
	}

	return fmt.Sprint(lines)
}

func computeCrossSectional(events []EventResult) CrossSectionalStats {
	var cars []float64
	for _, ev := range events {
		if ev.Status == "success" {
			cars = append(cars, ev.CAR)
		}
	}

	cs := CrossSectionalStats{}
	cs.NEvents = len(cars)
	if cs.NEvents == 0 {
		return cs
	}

	cs.CARS = cars
	cs.MeanCAR = aggregation.MeanCAR(cars)

	// std
	n := float64(len(cars))
	v := 0.0
	for _, c := range cars {
		d := c - cs.MeanCAR
		v += d * d
	}
	cs.StdCAR = math.Sqrt(v / (n - 1))

	// min/max
	cs.MinCAR = cars[0]
	cs.MaxCAR = cars[0]
	for _, c := range cars[1:] {
		if c < cs.MinCAR {
			cs.MinCAR = c
		}
		if c > cs.MaxCAR {
			cs.MaxCAR = c
		}
	}

	// median
	sorted := make([]float64, len(cars))
	copy(sorted, cars)
	sort.Float64s(sorted)
	if len(sorted)%2 == 0 {
		cs.MedianCAR = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	} else {
		cs.MedianCAR = sorted[len(sorted)/2]
	}

	// positive
	for _, c := range cars {
		if c > 0 {
			cs.PositiveCount++
		}
	}
	cs.PositiveRatio = float64(cs.PositiveCount) / n

	// t-stat
	if cs.NEvents >= 2 {
		cs.TStat = aggregation.CrossSectionalTTest(cars)
	}

	// BMP/KP only if events have sigmas (fixture benchmark w/ market model)
	hasSigmas := true
	var sigmas []float64
	for _, ev := range events {
		if ev.Status == "success" {
			if ev.MarketModel == nil {
				hasSigmas = false
				break
			}
			// sigma_CAR ~ sigma_eps * sqrt(L2_window_days)
			// For CAR(0,+30), L2_window_days = 31
			// var(CAR) = L2 * sigma_eps^2 (assuming iid AR)
			l2Days := float64(len(ev.AR))
			if l2Days == 0 {
				l2Days = 31
			}
			sigmas = append(sigmas, ev.MarketModel.SigmaEps*math.Sqrt(l2Days))
		}
	}
	if hasSigmas && len(sigmas) == cs.NEvents && cs.NEvents >= 2 {
		bmp := aggregation.BMPTest(cars, sigmas)
		cs.BMPTStat = &bmp

		rBar := estimateRBar(cars, sigmas)
		cs.RBar = &rBar
		kp := aggregation.KolariPynnonen(cars, sigmas, rBar)
		cs.KPTStat = &kp
	}

	return cs
}

// estimateRBar estimates mean pairwise SCAR cross-correlation.
// For small N (4 halving events), uses shrinkage toward zero.
func estimateRBar(cars, sigmas []float64) float64 {
	n := len(cars)
	if n < 2 {
		return 0
	}
	scars := make([]float64, n)
	for i := range cars {
		scars[i] = cars[i] / sigmas[i]
	}
	var sumR float64
	var count int
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			// Pearson correlation of SCARs: with only 2 points per pair, use sign agreement
			if (scars[i] > 0 && scars[j] > 0) || (scars[i] < 0 && scars[j] < 0) {
				sumR += 0.5 // positive correlation proxy
			} else {
				sumR -= 0.5
			}
			count++
		}
	}
	if count == 0 {
		return 0
	}
	r := sumR / float64(count)
	// Shrink toward zero for small samples (James-Stein style)
	shrinkage := 1.0 - 3.0/float64(n)
	if shrinkage < 0 {
		shrinkage = 0
	}
	return r * shrinkage
}

// --- price series helpers ---------------------------------------------------

func logReturns(prices []shared.PricePoint) []float64 {
	if len(prices) < 2 {
		return nil
	}
	out := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		out[i-1] = math.Log(prices[i].Close / prices[i-1].Close)
	}
	return out
}

func closePrices(pp []shared.PricePoint) []float64 {
	out := make([]float64, len(pp))
	for i, p := range pp {
		out[i] = p.Close
	}
	return out
}

// --- BTC halving CAR benchmark (live data) ----------------------------------

var halvingEvents = []shared.Event{
	{ID: "btc-halving-2012", Type: "btc_halving", Timestamp: time.Date(2012, 11, 28, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2016", Type: "btc_halving", Timestamp: time.Date(2016, 7, 9, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2020", Type: "btc_halving", Timestamp: time.Date(2020, 5, 11, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
	{ID: "btc-halving-2024", Type: "btc_halving", Timestamp: time.Date(2024, 4, 19, 0, 0, 0, 0, time.UTC), Asset: "BTCUSDT"},
}

func runHalvingBenchmark(ctx context.Context, bridge *signalmatrix.RustBridge, cfg ConfigInfo, mode string, insecureTLS bool) BenchmarkReport {
	name := "BTC Halving CAR"

	httpClient := http.DefaultClient
	if insecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	var events []EventResult

	for _, evt := range halvingEvents {
		result := EventResult{
			ID: evt.ID,
			Event: EventInfo{
				ID: evt.ID, Type: string(evt.Type),
				Timestamp: evt.Timestamp.Format(time.RFC3339),
				Asset:     string(evt.Asset),
			},
		}

		from := evt.Timestamp.AddDate(0, 0, -365)
		to := evt.Timestamp.AddDate(0, 0, cfg.CARWindowDays+30)

		result.Data.FetchRange = DateInfo{
			From: from.Format(time.RFC3339),
			To:   to.Format(time.RFC3339),
		}

		fetcher := &ingestion.BinanceFetcher{Interval: "1d", HTTPClient: httpClient}
		store := ingestion.NewMemStore()
		pipe := &ingestion.Pipeline{Fetcher: fetcher, Deduper: &ingestion.MemDeduper{}, Store: store}
		if err := pipe.Run(ctx, evt.Asset, shared.TimeRange{From: from, To: to}); err != nil {
			result.Status = "skipped"
			result.Error = fmt.Sprintf("fetch failed: %v", err)
			events = append(events, result)
			fmt.Fprintf(os.Stderr, "  halving %s: fetch failed: %v\n", evt.ID, err)
			continue
		}

		prices, err := store.Get(ctx, evt.Asset, shared.TimeRange{From: from, To: to})
		if err != nil || len(prices) < cfg.CARWindowDays+cfg.EstimationWindowDays {
			result.Status = "skipped"
			result.Error = fmt.Sprintf("insufficient data (%d pts)", len(prices))
			events = append(events, result)
			fmt.Fprintf(os.Stderr, "  halving %s: insufficient data (%d pts)\n", evt.ID, len(prices))
			continue
		}

		result.Data.PriceCount = len(prices)
		result.Data.DateRange = DateInfo{
			From: prices[0].Timestamp.Format(time.RFC3339),
			To:   prices[len(prices)-1].Timestamp.Format(time.RFC3339),
		}

		// Find T0 index
		t0Idx := -1
		for i, p := range prices {
			if p.AssetID == evt.Asset && p.Timestamp.Equal(evt.Timestamp) {
				t0Idx = i
				break
			}
		}
		if t0Idx < 0 {
			result.Status = "skipped"
			result.Error = "T0 not found in price series"
			events = append(events, result)
			continue
		}

		fmt.Fprintf(os.Stderr, "  halving %s: %d price points, T0 at index %d\n", evt.ID, len(prices), t0Idx)

		// L1 window: [T0-250, T0-11]
		l1Start := t0Idx - cfg.EstimationWindowDays
		l1End := t0Idx - 11
		if l1Start >= 0 && l1End > l1Start && l1End < len(prices) {
			l1 := prices[l1Start : l1End+1]
			result.L1Window = &WindowInfo{
				NStart: l1Start, NEnd: l1End, NObs: len(l1),
				Start: l1[0].Timestamp.Format(time.RFC3339),
				End:   l1[len(l1)-1].Timestamp.Format(time.RFC3339),
			}
		}

		// L2 window for CAR: [T0, T0+30] inclusive = 31 days
		l2Start := t0Idx
		l2End := t0Idx + cfg.PostEventDays
		if l2End >= len(prices) {
			l2End = len(prices) - 1
		}
		l2 := prices[l2Start : l2End+1]
		result.L2Window = &WindowInfo{
			NStart: l2Start, NEnd: l2End, NObs: len(l2),
			Start: l2[0].Timestamp.Format(time.RFC3339),
			End:   l2[len(l2)-1].Timestamp.Format(time.RFC3339),
		}

		// Log returns of CAR window
		l2Returns := logReturns(l2)

		// CAR = sum of log returns over [T0, T0+30]
		car := 0.0
		for _, r := range l2Returns {
			car += r
		}
		result.CAR = car
		result.Status = "success"

		events = append(events, result)
	}

	return buildReport(name, cfg, mode, events)
}

// --- Fixture (synthetic data) benchmarks ------------------------------------

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

func runHalvingFixtureBenchmark(ctx context.Context, bridge *signalmatrix.RustBridge, cfg ConfigInfo, mode string) BenchmarkReport {
	name := "BTC Halving CAR (fixture)"

	nDays := 500
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	drifts := []float64{0.003, 0.004, 0.002, 0.005}

	var events []EventResult

	for i, hdrift := range drifts {
		mkPrices, stkPrices := makeFixturePrices(nDays, base, func(i int) float64 {
			return 0.0002 + 0.005*(float64(i%7)/7.0-0.5)
		}, 1.0)

		t0Idx := 200 + i*50

		// Inject post-halving drift into stock prices
		for j := t0Idx + 1; j < nDays && j <= t0Idx+cfg.PostEventDays; j++ {
			prevClose := stkPrices[j-1].Close
			rm := math.Log(mkPrices[j].Close / mkPrices[j-1].Close)
			re := 0.0001 + 1.0*rm + hdrift
			stkPrices[j] = shared.PricePoint{
				AssetID: "STK", Timestamp: base.AddDate(0, 0, j),
				Close: prevClose * math.Exp(re), Volume: 1,
			}
		}

		evt := shared.Event{
			ID:        fmt.Sprintf("halving-fixture-%d", i),
			Type:      "halving_fixture",
			Timestamp: stkPrices[t0Idx].Timestamp,
			Asset:     "STK",
		}

		result := EventResult{
			ID: evt.ID,
			Event: EventInfo{
				ID: evt.ID, Type: string(evt.Type),
				Timestamp: evt.Timestamp.Format(time.RFC3339),
				Asset:     string(evt.Asset),
			},
			Data: DataInfo{
				FetchRange: DateInfo{
					From: base.Format(time.RFC3339),
					To:   base.AddDate(0, 0, nDays-1).Format(time.RFC3339),
				},
				PriceCount: nDays,
				DateRange: DateInfo{
					From: stkPrices[0].Timestamp.Format(time.RFC3339),
					To:   stkPrices[nDays-1].Timestamp.Format(time.RFC3339),
				},
			},
		}

		// L1 window: [T0-250, T0-11]
		l1Start := t0Idx - cfg.EstimationWindowDays
		l1End := t0Idx - 11
		if l1Start >= 0 && l1End > l1Start && l1End < nDays {
			result.L1Window = &WindowInfo{
				NStart: l1Start, NEnd: l1End, NObs: l1End - l1Start + 1,
				Start: stkPrices[l1Start].Timestamp.Format(time.RFC3339),
				End:   stkPrices[l1End].Timestamp.Format(time.RFC3339),
			}
		}

		// L2 window: [T0, T0+30]
		l2Start := t0Idx
		l2End := t0Idx + cfg.PostEventDays
		if l2End >= nDays {
			l2End = nDays - 1
		}
		result.L2Window = &WindowInfo{
			NStart: l2Start, NEnd: l2End, NObs: l2End - l2Start + 1,
			Start: stkPrices[l2Start].Timestamp.Format(time.RFC3339),
			End:   stkPrices[l2End].Timestamp.Format(time.RFC3339),
		}

		// L1 log returns for market model
		mkL1 := mkPrices[l1Start : l1End+1]
		stkL1 := stkPrices[l1Start : l1End+1]
		mkL1Returns := logReturns(mkL1)
		stkL1Returns := logReturns(stkL1)

		// OLS market model via Rust bridge
		alpha, beta, sigmaEps, err := bridge.OLSMarketModel(ctx, stkL1Returns, mkL1Returns)
		if err != nil {
			result.Status = "skipped"
			result.Error = fmt.Sprintf("OLS failed: %v", err)
			events = append(events, result)
			fmt.Fprintf(os.Stderr, "  fixture %d: OLS failed: %v\n", i, err)
			continue
		}

		// R-squared from sigma_eps
		nObs := len(stkL1Returns)
		meanRi := 0.0
		for _, v := range stkL1Returns {
			meanRi += v
		}
		meanRi /= float64(nObs)
		sst := 0.0
		for _, v := range stkL1Returns {
			d := v - meanRi
			sst += d * d
		}
		ssr := sigmaEps * sigmaEps * float64(nObs-2)
		rSquared := 1.0 - ssr/sst

		result.MarketModel = &MarketModel{
			Alpha: alpha, Beta: beta, SigmaEps: sigmaEps,
			RSquared: rSquared, NObs: nObs,
		}

		// L2 log returns
		mkL2 := mkPrices[l2Start : l2End+1]
		stkL2 := stkPrices[l2Start : l2End+1]
		mkL2Returns := logReturns(mkL2)
		stkL2Returns := logReturns(stkL2)

		// Abnormal returns via Rust bridge
		ar, err := bridge.AbnormalReturn(ctx, stkL2Returns, mkL2Returns, alpha, beta)
		if err != nil {
			result.Status = "skipped"
			result.Error = fmt.Sprintf("AR failed: %v", err)
			events = append(events, result)
			fmt.Fprintf(os.Stderr, "  fixture %d: AR failed: %v\n", i, err)
			continue
		}
		result.AR = ar

		// CAR via Rust bridge
		car, err := bridge.CumulativeAbnormalReturn(ctx, ar)
		if err != nil {
			result.Status = "skipped"
			result.Error = fmt.Sprintf("CAR failed: %v", err)
			events = append(events, result)
			fmt.Fprintf(os.Stderr, "  fixture %d: CAR failed: %v\n", i, err)
			continue
		}
		result.CAR = car
		result.Status = "success"

		fmt.Fprintf(os.Stderr, "  fixture %d: drift=%.3f alpha=%.6f beta=%.4f σ=%.4f R²=%.4f CAR=%.6f\n",
			i, hdrift, alpha, beta, sigmaEps, rSquared, car)

		events = append(events, result)
	}

	return buildReport(name, cfg, mode, events)
}

// --- helpers -----------------------------------------------------------------

func parseAssets(s string) []shared.AssetID {
	parts := strings.Split(s, ",")
	out := make([]shared.AssetID, len(parts))
	for i, p := range parts {
		out[i] = shared.AssetID(strings.TrimSpace(p))
	}
	return out
}

// --- geopolitics CAR benchmark (Wikipedia + Yahoo Finance) --------------------

func runGeopoliticsBenchmark(ctx context.Context, bridge *signalmatrix.RustBridge, cfg ConfigInfo, mode string, eventType shared.EventType, assets []shared.AssetID, insecureTLS bool) BenchmarkReport {
	name := fmt.Sprintf("Geopolitics %s CAR", eventType)

	httpClient := http.DefaultClient
	if insecureTLS {
		httpClient = &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	lookup := signalmatrix.NewWikiEventLookup()
	wikiEvents, err := lookup.LoadHistorical(ctx, eventType)
	if err != nil {
		fmt.Fprintf(os.Stderr, "  wiki lookup failed: %v\n", err)
		return buildReport(name, cfg, mode, nil)
	}
	fmt.Fprintf(os.Stderr, "  wiki events: %d\n", len(wikiEvents))

	if len(wikiEvents) == 0 {
		return buildReport(name, cfg, mode, nil)
	}

	var events []EventResult

	for _, we := range wikiEvents {
		for _, asset := range assets {
			evt := we
			evt.Asset = asset

			result := EventResult{
				ID: fmt.Sprintf("%s-%s", we.ID, asset),
				Event: EventInfo{
					ID: evt.ID, Type: string(evt.Type),
					Timestamp: evt.Timestamp.Format(time.RFC3339),
					Asset:     string(evt.Asset),
				},
			}

			from := evt.Timestamp.AddDate(0, 0, -cfg.EstimationWindowDays-30)
			to := evt.Timestamp.AddDate(0, 0, cfg.PostEventDays+10)

			result.Data.FetchRange = DateInfo{
				From: from.Format(time.RFC3339),
				To:   to.Format(time.RFC3339),
			}

			fetcher := &ingestion.YahooFinanceFetcher{HTTPClient: httpClient}
			store := ingestion.NewMemStore()
			pipe := &ingestion.Pipeline{Fetcher: fetcher, Deduper: &ingestion.MemDeduper{}, Store: store}

			spyAsset := shared.AssetID("SPY")
			if err := pipe.Run(ctx, spyAsset, shared.TimeRange{From: from, To: to}); err != nil {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("SPY fetch failed: %v", err)
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/SPY: fetch failed: %v\n", evt.ID, err)
				continue
			}
			if err := pipe.Run(ctx, asset, shared.TimeRange{From: from, To: to}); err != nil {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("%s fetch failed: %v", asset, err)
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/%s: fetch failed: %v\n", evt.ID, asset, err)
				continue
			}

			prices, err := store.Get(ctx, asset, shared.TimeRange{From: from, To: to})
			if err != nil || len(prices) < cfg.EstimationMinObs {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("insufficient %s data (%d pts)", asset, len(prices))
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/%s: insufficient data (%d pts)\n", evt.ID, asset, len(prices))
				continue
			}

			spyPrices, err := store.Get(ctx, spyAsset, shared.TimeRange{From: from, To: to})
			if err != nil || len(spyPrices) < cfg.EstimationMinObs {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("insufficient SPY data (%d pts)", len(spyPrices))
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/SPY: insufficient data (%d pts)\n", evt.ID, len(spyPrices))
				continue
			}

			result.Data.PriceCount = len(prices)
			result.Data.DateRange = DateInfo{
				From: prices[0].Timestamp.Format(time.RFC3339),
				To:   prices[len(prices)-1].Timestamp.Format(time.RFC3339),
			}

			t0Idx := -1
			for i, p := range prices {
				if p.AssetID == asset && p.Timestamp.Equal(evt.Timestamp) {
					t0Idx = i
					break
				}
			}
			if t0Idx < 0 {
				for i, p := range prices {
					if p.AssetID == asset && !p.Timestamp.After(evt.Timestamp) {
						if t0Idx < 0 || p.Timestamp.After(prices[t0Idx].Timestamp) {
							t0Idx = i
						}
					}
				}
			}

			if t0Idx < 0 {
				result.Status = "skipped"
				result.Error = "T0 not found in price series"
				events = append(events, result)
				continue
			}

			spyT0Idx := -1
			for i, p := range spyPrices {
				if !p.Timestamp.After(prices[t0Idx].Timestamp) {
					spyT0Idx = i
				}
			}
			if spyT0Idx < 0 {
				result.Status = "skipped"
				result.Error = "SPY T0 not found"
				events = append(events, result)
				continue
			}

			fmt.Fprintf(os.Stderr, "  %s/%s: %d price pts, T0 at index %d\n", evt.ID, asset, len(prices), t0Idx)

			l1Start := t0Idx - cfg.EstimationWindowDays
			l1End := t0Idx - 11
			if l1Start >= 0 && l1End > l1Start && l1End < len(prices) {
				l1 := prices[l1Start : l1End+1]
				result.L1Window = &WindowInfo{
					NStart: l1Start, NEnd: l1End, NObs: len(l1),
					Start: l1[0].Timestamp.Format(time.RFC3339),
					End:   l1[len(l1)-1].Timestamp.Format(time.RFC3339),
				}
			}

			l2Start := t0Idx
			l2End := t0Idx + cfg.PostEventDays
			if l2End >= len(prices) {
				l2End = len(prices) - 1
			}
			l2 := prices[l2Start : l2End+1]
			result.L2Window = &WindowInfo{
				NStart: l2Start, NEnd: l2End, NObs: len(l2),
				Start: l2[0].Timestamp.Format(time.RFC3339),
				End:   l2[len(l2)-1].Timestamp.Format(time.RFC3339),
			}

			spyL1Start := spyT0Idx - cfg.EstimationWindowDays
			spyL1End := spyT0Idx - 11
			spyL2Start := spyT0Idx
			spyL2End := spyT0Idx + cfg.PostEventDays
			if spyL2End >= len(spyPrices) {
				spyL2End = len(spyPrices) - 1
			}

			if spyL1Start < 0 || spyL1End <= spyL1Start || spyL1End >= len(spyPrices) {
				result.Status = "skipped"
				result.Error = "insufficient SPY estimation window"
				events = append(events, result)
				continue
			}

			spyL1 := spyPrices[spyL1Start : spyL1End+1]
			stkL1 := prices[l1Start : l1End+1]
			spyL1Returns := logReturns(spyL1)
			stkL1Returns := logReturns(stkL1)

			alpha, beta, sigmaEps, err := bridge.OLSMarketModel(ctx, stkL1Returns, spyL1Returns)
			if err != nil {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("OLS failed: %v", err)
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/%s: OLS failed: %v\n", evt.ID, asset, err)
				continue
			}

			nObs := len(stkL1Returns)
			meanRi := 0.0
			for _, v := range stkL1Returns {
				meanRi += v
			}
			meanRi /= float64(nObs)
			sst := 0.0
			for _, v := range stkL1Returns {
				d := v - meanRi
				sst += d * d
			}
			ssr := sigmaEps * sigmaEps * float64(nObs-2)
			rSquared := 1.0 - ssr/sst

			result.MarketModel = &MarketModel{
				Alpha: alpha, Beta: beta, SigmaEps: sigmaEps,
				RSquared: rSquared, NObs: nObs,
			}

			spyL2 := spyPrices[spyL2Start : spyL2End+1]
			stkL2 := prices[l2Start : l2End+1]
			spyL2Returns := logReturns(spyL2)
			stkL2Returns := logReturns(stkL2)

			ar, err := bridge.AbnormalReturn(ctx, stkL2Returns, spyL2Returns, alpha, beta)
			if err != nil {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("AR failed: %v", err)
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/%s: AR failed: %v\n", evt.ID, asset, err)
				continue
			}
			result.AR = ar

			car, err := bridge.CumulativeAbnormalReturn(ctx, ar)
			if err != nil {
				result.Status = "skipped"
				result.Error = fmt.Sprintf("CAR failed: %v", err)
				events = append(events, result)
				fmt.Fprintf(os.Stderr, "  %s/%s: CAR failed: %v\n", evt.ID, asset, err)
				continue
			}
			result.CAR = car
			result.Status = "success"

			fmt.Fprintf(os.Stderr, "  %s/%s: α=%.6f β=%.4f σ=%.4f R²=%.4f CAR(0,+%d)=%.6f\n",
				evt.ID, asset, alpha, beta, sigmaEps, rSquared, cfg.PostEventDays, car)

			events = append(events, result)
		}
	}

	return buildReport(name, cfg, mode, events)
}
