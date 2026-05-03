package signalmatrix

import (
	"errors"
	"fmt"

	"phantom/pkg/shared"
)

// WindowBuilderImpl implements WindowBuilder using a slice of PricePoints.
type WindowBuilderImpl struct {
	prices       []shared.PricePoint
	priorEvents  []shared.Event // used for overlap detection
}

// NewWindowBuilderImpl creates a WindowBuilderImpl backed by the given price series.
func NewWindowBuilderImpl(prices []shared.PricePoint) *WindowBuilderImpl {
	return &WindowBuilderImpl{prices: prices}
}

// NewWindowBuilderImplWithPriors creates a WindowBuilderImpl that skips events
// whose L1 estimation window overlaps a prior event.
func NewWindowBuilderImplWithPriors(prices []shared.PricePoint, priorEvents []shared.Event) *WindowBuilderImpl {
	return &WindowBuilderImpl{prices: prices, priorEvents: priorEvents}
}

// BuildWindows builds estimation (L1) and event (L2) windows around the event's T0.
//
// L1 (estimation): [T0-250, T0-11] = 240 bars
// L2 (event):      [T0-10, T0+10] = 21 bars
//
// Returns an error if:
//   - T0 not found in price series (matching both Timestamp and Asset)
//   - L1 has fewer than 200 observations (Brown-Warner threshold)
//   - Any prior event's T0 falls within the L1 window (clustering contamination)
//   - Any price in L2 has Close == 0 (trading halt proxy)
func (wb *WindowBuilderImpl) BuildWindows(event shared.Event) (shared.PriceWindow, error) {
	// Find T0 index
	t0Idx := -1
	for i, p := range wb.prices {
		if p.AssetID == event.Asset && p.Timestamp.Equal(event.Timestamp) {
			t0Idx = i
			break
		}
	}
	if t0Idx < 0 {
		return shared.PriceWindow{}, fmt.Errorf("T0 not found for asset %s at %v", event.Asset, event.Timestamp)
	}

	// L1: [T0-250, T0-11] inclusive → indices [t0Idx-250, t0Idx-11]
	l1Start := t0Idx - 250
	l1End := t0Idx - 11 // inclusive

	if l1Start < 0 {
		return shared.PriceWindow{}, errors.New("insufficient estimation window")
	}

	l1 := wb.prices[l1Start : l1End+1] // 240 bars

	// Check Brown-Warner threshold: at least 200 observations
	if len(l1) < 200 {
		return shared.PriceWindow{}, errors.New("insufficient estimation window")
	}

	// Check for prior event overlap within L1 window (same asset)
	l1StartTs := wb.prices[l1Start].Timestamp
	l1EndTs := wb.prices[l1End].Timestamp
	for _, prior := range wb.priorEvents {
		if prior.Asset != event.Asset {
			continue
		}
		if !prior.Timestamp.Before(l1StartTs) && !prior.Timestamp.After(l1EndTs) {
			return shared.PriceWindow{}, errors.New("prior event overlaps estimation window")
		}
	}

	// L2: [T0-10, T0+10] inclusive → indices [t0Idx-10, t0Idx+10]
	l2Start := t0Idx - 10
	l2End := t0Idx + 10

	if l2Start < 0 || l2End >= len(wb.prices) {
		return shared.PriceWindow{}, errors.New("insufficient data for event window")
	}

	l2 := wb.prices[l2Start : l2End+1] // 21 bars

	// Check for trading halt
	for _, p := range l2 {
		if p.Close == 0 {
			return shared.PriceWindow{}, errors.New("trading halt detected")
		}
	}

	// Copy slices to avoid aliasing
	estimation := make([]shared.PricePoint, len(l1))
	copy(estimation, l1)

	eventWindow := make([]shared.PricePoint, len(l2))
	copy(eventWindow, l2)

	return shared.PriceWindow{
		Asset:      event.Asset,
		Estimation: estimation,
		Event:      eventWindow,
	}, nil
}
