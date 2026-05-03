package strategy

import (
	"math"
	"testing"
)

func TestSharpeRatio_NumpyGolden(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.005}
	dailySharpe := SharpeRatio(returns, 0, 1) // no annualization, no rf
	// numpy: np.mean([0.01, 0.02, -0.005]) / np.std([0.01, 0.02, -0.005], ddof=1)
	// mean = 0.008333333333333333, std = 0.012583056205200843
	want := 0.008333333333333333 / 0.012583056205200843
	if math.Abs(dailySharpe-want) > 1e-6 {
		t.Errorf("SharpeRatio = %f, want %f", dailySharpe, want)
	}

	annualized := SharpeRatio(returns, 0, 252)
	wantAnnual := want * math.Sqrt(252)
	if math.Abs(annualized-wantAnnual) > 1e-6 {
		t.Errorf("annualized Sharpe = %f, want %f", annualized, wantAnnual)
	}
}

func TestSharpeRatio_RFRate(t *testing.T) {
	returns := []float64{0.01, 0.02, -0.005}
	// With 5% annual rf = 0.05/252 daily
	rfDaily := 0.05 / 252
	sharpe := SharpeRatio(returns, rfDaily, 252)
	sharpeNoRF := SharpeRatio(returns, 0, 252)
	if sharpe >= sharpeNoRF {
		t.Error("Sharpe with positive rf should be lower than with zero rf")
	}
}

func TestSharpeRatio_EdgeCases(t *testing.T) {
	if SharpeRatio(nil, 0, 252) != 0 {
		t.Error("nil slice should return 0")
	}
	if SharpeRatio([]float64{}, 0, 252) != 0 {
		t.Error("empty slice should return 0")
	}
	if SharpeRatio([]float64{1.0}, 0, 252) != 0 {
		t.Error("single element should return 0")
	}
	if SharpeRatio([]float64{0, 0, 0}, 0, 252) != 0 {
		t.Error("zero variance should return 0")
	}
	if SharpeRatio([]float64{1, 1, 1}, 0, 252) != 0 {
		t.Error("constant returns should return 0")
	}
}

func TestMaxDrawdown_KnownSeries(t *testing.T) {
	cum := []float64{100, 110, 102, 95, 105, 120}
	dd, peak, trough := MaxDrawdown(cum)
	// Peak at 110 (idx 1), trough at 95 (idx 3): (95-110)/110 = -0.1363636
	wantDD := -0.13636363636363635
	if math.Abs(dd-wantDD) > 1e-10 {
		t.Errorf("MaxDrawdown = %f, want %f", dd, wantDD)
	}
	if peak != 1 || trough != 3 {
		t.Errorf("MaxDrawdown peak=%d trough=%d, want peak=1 trough=3", peak, trough)
	}
}

func TestMaxDrawdown_Increasing(t *testing.T) {
	cum := []float64{100, 110, 120, 130}
	dd, _, _ := MaxDrawdown(cum)
	if dd != 0 {
		t.Errorf("MaxDrawdown of increasing series = %f, want 0", dd)
	}
}

func TestMaxDrawdown_EdgeCases(t *testing.T) {
	dd, _, _ := MaxDrawdown(nil)
	if dd != 0 {
		t.Error("nil slice should return 0")
	}
	dd, _, _ = MaxDrawdown([]float64{100})
	if dd != 0 {
		t.Error("single element should return 0")
	}
}

func TestHitRate_Basic(t *testing.T) {
	if HitRate([]float64{1, -1, 2, -2, 3}) != 0.6 {
		t.Error("HitRate of [1,-1,2,-2,3] should be 0.6")
	}
	if HitRate(nil) != 0 {
		t.Error("HitRate of nil should be 0")
	}
	if HitRate([]float64{-1, -2}) != 0 {
		t.Error("HitRate of all negative should be 0")
	}
	if HitRate([]float64{1, 2}) != 1 {
		t.Error("HitRate of all positive should be 1")
	}
}
