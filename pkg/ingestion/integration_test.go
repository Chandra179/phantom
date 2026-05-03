//go:build integration

package ingestion

import (
	"context"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"phantom/pkg/shared"
)

func TestStooqIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	ctx := context.Background()

	// Use a one-month window of known trading history.
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	r := shared.TimeRange{From: from, To: to}

	store := NewMemStore()
	pipe := &Pipeline{
		Fetcher: &StooqFetcher{},
		Deduper: &MemDeduper{},
		Store:   store,
		Limiter: rate.NewLimiter(1, 1),
	}

	if err := pipe.Run(ctx, "AAPL.US", r); err != nil {
		t.Fatalf("pipeline Run: %v", err)
	}

	points, err := store.Get(ctx, "AAPL.US", r)
	if err != nil {
		t.Fatalf("store Get: %v", err)
	}
	if len(points) == 0 {
		t.Fatal("expected price points from Stooq, got none")
	}

	// Compute mean Close and assert > 0.
	var sum float64
	for _, p := range points {
		sum += p.Close
	}
	mean := sum / float64(len(points))
	if mean <= 0 {
		t.Errorf("mean Close price should be > 0, got %v", mean)
	}
	t.Logf("AAPL.US mean Close Jan 2024: %.4f over %d points", mean, len(points))
}
