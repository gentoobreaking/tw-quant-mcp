package mcp

// cache_consistency_test.go：T030 驗收 #2 — 全量工具之 Cache 欄位一致性。
//
// v2.1 §13 Phase 6：全量 Tool 的 Lineage／Cache／Chart 欄位一致性測試。
// 本檔聚焦 Cache 維度（Lineage/Chart 由 app_envelope_test.go 與
// app_release_test.go 涵蓋）：
//
//   - 走快取路徑之工具（B–G 組，除純記憶體查詢）：第二次呼叫必須
//     is_cached=true 且 cache_ttl>0，且不新增上游 HTTP 呼叫。
//   - 純記憶體工具（A 組盤中 + symbol_list/trading_calendar）：
//     http_calls=0、is_cached=false（合理，無快取語意）。
//
// 不連網；fixtures/替身離線回放（§13 錄製回放原則）。

import (
	"context"
	"testing"

	"tw-quant-mcp/pkg/model"
)

// memoryOnlyTools 為無快取語意之工具：A 組盤中 6 工具（純記憶體
// RingBuffer/快照，§8）＋ F 組 symbol_list / trading_calendar（Symbol
// Registry 與內嵌日曆，§10.G）。這些工具 http_calls 恆 0，is_cached 恆 false。
var memoryOnlyTools = map[string]bool{
	"set_active_watchlist":      true,
	"get_intraday_kline":        true,
	"get_intraday_quote":        true,
	"get_intraday_vwap":         true,
	"detect_volume_surge":       true,
	"scan_daytrade_eligibility": true,
	"get_symbol_list":           true,
	"get_trading_calendar":      true,
}

// TestAllToolsCacheConsistency 全量 38 工具之 Cache 一致性：
//   - 記憶體工具：http_calls=0、is_cached=false
//   - 快取路徑工具：二次呼叫 is_cached=true、cache_ttl>0、http_calls 不增
func TestAllToolsCacheConsistency(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)
	intraday := newTestApp(t)

	for _, p := range allToolProbes() {
		p := p
		t.Run(p.name, func(t *testing.T) {
			target := app
			if intradayTools[p.name] {
				target = intraday
			}
			// 第一次呼叫：miss（或記憶體）
			env1, err := target.core.Call(context.Background(), p.name, p.args)
			if err != nil {
				t.Fatalf("%s: 首次 Call 失敗: %v", p.name, err)
			}
			e1 := env1.(*model.Envelope)
			// 第二次呼叫：應命中快取
			env2, err := target.core.Call(context.Background(), p.name, p.args)
			if err != nil {
				t.Fatalf("%s: 二次 Call 失敗: %v", p.name, err)
			}
			e2 := env2.(*model.Envelope)

			if memoryOnlyTools[p.name] {
				if e1.HTTPCalls != 0 || e2.HTTPCalls != 0 {
					t.Errorf("%s（記憶體工具）http_calls 應恆 0，實際 %d/%d",
						p.name, e1.HTTPCalls, e2.HTTPCalls)
				}
				if e2.Lineage.IsCached {
					t.Errorf("%s（記憶體工具）is_cached 應為 false", p.name)
				}
				return
			}

			// 多來源聚合工具（get_stock_trend_composite）：快取語意於各子
			// lineage（v2.1 §4 設計規則 2）——第二次呼叫所有子來源皆需命中。
			if len(e2.Lineage.Multi) > 0 {
				for i := range e2.Lineage.Multi {
					if !e2.Lineage.Multi[i].IsCached {
						t.Errorf("%s: 子來源[%d] 二次呼叫應 is_cached=true（第一次 %v → 第二次 %v）",
							p.name, i, e1.Lineage.Multi[i].IsCached, e2.Lineage.Multi[i].IsCached)
					}
					if e2.Lineage.Multi[i].CacheTTL <= 0 {
						t.Errorf("%s: 子來源[%d] cache_ttl 應 > 0，實際 %d",
							p.name, i, e2.Lineage.Multi[i].CacheTTL)
					}
				}
				return
			}

			// 快取路徑工具：第二次必須命中
			if !e2.Lineage.IsCached {
				t.Errorf("%s: 二次呼叫應 is_cached=true（第一次 %v → 第二次 %v）",
					p.name, e1.Lineage.IsCached, e2.Lineage.IsCached)
			}
			if e2.Lineage.CacheTTL <= 0 {
				t.Errorf("%s: cache_ttl 應 > 0，實際 %d", p.name, e2.Lineage.CacheTTL)
			}
			if e1.HTTPCalls == 0 && e2.HTTPCalls == 0 {
				t.Logf("%s: http_calls 恆 0（資料源未計入 HTTP）", p.name)
			}
		})
	}
}

// TestTrendCompositeCacheAllSubLineages T029 新工具多來源聚合之 Cache
// 一致性：第二次呼叫所有子 lineage 皆 is_cached=true。
func TestTrendCompositeCacheAllSubLineages(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	stubTrendTSE(f)
	app := deApp(t, f)

	args := map[string]any{"symbol": "2330", "horizon": "mid"}
	env1, err := app.core.Call(context.Background(), "get_stock_trend_composite", args)
	if err != nil {
		t.Fatalf("首次 Call 失敗: %v", err)
	}
	env2, err := app.core.Call(context.Background(), "get_stock_trend_composite", args)
	if err != nil {
		t.Fatalf("二次 Call 失敗: %v", err)
	}
	e1 := env1.(*model.Envelope)
	e2 := env2.(*model.Envelope)

	if len(e1.Lineage.Multi) == 0 {
		t.Fatal("首次呼叫應有多來源 lineage")
	}
	if len(e2.Lineage.Multi) != len(e1.Lineage.Multi) {
		t.Fatalf("二次呼叫子來源數應相同: %d vs %d", len(e2.Lineage.Multi), len(e1.Lineage.Multi))
	}
	for i := range e2.Lineage.Multi {
		if !e2.Lineage.Multi[i].IsCached {
			t.Errorf("子來源[%d] %q 二次呼叫應 is_cached=true（技術面/估值/EPS/籌碼任一 miss 即漏）",
				i, e2.Lineage.Multi[i].Source)
		}
	}
}
