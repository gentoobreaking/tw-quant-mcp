package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sync"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// app_bc_test.go：§10.B/C 盤後工具之整合測試。
// 以 fake fetcher 注入（驗證 URL/參數建構、快取讀穿、lineage、錯誤路徑），
// 不進行真實 HTTP。

// fakeFetch 為 WebFetcher/APIFetcher/TPExFetcher 之測試替身：
// bodies 依「ds|params」鍵回傳已 normalize 之 JSON；不存在的鍵回 404 錯誤。
// 以 mutex 保護 map：rebuildScreenerIndex 之 bounded concurrency 掃描
// 會由多個 goroutine 併發 Fetch（§10.2）。
type fakeFetch struct {
	t        *testing.T
	mu       sync.Mutex
	bodies   map[string]string // key → normalized JSON body
	calls    map[string]int
	notFound map[string]bool // key → 模擬官方查無資料（404）
}

func newFake(t *testing.T) *fakeFetch {
	return &fakeFetch{t: t, bodies: map[string]string{}, calls: map[string]int{}, notFound: map[string]bool{}}
}

// key 依資料集與參數建構假 URL（測試可預期性）。
func fakeKey(ds string, params url.Values) string {
	return ds + "|" + params.Encode()
}

func (f *fakeFetch) stub(ds string, params url.Values, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bodies[fakeKey(ds, params)] = body
}

func (f *fakeFetch) stub404(ds string, params url.Values) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notFound[fakeKey(ds, params)] = true
}

func (f *fakeFetch) called(ds string, params url.Values) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[fakeKey(ds, params)]
}

// fakeWeb/fakeAPI/fakeTPEx 為 fakeFetch 之介面適配（URL 參數型別各異）。
type fakeWeb struct{ f *fakeFetch }

func (w fakeWeb) URL(ds provider.TWSEWebDataset, params url.Values) string {
	return fakeKey(fmt.Sprint(ds), params)
}
func (w fakeWeb) Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.Fetch(ctx, req)
}
func (w fakeWeb) Validate(raw *provider.RawResponse) error            { return w.f.Validate(raw) }
func (w fakeWeb) Normalize(raw *provider.RawResponse) ([]byte, error) { return w.f.Normalize(raw) }

type fakeAPI struct{ f *fakeFetch }

func (w fakeAPI) URL(ds provider.TWSEAPIDataset, params url.Values) string {
	return fakeKey(fmt.Sprint(ds), params)
}
func (w fakeAPI) Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.Fetch(ctx, req)
}
func (w fakeAPI) Validate(raw *provider.RawResponse) error            { return w.f.Validate(raw) }
func (w fakeAPI) Normalize(raw *provider.RawResponse) ([]byte, error) { return w.f.Normalize(raw) }

type fakeTPEx struct{ f *fakeFetch }

func (w fakeTPEx) URL(ds provider.TPExDataset, params url.Values) string {
	return fakeKey(fmt.Sprint(ds), params)
}
func (w fakeTPEx) Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.Fetch(ctx, req)
}
func (w fakeTPEx) Validate(raw *provider.RawResponse) error            { return w.f.Validate(raw) }
func (w fakeTPEx) Normalize(raw *provider.RawResponse) ([]byte, error) { return w.f.Normalize(raw) }

// fakeMOPS 為 MOPSFetcher 之測試替身（T012）。
type fakeMOPS struct{ f *fakeFetch }

func (w fakeMOPS) URL(ds provider.MOPSDataset, params url.Values) string {
	return fakeKey(fmt.Sprint(ds), params)
}
func (w fakeMOPS) Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.Fetch(ctx, req)
}
func (w fakeMOPS) Validate(raw *provider.RawResponse) error            { return w.f.Validate(raw) }
func (w fakeMOPS) Normalize(raw *provider.RawResponse) ([]byte, error) { return w.f.Normalize(raw) }
func (w fakeMOPS) RawNormalize(raw *provider.RawResponse) ([]byte, error) {
	return w.f.Normalize(raw)
}

