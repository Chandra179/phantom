package signalmatrix

import (
	"context"
	"math"
	"os/exec"
	"testing"
	"time"
)

// TestRustBridge_Integration starts the real Rust compute_server binary and
// verifies AR results match the expected values from Rust unit tests.
// Skipped under -short.
func TestRustBridge_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test under -short")
	}

	// Binary built by 'make build-rust'; path relative to repo root resolved via runtime.
	// Use absolute path to avoid working-directory sensitivity.
	binary := "/home/koala/Work/phantom/rust/compute_server/target/debug/compute_server"
	addr := "127.0.0.1:50052"
	_ = addr // server hardcodes [::1]:50051; connect there

	cmd := exec.Command(binary)
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start compute_server: %v — run 'make build-rust' first", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })

	// Give server time to bind
	time.Sleep(200 * time.Millisecond)

	bridge, err := NewRustBridge("[::1]:50051")
	if err != nil {
		t.Fatalf("NewRustBridge: %v", err)
	}

	ctx := context.Background()

	// percent_changes: [1, 2, 4] → [ln(2), ln(2)]
	changes, err := bridge.PercentChanges(ctx, []float64{1.0, 2.0, 4.0})
	if err != nil {
		t.Fatalf("PercentChanges: %v", err)
	}
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes, got %d", len(changes))
	}
	ln2 := math.Log(2)
	if math.Abs(changes[0]-ln2) > 1e-10 {
		t.Errorf("changes[0] = %v, want %v", changes[0], ln2)
	}
	if math.Abs(changes[1]-ln2) > 1e-10 {
		t.Errorf("changes[1] = %v, want %v", changes[1], ln2)
	}

	// abnormal_return: actual=[0.05,0.03], market=[0.02,0.01], alpha=0, beta=1 → [0.03, 0.02]
	ar, err := bridge.AbnormalReturn(ctx, []float64{0.05, 0.03}, []float64{0.02, 0.01}, 0.0, 1.0)
	if err != nil {
		t.Fatalf("AbnormalReturn: %v", err)
	}
	if len(ar) != 2 {
		t.Fatalf("expected 2 AR values, got %d", len(ar))
	}
	if math.Abs(ar[0]-0.03) > 1e-10 {
		t.Errorf("ar[0] = %v, want 0.03", ar[0])
	}
	if math.Abs(ar[1]-0.02) > 1e-10 {
		t.Errorf("ar[1] = %v, want 0.02", ar[1])
	}

	// cumulative_abnormal_return: [0.01, 0.02, -0.005] → 0.025
	car, err := bridge.CumulativeAbnormalReturn(ctx, []float64{0.01, 0.02, -0.005})
	if err != nil {
		t.Fatalf("CumulativeAbnormalReturn: %v", err)
	}
	if math.Abs(car-0.025) > 1e-10 {
		t.Errorf("car = %v, want 0.025", car)
	}

	// t_test_one_sample: [1,2,3] → 2*sqrt(3) ≈ 3.4641016
	tstat, err := bridge.TTestOneSample(ctx, []float64{1.0, 2.0, 3.0})
	if err != nil {
		t.Fatalf("TTestOneSample: %v", err)
	}
	expected := 2.0 * math.Sqrt(3)
	if math.Abs(tstat-expected) > 1e-6 {
		t.Errorf("t_statistic = %v, want %v", tstat, expected)
	}
}
