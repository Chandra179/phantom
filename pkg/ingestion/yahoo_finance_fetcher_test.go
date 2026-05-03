package ingestion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"phantom/pkg/shared"
)

const yahooFixtureCSV = `Date,Open,High,Low,Close,Adj Close,Volume
2024-01-02,150.12,152.00,149.50,151.00,150.80,1234567
2024-01-03,151.00,155.50,150.00,154.25,154.00,2345678
`

func TestYahooFetcherParsesCSV(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(yahooFixtureCSV))
	}))
	defer srv.Close()

	from := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)
	f := &YahooFinanceFetcher{BaseURL: srv.URL}
	points, err := f.Fetch(context.Background(), "AAPL", shared.TimeRange{From: from, To: to})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(points) != 2 {
		t.Fatalf("expected 2 points, got %d", len(points))
	}

	p0 := points[0]
	if p0.Open != 150.12 {
		t.Errorf("Open: got %v want 150.12", p0.Open)
	}
	if p0.Close != 151.00 {
		t.Errorf("Close: got %v want 151.00", p0.Close)
	}
	if p0.Volume != 1234567 {
		t.Errorf("Volume: got %v want 1234567", p0.Volume)
	}
	if p0.Source != "yahoo" {
		t.Errorf("Source: got %q want \"yahoo\"", p0.Source)
	}
}

func TestYahooFetcherEmptyOnHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>unknown ticker</html>"))
	}))
	defer srv.Close()

	f := &YahooFinanceFetcher{BaseURL: srv.URL}
	pts, err := f.Fetch(context.Background(), "FAKE", shared.TimeRange{})
	if err != nil {
		t.Fatalf("expected nil error for HTML, got %v", err)
	}
	if len(pts) != 0 {
		t.Fatalf("expected 0 points, got %d", len(pts))
	}
}

func TestYahooFetcherEmptyOnJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"Not Found"}`))
	}))
	defer srv.Close()

	f := &YahooFinanceFetcher{BaseURL: srv.URL}
	pts, err := f.Fetch(context.Background(), "FAKE", shared.TimeRange{})
	if err != nil {
		t.Fatalf("expected nil error for JSON, got %v", err)
	}
	if len(pts) != 0 {
		t.Fatalf("expected 0 points, got %d", len(pts))
	}
}