func (f *fakeFetch) Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	key := req.URL
	f.mu.Lock()
	f.calls[key]++
	nf := f.notFound[key]
	body, ok := f.bodies[key]
	n := len(f.bodies)
	f.mu.Unlock()
	if nf {
		return nil, fmt.Errorf("provider: 上游 404（查無資料）")
	}
	if !ok {
		f.t.Fatalf("fakeFetch: 未 stub 之請求鍵 %q（已 stub: %d）", key, n)
	}
	return &provider.RawResponse{Body: []byte(body), SourceURL: key, StatusCode: 200}, nil
}

func (f *fakeFetch) Validate(raw *provider.RawResponse) error { return nil }

func (f *fakeFetch) Normalize(raw *provider.RawResponse) ([]byte, error) { return raw.Body, nil }

// mkDailyMonth 生成某月 n 根日 K（收盤價 100+i，線性上升，timestamp 連續）。
func mkDailyMonth(prefix, month string, start, n int) []byte {
	candles := make([]model.Candle, 0, n)
	for i := 0; i < n; i++ {
		c := model.Candle{
			Timestamp: fmt.Sprintf("%s-%s-%02d", prefix, month, i+1),
			Open:      100 + float64(start+i),
			High:      101 + float64(start+i),
			Low:       99 + float64(start+i),
			Close:     100 + float64(start+i),
			Volume:    1000,
			Amount:    100000,
		}
		candles = append(candles, c)
	}
	b, _ := json.Marshal(candles)
	return b
}

// bcApp 建立注入 fake fetcher 之 App（盤後工具測試基底）。
func bcApp(t *testing.T, f *fakeFetch) *App {
	t.Helper()
	symbols := seedSymbols()
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, model.Taipei()) // 盤後 16:00
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(symbols),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
		WithAppMOPS(fakeMOPS{f}),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func TestBCGetStockDailyQuoteTSE(t *testing.T) {
	f := newFake(t)
	// 2026-05（20 根）、06（20 根）、07（30 根）→ 共 70 根，MA60 可算
	f.stub("daily_k", url.Values{"date": {"20260501"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "05", 0, 20)))
	f.stub("daily_k", url.Values{"date": {"20260601"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "06", 20, 20)))
	f.stub("daily_k", url.Values{"date": {"20260701"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 40, 30)))
	app := bcApp(t, f)

	env := callEnv(t, app, "get_stock_daily_quote", map[string]any{"symbol": "2330", "date": "2026-07-30"})
	q, ok := env.Data.(model.DailyQuote)
	if !ok {
		t.Fatalf("Data 應為 DailyQuote，實際 %T", env.Data)
	}
	if q.Symbol != "2330" || q.Date != "2026-07-30" || q.Close != 169 {
		t.Errorf("報價欄位不符: %+v", q)
	}
	if q.Indicators.MA60 == 0 || q.Indicators.MA20 == 0 {
		t.Errorf("MA 指標應已計算: %+v", q.Indicators)
	}
	if q.Indicators.RSI14 != 100 {
		t.Errorf("單調上升序列 RSI 應為 100，實際 %v", q.Indicators.RSI14)
	}
	if q.Indicators.MACD.MACD <= 0 {
		t.Errorf("上升序列 MACD DIF 應 > 0，實際 %+v", q.Indicators.MACD)
	}
	if env.Lineage.Source != model.SourceTWSEWeb || env.Lineage.Freshness != model.FreshnessPostMarket {
		t.Errorf("lineage 不符: %+v", env.Lineage)
	}
	if len(env.Lineage.DerivedFrom) != 1 || env.Lineage.DerivedFrom[0] != "TWSE_WEB:daily_k" {
		t.Errorf("derived_from 應標明父資料集: %+v", env.Lineage.DerivedFrom)
	}
	// 二次查詢命中快取
	env2 := callEnv(t, app, "get_stock_daily_quote", map[string]any{"symbol": "2330", "date": "2026-07-30"})
	if !env2.Lineage.IsCached {
		t.Error("二次查詢應 is_cached=true")
	}
	if env2.Lineage.CacheTTL <= 0 {
		t.Errorf("cache_ttl 應為正（§4.2 盤後至隔日 08:00），實際 %d", env2.Lineage.CacheTTL)
	}
}

