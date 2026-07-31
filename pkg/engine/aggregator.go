package engine

import (
	"fmt"
	"sort"
	"strings"

	"tw-quant-mcp/pkg/model"
)

// Aggregator 是 Snapshot → 1m/5m K 線之記憶體重採樣器（§8.4）：
// 依 Snapshot 之 tlong 分桶至 HH:MM:00，全程零 HTTP（純記憶體讀取）。
// 5m K 線由 1m 桶二次聚合（不重複計算）。
type Aggregator struct {
	rings *RingStore
}

// NewAggregator 建立綁定 RingStore 之重採樣器。
func NewAggregator(rings *RingStore) *Aggregator {
	return &Aggregator{rings: rings}
}

// Klines 重採樣指定代碼之盤中 K 線（§10.A get_intraday_kline 讀取路徑）。
// timeframe 支援 "1m"/"5m"；limit > 0 時僅回傳最後 limit 根；
// 無資料或非法參數回傳錯誤（供 Tool handler 對 Client 回覆）。
func (a *Aggregator) Klines(code, timeframe string, limit int) ([]model.Candle, error) {
	snaps := a.rings.Snapshots(code)
	if len(snaps) == 0 {
		return nil, fmt.Errorf("engine: 代碼 %s 目前無盤中資料（請先加入 watchlist 並等待採樣）", code)
	}
	var out []model.Candle
	switch timeframe {
	case "1m":
		out = resample1m(snaps)
	case "5m":
		out = resample5m(resample1m(snaps))
	default:
		return nil, fmt.Errorf("engine: timeframe %q 僅支援 1m/5m", timeframe)
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, nil
}

// bucket 是 1m 桶之累計狀態。
type bucket struct {
	open, high, low, close float64
	firstTV, lastTV        int64
	n                      int
}

// resample1m 依 §8.4 規則表將快照重採樣為 1m K 線：
// Open=桶首 z、High=max、Low=min、Close=桶末 z、
// Volume=桶末 tv − 桶初 tv（單筆桶以該筆 tv 近似）。
func resample1m(snaps []model.Snapshot) []model.Candle {
	byKey := make(map[string]*bucket)
	var keys []string
	for _, s := range snaps {
		key := s.Time.In(model.Taipei()).Format("15:04:00")
		b, ok := byKey[key]
		if !ok {
			byKey[key] = &bucket{open: s.Last, high: s.Last, low: s.Last, close: s.Last,
				firstTV: s.MinuteVol, lastTV: s.MinuteVol, n: 1}
			keys = append(keys, key)
			continue
		}
		if s.Last > b.high {
			b.high = s.Last
		}
		if s.Last < b.low {
			b.low = s.Last
		}
		b.close = s.Last
		b.lastTV = s.MinuteVol
		b.n++
	}
	sort.Strings(keys)
	out := make([]model.Candle, 0, len(keys))
	for _, k := range keys {
		b := byKey[k]
		vol := b.lastTV - b.firstTV
		if b.n == 1 {
			vol = b.firstTV
		}
		if vol < 0 {
			vol = 0
		}
		out = append(out, model.Candle{
			Timestamp: k,
			Open:      b.open,
			High:      b.high,
			Low:       b.low,
			Close:     b.close,
			Volume:    vol,
		})
	}
	return out
}

// resample5m 將 1m K 線二次聚合為 5m（§8.4：不重複計算）。
// 桶對齊自 09:00 起每 5 分鐘（09:00 % 5 == 0）。
func resample5m(bars []model.Candle) []model.Candle {
	byKey := make(map[string]*bucket5)
	var keys []string
	for _, b := range bars {
		h, m := minuteOf(b.Timestamp)
		bucketMin := (h*60 + m) / 5 * 5
		key := fmt.Sprintf("%02d:%02d:00", bucketMin/60, bucketMin%60)
		g, ok := byKey[key]
		if !ok {
			byKey[key] = &bucket5{open: b.Open, high: b.High, low: b.Low, close: b.Close, vol: b.Volume}
			keys = append(keys, key)
			continue
		}
		if b.High > g.high {
			g.high = b.High
		}
		if b.Low < g.low {
			g.low = b.Low
		}
		g.close = b.Close
		g.vol += b.Volume
	}
	sort.Strings(keys)
	out := make([]model.Candle, 0, len(keys))
	for _, k := range keys {
		g := byKey[k]
		out = append(out, model.Candle{
			Timestamp: k,
			Open:      g.open,
			High:      g.high,
			Low:       g.low,
			Close:     g.close,
			Volume:    g.vol,
		})
	}
	return out
}

type bucket5 struct {
	open, high, low, close float64
	vol                    int64
}

// minuteOf 解析 "HH:MM:00" 回傳小時與分鐘。
func minuteOf(ts string) (int, int) {
	parts := strings.Split(ts, ":")
	if len(parts) < 2 {
		return 0, 0
	}
	var h, m int
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return h, m
}
