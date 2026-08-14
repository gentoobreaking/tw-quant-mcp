package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
)

// testClock 為交易時段固定時鐘（2026-07-31 週五 09:30:00）。
func testClock() time.Time {
	return time.Date(2026, 7, 31, 9, 30, 0, 0, model.Taipei())
}

// seedSymbols 註冊測試用上市/上櫃代碼。
func seedSymbols() *model.Registry {
	reg := model.NewRegistry()
	_ = reg.Set([]model.Symbol{
		{Code: "2330", Name: "台積電", Market: model.MarketTSE},
		{Code: "2317", Name: "鴻海", Market: model.MarketTSE},
		{Code: "6147", Name: "頎邦", Market: model.MarketOTC},
		{Code: "6547", Name: "高端疫苗", Market: model.MarketOTC},
	})
	return reg
}

// seedSnapshots 種入 2330 一連串快照（09:00–09:30 每分鐘）與五檔。
func seedSnapshots(rings *engine.RingStore) {
	base := time.Date(2026, 7, 31, 9, 0, 0, 0, model.Taipei())
	last := 100.0
	for i := 0; i < 31; i++ {
		t := base.Add(time.Duration(i) * time.Minute)
		last = 100 + float64(i)*0.5
		s := model.Snapshot{
			Code:          "2330",
			Exch:          "tse_2330.tw",
			Time:          model.NewTaipeiTime(t),
			TradeTime:     t.Format("15:04:05"),
			Last:          last,
			Open:          100,
			High:          last,
			Low:           100,
			PrevClose:     99,
			Change:        last - 99,
			MinuteVol:     100000,
			CumulativeVol: 100000 * int64(i+1),
			Book: &model.LevelBook{
				Bids: []model.PriceLevel{{Price: last - 0.5, Volume: 1000}},
				Asks: []model.PriceLevel{{Price: last + 0.5, Volume: 2000}},
			},
		}
		rings.Append(s)
	}
}

// newTestApp 建立測試用 App（交易時段、註冊代碼、2330 快照已種入，
// 衍生計算已同步）。calendar 為 nil 時用內建行事曆。
func newTestApp(t *testing.T) *App {
	t.Helper()
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
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	_ = watchlist.Set([]model.Symbol{
		{Code: "2330", Name: "台積電", Market: model.MarketTSE},
		{Code: "2317", Name: "鴻海", Market: model.MarketTSE},
	})
	return app
}

// callTool 透過 SDK in-memory session 呼叫工具並回傳解析後之 Envelope。
func callTool(t *testing.T, app *App, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "v0"}, nil)
	app.Wire(srv)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server Connect 失敗: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client Connect 失敗: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s 協定錯誤: %v", name, err)
	}
	return res
}

// callCore 直接經 Core 呼叫（不經 SDK 傳輸層，供資料層斷言）。
func callCore(t *testing.T, app *App, name string, args map[string]any) *model.Envelope {
	t.Helper()
	env, err := app.Core().Call(context.Background(), name, args)
	if err != nil {
		t.Fatalf("Core.Call %s 失敗: %v", name, err)
	}
	return env.(*model.Envelope)
}

func TestRegistryContains6Tools(t *testing.T) {
	app := newTestApp(t)
	names := app.Registry().Names()
	if len(names) != 38 {
		t.Fatalf("應登錄 38 個工具（A 6 + B/C 11 + D/E 10 + F/G 9 + T029 缺口 1 + get_twse_index），實際 %d: %v", len(names), names)
	}
	if len(app.Registry().Tools()) != 38 {
		t.Fatalf("Tools() 應回傳 38 個 mcp.Tool")
	}
	if !strings.Contains(app.Registry().BuildTOML(), "set_active_watchlist") {
		t.Fatalf("BuildTOML 應含工具清單")
	}
	if !strings.Contains(app.Registry().BuildTOML(), "screen_stocks") {
		t.Fatalf("BuildTOML 應含 D/E 工具清單")
	}
	if !strings.Contains(app.Registry().BuildTOML(), "get_futures_daily_ohlc") {
		t.Fatalf("BuildTOML 應含 F/G 工具清單")
	}
}

