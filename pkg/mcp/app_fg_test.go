package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// app_fg_test.go：§10.F/G 工具之整合測試（T015）。
// 多數測試以 fakeTAIFEX 替身（驗證契約白名單、lineage、chart meta、邊界）；
// 另以 httptest 真實 TAIFEXQuery 驗證 §9.3 路徑：最新交易日→API、
// 其餘→DL、L2 命中不重複下載。

// fakeTAIFEX 為 TAIFEXQuerier 之測試替身：
// single 依 "ds|date|contract" 鍵；ranges 依 "ds|start|end|contract" 鍵。
type fakeTAIFEX struct {
	t        *testing.T
	latest   string
	single   map[string]provider.TAIFEXQueryResult
	ranges   map[string]map[string]provider.TAIFEXQueryResult
	cached   map[string]bool // single 鍵 → 模擬快取命中
	latestFn func() string
}

func newFakeTAIFEX(t *testing.T, latest string) *fakeTAIFEX {
	return &fakeTAIFEX{t: t, latest: latest, single: map[string]provider.TAIFEXQueryResult{}, ranges: map[string]map[string]provider.TAIFEXQueryResult{}, cached: map[string]bool{}}
}

func tfKey(ds model.TAIFEXDataset, date, contract string) string {
	return string(ds) + "|" + date + "|" + contract
}

func tfRangeKey(ds model.TAIFEXDataset, start, end, contract string) string {
	return string(ds) + "|" + start + "|" + end + "|" + contract
}

// tfStub 以 model 值建立 TAIFEXQueryResult（API 來源）。
func tfStub(v any) provider.TAIFEXQueryResult {
	b, _ := json.Marshal(v)
	return provider.TAIFEXQueryResult{Data: b, Source: model.SourceTAIFEXAPI}
}

// tfStubDL 以 model 值建立 TAIFEXQueryResult（DL 來源，可含補檔標記）。
func tfStubDL(v any, derived string) provider.TAIFEXQueryResult {
	b, _ := json.Marshal(v)
	return provider.TAIFEXQueryResult{Data: b, Source: model.SourceTAIFEXDL, DerivedFrom: derived}
}

func (f *fakeTAIFEX) Fetch(_ context.Context, ds model.TAIFEXDataset, date, contract string) (provider.TAIFEXQueryResult, bool, error) {
	key := tfKey(ds, date, contract)
	res, ok := f.single[key]
	if !ok {
		f.t.Fatalf("fakeTAIFEX: 未 stub 之查詢鍵 %q", key)
	}
	return res, f.cached[key], nil
}

func (f *fakeTAIFEX) FetchRange(_ context.Context, ds model.TAIFEXDataset, start, end, contract string) (map[string]provider.TAIFEXQueryResult, error) {
	key := tfRangeKey(ds, start, end, contract)
	byDay, ok := f.ranges[key]
	if !ok {
		f.t.Fatalf("fakeTAIFEX: 未 stub 之範圍鍵 %q", key)
	}
	return byDay, nil
}

func (f *fakeTAIFEX) LatestTradingDay(_ context.Context) (string, error) {
	if f.latestFn != nil {
		return f.latestFn(), nil
	}
	return f.latest, nil
}

