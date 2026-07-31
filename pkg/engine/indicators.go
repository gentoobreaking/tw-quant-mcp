package engine

import "math"

// indicators.go 實作 get_stock_daily_quote 之 helper 技術指標（§10.B）：
// MA20/MA60、RSI(14)、MACD(12,26,9)。全為收盤價序列之純函數計算，
// 輸入不足窗口長度時回傳 0（handler 負責以 null/omitempty 呈現）。

// SMA 計算 n 期簡單移動平均；序列不足 n 期時回傳 0。
func SMA(closes []float64, n int) float64 {
	if len(closes) < n || n <= 0 {
		return 0
	}
	sum := 0.0
	for _, c := range closes[len(closes)-n:] {
		sum += c
	}
	return sum / float64(n)
}

// RSI 計算 Wilder 平滑之 n 期相對強弱指標（最後一期值）。
// 序列不足 n+1 期時回傳 0。全漲全跌時依定義回傳 100/0。
func RSI(closes []float64, n int) float64 {
	if n <= 0 || len(closes) < n+1 {
		return 0
	}
	var gain, loss float64
	// 首期以簡單平均初始化
	for i := 1; i <= n; i++ {
		d := closes[i] - closes[i-1]
		if d >= 0 {
			gain += d
		} else {
			loss -= d
		}
	}
	avgGain, avgLoss := gain/float64(n), loss/float64(n)
	for i := n + 1; i < len(closes); i++ {
		d := closes[i] - closes[i-1]
		g, l := 0.0, 0.0
		if d >= 0 {
			g = d
		} else {
			l = -d
		}
		avgGain = (avgGain*float64(n-1) + g) / float64(n)
		avgLoss = (avgLoss*float64(n-1) + l) / float64(n)
	}
	if avgLoss == 0 {
		if avgGain == 0 {
			return 0 // 無波動（判定為中性）
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - 100/(1+rs)
}

// MACDPoint 為 MACD 於單一期之三值（§10.B）。
type MACDPoint struct {
	MACD   float64 `json:"macd"`   // DIF（12-26 EMA 差）
	Signal float64 `json:"signal"` // DEA（DIF 之 9 EMA）
	Hist   float64 `json:"hist"`   // MACD Histogram（DIF − DEA）
}

// MACD 計算 12/26/9 標準參數之 DIF/DEA/Hist（最後一期值）。
// 序列不足 34 期（26+9−1）時回傳零值。
func MACD(closes []float64) MACDPoint {
	const fast, slow, signal = 12, 26, 9
	if len(closes) < slow+signal-1 {
		return MACDPoint{}
	}
	dif := emaSeries(closes, fast, slow)
	dea := emaLast(dif, signal)
	last := dif[len(dif)-1]
	return MACDPoint{MACD: last, Signal: dea, Hist: last - dea}
}

// emaSeries 計算 fast/slow 兩條 EMA 之差（DIF 序列）。
func emaSeries(closes []float64, fast, slow int) []float64 {
	ef := emaValues(closes, fast)
	es := emaValues(closes, slow)
	out := make([]float64, len(ef))
	for i := range ef {
		out[i] = ef[i] - es[i]
	}
	return out
}

// emaValues 計算收盤價之 n 期 EMA（alpha=2/(n+1)，以 SMA 前導）。
func emaValues(v []float64, n int) []float64 {
	out := make([]float64, len(v))
	if len(v) < n {
		return out
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += v[i]
	}
	alpha := 2 / float64(n+1)
	out[n-1] = sum / float64(n)
	for i := n; i < len(v); i++ {
		out[i] = v[i]*alpha + out[i-1]*(1-alpha)
	}
	return out
}

// emaLast 回傳序列最後一期的 EMA（獨立重新計算，避免相依前導 SMA 位置）。
func emaLast(v []float64, n int) float64 {
	if len(v) < n {
		return 0
	}
	sum := 0.0
	for i := len(v) - n; i < len(v); i++ {
		sum += v[i]
	}
	alpha := 2 / float64(n+1)
	prev := sum / float64(n)
	for i := len(v) - n + 1; i < len(v); i++ {
		prev = v[i]*alpha + prev*(1-alpha)
	}
	return prev
}

// round2 四捨五入至小數第二位（指標輸出格式化）。
func round2(f float64) float64 {
	return math.Round(f*100) / 100
}
