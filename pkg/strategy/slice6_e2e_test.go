package strategy

import (
	"context"
	"math"
	"os/exec"
	"testing"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/shared"
	"phantom/pkg/signalmatrix"
)

func TestSlice6_E2E_FullPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e under -short")
	}

	binary := "/home/koala/Work/phantom/rust/compute_server/target/debug/compute_server"
	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start compute_server: %v — run 'make build-rust' first", err)
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

	// Market prices
	marketCloses := make([]float64, n)
	marketCloses[0] = 100.0
	for i := 1; i < n; i++ {
		rm := 0.0002 + 0.005*(float64(i%7)/7.0-0.5)
		marketCloses[i] = marketCloses[i-1] * math.Exp(rm)
	}
	mkPrices := make([]shared.PricePoint, n)
	for i := 0; i < n; i++ {
		mkPrices[i] = shared.PricePoint{
			AssetID: "SPY", Timestamp: base.AddDate(0, 0, i),
			Close: marketCloses[i], Volume: 1,
		}
	}

	l1Start := t0Idx - 250
	l1End := t0Idx - 11
	l2Start := t0Idx - 10
	l2End := t0Idx + 10
	mkL1 := mkPrices[l1Start : l1End+1]
	mkL2 := mkPrices[l2Start : l2End+1]

	// 4 stocks: strong-positive, weak-positive, negative, flat event impact
	type stockCase struct {
		name     string
		drift    float64 // extra daily log-return T0+1..T0+5
		wantCAR  string  // "pos", "neg", "zero"
		wantTrd  bool
	}
	cases := []stockCase{
		{"STRONG", 0.008, "pos", true},  // ~4% CAR
		{"WEAK", 0.003, "pos", false},   // ~1.5% CAR < threshold
		{"NEG", -0.003, "neg", false},
		{"FLAT", 0.0, "zero", false},
	}

	lr := func(series []shared.PricePoint) []float64 {
		return shared.PriceWindow{}.LogReturns(series)
	}

	var cars []float64
	var entryPrices []float64
	var exitPrices []float64

	for _, c := range cases {
		closes := make([]float64, n)
		closes[0] = 100.0
		for i := 1; i < n; i++ {
			rm := math.Log(marketCloses[i] / marketCloses[i-1])
			ri := 0.0001 + 1.0*rm
			if i > t0Idx && i <= t0Idx+5 {
				ri += c.drift
			}
			closes[i] = closes[i-1] * math.Exp(ri)
		}

		prices := make([]shared.PricePoint, n)
		for i := 0; i < n; i++ {
			prices[i] = shared.PricePoint{
				AssetID: shared.AssetID(c.name), Timestamp: base.AddDate(0, 0, i),
				Close: closes[i], Volume: 1,
			}
		}

		evt := shared.Event{
			ID: c.name, Type: "test_event",
			Timestamp: prices[t0Idx].Timestamp,
			Asset:     shared.AssetID(c.name),
		}
		wb := signalmatrix.NewWindowBuilderImpl(prices)
		pw, err := wb.BuildWindows(evt)
		if err != nil {
			t.Fatalf("BuildWindows %s: %v", c.name, err)
		}

		riL1 := lr(pw.Estimation)
		rmL1 := lr(mkL1)
		riL2 := lr(pw.Event)
		rmL2 := lr(mkL2)

		alpha, beta, _, err := bridge.OLSMarketModel(ctx, riL1, rmL1)
		if err != nil {
			t.Fatalf("OLSMarketModel %s: %v", c.name, err)
		}
		ar, err := bridge.AbnormalReturn(ctx, riL2, rmL2, alpha, beta)
		if err != nil {
			t.Fatalf("AbnormalReturn %s: %v", c.name, err)
		}

		// CAR(0,+5) = sum of AR at indices 10-15 (T0 to T0+5)
		car05 := 0.0
		for j := 10; j <= 15; j++ {
			car05 += ar[j]
		}
		cars = append(cars, car05)

		// Entry = T0+1 (Event[11]), Exit = T0+6 (Event[16])
		entryPrices = append(entryPrices, pw.Event[11].Close)
		exitPrices = append(exitPrices, pw.Event[16].Close)

		// Verify CAR sign
		switch c.wantCAR {
		case "pos":
			if car05 <= 0 {
				t.Errorf("%s CAR(0,+5)=%f, want >0", c.name, car05)
			}
		case "neg":
			if car05 >= 0 {
				t.Errorf("%s CAR(0,+5)=%f, want <0", c.name, car05)
			}
		}
	}

	// Strategy simulation: CAR(0,+5) > 2% threshold, 5bps slippage
	threshold := 0.02
	trades := SimulateTrades(cars, entryPrices, exitPrices, threshold, 5)

	// STRONG should trade (~4% CAR > 2%), others should not
	if c := cases[0]; c.wantTrd {
		found := false
		for _, tr := range trades {
			if tr.Return > 0 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("STRONG stock should produce profitable trade, got %d trades", len(trades))
		}
	}

	if len(trades) == 0 {
		t.Fatal("expected at least 1 trade, got 0")
	}

	result := ComputeMetrics(trades, 0, 252)
	t.Logf("trades=%d sharpe=%f maxDD=%f hitRate=%f totalReturn=%f",
		result.NumTrades, result.SharpeRatio, result.MaxDrawdown, result.HitRate, result.TotalReturn)

	// Sanity checks
	if result.NumTrades < 1 {
		t.Error("expected >=1 trade")
	}
	if result.HitRate <= 0 {
		t.Error("hit rate should be positive for drift-up stocks")
	}

	// Also compute cross-sectional mean CAR for reporting
	meanCAR := aggregation.MeanCAR(cars)
	t.Logf("cars=%v meanCAR=%f threshold=%f", cars, meanCAR, threshold)
}