// stubFG 建立 F/G 工具測試之常用 stub。
func stubFG(f *fakeTAIFEX) {
	// 期貨每日行情（TX，最新日 2026-07-29）
	f.single[tfKey(model.TAFuturesDaily, "2026-07-29", "TX")] = tfStub([]model.FuturesDailyRow{
		{Date: "2026-07-29", Contract: "TX", ContractMonth: "202608", Session: "一般", Open: 41915, High: 42070, Low: 39442, Close: 40392, Change: -1181, ChangePct: -2.84, Volume: 124405, Settlement: 40328, OpenInterest: 108507},
		{Date: "2026-07-29", Contract: "TX", ContractMonth: "202608", Session: "盤後", Open: 41779, High: 42108, Low: 41178, Close: 41889, Change: 316, ChangePct: 0.76, Volume: 54682},
	})
	// 歷史（DL）：TX 2026-07-27 / 07-28
	f.ranges[tfRangeKey(model.TAFuturesDaily, "2026-07-27", "2026-07-29", "TX")] = map[string]provider.TAIFEXQueryResult{
		"2026-07-27": tfStubDL([]model.FuturesDailyRow{
			{Date: "2026-07-27", Contract: "TX", ContractMonth: "202608", Session: "一般", Open: 41000, High: 41200, Low: 40800, Close: 41100, Volume: 100000},
		}, ""),
		"2026-07-28": tfStubDL([]model.FuturesDailyRow{
			{Date: "2026-07-28", Contract: "TX", ContractMonth: "202608", Session: "一般", Open: 41100, High: 41500, Low: 40900, Close: 41400, Volume: 110000},
		}, ""),
		// 07-29 缺口 → 補檔自 07-28
		"2026-07-29": tfStubDL(nil, "2026-07-28"),
	}
	// 買賣權比：單日 + 範圍
	f.single[tfKey(model.TAPutCallRatio, "2026-07-29", "")] = tfStub([]model.PCRow{
		{Date: "2026-07-29", CallVolume: 100000, PutVolume: 120500, VolumeRatio: 120.5, CallOI: 200000, PutOI: 210000, OIRatio: 105.0},
	})
	f.ranges[tfRangeKey(model.TAPutCallRatio, "2026-07-27", "2026-07-29", "")] = map[string]provider.TAIFEXQueryResult{
		"2026-07-27": tfStubDL([]model.PCRow{{Date: "2026-07-27", CallVolume: 90000, PutVolume: 99000, VolumeRatio: 110.0}}, ""),
		"2026-07-28": tfStubDL([]model.PCRow{{Date: "2026-07-28", CallVolume: 95000, PutVolume: 114000, VolumeRatio: 120.0}}, ""),
		"2026-07-29": tfStubDL([]model.PCRow{{Date: "2026-07-29", CallVolume: 100000, PutVolume: 120500, VolumeRatio: 120.5}}, ""),
	}
	// 三大法人期貨/選擇權（最新日）
	f.single[tfKey(model.TAInstiFutures, "2026-07-29", "")] = tfStub([]model.InstitutionalRow{
		{Date: "2026-07-29", Contract: "臺股期貨", Investor: "自營商", LongVolume: 12186, NetVolume: 5000, OINet: 10000},
		{Date: "2026-07-29", Contract: "臺股期貨", Investor: "投信", LongVolume: 2000, NetVolume: -1000, OINet: 2000},
		{Date: "2026-07-29", Contract: "臺股期貨", Investor: "外資及陸資", LongVolume: 50000, NetVolume: 12000, OINet: 60000},
	})
	f.single[tfKey(model.TAInstiOptions, "2026-07-29", "")] = tfStub([]model.InstitutionalRow{
		{Date: "2026-07-29", Contract: "臺指選擇權", Investor: "自營商", LongVolume: 30000, NetVolume: 8000, OINet: 20000},
		{Date: "2026-07-29", Contract: "臺指選擇權", Investor: "外資及陸資", LongVolume: 40000, NetVolume: -3000, OINet: 50000},
	})
	// 三大法人期貨歷史
	f.ranges[tfRangeKey(model.TAInstiFutures, "2026-07-27", "2026-07-29", "")] = map[string]provider.TAIFEXQueryResult{
		"2026-07-27": tfStubDL([]model.InstitutionalRow{{Date: "2026-07-27", Contract: "臺股期貨", Investor: "外資及陸資", NetVolume: 8000}}, ""),
		"2026-07-28": tfStubDL([]model.InstitutionalRow{{Date: "2026-07-28", Contract: "臺股期貨", Investor: "外資及陸資", NetVolume: 9000}}, ""),
		"2026-07-29": tfStubDL([]model.InstitutionalRow{{Date: "2026-07-29", Contract: "臺股期貨", Investor: "外資及陸資", NetVolume: 12000}}, ""),
	}
	// 大額交易人（期貨+選擇權）
	f.single[tfKey(model.TALargeTraderFut, "2026-07-29", "")] = tfStub([]model.LargeTraderRow{
		{Date: "2026-07-29", Contract: "TX", ContractName: "臺股期貨", ContractMonth: "202608", TraderType: "1", Top5Long: 30000, Top5Short: 25000, Top10Long: 45000, Top10Short: 40000, MarketOI: 100000},
	})
	f.single[tfKey(model.TALargeTraderOpt, "2026-07-29", "")] = tfStub([]model.LargeTraderRow{
		{Date: "2026-07-29", Contract: "TXO", ContractName: "臺指選擇權", ContractMonth: "202608", CallPut: "買權", TraderType: "1", Top5Long: 50000, Top5Short: 40000, Top10Long: 70000, Top10Short: 65000, MarketOI: 300000},
	})
	// 大額交易人範圍
	f.ranges[tfRangeKey(model.TALargeTraderFut, "2026-07-28", "2026-07-29", "")] = map[string]provider.TAIFEXQueryResult{
		"2026-07-28": tfStubDL([]model.LargeTraderRow{{Date: "2026-07-28", Contract: "TX", TraderType: "1", Top5Long: 29000, MarketOI: 99000}}, ""),
		"2026-07-29": tfStubDL([]model.LargeTraderRow{{Date: "2026-07-29", Contract: "TX", TraderType: "1", Top5Long: 30000, MarketOI: 100000}}, ""),
	}
	f.ranges[tfRangeKey(model.TALargeTraderOpt, "2026-07-28", "2026-07-29", "")] = map[string]provider.TAIFEXQueryResult{
		"2026-07-28": tfStubDL([]model.LargeTraderRow{{Date: "2026-07-28", Contract: "TXO", TraderType: "1", Top5Long: 48000, MarketOI: 290000}}, ""),
		"2026-07-29": tfStubDL([]model.LargeTraderRow{{Date: "2026-07-29", Contract: "TXO", TraderType: "1", Top5Long: 50000, MarketOI: 300000}}, ""),
	}
}

