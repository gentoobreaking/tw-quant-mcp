package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/domain/fundamental"
	"tw-quant-mcp/pkg/domain/screener"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// tools_de.go 實作 §10.D（基本面與篩選）與 §10.E（股利）工具（T014）。
// 篩選類工具遵循 §12.4：整批以快取 + 記憶體計算，不逐股打上游。
// 五面向評分（get_financial_health_check）由 pkg/domain/fundamental 入口
// 委託 T017 composite engine 提供，本檔不實作評分邏輯。

// ************** 共用整批取得 helpers（§12.4） **************

// mopsRows 取得 MOPS Open Data CSV 全量資料（一次下載，快取鍵不含過濾參數，
// 各 symbol/過濾組合共用同一份快取，§12.4）。
func mopsRows[T any](a *App, ctx context.Context, ds provider.MOPSDataset) ([]T, bool, bool, error) {
	if a.mops == nil {
		return nil, false, false, fmt.Errorf("MOPS 資料源尚未接線")
	}
	dataDate := a.now().Format("2006-01-02")
	key := cache.KeyString(model.SourceMOPS, string(ds), dataDate, "", nil)
	cached, stale, raw, err := a.fetchRaw(ctx, string(ds), dataDate, key, func() ([]byte, error) {
		req := provider.RawRequest{URL: a.mops.URL(ds, nil)}
		resp, err := a.mops.Fetch(ctx, req)
		if err != nil {
			return nil, err
		}
		if err := a.mops.Validate(resp); err != nil {
			return nil, err
		}
		return a.mops.Normalize(resp)
	})
	if err != nil {
		return nil, false, false, fmt.Errorf("MOPS %s 取得失敗: %w", ds, err)
	}
	var rows []T
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, false, false, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	return rows, cached, stale, nil
}

// mopsStatement 取得 MOPS AJAX 財報三表（POST form-encoded，T012 discovery
// 規格：ajax_t164sb0{3|4|5}，year 為西元、season 1-4、isnew=true）。
func mopsStatement[T any](a *App, ctx context.Context, ds provider.MOPSDataset,
	code string, year, quarter int) (T, bool, bool, error) {
	var zero T
	if a.mops == nil {
		return zero, false, false, fmt.Errorf("MOPS 資料源尚未接線")
	}
	form := url.Values{
		"step": {"1"}, "firstin": {"1"}, "off": {"1"}, "TYPEK": {"all"},
		"co_id": {code}, "year": {fmt.Sprint(year)}, "season": {fmt.Sprint(quarter)},
		"isnew": {"true"}, "queryName": {"co_id"}, "inpuType": {"co_id"},
		"keyword4": {""}, "code1": {""}, "TYPEK2": {""}, "checkbtn": {""},
	}
	params := url.Values{
		"co_id": {code}, "year": {fmt.Sprint(year)}, "season": {fmt.Sprint(quarter)},
	}
	dataDate := a.now().Format("2006-01-02")
	key := cache.KeyString(model.SourceMOPS, string(ds), dataDate, code, vals(params))
	cached, stale, raw, err := a.fetchRaw(ctx, string(ds), dataDate, key, func() ([]byte, error) {
		req := provider.RawRequest{
			URL:     a.mops.URL(ds, params),
			Body:    []byte(form.Encode()),
			Headers: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}},
		}
		resp, err := a.mops.Fetch(ctx, req)
		if err != nil {
			return nil, err
		}
		if err := a.mops.Validate(resp); err != nil {
			return nil, err
		}
		return a.mops.Normalize(resp)
	})
	if err != nil {
		return zero, false, false, err
	}
	if err := json.Unmarshal(raw, &zero); err != nil {
		return zero, false, false, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	return zero, cached, stale, nil
}

// valuationTSE 取得上市估值快照（BWIBBU_ALL，全市場，快取共用）。
func (a *App) valuationTSE(ctx context.Context) ([]provider.ValuationRow, bool, bool, error) {
	rows, cached, stale, err := apiRows[provider.ValuationRow](a, ctx, provider.TWSEAPIValuation)
	if err != nil {
		return nil, false, false, fmt.Errorf("上市估值資料取得失敗: %w", err)
	}
	return rows, cached, stale, nil
}

// apiRows 取得 TWSE-API 全市場快照（無參數資料集，快取共用，§12.4）。
func apiRows[T any](a *App, ctx context.Context, ds provider.TWSEAPIDataset) ([]T, bool, bool, error) {
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]T](a, ctx, string(ds), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(ds), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, ds, nil) })
	if err != nil {
		return nil, false, false, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	return rows, cached, stale, nil
}

// valuationOTC 取得上櫃估值快照（TPEx 本益比/殖利率/淨值比，全市場）。
func (a *App) valuationOTC(ctx context.Context) ([]provider.TPExPEValuationRow, bool, bool, error) {
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]provider.TPExPEValuationRow](a, ctx, string(provider.TPExPEValuation),
		dataDate, cache.KeyString(model.SourceTPExAPI, string(provider.TPExPEValuation), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExPEValuation, nil) })
	if err != nil {
		return nil, false, false, fmt.Errorf("上櫃估值資料取得失敗: %w", err)
	}
	return rows, cached, stale, nil
}

// ************** D. 基本面與篩選 **************

