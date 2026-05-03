package aggregation

import "math"

// MeanCAR returns cross-sectional mean of CARs. Returns NaN for empty slice.
func MeanCAR(cars []float64) float64 {
	if len(cars) == 0 {
		return math.NaN()
	}
	sum := 0.0
	for _, c := range cars {
		sum += c
	}
	return sum / float64(len(cars))
}

// CrossSectionalTTest computes t = mean(CARs) / (std(CARs) / sqrt(N)).
// Returns NaN if N < 2.
func CrossSectionalTTest(cars []float64) float64 {
	n := len(cars)
	if n < 2 {
		return math.NaN()
	}
	nf := float64(n)
	mean := MeanCAR(cars)
	variance := 0.0
	for _, c := range cars {
		d := c - mean
		variance += d * d
	}
	variance /= nf - 1
	std := math.Sqrt(variance)
	return mean / (std / math.Sqrt(nf))
}

// BMPTest is the Boehmer-Musumeci-Poulsen (1991) test.
// Standardises each CAR by its own sigma then applies t-test on SCARs.
// Returns NaN if N < 2.
func BMPTest(cars, sigmas []float64) float64 {
	n := len(cars)
	if n < 2 {
		return math.NaN()
	}
	scars := make([]float64, n)
	for i := range cars {
		scars[i] = cars[i] / sigmas[i]
	}
	return CrossSectionalTTest(scars)
}

// KolariPynnonen adjusts the BMP t-stat for cross-sectional correlation.
// rBar is the mean pairwise SCAR cross-correlation estimate.
// Formula: t_KP = t_BMP / sqrt(1 + (N-1)*rBar)  (Kolari & Pynnönen 2010, eq. 9)
func KolariPynnonen(cars, sigmas []float64, rBar float64) float64 {
	n := float64(len(cars))
	bmp := BMPTest(cars, sigmas)
	denom := math.Sqrt(1.0 + (n-1)*rBar)
	return bmp / denom
}
