package ingestion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"phantom/pkg/shared"
)

// ---- MockFetcher tests ----

func TestMockFetcherReturnsData(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	points := []shared.PricePoint{
		{AssetID: "AAPL", Timestamp: ts, Open: 150, High: 152, Low: 149, Close: 151, Volume: 1000000, Source: "mock"},
	}
	mf := &MockFetcher{Points: points}
	r := shared.TimeRange{From: ts, To: ts}
	got, err := mf.Fetch(context.Background(), "AAPL", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 point, got %d", len(got))
	}
	if got[0].Close != 151 {
		t.Errorf("Close: got %v want 151", got[0].Close)
	}
}

func TestStooqFetcherNoDataReturnsEmpty(t *testing.T) {
	t.Run("empty body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte{})
		}))
		defer srv.Close()
		f := &StooqFetcher{BaseURL: srv.URL}
		pts, err := f.Fetch(context.Background(), "FAKE.US", shared.TimeRange{})
		if err != nil {
			t.Fatalf("expected nil error for empty body, got %v", err)
		}
		if len(pts) != 0 {
			t.Fatalf("expected 0 points, got %d", len(pts))
		}
	})

	t.Run("no data text", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("No data"))
		}))
		defer srv.Close()
		f := &StooqFetcher{BaseURL: srv.URL}
		pts, err := f.Fetch(context.Background(), "FAKE.US", shared.TimeRange{})
		if err != nil {
			t.Fatalf("expected nil error for 'No data', got %v", err)
		}
		if len(pts) != 0 {
			t.Fatalf("expected 0 points, got %d", len(pts))
		}
	})
}

func TestMockFetcherReturnsError(t *testing.T) {
	mf := &MockFetcher{Err: errors.New("network error")}
	_, err := mf.Fetch(context.Background(), "AAPL", shared.TimeRange{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ---- StooqFetcher tests ----

const stooqFixtureCSV = `Date,Open,High,Low,Close,Volume
2024-01-02,150.12,152.00,149.50,151.00,1234567
2024-01-03,151.00,155.50,150.00,154.25,2345678
`

func TestStooqFetcherParsesCSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(stooqFixtureCSV))
	}))
	defer srv.Close()

	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	r := shared.TimeRange{From: from, To: to}

	fetcher := &StooqFetcher{BaseURL: srv.URL}
	points, err := fetcher.Fetch(context.Background(), "AAPL.US", r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	p0 := points[0]
	if p0.Open != 150.12 {
		t.Errorf("point[0].Open: got %v want 150.12", p0.Open)
	}
	if p0.High != 152.00 {
		t.Errorf("point[0].High: got %v want 152.00", p0.High)
	}
	if p0.Low != 149.50 {
		t.Errorf("point[0].Low: got %v want 149.50", p0.Low)
	}
	if p0.Close != 151.00 {
		t.Errorf("point[0].Close: got %v want 151.00", p0.Close)
	}
	if p0.Volume != 1234567 {
		t.Errorf("point[0].Volume: got %v want 1234567", p0.Volume)
	}
	if p0.Source != "stooq" {
		t.Errorf("point[0].Source: got %q want \"stooq\"", p0.Source)
	}
	if p0.AssetID != "AAPL.US" {
		t.Errorf("point[0].AssetID: got %q want \"AAPL.US\"", p0.AssetID)
	}
	wantDate := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if !p0.Timestamp.Equal(wantDate) {
		t.Errorf("point[0].Timestamp: got %v want %v", p0.Timestamp, wantDate)
	}

	p1 := points[1]
	if p1.Close != 154.25 {
		t.Errorf("point[1].Close: got %v want 154.25", p1.Close)
	}
}

const malformedCSV = `Date,Open
2024-01-02,150.12
`

func TestStooqFetcherMalformedCSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(malformedCSV))
	}))
	defer srv.Close()

	fetcher := &StooqFetcher{BaseURL: srv.URL}
	_, err := fetcher.Fetch(context.Background(), "AAPL.US", shared.TimeRange{
		From: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		To:   time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC),
	})
	if !errors.Is(err, ErrMalformedCSV) {
		t.Fatalf("expected ErrMalformedCSV, got %v", err)
	}
}