// handlerGetFinancialStatements：財報三表（§10.D）。
// period 支援 "2026Q1"（或 "2026" 年度）；statement 省略時回傳全部。
func handlerGetFinancialStatements(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	period := strVal(args["period"])
	statement := strVal(args["statement"])
	switch statement {
	case "", "income", "balance", "cashflow":
	default:
		return HandlerResult{}, fmt.Errorf("參數 statement 僅允許 income|balance|cashflow")
	}

	income, _, _, err := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	if err != nil {
		return HandlerResult{}, err
	}
	bySym := incomeOf(income, sym.Code)
	if len(bySym) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無損益表摘要資料", sym.Code)
	}
	year, quarter, err := parsePeriod(period, bySym)
	if err != nil {
		return HandlerResult{}, err
	}
	// 財報缺期邊界：請求期間無對應列（資料未釋出）
	incomeRows := filterPeriod(bySym, year, quarter)
	if len(incomeRows) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無 %dQ%d 財報（資料未釋出或期間不存在）", sym.Code, year, quarter)
	}

	out := model.FinancialStatements{
		Symbol: sym.Code, Name: sym.Name, Year: year, Quarter: quarter,
		Income: incomeRows,
	}
	cachedAny := false
	staleAny := false
	var profit []model.ProfitabilityRatio
	switch statement {
	case "", "income":
		profit, cachedAny, staleAny, err = mopsRows[model.ProfitabilityRatio](a, ctx, provider.MOPSProfitRatios)
		if err != nil {
			return HandlerResult{}, err
		}
		out.ProfitRatios = filterProfit(profit, sym.Code, year, quarter)
	}
	switch statement {
	case "", "balance":
		bs, cached, stale, err := mopsStatement[model.BalanceSheet](a, ctx, provider.MOPSBalanceSheet, sym.Code, year, quarter)
		if err != nil {
			return HandlerResult{}, fmt.Errorf("資產負債表取得失敗: %w", err)
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		out.BalanceSheet = &bs
	}
	switch statement {
	case "", "cashflow":
		cf, cached, stale, err := mopsStatement[model.CashFlowStatement](a, ctx, provider.MOPSCashFlow, sym.Code, year, quarter)
		if err != nil {
			return HandlerResult{}, fmt.Errorf("現金流量表取得失敗: %w", err)
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		out.CashFlow = &cf
	}
	ttl, _ := a.ttlOf(string(provider.MOPSIncomeSummary))
	lg := postLineage(model.SourceMOPS, a.now().Format("2006-01-02"), cachedAny || staleAny, staleAny, ttl)
	lg.SourceRole = model.SourceRoleCanonical
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// incomeOf 篩選代碼之損益表摘要列。
func incomeOf(rows []model.IncomeStatementRow, code string) []model.IncomeStatementRow {
	out := make([]model.IncomeStatementRow, 0)
	for _, r := range rows {
		if r.Code == code {
			out = append(out, r)
		}
	}
	return out
}

// filterPeriod 篩選指定（年, 季）之列。
func filterPeriod(rows []model.IncomeStatementRow, year, quarter int) []model.IncomeStatementRow {
	out := make([]model.IncomeStatementRow, 0)
	for _, r := range rows {
		if r.Year == year && r.Quarter == quarter {
			out = append(out, r)
		}
	}
	return out
}

// filterProfit 篩選指定（年, 季）之獲利能力指標。
func filterProfit(rows []model.ProfitabilityRatio, code string, year, quarter int) []model.ProfitabilityRatio {
	out := make([]model.ProfitabilityRatio, 0)
	for _, r := range rows {
		if r.Code == code && r.Year == year && r.Quarter == quarter {
			out = append(out, r)
		}
	}
	return out
}

// parsePeriod 解析 period 參數（"2026Q1" 或 "2026"＝年度）。
// 省略時取代碼最新（年, 季）。
func parsePeriod(period string, bySym []model.IncomeStatementRow) (int, int, error) {
	if period == "" {
		y, q := 0, 0
		for _, r := range bySym {
			if r.Year > y || (r.Year == y && r.Quarter > q) {
				y, q = r.Year, r.Quarter
			}
		}
		return y, q, nil
	}
	var year, quarter int
	if n, err := fmt.Sscanf(period, "%dQ%d", &year, &quarter); err == nil && n == 2 {
		if quarter < 1 || quarter > 4 {
			return 0, 0, fmt.Errorf("參數 period 之季度必須為 1-4")
		}
		return year, quarter, nil
	}
	if _, err := fmt.Sscanf(period, "%d", &year); err == nil {
		return year, 4, nil
	}
	return 0, 0, fmt.Errorf("參數 period 格式必須為 \"2026Q1\" 或 \"2026\"")
}

// handlerGetMonthlyRevenue：月營收 + 成長率（§10.D）。
func handlerGetMonthlyRevenue(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	years := 2
	if v, ok := args["years"]; ok {
		if n, e := asInt(v); e == nil && n >= 1 && n <= 10 {
			years = n
		}
	}
	rows, cached, stale, err := mopsRows[model.MonthlyRevenueRow](a, ctx, provider.MOPSMonthlyRevenue)
	if err != nil {
		return HandlerResult{}, err
	}
	bySym := make([]model.MonthlyRevenueRow, 0)
	for _, r := range rows {
		if r.Code == sym.Code {
			bySym = append(bySym, r)
		}
	}
	if len(bySym) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無月營收資料", sym.Code)
	}
	sort.Slice(bySym, func(i, j int) bool { return bySym[i].DataYearMonth > bySym[j].DataYearMonth })
	limit := years * 12
	if len(bySym) > limit {
		bySym = bySym[:limit]
	}
	ttl, _ := a.ttlOf(string(provider.MOPSMonthlyRevenue))
	return HandlerResult{Data: model.MonthlyRevenueBundle{
		Symbol: sym.Code, Name: sym.Name, Market: sym.Market, Rows: bySym,
	}, Lineage: postLineage(model.SourceMOPS, a.now().Format("2006-01-02"), cached || stale, stale, ttl)}, nil
}

// handlerGetFinancialHealthCheck：五面向評分（§10.D，T017 composite engine）。
// 評分輸入全部來自 T014 已快取之 raw 資料（財報/估值/股利/ESG），
// 引擎不直接呼叫 Adapter（§12.4）；輸出為 helper 資料，lineage
// 標明 source_role=helper 且 derived_from 列出所有父資料集。
func handlerGetFinancialHealthCheck(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	cfg, err := a.scoringConfig()
	if err != nil {
		return HandlerResult{}, err
	}
	in := fundamental.HealthInput{Code: sym.Code, Name: sym.Name, Market: sym.Market}
	derived := []string{}

	// 獲利能力 + 成長性：MOPS 損益表摘要（整批）＋獲利能力指標（整批）
	income, cachedI, staleI, err := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	if err != nil {
		return HandlerResult{}, err
	}
	derived = append(derived, "MOPS:income_summary")
	in.Income = incomeOf(income, sym.Code)
	profit, cachedP, staleP, err := mopsRows[model.ProfitabilityRatio](a, ctx, provider.MOPSProfitRatios)
	if err != nil {
		return HandlerResult{}, err
	}
	derived = append(derived, "MOPS:profit_ratios")
	bySym := incomeOf(income, sym.Code)
	if len(bySym) > 0 {
		year, quarter, perr := parsePeriod("", bySym)
		if perr == nil {
			in.Profit = filterProfit(profit, sym.Code, year, quarter)
		}
	}

	// 財務結構：最新一季資產負債表/現金流量表（快取共用）
	year, quarter := 0, 0
	if len(in.Income) > 0 {
		latest, _ := compositeLatestIncome(in.Income)
		year, quarter = latest.Year, latest.Quarter
	}
	if year > 0 {
		if bs, cached, stale, berr := mopsStatement[model.BalanceSheet](a, ctx, provider.MOPSBalanceSheet, sym.Code, year, quarter); berr == nil {
			in.Balance = &bs
			derived = append(derived, "MOPS:balance_sheet")
			cachedI = cachedI || cached
			staleI = staleI || stale
		}
		if cf, cached, stale, cerr := mopsStatement[model.CashFlowStatement](a, ctx, provider.MOPSCashFlow, sym.Code, year, quarter); cerr == nil {
			in.CashFlow = &cf
			derived = append(derived, "MOPS:cash_flow")
			cachedI = cachedI || cached
			staleI = staleI || stale
		}
	}

	// 配息政策 + 殖利率：上市 t187ap45_L 整批 / 上櫃 TPEx 估值
	if sym.Market == model.MarketOTC {
		otc, cached, stale, oerr := a.valuationOTC(ctx)
		if oerr != nil {
			return HandlerResult{}, err
		}
		cachedI = cachedI || cached
		staleI = staleI || stale
		derived = append(derived, "TPEx_API:pe_valuation")
		for _, r := range otc {
			if r.Code == sym.Code {
				in.Yield = r.YieldRatio
				if r.DividendPerShare > 0 {
					in.DividendYears = []fundamental.DividendYear{{Year: "最新", Cash: r.DividendPerShare}}
				}
				break
			}
		}
	} else {
		divRows, cached, stale, derr := apiRows[provider.DividendRow](a, ctx, provider.TWSEAPIDividend)
		if derr != nil {
			return HandlerResult{}, err
		}
		cachedI = cachedI || cached
		staleI = staleI || stale
		derived = append(derived, "TWSE_API:dividend")
		byCode := make([]provider.DividendRow, 0)
		for _, r := range divRows {
			if r.Code == sym.Code {
				byCode = append(byCode, r)
			}
		}
		sort.Slice(byCode, func(i, j int) bool { return byCode[i].DividendYear > byCode[j].DividendYear })
		for _, r := range byCode {
			in.DividendYears = append(in.DividendYears, fundamental.DividendYear{Year: r.DividendYear, Cash: r.CashDividend})
		}
		valRows, _, _, verr := a.valuationTSE(ctx)
		if verr == nil {
			derived = append(derived, "TWSE_API:valuation")
			for _, r := range valRows {
				if r.Code == sym.Code {
					in.Yield = r.DividendYield
					break
				}
			}
		}
	}

	// 公司治理：ESG（topic=1）＋公司治理規程（整批，快取共用）
	esgSet, err := a.esgCodes(ctx)
	if err != nil {
		return HandlerResult{}, err
	}
	derived = append(derived, "TWSE_API:esg")
	in.ESGDisclosed = esgSet[sym.Code]
	dataDate := a.now().Format("2006-01-02")
	govRows, cachedG, staleG, gerr := fetchNormalize[[]provider.GovernanceRow](a, ctx, string(provider.TWSEAPIGovernance),
		dataDate, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIGovernance), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIGovernance, nil) })
	if gerr == nil {
		derived = append(derived, "TWSE_API:company_governance")
		for _, r := range govRows {
			if r.Code == sym.Code {
				in.GovernanceDisclosed = true
				break
			}
		}
	}

	score := fundamental.ScoreHealth(in, cfg)
	score.DataDate = dataDate
	score.Note = "評分輸入來自 T014 已快取之官方資料（MOPS 財報/TWSE 估值・股利・ESG/TPEx 估值）"
	ttl, _ := a.ttlOf(string(provider.MOPSIncomeSummary))
	lg := postLineage(model.SourceMOPS, dataDate, cachedI || cachedP || cachedG || staleI || staleP || staleG, staleI || staleP || staleG, ttl)
	lg.DerivedFrom = derived
	return HandlerResult{Data: score, Lineage: lg}, nil
}

