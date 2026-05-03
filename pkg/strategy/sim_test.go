package strategy

import (
	"math"
	"testing"
)

func TestSimulateTrades_Threshold(t *testing.T) {
	cars := []float64{0.02, -0.01, 0.005, 0.03}
	entryPrices := []float64{100, 100, 100, 100}
	exitPrices := []float64{105, 95, 101, 110}

	// threshold=0.01: cars[0]=0.02 > 0.01, cars[2]=0.005 < 0.01, cars[3]=0.03 > 0.01
	// cars[1]=-0.01 is not > 0.01 => 2 trades at indices 0,3
	trades := SimulateTrades(cars, entryPrices, exitPrices, 0.01, 0)
	if len(trades) != 2 {
		t.Fatalf("expected 2 trades, got %d", len(trades))
	}
	wantR0 := math.Log(105.0 / 100.0)
	if math.Abs(trades[0].Return-wantR0) > 1e-10 {
		t.Errorf("trade 0 return = %f, want %f", trades[0].Return, wantR0)
	}
	wantR1 := math.Log(110.0 / 100.0)
	if math.Abs(trades[1].Return-wantR1) > 1e-10 {
		t.Errorf("trade 1 return = %f, want %f", trades[1].Return, wantR1)
	}
}

func TestSimulateTrades_Slippage5bps(t *testing.T) {
	cars := []float64{0.02}
	entryPrices := []float64{100}
	exitPrices := []float64{105}

	tradesNoSlip := SimulateTrades(cars, entryPrices, exitPrices, 0.01, 0)
	tradesSlip := SimulateTrades(cars, entryPrices, exitPrices, 0.01, 5)

	if len(tradesNoSlip) != 1 || len(tradesSlip) != 1 {
		t.Fatal("expected 1 trade")
	}

	// 5bps slippage: entry=100*(1+0.0005)=100.05, exit=105*(1-0.0005)=104.9475
	wantSlip := math.Log(104.9475 / 100.05)
	if math.Abs(tradesSlip[0].Return-wantSlip) > 1e-10 {
		t.Errorf("slipped return = %f, want %f", tradesSlip[0].Return, wantSlip)
	}
	// Slippage reduces return
	if tradesSlip[0].Return >= tradesNoSlip[0].Return {
		t.Error("slippage should reduce trade return")
	}
}

func TestSimulateTrades_EdgeCases(t *testing.T) {
	if trades := SimulateTrades(nil, nil, nil, 0.01, 0); trades != nil {
		t.Error("nil input should return nil")
	}
	if trades := SimulateTrades([]float64{}, []float64{}, []float64{}, 0.01, 0); len(trades) != 0 {
		t.Error("empty input should return empty slice")
	}
	if trades := SimulateTrades([]float64{0.02}, []float64{100}, []float64{}, 0.01, 0); trades != nil {
		t.Error("mismatched lengths should return nil")
	}
}

func TestComputeMetrics_Basic(t *testing.T) {
	trades := []Trade{
		{EntryPrice: 100, ExitPrice: 105, Return: math.Log(105.0 / 100.0)},
		{EntryPrice: 100, ExitPrice: 102, Return: math.Log(102.0 / 100.0)},
		{EntryPrice: 100, ExitPrice: 95, Return: math.Log(95.0 / 100.0)},
	}

	result := ComputeMetrics(trades, 0, 252)

	if result.NumTrades != 3 {
		t.Errorf("NumTrades = %d, want 3", result.NumTrades)
	}
	if result.HitRate != 2.0/3.0 {
		t.Errorf("HitRate = %f, want %f", result.HitRate, 2.0/3.0)
	}
	if result.MaxDrawdown >= 0 {
		t.Errorf("MaxDrawdown = %f, want negative", result.MaxDrawdown)
	}
}

func TestComputeMetrics_EmptyTrades(t *testing.T) {
	result := ComputeMetrics(nil, 0, 252)
	if result.NumTrades != 0 {
		t.Error("empty trades should return zero-valued result")
	}
}
