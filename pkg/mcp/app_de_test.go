package mcp

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"tw-quant-mcp/pkg/engine/composite"
	"tw-quant-mcp/pkg/model"
)

// app_de_test.go：§10.D/E 基本面・篩選・股利工具之整合測試（T014）。
// 以 fake fetcher 注入（驗證整批快取讀穿、過濾、lineage、chart meta、
// 邊界案例），不進行真實 HTTP。

// deApp 建立注入 fake fetcher 之 App（含 T014 測試用代碼）。
func deApp(t *testing.T, f *fakeFetch) *App {
	t.Helper()
	symbols := model.NewRegistry()
	_ = symbols.Set([]model.Symbol{
		{Code: "2330", Name: "台積電", Market: model.MarketTSE},
		{Code: "2317", Name: "鴻海", Market: model.MarketTSE},
		{Code: "1101", Name: "台泥", Market: model.MarketTSE},
		{Code: "1210", Name: "大成", Market: model.MarketTSE},
		{Code: "6147", Name: "頎邦", Market: model.MarketOTC},
		{Code: "6547", Name: "高端疫苗", Market: model.MarketOTC},
	})
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

// stubDE 建立 D/E 組工具測試之常用整批 stub（§12.4）。
func stubDE(f *fakeFetch) {
	// 上市估值（BWIBBU_ALL 之 Normalize 輸出；1101 虧損 pe=0）
	f.stub("valuation", nil, `[
		{"date":"2026-07-31","code":"1101","name":"台泥","pe":0,"dividend_yield":3.29,"pb":0.77},
		{"date":"2026-07-31","code":"2317","name":"鴻海","pe":12,"dividend_yield":3.5,"pb":1.5},
		{"date":"2026-07-31","code":"2330","name":"台積電","pe":20,"dividend_yield":2.1,"pb":5.2}]`)
	// 股利分派（t187ap45_L；2330 兩年度、1101 公積配息）
	f.stub("dividend", nil, `[
		{"table_date":"2026-07-31","code":"2330","name":"台積電","progress":"董事會決議","dividend_year":"115","cash_dividend":7.0,"stock_dividend":0,"cash_total":181500000000,"net_income":0,"retained":0},
		{"table_date":"2026-07-31","code":"2330","name":"台積電","progress":"董事會決議","dividend_year":"114","cash_dividend":6.0,"stock_dividend":0,"cash_total":155600000000,"net_income":0,"retained":0},
		{"table_date":"2026-07-31","code":"2317","name":"鴻海","progress":"董事會決議","dividend_year":"115","cash_dividend":7.2,"stock_dividend":0,"cash_total":0,"net_income":0,"retained":0},
		{"table_date":"2026-07-31","code":"2317","name":"鴻海","progress":"董事會決議","dividend_year":"114","cash_dividend":6.0,"stock_dividend":0,"cash_total":0,"net_income":0,"retained":0},
		{"table_date":"2026-07-31","code":"1101","name":"台泥","progress":"股東會確認","dividend_year":"114","cash_dividend":0.8,"stock_dividend":0,"cash_total":0,"net_income":0,"retained":0}]`)
	// 除權除息預告（TWT48U_ALL）
	f.stub("ex_div", nil, `[
		{"date":"2026-08-07","code":"1210","name":"大成","kind":"息","cash_dividend":3.0,"stock_ratio":0},
		{"date":"2026-07-30","code":"1231","name":"聯華食","kind":"權息","cash_dividend":1.5,"stock_ratio":0.1},
		{"date":"2026-08-10","code":"2330","name":"台積電","kind":"息","cash_dividend":7.0,"stock_ratio":0}]`)
	// 上櫃估值（TPEx peratio）
	f.stub("pe_valuation", nil, `[
		{"date":"2026-07-31","code":"6147","name":"頎邦","pe":15,"dividend_per_share":4.0,"yield_ratio":4.0,"price_book_ratio":2.0},
		{"date":"2026-07-31","code":"6547","name":"高端疫苗","pe":40,"dividend_per_share":0,"yield_ratio":0.5,"price_book_ratio":8.0}]`)
	// 上櫃除權除息
	f.stub("ex_rights", nil, `[
		{"date":"2026-08-10","code":"6147","name":"頎邦","kind":"除息","stock_dividend_ratio":0,"subscription_ratio":0,"subscription_price":0,"cash_dividend":4.0}]`)
	// ESG（topic=1..8，T037 雙來源）與公司治理
	f.stub("esg", urlValuesTopic(1), `[
		{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"範疇一排放量(噸CO2e)":"1234"}},
		{"report_date":"2026-07-31","year":"2025","code":"2317","name":"鴻海","fields":{"範疇一排放量(噸CO2e)":"5678"}}]`)
	for topic := 2; topic <= 21; topic++ {
		f.stub("esg", urlValuesTopic(topic), `[
			{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"指標":"topic`+strconv.Itoa(topic)+`"}}]`)
	}
	// ESG topic 9：公司治理資訊（T087）
	f.stub("esg", urlValuesTopic(9), `[
		{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"公司治理評估結果":"第六級"}}]`)
	// ESG topic 15：煉油廠（T065）
	f.stub("esg", urlValuesTopic(15), `[
		{"report_date":"2026-07-31","year":"2025","code":"6505","name":"台塑石化","fields":{"在人口密集地區的煉油廠數量(座)":"3"}},
		{"report_date":"2026-07-31","year":"2025","code":"1102","name":"亞泥","fields":{"在人口密集地區的煉油廠數量(座)":"0"}},
		{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"指標":"topic15"}}]`)
	for _, ds := range mopsESGDatasets {
		f.stub(string(ds), nil, `[
			{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"指標":"MOPS"}}]`)
	}
	f.stub("company_governance", nil, `[
		{"report_date":"2026-07-31","code":"2330","name":"台積電","rules":"訂有公司治理實務守則"}]`)
	// MOPS 損益表摘要（2330：2026Q1 + 2025Q4 + 2025Q1；1101：2026Q1）
	f.stub("income_summary", nil, `[
		{"table_date":"2026-07-31","year":2026,"quarter":1,"code":"2330","name":"台積電","industry":"半導體","eps":14.5,"par_value":"新台幣 10.0000元","revenue":1134103440000,"operating_profit":470000000000,"non_operating_items":-5000000000,"net_income":460000000000},
		{"table_date":"2026-04-30","year":2025,"quarter":4,"code":"2330","name":"台積電","industry":"半導體","eps":15.0,"par_value":"新台幣 10.0000元","revenue":1100000000000,"operating_profit":500000000000,"non_operating_items":0,"net_income":520000000000},
		{"table_date":"2026-04-30","year":2025,"quarter":1,"code":"2330","name":"台積電","industry":"半導體","eps":13.0,"par_value":"新台幣 10.0000元","revenue":1000000000000,"operating_profit":400000000000,"non_operating_items":0,"net_income":390000000000},
		{"table_date":"2026-07-31","year":2026,"quarter":1,"code":"1101","name":"台泥","industry":"水泥工業","eps":0.1,"par_value":"新台幣 10.0000元","revenue":33168148000,"operating_profit":2792191000,"non_operating_items":-659456000,"net_income":1204739000}]`)
	// MOPS 獲利能力（2330 2026Q1）
	f.stub("profit_ratios", nil, `[
		{"table_date":"2026-07-31","year":2026,"quarter":1,"code":"2330","name":"台積電","revenue_million":1134103440,"gross_margin_pct":59.0,"operating_margin_pct":41.4,"pretax_margin_pct":45.0,"net_margin_pct":40.6}]`)
	// MOPS 資產負債表 AJAX（2330 2026Q1）
	f.stub("balance_sheet", url.Values{"co_id": {"2330"}, "year": {"2026"}, "season": {"1"}}, `{
		"table_date":"2026-07-31","year":2026,"quarter":1,"total_assets":6600000000000,"current_assets":2500000000000,"non_current_assets":4100000000000,"total_liabilities":1500000000000,"current_liabilities":1000000000000,"non_current_liabilities":500000000000,"total_equity":5600000000000}`)
	// MOPS 資產負債表 AJAX（1101 2026Q1，供虧損公司 ROE 路徑）
	f.stub("balance_sheet", url.Values{"co_id": {"1101"}, "year": {"2026"}, "season": {"1"}}, `{
		"table_date":"2026-07-31","year":2026,"quarter":1,"total_assets":500000000000,"current_assets":200000000000,"non_current_assets":300000000000,"total_liabilities":250000000000,"current_liabilities":180000000000,"non_current_liabilities":70000000000,"total_equity":250000000000}`)
	// MOPS 現金流量表 AJAX（2330 2026Q1）
	f.stub("cash_flow", url.Values{"co_id": {"2330"}, "year": {"2026"}, "season": {"1"}}, `{
		"table_date":"2026-07-31","year":2026,"quarter":1,"operating_cash_flow":350000000000,"investing_cash_flow":-400000000000,"financing_cash_flow":-100000000000,"ending_cash_balance":2500000000000}`)
	// MOPS 現金流量表 AJAX（1101 2026Q1，供結構評分）
	f.stub("cash_flow", url.Values{"co_id": {"1101"}, "year": {"2026"}, "season": {"1"}}, `{
		"table_date":"2026-07-31","year":2026,"quarter":1,"operating_cash_flow":10000000000,"investing_cash_flow":-20000000000,"financing_cash_flow":5000000000,"ending_cash_balance":30000000000}`)
	// MOPS 損益表 AJAX（6147 2026Q2，供上櫃 ROE fallback）
	f.stub("income_statement", url.Values{"co_id": {"6147"}, "year": {"2026"}, "season": {"2"}}, `{
		"table_date":"2026-10-31","year":2026,"quarter":2,"revenue":3000000000,"operating_profit":1000000000,"non_operating_items":100000000,"net_income":800000000}`)
	// MOPS 資產負債表 AJAX（6147 2026Q2，供上櫃 ROE fallback）
	f.stub("balance_sheet", url.Values{"co_id": {"6147"}, "year": {"2026"}, "season": {"2"}}, `{
		"table_date":"2026-10-31","year":2026,"quarter":2,"total_assets":20000000000,"current_assets":10000000000,"non_current_assets":10000000000,"total_liabilities":8000000000,"current_liabilities":5000000000,"non_current_liabilities":3000000000,"total_equity":12000000000}`)
	// MOPS 月營收（YoY）
	f.stub("monthly_revenue", nil, `[
		{"table_date":"2026-07-10","data_year_month":"202606","code":"2330","name":"台積電","industry":"半導體","revenue":300000000000,"last_month_revenue":280000000000,"last_year_revenue":250000000000,"mom_change_pct":7.1,"yoy_change_pct":20.0,"cum_revenue":1700000000000,"cum_last_year":1500000000000,"cum_change_pct":13.3},
		{"table_date":"2026-07-10","data_year_month":"202606","code":"2317","name":"鴻海","industry":"其他電子","revenue":600000000000,"last_month_revenue":580000000000,"last_year_revenue":555000000000,"mom_change_pct":3.4,"yoy_change_pct":8.0,"cum_revenue":3400000000000,"cum_last_year":3300000000000,"cum_change_pct":3.0},
		{"table_date":"2026-07-10","data_year_month":"202606","code":"1101","name":"台泥","industry":"水泥工業","revenue":15000000000,"last_month_revenue":16000000000,"last_year_revenue":15800000000,"mom_change_pct":-6.2,"yoy_change_pct":-5.0,"cum_revenue":90000000000,"cum_last_year":95000000000,"cum_change_pct":-5.3},
		{"table_date":"2026-07-10","data_year_month":"202606","code":"6147","name":"頎邦","industry":"半導體","revenue":3000000000,"last_month_revenue":2900000000,"last_year_revenue":2700000000,"mom_change_pct":3.4,"yoy_change_pct":11.0,"cum_revenue":18000000000,"cum_last_year":16000000000,"cum_change_pct":12.5}]`)
	// MOPS 公司基本資料
	f.stub("company_profile", nil, `[
		{"table_date":"2026-07-31","code":"2330","name":"台灣積體電路製造股份有限公司","short_name":"台積電","foreign_reg":"","industry":"半導體","address":"新竹科學園區力行六路8號","tax_id":"22099131","chairman":"魏哲家","president":"魏哲家","spokesman":"黃仁昭","spokesman_title":"資深副總經理","deputy_spokesman":"","phone":"03-5636688","established":"1987-02-21","listed":"1994-09-05","par_value":"新台幣 10.0000元","capital":259303805000,"private_stock":0,"preferred_stock":0,"fin_report_type":"","transfer_agent":"","transfer_phone":"","transfer_address":"","auditor_firm":"","auditor_1":"","auditor_2":"","english_name":"Taiwan Semiconductor Manufacturing Company","english_address":"","fax":"","email":"","website":"https://www.tsmc.com","shares_outstanding":25930380500}]`)
}

// ************** D. 基本面 **************

func TestDEGetFinancialStatements(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_financial_statements", map[string]any{"symbol": "2330", "period": "2026Q1"})
	fs, ok := env.Data.(model.FinancialStatements)
	if !ok {
		t.Fatalf("Data 應為 FinancialStatements，實際 %T", env.Data)
	}
	if fs.Symbol != "2330" || fs.Year != 2026 || fs.Quarter != 1 {
		t.Errorf("期間錯誤: %+v", fs)
	}
	if len(fs.Income) != 1 || fs.Income[0].NetIncome != 460000000000 {
		t.Errorf("損益表摘要錯誤: %+v", fs.Income)
	}
	if len(fs.ProfitRatios) != 1 || fs.ProfitRatios[0].NetMargin != 40.6 {
		t.Errorf("獲利能力錯誤: %+v", fs.ProfitRatios)
	}
	if fs.BalanceSheet == nil || fs.BalanceSheet.TotalEquity != 5600000000000 {
		t.Errorf("資產負債表錯誤: %+v", fs.BalanceSheet)
	}
	if fs.CashFlow == nil || fs.CashFlow.OperatingCashFlow != 350000000000 {
		t.Errorf("現金流量表錯誤: %+v", fs.CashFlow)
	}
	if env.Lineage.Source != model.SourceMOPS || env.Lineage.DataDate == "" {
		t.Errorf("lineage 不符: %+v", env.Lineage)
	}
}

func TestDEGetFinancialStatementsStatementFilter(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_financial_statements",
		map[string]any{"symbol": "2330", "period": "2026Q1", "statement": "balance"})
	fs := env.Data.(model.FinancialStatements)
	if fs.BalanceSheet == nil {
		t.Error("statement=balance 應含資產負債表")
	}
	if len(fs.ProfitRatios) != 0 {
		t.Error("statement=balance 不應含獲利能力")
	}
	if fs.CashFlow != nil {
		t.Error("statement=balance 不應含現金流量表")
	}
	// 單一報表查詢不觸發其他 AJAX（呼叫次數驗證）
	if n := f.called("cash_flow", url.Values{"co_id": {"2330"}, "year": {"2026"}, "season": {"1"}}); n != 0 {
		t.Errorf("statement=balance 不應請求 cash_flow，實際 %d 次", n)
	}
}