// scoringConfig 回傳五面向評分規則（預設 v1；config 可覆寫）。
func (a *App) scoringConfig() (fundamental.ScoringConfig, error) {
	if a.cfg == nil {
		return fundamental.DefaultScoringConfig(), nil
	}
	return a.cfg.Scoring()
}

// compositeLatestIncome 回傳該代碼最新（年, 季）之損益表摘要。
func compositeLatestIncome(rows []model.IncomeStatementRow) (model.IncomeStatementRow, bool) {
	var best model.IncomeStatementRow
	found := false
	for _, r := range rows {
		if !found || r.Year > best.Year || (r.Year == best.Year && r.Quarter > best.Quarter) {
			best = r
			found = true
		}
	}
	return best, found
}

// handlerGetValuationRatios：PE/PB/ROE/殖利率（§10.D）。
// ROE 由 MOPS 損益表摘要（年化）÷ 資產負債表權益計算（官方無直接端點）。
func handlerGetValuationRatios(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	out := model.ValuationRatios{Symbol: sym.Code, Name: sym.Name, Market: sym.Market}
	var ttl time.Duration
	if sym.Market == model.MarketOTC {
		rows, cached, stale, err := a.valuationOTC(ctx)
		if err != nil {
			return HandlerResult{}, err
		}
		ttl, _ = a.ttlOf(string(provider.TPExPEValuation))
		var row *provider.TPExPEValuationRow
		for i := range rows {
			if rows[i].Code == sym.Code {
				row = &rows[i]
				break
			}
		}
		if row == nil {
			return HandlerResult{}, fmt.Errorf("代碼 %s 無上櫃估值資料", sym.Code)
		}
		out.Date = row.Date
		out.PE = row.PE
		out.PEAvailable = row.PE > 0
		out.PB = row.PriceBookRatio
		out.DividendYield = row.YieldRatio
		out.DividendPerShare = row.DividendPerShare
		lg := postLineage(model.SourceTPExAPI, row.Date, cached || stale, stale, ttl)
		if err := a.fillROE(ctx, sym.Code, &out); err != nil {
			out.Note = "ROE 計算失敗：" + err.Error()
		}
		return HandlerResult{Data: out, Lineage: lg}, nil
	}
	rows, cached, stale, err := a.valuationTSE(ctx)
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ = a.ttlOf(string(provider.TWSEAPIValuation))
	var row *provider.ValuationRow
	for i := range rows {
		if rows[i].Code == sym.Code {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無上市估值資料", sym.Code)
	}
	out.Date = row.Date
	out.PE = row.PE
	out.PEAvailable = row.PE > 0
	out.PB = row.PB
	out.DividendYield = row.DividendYield
	// 每股現金股利（最新年度，t187ap45_L）
	if div, _, _, err := apiRows[provider.DividendRow](a, ctx, provider.TWSEAPIDividend); err == nil {
		if d := latestDividend(div, sym.Code); d != nil {
			out.DividendPerShare = d.CashDividend
		}
	}
	lg := postLineage(model.SourceTWSEAPI, row.Date, cached || stale, stale, ttl)
	if err := a.fillROE(ctx, sym.Code, &out); err != nil {
		out.Note = "ROE 計算失敗：" + err.Error()
	}
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// fillROE 以 MOPS 損益表摘要（最新一季，年化）÷ 資產負債表權益計算 ROE。
func (a *App) fillROE(ctx context.Context, code string, out *model.ValuationRatios) error {
	income, _, _, err := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	if err != nil {
		return err
	}
	bySym := incomeOf(income, code)
	if len(bySym) == 0 {
		return fmt.Errorf("無損益表摘要")
	}
	year, quarter, err := parsePeriod("", bySym)
	if err != nil {
		return err
	}
	var latest *model.IncomeStatementRow
	for i := range bySym {
		if bySym[i].Year == year && bySym[i].Quarter == quarter {
			latest = &bySym[i]
			break
		}
	}
	if latest == nil || latest.NetIncome == 0 {
		return fmt.Errorf("損益表摘要無淨利")
	}
	bs, _, _, err := mopsStatement[model.BalanceSheet](a, ctx, provider.MOPSBalanceSheet, code, year, quarter)
	if err != nil {
		return err
	}
	if bs.TotalEquity <= 0 {
		return fmt.Errorf("資產負債表無權益")
	}
	// 損益表摘要為累計數：年化 = 淨利 × 4/季別
	annualized := float64(latest.NetIncome) * 4 / float64(quarter)
	out.ROE = annualized / float64(bs.TotalEquity) * 100
	out.ROEMethod = fmt.Sprintf("MOPS 損益表摘要 %dQ%d 累計稅後淨利 ×4/%d ÷ 資產負債表權益（年化估計）", year, quarter, quarter)
	return nil
}

// latestDividend 回傳代碼最新股利年度之分派。
func latestDividend(rows []provider.DividendRow, code string) *provider.DividendRow {
	var best *provider.DividendRow
	for i := range rows {
		if rows[i].Code != code {
			continue
		}
		if best == nil || rows[i].DividendYear > best.DividendYear {
			best = &rows[i]
		}
	}
	return best
}

// handlerGetESGReport：ESG/公司治理指標（§10.D，TWSE OpenAPI）。
func handlerGetESGReport(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	dataDate := a.now().Format("2006-01-02")
	esgRows, cachedESG, staleESG, err := fetchNormalize[[]provider.ESGRow](a, ctx, string(provider.TWSEAPIESG),
		dataDate, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIESG), dataDate, "", map[string]string{"topic": "1"}),
		func() ([]byte, error) {
			return a.fetchAPIRaw(ctx, provider.TWSEAPIESG, url.Values{"topic": {"1"}})
		})
	if err != nil {
		return HandlerResult{}, err
	}
	govRows, cachedGov, staleGov, err := fetchNormalize[[]provider.GovernanceRow](a, ctx, string(provider.TWSEAPIGovernance),
		dataDate, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIGovernance), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIGovernance, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	out := model.ESGReport{Symbol: sym.Code, Name: sym.Name, Market: sym.Market, Topics: make([]model.ESGTopic, 0)}
	for _, r := range esgRows {
		if r.Code == sym.Code {
			out.Topics = append(out.Topics, model.ESGTopic{
				Topic: "溫室氣體排放", Year: r.Year, ReportDate: r.ReportDate, Fields: r.Fields,
			})
		}
	}
	for _, r := range govRows {
		if r.Code == sym.Code {
			out.Topics = append(out.Topics, model.ESGTopic{
				Topic: "公司治理", ReportDate: r.ReportDate,
				Fields: map[string]string{"公司治理之相關規程規則": r.Rules},
			})
		}
	}
	if len(out.Topics) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無 ESG/公司治理揭露資料", sym.Code)
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIESG))
	lg := postLineage(model.SourceTWSEAPI, dataDate, cachedESG || cachedGov || staleESG || staleGov, staleESG || staleGov, ttl)
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// handlerGetCompanyProfile：公司基本資料（§10.D）。
func handlerGetCompanyProfile(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	rows, cached, stale, err := mopsRows[model.CompanyProfile](a, ctx, provider.MOPSCompanyProfile)
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range rows {
		if r.Code == sym.Code {
			ttl, _ := a.ttlOf(string(provider.MOPSCompanyProfile))
			return HandlerResult{Data: r, Lineage: postLineage(model.SourceMOPS, r.TableDate, cached || stale, stale, ttl)}, nil
		}
	}
	return HandlerResult{}, fmt.Errorf("代碼 %s 無公司基本資料", sym.Code)
}

