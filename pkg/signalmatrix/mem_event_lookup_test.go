package signalmatrix

import (
	"context"
	"testing"
	"time"

	"phantom/pkg/shared"
)

func TestMemEventLookup_SaveAndLoad(t *testing.T) {
	ctx := context.Background()
	lookup := NewMemEventLookup()

	t0 := time.Date(2023, 1, 10, 9, 30, 0, 0, time.UTC)
	t1 := time.Date(2023, 1, 5, 9, 30, 0, 0, time.UTC)
	t2 := time.Date(2023, 1, 20, 9, 30, 0, 0, time.UTC)

	evtA1 := shared.Event{ID: "a1", Type: "edgar_8k", Timestamp: t0, Asset: "AAPL"}
	evtA2 := shared.Event{ID: "a2", Type: "edgar_8k", Timestamp: t1, Asset: "MSFT"}
	evtB := shared.Event{ID: "b1", Type: "earnings", Timestamp: t2, Asset: "AAPL"}

	if err := lookup.SaveHistorical(ctx, evtA1); err != nil {
		t.Fatalf("SaveHistorical: %v", err)
	}
	if err := lookup.SaveHistorical(ctx, evtA2); err != nil {
		t.Fatalf("SaveHistorical: %v", err)
	}
	if err := lookup.SaveHistorical(ctx, evtB); err != nil {
		t.Fatalf("SaveHistorical: %v", err)
	}

	events, err := lookup.LoadHistorical(ctx, "edgar_8k")
	if err != nil {
		t.Fatalf("LoadHistorical: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	// Should be sorted by Timestamp asc: t1 < t0
	if !events[0].Timestamp.Equal(t1) {
		t.Errorf("expected first event timestamp %v, got %v", t1, events[0].Timestamp)
	}
	if !events[1].Timestamp.Equal(t0) {
		t.Errorf("expected second event timestamp %v, got %v", t0, events[1].Timestamp)
	}

	// Verify all returned events are edgar_8k type
	for _, e := range events {
		if e.Type != "edgar_8k" {
			t.Errorf("unexpected event type %q", e.Type)
		}
	}
}

func TestMemEventLookup_LoadEmpty(t *testing.T) {
	ctx := context.Background()
	lookup := NewMemEventLookup()

	events, err := lookup.LoadHistorical(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("LoadHistorical: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}
