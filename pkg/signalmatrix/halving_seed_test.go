package signalmatrix

import (
	"testing"
	"time"

	"phantom/pkg/shared"
)

func TestHalvingEventsCount(t *testing.T) {
	events := HalvingEvents()
	if len(events) != 4 {
		t.Fatalf("expected 4 halving events, got %d", len(events))
	}
}

func TestHalvingEventsType(t *testing.T) {
	for _, e := range HalvingEvents() {
		if e.Type != HalvingEventType {
			t.Errorf("event %s: expected type %s, got %s", e.ID, HalvingEventType, e.Type)
		}
		if e.Asset != "BTCUSDT" {
			t.Errorf("event %s: expected asset BTCUSDT, got %s", e.ID, e.Asset)
		}
	}
}

func TestHalvingEventsDates(t *testing.T) {
	events := HalvingEvents()
	expected := []struct {
		id   string
		date string
	}{
		{"btc-halving-2012", "2012-11-28"},
		{"btc-halving-2016", "2016-07-09"},
		{"btc-halving-2020", "2020-05-11"},
		{"btc-halving-2024", "2024-04-19"},
	}

	for i, exp := range expected {
		if events[i].ID != exp.id {
			t.Errorf("events[%d].ID: got %s want %s", i, events[i].ID, exp.id)
		}
		want, _ := time.Parse("2006-01-02", exp.date)
		if !events[i].Timestamp.Equal(want) {
			t.Errorf("events[%d] %s: got %v want %v", i, exp.id, events[i].Timestamp, want)
		}
	}
}

func TestHalvingEventsBuildWindow(t *testing.T) {
	events := HalvingEvents()
	evt2024 := events[3]

	// Create synthetic price series around 2024 halving
	base := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	n := 500
	prices := make([]shared.PricePoint, n)
	for i := 0; i < n; i++ {
		ts := base.AddDate(0, 0, i)
		closePrice := 30000.0 + float64(i)*10.0
		prices[i] = shared.PricePoint{
			AssetID:   "BTCUSDT",
			Timestamp: ts,
			Open:      closePrice,
			High:      closePrice * 1.01,
			Low:       closePrice * 0.99,
			Close:     closePrice,
			Volume:    1000,
		}
	}

	// The halving date must be present in the prices series
	// Find index where timestamp >= halving date
	t0Idx := -1
	for i, p := range prices {
		if !p.Timestamp.Before(evt2024.Timestamp) {
			t0Idx = i
			break
		}
	}
	if t0Idx < 0 || t0Idx < 260 {
		t.Skip("price series too short for window building (need T0 at index >= 260)")
	}

	// Adjust the event to match an actual price timestamp
	evt := shared.Event{
		ID:        evt2024.ID,
		Type:      evt2024.Type,
		Timestamp: prices[t0Idx].Timestamp,
		Asset:     evt2024.Asset,
	}

	wb := NewWindowBuilderImpl(prices)
	pw, err := wb.BuildWindows(evt)
	if err != nil {
		t.Fatalf("BuildWindows: %v", err)
	}

	// L1: [T0-250, T0-11] = 240 bars
	if len(pw.Estimation) != 240 {
		t.Errorf("expected 240 estimation bars, got %d", len(pw.Estimation))
	}
	// L2: [T0-10, T0+10] = 21 bars
	if len(pw.Event) != 21 {
		t.Errorf("expected 21 event bars, got %d", len(pw.Event))
	}

	if pw.Asset != "BTCUSDT" {
		t.Errorf("expected asset BTCUSDT, got %s", pw.Asset)
	}
}