// handlerScreenStocks：價值/成長篩選（§10.D，T017 引擎批次過濾）。
// 全市場快照（上市 BWIBBU_ALL + 上櫃 TPEx 估值 + MOPS 月營收 YoY），
// 記憶體過濾，不逐股打上游（§12.4）。
func handlerScreenStocks(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	market, _ := args["market"].(string)
	c := screener.ValueCriterion{}
	if v, ok := args["max_pe"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MaxPE = f
		}
	}
	if v, ok := args["max_pb"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MaxPB = f
		}
	}
	if v, ok := args["min_yield"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MinYield = f
		}
	}
	if v, ok := args["min_growth"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MinGrowth = f
		}
	}
	if v, ok := args["min_profit_growth"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MinProfitGrowth = f
		}
	}
	requireESG, _ := args["require_esg"].(bool)
	limit := 50
	if v, ok := args["limit"]; ok {
		if n, e := asInt(v); e == nil && n >= 1 && n <= 200 {
			limit = n
		}
	}
	c.TopN = limit
	// 排序（T017）：pe（預設）| yield | pb | growth
	switch strVal(args["sort"]) {
	case "yield":
		c.Sort = screener.SortByYield
	case "pb":
		c.Sort = screener.SortByPB
	case "growth":
		c.Sort = screener.SortByGrowth
	case "", "pe":
		c.Sort = screener.SortByPE
	default:
		return HandlerResult{}, fmt.Errorf("參數 sort 僅允許 pe|yield|pb|growth")
	}
	metrics, meta, err := a.screenMetrics(ctx, market)
	if err != nil {
		return HandlerResult{}, err
	}
	if requireESG {
		esgSet, err := a.esgCodes(ctx)
		if err != nil {
			return HandlerResult{}, err
		}
		filtered := metrics[:0]
		for _, v := range metrics {
			if esgSet[v.Code] {
				filtered = append(filtered, v)
			}
		}
		metrics = filtered
	}
	matches := screener.ScreenValue(metrics, c)
	out := model.ScreenResult{Total: len(metrics), Matched: len(matches), Limit: limit}
	for _, m := range matches {
		out.Rows = append(out.Rows, model.ScreenStock{
			Code: m.Code, Name: m.Name, Market: m.Market,
			PE: m.PE, PEAvailable: m.PEAvailable, PB: m.PB,
			DividendYield: m.DividendYield, RevenueGrowth: m.RevenueGrowth,
			ProfitGrowth: m.ProfitGrowth,
			Matched:      m.Matched,
		})
	}
	lg := meta.lineage()
	if c.MinGrowth > 0 || c.MinProfitGrowth > 0 {
		lg.DerivedFrom = append(lg.DerivedFrom, "MOPS:monthly_revenue")
	}
	if c.MinProfitGrowth > 0 {
		lg.DerivedFrom = append(lg.DerivedFrom, "MOPS:income_summary")
	}
	if requireESG {
		lg.DerivedFrom = append(lg.DerivedFrom, "TWSE_API:esg")
	}
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// ************** E. 股利 **************