func TestBCGetStockDailyQuoteOTC(t *testing.T) {
	f := newFake(t)
	f.stub("daily_close", url.Values{"date": {"20260730"}}, `[{"date":"2026-07-30","code":"6147","name":"頎邦","close":75.5,"change_dir":"+","change":1.2,"open":74.3,"high":76,"low":74.1,"volume":1200000}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_stock_daily_quote", map[string]any{"symbol": "6147", "date": "2026-07-30"})
	q := env.Data.(model.DailyQuote)
	if q.Market != model.MarketOTC || q.Close != 75.5 || q.Note == "" {
		t.Errorf("上櫃報價不符: %+v", q)
	}
	if env.Lineage.Source != model.SourceTPExAPI {
		t.Errorf("上櫃 lineage 應為 TPEX_API，實際 %v", env.Lineage.Source)
	}
}

func TestBCGetStockDailyQuoteNoData(t *testing.T) {
	f := newFake(t)
	f.stub("daily_k", url.Values{"date": {"20260501"}, "stockNo": {"2330"}}, "[]")
	f.stub("daily_k", url.Values{"date": {"20260601"}, "stockNo": {"2330"}}, "[]")
	f.stub("daily_k", url.Values{"date": {"20260701"}, "stockNo": {"2330"}}, "[]")
	app := bcApp(t, f)
	if _, err := app.core.Call(context.Background(), "get_stock_daily_quote",
		map[string]any{"symbol": "2330", "date": "2026-07-30"}); err == nil {
		t.Fatal("無資料日期應回明確錯誤")
	}
}

func TestBCGetStockDailyKline(t *testing.T) {
	f := newFake(t)
	f.stub("daily_k", url.Values{"date": {"20260730"}, "stockNo": {"2330"}, "period": {"week"}, "adjust": {"Y"}}, string(mkDailyMonth("2026", "07", 0, 10)))
	app := bcApp(t, f)
	env := callEnv(t, app, "get_stock_daily_kline", map[string]any{"symbol": "2330", "date": "2026-07-30", "period": "week", "adjust": true})
	rows, ok := env.Data.([]model.Candle)
	if !ok || len(rows) != 10 {
		t.Fatalf("Data 應為 10 根 Candle，實際 %T(%d)", env.Data, len(rows))
	}
	if f.called("daily_k", url.Values{"date": {"20260730"}, "stockNo": {"2330"}, "period": {"week"}, "adjust": {"Y"}}) != 1 {
		t.Error("應以 week/adjust=Y 參數請求 STOCK_DAY")
	}
	if chartType(env) != "candlestick" {
		t.Errorf("chart 應為 candlestick，實際 %v", chartType(env))
	}
	// 上櫃 → 明確錯誤
	if _, err := app.core.Call(context.Background(), "get_stock_daily_kline",
		map[string]any{"symbol": "6147"}); err == nil {
		t.Fatal("上櫃 K 線應為明確錯誤")
	}
}

func TestBCGetMarketSummary(t *testing.T) {
	f := newFake(t)
	tse := `[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20}]` +
		`[{"code":"2317","name":"鴻海","volume":2000,"amount":200000,"open":100,"high":100,"low":90,"close":90,"change_dir":"-","change":-10,"pe":15}]`
	// market_close 為兩表結構，fake 直接以 []MarketCloseRow 提供（normalize 已在 provider 層）
	tse = `[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20},{"code":"2317","name":"鴻海","volume":2000,"amount":200000,"open":100,"high":100,"low":90,"close":90,"change_dir":"-","change":-10,"pe":15}]`
	f.stub("market_close", url.Values{"date": {"20260730"}, "type": {"ALL"}}, tse)
	otc := `[{"date":"2026-07-30","code":"6147","name":"頎邦","close":75.5,"change_dir":"+","change":1.2,"open":74.3,"high":76,"low":74.1,"volume":1200000}]`
	f.stub("daily_close", url.Values{"date": {"20260730"}}, otc)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_market_summary", map[string]any{"date": "2026-07-30"})
	ms := env.Data.(model.MarketSummary)
	if ms.TSE.Advancers != 1 || ms.TSE.Decliners != 1 {
		t.Errorf("上市漲跌家數不符: %+v", ms.TSE)
	}
	if ms.TSE.LimitUp != 1 || ms.TSE.LimitDown != 1 {
		t.Errorf("漲跌停判定不符: %+v", ms.TSE)
	}
	if ms.OTC.Advancers != 1 || ms.TSE.TotalAmount != 300000 {
		t.Errorf("上櫃/總量不符: %+v %+v", ms.TSE, ms.OTC)
	}
}

func TestBCGetInstitutionalInvestors(t *testing.T) {
	f := newFake(t)
	f.stub("institutional", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","foreign_buy":1000,"foreign_sell":400,"foreign_net":600,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":0,"investment_sell":0,"investment_net":0}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_institutional_investors", map[string]any{"market": "tse", "date": "2026-07-30"})
	s := env.Data.(model.InstitutionalSummary)
	if s.TotalNet != 600 || len(s.Rows.([]provider.InstitutionalRow)) != 1 {
		t.Errorf("法人彙總不符: %+v", s)
	}
}

func TestBCGetForeignIndustryHoldings(t *testing.T) {
	f := newFake(t)
	f.stub("foreign_holdings", nil,
		`[{"industry":"半導體業","company_count":10,"share_number":1000,"foreign_share":500,"percentage":50.0}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_foreign_industry_holdings", map[string]any{"date": "2026-07-30"})
	rows := env.Data.([]provider.ForeignHoldingRow)
	if len(rows) != 1 || rows[0].Percentage != 50 {
		t.Errorf("外資產業配置不符: %+v", rows)
	}
	if chartType(env) != "pie" {
		t.Errorf("chart 應為 pie，實際 %v", chartType(env))
	}
}