func TestDEGetFinancialStatementsNoPeriod(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	// period 省略 → 最新一季（2026Q1）
	env := callEnv(t, app, "get_financial_statements", map[string]any{"symbol": "2330"})
	fs := env.Data.(model.FinancialStatements)
	if fs.Year != 2026 || fs.Quarter != 1 {
		t.Errorf("省略 period 應取最新一季，實際 %dQ%d", fs.Year, fs.Quarter)
	}
}

// 邊界：財報缺期（資料未釋出）
func TestDEGetFinancialStatementsMissingPeriod(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	if _, err := app.core.Call(context.Background(), "get_financial_statements",
		map[string]any{"symbol": "2330", "period": "2026Q3"}); err == nil {
		t.Fatal("無資料期間應回明確錯誤")
	}
}

func TestDEGetMonthlyRevenue(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_monthly_revenue", map[string]any{"symbol": "2330"})
	mb, ok := env.Data.(model.MonthlyRevenueBundle)
	if !ok {
		t.Fatalf("Data 應為 MonthlyRevenueBundle，實際 %T", env.Data)
	}
	if mb.Symbol != "2330" || len(mb.Rows) != 1 || mb.Rows[0].YoYChange != 20.0 {
		t.Errorf("月營收錯誤: %+v", mb)
	}
	if chartType(env) != "bar" {
		t.Errorf("月營收 chart 應為 bar，實際 %s", chartType(env))
	}
	// 二次查詢命中快取（整批共用鍵）
	callEnv(t, app, "get_monthly_revenue", map[string]any{"symbol": "2330"})
	if n := f.called("monthly_revenue", nil); n != 1 {
		t.Errorf("monthly_revenue 應僅請求 1 次（快取共用），實際 %d", n)
	}
}