// handlerGetDividendHistory：配息歷史 + 穩定性（§10.E）。
func handlerGetDividendHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	out := model.DividendHistory{Symbol: sym.Code, Name: sym.Name, Market: sym.Market}
	var ttl time.Duration
	if sym.Market == model.MarketOTC {
		rows, cached, stale, err := a.valuationOTC(ctx)
		if err != nil {
			return HandlerResult{}, err
		}
		ttl, _ = a.ttlOf(string(provider.TPExPEValuation))
		for _, r := range rows {
			if r.Code == sym.Code {
				out.Years = []model.DividendYear{{
					DividendYear: "最新", Progress: "官方現行資料",
					CashDividend: r.DividendPerShare,
				}}
				out.TotalYears = 1
				if r.DividendPerShare > 0 {
					out.ConsecutiveYears = 1
				}
				out.AvgCashDividend = r.DividendPerShare
				out.LastYield = r.YieldRatio
				out.Note = "上櫃多年配息歷史（TPEx 歷史除息資料）尚未接線；僅提供最新年度每股股利與殖利率"
				return HandlerResult{Data: out, Lineage: postLineage(model.SourceTPExAPI, r.Date, cached || stale, stale, ttl)}, nil
			}
		}
		return HandlerResult{}, fmt.Errorf("代碼 %s 無上櫃估值資料", sym.Code)
	}
	divRows, cached, stale, err := apiRows[provider.DividendRow](a, ctx, provider.TWSEAPIDividend)
	if err != nil {
		return HandlerResult{}, err
	}
	years := make([]model.DividendYear, 0)
	for _, r := range divRows {
		if r.Code == sym.Code {
			years = append(years, model.DividendYear{
				DividendYear: r.DividendYear, Progress: r.Progress,
				CashDividend: r.CashDividend, StockDividend: r.StockDividend,
				CashTotal: int64(r.CashTotal), NetIncome: int64(r.NetIncome), Retained: int64(r.Retained),
			})
		}
	}
	if len(years) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無股利分派資料", sym.Code)
	}
	sort.Slice(years, func(i, j int) bool { return years[i].DividendYear > years[j].DividendYear })
	out.Years = years
	out.TotalYears = len(years)
	var sum float64
	for _, y := range years {
		if y.CashDividend > 0 {
			out.ConsecutiveYears++
		}
		sum += y.CashDividend
	}
	out.AvgCashDividend = sum / float64(len(years))
	// 最新殖利率（上市估值快照）
	valRows, _, _, err := a.valuationTSE(ctx)
	if err == nil {
		for _, r := range valRows {
			if r.Code == sym.Code {
				out.LastYield = r.DividendYield
				break
			}
		}
	}
	out.Note = "股利年度以官方（民國）為準；連續配息年數僅以官方現行提供之年度計算"
	ttl, _ = a.ttlOf(string(provider.TWSEAPIDividend))
	return HandlerResult{Data: out, Lineage: postLineage(model.SourceTWSEAPI, a.now().Format("2006-01-02"), cached || stale, stale, ttl)}, nil
}