// fgApp 建立注入 fake fetcher 與 fake TAIFEX 之 App。
func fgApp(t *testing.T, f *fakeFetch, tq TAIFEXQuerier) *App {
	t.Helper()
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, model.Taipei())
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(seedSymbols()),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
		WithAppMOPS(fakeMOPS{f}),
		WithAppTAIFEX(tq),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// ************** F. 期貨與選擇權 **************

func TestFGFuturesDailyOHLCLatest(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	// date 省略 → 最新交易日（API 路徑）
	env := callEnv(t, app, "get_futures_daily_ohlc", map[string]any{"contract": "TX"})
	rows, ok := env.Data.([]model.FuturesDailyRow)
	if !ok {
		t.Fatalf("Data 應為 []FuturesDailyRow，實際 %T", env.Data)
	}
	if len(rows) != 2 || rows[0].Contract != "TX" || rows[0].Close != 40392 {
		t.Errorf("期貨行情錯誤: %+v", rows)
	}
	if env.Lineage.Source != model.SourceTAIFEXAPI || env.Lineage.Freshness != model.FreshnessPostMarketToday {
		t.Errorf("lineage 應為 API/POST_MARKET_TODAY: %+v", env.Lineage)
	}
	if env.Lineage.DataDate != "2026-07-29" {
		t.Errorf("data_date 應為 2026-07-29，實際 %s", env.Lineage.DataDate)
	}
	if chartType(env) != "candlestick" {
		t.Errorf("chart 應為 candlestick，實際 %s", chartType(env))
	}
}

