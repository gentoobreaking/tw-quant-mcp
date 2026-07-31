package engine

import (
	"math"
	"testing"
)

func TestTickPrices(t *testing.T) {
	cases := []struct {
		prev, up, down float64
	}{
		{9.9, 10.89, 8.91},         // <10: tick 0.01
		{20.0, 22.00, 18.00},       // <50: tick 0.05
		{60.0, 66.00, 54.00},       // <100: tick 0.1
		{200.0, 220.00, 180.00},    // <500: tick 0.5
		{600.0, 660.00, 540.00},    // <1000: tick 1
		{2000.0, 2200.00, 1800.00}, // <5000: tick 5
		{6000.0, 6600.00, 5400.00}, // >=5000: tick 10
	}
	for _, c := range cases {
		if got := LimitUpPrice(c.prev); math.Abs(got-c.up) > 1e-6 {
			t.Errorf("prev=%v 漲停價應為 %v，實際 %v", c.prev, c.up, got)
		}
		if got := LimitDownPrice(c.prev); math.Abs(got-c.down) > 1e-6 {
			t.Errorf("prev=%v 跌停價應為 %v，實際 %v", c.prev, c.down, got)
		}
	}
	// 未到漲停價 → 非漲停
	if IsLimitUp(21.99, 20) {
		t.Error("21.99 非 22.00 之漲停價，應為 false")
	}
	if !IsLimitUp(22.00, 20) {
		t.Error("22.00 應判定為漲停")
	}
	if !IsLimitDown(18.00, 20) {
		t.Error("18.00 應判定為跌停")
	}
	if prev := 0.0; IsLimitUp(5, prev) {
		t.Error("昨收 0（新上市）不判定漲跌停")
	}
}
