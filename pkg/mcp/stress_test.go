package mcp

// stress_test.go：T030 驗收 #5 — 壓力測試。
//
// v2.1 §13 Phase 6：20 併發 Client 對同一熱門股（2330）重複查詢
// get_stock_daily_quote，驗證 Single-flight 併流與 L1/L2 快取命中率
// （目標 ≥ 80%）。離線回放（fake 資料源，§13 錄製回放原則），
// 於 `go test ./...` 全量回歸中執行，不需連網。

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"tw-quant-mcp/pkg/model"
)

// TestConcurrentHotStockHitRate 20 併發同熱門股查詢：
//   - 快取命中率 ≥ 80%（§13 目標，T030 驗收 #5）；
//   - 上游呼叫次數遠小於查詢次數（Single-flight 併流）。
func TestConcurrentHotStockHitRate(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	app := deApp(t, f) // deApp 含快取（L1-only）與 2330 等 Symbol

	args := map[string]any{"symbol": "2330", "date": "2026-07-30"}

	// 預熱：暖 L1 以量測真實命中率（§13：與 loadtest 同法）
	for i := 0; i < 5; i++ {
		if _, err := app.core.Call(context.Background(), "get_stock_daily_quote", args); err != nil {
			t.Fatalf("預熱失敗: %v", err)
		}
	}

	const clients, perClient = 20, 10
	var hits, total atomic.Int64
	before := app.httpCalls.Load()

	var wg sync.WaitGroup
	start := make(chan struct{})
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < perClient; j++ {
				env, err := app.core.Call(context.Background(), "get_stock_daily_quote", args)
				if err != nil {
					t.Errorf("併發查詢失敗: %v", err)
					continue
				}
				e := env.(*model.Envelope)
				total.Add(1)
				if e.Lineage.IsCached {
					hits.Add(1)
				}
			}
		}()
	}
	close(start)
	wg.Wait()

	after := app.httpCalls.Load()
	hitRate := float64(hits.Load()) / float64(total.Load()) * 100
	upstream := after - before // 僅量測併發區段之上游呼叫（Single-flight 驗證）

	t.Logf("併發查詢 %d 次：命中 %d（%.1f%%），上游呼叫 %d（預熱後新增）",
		total.Load(), hits.Load(), hitRate, upstream)

	if hitRate < 80 {
		t.Errorf("快取命中率應 ≥ 80%%，實際 %.1f%%（%d/%d）", hitRate, hits.Load(), total.Load())
	}
	// 20×10=200 次查詢，資料源僅 1 組（日 K）→ Single-flight 後上游呼叫應遠小於 200
	if upstream > 20 {
		t.Errorf("Single-flight 併流後上游呼叫應 ≪ 查詢數，實際 %d", upstream)
	}
	if total.Load() != clients*perClient {
		t.Errorf("應完成 %d 次查詢，實際 %d", clients*perClient, total.Load())
	}
}