func TestFGFuturesDailyOHLCSpecificDate(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	// 指定非最新日 → 未走 API（替身直接回 DL 結果），lineage 標 HISTORICAL
	env := callEnv(t, app, "get_futures_daily_ohlc", map[string]any{"contract": "TX", "date": "2026-07-29"})
	rows := env.Data.([]model.FuturesDailyRow)
	if len(rows) != 2 {
		t.Fatalf("應有 2 列，實際 %d", len(rows))
	}
}

// 邊界：契約代碼白名單外 → 錯誤
func TestFGFuturesDailyOHLCBadContract(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	// 白名單外（且含注入嘗試字元）→ 拒絕
	for _, c := range []string{"TX;DROP", "CDF", "tx"} {
		if _, err := app.core.Call(context.Background(), "get_futures_daily_ohlc",
			map[string]any{"contract": c}); err == nil {
			t.Errorf("契約 %q 應被白名單拒絕", c)
		}
	}
}

// 邊界：無資料日期（缺口）→ 明確錯誤
func TestFGFuturesDailyOHLCGap(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	// 未 stub 之鍵 → 替身視為缺口（Note 結果）
	tq.single[tfKey(model.TAFuturesDaily, "2026-07-28", "TX")] = provider.TAIFEXQueryResult{Note: "官方無 2026-07-28 之 futures_daily 資料（可能為非交易日或缺口）"}
	app := fgApp(t, f, tq)

	if _, err := app.core.Call(context.Background(), "get_futures_daily_ohlc",
		map[string]any{"contract": "TX", "date": "2026-07-28"}); err == nil {
		t.Fatal("缺口日應回明確錯誤")
	}
}

func TestFGFuturesHistory(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_futures_history",
		map[string]any{"contract": "TX", "start": "2026-07-27", "end": "2026-07-29"})
	rows, ok := env.Data.([]model.FuturesDailyRow)
	if !ok {
		t.Fatalf("Data 應為 []FuturesDailyRow，實際 %T", env.Data)
	}
	// 07-27、07-28 有資料；07-29 缺口被補檔註記（不產生資料列）
	if len(rows) != 2 || rows[0].Date != "2026-07-27" || rows[1].Date != "2026-07-28" {
		t.Errorf("歷史行情排序錯誤: %+v", rows)
	}
	if env.Lineage.Source != model.SourceTAIFEXDL || env.Lineage.Freshness != model.FreshnessHistorical {
		t.Errorf("lineage 應為 DL/HISTORICAL: %+v", env.Lineage)
	}
	if len(env.Lineage.DerivedFrom) == 0 {
		t.Error("範圍內含補檔日，derived_from 應標記")
	}
	if chartType(env) != "candlestick" {
		t.Errorf("chart 應為 candlestick，實際 %s", chartType(env))
	}
}

// 邊界：範圍跨年/跨度超限
func TestFGFuturesHistoryRangeTooLong(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	if _, err := app.core.Call(context.Background(), "get_futures_history",
		map[string]any{"contract": "TX", "start": "2025-01-01", "end": "2026-07-29"}); err == nil {
		t.Fatal("跨度 >366 日應被拒絕")
	}
	// start 晚於 end
	if _, err := app.core.Call(context.Background(), "get_futures_history",
		map[string]any{"contract": "TX", "start": "2026-07-29", "end": "2026-07-27"}); err == nil {
		t.Fatal("start 晚於 end 應被拒絕")
	}
}

func TestFGPutCallRatioSingleDate(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_put_call_ratio", map[string]any{"date": "2026-07-29"})
	rows := env.Data.([]model.PCRow)
	if len(rows) != 1 || rows[0].VolumeRatio != 120.5 {
		t.Errorf("PCR 錯誤: %+v", rows)
	}
	if env.Lineage.Freshness != model.FreshnessPostMarketToday {
		t.Errorf("API 單日 freshness 應為 POST_MARKET_TODAY: %+v", env.Lineage)
	}
	if chartType(env) != "line" {
		t.Fatalf("PCR chart 應為 line，實際 %s", chartType(env))
	}
	anns := env.ChartMeta.Annotations
	if len(anns) != 1 {
		t.Fatalf("應有 1 個 annotation（多空分界線），實際 %v", anns)
	}
	if anns[0].Type != "hline" || anns[0].Value != float64(1) {
		t.Errorf("annotation 應為 hline 1.0，實際 %v", anns[0])
	}
}

