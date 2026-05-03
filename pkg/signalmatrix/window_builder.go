package signalmatrix

import "phantom/pkg/shared"

// WindowBuilder creates price windows around T0.
type WindowBuilder interface {
	// BuildWindows returns estimation and event windows for the given event.
	BuildWindows(event shared.Event) (shared.PriceWindow, error)
}
