package aggregation_test

import (
	"math"
	"testing"

	"phantom/pkg/aggregation"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func almostEqual(a, b, tol float64) bool {
	return math.Abs(a-b) <= tol
}

// ── cross-sectional mean CAR ─────────────────────────────────────────────────

func TestMeanCAR_Basic(t *testing.T) {
	cars := []float64{0.02, 0.04, 0.06}
	got := aggregation.MeanCAR(cars)
	want := 0.04
	if !almostEqual(got, want, 1e-10) {
		t.Errorf("MeanCAR = %f, want %f", got, want)
	}
}

func TestMeanCAR_Empty(t *testing.T) {
	got := aggregation.MeanCAR(nil)
	if !math.IsNaN(got) {
		t.Errorf("MeanCAR(nil) = %f, want NaN", got)
	}
}

func TestMeanCAR_Single(t *testing.T) {
	got := aggregation.MeanCAR([]float64{0.03})
	if !almostEqual(got, 0.03, 1e-10) {
		t.Errorf("MeanCAR([0.03]) = %f, want 0.03", got)
	}
}

// ── cross-sectional t-test ───────────────────────────────────────────────────

// t = meanCAR / (stdCAR / sqrt(N))
// For [1,2,3]: mean=2, sd=1, N=3 → t = 2 / (1/sqrt(3)) = 2*sqrt(3) ≈ 3.4641
func TestCrossSectionalTTest_Known(t *testing.T) {
	cars := []float64{1.0, 2.0, 3.0}
	got := aggregation.CrossSectionalTTest(cars)
	want := 2.0 * math.Sqrt(3.0)
	if !almostEqual(got, want, 1e-6) {
		t.Errorf("CrossSectionalTTest = %f, want %f", got, want)
	}
}

func TestCrossSectionalTTest_TooFew(t *testing.T) {
	got := aggregation.CrossSectionalTTest([]float64{0.05})
	if !math.IsNaN(got) {
		t.Errorf("CrossSectionalTTest(1 sample) = %f, want NaN", got)
	}
}

func TestCrossSectionalTTest_ZeroVariance(t *testing.T) {
	// All identical → sd approaches 0 → t very large (or +Inf). Should not panic.
	// Float arithmetic: [0.05,0.05,0.05] gives sd≈0 but not exactly 0 due to rounding;
	// result may be very large finite or Inf — either acceptable.
	got := aggregation.CrossSectionalTTest([]float64{0.05, 0.05, 0.05})
	if math.IsNaN(got) || got < 0 {
		t.Errorf("expected large positive (or +Inf) for zero-variance, got %f", got)
	}
}

// ── BMP test ─────────────────────────────────────────────────────────────────
//
// BMP: standardise each CAR_i by its own σ(CAR_i), then t-test the SCARs.
// SCAR_i = CAR_i / σ_i.  Aggregate t = mean(SCAR) / (std(SCAR) / sqrt(N)).
//
// Differs from naive t when σ_i vary (event-induced heteroskedasticity).

func TestBMPTest_DiffersFromNaiveTOnClustered(t *testing.T) {
	// Construct cars and sigmas so BMP ≠ naive t-test.
	// High σ on last event → BMP down-weights it vs naive.
	cars := []float64{0.02, 0.04, 0.10}
	sigmas := []float64{0.01, 0.01, 0.20} // last event has huge uncertainty

	bmp := aggregation.BMPTest(cars, sigmas)
	naive := aggregation.CrossSectionalTTest(cars)

	if almostEqual(bmp, naive, 1e-6) {
		t.Errorf("BMP (%f) == naive t (%f); expected to differ with heteroskedastic sigmas", bmp, naive)
	}
}

func TestBMPTest_HomoskedasticEqualsNaive(t *testing.T) {
	// Equal sigmas → SCAR_i proportional to CAR_i → t-ratio same as naive.
	cars := []float64{0.01, 0.02, 0.03}
	sigma := 0.05
	sigmas := []float64{sigma, sigma, sigma}

	bmp := aggregation.BMPTest(cars, sigmas)
	naive := aggregation.CrossSectionalTTest(cars)

	if !almostEqual(bmp, naive, 1e-6) {
		t.Errorf("BMP (%f) != naive (%f) for homoskedastic sigmas", bmp, naive)
	}
}

func TestBMPTest_TooFew(t *testing.T) {
	got := aggregation.BMPTest([]float64{0.05}, []float64{0.01})
	if !math.IsNaN(got) {
		t.Errorf("BMPTest(1 sample) = %f, want NaN", got)
	}
}

// ── Kolari-Pynnönen correction ───────────────────────────────────────────────
//
// KP adjusts the BMP t-stat for cross-sectional correlation when events cluster
// in calendar time.  Correction factor: sqrt((1 + (N-1)*r̄) / N) where r̄ is
// mean pairwise SCAR cross-correlation.
//
// When r̄=0 → no adjustment (identical to BMP).
// When r̄>0 → statistic shrinks (more conservative).

func TestKolariPynnonen_ZeroCorrelation(t *testing.T) {
	// r̄=0 → KP t = BMP t
	cars := []float64{0.01, 0.02, 0.03}
	sigmas := []float64{0.05, 0.05, 0.05}

	bmp := aggregation.BMPTest(cars, sigmas)
	kp := aggregation.KolariPynnonen(cars, sigmas, 0.0)

	if !almostEqual(bmp, kp, 1e-6) {
		t.Errorf("KP (r̄=0) = %f, BMP = %f; expected equal", kp, bmp)
	}
}

func TestKolariPynnonen_PositiveCorrelation(t *testing.T) {
	// r̄>0 → KP t < BMP t (more conservative)
	cars := []float64{0.01, 0.02, 0.03}
	sigmas := []float64{0.05, 0.05, 0.05}

	bmp := aggregation.BMPTest(cars, sigmas)
	kp := aggregation.KolariPynnonen(cars, sigmas, 0.30)

	if kp >= bmp {
		t.Errorf("KP (%f) >= BMP (%f); expected KP < BMP for positive r̄", kp, bmp)
	}
}

func TestKolariPynnonen_NegativeCorrelation(t *testing.T) {
	// r̄<0 → KP t > BMP t (less conservative — hedged events)
	cars := []float64{0.01, 0.02, 0.03}
	sigmas := []float64{0.05, 0.05, 0.05}

	bmp := aggregation.BMPTest(cars, sigmas)
	kp := aggregation.KolariPynnonen(cars, sigmas, -0.20)

	if kp <= bmp {
		t.Errorf("KP (%f) <= BMP (%f); expected KP > BMP for negative r̄", kp, bmp)
	}
}

func TestKolariPynnonen_HandCalc(t *testing.T) {
	// cars=[0.06,0.06,0.06], sigmas=[0.02,0.02,0.02], r̄=0.5, N=3
	// SCAR_i = 0.06/0.02 = 3.0 for all i
	// mean(SCAR)=3, std(SCAR)=0 → BMP t=Inf
	// KP same direction but let's use distinct CARs to get finite result.
	//
	// cars=[0.02,0.04,0.06], sigmas=[0.02,0.02,0.02], r̄=0.5, N=3
	// SCAR = [1,2,3], mean=2, std=1
	// BMP t = 2 / (1/sqrt(3)) = 2*sqrt(3) ≈ 3.4641
	// KP factor = 1 / sqrt(1 + (3-1)*0.5) = 1/sqrt(2) ≈ 0.7071
	// KP t = 3.4641 * 0.7071 ≈ 2.4495

	cars := []float64{0.02, 0.04, 0.06}
	sigmas := []float64{0.02, 0.02, 0.02}
	rBar := 0.5

	got := aggregation.KolariPynnonen(cars, sigmas, rBar)

	bmpT := 2.0 * math.Sqrt(3.0) // verified above
	kpFactor := 1.0 / math.Sqrt(1.0+(float64(len(cars))-1)*rBar)
	want := bmpT * kpFactor

	if !almostEqual(got, want, 1e-5) {
		t.Errorf("KolariPynnonen = %f, want %f", got, want)
	}
}
