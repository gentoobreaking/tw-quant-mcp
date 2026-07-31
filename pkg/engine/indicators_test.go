package engine

import (
	"math"
	"testing"
)

func TestSMA(t *testing.T) {
	cs := []float64{1, 2, 3, 4, 5}
	if v := SMA(cs, 3); v != 4 {
		t.Errorf("SMA(3) 應為 4，實際 %v", v)
	}
	if v := SMA(cs, 5); v != 3 {
		t.Errorf("SMA(5) 應為 3，實際 %v", v)
	}
	if v := SMA(cs, 6); v != 0 {
		t.Errorf("窗口不足應為 0，實際 %v", v)
	}
}

func TestRSI(t *testing.T) {
	// 全漲序列 → 100
	up := make([]float64, 20)
	for i := range up {
		up[i] = float64(i + 1)
	}
	if v := RSI(up, 14); v != 100 {
		t.Errorf("全漲 RSI 應為 100，實際 %v", v)
	}
	// 全跌序列 → 0
	down := make([]float64, 20)
	for i := range down {
		down[i] = float64(20 - i)
	}
	if v := RSI(down, 14); v != 0 {
		t.Errorf("全跌 RSI 應為 0，實際 %v", v)
	}
	// 平穩序列 → 中性（無波動 0）
	flat := make([]float64, 20)
	for i := range flat {
		flat[i] = 10
	}
	if v := RSI(flat, 14); v != 0 {
		t.Errorf("無波動 RSI 應為 0，實際 %v", v)
	}
	// 長度不足 → 0
	if v := RSI([]float64{1, 2, 3}, 14); v != 0 {
		t.Errorf("不足窗口 RSI 應為 0，實際 %v", v)
	}
}

func TestMACD(t *testing.T) {
	// 長度不足 → 零值
	if v := MACD([]float64{1, 2, 3}); v.MACD != 0 {
		t.Errorf("不足窗口 MACD 應為零值")
	}
	// 上升趨勢：DIF > 0 且 Hist > 0（首段 34+ 期單調上升）
	up := make([]float64, 60)
	for i := range up {
		up[i] = 100 + float64(i)*2
	}
	v := MACD(up)
	if v.MACD <= 0 {
		t.Errorf("上升趨勢 DIF 應 > 0，實際 %v", v.MACD)
	}
	// 下降趨勢：DIF < 0
	down := make([]float64, 60)
	for i := range down {
		down[i] = 200 - float64(i)*2
	}
	v = MACD(down)
	if v.MACD >= 0 {
		t.Errorf("下降趨勢 DIF 應 < 0，實際 %v", v.MACD)
	}
	// 常數序列 → 三值皆 0
	flat := make([]float64, 60)
	for i := range flat {
		flat[i] = 50
	}
	v = MACD(flat)
	if math.Abs(v.MACD) > 1e-9 || math.Abs(v.Hist) > 1e-9 {
		t.Errorf("常數序列 MACD 應為 0，實際 %+v", v)
	}
}
