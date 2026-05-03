package signalmatrix

import (
	"testing"
	"time"

	"phantom/pkg/shared"
)

// TestLookAheadAudit verifies temporal integrity of L1/L2 window construction.
//
// Validation gate (§6): "No look-ahead: event T0 strictly < window data
// timestamps used in L2 entry decision."
//
// Checks:
//  1. L1 (estimation) ends at T0-11, strictly before T0.
//  2. Gap of 1 trading day between L1 end and L2 start.
//  3. L2 starts at T0-10, contains T0 at index 10.
//  4. Entry decision point (T0+1 close) is within L2 at index 11.
//  5. All L1 timestamps < T0-10 (no contamination).
func TestLookAheadAudit(t *testing.T) {
	n := 400
	t0Idx := 300

	prices := make([]shared.PricePoint, n)
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		prices[i] = shared.PricePoint{
			AssetID:   "TEST",
			Timestamp: base.AddDate(0, 0, i),
			Close:     float64(100 + i),
			Volume:    1,
		}
	}

	t0Ts := prices[t0Idx].Timestamp
	evt := shared.Event{
		ID:        "e1",
		Type:      "test",
		Timestamp: t0Ts,
		Asset:     "TEST",
	}

	wb := NewWindowBuilderImpl(prices)
	pw, err := wb.BuildWindows(evt)
	if err != nil {
		t.Fatalf("BuildWindows: %v", err)
	}

	l1 := pw.Estimation
	l2 := pw.Event

	// 1. L1 ends at T0-11
	expectedL1End := base.AddDate(0, 0, t0Idx-11)
	if !l1[len(l1)-1].Timestamp.Equal(expectedL1End) {
		t.Errorf("L1 end = %v, want T0-11 = %v", l1[len(l1)-1].Timestamp, expectedL1End)
	}

	// 2. L2 starts at T0-10 (1-day gap after L1 end)
	expectedL2Start := base.AddDate(0, 0, t0Idx-10)
	if !l2[0].Timestamp.Equal(expectedL2Start) {
		t.Errorf("L2 start = %v, want T0-10 = %v", l2[0].Timestamp, expectedL2Start)
	}

	// 3. All L1 timestamps strictly < T0-10
	l1Threshold := base.AddDate(0, 0, t0Idx-10)
	for i, p := range l1 {
		if !p.Timestamp.Before(l1Threshold) {
			t.Errorf("L1[%d] ts = %v, want < T0-10 = %v", i, p.Timestamp, l1Threshold)
		}
	}

	// 4. No L2 timestamp used for estimation (L1 only)
	// L1 timestamps all ≤ T0-11, L2 timestamps start at T0-10 — no overlap
	lastL1 := l1[len(l1)-1].Timestamp
	firstL2 := l2[0].Timestamp
	if !lastL1.Before(firstL2) {
		t.Errorf("L1 end %v not before L2 start %v", lastL1, firstL2)
	}

	// 5. T0 is at L2 index 10 (10 days before + T0 + 10 days after)
	if !l2[10].Timestamp.Equal(t0Ts) {
		t.Errorf("L2[10] = %v, want T0 = %v", l2[10].Timestamp, t0Ts)
	}

	// 6. Entry point T0+1 is at L2 index 11 — available in event window
	entryTs := base.AddDate(0, 0, t0Idx+1)
	if !l2[11].Timestamp.Equal(entryTs) {
		t.Errorf("L2[11] = %v, want T0+1 = %v (entry decision point)", l2[11].Timestamp, entryTs)
	}

	// 7. All timestamps in L1 are before all timestamps in L2 (strict ordering)
	for i, l1p := range l1 {
		for j, l2p := range l2 {
			if !l1p.Timestamp.Before(l2p.Timestamp) {
				t.Errorf("L1[%d] ts=%v not before L2[%d] ts=%v", i, l1p.Timestamp, j, l2p.Timestamp)
			}
		}
	}
}
