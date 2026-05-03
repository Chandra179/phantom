package shared

import (
	"math"
	"testing"
	"time"
)

func TestPricePointZeroVal(t *testing.T) {
	var p PricePoint
	if p.Open != 0 || p.High != 0 || p.Low != 0 || p.Close != 0 || p.Volume != 0 {
		t.Fatal("PricePoint zero-value fields should all be 0")
	}
	if p.Source != "" {
		t.Fatal("PricePoint.Source zero-value should be empty string")
	}
}

func TestEventEquality(t *testing.T) {
	ts := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	a := Event{ID: "e1", Type: "earnings", Timestamp: ts, Asset: "AAPL"}
	b := Event{ID: "e1", Type: "earnings", Timestamp: ts, Asset: "AAPL"}
	if a != b {
		t.Fatal("identical Events should be equal")
	}
}

func TestAssetIDFromString(t *testing.T) {
	raw := "AAPL.US"
	id := AssetID(raw)
	if string(id) != raw {
		t.Fatalf("AssetID round-trip: got %q want %q", id, raw)
	}
}

func TestPriceWindowLogReturns(t *testing.T) {
	series := []PricePoint{
		{Close: 100.0},
		{Close: 110.0},
		{Close: 99.0},
	}
	want := []float64{
		math.Log(110.0 / 100.0),
		math.Log(99.0 / 110.0),
	}
	pw := PriceWindow{}
	got := pw.LogReturns(series)
	if len(got) != len(want) {
		t.Fatalf("LogReturns len: got %d want %d", len(got), len(want))
	}
	const tol = 1e-9
	for i := range want {
		if math.Abs(got[i]-want[i]) > tol {
			t.Errorf("LogReturns[%d]: got %.15f want %.15f", i, got[i], want[i])
		}
	}
}

func TestPriceWindowLogReturnsEmpty(t *testing.T) {
	pw := PriceWindow{}
	if got := pw.LogReturns(nil); len(got) != 0 {
		t.Fatalf("LogReturns(nil) should return empty slice, got len %d", len(got))
	}
	single := []PricePoint{{Close: 100.0}}
	if got := pw.LogReturns(single); len(got) != 0 {
		t.Fatalf("LogReturns single element should return empty slice, got len %d", len(got))
	}
}

func TestTimeRange(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	r := TimeRange{From: from, To: to}
	if !r.From.Equal(from) || !r.To.Equal(to) {
		t.Fatal("TimeRange fields not set correctly")
	}
}
