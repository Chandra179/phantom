package signalmatrix

import (
	"math"
	"testing"
	"time"

	"phantom/pkg/aggregation"
	"phantom/pkg/shared"
)

// TestBernankeKuttner2005FOMCSign verifies FOMC surprise direction → CAR sign.
//
// Bernanke-Kuttner (2005): expansionary (rate cut) surprise → positive equity CAR;
// contractionary (rate hike) surprise → negative CAR.
//
// Uses synthetic securities with known post-T0 drift to simulate FOMC surprise.
func TestBernankeKuttner2005FOMCSign(t *testing.T) {
	base := time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)
	nDays := 400
	t0Idx := 260

	// Market returns
	rm := make([]float64, nDays)
	rm[0] = 0.0
	for i := 1; i < nDays; i++ {
		rm[i] = 0.0003 + 0.008*(float64(i%5)/5.0-0.5)
	}

	mkCloses := make([]float64, nDays)
	mkCloses[0] = 100.0
	for i := 1; i < nDays; i++ {
		mkCloses[i] = mkCloses[i-1] * math.Exp(rm[i])
	}

	type fomcCase struct {
		name    string
		drift   float64 // post-T0 drift per day
		wantPos bool    // true = expansionary → positive CAR
	}

	cases := []fomcCase{
		{name: "ExpansionarySurprise_A", drift: +0.006, wantPos: true},
		{name: "ExpansionarySurprise_B", drift: +0.004, wantPos: true},
		{name: "ExpansionarySurprise_C", drift: +0.005, wantPos: true},
		{name: "ContractionarySurprise_A", drift: -0.006, wantPos: false},
		{name: "ContractionarySurprise_B", drift: -0.004, wantPos: false},
		{name: "ContractionarySurprise_C", drift: -0.005, wantPos: false},
	}

	cars := make([]float64, len(cases))

	for ci, c := range cases {
		closes := make([]float64, nDays)
		closes[0] = 100.0
		for i := 1; i < nDays; i++ {
			r := 0.0001 + 1.0*rm[i]
			// Post-T0 drift for FOMC reaction days [0, +3]
			if i > t0Idx && i <= t0Idx+3 {
				r += c.drift
			}
			closes[i] = closes[i-1] * math.Exp(r)
		}

		stkPrices := make([]shared.PricePoint, nDays)
		spyPrices := make([]shared.PricePoint, nDays)
		for i := 0; i < nDays; i++ {
			ts := base.AddDate(0, 0, i)
			stkPrices[i] = shared.PricePoint{AssetID: shared.AssetID(c.name), Timestamp: ts, Close: closes[i], Volume: 1}
			spyPrices[i] = shared.PricePoint{AssetID: "SPY", Timestamp: ts, Close: mkCloses[i], Volume: 1}
		}

		evt := shared.Event{
			ID:        c.name,
			Type:      "fomc",
			Timestamp: stkPrices[t0Idx].Timestamp,
			Asset:     shared.AssetID(c.name),
		}

		wb := NewWindowBuilderImpl(stkPrices)
		pw, err := wb.BuildWindows(evt)
		if err != nil {
			t.Fatalf("%s: BuildWindows: %v", c.name, err)
		}

		l1Fn := func(p []shared.PricePoint) []float64 {
			return shared.PriceWindow{}.LogReturns(p)
		}

		l1s, l1e := t0Idx-250, t0Idx-11
		l2s, l2e := t0Idx-10, t0Idx+10

		mkL1 := spyPrices[l1s : l1e+1]
		mkL2 := spyPrices[l2s : l2e+1]

		riL1 := l1Fn(pw.Estimation)
		rmL1 := l1Fn(mkL1)
		riL2 := l1Fn(pw.Event)
		rmL2 := l1Fn(mkL2)

		a, b, _ := ols(riL1, rmL1)

		ar := make([]float64, len(riL2))
		for i := range ar {
			ar[i] = riL2[i] - (a + b*rmL2[i])
		}
		car := 0.0
		for _, v := range ar {
			car += v
		}
		cars[ci] = car
	}

	// Each individual sign check
	for ci, c := range cases {
		car := cars[ci]
		if c.wantPos && car <= 0 {
			t.Errorf("%s: CAR = %.6f, want > 0 (Bernanke-Kuttner expansionary → positive)", c.name, car)
		}
		if !c.wantPos && car >= 0 {
			t.Errorf("%s: CAR = %.6f, want < 0 (Bernanke-Kuttner contractionary → negative)", c.name, car)
		}
	}

	// Group mean comparison: mean(expansionary CAR) > mean(contractionary CAR)
	expCAR := cars[0:3]
	conCAR := cars[3:6]
	meanExp := aggregation.MeanCAR(expCAR)
	meanCon := aggregation.MeanCAR(conCAR)
	if meanExp <= meanCon {
		t.Errorf("mean expansionary CAR = %.6f <= mean contractionary CAR = %.6f, want opposite", meanExp, meanCon)
	}

	t.Logf("Expansionary CARs: %v (mean=%.6f)", cars[:3], meanExp)
	t.Logf("Contractionary CARs: %v (mean=%.6f)", cars[3:], meanCon)
}
