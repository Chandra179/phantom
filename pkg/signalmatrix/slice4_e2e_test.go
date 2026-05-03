package signalmatrix

import (
	"context"
	"math"
	"os/exec"
	"testing"
	"time"

	"phantom/pkg/shared"
)

// TestSlice4_E2E_BTCHalvingCAR computes CAR for the 2024 BTC halving
// using synthetic data and the real Rust gRPC compute_server.
//
// BTC shows positive abnormal returns in the weeks following halving
// (supply-shock narrative). This test verifies the full pipeline:
// halving seed → window builder → Rust market-model AR/CAR.
func TestSlice4_E2E_BTCHalvingCAR(t *testing.T) {
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

	bridge, err := NewRustBridge("[::1]:50051")
	if err != nil {
		t.Fatalf("NewRustBridge: %v", err)
	}
	ctx := context.Background()

	// Synthetic daily price series: 400 bars, T0 at index 260
	n := 400
	t0Idx := 260
	base := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)

	// Market index (crypto basket) — random-ish walk
	mktCloses := make([]float64, n)
	mktCloses[0] = 1000.0
	for i := 1; i < n; i++ {
		rm := 0.0003 + 0.004*(float64(i%7)/7.0-0.5)
		mktCloses[i] = mktCloses[i-1] * math.Exp(rm)
	}

	// BTC: correlated with market + post-halving drift (days T0+1..T0+10)
	btcCloses := make([]float64, n)
	btcCloses[0] = 30000.0
	for i := 1; i < n; i++ {
		rm := math.Log(mktCloses[i] / mktCloses[i-1])
		ri := 0.0002 + 1.2*rm
		if i > t0Idx && i <= t0Idx+10 {
			ri += 0.003 // +0.3% daily abnormal return post-halving
		}
		btcCloses[i] = btcCloses[i-1] * math.Exp(ri)
	}

	mktPrices := make([]shared.PricePoint, n)
	btcPrices := make([]shared.PricePoint, n)
	for i := 0; i < n; i++ {
		ts := base.AddDate(0, 0, i)
		mktPrices[i] = shared.PricePoint{AssetID: "CRYPTOBASKET", Timestamp: ts, Close: mktCloses[i], Volume: 1}
		btcPrices[i] = shared.PricePoint{AssetID: "BTCUSDT", Timestamp: ts, Close: btcCloses[i], Volume: 1}
	}

	// Halving event at T0
	evt := shared.Event{
		ID:        "btc-halving-2024",
		Type:      HalvingEventType,
		Timestamp: btcPrices[t0Idx].Timestamp,
		Asset:     "BTCUSDT",
	}

	wb := NewWindowBuilderImpl(btcPrices)
	pw, err := wb.BuildWindows(evt)
	if err != nil {
		t.Fatalf("BuildWindows: %v", err)
	}

	// Window indices (known after BuildWindows succeeds)
	l1Start := t0Idx - 250
	l1End := t0Idx - 11
	l2Start := t0Idx - 10
	l2End := t0Idx + 10

	mkL1 := mktPrices[l1Start : l1End+1]
	mkL2 := mktPrices[l2Start : l2End+1]

	lr := func(series []shared.PricePoint) []float64 {
		return shared.PriceWindow{}.LogReturns(series)
	}

	riL1 := lr(pw.Estimation)
	rmL1 := lr(mkL1)
	riL2 := lr(pw.Event)
	rmL2 := lr(mkL2)

	alpha, beta, _, err := bridge.OLSMarketModel(ctx, riL1, rmL1)
	if err != nil {
		t.Fatalf("OLSMarketModel: %v", err)
	}

	ar, err := bridge.AbnormalReturn(ctx, riL2, rmL2, alpha, beta)
	if err != nil {
		t.Fatalf("AbnormalReturn: %v", err)
	}

	car, err := bridge.CumulativeAbnormalReturn(ctx, ar)
	if err != nil {
		t.Fatalf("CumulativeAbnormalReturn: %v", err)
	}

	t.Logf("BTC halving 2024: alpha=%f beta=%f CAR=%f", alpha, beta, car)

	if car <= 0 {
		t.Errorf("post-halving CAR = %f, want > 0 (positive drift expected)", car)
	}
}
