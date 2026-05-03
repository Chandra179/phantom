package ingestion

import (
	"context"
	"fmt"

	"golang.org/x/time/rate"

	"phantom/pkg/shared"
)

// Pipeline orchestrates fetching, deduplication, and storage of price data.
type Pipeline struct {
	Fetcher Fetcher
	Deduper Deduper
	Store   Store
	Limiter *rate.Limiter
}

// Run fetches price data for the given asset and time range, deduplicates, and stores.
// Steps:
//  1. Wait for rate limiter token (respects ctx cancellation).
//  2. Fetch data from Fetcher.
//  3. Deduplicate each point via Deduper.
//  4. Store non-duplicate points via Store.
func (p *Pipeline) Run(ctx context.Context, asset shared.AssetID, r shared.TimeRange) error {
	// 1. Rate limit.
	if p.Limiter != nil {
		if err := p.Limiter.Wait(ctx); err != nil {
			return fmt.Errorf("pipeline: rate limiter: %w", err)
		}
	}

	// 2. Fetch.
	points, err := p.Fetcher.Fetch(ctx, asset, r)
	if err != nil {
		return fmt.Errorf("pipeline: fetch: %w", err)
	}

	// 3. Deduplicate.
	var deduped []shared.PricePoint
	for _, pt := range points {
		key := HashKey(pt.AssetID, pt.Timestamp, pt.Source)
		if p.Deduper.Seen(key) {
			continue
		}
		p.Deduper.Mark(key)
		deduped = append(deduped, pt)
	}

	// 4. Store.
	if len(deduped) == 0 {
		return nil
	}
	if err := p.Store.Put(ctx, deduped); err != nil {
		return fmt.Errorf("pipeline: store: %w", err)
	}
	return nil
}