// handlerGetExdividendCalendar：除權息行事曆（§10.E）。
// 上市 TWT48U_ALL + 上櫃 TPEx 除權除息預告；[start, end] 過濾。
func handlerGetExdividendCalendar(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	now := a.now()
	start, err := model.ParseDate(strVal(args["start"]))
	if err != nil {
		start = now
	}
	end, err := model.ParseDate(strVal(args["end"]))
	if err != nil {
		end = start.AddDate(0, 6, 0)
	}
	if end.Before(start) {
		return HandlerResult{}, fmt.Errorf("參數 end 不得早於 start")
	}
	startS, endS := model.FormatDate(start), model.FormatDate(end)
	out := model.ExDivCalendar{RangeStart: startS, RangeEnd: endS, Events: make([]model.ExDivEvent, 0)}

	// 上市
	tseRows, cachedTSE, staleTSE, err := fetchNormalize[[]provider.ExDivEventRow](a, ctx, string(provider.TWSEAPIExDiv),
		now.Format("2006-01-02"), cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIExDiv), now.Format("2006-01-02"), "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIExDiv, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range tseRows {
		if r.Date >= startS && r.Date <= endS {
			out.Events = append(out.Events, model.ExDivEvent{
				Date: r.Date, Code: r.Code, Name: r.Name, Market: model.MarketTSE,
				Kind: r.Kind, CashDividend: r.CashDividend, StockDividend: r.StockRatio,
			})
		}
	}
	// 上櫃
	otcRows, cachedOTC, staleOTC, err := fetchNormalize[[]provider.TPExExRightRow](a, ctx, string(provider.TPExExRights),
		now.Format("2006-01-02"), cache.KeyString(model.SourceTPExAPI, string(provider.TPExExRights), now.Format("2006-01-02"), "", nil),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExExRights, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range otcRows {
		if r.Date >= startS && r.Date <= endS {
			out.Events = append(out.Events, model.ExDivEvent{
				Date: r.Date, Code: r.Code, Name: r.Name, Market: model.MarketOTC,
				Kind: r.Kind, CashDividend: r.CashDividend, StockDividend: r.StockDividendRatio,
			})
		}
	}
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].Date < out.Events[j].Date })
	ttl, _ := a.ttlOf(string(provider.TWSEAPIExDiv))
	lg := postLineage(model.SourceTWSEAPI, now.Format("2006-01-02"), cachedTSE || cachedOTC || staleTSE || staleOTC, staleTSE || staleOTC, ttl)
	if len(out.Events) == 0 {
		return HandlerResult{Data: out, Lineage: lg}, nil
	}
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// handlerScreenHighYield：高殖利率排行（§9.4/§10.E）。
// §10.3 優先自 Materialized Screener Index 直接 SELECT（零即時 Adapter 請求，
// _lineage.freshness=索引建立時間）；索引未建立時退回 T017 引擎即時整批路徑（§12.4）。
func handlerScreenHighYield(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	market, _ := args["market"].(string)
	c := screener.HighYieldCriterion{}
	if v, ok := args["min_yield"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MinYield = f
		}
	}
	if c.MinYield == 0 {
		c.MinYield = 3.0
	}
	if v, ok := args["min_dividend"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MinDividend = f
		}
	}
	if v, ok := args["max_pe"]; ok {
		if f, e := asFloat(v); e == nil && f > 0 {
			c.MaxPE = f
		}
	}
	if v, ok := args["min_consecutive"]; ok {
		if n, e := asInt(v); e == nil && n >= 0 {
			c.MinConsecutive = n
		}
	}
	limit := 20
	if v, ok := args["limit"]; ok {
		if n, e := asInt(v); e == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}
	c.TopN = limit

	if res, used, err := a.queryHighYieldIndex(ctx, market, c); used {
		return res, err
	}

	metrics, meta, err := a.screenMetrics(ctx, market)
	if err != nil {
		return HandlerResult{}, err
	}
	matches := screener.ScreenHighYield(metrics, c)
	out := model.ScreenResult{Total: len(metrics), Matched: len(matches), Limit: limit}
	for _, m := range matches {
		out.Rows = append(out.Rows, model.ScreenStock{
			Code: m.Code, Name: m.Name, Market: m.Market,
			PE: m.PE, PEAvailable: m.PEAvailable, PB: m.PB,
			DividendYield: m.DividendYield, DividendShare: m.DividendShare,
			ConsecutiveYears: m.ConsecutiveYears,
			Matched:          m.Matched,
		})
	}
	return HandlerResult{Data: out, Lineage: meta.lineage()}, nil
}

