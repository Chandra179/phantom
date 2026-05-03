package strategy

import "math"

const eulerGamma = 0.5772156649015329

// ExpectedMaxStdNormal returns E[max of N i.i.d. standard normals].
// Bailey & López de Prado (2014), eq. A.1.
func ExpectedMaxStdNormal(N int) float64 {
	if N <= 1 {
		return 0
	}
	n := float64(N)
	lnN := math.Log(n)
	lnlnN := math.Log(lnN)
	return math.Sqrt(2*lnN) - (lnlnN+eulerGamma-math.Log(4*math.Pi))/(2*math.Sqrt(2*lnN))
}

// DeflatedSharpe adjusts observed Sharpe ratio for multiple testing.
// Returns z-score: (SR_obs*sqrt(T) - E[max Z_N]) / sqrt(1 + 0.5*SR_obs^2).
// Higher numStrategies -> lower deflated Sharpe (penalty for data mining).
func DeflatedSharpe(observedSharpe float64, numStrategies int, numObs int) float64 {
	if numStrategies < 1 || numObs < 2 {
		return observedSharpe
	}
	t := float64(numObs)
	eMax := ExpectedMaxStdNormal(numStrategies)
	numerator := observedSharpe*math.Sqrt(t) - eMax
	denominator := math.Sqrt(1.0 + 0.5*observedSharpe*observedSharpe)
	return numerator / denominator
}
