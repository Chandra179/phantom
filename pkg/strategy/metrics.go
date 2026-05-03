package strategy

import "math"

// SharpeRatio computes annualized Sharpe ratio.
// returns: log returns in measurement period.
// rfRate: risk-free rate per period (e.g. 0.05/252 for daily).
// periodsPerYear: e.g. 252 daily, 52 weekly, 12 monthly.
func SharpeRatio(returns []float64, rfRate float64, periodsPerYear float64) float64 {
	n := len(returns)
	if n < 2 {
		return 0
	}
	nf := float64(n)
	mean := 0.0
	for _, r := range returns {
		mean += r
	}
	mean /= nf
	excessMean := mean - rfRate

	variance := 0.0
	for _, r := range returns {
		d := r - mean
		variance += d * d
	}
	variance /= nf - 1
	std := math.Sqrt(variance)
	if std == 0 {
		return 0
	}
	return (excessMean / std) * math.Sqrt(periodsPerYear)
}

// MaxDrawdown returns maximum drawdown from peak to trough.
// cumulative: series of cumulative values (portfolio equity curve).
// Returns (maxDrawdown, peakIndex, troughIndex).
// Drawdowns are negative fractions.
func MaxDrawdown(cumulative []float64) (float64, int, int) {
	if len(cumulative) < 2 {
		return 0, 0, 0
	}
	peak := cumulative[0]
	peakIdx := 0
	maxDD := 0.0
	troughIdx := 0
	troughPeakIdx := 0
	for i := 1; i < len(cumulative); i++ {
		if cumulative[i] > peak {
			peak = cumulative[i]
			peakIdx = i
		}
		dd := (cumulative[i] - peak) / peak
		if dd < maxDD {
			maxDD = dd
			troughIdx = i
			troughPeakIdx = peakIdx
		}
	}
	return maxDD, troughPeakIdx, troughIdx
}

// HitRate returns fraction of positive values.
func HitRate(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	hits := 0
	for _, v := range values {
		if v > 0 {
			hits++
		}
	}
	return float64(hits) / float64(len(values))
}
