package ingestion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"phantom/pkg/shared"
)

const yahooFixtureJSON = `{
  "chart": {
    "result": [
      {
        "timestamp": [1704153600, 1704240000],
        "indicators": {
          "quote": [
            {
              "open": [150.12, 151.00],
              "high": [152.00, 155.50],
              "low": [149.50, 150.00],
              "close": [151.00, 154.25],
              "volume": [1234567, 2345678]
            }
          ]
        }
      }
    ],
    "error": null
  }
}`

func TestYahooFetcherParsesJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(yahooFixtureJSON))
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
	if !p0.Timestamp.Equal(time.Unix(1704153600, 0).UTC()) {
		t.Errorf("Timestamp: got %v", p0.Timestamp)
	}
}

func TestYahooFetcherEmptyOnNoResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chart":{"result":[],"error":null}}`))
	}))
	defer srv.Close()

	f := &YahooFinanceFetcher{BaseURL: srv.URL}
	pts, err := f.Fetch(context.Background(), "FAKE", shared.TimeRange{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(pts) != 0 {
		t.Fatalf("expected 0 points, got %d", len(pts))
	}
}

func TestYahooFetcherEmptyOnHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := &YahooFinanceFetcher{BaseURL: srv.URL}
	pts, err := f.Fetch(context.Background(), "FAKE", shared.TimeRange{})
	if err != nil {
		t.Fatalf("expected nil error for HTTP 404, got %v", err)
	}
	if len(pts) != 0 {
		t.Fatalf("expected 0 points, got %d", len(pts))
	}
}
