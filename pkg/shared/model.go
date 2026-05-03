package shared

import (
	"math"
	"time"
)

// AssetID is a ticker or instrument identifier (e.g. "AAPL.US").
type AssetID string

// EventType categorises market events (e.g. "earnings", "fomc").
type EventType string

// TimeRange is an inclusive [From, To] interval.
type TimeRange struct {
	From time.Time
	To   time.Time
}

// PricePoint is a single OHLCV bar for an asset at a given time.
type PricePoint struct {
	AssetID   AssetID
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	Source    string
}

// Event is a labelled market event (e.g. FOMC rate hike, earnings release).
type Event struct {
	ID        string
	Type      EventType
	Timestamp time.Time // T0
	Asset     AssetID
}

// PriceWindow holds price series around an event.
type PriceWindow struct {
	Asset      AssetID
	Estimation []PricePoint // L1: estimation window (pre-event)
	Event      []PricePoint // L2: event window
}

// LogReturns computes ln(P_t / P_{t-1}) using the Close field of each PricePoint.
// Returns an empty slice if len(series) < 2.
func (pw PriceWindow) LogReturns(series []PricePoint) []float64 {
	if len(series) < 2 {
		return []float64{}
	}
	out := make([]float64, len(series)-1)
	for i := 1; i < len(series); i++ {
		out[i-1] = math.Log(series[i].Close / series[i-1].Close)
	}
	return out
}
