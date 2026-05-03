package signalmatrix

import (
	"time"

	"phantom/pkg/shared"
)

const HalvingEventType shared.EventType = "btc_halving"

// HalvingEvents returns known BTC halving dates as Event values.
// Timestamps are 00:00 UTC on the block-date of each halving.
func HalvingEvents() []shared.Event {
	return []shared.Event{
		{
			ID:        "btc-halving-2012",
			Type:      HalvingEventType,
			Timestamp: time.Date(2012, 11, 28, 0, 0, 0, 0, time.UTC),
			Asset:     "BTCUSDT",
		},
		{
			ID:        "btc-halving-2016",
			Type:      HalvingEventType,
			Timestamp: time.Date(2016, 7, 9, 0, 0, 0, 0, time.UTC),
			Asset:     "BTCUSDT",
		},
		{
			ID:        "btc-halving-2020",
			Type:      HalvingEventType,
			Timestamp: time.Date(2020, 5, 11, 0, 0, 0, 0, time.UTC),
			Asset:     "BTCUSDT",
		},
		{
			ID:        "btc-halving-2024",
			Type:      HalvingEventType,
			Timestamp: time.Date(2024, 4, 19, 0, 0, 0, 0, time.UTC),
			Asset:     "BTCUSDT",
		},
	}
}
