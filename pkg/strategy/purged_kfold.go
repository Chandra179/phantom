package strategy

import (
	"fmt"
	"sort"
	"time"

	"phantom/pkg/shared"
)

// EmbargoedFold contains train/test index sets for one CV fold.
type EmbargoedFold struct {
	TrainIndices []int
	TestIndices  []int
}

// PurgedKFold splits events into k chronological folds with embargo.
// Events sorted by timestamp. For each test fold, training uses only events
// that occur before the test fold minus embargo (prevents leakage).
// Embargo of 0 means only strictly future-to-past separation.
func PurgedKFold(events []shared.Event, k int, embargo time.Duration) ([]EmbargoedFold, error) {
	if k < 2 {
		return nil, fmt.Errorf("purged k-fold: k must be >= 2, got %d", k)
	}
	if k > len(events) {
		return nil, fmt.Errorf("purged k-fold: k=%d > n_events=%d", k, len(events))
	}

	sorted := make([]shared.Event, len(events))
	copy(sorted, events)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	n := len(sorted)
	foldSize := n / k
	folds := make([]EmbargoedFold, k)

	for i := 0; i < k; i++ {
		start := i * foldSize
		end := (i + 1) * foldSize
		if i == k-1 {
			end = n
		}

		testIndices := make([]int, end-start)
		for j := start; j < end; j++ {
			testIndices[j-start] = j
		}

		var trainIndices []int
		testFoldStart := sorted[start].Timestamp
		for j := 0; j < start; j++ {
			if sorted[j].Timestamp.Add(embargo).Before(testFoldStart) {
				trainIndices = append(trainIndices, j)
			}
		}

		folds[i] = EmbargoedFold{TrainIndices: trainIndices, TestIndices: testIndices}
	}
	return folds, nil
}
