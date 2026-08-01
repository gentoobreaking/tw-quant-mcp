package engine

import (
	"sort"
	"testing"
	"time"
)

// seedDay 種入單一交易日（09:00–13:30 每 5 秒）之快照，回傳 RingStore。
func seedDay(rs *RingStore, code string) {
	h, m := 9, 0
	tv := int64(0)
	for {
		last := 100 + float64((h*60+m)%10) + 0.5
		tv += 100
		rs.Append(sn(code, h, m, 5, last, tv))
		m += 5
		if m >= 60 {
			m -= 60
			h++
		}
		if h > 13 || (h == 13 && m > 30) {
			break
		}
	}
}

// klineAssembly 執行單次組裝並回傳耗時。
func klineAssembly(agg *Aggregator, code, timeframe string) time.Duration {
	t0 := time.Now()
	if _, err := agg.Klines(code, timeframe, 0); err != nil {
		panic(err)
	}
	return time.Since(t0)
}

// p50p95 回傳排序後延遲序列之 P50/P95。
func p50p95(ds []time.Duration) (p50, p95 time.Duration) {
	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	p50 = ds[len(ds)/2]
	idx95 := len(ds) * 95 / 100
	if idx95 >= len(ds) {
		idx95 = len(ds) - 1
	}
	return p50, ds[idx95]
}

// T018 驗收：盤中 K 線組裝延遲 P95 < 10ms（§12.9 目標，純記憶體零 HTTP）。
func TestKlinesAssemblyP95Below10ms(t *testing.T) {
	rs := NewRingStore()
	seedDay(rs, "2330")
	agg := NewAggregator(rs)

	const iterations = 300
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		durations = append(durations, klineAssembly(agg, "2330", "1m"))
	}
	p50, p95 := p50p95(durations)
	t.Logf("1m 組裝 P50=%s P95=%s（%d 次）", p50, p95, iterations)
	if p95 > 10*time.Millisecond {
		t.Errorf("P95 %s 超過 10ms 目標", p95)
	}

	durations = durations[:0]
	for i := 0; i < iterations; i++ {
		durations = append(durations, klineAssembly(agg, "2330", "5m"))
	}
	p50, p95 = p50p95(durations)
	t.Logf("5m 組裝 P50=%s P95=%s（%d 次）", p50, p95, iterations)
	if p95 > 10*time.Millisecond {
		t.Errorf("5m P95 %s 超過 10ms 目標", p95)
	}
}

// BenchmarkKlinesAssembly：盤中 K 線組裝效能（P50/P95 < 10ms 目標，§12.9）。
func BenchmarkKlinesAssembly(b *testing.B) {
	rs := NewRingStore()
	seedDay(rs, "2330")
	agg := NewAggregator(rs)

	timeframes := []string{"1m", "5m"}
	for _, tf := range timeframes {
		tf := tf
		b.Run(tf, func(b *testing.B) {
			durations := make([]time.Duration, 0, b.N)
			for i := 0; i < b.N; i++ {
				durations = append(durations, klineAssembly(agg, "2330", tf))
			}
			p50, p95 := p50p95(durations)
			b.ReportMetric(float64(p50.Microseconds()), "p50_us")
			b.ReportMetric(float64(p95.Microseconds()), "p95_us")
			if p95 > 10*time.Millisecond {
				b.Errorf("P95 %s 超過 10ms 目標", p95)
			}
		})
	}

	// 15 檔 watchlist 規模（§12.4 批次上限）之組裝耗時
	b.Run("watchlist15_1m", func(b *testing.B) {
		codes := make([]string, 15)
		for i := range codes {
			code := string(rune('a' + i))
			codes[i] = "23" + code
			seedDay(rs, codes[i])
		}
		durations := make([]time.Duration, 0, b.N)
		for i := 0; i < b.N; i++ {
			t0 := time.Now()
			for _, code := range codes {
				if _, err := agg.Klines(code, "1m", 0); err != nil {
					b.Fatal(err)
				}
			}
			durations = append(durations, time.Since(t0))
		}
		p50, p95 := p50p95(durations)
		b.ReportMetric(float64(p50.Microseconds()), "p50_us")
		b.ReportMetric(float64(p95.Microseconds()), "p95_us")
	})
}
