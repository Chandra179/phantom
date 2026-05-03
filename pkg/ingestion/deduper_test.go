package ingestion

import (
	"sync"
	"testing"
	"time"

	"phantom/pkg/shared"
)

func TestHashKeyStable(t *testing.T) {
	asset := shared.AssetID("AAPL.US")
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	src := "stooq"

	first := HashKey(asset, ts, src)
	for i := 0; i < 1000; i++ {
		if got := HashKey(asset, ts, src); got != first {
			t.Fatalf("HashKey not stable: got %q want %q on iteration %d", got, first, i)
		}
	}
	// Should be 64-char hex (SHA-256)
	if len(first) != 64 {
		t.Errorf("HashKey length: got %d want 64", len(first))
	}
}

func TestHashKeyDistinct(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	k1 := HashKey("AAPL.US", ts, "stooq")
	k2 := HashKey("MSFT.US", ts, "stooq")
	if k1 == k2 {
		t.Error("different assets should produce different hash keys")
	}
}

func TestMemDeduperNewKeyNotSeen(t *testing.T) {
	d := &MemDeduper{}
	if d.Seen("nonexistent") {
		t.Error("Seen should return false for a key that was never marked")
	}
}

func TestMemDeduperMarkThenSeen(t *testing.T) {
	d := &MemDeduper{}
	d.Mark("key1")
	if !d.Seen("key1") {
		t.Error("Seen should return true after Mark")
	}
}

func TestMemDeduperConcurrentMark(t *testing.T) {
	d := &MemDeduper{}
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := HashKey(shared.AssetID("AAPL"), time.Unix(int64(n), 0), "stooq")
			d.Mark(key)
			d.Seen(key)
		}(i)
	}
	wg.Wait()
}