func TestFGPutCallRatioRange(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_put_call_ratio",
		map[string]any{"start": "2026-07-27", "end": "2026-07-29"})
	rows := env.Data.([]model.PCRow)
	if len(rows) != 3 || rows[0].Date != "2026-07-27" || rows[2].Date != "2026-07-29" {
		t.Errorf("PCR 範圍排序錯誤: %+v", rows)
	}
	if env.Lineage.Freshness != model.FreshnessHistorical {
		t.Errorf("範圍 freshness 應為 HISTORICAL: %+v", env.Lineage)
	}
}

func TestFGInstitutionalPositions(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_institutional_futures_positions", map[string]any{"date": "2026-07-29"})
	rows := env.Data.([]model.InstitutionalRow)
	if len(rows) != 3 {
		t.Fatalf("期貨法人部位應 3 列，實際 %d", len(rows))
	}
	if rows[2].Investor != "外資及陸資" || rows[2].NetVolume != 12000 {
		t.Errorf("外資部位錯誤: %+v", rows[2])
	}
	if env.Lineage.Freshness != model.FreshnessPostMarketToday {
		t.Errorf("freshness 應為 POST_MARKET_TODAY: %+v", env.Lineage)
	}
	if chartType(env) != "bar" {
		t.Errorf("chart 應為 bar，實際 %s", chartType(env))
	}

	env2 := callEnv(t, app, "get_institutional_options_positions", map[string]any{"date": "2026-07-29"})
	opt := env2.Data.([]model.InstitutionalRow)
	if len(opt) != 2 || opt[0].Contract != "臺指選擇權" {
		t.Errorf("選擇權法人部位錯誤: %+v", opt)
	}
}

func TestFGInstitutionalFuturesHistory(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_institutional_futures_history",
		map[string]any{"start": "2026-07-27", "end": "2026-07-29"})
	rows := env.Data.([]model.InstitutionalRow)
	if len(rows) != 3 || rows[0].Date != "2026-07-27" || rows[2].Date != "2026-07-29" {
		t.Errorf("法人歷史排序錯誤: %+v", rows)
	}
	if env.Lineage.Source != model.SourceTAIFEXDL {
		t.Errorf("歷史 lineage 應為 TAIFEX_DL: %+v", env.Lineage)
	}
}

func TestFGLargeTraderPositions(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_large_trader_positions", map[string]any{"date": "2026-07-29"})
	lt, ok := env.Data.(model.LargeTraderPositions)
	if !ok {
		t.Fatalf("Data 應為 LargeTraderPositions，實際 %T", env.Data)
	}
	if lt.Date != "2026-07-29" || len(lt.Futures) != 1 || len(lt.Options) != 1 {
		t.Errorf("單日大額交易人錯誤: %+v", lt)
	}
	if lt.Futures[0].Top5Long != 30000 || lt.Options[0].CallPut != "買權" {
		t.Errorf("大額交易人內容錯誤: %+v", lt)
	}

	// 範圍模式
	env = callEnv(t, app, "get_large_trader_positions",
		map[string]any{"start": "2026-07-28", "end": "2026-07-29"})
	lt = env.Data.(model.LargeTraderPositions)
	if lt.RangeStart != "2026-07-28" || lt.RangeEnd != "2026-07-29" ||
		len(lt.Futures) != 2 || len(lt.Options) != 2 {
		t.Errorf("範圍大額交易人錯誤: %+v", lt)
	}
}

// ************** G. 基礎設施 **************

