package signalmatrix

import (
	"math"
	"math/rand"
	"testing"

	"phantom/pkg/aggregation"
)

// ols fits market model R_it = α + β·R_mt + ε via OLS.
func ols(ri, rm []float64) (alpha, beta, sigmaEps float64) {
	n := float64(len(ri))
	meanRi, meanRm := 0.0, 0.0
	for i := range ri {
		meanRi += ri[i]
		meanRm += rm[i]
	}
	meanRi /= n
	meanRm /= n

	var cov, varRm float64
	for i := range ri {
		dRi := ri[i] - meanRi
		dRm := rm[i] - meanRm
		cov += dRi * dRm
		varRm += dRm * dRm
	}
	beta = cov / varRm
	alpha = meanRi - beta*meanRm

	var ssr float64
	for i := range ri {
		fitted := alpha + beta*rm[i]
		ssr += (ri[i] - fitted) * (ri[i] - fitted)
	}
	sigmaEps = math.Sqrt(ssr / (n - 2))
	return
}

// mean of float64 slice.
func mean(v []float64) float64 {
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

// TestBrownWarner1985Table2 replicates Brown-Warner (1985) Table 2.
//
// Under the null (random event dates, no real signal), cross-sectional CAR
// t-statistics should be well-specified with empirical standard deviation ≈ 1.0.
//
// Approach:
//   - M = 30 replications, each with N = 15 synthetic securities
//   - Daily returns follow market model: r_i = α + β·r_m + ε
//   - r_m ~ N(0.0005, 0.01²) — CRSP-like daily market returns
//   - β ~ N(1.0, 0.4²), α ~ N(0, 0.0002²), ε ~ N(0, 0.005²)
//   - Random event dates (null: no abnormal return)
//   - Market model OLS on L1 → AR on L2 → CAR → cross-sectional t-test
//   - t-stat std should fall in [0.65, 1.45] for 30 replications
func TestBrownWarner1985Table2ARVariance(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	reps := 30
	securities := 15
	nDays := 400

	tStats := make([]float64, reps)

	for r := 0; r < reps; r++ {
		// Market daily log-returns: ~12% annualised, ~16% vol
		rm := make([]float64, nDays)
		for i := range rm {
			rm[i] = 0.0005 + 0.01*rng.NormFloat64()
		}

		cars := make([]float64, securities)
		for s := 0; s < securities; s++ {
			beta := 0.8 + 0.4*rng.NormFloat64()
			alpha := 0.0002 * rng.NormFloat64()

			ri := make([]float64, nDays)
			for i := range ri {
				ri[i] = alpha + beta*rm[i] + 0.005*rng.NormFloat64()
			}

			// Random event date in middle of series
			t0Idx := 270 + rng.Intn(40)
			l1s, l1e := t0Idx-250, t0Idx-11
			l2s, l2e := t0Idx-10, t0Idx+10

			a, b, _ := ols(ri[l1s:l1e+1], rm[l1s:l1e+1])

			ar := make([]float64, l2e-l2s+1)
			riL2, rmL2 := ri[l2s:l2e+1], rm[l2s:l2e+1]
			for i := range ar {
				ar[i] = riL2[i] - (a + b*rmL2[i])
			}

			car := 0.0
			for _, v := range ar {
				car += v
			}
			cars[s] = car
		}

		tStats[r] = aggregation.CrossSectionalTTest(cars)
	}

	// Empirical std of t-stats — should be ≈ 1.0 under the null
	m := mean(tStats)
	v := 0.0
	for _, t := range tStats {
		d := t - m
		v += d * d
	}
	std := math.Sqrt(v / (float64(len(tStats)) - 1))

	// 95% CI for std with 30 reps: χ²(29) → ~[0.79, 1.37].
	// Broaden to [0.65, 1.45] for 15 securities per rep (extra sampling noise).
	if std < 0.65 || std > 1.45 {
		t.Errorf("BW1985 Tbl 2: std(t-stats) = %.4f, want ≈ 1.0 in [0.65, 1.45]", std)
	}
	t.Logf("BW1985 Tbl 2: std(t-stats) = %.4f (mean=%.4f, %d reps, %d securities)", std, m, reps, securities)
}