func TestDEGetValuationRatiosTSE(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_valuation_ratios", map[string]any{"symbol": "2330"})
	v, ok := env.Data.(model.ValuationRatios)
	if !ok {
		t.Fatalf("Data 應為 ValuationRatios，實際 %T", env.Data)
	}
	if v.Symbol != "2330" || v.Market != model.MarketTSE || v.Date != "2026-07-31" {
		t.Errorf("估值基本欄位錯誤: %+v", v)
	}
	if v.PE != 20 || !v.PEAvailable || v.PB != 5.2 || v.DividendYield != 2.1 {
		t.Errorf("估值指標錯誤: %+v", v)
	}
	if v.DividendPerShare != 7.0 {
		t.Errorf("每股股利應為最新年度 115 之 7.0，實際 %v", v.DividendPerShare)
	}
	// ROE = 4600億 ×4/1 ÷ 5.6兆 = 32.86%
	if v.ROE < 32 || v.ROE > 34 {
		t.Errorf("ROE 年化估計錯誤: %v（%s）", v.ROE, v.ROEMethod)
	}
	if env.Lineage.Source != model.SourceTWSEAPI {
		t.Errorf("lineage source 應為 TWSE_API，實際 %s", env.Lineage.Source)
	}
}

func TestDEGetValuationRatiosOTC(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_valuation_ratios", map[string]any{"symbol": "6147"})
	v := env.Data.(model.ValuationRatios)
	if v.Market != model.MarketOTC || v.PE != 15 || v.DividendYield != 4.0 || v.DividendPerShare != 4.0 {
		t.Errorf("上櫃估值錯誤: %+v", v)
	}
	if !v.PEAvailable {
		t.Error("6147 有本益比，pe_available 應為 true")
	}
	if env.Lineage.Source != model.SourceTPExAPI {
		t.Errorf("上櫃 lineage 應為 TPEX_API，實際 %s", env.Lineage.Source)
	}
}

