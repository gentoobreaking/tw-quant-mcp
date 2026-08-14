package mcp

// prewarm_test.go：§12.9 預熱排程（T018）測試。
// 以 httptest 注入行事曆/代碼表/MIS Session 端點，fake fetcher 注入
// 盤後資料源；驗證各階段於正確時段執行一次、非交易日跳過、
// 失敗不阻塞、跨日重置、ctx 取消結束。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/calendar"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
	"tw-quant-mcp/pkg/registry"
)

// prewarmAt 回傳 2026-07-31（週五，交易日）指定時刻。
func prewarmAt(h, m int) time.Time {
	return time.Date(2026, 7, 31, h, m, 0, 0, model.Taipei())
}

// prewarmApp 建立預熱排程測試之 App：L2 快取（dataDir）+ 短間隔 client +
// 注入式行事曆/代碼表/MIS 端點（httptest，避免真實網路）。
func prewarmApp(t *testing.T, f *fakeFetch, now func() time.Time) *App {
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
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

// scheduleServer 回傳行事曆 API 之 httptest 伺服器（含 2026-09-14 測試休市日）。
func scheduleServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"stat":"ok","date":"20260914","title":"test","data":[["2026-09-14","測試休市日",""]]}`))
	}))
	t.Cleanup(srv.Close)
	calendar.SetScheduleURL(srv.URL)
	return srv
}

// listServers 回傳 TWSE/TPEx/ETF 代碼表之 httptest 伺服器。
func listServers(t *testing.T) (string, string, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/tse":
			_, _ = w.Write([]byte(`[{"公司代號":"2330","公司名稱":"台積電","產業別":"半導體"}]`))
		case "/tpex":
			_, _ = w.Write([]byte(`[{"SecuritiesCompanyCode":"6147","CompanyName":"頎邦"}]`))
		case "/etf":
			_, _ = w.Write([]byte(`[]`)) // 空 ETF 清單
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	twseURL := srv.URL + "/tse"
	tpexURL := srv.URL + "/tpex"
	etfURL := srv.URL + "/etf"
	registry.SetListURLs(twseURL, tpexURL)
	registry.SetETFListURL(etfURL)
	return twseURL, tpexURL, etfURL
}

// misServer 回傳 MIS index.jsp 之 httptest 伺服器（統計請求次數）。
func misServer(t *testing.T) (srv *httptest.Server, hits func() int) {
	t.Helper()
	var n int
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	t.Cleanup(srv.Close)
	provider.SetMISIndexURL(srv.URL)
	t.Cleanup(func() { provider.SetMISIndexURL("https://mis.twse.com.tw/stock/index.jsp") })
	return srv, func() int { return n }
}

// stubEOD 建立 2026-07-31 盤後預熱所需之整批 stub（§12.4 彙總/名單端點）。
// 註：TSE/TPEx 法人同用 "institutional" 資料集 ID（fake 鍵相同，一體共用）。
func stubEOD(f *fakeFetch) {
	f.stub("market_close", url.Values{"date": {"20260731"}, "type": {"ALL"}},
		`[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20}]`)
	f.stub("daily_close", url.Values{"date": {"20260731"}},
		`[{"date":"2026-07-31","code":"6147","name":"頎邦","close":75.5,"change_dir":"+","change":1.2,"open":74.3,"high":76,"low":74.1,"volume":1200000}]`)
	f.stub("institutional", url.Values{"date": {"20260731"}},
		`[{"code":"2330","name":"台積電","foreign_buy":1000,"foreign_sell":400,"foreign_net":600,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":0,"investment_sell":0,"investment_net":0}]`)
	f.stub("foreign_holdings", nil,
		`[{"industry":"半導體業","company_count":10,"share_number":1000,"foreign_share":500,"percentage":50.0}]`)
	f.stub("abnormal_volume", url.Values{"date": {"20260731"}},
		`[{"code":"2330","name":"台積電","notice_count":2,"info":"連續三個營業日達注意標準","date":"2026-07-31","close":169,"pe":28}]`)
	f.stub("punish", url.Values{"date": {"20260731"}},
		`[{"number":"1","date":"1150731","code":"2317","name":"鴻海","notice_count":3,"reasons":"連續三次","disposition_period":"115/07/23～115/08/05","disposition_measure":"第一次處置","detail":"人工管制撮合"}]`)
	f.stub("attention", url.Values{"date": {"20260731"}},
		`[{"code":"6147","name":"頎邦","info":"最近六個營業日累積收盤價跌幅達標準"}]`)
	f.stub("disposition", url.Values{"date": {"20260731"}},
		`[{"code":"6547","name":"高端疫苗","info":"處置","disposition_period":"115/07/31～115/08/13"}]`)
	// Index endpoints for get_twse_index prewarm
	f.stub("indices", nil,
		`[{"date":"1150731","index_name":"發行量加權股價指數","close":17000.0,"change":-50.0,"change_percent":-0.29,"change_dir":"-","note":""}]`)
	f.stub("index_history", url.Values{"date": {"20260701"}},
		`{"stat":"OK","fields":["日期","開盤指數","最高指數","最低指數","收盤指數"],"data":[["115/07/01","17000.0","17100.0","16900.0","17050.0"],["115/07/02","17050.0","17150.0","16950.0","17100.0"]],"total":2}`)
}

// TestPrewarmMorning：08:00 後行事曆 + 代碼表入 L2，並載入 Symbol Registry。
func TestPrewarmMorning(t *testing.T) {
	f := newFake(t)
	app := prewarmApp(t, f, func() time.Time { return prewarmAt(8, 30) })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), prewarmAt(7, 0))
	if app.symbols.Len() != 0 {
		t.Fatal("07:00 不應預熱（未到 08:00）")
	}

	s.TickOnce(context.Background(), prewarmAt(8, 30))
	// 代碼表 → Symbol Registry
	if _, ok := app.symbols.Lookup("2330"); !ok {
		t.Error("預熱後 2330 應註冊於 Symbol Registry")
	}
	if _, ok := app.symbols.Lookup("6147"); !ok {
		t.Error("預熱後上櫃 6147 應註冊於 Symbol Registry")
	}
	// 行事曆 → 官方休市日已合併（2026-09-14 測試日）
	if app.calendar.IsTradingDay(time.Date(2026, 9, 14, 0, 0, 0, 0, model.Taipei())) {
		t.Error("官方休市日 2026-09-14 應為非交易日")
	}
	// 行事曆入 L2（24h TTL ≥ l2WriteMinTTL；鍵日期依 Loader/Calendar 之
	// 當日鍵（model.Now），與注入時鐘一致之交易日）
	ctx := context.Background()
	date := model.FormatDate(model.Now().Time)
	if _, ok, err := cache.Get[[]calendar.Holiday](ctx, app.cache,
		cache.KeyString(model.SourceTWSEWeb, cache.DatasetCalendar, date, "", nil),
		cache.WithDataset(cache.DatasetCalendar, date)); err != nil || !ok {
		t.Errorf("行事曆應已入 L2（ok=%v err=%v）", ok, err)
	}
	// Symbol Registry 已載入驗證（替代 L2 快取檢查，因快取鍵含 URL）
	if _, ok := app.symbols.Lookup("2330"); !ok {
		t.Error("預熱後 2330 應註冊於 Symbol Registry")
	}
	if _, ok := app.symbols.Lookup("6147"); !ok {
		t.Error("預熱後上櫃 6147 應註冊於 Symbol Registry")
	}
	// 每日一次：再次 tick 不重複抓取（Symbol 數不變，無額外狀態變化）
	s.TickOnce(context.Background(), prewarmAt(10, 0))
	if app.symbols.Len() != 2 {
		t.Errorf("同日不得重複預熱，Symbol 數應為 2，實際 %d", app.symbols.Len())
	}
}

