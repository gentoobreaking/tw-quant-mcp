package mcp

// index_build_test.go：§10.3 Materialized Screener Index 之排程與查詢測試。
//
//  1. 排程：交易日 15:00 重建一次、同日不重複、非交易日跳過（§10.3）；
//  2. 建置：rebuildScreenerIndex 以整批路徑（§12.4）寫入索引快照
//     （估值/股利/連年配息/OverallScore），freshness=建立時間；
//  3. 查詢：索引就緒時 screen_high_yield 直接 SELECT，零上游 HTTP，
//     lineage.freshness 標註索引建立時間（§10.3）。

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/config"
	"tw-quant-mcp/pkg/domain/screener"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
	"tw-quant-mcp/pkg/registry"
)

// newMCPIndexStore 建立 mcp 測試用之 §10.3 索引 store（temp dir）。
func newMCPIndexStore(t *testing.T) *screener.Store {
	t.Helper()
	s, err := screener.NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("screener.NewStore 失敗: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// indexSchedApp 建立含 §10.3 索引（store + 注入式 builder）之 App，
// 供排程測試（不觸發真實建置）。其餘組件與 prewarmApp 相同。
func indexSchedApp(t *testing.T, f *fakeFetch, now func() time.Time,
	store *screener.Store, builder func(context.Context) error) *App {
	t.Helper()
	scheduleServer(t)
	listServers(t)
	_, _ = misServer(t)
	cch, err := cache.New(cache.WithDataDir(t.TempDir()))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	app, err := NewApp(nil,
		WithAppClock(now),
		WithAppCache(cch),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
		WithAppCalendarClient(provider.NewBaseClient("www.twse.com.tw", provider.WithRateInterval(time.Millisecond))),
		WithAppRegistryLoader(registry.NewLoader(
			provider.NewBaseClient("openapi.twse.com.tw", provider.WithRateInterval(time.Millisecond)), cch)),
		WithAppMISClient(provider.NewBaseClient("mis.twse.com.tw", provider.WithRateInterval(time.Millisecond))),
		WithAppIndex(store),
		WithAppIndexBuilder(builder),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// indexDeApp 建立含真實建置路徑之 App（DataDir → 自動開啟 index.db；
// 注入 DE 整批 stub，供 rebuildScreenerIndex 測試）。
func indexDeApp(t *testing.T, f *fakeFetch, now func() time.Time) *App {
	t.Helper()
	dir := t.TempDir()
	cch, err := cache.New(cache.WithDataDir(dir))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	symbols := model.NewRegistry()
	_ = symbols.Set([]model.Symbol{
		{Code: "2330", Name: "台積電", Market: model.MarketTSE},
		{Code: "2317", Name: "鴻海", Market: model.MarketTSE},
		{Code: "1101", Name: "台泥", Market: model.MarketTSE},
		{Code: "6147", Name: "頎邦", Market: model.MarketOTC},
		{Code: "6547", Name: "高端疫苗", Market: model.MarketOTC},
	})
	app, err := NewApp(&config.Config{DataDir: dir},
		WithAppClock(now),
		WithAppSymbols(symbols),
		WithAppCache(cch),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
		WithAppMOPS(fakeMOPS{f}),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	if app.index == nil {
		t.Fatal("DataDir 設定後應自動開啟 Screener Index")
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// TestPrewarmIndexBuildsAt1500：交易日 15:00 重建一次，同日不重複。
func TestPrewarmIndexBuildsAt1500(t *testing.T) {
	f := newFake(t)
	stubEOD(f) // 17:00 tick 觸發 EOD 預熱，需整批 stub
	store := newMCPIndexStore(t)
	var calls int
	// 15:30 tick：morning 與 index 皆在同一次 tick 完成（morning 先執行）。
	// listServers() 已於 prewarmApp 內部呼叫（indexSchedApp → prewarmApp），
	// 代碼表 stub 已在測試環境全域設定；morning 失敗僅警告，不阻斷 index。
	app := indexSchedApp(t, f, func() time.Time { return prewarmAt(15, 30) },
		store, func(context.Context) error { calls++; return nil })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), prewarmAt(8, 30))
	if calls != 0 {
		t.Errorf("08:30 不應重建索引，實際 calls=%d", calls)
	}
	s.TickOnce(context.Background(), prewarmAt(15, 1))
	if calls != 1 {
		t.Errorf("15:00 後應重建索引一次，實際 calls=%d", calls)
	}
	// 同日再 tick：不重複
	s.TickOnce(context.Background(), prewarmAt(17, 0))
	if calls != 1 {
		t.Errorf("同日不得重複重建，實際 calls=%d", calls)
	}
}

// TestPrewarmIndexSkipsNonTradingDay：非交易日（週六）15:00 不重建。
func TestPrewarmIndexSkipsNonTradingDay(t *testing.T) {
	f := newFake(t)
	store := newMCPIndexStore(t)
	var calls int
	sat := time.Date(2026, 8, 1, 15, 30, 0, 0, model.Taipei()) // 週六
	app := indexSchedApp(t, f, func() time.Time { return sat },
		store, func(context.Context) error { calls++; return nil })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), sat)
	if calls != 0 {
		t.Errorf("非交易日不得重建索引，實際 calls=%d", calls)
	}
	// 基礎設施預熱仍執行（對照組）
	if app.symbols.Len() == 0 {
		t.Error("非交易日仍應載入基礎設施（公司代碼表）")
	}
}

// TestRebuildScreenerIndexSnapshot：rebuildScreenerIndex 以整批路徑
// 寫入估值/股利/連年配息/OverallScore 快照；freshness=建立時間。
func TestRebuildScreenerIndexSnapshot(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	now := time.Date(2026, 7, 31, 15, 0, 0, 0, model.Taipei()) // 週五 15:00
	app := indexDeApp(t, f, func() time.Time { return now })

	if err := app.rebuildScreenerIndex(context.Background()); err != nil {
		t.Fatalf("rebuildScreenerIndex 失敗: %v", err)
	}
	date := now.Format("2006-01-02")

	// 索引已建立，freshness=15:00
	builtAt, ok, err := app.index.BuiltAt(context.Background(), date)
	if err != nil || !ok {
		t.Fatalf("今日索引應已建立（ok=%v err=%v）", ok, err)
	}
	if !builtAt.Equal(now) {
		t.Errorf("freshness 應為建立時間 %v，實際 %v", now, builtAt)
	}

	// 全市場候選數 = stub 內全部估值標的（3 上市 + 2 上櫃）
	total, err := app.index.Count(context.Background(), date, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("索引候選數應為 5，實際 %d", total)
	}
	// 市場過濾
	tseN, _ := app.index.Count(context.Background(), date, "tse")
	otcN, _ := app.index.Count(context.Background(), date, "otc")
	if tseN != 3 || otcN != 2 {
		t.Errorf("市場過濾錯誤：tse=%d otc=%d", tseN, otcN)
	}

	// 單檔快照內容：2330（殖利率 2.1%、每股股利 7.0、連年配息 2）
	hit, err := app.index.Query(context.Background(), screener.IndexQuery{
		Date: date, Market: "tse", MinYield: 2, Rows: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var ts row2330
	found := false
	for _, r := range hit.Rows {
		if r.Symbol == "2330" {
			ts = row2330{r}
			found = true
		}
	}
	if !found {
		t.Fatal("索引應含 2330")
	}
	if ts.DividendYieldPct != 2.1 || ts.CashDividend != 7.0 || ts.ConsecutiveYears != 2 {
		t.Errorf("2330 快照錯誤: yield=%.2f cash=%.2f years=%d",
			ts.DividendYieldPct, ts.CashDividend, ts.ConsecutiveYears)
	}
	if !ts.PEAvailable || ts.PE != 20 {
		t.Errorf("2330 PE 快照錯誤: pe=%.1f available=%v", ts.PE, ts.PEAvailable)
	}

	// 上櫃標的：6147（殖利率 4.0、market=otc）
	otcHit, err := app.index.Query(context.Background(), screener.IndexQuery{
		Date: date, Market: "otc", Rows: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var otc6147 screener.IndexRow
	for _, r := range otcHit.Rows {
		if r.Symbol == "6147" {
			otc6147 = r
		}
	}
	if otc6147.Symbol != "6147" || otc6147.DividendYieldPct != 4.0 {
		t.Errorf("6147 快照錯誤: %+v", otc6147)
	}
}

// TestPrewarmIndexAndEODCoexist：15:00 索引排程與 16:45 盤後預熱併存。
// 同一交易日先重建 Materialized Index（成功寫入 L2 索引快照），
// 再執行盤後彙總預熱；兩者各自完成、互不覆蓋、不重複觸發，
// 併存後查詢零上游 HTTP（§13 驗收：併存不衝突）。
func TestPrewarmIndexAndEODCoexist(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	stubEOD(f)
	// 週五交易日：排程器時鐘與注入時鐘皆為 2026-07-31 15:00
	now := time.Date(2026, 7, 31, 15, 0, 0, 0, model.Taipei())
	app := indexDeApp(t, f, func() time.Time { return now })
	s := NewPrewarmScheduler(app)
	ctx := context.Background()

	// 15:00 後：索引重建一次並成功寫入
	s.TickOnce(ctx, time.Date(2026, 7, 31, 15, 1, 0, 0, model.Taipei()))
	_, ok, err := app.index.BuiltAt(ctx, "2026-07-31")
	if err != nil || !ok {
		t.Fatalf("15:00 索引應已建立並寫入（ok=%v err=%v）", ok, err)
	}
	if !s.indexDone {
		t.Error("indexDone 應為 true（15:00 索引已執行）")
	}
	if s.eodDone {
		t.Error("15:00 時點不應觸發 16:45 盤後預熱")
	}

	// 16:45：盤後預熱併存執行；索引不重複重建
	s.TickOnce(ctx, time.Date(2026, 7, 31, 16, 45, 0, 0, model.Taipei()))
	if !s.eodDone {
		t.Error("eodDone 應為 true（16:45 盤後已執行）")
	}
	// 同日 17:00 再 tick：兩者皆不重複
	s.TickOnce(ctx, time.Date(2026, 7, 31, 17, 0, 0, 0, model.Taipei()))

	// 併存後查詢零上游：盤後彙總（market_summary）
	env := callEnv(t, app, "get_market_summary", map[string]any{})
	if env.HTTPCalls != 0 {
		t.Errorf("盤後預熱後 market_summary http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	// 併存後查詢零上游：materialized index（screen_high_yield）
	env = callEnv(t, app, "screen_high_yield", map[string]any{"min_yield": 3})
	if env.HTTPCalls != 0 {
		t.Errorf("索引就緒後 screen_high_yield http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	if res, ok := env.Data.(model.ScreenResult); ok {
		if len(res.Rows) != 3 {
			t.Errorf("索引查詢應命中 3 檔（6147/2317/1101），實際 %d", len(res.Rows))
		}
	}
}

type row2330 struct{ screener.IndexRow }

// TestScreenHighYieldServesFromIndex：索引就緒時 screen_high_yield
// 直接 SELECT（http_calls=0），lineage.freshness=索引建立時間，
// DerivedFrom=screener_index。
func TestScreenHighYieldServesFromIndex(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	now := time.Date(2026, 7, 31, 15, 0, 0, 0, model.Taipei())
	app := indexDeApp(t, f, func() time.Time { return now })
	if err := app.rebuildScreenerIndex(context.Background()); err != nil {
		t.Fatalf("rebuildScreenerIndex 失敗: %v", err)
	}

	env := callEnv(t, app, "screen_high_yield", map[string]any{"min_yield": 3})
	if env.HTTPCalls != 0 {
		t.Errorf("索引就緒後查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	res, ok := env.Data.(model.ScreenResult)
	if !ok {
		t.Fatalf("Data 應為 ScreenResult，實際 %T", env.Data)
	}
	// 依殖利率遞減：6147(4.0) > 2317(3.5) > 1101(3.29)
	if len(res.Rows) != 3 {
		t.Fatalf("min_yield>=3 應命中 3 檔，實際 %d", len(res.Rows))
	}
	if res.Rows[0].Code != "6147" || res.Rows[0].DividendYield != 4.0 {
		t.Errorf("首名應為 6147（殖利率 4.0），實際 %+v", res.Rows[0])
	}
	// lineage：freshness=索引建立時間（15:00）、derived_from 標註
	lg := env.Lineage
	if lg.FetchedAt.IsZero() {
		t.Fatal("lineage 應標註 FetchedAt（索引建立時間）")
	}
	if !lg.FetchedAt.Time.Equal(now) {
		t.Errorf("FetchedAt 應為索引建立時間 %v，實際 %v", now, lg.FetchedAt.Time)
	}
	found := false
	for _, d := range lg.DerivedFrom {
		if d == "screener_index" {
			found = true
		}
	}
	if !found {
		t.Errorf("DerivedFrom 應含 screener_index，實際 %v", lg.DerivedFrom)
	}
}
