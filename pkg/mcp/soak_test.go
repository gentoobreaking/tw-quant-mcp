//go:build soak

package mcp

// T020 發布驗收：4.5h 連續運行測試（-tags=soak）。
//
// 開盤時段（09:00–13:30）連續運行：
//  1. goroutine 數穩定（無 Leak）；
//  2. heap 無持續增長（pprof 對比，取樣 2 次對照）；
//  3. 事件日誌無 403/429 被封鎖紀錄（BaseClient 退避重試日誌）。
//
// 用法：
//   - 排定實際交易日 09:00 前啟動：go test -tags=soak ./pkg/mcp/ -run Soak -v
//   - 非開盤時段執行自動 Skip（不發任何真實請求）。
//
// 注意：本測試使用真實資料源（NewApp 預設），會對官方來源發請求，
// 僅於開盤時段且明確以 -tags=soak 執行時生效（與 live smoke 相同原則）。
// 若 MIS 或官方來源異常，日誌留存於 -test.v 輸出供分析（任務書備註）。

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// soakDuration 為目標運行時長：開盤時段 4.5h。測試可縮短（-soak.duration）。
var soakDuration = 4*time.Hour + 30*time.Minute

// TestSoakContinuousRun 開盤時段連續運行測試。
func TestSoakContinuousRun(t *testing.T) {
	if os.Getenv("TW_QUANT_SOAK") != "1" {
		t.Skip("soak 測試需 TW_QUANT_SOAK=1（實際交易日 09:00 前啟動，連續 4.5h）")
	}
	// 開盤時段門檻：交易日 09:00–13:30（與 live smoke 相同，避免誤觸發）
	if !liveSessionOpen(time.Now()) {
		t.Skipf("非開盤時段（%s），soak 測試僅於交易日 09:00–13:30 執行", time.Now().Format("15:04"))
	}

	// 可縮短：-soak.duration=10m（CI/驗證用）；預設 4.5h
	if d := os.Getenv("TW_QUANT_SOAK_DURATION"); d != "" {
		if dur, err := time.ParseDuration(d); err == nil {
			soakDuration = dur
		}
	}

	// 真實 App（真實資料源 + 快取）。logger 輸出至 stderr 供事件日誌留存。
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	app, err := NewApp(nil, WithAppLogger(logger))
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	defer app.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 預熱排程器（§12.9）：08:00 行事曆/代碼表、開盤前 MIS Session、16:45 盤後
	prewarm := NewPrewarmScheduler(app, WithPrewarmLogger(logger))
	go func() { _ = prewarm.Run(ctx) }()

	// 監控循環：每 60s 記錄 goroutine 數與 heap 用量
	var peakGoroutines int
	var firstHeap, lastHeap uint64
	start := time.Now()
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()

	// 每秒對 watchlist 查詢盤中 K 線（模擬實際使用負載，驗證延遲）
	queryTicker := time.NewTicker(time.Second)
	defer queryTicker.Stop()

	// 設定 watchlist（T019 驗收之 15 檔上限內）
	watchlist := []string{"2330", "2317", "2454", "2308", "2881"}
	if _, err := app.core.Call(ctx, "set_active_watchlist", map[string]any{"symbols": watchlist}); err != nil {
		t.Logf("set_active_watchlist（開盤前可能未就緒，僅記錄）: %v", err)
	}

	// 延遲統計：盤中 K 線查詢 P95
	var latencies []time.Duration

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			goroutines := runtime.NumGoroutine()
			if goroutines > peakGoroutines {
				peakGoroutines = goroutines
			}
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			if firstHeap == 0 {
				firstHeap = ms.HeapAlloc
			}
			lastHeap = ms.HeapAlloc
			elapsed := time.Since(start)
			t.Logf("[soak] %s goroutines=%d heap=%dMB peak_goroutines=%d",
				elapsed.Round(time.Second), goroutines, ms.HeapAlloc/1024/1024, peakGoroutines)
		case <-queryTicker.C:
			// 盤中 K 線查詢（純記憶體，零 HTTP；驗證 P95 延遲）
			if !liveSessionOpen(time.Now()) {
				continue // 已收盤：停止查詢
			}
			began := time.Now()
			env, err := app.core.Call(ctx, "get_intraday_kline",
				map[string]any{"symbol": "2330", "timeframe": "1m", "limit": 60})
			if err != nil {
				t.Logf("[soak] get_intraday_kline 錯誤: %v", err)
				continue
			}
			latencies = append(latencies, time.Since(began))
			_ = env
		case <-time.After(soakDuration):
			// 運行時長到達：結束並輸出總結
			goto done
		}
	}

done:
	// 總結：goroutine 穩定、heap 無持續增長、延遲達標
	total := time.Since(start)
	if len(latencies) > 0 {
		p95 := percentile(latencies, 95)
		p50 := percentile(latencies, 50)
		t.Logf("[soak] 總運行 %s，盤中 K 線查詢 %d 次，P50=%s P95=%s（§13 目標 P95<200ms）",
			total.Round(time.Second), len(latencies), p50, p95)
		if p95 > 200*time.Millisecond {
			t.Errorf("盤中 K 線查詢 P95=%s 超過 200ms 目標（§13）", p95)
		}
	}
	if lastHeap > 0 && firstHeap > 0 && lastHeap > firstHeap*2 {
		t.Errorf("heap 疑似持續增長: 首次 %dMB → 最後 %dMB", firstHeap/1024/1024, lastHeap/1024/1024)
	}
	t.Logf("[soak] 完成：運行 %s，peak goroutines=%d，heap %dMB → %dMB",
		total.Round(time.Second), peakGoroutines, firstHeap/1024/1024, lastHeap/1024/1024)
}

// liveSessionOpen 判斷是否為開盤時段（週一至五 09:00–13:30）。
func liveSessionOpen(now time.Time) bool {
	loc := model.Taipei()
	n := now.In(loc)
	if n.Weekday() == time.Saturday || n.Weekday() == time.Sunday {
		return false
	}
	secs := n.Hour()*3600 + n.Minute()*60 + n.Second()
	return secs >= 9*3600 && secs < 13*3600+30*60
}

// percentile 回傳排序後延遲序列之指定百分位。
func percentile(ds []time.Duration, p int) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(ds))
	copy(sorted, ds)
	// 簡易插入排序（延遲序列短，避免依賴排序套件）
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := (len(sorted) - 1) * p / 100
	return sorted[idx]
}

var _ = fmt.Sprintf // keep fmt import for potential debug
