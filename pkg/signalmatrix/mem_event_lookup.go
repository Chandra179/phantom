package signalmatrix

import (
	"context"
	"sort"
	"sync"

	"phantom/pkg/shared"
)

// MemEventLookup is an in-memory implementation of EventLookup.
type MemEventLookup struct {
	mu     sync.RWMutex
	events []shared.Event
}

// NewMemEventLookup creates a new empty MemEventLookup.
func NewMemEventLookup() *MemEventLookup {
	return &MemEventLookup{}
}

// SaveHistorical stores an event in memory.
func (m *MemEventLookup) SaveHistorical(_ context.Context, event shared.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, event)
	return nil
}

// LoadHistorical returns all events of the given type, sorted by Timestamp asc.
func (m *MemEventLookup) LoadHistorical(_ context.Context, eventType shared.EventType) ([]shared.Event, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []shared.Event
	for _, e := range m.events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.Before(result[j].Timestamp)
	})

	return result, nil
}
