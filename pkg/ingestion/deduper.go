package ingestion

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"phantom/pkg/shared"
)

// HashKey computes a stable SHA-256 hex key for a (asset, timestamp, source) tuple.
func HashKey(asset shared.AssetID, ts time.Time, source string) string {
	h := sha256.New()
	// Use UTC Unix nanoseconds for a stable, timezone-independent representation.
	fmt.Fprintf(h, "%s\x00%d\x00%s", string(asset), ts.UTC().UnixNano(), source)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// MemDeduper is an in-memory, concurrency-safe Deduper backed by sync.Map.
type MemDeduper struct {
	m sync.Map
}

// Seen returns true if key has been marked.
func (d *MemDeduper) Seen(key string) bool {
	_, ok := d.m.Load(key)
	return ok
}

// Mark records key as seen.
func (d *MemDeduper) Mark(key string) {
	d.m.Store(key, struct{}{})
}
