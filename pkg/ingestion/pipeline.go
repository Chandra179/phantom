package ingestion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cenkalti/backoff/v4"
	"golang.org/x/time/rate"

	"phantom/pkg/shared"
)

// transientError wraps an error to mark it as retryable.
type transientError struct {
	err error
}

func (t *transientError) Error() string   { return t.err.Error() }
func (t *transientError) Unwrap() error   { return t.err }
func (t *transientError) Is(target error) bool {
	_, ok := target.(*transientError)
	return ok
}

// Transient wraps err so the pipeline retries it.
func Transient(err error) error {
	return &transientError{err: err}
}

// isTransient returns true if err or any error in its chain is transient.
func isTransient(err error) bool {
	var t *transientError
	return errors.As(err, &t)
}

// defaultBackOff returns an exponential backoff policy suitable for API retries.
func defaultBackOff() backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 500 * time.Millisecond
	b.MaxInterval = 30 * time.Second
	b.MaxElapsedTime = 5 * time.Minute
	return b
}

// Pipeline orchestrates fetching, deduplication, and storage of price data.
type Pipeline struct {
	Fetcher Fetcher
	Deduper Deduper
	Store   Store
	Limiter *rate.Limiter
	BackOff backoff.BackOff // optional, defaults to defaultBackOff
}

func (p *Pipeline) backOff() backoff.BackOff {
	if p.BackOff != nil {
		return p.BackOff
	}
	return defaultBackOff()
}

// Run fetches price data for the given asset and time range, deduplicates, and stores.
// Steps:
//  1. Wait for rate limiter token (respects ctx cancellation).
//  2. Fetch data from Fetcher, retrying on transient errors with exponential backoff.
//  3. Deduplicate each point via Deduper.
//  4. Store non-duplicate points via Store.
func (p *Pipeline) Run(ctx context.Context, asset shared.AssetID, r shared.TimeRange) error {
	// 1. Rate limit.
	if p.Limiter != nil {
		if err := p.Limiter.Wait(ctx); err != nil {
			return fmt.Errorf("pipeline: rate limiter: %w", err)
		}
	}

	// 2. Fetch with retry on transient errors.
	var points []shared.PricePoint
	op := func() error {
		var err error
		points, err = p.Fetcher.Fetch(ctx, asset, r)
		if err != nil {
			if isTransient(err) {
				return err
			}
			return backoff.Permanent(err)
		}
		return nil
	}

	b := backoff.WithContext(p.backOff(), ctx)
	if err := backoff.RetryNotify(op, b, func(err error, d time.Duration) {
		// TODO: log retry with structured logging when implemented
	}); err != nil {
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

// maxRetriesBackOff caps retry count. Useful in tests.
func maxRetriesBackOff(max uint64) backoff.BackOff {
	b := backoff.NewExponentialBackOff()
	b.InitialInterval = 10 * time.Millisecond
	b.MaxInterval = 100 * time.Millisecond
	b.MaxElapsedTime = 0 // disable time-based cutoff
	return backoff.WithMaxRetries(b, max)
}