// 虧損公司：PE 不適用
func TestDEGetValuationRatiosLossMaker(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_valuation_ratios", map[string]any{"symbol": "1101"})
	v := env.Data.(model.ValuationRatios)
	if v.PE != 0 || v.PEAvailable {
		t.Errorf("虧損公司 pe_available 應為 false: %+v", v)
	}
	if v.DividendYield != 3.29 || v.PB != 0.77 {
		t.Errorf("台泥估值錯誤: %+v", v)
	}
}

func TestDEGetESGReport(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	esg, ok := env.Data.(model.ESGReport)
	if !ok {
		t.Fatalf("Data 應為 ESGReport，實際 %T", env.Data)
	}
	// T037：8 主題＋治理規程
	if len(esg.Topics) != 9 {
		t.Fatalf("應有 9 個題材（8 主題＋治理規程），實際 %d", len(esg.Topics))
	}
	if esg.Topics[0].Topic != "溫室氣體排放" || esg.Topics[0].Year != "2025" {
		t.Errorf("排放題材錯誤: %+v", esg.Topics[0])
	}
	if esg.Topics[len(esg.Topics)-1].Topic != "公司治理規程" ||
		esg.Topics[len(esg.Topics)-1].Fields["公司治理之相關規程規則"] == "" {
		t.Errorf("治理規程題材錯誤: %+v", esg.Topics[len(esg.Topics)-1])
	}
	if env.Lineage.Source != model.SourceTWSEAPI {
		t.Errorf("lineage source 應為 TWSE_API（平手勝出），實際 %q", env.Lineage.Source)
	}
}