// TestPrewarmPreOpen：08:45 開盤前 MIS Session 重取（index.jsp 命中）。
func TestPrewarmPreOpen(t *testing.T) {
	f := newFake(t)
	app := prewarmApp(t, f, func() time.Time { return prewarmAt(8, 50) })
	_, hits := misServer(t) // 最後註冊者生效（prewarmApp 之伺服器被覆寫）
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), prewarmAt(8, 30))
	// MIS warmup 只有一次（preOpen 階段每日一次）
	s.TickOnce(context.Background(), prewarmAt(9, 0))
	if got := hits(); got != 1 {
		t.Errorf("MIS Session 預熱每日應恰一次，實際 %d", got)
	}
}

// TestPrewarmEOD：16:45 當日盤後資料入快取，後續查詢零上游 HTTP（http_calls=0）。
func TestPrewarmEOD(t *testing.T) {
	f := newFake(t)
	stubEOD(f)
	now := prewarmAt(16, 45)
	app := prewarmApp(t, f, func() time.Time { return now })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), prewarmAt(15, 0))
	now = prewarmAt(17, 0)
	s.TickOnce(context.Background(), prewarmAt(17, 0))
	// 預熱後之查詢應零上游 HTTP（§12.9 instrumentation：http_calls=0）
	env := callEnv(t, app, "get_market_summary", map[string]any{})
	if env.HTTPCalls != 0 {
		t.Errorf("market_summary 預熱後查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	env = callEnv(t, app, "get_institutional_investors", map[string]any{"market": "tse"})
	if env.HTTPCalls != 0 {
		t.Errorf("institutional(tse) 預熱後查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	env = callEnv(t, app, "get_attention_disposition_stocks", map[string]any{"market": "tse"})
	if env.HTTPCalls != 0 {
		t.Errorf("attention_disposition 預熱後查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
	// 未預熱之資料集（外資持股歷史）仍會打上游（http_calls>0，控制組）
	f.stub("qfiis", url.Values{"dayDate": {"20260730"}},
		`[{"date":"2026-07-30","code":"2330","name":"台積電","issue_shares":25930389000,"foreign_shares":1000000,"foreign_percent":10.5,"upper_limit_pct":100.0}]`)
	env = callEnv(t, app, "get_foreign_shareholding_history",
		map[string]any{"symbol": "2330", "range": 1, "date": "2026-07-30"})
	if env.HTTPCalls == 0 {
		t.Error("未預熱資料集之查詢應有上游 HTTP（控制組）")
	}
}

// TestPrewarmSkipsNonTradingDay：非交易日（週六）僅執行基礎設施預熱
// （交易日曆 + 公司代碼表，24h TTL 官方基礎資料），跳過盤中相關階段
// （MIS Session 重取、盤後彙總）。
func TestPrewarmSkipsNonTradingDay(t *testing.T) {
	_, hits := misServer(t)
	f := newFake(t)
	sat := time.Date(2026, 8, 1, 17, 0, 0, 0, model.Taipei()) // 週六
	app := prewarmApp(t, f, func() time.Time { return sat })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), sat)
	if app.symbols.Len() == 0 {
		t.Error("非交易日應載入基礎設施（公司代碼表）")
	}
	if _, ok := app.symbols.Lookup("2330"); !ok {
		t.Error("非交易日載入後 2330 應註冊於 Symbol Registry")
	}
	if hits() != 0 {
		t.Error("非交易日不得執行 MIS Session 預熱")
	}
}

// TestPrewarmFailureDoesNotBlock：階段失敗僅記錄，其餘階段照常執行。
func TestPrewarmFailureDoesNotBlock(t *testing.T) {
	f := newFake(t)
	stubEOD(f)
	now := prewarmAt(17, 0)
	app := prewarmApp(t, f, func() time.Time { return now })
	_, hits := misServer(t)

	// 行事曆/代碼表端點改為全數 500（failure）；MIS 正常
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(bad.Close)
	calendar.SetScheduleURL(bad.URL)
	registry.SetListURLs(bad.URL, bad.URL)

	// 三階段全部到達：行事曆/代碼表失敗 → 不阻塞 preOpen 與 EOD
	s := NewPrewarmScheduler(app)
	s.TickOnce(context.Background(), prewarmAt(17, 0))
	if hits() != 1 {
		t.Errorf("preOpen 應已執行（MIS hits=%d）", hits())
	}
	if !s.eodDone {
		t.Error("EOD 應已標記完成（即使有失敗也僅記錄）")
	}
	// EOD 仍正常生效（失敗不影響後續查詢路徑：http_calls=0）
	env := callEnv(t, app, "get_market_summary", map[string]any{})
	if env.HTTPCalls != 0 {
		t.Errorf("失敗階段之後 EOD 預熱仍應生效，http_calls=%d", env.HTTPCalls)
	}
}

// TestPrewarmDailyRollover：跨日重置旗標，次日交易日重新預熱。
func TestPrewarmDailyRollover(t *testing.T) {
	f := newFake(t)
	app := prewarmApp(t, f, func() time.Time { return prewarmAt(8, 30) })
	s := NewPrewarmScheduler(app)

	s.TickOnce(context.Background(), prewarmAt(8, 30)) // 週五預熱
	if app.symbols.Len() != 2 {
		t.Fatalf("首日應預熱，Symbol 數=%d", app.symbols.Len())
	}
	// 同日再 tick：不重複
	s.TickOnce(context.Background(), prewarmAt(12, 0))
	if app.symbols.Len() != 2 {
		t.Fatalf("同日不得重複，Symbol 數=%d", app.symbols.Len())
	}
	// 週六（非交易日）→ 旗標重置（不執行）
	s.TickOnce(context.Background(), time.Date(2026, 8, 1, 9, 0, 0, 0, model.Taipei()))
	// 下週一（2026-08-03 交易日）09:00 → 重新預熱（symbols 覆寫，數量仍 2，但狀態機已重跑）
	s.TickOnce(context.Background(), time.Date(2026, 8, 3, 9, 0, 0, 0, model.Taipei()))
	if !s.morningDone {
		t.Error("跨日後應重置並重新預熱 morning 階段")
	}
}

// TestPrewarmRunCancels：Run 隨 ctx 取消結束。
func TestPrewarmRunCancels(t *testing.T) {
	f := newFake(t)
	app := prewarmApp(t, f, func() time.Time { return prewarmAt(3, 0) })
	s := NewPrewarmScheduler(app, WithPrewarmTick(time.Hour))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run 應於 ctx 取消時正常結束，實際 %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run 未隨 ctx 取消結束（timeout）")
	}
}
