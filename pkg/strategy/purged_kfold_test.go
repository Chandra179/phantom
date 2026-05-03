package strategy

import (
	"testing"
	"time"

	"phantom/pkg/shared"
)

func TestPurgedKFold_Basic(t *testing.T) {
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	events := make([]shared.Event, 30)
	for i := 0; i < 30; i++ {
		events[i] = shared.Event{
			ID:        string(rune('a' + i)),
			Timestamp: base.AddDate(0, 0, i*5),
		}
	}

	k := 3
	embargo := 10 * 24 * time.Hour
	folds, err := PurgedKFold(events, k, embargo)
	if err != nil {
		t.Fatalf("PurgedKFold: %v", err)
	}
	if len(folds) != k {
		t.Fatalf("got %d folds, want %d", len(folds), k)
	}

	for i, fold := range folds {
		if len(fold.TestIndices) == 0 {
			t.Errorf("fold %d has empty test set", i)
		}
		trainSet := make(map[int]bool)
		for _, idx := range fold.TrainIndices {
			trainSet[idx] = true
		}
		for _, idx := range fold.TestIndices {
			if trainSet[idx] {
				t.Errorf("fold %d: index %d in both train and test", i, idx)
			}
		}
	}
}

func TestPurgedKFold_EmbargoRemovesLeakage(t *testing.T) {
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	events := make([]shared.Event, 30)
	for i := 0; i < 30; i++ {
		events[i] = shared.Event{
			ID:        string(rune('a' + i)),
			Timestamp: base.AddDate(0, 0, i),
		}
	}

	k := 3
	embargo := 10 * 24 * time.Hour
	folds, err := PurgedKFold(events, k, embargo)
	if err != nil {
		t.Fatalf("PurgedKFold: %v", err)
	}

	// Fold 0: test=[0,10), train should be empty (no events before)
	if len(folds[0].TrainIndices) != 0 {
		t.Errorf("fold 0 train size = %d, want 0", len(folds[0].TrainIndices))
	}
	if len(folds[0].TestIndices) != 10 {
		t.Errorf("fold 0 test size = %d, want 10", len(folds[0].TestIndices))
	}

	// Fold 1: test=[10,20), train candidates [0,10)
	// All within 10 days of day 10 => all embargoed => train=[]
	if len(folds[1].TrainIndices) != 0 {
		t.Errorf("fold 1 train size = %d, want 0 (all embargoed)", len(folds[1].TrainIndices))
	}

	// Fold 2: test=[20,30), train candidates [0,20)
	// Events at days 0-9 have day+10 < 20 => included
	// Events at days 10-19 have day+10 >= 20 => embargoed
	if len(folds[2].TrainIndices) != 10 {
		t.Errorf("fold 2 train size = %d, want 10 (indices 0-9)", len(folds[2].TrainIndices))
	}

	// Verify strict temporal ordering for fold 2: all train + embargo < test min
	var trainMax time.Time
	for _, idx := range folds[2].TrainIndices {
		if events[idx].Timestamp.After(trainMax) {
			trainMax = events[idx].Timestamp
		}
	}
	testMin := events[folds[2].TestIndices[0]].Timestamp
	if !trainMax.Add(embargo).Before(testMin) {
		t.Errorf("train max %v + embargo %v not before test min %v", trainMax, embargo, testMin)
	}
}

func TestPurgedKFold_Errors(t *testing.T) {
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	events := []shared.Event{{ID: "a", Timestamp: base}}

	_, err := PurgedKFold(events, 1, time.Hour)
	if err == nil {
		t.Error("k=1 should error")
	}
	_, err = PurgedKFold(events, 5, time.Hour)
	if err == nil {
		t.Error("k > len(events) should error")
	}
}

func TestPurgedKFold_PreservesTimestamps(t *testing.T) {
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	events := make([]shared.Event, 20)
	for i := 0; i < 20; i++ {
		events[i] = shared.Event{
			ID:        string(rune('a' + i)),
			Timestamp: base.AddDate(0, 0, i*3),
		}
	}

	k := 2
	folds, err := PurgedKFold(events, k, time.Hour)
	if err != nil {
		t.Fatalf("PurgedKFold: %v", err)
	}

	// Fold 0: test=[0,10), train empty
	// Fold 1: test=[10,20), train=[0,10)
	cases := []struct {
		name      string
		testStart int
		testEnd   int
		trainLen  int
	}{
		{"fold 0", 0, 10, 0},
		{"fold 1", 10, 20, 10},
	}
	for _, c := range cases {
		f := folds[c.testStart/10]
		if len(f.TestIndices) != c.testEnd-c.testStart {
			t.Errorf("%s test count: got %d, want %d", c.name, len(f.TestIndices), c.testEnd-c.testStart)
		}
		if len(f.TrainIndices) != c.trainLen {
			t.Errorf("%s train count: got %d, want %d", c.name, len(f.TrainIndices), c.trainLen)
		}
	}
}
