package strategy

import "math"

// Trade represents a single simulated trade.
type Trade struct {
	EntryPrice float64
	ExitPrice  float64
	Return     float64
}

// StrategyResult holds aggregated backtest metrics.
type StrategyResult struct {
	TotalReturn float64
	SharpeRatio float64
	MaxDrawdown float64
	HitRate     float64
	NumTrades   int
}

// SimulateTrades simulates event-driven strategy.
// Enters long when CAR(0,+5) > threshold, entry at T0+1 price, exit at T0+6 price.
// slippageBPS: round-trip slippage in basis points (applied each fill).
func SimulateTrades(cars, entryPrices, exitPrices []float64, threshold float64, slippageBPS float64) []Trade {
	if cars == nil {
		return nil
	}
	n := len(cars)
	if n == 0 {
		return []Trade{}
	}
	if len(entryPrices) != n || len(exitPrices) != n {
		return nil
	}

	slip := slippageBPS / 10000.0
	var trades []Trade

	for i := 0; i < n; i++ {
		if cars[i] > threshold {
			entry := entryPrices[i] * (1 + slip)
			exit := exitPrices[i] * (1 - slip)
			trades = append(trades, Trade{
				EntryPrice: entry,
				ExitPrice:  exit,
				Return:     math.Log(exit / entry),
			})
		}
	}
	return trades
}

// ComputeMetrics calculates backtest stats from trade list.
func ComputeMetrics(trades []Trade, rfRate float64, periodsPerYear float64) StrategyResult {
	if len(trades) == 0 {
		return StrategyResult{}
	}

	returns := make([]float64, len(trades))
	for i, t := range trades {
		returns[i] = t.Return
	}

	cum := make([]float64, len(trades)+1)
	cum[0] = 1.0
	for i, r := range returns {
		cum[i+1] = cum[i] * math.Exp(r)
	}

	maxDD, _, _ := MaxDrawdown(cum)
	sharpe := SharpeRatio(returns, rfRate, periodsPerYear)
	hitRate := HitRate(returns)
	totalReturn := (cum[len(cum)-1] - cum[0]) / cum[0]

	return StrategyResult{
		TotalReturn: totalReturn,
		SharpeRatio: sharpe,
		MaxDrawdown: maxDD,
		HitRate:     hitRate,
		NumTrades:   len(trades),
	}
}
