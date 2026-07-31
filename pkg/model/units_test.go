package model

import "testing"

func TestThousandToYuan(t *testing.T) {
	tests := []struct {
		in   int64
		want int64
	}{
		{0, 0},
		{1, 1000},
		{123, 123000},
		{12345, 12345000},
	}
	for _, tt := range tests {
		if got := ThousandToYuan(tt.in); got != tt.want {
			t.Errorf("ThousandToYuan(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestLotsToShares(t *testing.T) {
	tests := []struct {
		in   int64
		want int64
	}{
		{0, 0},
		{1, 1000},
		{5, 5000},
		{12345, 12345000},
	}
	for _, tt := range tests {
		if got := LotsToShares(tt.in); got != tt.want {
			t.Errorf("LotsToShares(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestRoundPrice(t *testing.T) {
	tests := []struct {
		in   float64
		want float64
	}{
		{1.2345, 1.23},
		{1.125, 1.13}, // 1.125 可精確表示：112.5 四捨五入（half away from zero）→ 113
		{1234.567, 1234.57},
		{99.99, 99.99},
		{100, 100},
		{1.2, 1.2},
	}
	for _, tt := range tests {
		if got := RoundPrice(tt.in); got != tt.want {
			t.Errorf("RoundPrice(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPercentConversions(t *testing.T) {
	if got := RatioToPercent(0.0148); got != 1.48 {
		t.Errorf("RatioToPercent(0.0148) = %v, want 1.48", got)
	}
	if got := PercentToRatio(1.48); got != 0.0148 {
		t.Errorf("PercentToRatio(1.48) = %v, want 0.0148", got)
	}
	if got := RatioToPercent(1); got != 100 {
		t.Errorf("RatioToPercent(1) = %v, want 100", got)
	}
}
