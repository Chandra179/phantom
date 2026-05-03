package ingestion

import (
	"context"
	"errors"

	"phantom/pkg/shared"
)

// ErrMalformedCSV is returned when a CSV response cannot be parsed.
var ErrMalformedCSV = errors.New("malformed CSV")

// Fetcher pulls raw price data from an external API.
type Fetcher interface {
	Fetch(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error)
}

// Deduper ensures idempotent writes by tracking seen keys.
type Deduper interface {
	Seen(key string) bool
	Mark(key string)
}

// Store persists and retrieves PricePoints.
type Store interface {
	Put(ctx context.Context, points []shared.PricePoint) error
	Get(ctx context.Context, asset shared.AssetID, r shared.TimeRange) ([]shared.PricePoint, error)
}