// queryHighYieldIndex 自 §10.3 materialized index 直接 SELECT 高殖利率工具；
// used=true 表示索引已建立並完成查詢（零即時 Adapter 請求，freshness=索引建立時間）。
// used=false 時呼叫端退回既有即時引擎路徑（索引未啟用/該日期尚未建立）。
func (a *App) queryHighYieldIndex(ctx context.Context, market string, c screener.HighYieldCriterion) (HandlerResult, bool, error) {
	if a.index == nil {
		return HandlerResult{}, false, nil
	}
	date := a.now().Format("2006-01-02")
	if _, ok, err := a.index.BuiltAt(ctx, date); err != nil || !ok {
		return HandlerResult{}, false, nil
	}
	hit, err := a.index.Query(ctx, screener.IndexQuery{
		Date:           date,
		Market:         screener.IndexMarket(market),
		MinYield:       c.MinYield,
		MinDividend:    c.MinDividend,
		MaxPE:          c.MaxPE,
		MinConsecutive: c.MinConsecutive,
		Rows:           c.TopN,
	})
	if err != nil {
		return HandlerResult{}, false, err
	}
	total, err := a.index.Count(ctx, date, screener.IndexMarket(market))
	if err != nil {
		return HandlerResult{}, false, err
	}
	out := model.ScreenResult{Total: total, Matched: len(hit.Rows), Limit: c.TopN}
	for _, r := range hit.Rows {
		out.Rows = append(out.Rows, model.ScreenStock{
			Code: r.Symbol, Name: r.Name, Market: r.Market,
			PE: r.PE, PEAvailable: r.PEAvailable, PB: r.PB,
			DividendYield: r.DividendYieldPct, DividendShare: r.CashDividend,
			ConsecutiveYears: r.ConsecutiveYears,
			Matched:          indexMatchedReasons(r, c),
		})
	}
	ttl, _ := a.ttlOf(cache.DatasetValuation)
	lg := postLineage(model.SourceTWSEAPI, date, true, false, ttl)
	lg.FetchedAt = model.NewTaipeiTime(hit.BuiltAt) // §10.3：freshness=索引建立時間
	lg.DerivedFrom = []string{"screener_index"}
	lg.CacheAgeSec = int64(a.now().Sub(hit.BuiltAt).Seconds())
	return HandlerResult{Data: out, Lineage: lg}, true, nil
}

// indexMatchedReasons 依條件說明索引命中理由（與 T017 引擎 matched 同語意）。
func indexMatchedReasons(r screener.IndexRow, c screener.HighYieldCriterion) []string {
	var m []string
	if c.MinYield > 0 {
		m = append(m, "高殖利率")
	}
	if c.MinDividend > 0 {
		m = append(m, "高每股股利")
	}
	if c.MaxPE > 0 {
		m = append(m, "低本益比")
	}
	if c.MinConsecutive > 0 {
		m = append(m, "配息穩定")
	}
	return m
}

// ************** 篩選共用 **************

// screenMeta 記錄篩選工具之資料源與快取狀態（供 lineage 聚合）。
type screenMeta struct {
	rows     []screener.ValuationMetrics
	source   string
	dataDate string
	cached   bool
	stale    bool // v2.1 §5.2 stale-if-error
	ttl      time.Duration
	derived  []string
}

func (m *screenMeta) lineage() *model.Lineage {
	lg := postLineage(m.source, m.dataDate, m.cached || m.stale, m.stale, m.ttl)
	lg.SourceRole = model.SourceRoleCanonical
	lg.DerivedFrom = m.derived
	return lg
}