func TestDEGetCompanyProfile(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_company_profile", map[string]any{"symbol": "2330"})
	cp, ok := env.Data.(model.CompanyProfile)
	if !ok {
		t.Fatalf("Data 應為 CompanyProfile，實際 %T", env.Data)
	}
	if cp.Code != "2330" || cp.Chairman != "魏哲家" || cp.Industry != "半導體" {
		t.Errorf("公司資料錯誤: %+v", cp)
	}
	if env.Lineage.Source != model.SourceMOPS {
		t.Errorf("lineage source 應為 MOPS: %+v", env.Lineage)
	}
}

// ************** D. 篩選 **************

func TestDEScreenStocks(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "screen_stocks", map[string]any{
		"max_pe": 15, "max_pb": 2.0, "min_yield": 3.0, "min_growth": 5.0,
	})
	sr, ok := env.Data.(model.ScreenResult)
	if !ok {
		t.Fatalf("Data 應為 ScreenResult，實際 %T", env.Data)
	}
	// 2317：PE 12、PB 1.5、殖利率 3.5、成長 8 → 命中；6147：PE 15、PB 2.0、殖利率 4.0、成長 11 → 命中
	// （頎邦恰好壓線；2317 本益比較低故排序在前）；1101 虧損無 PE、2330 PE 20 過高、ETF 排除
	if sr.Matched != 2 || len(sr.Rows) != 2 {
		t.Fatalf("應命中 2317、6147，實際 %+v", sr)
	}
	if sr.Rows[0].Code != "2317" || sr.Rows[1].Code != "6147" {
		t.Errorf("應依本益比遞增排序 2317→6147，實際 %s→%s",
			sr.Rows[0].Code, sr.Rows[1].Code)
	}
	r := sr.Rows[0]
	if r.PE != 12 || r.DividendYield != 3.5 || r.RevenueGrowth != 8.0 {
		t.Errorf("命中列指標錯誤: %+v", r)
	}
	if len(r.Matched) != 4 {
		t.Errorf("應命中 4 條件，實際 %v", r.Matched)
	}
	if env.Lineage.Source != model.SourceTWSEAPI || len(env.Lineage.DerivedFrom) == 0 {
		t.Errorf("lineage 不符: %+v", env.Lineage)
	}
	if chartType(env) != "scatter" {
		t.Errorf("篩選 chart 應為 scatter，實際 %s", chartType(env))
	}
}

// 邊界：篩選無結果
func TestDEScreenStocksNoMatch(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "screen_stocks", map[string]any{"max_pe": 5})
	sr := env.Data.(model.ScreenResult)
	if sr.Matched != 0 || len(sr.Rows) != 0 {
		t.Fatalf("無命中應為空，實際 %+v", sr)
	}
	if sr.Total == 0 {
		t.Error("Total 應為全市場候選數")
	}
}

func TestDEScreenStocksRequireESG(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	// require_esg：僅具 ESG 揭露之 2330、2317 可通過；1101 無揭露排除，
	// 上櫃 6147/6547 無揭露亦排除
	env := callEnv(t, app, "screen_stocks", map[string]any{
		"max_pe": 30, "max_pb": 10, "require_esg": true,
	})
	sr := env.Data.(model.ScreenResult)
	for _, r := range sr.Rows {
		if r.Code != "2330" && r.Code != "2317" {
			t.Errorf("require_esg 後不應出現 %s（無 ESG 揭露）", r.Code)
		}
	}
	if len(sr.Rows) == 0 {
		t.Error("require_esg 應仍命中具揭露之 2330/2317")
	}
}

// 獲利成長（淨利 YoY）：僅 2330 有去年同期財報（2026Q1 vs 2025Q1，+17.9%）
func TestDEScreenStocksProfitGrowth(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "screen_stocks", map[string]any{
		"min_profit_growth": 10,
	})
	sr := env.Data.(model.ScreenResult)
	if len(sr.Rows) != 1 || sr.Rows[0].Code != "2330" {
		t.Fatalf("MinProfitGrowth 10 應僅命中 2330（+17.9%%），實際 %+v", sr)
	}
	r := sr.Rows[0]
	if r.ProfitGrowth < 17.8 || r.ProfitGrowth > 18.0 {
		t.Errorf("profit_growth_pct 應為 17.9，實際 %v", r.ProfitGrowth)
	}
	matched := false
	for _, m := range r.Matched {
		if m == "獲利成長" {
			matched = true
		}
	}
	if !matched {
		t.Errorf("應標記獲利成長條件，實際 %v", r.Matched)
	}
	// lineage：獲利成長條件引入 income_summary 父資料集
	hasIncome := false
	for _, d := range env.Lineage.DerivedFrom {
		if d == "MOPS:income_summary" {
			hasIncome = true
		}
	}
	if !hasIncome {
		t.Errorf("derived_from 應含 MOPS:income_summary，實際 %v", env.Lineage.DerivedFrom)
	}
}