func TestFGGetSymbolList(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	app := fgApp(t, f, tq)

	env := callEnv(t, app, "get_symbol_list", nil)
	symbols, ok := env.Data.([]model.Symbol)
	if !ok {
		t.Fatalf("Data 應為 []Symbol，實際 %T", env.Data)
	}
	if len(symbols) != 4 {
		t.Fatalf("應回傳全部 4 檔，實際 %d", len(symbols))
	}
	if symbols[0].Code != "2317" || symbols[0].Market != model.MarketTSE {
		t.Errorf("代碼應依 Code 排序: %+v", symbols[0])
	}

	env = callEnv(t, app, "get_symbol_list", map[string]any{"market": "otc"})
	otc := env.Data.([]model.Symbol)
	if len(otc) != 2 || otc[0].Market != model.MarketOTC {
		t.Errorf("otc 過濾錯誤: %+v", otc)
	}

	// 非法 market → 錯誤
	if _, err := app.core.Call(context.Background(), "get_symbol_list",
		map[string]any{"market": "hk"}); err == nil {
		t.Fatal("非法 market 應被拒絕")
	}
}

func TestFGGetTradingCalendar(t *testing.T) {
	f := newFake(t)
	tq := newFakeTAIFEX(t, "2026-07-29")
	app := fgApp(t, f, tq)

	// 2026 年 2 月：2/13 前、2/19-20 春節補班日等；官方休市日含 2/15-20、2/27-28
	env := callEnv(t, app, "get_trading_calendar", map[string]any{"year": 2026, "month": 2})
	cal, ok := env.Data.(model.TradingCalendar)
	if !ok {
		t.Fatalf("Data 應為 TradingCalendar，實際 %T", env.Data)
	}
	if cal.Year != 2026 || cal.Month != 2 || len(cal.TradingDays) == 0 {
		t.Errorf("行事曆基本欄位錯誤: %+v", cal)
	}
	// 2/20（週五）為春節補假休市 → 不得為交易日
	for _, d := range cal.TradingDays {
		if d == "2026-02-20" {
			t.Errorf("2/20 為官方休市日，不應在交易日清單")
		}
	}
	if len(cal.Holidays) == 0 {
		t.Error("2 月應有官方休市清單")
	}
	if cal.Note == "" {
		t.Error("應標示行事曆版本")
	}
	if env.Lineage.Freshness != model.FreshnessHistorical {
		t.Errorf("行事曆 freshness 應為 HISTORICAL: %+v", env.Lineage)
	}

	// 全年模式
	env = callEnv(t, app, "get_trading_calendar", map[string]any{"year": 2026})
	year := env.Data.(model.TradingCalendar)
	if year.Month != 0 || len(year.TradingDays) < 240 {
		t.Errorf("全年交易日應 >240 日，實際 %d", len(year.TradingDays))
	}

	// 非法 year/month → 錯誤
	if _, err := app.core.Call(context.Background(), "get_trading_calendar",
		map[string]any{"month": 13}); err == nil {
		t.Fatal("非法 month 應被拒絕")
	}
}

// ************** 路徑驗證（真實 TAIFEXQuery + httptest，§9.3） **************

// readFixture 讀取 provider 套件之官方錄製 fixture（測試工作目錄為 pkg/mcp）。
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "provider", "testdata", name))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return b
}

// tfxAPIHandler 依路徑回傳官方錄製 fixture。
func tfxAPIHandler(t *testing.T, w http.ResponseWriter, r *http.Request) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/PutCallRatio"):
		w.Write(readFixture(t, "tfx_PutCallRatio.json"))
	case strings.HasSuffix(r.URL.Path, "/DailyMarketReportFut"):
		w.Write(readFixture(t, "tfx_fut.json"))
	default:
		http.NotFound(w, r)
	}
}

