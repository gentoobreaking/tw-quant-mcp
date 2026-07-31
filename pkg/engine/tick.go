package engine

// tick.go 實作上市/上櫃漲跌停價之 tick 進位演算法（TSE 級距規則）。
// 用於 get_market_summary 之漲跌停家數判定（§10.B）。

import "math"

// tickFor 依昨收價回傳股價級距（元）。
// 級距表：<10 → 0.01；<50 → 0.05；<100 → 0.1；<500 → 0.5；
// <1000 → 1；<5000 → 5；≥5000 → 10。
func tickFor(prev float64) float64 {
	switch {
	case prev < 10:
		return 0.01
	case prev < 50:
		return 0.05
	case prev < 100:
		return 0.1
	case prev < 500:
		return 0.5
	case prev < 1000:
		return 1
	case prev < 5000:
		return 5
	default:
		return 10
	}
}

func tickCeil(v, tick float64) float64 {
	return math.Ceil(v/tick-1e-9) * tick
}

func tickFloor(v, tick float64) float64 {
	return math.Floor(v/tick+1e-9) * tick
}

// LimitUpPrice 計算漲停價（昨收 × 1.10 向上進位）。
func LimitUpPrice(prev float64) float64 {
	return tickCeil(prev*1.1, tickFor(prev))
}

// LimitDownPrice 計算跌停價（昨收 × 0.90 向下捨位）。
func LimitDownPrice(prev float64) float64 {
	return tickFloor(prev*0.9, tickFor(prev))
}

// IsLimitUp 判定收盤價是否為當日漲停價（誤差 1e-6）。
func IsLimitUp(close, prev float64) bool {
	return prev > 0 && math.Abs(close-LimitUpPrice(prev)) < 1e-6
}

// IsLimitDown 判定收盤價是否為當日跌停價（誤差 1e-6）。
func IsLimitDown(close, prev float64) bool {
	return prev > 0 && math.Abs(close-LimitDownPrice(prev)) < 1e-6
}
