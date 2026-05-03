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

const (
	yahooDefaultBaseURL = "https://query1.finance.yahoo.com"
	yahooDownloadPath   = "/v7/finance/download"
	yahooSource         = "yahoo"
)

// YahooFinanceFetcher fetches daily OHLCV data from Yahoo Finance v7 API.
// Free, no API key required. Rate-limit ~1 req/s advised.
type YahooFinanceFetcher struct {
	BaseURL    string
	HTTPClient *http.Client
}

func (f *YahooFinanceFetcher) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return http.DefaultClient
}

func (f *YahooFinanceFetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return yahooDefaultBaseURL
}

// Fetch downloads daily bars from Yahoo Finance for asset over time range.
// CSV columns: Date,Open,High,Low,Close,Adj Close,Volume.
func (f *YahooFinanceFetcher) Fetch(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	url := fmt.Sprintf("%s%s/%s?period1=%d&period2=%d&interval=1d&events=history&includeAdjustedClose=true",
		f.baseURL(), yahooDownloadPath, string(asset),
		r.From.Unix(), r.To.Unix())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("yahoo: build request: %w", err)
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err := f.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo: http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("yahoo: read body: %w", err)
	}

	// Yahoo returns HTML/JSON error page for unknown tickers — treat as empty.
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] == '<' || trimmed[0] == '{' {
		return nil, nil
	}

	return parseYahooCSV(body, asset)
}

// parseYahooCSV parses Yahoo Finance CSV format:
// Date,Open,High,Low,Close,Adj Close,Volume
func parseYahooCSV(data []byte, asset shared.AssetID) ([]shared.PricePoint, error) {
	cr := csv.NewReader(bytes.NewReader(data))
	cr.TrimLeadingSpace = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("%w: yahoo header: %v", ErrMalformedCSV, err)
	}
	if len(header) < 7 {
		return nil, fmt.Errorf("%w: yahoo expected 7 columns, got %d", ErrMalformedCSV, len(header))
	}

	var points []shared.PricePoint
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: yahoo row: %v", ErrMalformedCSV, err)
		}
		if len(rec) < 7 {
			return nil, fmt.Errorf("%w: yahoo row has %d columns, want 7", ErrMalformedCSV, len(rec))
		}

		ts, err := time.Parse("2006-01-02", rec[0])
		if err != nil {
			return nil, fmt.Errorf("%w: yahoo date %q: %v", ErrMalformedCSV, rec[0], err)
		}

		open, _ := strconv.ParseFloat(rec[1], 64)
		high, _ := strconv.ParseFloat(rec[2], 64)
		low, _ := strconv.ParseFloat(rec[3], 64)
		close_, _ := strconv.ParseFloat(rec[4], 64)
		vol, _ := strconv.ParseFloat(rec[6], 64)

		// rec[5] is Adj Close, skip — use Close for consistency.

		points = append(points, shared.PricePoint{
			AssetID:   asset,
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    vol,
			Source:    yahooSource,
		})
	}
	return points, nil
}
