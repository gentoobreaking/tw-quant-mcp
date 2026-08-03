package mcp

// tools_trend.go 實作 v2.1 §9.1 唯一缺口工具 get_stock_trend_composite
//（T029）：短中長期「技術面+基本面+籌碼面」綜合研判。跨來源聚合
//（TWSE Web API 日 K/法人 + TWSE-API 估值 + TPEx-API 估值/法人 +
// MOPS 損益表摘要），_lineage 以 []Lineage 陣列逐一標註（v2.1 §4
// 設計規則 2）。Grade PREVIEW（v2.1 §9.1）。

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/chart"
	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/model/domain"
	"tw-quant-mcp/pkg/provider"
)

// trendHorizon 對應 horizon 參數之研判窗口（v2.1 §9.1 short/mid/long）。
//   - short：近 1 個月日 K（MA5/MA20）＋法人近 5 日
//   - mid：近 3 個月日 K（MA20/MA60）＋法人近 20 日
//   - long：近 6 個月日 K（MA20/MA60）＋法人近 60 日
type trendHorizon struct {
	label  string
	months int // 日 K 回溯月數
	days   int // 法人累計交易日數
	note   string
}

var trendHorizons = map[string]trendHorizon{
	"short": {label: "short", months: 1, days: 5, note: "短期：近 1 月技術面（MA5/MA20）+ 法人近 5 日"},
	"mid":   {label: "mid", months: 3, days: 20, note: "中期：近 3 月技術面（MA20/MA60）+ 法人近 20 日"},
	"long":  {label: "long", months: 6, days: 60, note: "長期：近 6 月技術面（MA20/MA60）+ 法人近 60 日"},
}

// trendSignal 依 MA 排列與 RSI 產出趨勢訊號。
func trendSignal(ma5, ma20, ma60, rsi float64) string {
	switch {
	case ma5 > 0 && ma20 > 0 && ma5 >= ma20:
		return "BULLISH"
	case ma5 > 0 && ma20 > 0 && ma20 > ma60 && ma5 < ma20:
		return "NEUTRAL"
	case ma5 > 0 && ma20 > 0 && ma5 < ma20 && ma20 <= ma60:
		return "BEARISH"
	}
	if rsi >= 70 {
		return "BULLISH"
	}
	if rsi <= 30 {
		return "BEARISH"
	}
	return "NEUTRAL"
}

// handlerGetStockTrendComposite：短中長期綜合研判（§9.1，Grade PREVIEW）。
func handlerGetStockTrendComposite(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	h, ok := trendHorizons[strVal(args["horizon"])]
	if !ok {
		// 省略時預設 mid（v2.1 §9.1 參數選填）
		if strVal(args["horizon"]) == "" {
			h = trendHorizons["mid"]
		} else {
			return HandlerResult{}, fmt.Errorf("參數 horizon 必須為 short/mid/long")
		}
	}
	date, err := a.resolveDate("")
	if err != nil {
		return HandlerResult{}, err
	}

	out := domain.TrendComposite{
		Stock:   domain.StockIdentity{Symbol: sym.Code, Name: sym.Name, Market: sym.Market},
		Horizon: h.label,
		Lineage: []model.Lineage{},
	}

	// ── 技術面：日 K（TWSE Web API）──
	tech, techLg, err := a.trendTechnical(ctx, sym, date, h)
	if err != nil {
		return HandlerResult{}, err
	}
	out.Technical = tech
	out.Lineage = append(out.Lineage, techLg)

	// ── 基本面：估值（TWSE-API / TPEx-API）＋ EPS 成長（MOPS）──
	fund, fundLgs, err := a.trendFundamental(ctx, sym, date)
	if err != nil {
		return HandlerResult{}, err
	}
	out.Fundamental = fund
	out.Lineage = append(out.Lineage, fundLgs...)

	// ── 籌碼面：三大法人（TWSE Web API / TPEx-API）──
	chip, chipLg, err := a.trendChip(ctx, sym, date, h)
	if err != nil {
		return HandlerResult{}, err
	}
	out.Chip = chip
	out.Lineage = append(out.Lineage, chipLg)

	meta := trendChartMeta(tech, fund, chip)
	out.ChartData = meta
	return HandlerResult{Data: out, MultiLineage: out.Lineage, ChartMeta: meta}, nil
}

