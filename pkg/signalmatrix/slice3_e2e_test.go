package signalmatrix

import (
	"context"
	"math"
	"os/exec"
	"testing"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/shared"
)

// TestSlice3_E2E_CARSignMatchesBallBrown implements Slice-3 e2e from IMPLEMENTATION_PLAN.md.
// Synthetic EDGAR 8-K earnings events: good-news stock drifts up after T0,
// bad-news stock drifts down. CAR sign must match surprise direction (Ball & Brown 1968).
func TestSlice3_E2E_CARSignMatchesBallBrown(t *testing.T) {
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

	// Good stock: positive drift days T0+1..T0+5
	goodCloses := make([]float64, n)
	goodCloses[0] = 100.0
	for i := 1; i < n; i++ {
		rm := math.Log(marketCloses[i] / marketCloses[i-1])
		ri := 0.0001 + 1.1*rm
		if i > t0Idx && i <= t0Idx+5 {
			ri += 0.004
		}
		goodCloses[i] = goodCloses[i-1] * math.Exp(ri)
	}

	// Bad stock: negative drift days T0+1..T0+5
	badCloses := make([]float64, n)
	badCloses[0] = 100.0
	for i := 1; i < n; i++ {
		rm := math.Log(marketCloses[i] / marketCloses[i-1])
		ri := 0.0001 + 0.9*rm
		if i > t0Idx && i <= t0Idx+5 {
			ri -= 0.004
		}
		badCloses[i] = badCloses[i-1] * math.Exp(ri)
	}

	mkPrices := make([]shared.PricePoint, n)
	goodPrices := make([]shared.PricePoint, n)
	badPrices := make([]shared.PricePoint, n)
	for i := 0; i < n; i++ {
		ts := base.AddDate(0, 0, i)
		mkPrices[i] = shared.PricePoint{AssetID: "SPY", Timestamp: ts, Close: marketCloses[i], Volume: 1}
		goodPrices[i] = shared.PricePoint{AssetID: "GOOD", Timestamp: ts, Close: goodCloses[i], Volume: 1}
		badPrices[i] = shared.PricePoint{AssetID: "BAD", Timestamp: ts, Close: badCloses[i], Volume: 1}
	}

	evtGood := shared.Event{ID: "g1", Type: "edgar_8k_earnings", Timestamp: goodPrices[t0Idx].Timestamp, Asset: "GOOD"}
	evtBad := shared.Event{ID: "b1", Type: "edgar_8k_earnings", Timestamp: badPrices[t0Idx].Timestamp, Asset: "BAD"}

	wbGood := NewWindowBuilderImpl(goodPrices)
	pwGood, err := wbGood.BuildWindows(evtGood)
	if err != nil {
		t.Fatalf("BuildWindows good: %v", err)
	}
	wbBad := NewWindowBuilderImpl(badPrices)
	pwBad, err := wbBad.BuildWindows(evtBad)
	if err != nil {
		t.Fatalf("BuildWindows bad: %v", err)
	}

	// L1 / L2 indices known after BuildWindows success
	l1Start := t0Idx - 250
	l1End := t0Idx - 11
	l2Start := t0Idx - 10
	l2End := t0Idx + 10

	mkL1 := mkPrices[l1Start : l1End+1]
	mkL2 := mkPrices[l2Start : l2End+1]

	lr := func(series []shared.PricePoint) []float64 {
		return shared.PriceWindow{}.LogReturns(series)
	}

	riGoodL1 := lr(pwGood.Estimation)
	rmGoodL1 := lr(mkL1)
	riGoodL2 := lr(pwGood.Event)
	rmGoodL2 := lr(mkL2)

	alphaG, betaG, _, err := bridge.OLSMarketModel(ctx, riGoodL1, rmGoodL1)
	if err != nil {
		t.Fatalf("OLSMarketModel good: %v", err)
	}
	arGood, err := bridge.AbnormalReturn(ctx, riGoodL2, rmGoodL2, alphaG, betaG)
	if err != nil {
		t.Fatalf("AbnormalReturn good: %v", err)
	}
	carGood, err := bridge.CumulativeAbnormalReturn(ctx, arGood)
	if err != nil {
		t.Fatalf("CAR good: %v", err)
	}

	riBadL1 := lr(pwBad.Estimation)
	rmBadL1 := lr(mkL1)
	riBadL2 := lr(pwBad.Event)
	rmBadL2 := lr(mkL2)

	alphaB, betaB, _, err := bridge.OLSMarketModel(ctx, riBadL1, rmBadL1)
	if err != nil {
		t.Fatalf("OLSMarketModel bad: %v", err)
	}
	arBad, err := bridge.AbnormalReturn(ctx, riBadL2, rmBadL2, alphaB, betaB)
	if err != nil {
		t.Fatalf("AbnormalReturn bad: %v", err)
	}
	carBad, err := bridge.CumulativeAbnormalReturn(ctx, arBad)
	if err != nil {
		t.Fatalf("CAR bad: %v", err)
	}

	// Ball-Brown: sign(CAR) matches earnings surprise direction
	if carGood <= 0 {
		t.Errorf("good-news CAR = %f, want > 0", carGood)
	}
	if carBad >= 0 {
		t.Errorf("bad-news CAR = %f, want < 0", carBad)
	}

	meanCAR := aggregation.MeanCAR([]float64{carGood, carBad})
	t.Logf("CAR good=%f bad=%f mean=%f", carGood, carBad, meanCAR)
}
