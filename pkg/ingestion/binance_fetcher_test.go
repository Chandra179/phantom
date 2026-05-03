package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"

	"phantom/pkg/shared"
)

// binanceKlineFixture returns a JSON-encoded Binance kline array element.
func binanceKlineFixture(ts time.Time, open, high, low, close_, vol float64) []any {
	return []any{
		float64(ts.UnixMilli()),
		fmt.Sprintf("%.8f", open),
		fmt.Sprintf("%.8f", high),
		fmt.Sprintf("%.8f", low),
		fmt.Sprintf("%.8f", close_),
		fmt.Sprintf("%.8f", vol),
		float64(ts.UnixMilli()),
		"0.00000000",
		0,
		"0.00000000",
		"0.00000000",
		"0",
	}
}

// generateKlines creates n daily klines starting at base date.
func generateKlines(base time.Time, n int, startPrice float64) []any {
	klines := make([]any, n)
	price := startPrice
	for i := 0; i < n; i++ {
		ts := base.AddDate(0, 0, i)
		open := price
		high := price * 1.01
		low := price * 0.99
		close_ := price * 1.002
		vol := 1000.0 + float64(i)
		klines[i] = binanceKlineFixture(ts, open, high, low, close_, vol)
		price = close_
	}
	return klines
}

func TestBinanceFetcherParsesKlines(t *testing.T) {
	base := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	fixture := generateKlines(base, 3, 100.0)

	body, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	fetcher := &BinanceFetcher{BaseURL: srv.URL}
	from := base
	to := base.AddDate(0, 0, 2)
	points, err := fetcher.Fetch(context.Background(), "BTCUSDT", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("expected 3 points, got %d", len(points))
	}

	p0 := points[0]
	if p0.AssetID != "BTCUSDT" {
		t.Errorf("AssetID: got %q want BTCUSDT", p0.AssetID)
	}
	if !p0.Timestamp.Equal(base) {
		t.Errorf("Timestamp: got %v want %v", p0.Timestamp, base)
	}
	if p0.Open != 100.0 {
		t.Errorf("Open: got %f want 100.0", p0.Open)
	}
	if p0.Source != "binance" {
		t.Errorf("Source: got %q want binance", p0.Source)
	}

	p2 := points[2]
	wantTS := base.AddDate(0, 0, 2)
	if !p2.Timestamp.Equal(wantTS) {
		t.Errorf("point[2] Timestamp: got %v want %v", p2.Timestamp, wantTS)
	}
}

func TestBinanceFetcherPagination(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	page1 := generateKlines(base, 1000, 100.0)
	page1Last := base.AddDate(0, 0, 999)
	page2 := generateKlines(page1Last.AddDate(0, 0, 1), 300, 200.0)

	reqCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		var data []any
		if reqCount == 1 {
			data = page1
		} else {
			data = page2
		}
		b, _ := json.Marshal(data)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	fetcher := &BinanceFetcher{
		BaseURL: srv.URL,
		Limit:   1000,
	}

	from := base
	to := base.AddDate(0, 0, 1300)
	points, err := fetcher.Fetch(context.Background(), "BTCUSDT", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if reqCount != 2 {
		t.Errorf("expected 2 HTTP requests, got %d", reqCount)
	}

	expectedTotal := 1300
	if len(points) != 1300 {
		t.Fatalf("expected %d points, got %d", expectedTotal, len(points))
	}

	// Verify last point
	last := points[len(points)-1]
	wantTS := base.AddDate(0, 0, 1299)
	if !last.Timestamp.Equal(wantTS) {
		t.Errorf("last point Timestamp: got %v want %v", last.Timestamp, wantTS)
	}
}

func TestBinanceFetcherStopsAtEndTime(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	klines := generateKlines(base, 500, 100.0)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(klines)
		_, _ = w.Write(b)
	}))
	defer srv.Close()

	fetcher := &BinanceFetcher{BaseURL: srv.URL, Limit: 1000}

	// Range covers only first 100 days
	from := base
	to := base.AddDate(0, 0, 99)
	points, err := fetcher.Fetch(context.Background(), "BTCUSDT", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(points) != 100 {
		t.Errorf("expected 100 points (endTime bound), got %d", len(points))
	}
}

func TestBinanceFetcherHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":-1015,"msg":"Too many requests"}`))
	}))
	defer srv.Close()

	fetcher := &BinanceFetcher{BaseURL: srv.URL}
	_, err := fetcher.Fetch(context.Background(), "BTCUSDT", shared.TimeRange{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC),
	})
	if err == nil {
		t.Fatal("expected error for HTTP 429")
	}
}

func TestPipelineWithBinanceRateLimit(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	klines := generateKlines(base, 5, 100.0)
	body, _ := json.Marshal(klines)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	fetcher := &BinanceFetcher{BaseURL: srv.URL}
	store := NewMemStore()
	pipe := &Pipeline{
		Fetcher: fetcher,
		Deduper: &MemDeduper{},
		Store:   store,
		Limiter: rate.NewLimiter(20, 5), // 1200 req/min → 20/sec, burst 5
	}

	ctx := context.Background()
	from := base
	to := base.AddDate(0, 0, 4)
	err := pipe.Run(ctx, "BTCUSDT", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	got, err := store.Get(ctx, "BTCUSDT", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("expected 5 stored points, got %d", len(got))
	}
}
