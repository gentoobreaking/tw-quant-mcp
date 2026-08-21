package mcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// app_envelope_test.go：T019 驗收 #3 — Envelope 一致性測試。
//
// 對**所有已註冊 Tool**（A 6 + B/C 11 + D/E 10 + F/G 9 = 36）逐一呼叫，
// 驗證回傳 Envelope 之 `_lineage` 欄位齊全且語意正確（§3.2/§5）：
//   - source/source_role 非空且為登錄值
//   - data_date 為 YYYY-MM-DD
//   - freshness 為 §3.2 允許之三值之一
//   - fetched_at 非零（RFC3339）；latency_ms/cache_ttl ≥ 0
//   - data 非 nil；http_calls ≥ 0
//
// A 組盤中工具以 newTestApp（交易時段 09:30 + 快照種入）執行，
// 其餘以 fgApp（盤後 16:00 + fake 資料替身）執行。全程離線不連網
// （§13 測試策略：錄製回放，CI 不觸發 Rate Limit）。

// dateOnlyRE 為 §5.1 日期格式（YYYY-MM-DD）。
var dateOnlyRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// envelopeProbe 描述單一工具之最小呼叫（args 依各工具 schema 最小可行值）。
type envelopeProbe struct {
	name string
	args map[string]any
}

// intradayTools 為 A 組盤中（交易時段 gate）工具。
var intradayTools = map[string]bool{
	"set_active_watchlist":      true,
	"get_intraday_kline":        true,
	"get_intraday_quote":        true,
	"get_intraday_vwap":         true,
	"detect_volume_surge":       true,
	"scan_daytrade_eligibility": true,
}

