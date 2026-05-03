package signalmatrix

import (
	"testing"
	"time"

	"phantom/pkg/shared"
)

// makePrices creates n synthetic PricePoints for asset "TEST" starting at day 0.
func makePrices(n int, asset shared.AssetID) []shared.PricePoint {
	prices := make([]shared.PricePoint, n)
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		prices[i] = shared.PricePoint{
			AssetID:   asset,
			Timestamp: base.AddDate(0, 0, i),
			Open:      float64(100 + i),
			High:      float64(105 + i),
			Low:       float64(95 + i),
			Close:     float64(100 + i),
			Volume:    1000,
		}
	}
	return prices
}

func TestWindowBuilderImpl_HappyPath(t *testing.T) {
	// 300-bar series, T0 at index 260 → L1=240 bars [10..250], L2=21 bars [250..271]
	prices := makePrices(300, "TEST")
	wb := NewWindowBuilderImpl(prices)

	t0 := prices[260]
	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: t0.Timestamp,
		Asset:     "TEST",
	}

	pw, err := wb.BuildWindows(evt)
	if err != nil {
		t.Fatalf("BuildWindows: %v", err)
	}

	// L1: [T0-250, T0-11] = [260-250, 260-11] = [10, 249] inclusive = 240 bars
	if len(pw.Estimation) != 240 {
		t.Errorf("expected 240 estimation bars, got %d", len(pw.Estimation))
	}

	// L2: [T0-10, T0+10] = [250, 270] inclusive = 21 bars
	if len(pw.Event) != 21 {
		t.Errorf("expected 21 event bars, got %d", len(pw.Event))
	}

	if pw.Asset != "TEST" {
		t.Errorf("expected asset TEST, got %s", pw.Asset)
	}

	// Verify boundaries
	if pw.Estimation[0].Timestamp != prices[10].Timestamp {
		t.Errorf("estimation should start at index 10, got %v", pw.Estimation[0].Timestamp)
	}
	if pw.Estimation[len(pw.Estimation)-1].Timestamp != prices[249].Timestamp {
		t.Errorf("estimation should end at index 249, got %v", pw.Estimation[len(pw.Estimation)-1].Timestamp)
	}
	if pw.Event[0].Timestamp != prices[250].Timestamp {
		t.Errorf("event window should start at index 250, got %v", pw.Event[0].Timestamp)
	}
	if pw.Event[len(pw.Event)-1].Timestamp != prices[270].Timestamp {
		t.Errorf("event window should end at index 270, got %v", pw.Event[len(pw.Event)-1].Timestamp)
	}
}

func TestWindowBuilderImpl_InsufficientEstimation(t *testing.T) {
	// T0 at index 200: L1 would be [200-250,..] → out of range → < 200 observations
	prices := makePrices(250, "TEST")
	wb := NewWindowBuilderImpl(prices)

	// T0 at index 50: L1 = [50-250, 50-11] = [-200, 39] → can't go negative
	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: prices[50].Timestamp,
		Asset:     "TEST",
	}

	_, err := wb.BuildWindows(evt)
	if err == nil {
		t.Fatal("expected error for insufficient estimation window")
	}

	if err.Error() != "insufficient estimation window" {
		t.Errorf("expected 'insufficient estimation window', got %q", err.Error())
	}
}

func TestWindowBuilderImpl_TradingHalt(t *testing.T) {
	prices := makePrices(300, "TEST")
	// Introduce a halt at index 255 (within L2 for T0=260)
	prices[255].Close = 0

	wb := NewWindowBuilderImpl(prices)

	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: prices[260].Timestamp,
		Asset:     "TEST",
	}

	_, err := wb.BuildWindows(evt)
	if err == nil {
		t.Fatal("expected error for trading halt")
	}

	if err.Error() != "trading halt detected" {
		t.Errorf("expected 'trading halt detected', got %q", err.Error())
	}
}

func TestWindowBuilderImpl_T0NotFound(t *testing.T) {
	prices := makePrices(300, "TEST")
	wb := NewWindowBuilderImpl(prices)

	// Use a timestamp that doesn't exist in prices
	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC),
		Asset:     "TEST",
	}

	_, err := wb.BuildWindows(evt)
	if err == nil {
		t.Fatal("expected error when T0 not found")
	}
}

func TestWindowBuilderImpl_OverlapPriorEvent(t *testing.T) {
	prices := makePrices(300, "TEST")

	// T0 at index 260; L1 = prices[10..249]. Place prior event at index 100 (within L1).
	priorEvent := shared.Event{
		ID:        "prior",
		Type:      "edgar_8k",
		Timestamp: prices[100].Timestamp,
		Asset:     "TEST",
	}

	wb := NewWindowBuilderImplWithPriors(prices, []shared.Event{priorEvent})

	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: prices[260].Timestamp,
		Asset:     "TEST",
	}

	_, err := wb.BuildWindows(evt)
	if err == nil {
		t.Fatal("expected error for prior event overlap")
	}
	if err.Error() != "prior event overlaps estimation window" {
		t.Errorf("expected 'prior event overlaps estimation window', got %q", err.Error())
	}
}

func TestWindowBuilderImpl_OverlapDifferentAssetIgnored(t *testing.T) {
	prices := makePrices(300, "TEST")

	// Prior event on different asset — should NOT block
	priorEvent := shared.Event{
		ID:        "prior",
		Type:      "edgar_8k",
		Timestamp: prices[100].Timestamp,
		Asset:     "OTHER",
	}

	wb := NewWindowBuilderImplWithPriors(prices, []shared.Event{priorEvent})

	evt := shared.Event{
		ID:        "e1",
		Type:      "edgar_8k",
		Timestamp: prices[260].Timestamp,
		Asset:     "TEST",
	}

	_, err := wb.BuildWindows(evt)
	if err != nil {
		t.Fatalf("expected no error for prior event on different asset, got: %v", err)
	}
}