// trendKline 回溯 months 個月日 K 並組收盤序列（上市 STOCK_DAY；上櫃無歷史
// 資料源，回傳錯誤由呼叫端轉為 note）。
func (a *App) trendKline(ctx context.Context, sym model.Symbol, date string, months int) ([]model.Candle, bool, bool, error) {
	start, _ := model.ParseDate(date)
	starts := monthStarts(start, months)
	var all []model.Candle
	cachedAny, staleAny := false, false
	for _, ms := range starts {
		params := url.Values{"date": {ms.Format("20060102")}, "stockNo": {sym.Code}}
		rows, cached, stale, err := fetchNormalize[[]model.Candle](a, ctx, string(provider.TWSEWDDailyK),
			date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDDailyK), date, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDDailyK, params) })
		if err != nil {
			return nil, false, false, err
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		all = append(all, rows...)
	}
	return all, cachedAny, staleAny, nil
}

// trendTechnical 計算技術面視圖（MA5/MA20/MA60/RSI14 + 訊號）。
// 上櫃無歷史 K 線資料源（T018 未接線），以 0 值 + note 標註。
func (a *App) trendTechnical(ctx context.Context, sym model.Symbol, date string, h trendHorizon) (domain.TechnicalView, model.Lineage, error) {
	view := domain.TechnicalView{}
	lg := model.Lineage{Source: model.SourceTWSEWeb, SourceRole: model.SourceRoleCanonical,
		Freshness: model.FreshnessPostMarket, DataDate: date, Grade: model.GradePreview,
		DerivedFrom: []string{"TWSE_WEB:daily_k"}}
	ttl, _ := a.ttlOf(string(provider.TWSEWDDailyK))
	lg.CacheTTL = int(ttl.Seconds())
	if sym.Market == model.MarketOTC {
		// 上櫃：回傳 0 值視圖，來源仍為 TPEx（收盤行情）；technical 由
		// handler 以 note 說明從缺。此處不 fetch，避免無謂請求。
		lg.Source = model.SourceTPExAPI
		lg.DerivedFrom = []string{"TPEX_API:daily_close"}
		lg.SourceRole = model.SourceRoleFallback
		return view, lg, nil
	}
	all, cached, stale, err := a.trendKline(ctx, sym, date, h.months)
	if err != nil {
		return view, model.Lineage{}, err
	}
	byDay := make(map[string]model.Candle, len(all))
	for _, c := range all {
		byDay[c.Timestamp] = c
	}
	closes := make([]float64, 0, len(byDay))
	for _, d := range sortedKeys(byDay) {
		closes = append(closes, byDay[d].Close)
	}
	view.MA5 = engine.SMA(closes, 5)
	view.MA20 = engine.SMA(closes, 20)
	view.MA60 = engine.SMA(closes, 60)
	view.RSI14 = engine.RSI(closes, 14)
	view.TrendSignal = trendSignal(view.MA5, view.MA20, view.MA60, view.RSI14)
	lg.IsCached = cached || stale
	lg.CacheTTL = int(ttl.Seconds())
	return view, lg, nil
}

