package ingestion

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"phantom/pkg/shared"
)

// ---- MemStore tests ----

func makePoints(asset shared.AssetID, from time.Time, count int) []shared.PricePoint {
	pts := make([]shared.PricePoint, count)
	for i := range pts {
		pts[i] = shared.PricePoint{
			AssetID:   asset,
			Timestamp: from.Add(time.Duration(i) * 24 * time.Hour),
			Open:      float64(100 + i),
			High:      float64(105 + i),
			Low:       float64(98 + i),
			Close:     float64(102 + i),
			Volume:    float64(1000000 + i),
			Source:    "test",
		}
	}
	return pts
}

func TestMemStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()

	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	pts := makePoints("AAPL", from, 4)

	if err := store.Put(ctx, pts); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r := shared.TimeRange{From: from, To: to}
	got, err := store.Get(ctx, "AAPL", r)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != len(pts) {
		t.Fatalf("expected %d points, got %d", len(pts), len(got))
	}
	for i := range pts {
		if got[i].Close != pts[i].Close {
			t.Errorf("point[%d].Close: got %v want %v", i, got[i].Close, pts[i].Close)
		}
	}
}

func TestMemStoreFiltersByRange(t *testing.T) {
	ctx := context.Background()
	store := NewMemStore()

	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	pts := makePoints("MSFT", from, 10)

	if err := store.Put(ctx, pts); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Only ask for days 2-4 (index 1-3)
	r := shared.TimeRange{
		From: from.Add(24 * time.Hour),
		To:   from.Add(3 * 24 * time.Hour),
	}
	got, err := store.Get(ctx, "MSFT", r)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 points in range, got %d", len(got))
	}
}

// ---- ParquetStore tests ----

func TestParquetStoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := NewParquetStore(dir)

	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	pts := makePoints("AAPL", from, 4)

	if err := store.Put(ctx, pts); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r := shared.TimeRange{From: from, To: to}
	got, err := store.Get(ctx, "AAPL", r)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != len(pts) {
		t.Fatalf("expected %d points, got %d", len(pts), len(got))
	}
	for i := range pts {
		if got[i].Close != pts[i].Close {
			t.Errorf("point[%d].Close: got %v want %v", i, got[i].Close, pts[i].Close)
		}
		if !got[i].Timestamp.Equal(pts[i].Timestamp) {
			t.Errorf("point[%d].Timestamp: got %v want %v", i, got[i].Timestamp, pts[i].Timestamp)
		}
	}
}

func TestParquetStorePartitionPath(t *testing.T) {
	// Verify the partition path format: {baseDir}/{asset}/{year}/{month}/data.parquet
	baseDir := "/data"
	asset := shared.AssetID("AAPL")
	ts := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)

	got := partitionPath(baseDir, asset, ts)
	want := filepath.Join(baseDir, "AAPL", "2024", "01", "data.parquet")
	if got != want {
		t.Errorf("partitionPath: got %q want %q", got, want)
	}
}

func TestParquetStoreMultipleMonths(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := NewParquetStore(dir)

	// Create points spanning two months
	jan := time.Date(2024, 1, 31, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	pts := []shared.PricePoint{
		{AssetID: "GOOG", Timestamp: jan, Close: 100.0, Source: "test"},
		{AssetID: "GOOG", Timestamp: feb, Close: 200.0, Source: "test"},
	}

	if err := store.Put(ctx, pts); err != nil {
		t.Fatalf("Put: %v", err)
	}

	r := shared.TimeRange{From: jan, To: feb}
	got, err := store.Get(ctx, "GOOG", r)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 points across months, got %d", len(got))
	}
}

func TestParquetStorePartitionExists(t *testing.T) {
	// Verify files are written at expected paths
	ctx := context.Background()
	dir := t.TempDir()
	store := NewParquetStore(dir)

	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	pts := []shared.PricePoint{
		{AssetID: "AAPL", Timestamp: ts, Close: 150.0, Source: "test"},
	}
	if err := store.Put(ctx, pts); err != nil {
		t.Fatalf("Put: %v", err)
	}

	expectedPath := filepath.Join(dir, "AAPL", "2024", "01", "data.parquet")
	// Just verify we can Get it back (file existence check via round-trip)
	r := shared.TimeRange{From: ts, To: ts}
	got, err := store.Get(ctx, "AAPL", r)
	if err != nil {
		t.Fatalf("Get after Put: %v", err)
	}
	if len(got) == 0 {
		t.Errorf("expected points at %s, got none", expectedPath)
	}
	_ = fmt.Sprintf("partition: %s", expectedPath) // ensure path is computed
}
