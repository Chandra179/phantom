package strategy

import (
	"context"
	"math"
	"os/exec"
	"testing"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/ingestion"
	"phantom/pkg/shared"
	"phantom/pkg/signalmatrix"
)

func TestSlice7_E2E_IngestionThroughStrategy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e under -short")
	}

	binary := "/home/koala/Work/phantom/rust/compute_server/target/debug/compute_server"
	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start compute_server: %v — run 'make build-rust' first", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	time.Sleep(200 * time.Millisecond)

	bridge, err := signalmatrix.NewRustBridge("[::1]:50051")
	if err != nil {
		t.Fatalf("NewRustBridge: %v", err)
	}
	ctx := context.Background()

	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	n := 400
	t0Idx := 260

	marketCloses := make([]float64, n)
	marketCloses[0] = 100.0
	for i := 1; i < n; i++ {
		rm := 0.0002 + 0.005*(float64(i%7)/7.0-0.5)
		marketCloses[i] = marketCloses[i-1] * math.Exp(rm)
	}

	type stockDef struct {
		name  string
		drift float64
	}
	stocks := []stockDef{
		{"POS", 0.008},
		{"WEAK", 0.003},
		{"NEG", -0.003},
		{"FLAT", 0.0},
	}

	store := ingestion.NewMemStore()

	var mktPrices []shared.PricePoint
	for i := 0; i < n; i++ {
		mktPrices = append(mktPrices, shared.PricePoint{
			AssetID: "SPY", Timestamp: base.AddDate(0, 0, i),
			Close: marketCloses[i], Volume: 1,
		})
	}
	if err := store.Put(ctx, mktPrices); err != nil {
		t.Fatalf("store.Put market: %v", err)
	}

	eventLookup := signalmatrix.NewMemEventLookup()

	for _, s := range stocks {
		closes := make([]float64, n)
		closes[0] = 100.0
		for i := 1; i < n; i++ {
			rm := math.Log(marketCloses[i] / marketCloses[i-1])
			ri := 0.0001 + 1.0*rm
			if i > t0Idx && i <= t0Idx+5 {
				ri += s.drift
			}
			closes[i] = closes[i-1] * math.Exp(ri)
		}

		var prices []shared.PricePoint
		for i := 0; i < n; i++ {
			prices = append(prices, shared.PricePoint{
				AssetID: shared.AssetID(s.name), Timestamp: base.AddDate(0, 0, i),
				Close: closes[i], Volume: 1,
			})
		}
		if err := store.Put(ctx, prices); err != nil {
			t.Fatalf("store.Put %s: %v", s.name, err)
		}

		evt := shared.Event{
			ID: s.name, Type: "slice7_test",
			Timestamp: base.AddDate(0, 0, t0Idx),
			Asset:     shared.AssetID(s.name),
		}
		if err := eventLookup.SaveHistorical(ctx, evt); err != nil {
			t.Fatalf("SaveHistorical %s: %v", s.name, err)
		}
	}

	events, err := eventLookup.LoadHistorical(ctx, "slice7_test")
	if err != nil {
		t.Fatalf("LoadHistorical: %v", err)
	}
	if len(events) != len(stocks) {
		t.Fatalf("expected %d events, got %d", len(stocks), len(events))
	}

	l1Start := t0Idx - 250
	l1End := t0Idx - 11
	l2Start := t0Idx - 10
	l2End := t0Idx + 10

	mkL1 := mktPrices[l1Start : l1End+1]
	mkL2 := mktPrices[l2Start : l2End+1]

	lr := func(series []shared.PricePoint) []float64 {
		return shared.PriceWindow{}.LogReturns(series)
	}

	var cars []float64
	var sigmas []float64
	var entryPrices []float64
	var exitPrices []float64

	for _, evt := range events {
		allPrices, err := store.Get(ctx, evt.Asset, shared.TimeRange{
			From: base,
			To:   base.AddDate(0, 0, n-1),
		})
		if err != nil {
			t.Fatalf("store.Get %s: %v", evt.Asset, err)
		}

		if len(allPrices) != n {
			t.Fatalf("expected %d prices for %s, got %d", n, evt.Asset, len(allPrices))
		}

		wb := signalmatrix.NewWindowBuilderImpl(allPrices)
		pw, err := wb.BuildWindows(evt)
		if err != nil {
			t.Fatalf("BuildWindows %s: %v", evt.Asset, err)
		}

		riL1 := lr(pw.Estimation)
		rmL1 := lr(mkL1)
		riL2 := lr(pw.Event)
		rmL2 := lr(mkL2)

		alpha, beta, sigmaEps, err := bridge.OLSMarketModel(ctx, riL1, rmL1)
		if err != nil {
			t.Fatalf("OLSMarketModel %s: %v", evt.Asset, err)
		}
		ar, err := bridge.AbnormalReturn(ctx, riL2, rmL2, alpha, beta)
		if err != nil {
			t.Fatalf("AbnormalReturn %s: %v", evt.Asset, err)
		}

		car05 := 0.0
		for j := 10; j <= 15; j++ {
			car05 += ar[j]
		}
		cars = append(cars, car05)
		sigmas = append(sigmas, sigmaEps*math.Sqrt(6))

		entryPrices = append(entryPrices, pw.Event[11].Close)
		exitPrices = append(exitPrices, pw.Event[16].Close)
	}

	threshold := 0.02
	trades := SimulateTrades(cars, entryPrices, exitPrices, threshold, 5)
	result := ComputeMetrics(trades, 0, 252)

	t.Logf("cars=%v meanCAR=%f", cars, aggregation.MeanCAR(cars))
	t.Logf("trades=%d sharpe=%f hitRate=%f totalReturn=%f",
		result.NumTrades, result.SharpeRatio, result.HitRate, result.TotalReturn)

	if result.NumTrades < 1 {
		t.Error("expected >=1 trade")
	}
	if result.HitRate <= 0 {
		t.Error("hit rate should be positive")
	}

	meanCAR := aggregation.MeanCAR(cars)
	if meanCAR <= 0 {
		t.Errorf("mean CAR = %f, want > 0 (more positive than negative drift stocks)", meanCAR)
	}

	tStat := aggregation.CrossSectionalTTest(cars)
	t.Logf("cross-sectional t-stat=%f", tStat)

	bmpStat := aggregation.BMPTest(cars, sigmas)
	t.Logf("BMP test stat=%f", bmpStat)
}
