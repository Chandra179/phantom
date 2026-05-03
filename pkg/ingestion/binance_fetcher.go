package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"phantom/pkg/shared"
)

const (
	binanceDefaultBaseURL = "https://api.binance.com"
	binanceKlinesPath     = "/api/v3/klines"
	binanceDefaultLimit   = 1000
	binanceSource         = "binance"
)

// BinanceFetcher fetches OHLCV data from the Binance public REST klines endpoint.
type BinanceFetcher struct {
	BaseURL    string
	HTTPClient *http.Client
	Interval   string
	Limit      int
}

func (f *BinanceFetcher) client() *http.Client {
	if f.HTTPClient != nil {
		return f.HTTPClient
	}
	return http.DefaultClient
}

func (f *BinanceFetcher) baseURL() string {
	if f.BaseURL != "" {
		return f.BaseURL
	}
	return binanceDefaultBaseURL
}

func (f *BinanceFetcher) interval() string {
	if f.Interval != "" {
		return f.Interval
	}
	return "1d"
}

func (f *BinanceFetcher) limit() int {
	if f.Limit > 0 {
		return f.Limit
	}
	return binanceDefaultLimit
}

// Fetch downloads klines for the given asset over the time range.
// Paginates in batches of up to 1000 klines per request.
func (f *BinanceFetcher) Fetch(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error) {
	var all []shared.PricePoint
	startMs := r.From.UnixMilli()
	endMs := r.To.UnixMilli()

	for {
		url := fmt.Sprintf("%s%s?symbol=%s&interval=%s&startTime=%d&endTime=%d&limit=%d",
			f.baseURL(), binanceKlinesPath, string(asset), f.interval(), startMs, endMs, f.limit())

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("binance: build request: %w", err)
		}

		resp, err := f.client().Do(req)
		if err != nil {
			return nil, fmt.Errorf("binance: http: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("binance: read body: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("binance: HTTP %d: %s", resp.StatusCode, string(body))
		}

		points, lastTs, err := parseBinanceKlines(body, asset, endMs)
		if err != nil {
			return nil, fmt.Errorf("binance: parse: %w", err)
		}

		all = append(all, points...)

		if len(points) < f.limit() || lastTs == 0 {
			break
		}

		startMs = lastTs + 1
	}

	return all, nil
}

// binanceKline represents one kline from the Binance klines endpoint.
// Fields: [OpenTime, Open, High, Low, Close, Volume, CloseTime, QuoteVol, Trades, TakerBuyBase, TakerBuyQuote, Ignore]
type binanceKline []any

func parseBinanceKlines(body []byte, asset shared.AssetID, endMs int64) ([]shared.PricePoint, int64, error) {
	var raw []binanceKline
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, fmt.Errorf("unmarshal: %w", err)
	}

	var points []shared.PricePoint
	var lastTs int64

	for _, k := range raw {
		if len(k) < 6 {
			continue
		}

		openTimeF, err := parseFloat64(k[0])
		if err != nil {
			return nil, 0, fmt.Errorf("openTime: %w", err)
		}
		openTimeMs := int64(openTimeF)

		if openTimeMs > endMs {
			break
		}

		open, _ := parseFloat64(k[1])
		high, _ := parseFloat64(k[2])
		low, _ := parseFloat64(k[3])
		close_, _ := parseFloat64(k[4])
		volume, _ := parseFloat64(k[5])

		ts := time.UnixMilli(openTimeMs).UTC()

		points = append(points, shared.PricePoint{
			AssetID:   asset,
			Timestamp: ts,
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close_,
			Volume:    volume,
			Source:    binanceSource,
		})
		lastTs = openTimeMs
	}

	return points, lastTs, nil
}

func parseFloat64(v any) (float64, error) {
	switch val := v.(type) {
	case float64:
		return val, nil
	case string:
		return strconv.ParseFloat(val, 64)
	default:
		return 0, fmt.Errorf("cannot parse %T %v as float64", v, v)
	}
}
