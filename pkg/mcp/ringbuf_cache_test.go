package mcp

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
)

// TestRingBufferNotInCache v2.1 §5.1 守門：盤中 RingBuffer 即時資料
// （MIS 快照驅動之報價/內外盤/K線/VWAP/爆量偵測）完全不經過 L1/L2，
// 不寫入任何快取列（§5.2 MIS 亦為 PostNotCached / L2 禁用）。
func TestRingBufferNotInCache(t *testing.T) {
	ctx := context.Background()
	c, err := cache.New(cache.WithSQLitePath(filepath.Join(t.TempDir(), "cache.db")))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	symbols := seedSymbols()
	rings := engine.NewRingStore()
	seedSnapshots(rings)
	agg := engine.NewAggregator(rings)
	intraday := engine.NewIntradayStore()
	intraday.UpdateAll(rings.Snapshots("2330"))
	watchlist := engine.NewWatchlist(func(time.Time) bool { return true })

	app, err := NewApp(nil,
		WithAppClock(testClock),
		WithAppSymbols(symbols),
		WithAppEngine(watchlist, rings, agg, intraday),
		WithAppCache(c),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	_ = watchlist.Set([]model.Symbol{{Code: "2330", Name: "台積電", Market: model.MarketTSE}})

	// 依序觸發四個 §8 盤中引擎工具（皆由 RingBuffer 快照驅動）。
	for _, tc := range []struct {
		name string
		args map[string]any
	}{
		{"get_intraday_quote", map[string]any{"symbol": "2330"}},
		{"get_intraday_kline", map[string]any{"symbol": "2330"}},
		{"get_intraday_vwap", map[string]any{"symbol": "2330"}},
		{"detect_volume_surge", map[string]any{"symbol": "2330", "minutes": float64(5)}},
	} {
		if _, err := app.Core().Call(ctx, tc.name, tc.args); err != nil {
			t.Fatalf("%s 失敗: %v", tc.name, err)
		}
	}

	// L2 不得有任何 MIS 列（§5.2 AllowL2=false + PostNotCached）。
	if n, err := c.L2Count(ctx, cache.DatasetMISSnapshot); err != nil || n != 0 {
		t.Errorf("RingBuffer 資料不得寫入 L2（MIS 列應為 0），實際 n=%d err=%v", n, err)
	}
	// L1 亦不得有快照鍵（探測鍵 miss 即無寫入）。
	if v, ok, err := cache.Get[string](ctx, c, "ringbuf:2330",
		cache.WithDataset(cache.DatasetMISSnapshot, "2026-07-31")); err != nil || ok {
		t.Errorf("RingBuffer 資料不得寫入 L1（應 miss），實際 ok=%v v=%q err=%v", ok, v, err)
	}
}