// realTAIFEXApp 建立以 httptest 之真實 TAIFEXQuery 為後端之 App（L2 於 dir）。
func realTAIFEXApp(t *testing.T, apiSrv, dlSrv, dir string) *App {
	t.Helper()
	c, err := cache.New(cache.WithDataDir(dir))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	now := time.Date(2026, 7, 31, 16, 0, 0, 0, model.Taipei())
	q, err := provider.NewTAIFEXQuery(
		provider.NewTAIFEXAPISourceWithBase(apiSrv+"/v1"),
		provider.NewTAIFEXDLSourceWithBase(dlSrv+"/cht/3/"),
		c, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewTAIFEXQuery 失敗: %v", err)
	}
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(seedSymbols()),
		WithAppTAIFEX(q),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// TestFGFuturesPathLatestUsesAPI 最新交易日 → API（DL 零下載）。
func TestFGFuturesPathLatestUsesAPI(t *testing.T) {
	var apiHits, dlDownloads int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&apiHits, 1)
		tfxAPIHandler(t, w, r)
	}))
	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte("<html>view</html>"))
			return
		}
		atomic.AddInt32(&dlDownloads, 1)
		w.Write(readFixture(t, "taifex_fut_daily.csv"))
	}))
	t.Cleanup(func() { apiSrv.Close(); dlSrv.Close() })

	app := realTAIFEXApp(t, apiSrv.URL, dlSrv.URL, t.TempDir())
	env := callEnv(t, app, "get_futures_daily_ohlc", map[string]any{"contract": "TX"})
	rows := env.Data.([]model.FuturesDailyRow)
	if len(rows) == 0 {
		t.Fatal("API 最新日應有 TX 行情")
	}
	// 最新交易日 2026-07-31（PutCallRatio fixture 之最大日期）
	if rows[0].Date != "2026-07-31" {
		t.Errorf("應回傳 2026-07-31 行情，實際 %s", rows[0].Date)
	}
	if env.Lineage.Source != model.SourceTAIFEXAPI {
		t.Errorf("最新日 lineage 應為 TAIFEX_API: %+v", env.Lineage)
	}
	if dlDownloads != 0 {
		t.Errorf("最新日走 API 路徑，DL 不應下載（實際 %d 次）", dlDownloads)
	}
	if apiHits == 0 {
		t.Error("API 應被呼叫")
	}
}

// TestFGFuturesPathHistoryDLAndL2 歷史 → DL 下載；二次查詢 L2 命中不重複下載；
// 跨 App 實例（新 L1）仍 L2 命中。
func TestFGFuturesPathHistoryDLAndL2(t *testing.T) {
	var downloads int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tfxAPIHandler(t, w, r)
	}))
	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte("<html>view</html>"))
			return
		}
		atomic.AddInt32(&downloads, 1)
		w.Write(readFixture(t, "taifex_fut_daily.csv"))
	}))
	t.Cleanup(func() { apiSrv.Close(); dlSrv.Close() })

	dir := t.TempDir()
	args := map[string]any{"contract": "TX", "start": "2026-07-29", "end": "2026-07-29"}

	// 第一次：DL 下載 1 次（L2 寫入永久 TTL）
	app := realTAIFEXApp(t, apiSrv.URL, dlSrv.URL, dir)
	env := callEnv(t, app, "get_futures_history", args)
	rows := env.Data.([]model.FuturesDailyRow)
	if len(rows) == 0 {
		t.Fatal("歷史應有 TX 資料")
	}
	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Fatalf("首次歷史查詢應下載 1 次，實際 %d", got)
	}
	if env.Lineage.IsCached {
		t.Error("首次查詢不應 is_cached")
	}
	app.Close()

	// 第二次：新 App 實例（新 L1），L2 命中 → 不重複下載
	app2 := realTAIFEXApp(t, apiSrv.URL, dlSrv.URL, dir)
	env2 := callEnv(t, app2, "get_futures_history", args)
	if !env2.Lineage.IsCached {
		t.Errorf("L2 命中應 is_cached=true: %+v", env2.Lineage)
	}
	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Errorf("L2 命中後不應重複下載，實際 %d 次", got)
	}
}