// trendFundamental 計算基本面視圖（PE/PB/殖利率 + EPS YoY）。
// 估值來源：上市 TWSE-API BWIBBU_ALL；上櫃 TPEx-API 本益比/殖利率/淨值比。
// EPS 成長：MOPS 損益表摘要（最新季 vs 去年同期）。
func (a *App) trendFundamental(ctx context.Context, sym model.Symbol, date string) (domain.FundamentalView, []model.Lineage, error) {
	view := domain.FundamentalView{}
	var lgs []model.Lineage
	if sym.Market == model.MarketOTC {
		rows, cached, stale, err := a.valuationOTC(ctx)
		if err != nil {
			return view, nil, err
		}
		ttl, _ := a.ttlOf(string(provider.TPExPEValuation))
		lg := postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)
		lg.Grade = model.GradePreview
		lg.DerivedFrom = []string{"TPEX_API:pe_valuation"}
		lgs = append(lgs, *lg)
		var row *provider.TPExPEValuationRow
		for i := range rows {
			if rows[i].Code == sym.Code {
				row = &rows[i]
				break
			}
		}
		if row == nil {
			return view, lgs, fmt.Errorf("代碼 %s 無上櫃估值資料", sym.Code)
		}
		view.PE = row.PE
		view.PB = row.PriceBookRatio
		view.DividendYieldPct = row.YieldRatio
	} else {
		rows, cached, stale, err := a.valuationTSE(ctx)
		if err != nil {
			return view, nil, err
		}
		ttl, _ := a.ttlOf(string(provider.TWSEAPIValuation))
		lg := postLineage(model.SourceTWSEAPI, date, cached || stale, stale, ttl)
		lg.Grade = model.GradePreview
		lg.DerivedFrom = []string{"TWSE_API:valuation"}
		lgs = append(lgs, *lg)
		var row *provider.ValuationRow
		for i := range rows {
			if rows[i].Code == sym.Code {
				row = &rows[i]
				break
			}
		}
		if row == nil {
			return view, lgs, fmt.Errorf("代碼 %s 無上市估值資料", sym.Code)
		}
		view.PE = row.PE
		view.PB = row.PB
		view.DividendYieldPct = row.DividendYield
	}

	// EPS 成長（MOPS）：最新季 vs 去年同期。
	if eps, lg, err := a.epsGrowthYoY(ctx, sym.Code); err == nil {
		view.EPSGrowthYoYPct = eps
		if lg.Source != "" {
			lgs = append(lgs, lg)
		}
	}
	return view, lgs, nil
}

// epsGrowthYoY 以 MOPS 損益表摘要計算最新季 EPS YoY（%）。
// 缺去年同期（新上市/資料不足）時回傳 0 且不附加 lineage（上層視為選填）。
func (a *App) epsGrowthYoY(ctx context.Context, code string) (float64, model.Lineage, error) {
	var zero model.Lineage
	rows, cached, stale, err := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	if err != nil {
		return 0, zero, err
	}
	bySym := incomeOf(rows, code)
	if len(bySym) == 0 {
		return 0, zero, fmt.Errorf("無損益表摘要")
	}
	y, q, err := parsePeriod("", bySym)
	if err != nil {
		return 0, zero, err
	}
	latest := filterPeriod(bySym, y, q)
	if len(latest) == 0 || latest[0].EPS == 0 {
		return 0, zero, fmt.Errorf("最新季無 EPS")
	}
	prevQ := q
	prevY := y - 1 // 去年同期＝去年同季（v2.1 §9.1：YoY 比較）
	prev := filterPeriod(bySym, prevY, prevQ)
	if len(prev) == 0 || prev[0].EPS == 0 {
		return 0, zero, fmt.Errorf("缺去年同期 EPS")
	}
	pct := (latest[0].EPS - prev[0].EPS) / prev[0].EPS * 100
	ttl, _ := a.ttlOf(string(provider.MOPSIncomeSummary))
	lg := postLineage(model.SourceMOPS, a.now().Format("2006-01-02"), cached || stale, stale, ttl)
	lg.Grade = model.GradePreview
	lg.DerivedFrom = []string{"MOPS:income_summary"}
	return pct, *lg, nil
}

