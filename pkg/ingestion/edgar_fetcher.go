package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"time"

	"phantom/pkg/shared"
)

// EdgarParseError is returned when the EDGAR response cannot be parsed.
type EdgarParseError struct {
	Cause error
}

func (e *EdgarParseError) Error() string {
	return fmt.Sprintf("edgar: parse error: %v", e.Cause)
}

func (e *EdgarParseError) Unwrap() error {
	return e.Cause
}

// edgarSubmissions mirrors the EDGAR submissions JSON structure.
type edgarSubmissions struct {
	CIK  string `json:"cik"`
	Name string `json:"name"`
	Filings struct {
		Recent struct {
			AccessionNumber  []string `json:"accessionNumber"`
			Form             []string `json:"form"`
			AcceptedDateTime []string `json:"acceptedDateTime"`
			PrimaryDocument  []string `json:"primaryDocument"`
		} `json:"recent"`
	} `json:"filings"`
}

// EdgarFetcher fetches 8-K events from the EDGAR submissions API.
type EdgarFetcher struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (f *EdgarFetcher) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return http.DefaultClient
}

func (f *EdgarFetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return "https://data.sec.gov"
}

// FetchEvents retrieves all 8-K filings for the given CIK from EDGAR.
func (f *EdgarFetcher) FetchEvents(ctx context.Context, cik string) ([]shared.Event, error) {
	url := fmt.Sprintf("%s/submissions/CIK%s.json", f.baseURL(), cik)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("edgar: build request: %w", err)
	}
	req.Header.Set("User-Agent", "phantom-quant/0.1")

	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("edgar: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("edgar: read body: %w", err)
	}

	var subs edgarSubmissions
	if err := json.Unmarshal(body, &subs); err != nil {
		return nil, &EdgarParseError{Cause: err}
	}

	etLoc, err := time.LoadLocation("America/New_York")
	if err != nil {
		return nil, fmt.Errorf("edgar: load timezone: %w", err)
	}

	recent := subs.Filings.Recent
	var events []shared.Event

	for i, form := range recent.Form {
		if form != "8-K" {
			continue
		}
		if i >= len(recent.AcceptedDateTime) {
			continue
		}

		rawTime := recent.AcceptedDateTime[i]
		t0, err := parseEdgarTime(rawTime)
		if err != nil {
			return nil, &EdgarParseError{Cause: fmt.Errorf("parse time %q: %w", rawTime, err)}
		}

		t0 = applyAfterHoursRule(t0, etLoc)

		accession := ""
		if i < len(recent.AccessionNumber) {
			accession = recent.AccessionNumber[i]
		}

		events = append(events, shared.Event{
			ID:        accession,
			Type:      "edgar_8k",
			Timestamp: t0,
			Asset:     shared.AssetID(cik),
		})
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.Before(events[j].Timestamp)
	})

	return events, nil
}

// parseEdgarTime parses EDGAR acceptedDateTime format.
func parseEdgarTime(s string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05.000Z",
		time.RFC3339,
		"2006-01-02T15:04:05Z",
	}
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse %q as time", s)
}

// applyAfterHoursRule shifts T0 to the next weekday 09:30 ET if the filing
// is after 16:00 ET or on a weekend.
func applyAfterHoursRule(t time.Time, etLoc *time.Location) time.Time {
	tET := t.In(etLoc)

	needsShift := false

	// Weekend check
	wd := tET.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		needsShift = true
	}

	// After-hours check: hour >= 16
	if tET.Hour() >= 16 {
		needsShift = true
	}

	if !needsShift {
		return t
	}

	// Advance by one day until we're on a weekday
	next := tET.AddDate(0, 0, 1)
	for next.Weekday() == time.Saturday || next.Weekday() == time.Sunday {
		next = next.AddDate(0, 0, 1)
	}

	// Set to 09:30 ET
	shifted := time.Date(next.Year(), next.Month(), next.Day(), 9, 30, 0, 0, etLoc)
	return shifted
}
