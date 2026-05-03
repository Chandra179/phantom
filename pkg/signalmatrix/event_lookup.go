package signalmatrix

import (
	"context"
	"phantom/pkg/shared"
)

// EventLookup maps an event type to historical timestamps.
type EventLookup interface {
	// LoadHistorical returns all T0 timestamps for the given event type.
	LoadHistorical(ctx context.Context, eventType shared.EventType) ([]shared.Event, error)
	// SaveHistorical adds a newly labeled event.
	SaveHistorical(ctx context.Context, event shared.Event) error
}
