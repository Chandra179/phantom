package ingestion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEdgarFetcher_MarketHours8K(t *testing.T) {
	// 1x 8-K at 06:01 (market hours) + 1x 10-K → returns exactly 1 event
	fixture := `{
		"cik": "0000320193",
		"name": "Apple Inc.",
		"filings": {
			"recent": {
				"accessionNumber": ["0000320193-23-000106", "0000320193-23-000064"],
				"form": ["8-K", "10-K"],
				"acceptedDateTime": ["2023-11-03T06:01:09.000Z", "2023-10-27T18:01:16.000Z"],
				"primaryDocument": ["a.htm", "b.htm"]
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	f := &EdgarFetcher{BaseURL: srv.URL}
	events, err := f.FetchEvents(context.Background(), "0000320193")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	evt := events[0]
	if string(evt.Asset) != "0000320193" {
		t.Errorf("expected asset 0000320193, got %s", evt.Asset)
	}
	if string(evt.Type) != "edgar_8k" {
		t.Errorf("expected type edgar_8k, got %s", evt.Type)
	}
	// 06:01 ET is market hours — no shift
	expectedHour := 6
	if evt.Timestamp.Hour() != expectedHour {
		t.Errorf("expected hour %d, got %d", expectedHour, evt.Timestamp.Hour())
	}
}

func TestEdgarFetcher_AfterHoursShift(t *testing.T) {
	// 8-K at 18:00 ET → T0 shifted to next day 09:30 ET
	// 2023-11-06 is Monday; 18:00 ET = 23:00 UTC → next day 2023-11-07 09:30 ET
	fixture := `{
		"cik": "0000320193",
		"name": "Apple Inc.",
		"filings": {
			"recent": {
				"accessionNumber": ["0000320193-23-000107"],
				"form": ["8-K"],
				"acceptedDateTime": ["2023-11-06T23:00:00.000Z"],
				"primaryDocument": ["a.htm"]
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	f := &EdgarFetcher{BaseURL: srv.URL}
	events, err := f.FetchEvents(context.Background(), "0000320193")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	etLoc, _ := time.LoadLocation("America/New_York")
	evtET := evt.Timestamp.In(etLoc)

	// Should be next day (2023-11-07) at 09:30 ET
	if evtET.Year() != 2023 || evtET.Month() != time.November || evtET.Day() != 7 {
		t.Errorf("expected 2023-11-07, got %v", evtET)
	}
	if evtET.Hour() != 9 || evtET.Minute() != 30 {
		t.Errorf("expected 09:30, got %02d:%02d", evtET.Hour(), evtET.Minute())
	}
}

func TestEdgarFetcher_WeekendShift(t *testing.T) {
	// 8-K on Saturday → T0 shifted to Monday 09:30 ET
	// 2023-11-04 is Saturday
	fixture := `{
		"cik": "0000320193",
		"name": "Apple Inc.",
		"filings": {
			"recent": {
				"accessionNumber": ["0000320193-23-000108"],
				"form": ["8-K"],
				"acceptedDateTime": ["2023-11-04T10:00:00.000Z"],
				"primaryDocument": ["a.htm"]
			}
		}
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fixture))
	}))
	defer srv.Close()

	f := &EdgarFetcher{BaseURL: srv.URL}
	events, err := f.FetchEvents(context.Background(), "0000320193")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	evt := events[0]
	etLoc, _ := time.LoadLocation("America/New_York")
	evtET := evt.Timestamp.In(etLoc)

	// Should be Monday 2023-11-06 at 09:30 ET
	if evtET.Weekday() != time.Monday {
		t.Errorf("expected Monday, got %v", evtET.Weekday())
	}
	if evtET.Year() != 2023 || evtET.Month() != time.November || evtET.Day() != 6 {
		t.Errorf("expected 2023-11-06, got %v", evtET)
	}
	if evtET.Hour() != 9 || evtET.Minute() != 30 {
		t.Errorf("expected 09:30, got %02d:%02d", evtET.Hour(), evtET.Minute())
	}
}

func TestEdgarFetcher_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	f := &EdgarFetcher{BaseURL: srv.URL}
	_, err := f.FetchEvents(context.Background(), "0000320193")
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}

	var parseErr *EdgarParseError
	if !errors.As(err, &parseErr) {
		t.Errorf("expected *EdgarParseError, got %T: %v", err, err)
	}
}