// ************** E. 股利 **************

func TestDEGetDividendHistory(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_dividend_history", map[string]any{"symbol": "2330"})
	dh, ok := env.Data.(model.DividendHistory)
	if !ok {
		t.Fatalf("Data 應為 DividendHistory，實際 %T", env.Data)
	}
	if dh.Symbol != "2330" || dh.TotalYears != 2 {
		t.Errorf("股利歷史錯誤: %+v", dh)
	}
	if dh.Years[0].DividendYear != "115" || dh.Years[0].CashDividend != 7.0 {
		t.Errorf("最新年度應為 115（7.0），實際 %+v", dh.Years[0])
	}
	if dh.ConsecutiveYears != 2 || dh.AvgCashDividend != 6.5 {
		t.Errorf("穩定性錯誤: %d/%v", dh.ConsecutiveYears, dh.AvgCashDividend)
	}
	if dh.LastYield != 2.1 {
		t.Errorf("最新殖利率錯誤: %v", dh.LastYield)
	}
	if chartType(env) != "bar" {
		t.Errorf("股利歷史 chart 應為 bar，實際 %s", chartType(env))
	}
}

// 邊界：股利為 0（上櫃不分派公司）
func TestDEGetDividendHistoryZeroDividend(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_dividend_history", map[string]any{"symbol": "6547"})
	dh := env.Data.(model.DividendHistory)
	if dh.TotalYears != 1 || dh.Years[0].CashDividend != 0 {
		t.Errorf("不分派公司應有 1 年度且股利 0: %+v", dh)
	}
	if dh.ConsecutiveYears != 0 {
		t.Errorf("不分派公司連續配息應為 0，實際 %d", dh.ConsecutiveYears)
	}
	if chartType(env) != "bar" {
		t.Errorf("股利歷史 chart 應為 bar，實際 %s", chartType(env))
	}
}

// TSE 股利歷史 ex_date 來源 TWT48U 行事曆
func TestDEGetDividendHistoryExDateTSE(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_dividend_history", map[string]any{"symbol": "2330"})
	dh := env.Data.(model.DividendHistory)
	if dh.TotalYears != 2 {
		t.Fatalf("應有 2 年度股利，實際 %d", dh.TotalYears)
	}
	// 115 年度應有 ex_date（TWT48U stub 有 2026-08-10 2330 除息）
	found115 := false
	for _, y := range dh.Years {
		if y.DividendYear == "115" {
			found115 = true
			if y.ExDate == "" {
				t.Errorf("115 年度應有 ex_date，實際為空")
			} else if y.ExDate != "2026-08-10" {
				t.Errorf("115 年度 ex_date 應為 2026-08-10，實際 %s", y.ExDate)
			}
		}
	}
	if !found115 {
		t.Error("應找到 115 年度股利")
	}
}

// OTC 股利歷史 ex_date 來源 TPEx ex_rights
func TestDEGetDividendHistoryExDateOTC(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_dividend_history", map[string]any{"symbol": "6147"})
	dh := env.Data.(model.DividendHistory)
	if dh.TotalYears != 1 {
		t.Fatalf("OTC 應有 1 年度股利，實際 %d", dh.TotalYears)
	}
	// "最新" 年度對應 115 年（2026），ex_rights stub 有 2026-08-10 6147 除息
	if dh.Years[0].ExDate == "" {
		t.Error("OTC 最新年度應有 ex_date")
	} else if dh.Years[0].ExDate != "2026-08-10" {
		t.Errorf("OTC 最新年度 ex_date 應為 2026-08-10，實際 %s", dh.Years[0].ExDate)
	}
	if dh.Note == "" || !strings.Contains(dh.Note, "ex_date") {
		t.Errorf("Note 應說明 ex_date 來源: %s", dh.Note)
	}
}

// 股利年份與行事曆對不上時 ex_date 為空
func TestDEGetDividendHistoryExDateMissing(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	// 1101 在 TWT48U stub 中無除息事件
	env := callEnv(t, app, "get_dividend_history", map[string]any{"symbol": "1101"})
	dh := env.Data.(model.DividendHistory)
	if dh.TotalYears != 1 {
		t.Fatalf("1101 應有 1 年度股利，實際 %d", dh.TotalYears)
	}
	if dh.Years[0].ExDate != "" {
		t.Errorf("1101 無 TWT48U 除息事件，ex_date 應為空，實際 %s", dh.Years[0].ExDate)
	}
}