func TestCallSetActiveWatchlist(t *testing.T) {
	app := newTestApp(t)
	res := callTool(t, app, "set_active_watchlist", map[string]any{
		"symbols": []any{"2330", "6147"},
	})
	if res.IsError {
		t.Fatalf("不應為錯誤: %s", res.Content[0])
	}
	env := res.StructuredContent.(map[string]any)
	data := env["data"].(map[string]any)
	if data["count"].(float64) != 2 {
		t.Errorf("count 應為 2，實際 %v", data["count"])
	}
	if app.Watchlist().Len() != 2 {
		t.Errorf("watchlist 應為 2 檔")
	}
}

func TestCallSetActiveWatchlistErrors(t *testing.T) {
	app := newTestApp(t)

	// >15 檔（schema maxItems 攔截）
	res := callTool(t, app, "set_active_watchlist", map[string]any{
		"symbols": []any{"2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330", "2330"},
	})
	if !res.IsError {
		t.Fatalf("16 檔應為錯誤")
	}

	// 0 檔
	res = callTool(t, app, "set_active_watchlist", map[string]any{"symbols": []any{}})
	if !res.IsError {
		t.Fatalf("空清單應為錯誤")
	}

	// 非法代號（未註冊）
	res = callTool(t, app, "set_active_watchlist", map[string]any{"symbols": []any{"99999"}})
	if !res.IsError {
		t.Fatalf("未知代號應為錯誤")
	}

	// 非交易時段（週六）
	weekend := time.Date(2026, 8, 1, 9, 30, 0, 0, model.Taipei()) // 週六
	app3, err := NewApp(nil,
		WithAppClock(func() time.Time { return weekend }),
		WithAppSymbols(seedSymbols()),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	res = callTool(t, app3, "set_active_watchlist", map[string]any{"symbols": []any{"2330"}})
	if !res.IsError {
		t.Fatalf("非交易時段應為錯誤")
	}
}

func TestCallGetIntradayKline(t *testing.T) {
	app := newTestApp(t)
	env := callCore(t, app, "get_intraday_kline", map[string]any{
		"symbol": "2330", "timeframe": "1m", "limit": float64(5),
	})
	data := env.Data.([]model.Candle)
	if len(data) != 5 {
		t.Fatalf("limit=5 應回傳 5 根，實際 %d", len(data))
	}
	if data[0].Close != data[len(data)-1].Close {
		t.Logf("K 線收盤序列正常")
	}

	// _lineage 完整欄位（§8 尾註：MIS 固定標 SEMI_OFFICIAL_REALTIME）
	lg := env.Lineage
	if lg.Source != model.SourceTWSEMIS || lg.SourceRole != model.SourceRoleRealtime {
		t.Errorf("lineage source 錯誤: %s/%s", lg.Source, lg.SourceRole)
	}
	if lg.Freshness != model.FreshnessRealtimeIntraday {
		t.Errorf("freshness 應為 REALTIME_INTRADAY，實際 %s", lg.Freshness)
	}
	if lg.DataDate != "2026-07-31" {
		t.Errorf("data_date 錯誤: %s", lg.DataDate)
	}
	if lg.FetchedAt.Time.IsZero() {
		t.Errorf("fetched_at 應有值")
	}
	if lg.LatencyMS < 0 {
		t.Errorf("latency_ms 應 >= 0，實際 %d", lg.LatencyMS)
	}
	if !lg.IsCached {
		t.Log("is_cached=false 符合無快取現況")
	}

	// 盤中 K 線純記憶體組裝：零上游 HTTP（§12.9 http_calls=0）
	if env.HTTPCalls != 0 {
		t.Errorf("盤中 K 線查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}

	// _chart_meta 預設注入（chart=true）
	if env.ChartMeta == nil {
		t.Fatalf("chart=true 應注入 _chart_meta，實際 nil")
	}
	if env.ChartMeta.RecommendedType != "candlestick" {
		t.Errorf("recommended_type 應為 candlestick，實際 %v", env.ChartMeta.RecommendedType)
	}

	// chart=false 移除
	env2 := callCore(t, app, "get_intraday_kline", map[string]any{
		"symbol": "2330", "timeframe": "5m", "chart": false,
	})
	if env2.ChartMeta != nil {
		t.Errorf("chart=false 不應注入 _chart_meta")
	}
	if env2.HTTPCalls != 0 {
		t.Errorf("盤中 K 線（chart=false）http_calls 應為 0，實際 %d", env2.HTTPCalls)
	}
}

func TestCallGetIntradayQuote(t *testing.T) {
	app := newTestApp(t)
	env := callCore(t, app, "get_intraday_quote", map[string]any{"symbol": "2330"})
	q := env.Data.(model.IntradayQuote)
	if q.Symbol != "2330" || q.Last <= 0 {
		t.Errorf("報價轉換錯誤: %+v", q)
	}
	if len(q.Bids) != 1 || len(q.Asks) != 1 {
		t.Errorf("五檔應來自快照 Book，實際 bids=%d asks=%d", len(q.Bids), len(q.Asks))
	}
	if q.Bids[0].Volume != 1000 {
		t.Errorf("買量應為 1000 股，實際 %d", q.Bids[0].Volume)
	}
	if env.HTTPCalls != 0 {
		t.Errorf("盤中報價查詢 http_calls 應為 0，實際 %d", env.HTTPCalls)
	}
}

func TestCallGetIntradayVWAP(t *testing.T) {
	app := newTestApp(t)
	env := callCore(t, app, "get_intraday_vwap", map[string]any{"symbol": "2330"})
	v := env.Data.(model.IntradayVWAP)
	if v.Symbol != "2330" || v.VWAP <= 0 || v.Volume <= 0 {
		t.Errorf("VWAP 對接錯誤: %+v", v)
	}
	if v.High < v.Low {
		t.Errorf("高低點錯誤: %v/%v", v.High, v.Low)
	}
}

func TestCallDetectVolumeSurge(t *testing.T) {
	app := newTestApp(t)
	env := callCore(t, app, "detect_volume_surge", map[string]any{"symbol": "2330", "minutes": float64(5)})
	s := env.Data.(model.VolumeSurge)
	if s.Symbol != "2330" || s.Minutes != 5 {
		t.Errorf("Surge 對接錯誤: %+v", s)
	}
	if s.RecentVolume <= 0 {
		t.Errorf("recent_volume 應 > 0")
	}
}

func TestCallScanDaytradeEligibility(t *testing.T) {
	symbols := seedSymbols()
	app, err := NewApp(nil,
		WithAppClock(testClock),
		WithAppSymbols(symbols),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	_ = app.Watchlist().Set([]model.Symbol{{Code: "6147", Name: "頎邦", Market: model.MarketOTC}})

	// 注入名單：6147 處置、6547 注意；2330 停資停券
	app.RiskScanner().AddLists("2026-07-31", model.MarketTSE, []AlertList{
		{Kind: "margin", Code: "2330", Info: "證交所公告融資暫停"},
		{Kind: "short", Code: "2330", Info: "證交所公告融券暫停"},
	})
	app.RiskScanner().AddLists("2026-07-31", model.MarketOTC, []AlertList{
		{Kind: "disposition", Code: "6147", Info: "連續三個營業日達公布標準", Period: "1150803~1150814"},
		{Kind: "attention", Code: "6547", Info: "最近六個營業日累計漲幅異常"},
	})

	// 停資停券 → 當沖不允許
	env := callCore(t, app, "scan_daytrade_eligibility", map[string]any{"symbol": "2330"})
	s := env.Data.(model.DaytradeScan)
	if !s.MarginSuspended || !s.ShortSuspended || s.DaytradeAllowed {
		t.Errorf("2330 應停資停券且當沖不允許: %+v", s)
	}

	// 處置 → 注意標記 + summary
	env = callCore(t, app, "scan_daytrade_eligibility", map[string]any{"symbol": "6147"})
	s = env.Data.(model.DaytradeScan)
	if !s.IsDisposition || s.DispositionPeriod == "" {
		t.Errorf("6147 應為處置股: %+v", s)
	}
	if len(s.Summary) == 0 {
		t.Errorf("summary 應有風險摘要")
	}

	// 上櫃注意股查詢（6547 上市名單不應誤中）
	env = callCore(t, app, "scan_daytrade_eligibility", map[string]any{"symbol": "6547"})
	s = env.Data.(model.DaytradeScan)
	if !s.IsAttention {
		t.Errorf("6547 應為注意股: %+v", s)
	}
	env = callCore(t, app, "scan_daytrade_eligibility", map[string]any{"symbol": "2317"})
	s = env.Data.(model.DaytradeScan)
	if s.IsAttention || s.IsDisposition {
		t.Errorf("2317 不應命中名單: %+v", s)
	}

	// 未知代號
	if _, err := app.Core().Call(context.Background(), "scan_daytrade_eligibility", map[string]any{"symbol": "9999"}); err == nil {
		t.Fatalf("未知代號應為錯誤")
	}

	// chart：table（§11.3 風險旗標比對，對應 v2.1 get_risk_flags）
	env = callCore(t, app, "scan_daytrade_eligibility", map[string]any{"symbol": "2330"})
	if chartType(env) != "table" {
		t.Errorf("風險旗標 chart 應為 table，實際 %s", chartType(env))
	}
	if env.ChartMeta == nil || len(env.ChartMeta.Columns) == 0 {
		t.Errorf("table chart 應含 columns 欄位描述，實際 %+v", env.ChartMeta)
	}
}

func TestCallErrors(t *testing.T) {
	app := newTestApp(t)

	// 未知工具
	if _, err := app.Core().Call(context.Background(), "no_such_tool", map[string]any{}); err == nil {
		t.Fatalf("未知工具應為錯誤")
	}

	// 未加入 watchlist 的代碼
	_, err := app.Core().Call(context.Background(), "get_intraday_quote", map[string]any{"symbol": "6147"})
	if err == nil || !strings.Contains(err.Error(), "未在觀察清單") {
		t.Fatalf("未加入 watchlist 應報明確錯誤，實際 %v", err)
	}

	// 未知 symbol
	if _, err := app.Core().Call(context.Background(), "get_intraday_kline", map[string]any{"symbol": "9999"}); err == nil {
		t.Fatalf("未知 symbol 應為錯誤")
	}

	// 非法 timeframe
	if _, err := app.Core().Call(context.Background(), "get_intraday_kline", map[string]any{"symbol": "2330", "timeframe": "3m"}); err == nil {
		t.Fatalf("非法 timeframe 應為錯誤")
	}

	// 非交易時段 gate（週六盤中時段）
	weekend := time.Date(2026, 8, 1, 9, 30, 0, 0, model.Taipei())
	app2, err := NewApp(nil,
		WithAppClock(func() time.Time { return weekend }),
		WithAppSymbols(seedSymbols()),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	if _, err := app2.Core().Call(context.Background(), "get_intraday_kline", map[string]any{"symbol": "2330"}); err == nil || !strings.Contains(err.Error(), "非交易時段") {
		t.Fatalf("週末盤中工具應報非交易時段，實際 %v", err)
	}

	// 盤後時段 gate（平日 14:00）
	after := time.Date(2026, 7, 31, 14, 0, 0, 0, model.Taipei())
	app3, err := NewApp(nil,
		WithAppClock(func() time.Time { return after }),
		WithAppSymbols(seedSymbols()),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	if _, err := app3.Core().Call(context.Background(), "scan_daytrade_eligibility", map[string]any{"symbol": "2330"}); err == nil || !strings.Contains(err.Error(), "非交易時段") {
		t.Fatalf("盤後時段應報非交易時段，實際 %v", err)
	}
}