// stubBCEnvelope 建立 B/C 組探針所需之 fake 資料（與既有 B/C 測試同款）。
func stubBCEnvelope(f *fakeFetch) {
	// get_etf_nav（§30.1：fundPric netPrice+atmps / close 市價）
	f.bodies["etf|0050|fundPric"] = `{"netPrice":[{"date":"2026/07/30","count":101.0},{"date":"2026/07/29","count":100.0}],"atmps":[{"date":"2026/07/30","count":0.15},{"date":"2026/07/29","count":-0.1}]}`
	f.bodies["etf|0050|close"] = `[{"date":"2026/07/30","count":101.15},{"date":"2026/07/29","count":99.9}]`
	// get_stock_daily_quote（TSE：3 個月日 K，2026-07-30 在最後月份）
	f.stub("daily_k", url.Values{"date": {"20260501"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "05", 0, 20)))
	f.stub("daily_k", url.Values{"date": {"20260601"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "06", 20, 20)))
	f.stub("daily_k", url.Values{"date": {"20260701"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 40, 30)))
	// get_stock_daily_kline（day 預設）
	f.stub("daily_k", url.Values{"date": {"20260730"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 0, 10)))
	// get_market_summary
	f.stub("market_close", url.Values{"date": {"20260730"}, "type": {"ALL"}}, `[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20}]`)
	f.stub("daily_close", url.Values{"date": {"20260730"}}, `[{"date":"2026-07-30","code":"6147","name":"頎邦","close":75.5,"change_dir":"+","change":1.2,"open":74.3,"high":76,"low":74.1,"volume":1200000}]`)
	// get_institutional_investors
	f.stub("institutional", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","foreign_buy":1000,"foreign_sell":400,"foreign_net":600,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":0,"investment_sell":0,"investment_net":0}]`)
	// get_stock_trend_composite 法人回溯（2026-07-29/28/27/24/23：short 5 日，跳過週末）
	for _, d := range []string{"20260729", "20260728", "20260727", "20260724", "20260723"} {
		f.stub("institutional", url.Values{"date": {d}},
			`[{"code":"2330","name":"台積電","foreign_buy":500,"foreign_sell":300,"foreign_net":200,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":100,"investment_sell":50,"investment_net":50}]`)
	}
	// get_foreign_industry_holdings
	f.stub("foreign_holdings", nil,
		`[{"industry":"半導體業","company_count":10,"share_number":1000,"foreign_share":500,"percentage":50.0}]`)
	// get_foreign_shareholding_history（range=3）
	for _, d := range []string{"20260730", "20260729", "20260728"} {
		f.stub("qfiis", url.Values{"dayDate": {d}},
			`[{"date":"`+d[:4]+`-`+d[4:6]+`-`+d[6:]+`","code":"2330","name":"台積電","issue_shares":25930389000,"foreign_shares":1000000,"foreign_percent":10.5,"upper_limit_pct":100.0,"change_reason":"","last_changed_date":""}]`)
	}
	// get_margin_trading（TSE）
	f.stub("margin", url.Values{"date": {"20260730"}, "selectType": {"ALL"}},
		`[{"code":"2330","name":"台積電","margin_buy":100000,"margin_sell":50000,"margin_cash_redeem":10000,"margin_prev_balance":1000000,"margin_balance":1040000,"margin_limit":2000000}]`)
	// get_abnormal_trading + get_attention_disposition_stocks（abnormal_volume/punish）
	f.stub("abnormal_volume", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","notice_count":2,"info":"連續三個營業日達注意標準","date":"2026-07-30","close":169,"pe":28}]`)
	f.stub("punish", url.Values{"date": {"20260730"}},
		`[{"number":"1","date":"1150722","code":"2317","name":"鴻海","notice_count":3,"reasons":"連續三次","disposition_period":"115/07/23～115/08/05","disposition_measure":"第一次處置","detail":"人工管制撮合"}]`)
	// get_warrant_activity
	f.stub("warrants", nil, `[{"trade_date":"2026-07-30","code":"052644","name":"台積電國票41購01","amount":5000000,"volume":100000}]`)
	// get_major_announcements
	f.stub("announcements", nil, `[
{"table_date":"2026-07-30","announce_date":"2026-07-30","announce_time":"18:30:00",
"code":"2330","name":"台積電","subject":"本公司董事會決議配發現金股利",
"clause":"第14款","fact_date":"2026-07-30","description":"每股配發新台幣8元"}]`)
	// get_twse_index (stub normalized JSON for fakeFetch)
	f.stub("indices", nil,
		`[{"date":"2026-07-30","index_name":"發行量加權股價指數","close":17000.0,"change":-50.0,"change_percent":-0.29,"change_dir":"-","note":""}]`)
	f.stub("index_history", url.Values{"date": {"20260701"}},
		`[{"date":"2026-07-01","open":17000.0,"high":17100.0,"low":16900.0,"close":17050.0},{"date":"2026-07-02","open":17050.0,"high":17150.0,"low":16950.0,"close":17100.0}]`)
}

// allToolProbes 為全部 40 個註冊工具之呼叫探針。
func allToolProbes() []envelopeProbe {
	return []envelopeProbe{
		// ── A 組（盤中，6；以 newTestApp 交易時段執行）──
		{name: "set_active_watchlist", args: map[string]any{"symbols": []any{"2330"}}},
		{name: "get_intraday_kline", args: map[string]any{"symbol": "2330", "timeframe": "1m", "limit": float64(5)}},
		{name: "get_intraday_quote", args: map[string]any{"symbol": "2330"}},
		{name: "get_intraday_vwap", args: map[string]any{"symbol": "2330"}},
		{name: "detect_volume_surge", args: map[string]any{"symbol": "2330", "minutes": float64(5)}},
		{name: "scan_daytrade_eligibility", args: map[string]any{"symbol": "2330"}},
		// ── B 組（盤後行情，6）──
		{name: "get_stock_daily_quote", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_stock_daily_kline", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_market_summary", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_abnormal_trading", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		{name: "get_warrant_activity", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_twse_index", args: map[string]any{"symbol": "發行量加權股價指數", "date": "2026-07-30"}},
		// ── ETF（§30.1，2）──
		{name: "get_etf_nav", args: map[string]any{"symbol": "0050"}},
		{name: "get_etf_dividend", args: map[string]any{"symbol": "0056"}},
		// ── C 組（籌碼/法人，6）──
		{name: "get_institutional_investors", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		{name: "get_foreign_industry_holdings", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_foreign_shareholding_history", args: map[string]any{"symbol": "2330", "range": float64(3), "date": "2026-07-30"}},
		{name: "get_margin_trading", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_major_announcements", args: map[string]any{}},
		{name: "get_attention_disposition_stocks", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		// ── T029 缺口工具（跨來源聚合，Grade PREVIEW）──
		{name: "get_stock_trend_composite", args: map[string]any{"symbol": "2330", "horizon": "short"}},
		// ── D 組（基本面，6）──
		{name: "get_financial_statements", args: map[string]any{"symbol": "2330", "period": "2026Q1"}},
		{name: "get_monthly_revenue", args: map[string]any{"symbol": "2330"}},
		{name: "get_financial_health_check", args: map[string]any{"symbol": "2330"}},
		{name: "get_valuation_ratios", args: map[string]any{"symbol": "2330"}},
		{name: "get_esg_report", args: map[string]any{"symbol": "2330"}},
		{name: "get_company_profile", args: map[string]any{"symbol": "2330"}},
		// ── E 組（篩選/股利，4）──
		{name: "screen_stocks", args: map[string]any{"min_eps": float64(1)}},
		{name: "get_dividend_history", args: map[string]any{"symbol": "2330"}},
		{name: "get_exdividend_calendar", args: map[string]any{}},
		{name: "screen_high_yield", args: map[string]any{"min_yield": float64(1)}},
		// ── F 組（期貨選擇權，7）──
		{name: "get_futures_daily_ohlc", args: map[string]any{"contract": "TX"}},
		{name: "get_futures_history", args: map[string]any{"contract": "TX", "start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_put_call_ratio", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_large_trader_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_institutional_futures_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_institutional_options_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_institutional_futures_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		// ── G 組（基礎設施，3）──
		{name: "get_symbol_list", args: map[string]any{}},
		{name: "get_trading_calendar", args: map[string]any{"year": float64(2026), "month": float64(2)}},
	}
}

// TestAllToolsEnvelopeConsistent 對全部 37 個註冊工具驗證 Envelope 一致性。
func TestAllToolsEnvelopeConsistent(t *testing.T) {
	// 盤後 App（B–G 工具）：fake 資料替身
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f) // D/E 資料
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq) // F/G 資料
	app := fgApp(t, f, tq)

	// 盤中 App（A 工具）：交易時段 + 快照 + watchlist
	intraday := newTestApp(t)

	names := intraday.Registry().Names()
	if len(names) != 40 {
		t.Fatalf("前置：應登錄 40 工具，實際 %d", len(names))
	}
	covered := map[string]bool{}
	for _, p := range allToolProbes() {
		covered[p.name] = true
	}
	for _, n := range names {
		if !covered[n] {
			t.Errorf("探針清單缺漏工具 %q（驗收要求覆蓋所有已註冊 Tool）", n)
		}
	}
	if len(covered) != 40 {
		t.Fatalf("探針應覆蓋 40 工具，實際 %d", len(covered))
	}

	for _, p := range allToolProbes() {
		p := p
		t.Run(p.name, func(t *testing.T) {
			target := app
			if intradayTools[p.name] {
				target = intraday
			}
			env, err := target.core.Call(context.Background(), p.name, p.args)
			if err != nil {
				t.Fatalf("Call 失敗: %v", err)
			}
			e, ok := env.(*model.Envelope)
			if !ok {
				t.Fatalf("回傳應為 *model.Envelope，實際 %T", env)
			}
			checkEnvelopeConsistency(t, p.name, e)
		})
	}
}

// checkEnvelopeConsistency 驗證單一 Envelope 之 _lineage 欄位齊全（§3.2/§5）。
func checkEnvelopeConsistency(t *testing.T, name string, e *model.Envelope) {
	t.Helper()
	// 附錄 A：所有回傳附加免責欄位（僅供研究參考，不構成投資建議）
	if e.Disclaimer != model.DisclaimerText {
		t.Errorf("%s: 缺免責欄位（附錄 A），實際 %q", name, e.Disclaimer)
	}
	// 多來源聚合工具（v2.1 §4 設計規則 2）：_lineage 為 []Lineage，逐一驗證
	//（lineage 陣列存在時，primary Lineage 僅為 first() 之檢視）。
	if len(e.Lineage.Multi) > 0 {
		for i, sub := range e.Lineage.Multi {
			checkLineageFields(t, fmt.Sprintf("%s[%d]", name, i), sub)
		}
	} else {
		checkLineageFields(t, name, e.Lineage.Lineage)
	}
	if e.Data == nil {
		t.Errorf("%s: data 不得為 nil", name)
	}
	if e.HTTPCalls < 0 {
		t.Errorf("%s: http_calls 應 ≥ 0，實際 %d", name, e.HTTPCalls)
	}
	if e.ChartMeta != nil && e.ChartMeta.RecommendedType == "" {
		t.Errorf("%s: _chart_meta.recommended_type 不得為空", name)
	}
}

// checkLineageFields 驗證單一 lineage 之必填語意欄位（§3.2 附錄 A）。
func checkLineageFields(t *testing.T, name string, lg model.Lineage) {
	t.Helper()
	if lg.Source == "" {
		t.Errorf("%s: source 不得為空", name)
	}
	switch lg.Source {
	case model.SourceTWSEAPI, model.SourceTWSEWeb, model.SourceTWSEMIS,
		model.SourceTPExAPI, model.SourceMOPS, model.SourceTAIFEXAPI, model.SourceTAIFEXDL:
	default:
		t.Errorf("%s: source 非登錄值 %q", name, lg.Source)
	}
	if lg.SourceRole == "" {
		t.Errorf("%s: source_role 不得為空", name)
	}
	switch lg.SourceRole {
	case model.SourceRoleCanonical, model.SourceRoleRealtime, model.SourceRoleFallback:
	default:
		t.Errorf("%s: source_role 非登錄值 %q", name, lg.SourceRole)
	}
	if !model.ValidFreshness(lg.Freshness) {
		t.Errorf("%s: freshness 非法 %q（v2.1 §4 僅允許五值）", name, lg.Freshness)
	}
	if lg.DataDate == "" {
		t.Errorf("%s: data_date 不得為空", name)
	} else if !dateOnlyRE.MatchString(lg.DataDate) {
		t.Errorf("%s: data_date 格式不符 YYYY-MM-DD: %q", name, lg.DataDate)
	}
	if lg.FetchedAt.Time.IsZero() {
		t.Errorf("%s: fetched_at 不得為零值", name)
	}
	if lg.LatencyMS < 0 {
		t.Errorf("%s: latency_ms 應 ≥ 0，實際 %d", name, lg.LatencyMS)
	}
	if lg.CacheTTL < 0 {
		t.Errorf("%s: cache_ttl 應 ≥ 0，實際 %d", name, lg.CacheTTL)
	}
	if lg.CacheAgeSec < 0 {
		t.Errorf("%s: cache_age_sec 應 ≥ 0，實際 %d", name, lg.CacheAgeSec)
	}
}

// TestIntradayToolsZeroHTTP 盤中 A 組工具之 Envelope 必須 http_calls=0
// （純記憶體組裝，§12.9 零 HTTP 驗收）。
func TestIntradayToolsZeroHTTP(t *testing.T) {
	app := newTestApp(t) // 已含快照種入
	for _, p := range []envelopeProbe{
		{name: "get_intraday_kline", args: map[string]any{"symbol": "2330", "timeframe": "1m"}},
		{name: "get_intraday_quote", args: map[string]any{"symbol": "2330"}},
		{name: "get_intraday_vwap", args: map[string]any{"symbol": "2330"}},
		{name: "detect_volume_surge", args: map[string]any{"symbol": "2330", "minutes": float64(5)}},
		{name: "scan_daytrade_eligibility", args: map[string]any{"symbol": "2330"}},
	} {
		p := p
		t.Run(p.name, func(t *testing.T) {
			env := callCore(t, app, p.name, p.args)
			if env.HTTPCalls != 0 {
				t.Errorf("%s: http_calls 應為 0（盤中純記憶體），實際 %d", p.name, env.HTTPCalls)
			}
			if !env.Lineage.FetchedAt.Time.Equal(testClock()) {
				t.Logf("%s: fetched_at=%s（testClock 09:30）", p.name, env.Lineage.FetchedAt.Format(time.RFC3339))
			}
		})
	}
}
