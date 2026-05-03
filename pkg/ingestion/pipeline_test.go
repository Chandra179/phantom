package ingestion

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"phantom/pkg/shared"
)

func buildPipeline(fetcher Fetcher) (*Pipeline, *MemStore) {
	store := NewMemStore()
	p := &Pipeline{
		Fetcher: fetcher,
		Deduper: &MemDeduper{},
		Store:   store,
		Limiter: rate.NewLimiter(rate.Inf, 1), // no effective limit for tests
	}
	return p, store
}

func TestPipelineStoresPoints(t *testing.T) {
	ctx := context.Background()
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	pts := makePoints("AAPL", from, 4)

	pipe, store := buildPipeline(&MockFetcher{Points: pts})
	if err := pipe.Run(ctx, "AAPL", shared.TimeRange{From: from, To: to}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := store.Get(ctx, "AAPL", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != len(pts) {
		t.Fatalf("expected %d stored points, got %d", len(pts), len(got))
	}
}

func TestPipelineDeduplicates(t *testing.T) {
	ctx := context.Background()
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	pts := makePoints("MSFT", from, 4)

	// Run the same points twice.
	pipe, store := buildPipeline(&MockFetcher{Points: pts})
	r := shared.TimeRange{From: from, To: to}
	if err := pipe.Run(ctx, "MSFT", r); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if err := pipe.Run(ctx, "MSFT", r); err != nil {
		t.Fatalf("second Run: %v", err)
	}

	got, err := store.Get(ctx, "MSFT", r)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != len(pts) {
		t.Errorf("dedup: expected %d points after two runs, got %d", len(pts), len(got))
	}
}

func TestPipelineFetchError(t *testing.T) {
	ctx := context.Background()
	fetchErr := errors.New("network error")
	pipe, _ := buildPipeline(&MockFetcher{Err: fetchErr})
	err := pipe.Run(ctx, "AAPL", shared.TimeRange{})
	if !errors.Is(err, fetchErr) {
		t.Fatalf("expected fetch error, got %v", err)
	}
}

func TestPipelineCtxCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before Run

	pipe, _ := buildPipeline(&MockFetcher{Points: makePoints("AAPL", time.Now(), 1)})
	err := pipe.Run(ctx, "AAPL", shared.TimeRange{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestPipelineRateLimiterCalled(t *testing.T) {
	// Use a high-burst real limiter to verify Run still works under rate limiting.
	ctx := context.Background()
	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	pts := makePoints("GOOG", from, 2)
	store := NewMemStore()
	pipe := &Pipeline{
		Fetcher: &MockFetcher{Points: pts},
		Deduper: &MemDeduper{},
		Store:   store,
		Limiter: rate.NewLimiter(100, 100), // generous but real limiter
	}
	if err := pipe.Run(ctx, "GOOG", shared.TimeRange{From: from, To: from.Add(48 * time.Hour)}); err != nil {
		t.Fatalf("Run with real limiter: %v", err)
	}
	got, _ := store.Get(ctx, "GOOG", shared.TimeRange{From: from, To: from.Add(48 * time.Hour)})
	if len(got) != 2 {
		t.Errorf("expected 2 points, got %d", len(got))
	}
}