func TestDEGetExdividendCalendar(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_exdividend_calendar",
		map[string]any{"start": "2026-08-01", "end": "2026-08-31"})
	cal, ok := env.Data.(model.ExDivCalendar)
	if !ok {
		t.Fatalf("Data 應為 ExDivCalendar，實際 %T", env.Data)
	}
	// 08-07 大成（上市）+ 08-10 台積電（上市）+ 08-10 頎邦（上櫃）；07-30 聯華食在範圍外
	if len(cal.Events) != 3 {
		t.Fatalf("應命中 3 事件，實際 %+v", cal.Events)
	}
	if cal.Events[0].Code != "1210" || cal.Events[0].Date != "2026-08-07" || cal.Events[0].Market != model.MarketTSE {
		t.Errorf("首事件應為大成 08-07: %+v", cal.Events[0])
	}
	// 08-10 有兩筆：台積電(上市) + 頎邦(上櫃)
	found2330, found6147 := false, false
	for _, e := range cal.Events {
		if e.Code == "2330" && e.Date == "2026-08-10" && e.Market == model.MarketTSE {
			found2330 = true
		}
		if e.Code == "6147" && e.Date == "2026-08-10" && e.Market == model.MarketOTC {
			found6147 = true
		}
	}
	if !found2330 {
		t.Error("應包含台積電 08-10 事件")
	}
	if !found6147 {
		t.Error("應包含頎邦 08-10 事件")
	}
	// chart：table（§11.3 除權息行事曆）
	if chartType(env) != "table" {
		t.Errorf("除權息行事曆 chart 應為 table，實際 %s", chartType(env))
	}
	if env.ChartMeta == nil || len(env.ChartMeta.Columns) == 0 {
		t.Errorf("table chart 應含 columns 欄位描述，實際 %+v", env.ChartMeta)
	}
}

// 邊界：範圍內無事件 → 空清單（非錯誤）
func TestDEGetExdividendCalendarEmpty(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_exdividend_calendar",
		map[string]any{"start": "2027-01-01", "end": "2027-01-31"})
	cal := env.Data.(model.ExDivCalendar)
	if len(cal.Events) != 0 {
		t.Fatalf("範圍內應無事件，實際 %+v", cal.Events)
	}
	if cal.RangeStart != "2027-01-01" {
		t.Errorf("範圍標記錯誤: %+v", cal)
	}
}

func TestDEScreenHighYield(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "screen_high_yield", map[string]any{"min_yield": 3.0})
	sr, ok := env.Data.(model.ScreenResult)
	if !ok {
		t.Fatalf("Data 應為 ScreenResult，實際 %T", env.Data)
	}
	// 殖利率 ≥3：2317（3.5，股利 7.2）、1101（3.29，股利 0.8）、6147（4.0，股利 4.0）
	// 依殖利率遞減：6147(4.0) → 2317(3.5) → 1101(3.29)
	if sr.Matched != 3 || len(sr.Rows) != 3 {
		t.Fatalf("應命中 3 列，實際 %+v", sr)
	}
	if sr.Rows[0].Code != "6147" || sr.Rows[1].Code != "2317" || sr.Rows[2].Code != "1101" {
		t.Errorf("殖利率遞減排序錯誤: %s/%s/%s",
			sr.Rows[0].Code, sr.Rows[1].Code, sr.Rows[2].Code)
	}
	// 每股股利門檻：2317（7.2）、6147（4.0）；1101（0.8）排除
	env = callEnv(t, app, "screen_high_yield", map[string]any{"min_yield": 3.0, "min_dividend": 4.0})
	sr = env.Data.(model.ScreenResult)
	if sr.Matched != 2 {
		t.Fatalf("min_dividend=4 應命中 2 列，實際 %+v", sr)
	}
	// 本益比上限：1101 虧損（無 PE）排除
	env = callEnv(t, app, "screen_high_yield", map[string]any{"min_yield": 3.0, "max_pe": 20})
	sr = env.Data.(model.ScreenResult)
	for _, r := range sr.Rows {
		if r.Code == "1101" {
			t.Errorf("虧損公司不應命中 max_pe 條件: %+v", r)
		}
	}
	// 配息穩定性（min_consecutive=2）：僅 2317 連年配息 2 年通過（1101/6147 為 1 年）
	env = callEnv(t, app, "screen_high_yield",
		map[string]any{"min_yield": 3.0, "min_consecutive": 2, "market": "tse"})
	sr = env.Data.(model.ScreenResult)
	if sr.Matched != 1 || len(sr.Rows) != 1 || sr.Rows[0].Code != "2317" {
		t.Fatalf("min_consecutive=2 應僅命中 2317，實際 %+v", sr)
	}
	if sr.Rows[0].ConsecutiveYears != 2 {
		t.Errorf("連年配息年數錯誤: %+v", sr.Rows[0])
	}
	if chartType(env) != "scatter" {
		t.Errorf("高殖利率 chart 應為 scatter，實際 %s", chartType(env))
	}
}

