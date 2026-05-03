package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"phantom/pkg/shared"
)

const (
	yahooDefaultBaseURL = "https://query2.finance.yahoo.com"
	yahooChartPath      = "/v8/finance/chart"
	yahooSource         = "yahoo"
)

// YahooFinanceFetcher fetches daily OHLCV data from Yahoo Finance v8 chart API.
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

// chartResponse models Yahoo Finance v8 chart JSON response.
type chartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []float64 `json:"open"`
					High   []float64 `json:"high"`
					Low    []float64 `json:"low"`
					Close  []float64 `json:"close"`
					Volume []int64   `json:"volume"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// Fetch downloads daily bars from Yahoo Finance v8 chart API.
func (f *YahooFinanceFetcher) Fetch(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	url := fmt.Sprintf("%s%s/%s?period1=%d&period2=%d&interval=1d",
		f.baseURL(), yahooChartPath, string(asset),
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

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo: HTTP %d", resp.StatusCode)
	}

	var cr chartResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return nil, fmt.Errorf("yahoo: json: %w", err)
	}

	if len(cr.Chart.Result) == 0 {
		return nil, nil
	}

	res := cr.Chart.Result[0]
	if len(res.Timestamp) == 0 || len(res.Indicators.Quote) == 0 {
		return nil, nil
	}

	quote := res.Indicators.Quote[0]
	n := len(res.Timestamp)
	if n > len(quote.Open) {
		n = len(quote.Open)
	}

	var points []shared.PricePoint
	for i := 0; i < n; i++ {
		ts := time.Unix(res.Timestamp[i], 0).UTC()

		open := 0.0
		if i < len(quote.Open) {
			open = quote.Open[i]
		}
		high := 0.0
		if i < len(quote.High) {
			high = quote.High[i]
		}
		low := 0.0
		if i < len(quote.Low) {
			low = quote.Low[i]
		}
		close_ := 0.0
		if i < len(quote.Close) {
			close_ = quote.Close[i]
		}
		vol := 0.0
		if i < len(quote.Volume) {
			vol = float64(quote.Volume[i])
		}

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