// trendChip 計算籌碼面視圖（外資/投信 N 日累計淨買賣）。
// 上市 TWSE-WEB T86（逐日，最多回溯 5 個交易日）；上櫃 TPEx 三大法人明細
// （單日，無逐日歷史，僅以最新一日代表，note 標註）。
func (a *App) trendChip(ctx context.Context, sym model.Symbol, date string, h trendHorizon) (domain.ChipView, model.Lineage, error) {
	view := domain.ChipView{}
	days := h.days
	if days > 5 {
		days = 5 // 兩資料源逐日回溯上限（官方僅供近期）
	}
	if sym.Market == model.MarketOTC {
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, stale, err := fetchNormalize[[]provider.TPExInstitutionalRow](a, ctx, string(provider.TPExInstitutional),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExInstitutional), date, "", vals(params)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExInstitutional, params) })
		if err != nil {
			return view, model.Lineage{}, err
		}
		ttl, _ := a.ttlOf(string(provider.TPExInstitutional))
		lg := postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)
		lg.Grade = model.GradePreview
		lg.DerivedFrom = []string{"TPEX_API:institutional"}
		var row *provider.TPExInstitutionalRow
		for i := range rows {
			if rows[i].Code == sym.Code {
				row = &rows[i]
				break
			}
		}
		if row != nil {
			view.ForeignNetShares5D = row.ForeignNet
			view.TrustNetShares5D = row.InvestmentNet
		}
		return view, *lg, nil
	}
	// 上市：逐日 T86（今起回溯 days 個交易日）。
	day, err := a.prevTradingDay(a.parseDateOrNow(date), 1)
	if err != nil {
		return view, model.Lineage{}, err
	}
	var fSum, tSum int64
	cachedAny, staleAny := false, false
	for i := 0; i < days; i++ {
		params := url.Values{"date": {dateYMD(model.FormatDate(day))}}
		rows, cached, stale, err := fetchNormalize[[]provider.InstitutionalRow](a, ctx, string(provider.TWSEWDInstitutional),
			date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDInstitutional), model.FormatDate(day), "", vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDInstitutional, params) })
		if err != nil {
			break
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		for _, r := range rows {
			if r.Code == sym.Code {
				fSum += r.ForeignNet
				tSum += r.InvestmentNet
			}
		}
		day, err = a.prevTradingDay(day, 1)
		if err != nil {
			break
		}
	}
	view.ForeignNetShares5D = fSum
	view.TrustNetShares5D = tSum
	ttl, _ := a.ttlOf(string(provider.TWSEWDInstitutional))
	lg := postLineage(model.SourceTWSEWeb, date, cachedAny || staleAny, staleAny, ttl)
	lg.Grade = model.GradePreview
	lg.DerivedFrom = []string{"TWSE_WEB:institutional"}
	return view, *lg, nil
}

// parseDateOrNow 解析 YYYY-MM-DD；失敗時回傳 now（趨勢工具內部用）。
func (a *App) parseDateOrNow(date string) time.Time {
	t, err := model.ParseDate(date)
	if err != nil {
		return a.now()
	}
	return t
}

// trendChartMeta 組 _chart_meta：line 型別（§11.2），Y 軸為技術/基本面數值
// （MA5/MA20/MA60/RSI14/PE/PB/殖利率/法人淨買賣），依 §11.3 以標準 builder
// 產出（Line + WithNote），跨來源聚合之 NOTE 標註 Grade PREVIEW。
func trendChartMeta(tech domain.TechnicalView, fund domain.FundamentalView, chip domain.ChipView) *chart.Meta {
	return chart.Line("數值", "indicator", "value",
		chart.WithNote("跨來源聚合（TWSE Web API + MOPS），Grade PREVIEW；value 為各指標數值（MA/RSI/PE/PB/殖利率/法人淨買賣，0 值省略）"))
}

// 編譯期註記：domain/trend 骨架（T026）為業務入口預留；實際引擎於
// pkg/mcp handler（此檔）接線，骨架 ErrNotImplemented 暫不觸發。