func TestBCGetForeignShareholdingHistory(t *testing.T) {
	f := newFake(t)
	mk := func(d string) string {
		return fmt.Sprintf(`[{"date":"%s","code":"2330","name":"台積電","issue_shares":25930389000,"foreign_shares":1000000,"foreign_percent":10.5,"upper_limit_pct":100.0,"change_reason":"","last_changed_date":""}]`, d)
	}
	f.stub("qfiis", url.Values{"dayDate": {"20260730"}}, mk("2026-07-30"))
	f.stub("qfiis", url.Values{"dayDate": {"20260729"}}, mk("2026-07-29"))
	f.stub("qfiis", url.Values{"dayDate": {"20260728"}}, mk("2026-07-28"))
	app := bcApp(t, f)
	env := callEnv(t, app, "get_foreign_shareholding_history", map[string]any{"symbol": "2330", "range": 3, "date": "2026-07-30"})
	h := env.Data.(model.ForeignShareholdingHistory)
	if len(h.Series) != 3 || h.Series[0].Date != "2026-07-30" || h.Series[2].ForeignPercent != 10.5 {
		t.Errorf("外資持股歷史不符: %+v", h.Series)
	}
	// 上櫃 → 明確錯誤
	if _, err := app.core.Call(context.Background(), "get_foreign_shareholding_history",
		map[string]any{"symbol": "6147"}); err == nil {
		t.Fatal("上櫃外資持股應為錯誤")
	}
	// 快取：二次查詢全命中
	env2 := callEnv(t, app, "get_foreign_shareholding_history", map[string]any{"symbol": "2330", "range": 3, "date": "2026-07-30"})
	if !env2.Lineage.IsCached {
		t.Error("二次查詢應 is_cached=true")
	}
}

