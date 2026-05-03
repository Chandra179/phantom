package ingestion

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"phantom/pkg/shared"
)

// MockFetcher is a test double for Fetcher. It returns Points or Err as-is.
type MockFetcher struct {
	Points []shared.PricePoint
	Err    error
}

func (m *MockFetcher) Fetch(_ context.Context, _ shared.AssetID, _ shared.TimeRange) ([]shared.PricePoint, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Points, nil
}

// StooqFetcher fetches daily OHLCV data from stooq.com.
// BaseURL may be overridden for testing.
// If APIKey is set it is appended as apikey query param (Stooq now requires an API key).
type StooqFetcher struct {
	BaseURL    string
	HTTPClient *http.Client
	APIKey     string
}

func (s *StooqFetcher) client() *http.Client {
	if s.HTTPClient != nil {
		return s.HTTPClient
	}
	return http.DefaultClient
}

func (s *StooqFetcher) baseURL() string {
	if s.BaseURL != "" {
		return s.BaseURL
	}
	return "https://stooq.com"
}

// Fetch downloads daily bars for the given asset over the time range.
func (s *StooqFetcher) Fetch(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	url := fmt.Sprintf("%s/q/d/l/?s=%s&d1=%s&d2=%s&i=d",
		s.baseURL(),
		string(asset),
		r.From.Format("20060102"),
		r.To.Format("20060102"),
	)
	if s.APIKey != "" {
		url += "&apikey=" + s.APIKey
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("stooq: build request: %w", err)
	}

	req.Header.Set("User-Agent", "phantom-quant/0.1")

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("stooq: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("stooq: read body: %w", err)
	}

	// Stooq returns "No data" (plain text) when ticker/date range empty.
	// Return empty slice, not error — no data is valid.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("No data")) {
		return nil, nil
	}

	// Detect HTML response (Stooq returns HTML when API key is missing)
	if trimmed[0] == '<' {
		if len(trimmed) > 200 {
			trimmed = trimmed[:200]
		}
		return nil, &transientError{fmt.Errorf("stooq: non-CSV response (missing API key?): %s", string(trimmed))}
	}

	return parseStooqCSVBytes(trimmed, asset)
}

// parseStooqCSVBytes parses Stooq CSV from raw bytes.
func parseStooqCSVBytes(data []byte, asset shared.AssetID) ([]shared.PricePoint, error) {
	return parseStooqCSV(bytes.NewReader(data), asset)
}

// parseStooqCSV parses the Stooq CSV format:
//
//	Date,Open,High,Low,Close,Volume
func parseStooqCSV(r io.Reader, asset shared.AssetID) ([]shared.PricePoint, error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true

	// Read header
	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: cannot read header: %v", ErrMalformedCSV, err)
	}
	if len(header) < 6 {
		return nil, fmt.Errorf("%w: expected 6 columns, got %d", ErrMalformedCSV, len(header))
	}

	var points []shared.PricePoint
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: read row: %v", ErrMalformedCSV, err)
		}
		if len(rec) < 6 {
			return nil, fmt.Errorf("%w: row has %d columns, want 6", ErrMalformedCSV, len(rec))
		}

		ts, err := time.Parse("2006-01-02", rec[0])
		if err != nil {
			return nil, fmt.Errorf("%w: parse date %q: %v", ErrMalformedCSV, rec[0], err)
		}

		open, err := strconv.ParseFloat(rec[1], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parse Open: %v", ErrMalformedCSV, err)
		}
		high, err := strconv.ParseFloat(rec[2], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parse High: %v", ErrMalformedCSV, err)
		}
		low, err := strconv.ParseFloat(rec[3], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parse Low: %v", ErrMalformedCSV, err)
		}
		close_, err := strconv.ParseFloat(rec[4], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parse Close: %v", ErrMalformedCSV, err)
		}
		vol, err := strconv.ParseFloat(rec[5], 64)
		if err != nil {
			return nil, fmt.Errorf("%w: parse Volume: %v", ErrMalformedCSV, err)
		}

		points = append(points, shared.PricePoint{
			AssetID:   asset,
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    vol,
			Source:    "stooq",
		})
	}
	return points, nil
}