// screenMetrics 建立篩選輸入（估值 + 月營收成長 + 每股現金股利；上市/上櫃
// 分批，全部整批快取，§12.4）。
func (a *App) screenMetrics(ctx context.Context, market string) ([]screener.ValuationMetrics, *screenMeta, error) {
	var metrics []screener.ValuationMetrics
	meta := &screenMeta{}
	switch market {
	case "", model.MarketTSE:
		tse, cached, stale, err := a.valuationTSE(ctx)
		if err != nil {
			return nil, nil, err
		}
		meta.cached = meta.cached || cached
		meta.stale = meta.stale || stale
		meta.derived = append(meta.derived, "TWSE_API:valuation")
		for _, r := range tse {
			metrics = append(metrics, screener.ValuationMetrics{
				Code: r.Code, Name: r.Name, Market: model.MarketTSE,
				PE: r.PE, PEAvailable: r.PE > 0, PB: r.PB, DividendYield: r.DividendYield,
			})
		}
	}
	switch market {
	case "", model.MarketOTC:
		otc, cached, stale, err := a.valuationOTC(ctx)
		if err != nil {
			return nil, nil, err
		}
		meta.cached = meta.cached || cached
		meta.stale = meta.stale || stale
		meta.derived = append(meta.derived, "TPEx_API:pe_valuation")
		for _, r := range otc {
			metrics = append(metrics, screener.ValuationMetrics{
				Code: r.Code, Name: r.Name, Market: model.MarketOTC,
				PE: r.PE, PEAvailable: r.PE > 0, PB: r.PriceBookRatio,
				DividendYield: r.YieldRatio, DividendShare: r.DividendPerShare,
			})
		}
	}
	if market != model.MarketTSE && market != model.MarketOTC && market != "" {
		return nil, nil, fmt.Errorf("參數 market 僅允許 tse|otc")
	}
	// 月營收 YoY（最近月份，整批 CSV）
	revenue, _, _, err := mopsRows[model.MonthlyRevenueRow](a, ctx, provider.MOPSMonthlyRevenue)
	if err != nil {
		return nil, nil, err
	}
	growth := make(map[string]float64)
	for _, r := range revenue {
		// 覆寫式取最後一筆＝資料集內最新月份（CSV 依月份排序）
		growth[r.Code] = r.YoYChange
	}
	meta.derived = append(meta.derived, "MOPS:monthly_revenue")
	for i := range metrics {
		if g, ok := growth[metrics[i].Code]; ok {
			metrics[i].RevenueGrowth = g
			metrics[i].HasGrowth = true
		}
	}
	// 獲利成長（淨利 YoY，MOPS 損益表摘要整批；最新季 vs 去年同期）
	income, _, _, ierr := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	if ierr == nil {
		latest, prev := latestIncomeOf(income), incomeAgoOf(income)
		meta.derived = append(meta.derived, "MOPS:income_summary")
		for i := range metrics {
			li, ok1 := latest[metrics[i].Code]
			pi, ok2 := prev[metrics[i].Code]
			if ok1 && ok2 && pi.NetIncome > 0 {
				metrics[i].ProfitGrowth = (float64(li.NetIncome) - float64(pi.NetIncome)) / float64(pi.NetIncome) * 100
				metrics[i].HasProfitGrowth = true
			}
		}
	}
	// 上市每股現金股利（t187ap45_L 整批）＋連年配息年數（配息穩定性，§10.E）
	if market != model.MarketOTC {
		divRows, _, _, err := apiRows[provider.DividendRow](a, ctx, provider.TWSEAPIDividend)
		if err != nil {
			return nil, nil, err
		}
		div := make(map[string]float64)
		divYear := make(map[string]string)
		divYears := make(map[string][]provider.DividendRow)
		for _, r := range divRows {
			if y, ok := divYear[r.Code]; !ok || r.DividendYear > y {
				divYear[r.Code] = r.DividendYear
				div[r.Code] = r.CashDividend
			}
			divYears[r.Code] = append(divYears[r.Code], r)
		}
		meta.derived = append(meta.derived, "TWSE_API:dividend")
		for i := range metrics {
			if d, ok := div[metrics[i].Code]; ok {
				metrics[i].DividendShare = d
			}
			metrics[i].ConsecutiveYears = consecutiveDividendYears(divYears[metrics[i].Code])
		}
	} else {
		// 上櫃：僅最新年度（TPEx 歷史除息資料未接線）
		for i := range metrics {
			if metrics[i].DividendShare > 0 {
				metrics[i].ConsecutiveYears = 1
			}
		}
	}
	meta.dataDate = a.now().Format("2006-01-02")
	meta.source = model.SourceTWSEAPI
	if market == model.MarketOTC {
		meta.source = model.SourceTPExAPI
	}
	meta.ttl, _ = a.ttlOf(cache.DatasetValuation)
	return metrics, meta, nil
}

// latestIncomeOf 回傳各代碼最新（年, 季）之損益表摘要（含去年同期對照）。
// 由 (latest, prev) 兩 map 組成：latest 為最新季、prev 為最新季之去年同期。
func latestIncomeOf(rows []model.IncomeStatementRow) map[string]model.IncomeStatementRow {
	out := make(map[string]model.IncomeStatementRow)
	for _, r := range rows {
		best, ok := out[r.Code]
		if !ok || r.Year > best.Year || (r.Year == best.Year && r.Quarter > best.Quarter) {
			out[r.Code] = r
		}
	}
	return out
}

func incomeAgoOf(rows []model.IncomeStatementRow) map[string]model.IncomeStatementRow {
	latest := latestIncomeOf(rows)
	out := make(map[string]model.IncomeStatementRow)
	for _, r := range rows {
		li, ok := latest[r.Code]
		if !ok {
			continue
		}
		if r.Year == li.Year-1 && r.Quarter == li.Quarter {
			out[r.Code] = r
		}
	}
	return out
}

// consecutiveDividendYears 計算連年配息年數（§10.E 配息穩定性）：
// 官方股利年度由新至舊，期間現金股利 > 0 之連續年數（0 年中斷即停）。
func consecutiveDividendYears(rows []provider.DividendRow) int {
	sort.Slice(rows, func(i, j int) bool { return rows[i].DividendYear > rows[j].DividendYear })
	n := 0
	for i, r := range rows {
		if r.CashDividend <= 0 {
			break
		}
		if i > 0 && rows[i-1].DividendYear == r.DividendYear {
			continue // 同年多次決議不重複計
		}
		n++
	}
	return n
}

// esgCodes 回傳具 ESG 揭露之代碼集合（topic=1 溫室氣體排放，全市場）。
func (a *App) esgCodes(ctx context.Context) (map[string]bool, error) {
	dataDate := a.now().Format("2006-01-02")
	rows, _, _, err := fetchNormalize[[]provider.ESGRow](a, ctx, string(provider.TWSEAPIESG),
		dataDate, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIESG), dataDate, "", map[string]string{"topic": "1"}),
		func() ([]byte, error) {
			return a.fetchAPIRaw(ctx, provider.TWSEAPIESG, url.Values{"topic": {"1"}})
		})
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.Code] = true
	}
	return out, nil
}

// asFloat 將 JSON number/string 轉為 float64。
func asFloat(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case int:
		return float64(n), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(strings.TrimSpace(n), "%g", &f); err != nil {
			return 0, fmt.Errorf("非數字字串 %q", n)
		}
		return f, nil
	}
	return 0, fmt.Errorf("非數字型別 %T", v)
}
