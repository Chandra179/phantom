package strategy

import (
	"math"
	"testing"
)

func TestExpectedMaxStdNormal(t *testing.T) {
	if ExpectedMaxStdNormal(1) != 0 {
		t.Error("E[max Z_1] should be 0")
	}
	if ExpectedMaxStdNormal(0) != 0 {
		t.Error("E[max Z_0] should be 0")
	}
	// N=100 hand-computed golden
	// sqrt(2*ln(100)) = 3.0349
	// (ln(ln(100)) + gamma - ln(4pi)) / (2*sqrt(2*ln(100)))
	// = (1.5265 + 0.5772 - 2.5310) / 6.0697 = -0.4273 / 6.0697 = -0.0704
	// result = 3.0349 - (-0.0704) = 3.1053
	want := 3.1053
	got := ExpectedMaxStdNormal(100)
	if math.Abs(got-want) > 0.001 {
		t.Errorf("E[max Z_100] = %f, want ~%f", got, want)
	}
	// Monotonic: larger N -> larger E[max]
	if ExpectedMaxStdNormal(10) >= ExpectedMaxStdNormal(100) {
		t.Error("E[max Z] should increase with N")
	}
}

func TestDeflatedSharpe_SingleStrategy(t *testing.T) {
	// N=1 => E[max Z] = 0 => z = SR*sqrt(T) / sqrt(1 + 0.5*SR^2)
	ds := DeflatedSharpe(1.0, 1, 252)
	want := math.Sqrt(252) / math.Sqrt(1.5)
	if math.Abs(ds-want) > 1e-10 {
		t.Errorf("deflated Sharpe N=1: %f, want %f", ds, want)
	}
	// For N=1 but numStrategies=0 edge case
	if DeflatedSharpe(1.0, 0, 252) != 1.0 {
		t.Error("numStrategies=0 should return observed SR")
	}
}

func TestDeflatedSharpe_MoreTrialsPenalizes(t *testing.T) {
	ds100 := DeflatedSharpe(1.0, 100, 252)
	ds1000 := DeflatedSharpe(1.0, 1000, 252)
	if ds1000 >= ds100 {
		t.Error("deflated Sharpe should decrease as num strategies increases")
	}
}

func TestDeflatedSharpe_HigherSRYieldsHigherDSR(t *testing.T) {
	dsLow := DeflatedSharpe(0.5, 100, 252)
	dsHigh := DeflatedSharpe(1.0, 100, 252)
	if dsHigh <= dsLow {
		t.Error("higher observed Sharpe should yield higher deflated Sharpe")
	}
}

func TestDeflatedSharpe_EdgeCases(t *testing.T) {
	if DeflatedSharpe(1.0, 0, 252) != 1.0 {
		t.Error("numStrategies=0 should return observed SR unchanged")
	}
	if DeflatedSharpe(1.0, 1, 1) != 1.0 {
		t.Error("numObs < 2 should return observed SR unchanged")
	}
}