// get_financial_health_check：T017 五面向評分（快取 raw 資料輸入，helper lineage）。
func TestDEGetFinancialHealthCheck(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_financial_health_check", map[string]any{"symbol": "2330"})
	s, ok := env.Data.(composite.HealthScore)
	if !ok {
		t.Fatalf("Data 應為 composite.HealthScore，實際 %T", env.Data)
	}
	if s.ScoringVersion != "v1" {
		t.Errorf("scoring_version 應為 v1，實際 %s", s.ScoringVersion)
	}
	// 獲利：毛利率 59%/營益率 41.4%/純益率 40.6% 皆 ≥ 門檻 → 100
	if !s.Profit.Available || s.Profit.Score != 100 {
		t.Errorf("獲利能力應為 100，實際 %+v", s.Profit)
	}
	// 成長：營收 YoY +13.4%（→67.1）、淨利 YoY +17.9%（→71.8）→ 平均 69.4
	if !s.Growth.Available || s.Growth.Score != 69.4 {
		t.Errorf("成長性應為 69.4，實際 %+v", s.Growth)
	}
	// 結構：負債比 22.7% → 100；營業現金流為正 → 加 5（上限 100）
	if !s.Structure.Available || s.Structure.Score != 100 {
		t.Errorf("財務結構應為 100，實際 %+v", s.Structure)
	}
	// 配息：連年 2 年 → 2/5×70=28；2/2 年度有配 → 30；合計 58（殖利率 2.1 < 5 無加分）
	if !s.Dividend.Available || s.Dividend.Score != 58 {
		t.Errorf("配息政策應為 58，實際 %+v", s.Dividend)
	}
	// 治理：ESG + 公司治理規程皆揭露 → 50+25+25=100
	if !s.Governance.Available || s.Governance.Score != 100 {
		t.Errorf("公司治理應為 100，實際 %+v", s.Governance)
	}
	// 總分：0.3×100 + 0.2×69.4 + 0.2×100 + 0.15×58 + 0.15×100 = 87.6
	if s.Total != 87.6 {
		t.Errorf("加權總分應為 87.6，實際 %v", s.Total)
	}
	// lineage：v2.1 無 helper 角色——聚合計算資料源為 MOPS（CANONICAL），
	// derived_from 標明所有父資料集（僅 debug/log 模式輸出）
	if env.Lineage.SourceRole != model.SourceRoleCanonical {
		t.Errorf("source_role 應為 CANONICAL，實際 %s", env.Lineage.SourceRole)
	}
	derived := strings.Join(env.Lineage.DerivedFrom, ",")
	for _, want := range []string{"MOPS:income_summary", "MOPS:profit_ratios",
		"MOPS:balance_sheet", "TWSE_API:dividend", "TWSE_API:esg", "TWSE_API:company_governance"} {
		if !strings.Contains(derived, want) {
			t.Errorf("derived_from 應含 %s，實際 %v", want, env.Lineage.DerivedFrom)
		}
	}
	// chart：radar（§11.3 財報五面向）
	if chartType(env) != "radar" {
		t.Errorf("財報體檢 chart 應為 radar，實際 %s", chartType(env))
	}
}

// 快取命中驗證（§12.4）：重複查詢對上游 Adapter 之呼叫次數 = 1。
func TestDEGetFinancialHealthCheckCacheHit(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	for i := 0; i < 2; i++ {
		callEnv(t, app, "get_financial_health_check", map[string]any{"symbol": "2330"})
	}
	for _, ds := range []string{"income_summary", "profit_ratios", "company_governance"} {
		if n := f.called(ds, nil); n != 1 {
			t.Errorf("整批資料集 %s 上游呼叫次數應為 1（快取命中），實際 %d", ds, n)
		}
	}
	if n := f.called("esg", url.Values{"topic": {"1"}}); n != 1 {
		t.Errorf("esg 上游呼叫次數應為 1（快取命中），實際 %d", n)
	}
	bs := url.Values{"co_id": {"2330"}, "year": {"2026"}, "season": {"1"}}
	if n := f.called("balance_sheet", bs); n != 1 {
		t.Errorf("資產負債表上游呼叫次數應為 1（快取命中），實際 %d", n)
	}
}

// 邊界：缺財報/ESG 資料之個股 → 該面向 0 分 + available=false + 註記（不臆測）。
func TestDEGetFinancialHealthCheckMissingData(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_financial_health_check", map[string]any{"symbol": "1101"})
	s, ok := env.Data.(composite.HealthScore)
	if !ok {
		t.Fatalf("Data 應為 composite.HealthScore，實際 %T", env.Data)
	}
	// 無獲利能力指標 stub → 不評分
	if s.Profit.Available || s.Profit.Score != 0 {
		t.Errorf("缺獲利能力資料應 available=false、0 分，實際 %+v", s.Profit)
	}
	// 無去年同期財報 → 成長性不評分（註記）
	if s.Growth.Available {
		t.Errorf("缺去年同期財報應不評分，實際 %+v", s.Growth)
	}
	if s.Growth.Note == "" {
		t.Error("成長性缺資料應有註記")
	}
	// 無 ESG/治理揭露 → 治理不評分
	if s.Governance.Available {
		t.Errorf("缺 ESG 揭露應不評分，實際 %+v", s.Governance)
	}
	// 有資產負債表 + 股利：仍正常評分
	if !s.Structure.Available || s.Structure.Score == 0 {
		t.Errorf("1101 有資產負債表，結構應評分，實際 %+v", s.Structure)
	}
	if !s.Dividend.Available || s.Dividend.Score == 0 {
		t.Errorf("1101 有股利資料，配息應評分，實際 %+v", s.Dividend)
	}
	if s.Total <= 0 {
		t.Errorf("總分應 > 0，實際 %v", s.Total)
	}
}