func TestBCGetMarginTrading(t *testing.T) {
	f := newFake(t)
	f.stub("margin", url.Values{"date": {"20260730"}, "selectType": {"ALL"}},
		`[{"code":"2330","name":"台積電","margin_buy":100000,"margin_sell":50000,"margin_cash_redeem":10000,"margin_prev_balance":1000000,"margin_balance":1040000,"margin_limit":2000000}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_margin_trading", map[string]any{"symbol": "2330", "date": "2026-07-30"})
	r, ok := env.Data.(provider.MarginRow)
	if !ok || r.MarginBalance != 1040000 {
		t.Errorf("融資融券不符: %+v", r)
	}
}

func TestBCGetAbnormalTrading(t *testing.T) {
	f := newFake(t)
	f.stub("abnormal_volume", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","notice_count":2,"info":"連續三個營業日達注意標準","date":"2026-07-30","close":169,"pe":28},{"code":"2317","name":"鴻海","notice_count":1,"info":"近六個營業日累計漲幅達標準","date":"2026-07-30","close":200,"pe":25}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_abnormal_trading", map[string]any{"market": "tse", "date": "2026-07-30", "top_n": 1})
	out := env.Data.([]model.AbnormalTrade)
	if len(out) != 1 || out[0].Code != "2330" || out[0].NoticeCount != 2 {
		t.Errorf("異常交易排名不符: %+v", out)
	}
}

func TestBCGetWarrantActivity(t *testing.T) {
	f := newFake(t)
	f.stub("warrants", nil, `[{"trade_date":"2026-07-30","code":"052644","name":"台積電國票41購01","amount":5000000,"volume":100000},{"trade_date":"2026-07-30","code":"052645","name":"台積電元大41購02","amount":3000000,"volume":200000}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_warrant_activity", map[string]any{"date": "2026-07-30", "top_n": 1})
	w := env.Data.(model.WarrantSummary)
	top := w.AmountTop[0].(provider.WarrantRow)
	if top.Code != "052644" {
		t.Errorf("金額 Top 不符: %+v", top)
	}
	vt := w.VolumeTop[0].(provider.WarrantRow)
	if vt.Code != "052645" {
		t.Errorf("張數 Top 不符: %+v", vt)
	}
}

func TestBCGetMajorAnnouncementsWired(t *testing.T) {
	f := newFake(t)
	// 模擬 MOPS 回傳已歸一化的重大訊息 JSON（full spectrum 過濾後）
	announcements := `[
{"table_date":"2026-07-30","announce_date":"2026-07-30","announce_time":"18:30:00",
"code":"2330","name":"台積電","subject":"本公司董事會決議配發現金股利",
"clause":"第14款","fact_date":"2026-07-30","description":"每股配發新台幣8元"},
{"table_date":"2026-07-30","announce_date":"2026-07-29","announce_time":"09:15:00",
"code":"2885","name":"元大金控","subject":"公告金管會核准本公司合併元大投信",
"clause":"第11款","fact_date":"2026-07-29","description":"以股份轉換方式納為100%持股子公司"}
]`
	f.stub("announcements", nil, announcements)
	app := bcApp(t, f)

	// 查詢全量（不含過濾參數）
	env := callEnv(t, app, "get_major_announcements", map[string]any{})
	rows, ok := env.Data.([]model.MajorAnnouncement)
	if !ok {
		t.Fatalf("Data 應為 []any，實際 %T", env.Data)
	}
	if len(rows) != 2 {
		t.Fatalf("應回 2 筆重大訊息，實際 %d 筆", len(rows))
	}
	// 驗證 lineage
	if env.Lineage.Source != model.SourceMOPS {
		t.Errorf("lineage 來源應為 MOPS，實際 %s", env.Lineage.Source)
	}

	// 查詢依 symbol 過濾（模擬：filterFn 穿透後只剩 2330）
	f2330 := newFake(t)
	f2330.stub("announcements", nil, `[
{"table_date":"2026-07-30","announce_date":"2026-07-30","announce_time":"18:30:00",
"code":"2330","name":"台積電","subject":"本公司董事會決議配發現金股利",
"clause":"第14款","fact_date":"2026-07-30","description":"每股配發新台幣8元"}
]`)
	app2 := bcApp(t, f2330)
	env2 := callEnv(t, app2, "get_major_announcements", map[string]any{"symbol": "2330"})
	rows2, _ := env2.Data.([]model.MajorAnnouncement)
	if len(rows2) != 1 {
		t.Fatalf("依 symbol 過濾後應為 1 筆，實際 %d 筆", len(rows2))
	}
}

func TestBCGetAttentionDispositionStocksTSE(t *testing.T) {
	f := newFake(t)
	f.stub("abnormal_volume", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","notice_count":2,"info":"連續三個營業日達注意標準","date":"2026-07-30","close":169,"pe":28}]`)
	f.stub("punish", url.Values{"date": {"20260730"}},
		`[{"number":"1","date":"1150722","code":"2317","name":"鴻海","notice_count":3,"reasons":"連續三次","disposition_period":"115/07/23～115/08/05","disposition_measure":"第一次處置","detail":"人工管制撮合"}]`)
	app := bcApp(t, f)
	env := callEnv(t, app, "get_attention_disposition_stocks", map[string]any{"market": "tse", "date": "2026-07-30"})
	list := env.Data.(model.AttentionDispositionList)
	if len(list.Attention) != 1 || len(list.Disposition) != 1 {
		t.Fatalf("名單不符: %+v", list)
	}
	if list.Disposition[0].Measure != "第一次處置" {
		t.Errorf("處置措施不符: %+v", list.Disposition[0])
	}
	// 名單應已餵入 DaytradeScanner（T010 scan 之供應器）
	scan, err := app.risk.Scan("2317")
	if err != nil {
		t.Fatalf("Scan 失敗: %v", err)
	}
	_ = scan
	if !app.risk.loaded {
		t.Error("名單應已載入 DaytradeScanner")
	}
}

func TestBCResolveDateDefault(t *testing.T) {
	// 週日（2026-07-26）盤後呼叫 → 回最近交易日（2026-07-24 週五）
	f := newFake(t)
	symbols := seedSymbols()
	now := time.Date(2026, 7, 26, 20, 0, 0, 0, model.Taipei())
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(symbols),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	defer app.Close()
	d, err := app.resolveDate("")
	if err != nil {
		t.Fatalf("resolveDate 失敗: %v", err)
	}
	if d != "2026-07-24" {
		t.Errorf("週日應回 2026-07-24，實際 %s", d)
	}
	// 盤中 10:00（非 15:00 後）→ 前一日
	now2 := time.Date(2026, 7, 30, 10, 0, 0, 0, model.Taipei())
	app2, _ := NewApp(nil, WithAppClock(func() time.Time { return now2 }), WithAppSymbols(seedSymbols()),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}))
	defer app2.Close()
	if d2, _ := app2.resolveDate(""); d2 != "2026-07-29" {
		t.Errorf("盤中 10:00 應回 2026-07-29，實際 %s", d2)
	}
}

// callEnv 呼叫工具並解析 Envelope（簡化版，不走 MCP transport）。
func callEnv(t *testing.T, app *App, name string, args map[string]any) *model.Envelope {
	t.Helper()
	env, err := app.core.Call(context.Background(), name, args)
	if err != nil {
		t.Fatalf("Call %s 失敗: %v", name, err)
	}
	e, ok := env.(*model.Envelope)
	if !ok {
		t.Fatalf("回傳應為 *model.Envelope，實際 %T", env)
	}
	return e
}

// chartType 取出 _chart_meta 之 recommended_type。
func chartType(env *model.Envelope) string {
	if env.ChartMeta == nil {
		return ""
	}
	return env.ChartMeta.RecommendedType
}
