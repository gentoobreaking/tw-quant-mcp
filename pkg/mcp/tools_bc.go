package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/model/domain"
	"tw-quant-mcp/pkg/provider"
)

// tools_bc.go 實作 §10.B（盤後行情・籌碼）與 §10.C（重大訊息與風險）工具。
// 全部為 POST_MARKET 資料：統一經 fetchRaw/fetchNormalize
//（§4.2 快取 TTL + §12.2 讀穿），並於 lineage 標明資料源（v2.1 §4）。

// postLineage 建構盤後工具之 lineage 預設值；cached/stale/ttl 由 handler 依快取
// 結果填入。stale=true（§5.2 stale-if-error）時 freshness=STALE_FALLBACK。
func postLineage(source, dataDate string, cached, stale bool, ttl time.Duration) *model.Lineage {
	freshness := model.FreshnessPostMarket
	if stale {
		freshness = model.FreshnessStaleFallback
	}
	return &model.Lineage{
		Source:     source,
		SourceRole: model.SourceRoleCanonical,
		DataDate:   dataDate,
		Freshness:  freshness,
		IsCached:   cached || stale,
		CacheTTL:   int(ttl.Seconds()),
	}
}

// ttlOf 查 §4.2 政策之資料類別 TTL。
func (a *App) ttlOf(dataset string) (time.Duration, bool) {
	return cache.TTLFor(policyDataset(dataset), a.now())
}

// ************** B. 盤後行情與籌碼 **************

// handlerGetStockDailyQuote：個股單日報價 + helper 技術指標（§10.B）。
// 上市以 TWSE-WEB 日 K（含前 2 個月計算 MA60）；上櫃以 TPEx-API 收盤行情
// （無歷史序列，指標從缺並以 note 說明）。
func handlerGetStockDailyQuote(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	if sym.Market == model.MarketOTC {
		return a.quoteOTC(ctx, sym, date)
	}
	return a.quoteTSE(ctx, sym, date)
}

// quoteTSE 以 3 個月 STOCK_DAY 計算單日報價與指標。
func (a *App) quoteTSE(ctx context.Context, sym model.Symbol, date string) (HandlerResult, error) {
	start, _ := model.ParseDate(date)
	months := monthStarts(start, 3)
	var all []model.Candle
	cachedAny := false
	staleAny := false
	for _, ms := range months {
		params := url.Values{"date": {ms.Format("20060102")}, "stockNo": {sym.Code}}
		rows, cached, stale, err := fetchNormalize[[]model.Candle](a, ctx, string(provider.TWSEWDDailyK),
			date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDDailyK), date, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDDailyK, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		all = append(all, rows...)
	}
	byDay := make(map[string]model.Candle, len(all))
	for _, c := range all {
		byDay[c.Timestamp] = c
	}
	target, ok := byDay[date]
	if !ok {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無日 K 資料（非交易日或無成交）", sym.Code, date)
	}
	closes := make([]float64, 0, len(byDay))
	for _, d := range sortedKeys(byDay) {
		closes = append(closes, byDay[d].Close)
	}
	ind := model.DailyIndicators{
		MA20:  engine.SMA(closes, 20),
		MA60:  engine.SMA(closes, 60),
		RSI14: engine.RSI(closes, 14),
		MACD:  macdOf(closes),
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDDailyK))
	q := model.DailyQuote{
		Symbol: sym.Code, Name: sym.Name, Market: model.MarketTSE, Date: date,
		Open: target.Open, High: target.High, Low: target.Low, Close: target.Close,
		Volume: target.Volume, Amount: target.Amount,
		Indicators: ind,
	}
	lg := postLineage(model.SourceTWSEWeb, date, cachedAny || staleAny, staleAny, ttl)
	lg.DerivedFrom = []string{"TWSE_WEB:daily_k"}
	lg.SourceRole = model.SourceRoleCanonical
	return HandlerResult{Data: q, Lineage: lg}, nil
}

// quoteOTC 以上櫃收盤行情取單日報價。
func (a *App) quoteOTC(ctx context.Context, sym model.Symbol, date string) (HandlerResult, error) {
	params := url.Values{"date": {dateYMD(date)}}
	rows, _, stale, err := fetchNormalize[[]provider.TPExDailyCloseRow](a, ctx, string(provider.TPExDailyClose),
		date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExDailyClose), date, sym.Code, vals(params)),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExDailyClose, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	var row *provider.TPExDailyCloseRow
	for i := range rows {
		if rows[i].Code == sym.Code {
			row = &rows[i]
			break
		}
	}
	if row == nil {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無上櫃收盤資料（非交易日或無成交）", sym.Code, date)
	}
	ttl, _ := a.ttlOf(string(provider.TPExDailyClose))
	q := model.DailyQuote{
		Symbol: sym.Code, Name: sym.Name, Market: model.MarketOTC, Date: row.Date,
		Open: row.Open, High: row.High, Low: row.Low, Close: row.Close,
		Volume: row.Volume,
		Note:   "上櫃指標暫缺：歷史 K 線資料源未接線（T018）",
	}
	return HandlerResult{Data: q, Lineage: postLineage(model.SourceTPExAPI, date, stale, stale, ttl)}, nil
}

// handlerGetStockDailyKline：個股日/週/月 K 線（§10.B）。
// 上市以 TWSE-WEB STOCK_DAY（period/adjust 官方參數）；上櫃資料源未接線。
func handlerGetStockDailyKline(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	if sym.Market == model.MarketOTC {
		return HandlerResult{}, fmt.Errorf("上櫃歷史 K 線資料源（TPEx-API 逐檔歷史）尚未接線；請以 get_stock_daily_quote 查詢最新日")
	}
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	period, _ := args["period"].(string)
	if period == "" {
		period = "day"
	}
	adjust := false
	if v, ok := args["adjust"]; ok {
		adjust, _ = v.(bool)
	}
	params := url.Values{"date": {dateYMD(date)}, "stockNo": {sym.Code}}
	if period != "day" {
		params.Set("period", period)
	}
	if adjust {
		params.Set("adjust", "Y")
	}
	rows, cached, stale, err := fetchNormalize[[]model.Candle](a, ctx, string(provider.TWSEWDDailyK),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDDailyK), date, sym.Code, vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDDailyK, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	if len(rows) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無日 K 資料（非交易日或無成交）", sym.Code, date)
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDDailyK))
	lg := postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)
	lg.SamplingSec = 0
	return HandlerResult{Data: rows, Lineage: lg}, nil
}

// handlerGetMarketSummary：全市場漲跌家數/成交量/漲跌停（§10.B）。
func handlerGetMarketSummary(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	tse, cachedTSE, staleTSE, err := a.marketStatsTSE(ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	otc, cachedOTC, staleOTC, err := a.marketStatsOTC(ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDMarketClose))
	lg := postLineage(model.SourceTWSEWeb, date, cachedTSE || cachedOTC, staleTSE || staleOTC, ttl)
	lg.SourceRole = model.SourceRoleCanonical
	return HandlerResult{Data: model.MarketSummary{Date: date, TSE: tse, OTC: otc}, Lineage: lg}, nil
}

func (a *App) marketStatsTSE(ctx context.Context, date string) (model.MarketStats, bool, bool, error) {
	// MI_INDEX 需 type=ALL 才回傳「每日收盤行情」表（§12.4 全市場彙總）。
	params := url.Values{"date": {dateYMD(date)}, "type": {"ALL"}}
	rows, cached, stale, err := fetchNormalize[[]provider.MarketCloseRow](a, ctx, string(provider.TWSEWDMarketClose),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDMarketClose), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDMarketClose, params) })
	if err != nil {
		return model.MarketStats{}, false, false, err
	}
	var st model.MarketStats
	for _, r := range rows {
		st.TotalVolume += r.Volume
		st.TotalAmount += r.Amount
		switch r.ChangeDir {
		case "+":
			st.Advancers++
			if r.Change > 0 && engine.IsLimitUp(r.Close, r.Close-r.Change) {
				st.LimitUp++
			}
		case "-":
			st.Decliners++
			if r.Change < 0 && engine.IsLimitDown(r.Close, r.Close-r.Change) {
				st.LimitDown++
			}
		default:
			st.Unchanged++
		}
	}
	return st, cached, stale, nil
}

func (a *App) marketStatsOTC(ctx context.Context, date string) (model.MarketStats, bool, bool, error) {
	params := url.Values{"date": {dateYMD(date)}}
	rows, cached, stale, err := fetchNormalize[[]provider.TPExDailyCloseRow](a, ctx, string(provider.TPExDailyClose),
		date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExDailyClose), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExDailyClose, params) })
	if err != nil {
		return model.MarketStats{}, false, false, err
	}
	var st model.MarketStats
	for _, r := range rows {
		st.TotalVolume += r.Volume
		switch r.ChangeDir {
		case "+":
			st.Advancers++
			if r.Change > 0 && engine.IsLimitUp(r.Close, r.Close-r.Change) {
				st.LimitUp++
			}
		case "-":
			st.Decliners++
			if r.Change < 0 && engine.IsLimitDown(r.Close, r.Close-r.Change) {
				st.LimitDown++
			}
		default:
			st.Unchanged++
		}
	}
	return st, cached, stale, nil
}

// handlerGetInstitutionalInvestors：三大法人買賣超（個股+彙總，§10.B）。
func handlerGetInstitutionalInvestors(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	market, _ := args["market"].(string)
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDInstitutional))
	switch market {
	case model.MarketTSE:
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, stale, err := fetchNormalize[[]provider.InstitutionalRow](a, ctx, string(provider.TWSEWDInstitutional),
			date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDInstitutional), date, "", vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDInstitutional, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		var total int64
		for _, r := range rows {
			total += r.ForeignNet + r.ForeignDealerNet + r.InvestmentNet
		}
		return HandlerResult{Data: model.InstitutionalSummary{
			Market: market, Date: date, Rows: rows, TotalNet: total,
			Note: institutionalNote(a.now(), date),
		}, Lineage: postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)}, nil
	case model.MarketOTC:
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, stale, err := fetchNormalize[[]provider.TPExInstitutionalRow](a, ctx, string(provider.TPExInstitutional),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExInstitutional), date, "", vals(params)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExInstitutional, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		var total int64
		for _, r := range rows {
			total += r.ForeignNet + r.ForeignDealerNet + r.InvestmentNet
		}
		return HandlerResult{Data: model.InstitutionalSummary{
			Market: market, Date: date, Rows: rows, TotalNet: total,
			Note: institutionalNote(a.now(), date),
		}, Lineage: postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)}, nil
	}
	return HandlerResult{}, fmt.Errorf("參數 market 僅允許 tse|otc")
}

// institutionalNote：15:00 前法人資料可能未齊全之註記。
func institutionalNote(now time.Time, date string) string {
	if model.FormatDate(now) == date && now.Hour()*3600+now.Minute()*60 < 15*3600 {
		return "三大法人資料當日 15:00 後始完全釋出；目前為部分資料"
	}
	return ""
}

// handlerGetForeignIndustryHoldings：外資產業配置（§10.B，chart pie）。
func handlerGetForeignIndustryHoldings(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	rows, cached, stale, err := fetchNormalize[[]provider.ForeignHoldingRow](a, ctx, string(provider.TWSEAPIForeignHoldings),
		date, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIForeignHoldings), date, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIForeignHoldings, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIForeignHoldings))
	return HandlerResult{Data: rows, Lineage: postLineage(model.SourceTWSEAPI, date, cached || stale, stale, ttl)}, nil
}

// qfiisSelectTypeByCategory：MI_QFIIS selectType（產業別代碼）→ Symbol
// Registry 產業類別名稱之對照。2026-08 實測：TWSE 已將 MI_QFIIS 改為
// 依 selectType 產業別查詢，未帶參數時僅回傳類別 01（水泥，約 8 列），
// 導致 get_foreign_shareholding_history 恆回「無資料」。
var qfiisSelectTypeByCategory = map[string]string{
	"水泥工業": "01", "食品工業": "02", "塑膠工業": "03", "紡織纖維": "04",
	"電機機械": "05", "電器電纜": "06", "玻璃陶瓷": "08", "造紙工業": "09",
	"鋼鐵工業": "10", "橡膠工業": "11", "汽車工業": "12", "建材營造": "14",
	"航運業": "15", "觀光餐旅": "16", "金融保險業": "17", "貿易百貨": "18",
	"其他": "20", "化學工業": "21", "生技醫療業": "22", "油電燃氣業": "23",
	"半導體業": "24", "電腦及週邊設備業": "25", "光電業": "26", "通信網路業": "27",
	"電子零組件業": "28", "電子通路業": "29", "資訊服務業": "30", "其他電子業": "31",
	"綠能環保": "35", "數位雲端": "36", "運動休閒": "37", "居家生活": "38",
}

// fetchQFIISDay 取得指定日期、指定 selectType 類別之外資持股快照。
func (a *App) fetchQFIISDay(ctx context.Context, d, sel string) (
	[]provider.ForeignHoldingPointRow, bool, bool, error) {
	params := url.Values{"dayDate": {dateYMD(d)}, "selectType": {sel}}
	return fetchNormalize[[]provider.ForeignHoldingPointRow](a, ctx,
		string(provider.TWSEWDForeignQFIIS), d,
		cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDForeignQFIIS), d, "", vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDForeignQFIIS, params) })
}

// qfiisDayPoints 取得單日快照中 code 之持股點。先查 sym.Category 對應類別；
// 未命中時掃描全部類別（01..40，含未映射之保留/特殊類別）。掃描路徑較慢
// （受上游 rate limit），但結果會寫入 per-(date,selectType) 快取。
func (a *App) qfiisDayPoints(ctx context.Context, d, code, category string) (
	*model.ForeignHoldingPoint, bool, bool, error) {
	type result struct {
		rows          []provider.ForeignHoldingPointRow
		cached, stale bool
	}
	fetchOne := func(sel string) (*result, error) {
		rows, cached, stale, err := a.fetchQFIISDay(ctx, d, sel)
		if err != nil {
			return nil, err
		}
		return &result{rows: rows, cached: cached, stale: stale}, nil
	}
	pick := func(res *result) *model.ForeignHoldingPoint {
		for _, r := range res.rows {
			if r.Code == code {
				return &model.ForeignHoldingPoint{
					Date: r.Date, ForeignShares: r.ForeignShares, ForeignPercent: r.ForeignPercent,
				}
			}
		}
		return nil
	}
	primary, mapped := qfiisSelectTypeByCategory[category]
	if !mapped {
		primary = "01"
	}
	var lastErr error
	anyOK := false
	var cachedAny, staleAny bool
	pickFrom := func(res *result) *model.ForeignHoldingPoint {
		cachedAny = cachedAny || res.cached
		staleAny = staleAny || res.stale
		return pick(res)
	}
	res, err := fetchOne(primary)
	if err != nil {
		lastErr = err
	} else {
		anyOK = true
		if p := pickFrom(res); p != nil {
			return p, cachedAny, staleAny, nil
		}
	}
	// fallback：掃描其餘類別（空類別或暫時性錯誤則略過）
	for i := 1; i <= 40; i++ {
		sel := fmt.Sprintf("%02d", i)
		if sel == primary {
			continue
		}
		r2, err2 := fetchOne(sel)
		if err2 != nil {
			if lastErr == nil {
				lastErr = err2
			}
			continue
		}
		anyOK = true
		if p := pickFrom(r2); p != nil {
			return p, cachedAny, staleAny, nil
		}
	}
	if !anyOK && lastErr != nil {
		return nil, false, false, lastErr
	}
	return nil, cachedAny, staleAny, nil
}

// handlerGetForeignShareholdingHistory：外資持股歷史（§10.B，逐日快照）。
func handlerGetForeignShareholdingHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	if sym.Market == model.MarketOTC {
		return HandlerResult{}, fmt.Errorf("外資持股統計僅涵蓋上市股票（TWSE-WEB MI_QFIIS）；上櫃外資持股暫未提供")
	}
	rnge := 5
	if v, ok := args["range"]; ok {
		if n, err := asInt(v); err == nil {
			rnge = n
		}
	}
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	series := make([]model.ForeignHoldingPoint, 0, rnge)
	seenDates := make(map[string]bool, rnge)
	snapshotTruncated := false
	cachedAny := false
	staleAny := false
	day, _ := model.ParseDate(date)
	for i := 0; i < rnge; i++ {
		d := model.FormatDate(day)
		point, cached, stale, err := a.qfiisDayPoints(ctx, d, sym.Code, sym.Category)
		if err != nil {
			return HandlerResult{}, err
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		// 2026-08 起 MI_QFIIS 對過去日期恆回最新快照（resp.date 固定），
		// 以快照日期去重，避免同一快照重複出現偽裝成歷史序列。
		if point != nil {
			if seenDates[point.Date] {
				snapshotTruncated = true
				break
			}
			seenDates[point.Date] = true
			series = append(series, *point)
		}
		if prev, err := a.prevTradingDay(day, 1); err == nil {
			day = prev
		} else {
			break
		}
	}
	if len(series) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 起 %d 個交易日無外資持股資料", sym.Code, date, rnge)
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDForeignQFIIS))
	lg := postLineage(model.SourceTWSEWeb, date, cachedAny || staleAny, staleAny, ttl)
	note := ""
	if snapshotTruncated {
		note = "MI_QFIIS 現僅提供最新快照，歷史交易日資料已不再供查詢，series 僅含可取得之快照"
	}
	return HandlerResult{Data: model.ForeignShareholdingHistory{
		Symbol: sym.Code, Name: sym.Name, Range: rnge, Series: series, Note: note,
	}, Lineage: lg}, nil
}

// handlerGetMarginTrading：單檔融資融券（§10.B）。
func handlerGetMarginTrading(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	if sym.Market == model.MarketOTC {
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, stale, err := fetchNormalize[[]provider.TPExMarginRow](a, ctx, string(provider.TPExMargin),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExMargin), date, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExMargin, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		for _, r := range rows {
			if r.Code == sym.Code {
				ttl, _ := a.ttlOf(string(provider.TPExMargin))
				return HandlerResult{Data: r, Lineage: postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)}, nil
			}
		}
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無上櫃融資融券資料", sym.Code, date)
	}
	params := url.Values{"date": {dateYMD(date)}, "selectType": {"ALL"}}
	rows, cached, stale, err := fetchNormalize[[]provider.MarginRow](a, ctx, string(provider.TWSEWDMargin),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDMargin), date, sym.Code, vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDMargin, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range rows {
		if r.Code == sym.Code {
			ttl, _ := a.ttlOf(string(provider.TWSEWDMargin))
			return HandlerResult{Data: r, Lineage: postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)}, nil
		}
	}
	return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無融資融券資料", sym.Code, date)
}

// handlerGetAbnormalTrading：異常成交量（注意股）排名（§10.B）。
func handlerGetAbnormalTrading(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	market, _ := args["market"].(string)
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	topN := 20
	if v, ok := args["top_n"]; ok {
		if n, err := asInt(v); err == nil {
			topN = n
		}
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDAbnormal))
	if market == model.MarketOTC {
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, stale, err := fetchNormalize[[]provider.TPExAttentionRow](a, ctx, string(provider.TPExAttention),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExAttention), date, "", vals(params)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExAttention, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		out := make([]model.AbnormalTrade, 0, len(rows))
		for _, r := range rows {
			out = append(out, model.AbnormalTrade{Code: r.Code, Name: r.Name, Info: r.Info})
		}
		if len(out) > topN {
			out = out[:topN]
		}
		return HandlerResult{Data: out, Lineage: postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)}, nil
	}
	params := url.Values{"date": {dateYMD(date)}}
	rows, cached, stale, err := fetchNormalize[[]provider.AbnormalVolumeRow](a, ctx, string(provider.TWSEWDAbnormal),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDAbnormal), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDAbnormal, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	out := make([]model.AbnormalTrade, 0, len(rows))
	for _, r := range rows {
		out = append(out, model.AbnormalTrade{
			Code: r.Code, Name: r.Name, NoticeCount: int64(r.NoticeCount), Info: r.Info,
		})
	}
	if len(out) > topN {
		out = out[:topN]
	}
	return HandlerResult{Data: out, Lineage: postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)}, nil
}

// handlerGetWarrantActivity：權證活躍度（成交金額/張數排名，§10.B）。
// handlerGetWarrantBasicInfo：權證基本資料查詢（TWSE-API t187ap37_L，T187）。
// code 可為：
//   - 權證代號（精確比對 權證代號 欄）；或
//   - 標的證券代號（經 Symbol Registry 解析名稱後與 標的證券/指數 欄比對，
//     因上游該欄存名稱而非代號，如 2330 → 台積電）。
//
// 省略時分頁回傳全部（limit 預設 100；全量約 3.8 萬列不宜一次回傳）。
func handlerGetWarrantBasicInfo(a *App, args map[string]any) (HandlerResult, error) {
	code := strVal(args["code"])
	limit, offset := 100, 0
	if v, ok := args["limit"]; ok {
		if n, err := asInt(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v, ok := args["offset"]; ok {
		if n, err := asInt(v); err == nil && n >= 0 {
			offset = n
		}
	}
	var underlyingName string
	if code != "" {
		if sym, err := a.symbolOf(code); err == nil {
			underlyingName = sym.Name // 標的證券代號 → 名稱（如 2330 → 台積電）
		}
	}
	ctx := context.Background()
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchFinancialRowsForCode(a, ctx,
		provider.TWSEAPIWarrantBasic, dataDate, "")
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIWarrantBasic))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)
	out := make([]any, 0, limit)
	matched := 0
	for _, r := range rows {
		if !warrantBasicMatches(r, code, underlyingName) {
			continue
		}
		if matched < offset {
			matched++
			continue
		}
		if len(out) >= limit {
			break
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		if code != "" {
			return HandlerResult{}, fmt.Errorf("查無 %s 之權證基本資料（權證代號或標的代號皆未命中）", code)
		}
		return HandlerResult{Data: []any{}, Lineage: lineage}, nil
	}
	return HandlerResult{Data: out, Lineage: lineage}, nil
}

// warrantBasicMatches 判定單列是否符合 code（權證代號或標的名稱）。
func warrantBasicMatches(r map[string]any, code, underlyingName string) bool {
	if code == "" {
		return true
	}
	if rowField(r, "權證代號", "code") == code {
		return true
	}
	if underlyingName == "" {
		return false
	}
	target := rowField(r, "標的證券/指數")
	return target != "" && strings.Contains(target, underlyingName)
}

func handlerGetWarrantActivity(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	rows, cached, stale, err := fetchNormalize[[]provider.WarrantRow](a, ctx, string(provider.TWSEAPIWarrants),
		date, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIWarrants), date, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIWarrants, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	topN := 10
	if v, ok := args["top_n"]; ok {
		if n, err := asInt(v); err == nil {
			topN = n
		}
	}
	amount := append([]provider.WarrantRow(nil), rows...)
	sort.Slice(amount, func(i, j int) bool { return amount[i].Amount > amount[j].Amount })
	volume := append([]provider.WarrantRow(nil), rows...)
	sort.Slice(volume, func(i, j int) bool { return volume[i].Volume > volume[j].Volume })
	if len(amount) > topN {
		amount = amount[:topN]
	}
	if len(volume) > topN {
		volume = volume[:topN]
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIWarrants))
	return HandlerResult{Data: model.WarrantSummary{
		Date: date, AmountTop: toAny(amount), VolumeTop: toAny(volume),
	}, Lineage: postLineage(model.SourceTWSEAPI, date, cached || stale, stale, ttl)}, nil
}

// ************** C. 重大訊息與風險 **************

// handlerGetMajorAnnouncements：MOPS 重大訊息（§10.C）。
func handlerGetMajorAnnouncements(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	date, _ := args["date"].(string)
	symbol, _ := args["symbol"].(string)
	keyword, _ := args["keyword"].(string)

	if a.mops == nil {
		return HandlerResult{}, fmt.Errorf("重大訊息資料源（MOPS）尚未接線")
	}

	// 快取全量（key 不含過濾參數）：一次下載供各 symbol/date/keyword 組合共用，
	// 過濾於快取讀取後於記憶體進行（重大訊息 TTL 5min，§4.2）
	dataset := string(provider.MOPSAnnouncements)
	dataDate := date
	if dataDate == "" {
		dataDate = a.now().Format("2006-01-02")
	}

	key := cache.KeyString(model.SourceMOPS, dataset, dataDate, "", nil)

	cached, stale, raw, err := a.fetchRaw(ctx, dataset, dataDate, key, func() ([]byte, error) {
		req := provider.RawRequest{
			URL: a.mops.URL(provider.MOPSAnnouncements, nil),
		}
		resp, fetchErr := a.mops.Fetch(ctx, req)
		if fetchErr != nil {
			return nil, fetchErr
		}
		if valErr := a.mops.Validate(resp); valErr != nil {
			return nil, valErr
		}
		return a.mops.Normalize(resp)
	})
	if err != nil {
		return HandlerResult{}, fmt.Errorf("MOPS 重大訊息取得失敗: %w", err)
	}

	var all []model.MajorAnnouncement
	if err := json.Unmarshal(raw, &all); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: 重大訊息解析失敗: %w", err)
	}

	ttl, _ := a.ttlOf(dataset)
	return HandlerResult{
		Data:    filterAnnouncements(all, date, symbol, keyword),
		Lineage: postLineage(model.SourceMOPS, dataDate, cached || stale, stale, ttl),
	}, nil
}

// filterAnnouncements 依日期/symbol/關鍵字過濾重大訊息。
func filterAnnouncements(rows []model.MajorAnnouncement, date, symbol, keyword string) []model.MajorAnnouncement {
	filtered := make([]model.MajorAnnouncement, 0, len(rows))
	for _, r := range rows {
		if date != "" && r.AnnounceDate != date {
			continue
		}
		if symbol != "" && r.Code != symbol {
			continue
		}
		if keyword != "" && !strings.Contains(r.Subject, keyword) &&
			!strings.Contains(r.Description, keyword) {
			continue
		}
		filtered = append(filtered, r)
	}
	return filtered
}

// handlerGetAttentionDispositionStocks：注意股/處置股清單（§10.C）。
// 上市：TWSE-WEB notice（注意）+ TWSE-API punish（處置）；
// 上櫃：TPEx-API attention/disposition。成功後餵入 DaytradeScanner
// （T010 scan_daytrade_eligibility 之名單供應器）。
func handlerGetAttentionDispositionStocks(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	market, _ := args["market"].(string)
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDAbnormal))
	out := model.AttentionDispositionList{Market: market, Date: date}
	var alerts []AlertList
	if market == model.MarketOTC {
		ap := url.Values{"date": {dateYMD(date)}}
		att, cached, stale, err := fetchNormalize[[]provider.TPExAttentionRow](a, ctx, string(provider.TPExAttention),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExAttention), date, "", vals(ap)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExAttention, ap) })
		if err != nil {
			return HandlerResult{}, err
		}
		dp := url.Values{"date": {dateYMD(date)}}
		disp, dCached, dStale, err := fetchNormalize[[]provider.TPExDispositionRow](a, ctx, string(provider.TPExDisposition),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExDisposition), date, "", vals(dp)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExDisposition, dp) })
		if err != nil {
			return HandlerResult{}, err
		}
		for _, r := range att {
			out.Attention = append(out.Attention, model.AttentionStock{Code: r.Code, Name: r.Name, Info: r.Info})
			alerts = append(alerts, AlertList{Scope: market, Kind: "attention", Code: r.Code, Info: r.Info})
		}
		for _, r := range disp {
			out.Disposition = append(out.Disposition, model.DispositionStock{
				Code: r.Code, Name: r.Name, Period: r.Period, Reason: r.Reasons,
			})
			alerts = append(alerts, AlertList{Scope: market, Kind: "disposition", Code: r.Code, Info: r.Reasons, Period: r.Period})
		}
		a.risk.AddLists(date, market, alerts)
		return HandlerResult{Data: out, Lineage: postLineage(model.SourceTPExAPI, date, cached || dCached || stale || dStale, stale || dStale, ttl)}, nil
	}
	ap := url.Values{"date": {dateYMD(date)}}
	att, cached, stale, err := fetchNormalize[[]provider.AbnormalVolumeRow](a, ctx, string(provider.TWSEWDAbnormal),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDAbnormal), date, "", vals(ap)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDAbnormal, ap) })
	if err != nil {
		return HandlerResult{}, err
	}
	dp := url.Values{"date": {dateYMD(date)}}
	disp, dCached, dStale, err := fetchNormalize[[]provider.PunishRow](a, ctx, string(provider.TWSEAPIPunish),
		date, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIPunish), date, "", vals(dp)),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIPunish, dp) })
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range att {
		out.Attention = append(out.Attention, model.AttentionStock{Code: r.Code, Name: r.Name, Info: r.Info})
		alerts = append(alerts, AlertList{Scope: market, Kind: "attention", Code: r.Code, Info: r.Info})
	}
	for _, r := range disp {
		out.Disposition = append(out.Disposition, model.DispositionStock{
			Code: r.Code, Name: r.Name, Period: r.DispositionPeriod, Reason: r.Reasons,
			Measure: r.DispositionMeasure, Detail: r.Detail,
		})
		alerts = append(alerts, AlertList{Scope: market, Kind: "disposition", Code: r.Code, Info: r.Reasons, Period: r.DispositionPeriod})
	}
	out.Note = "上市處置股來源 TWSE-API announcement/punish；注意股來源 TWSE-WEB announcement/notice"
	a.risk.AddLists(date, market, alerts)
	return HandlerResult{Data: out, Lineage: postLineage(model.SourceTWSEWeb, date, cached || dCached || stale || dStale, stale || dStale, ttl)}, nil
}

// ************** 共用 fetch / 工具 **************

// fetchWebRaw：TWSE-WEB URL 建構 → Fetch → Validate → Normalize。
func (a *App) fetchWebRaw(ctx context.Context, ds provider.TWSEWebDataset, params url.Values) ([]byte, error) {
	u := a.twseWeb.URL(ds, params)
	resp, err := a.twseWeb.Fetch(ctx, provider.RawRequest{URL: u})
	if err != nil {
		return nil, err
	}
	if err := a.twseWeb.Validate(resp); err != nil {
		return nil, err
	}
	return a.twseWeb.Normalize(resp)
}

// fetchAPIRaw：TWSE-API 資料源版 fetchWebRaw。
func (a *App) fetchAPIRaw(ctx context.Context, ds provider.TWSEAPIDataset, params url.Values) ([]byte, error) {
	u := a.twseAPI.URL(ds, params)
	resp, err := a.twseAPI.Fetch(ctx, provider.RawRequest{URL: u})
	if err != nil {
		return nil, err
	}
	if err := a.twseAPI.Validate(resp); err != nil {
		return nil, err
	}
	return a.twseAPI.Normalize(resp)
}

// fetchTPExRaw：TPEx-API 資料源版 fetchWebRaw。
func (a *App) fetchTPExRaw(ctx context.Context, ds provider.TPExDataset, params url.Values) ([]byte, error) {
	u := a.tpex.URL(ds, params)
	resp, err := a.tpex.Fetch(ctx, provider.RawRequest{URL: u})
	if err != nil {
		return nil, err
	}
	if err := a.tpex.Validate(resp); err != nil {
		return nil, err
	}
	return a.tpex.Normalize(resp)
}

// strVal 回傳參數字串值（缺省為空字串）。
func strVal(v any) string {
	s, _ := v.(string)
	return s
}

// dateYMD 將 YYYY-MM-DD 轉為官方 YYYYMMDD（請求參數）。
func dateYMD(date string) string {
	return date[:4] + date[5:7] + date[8:10]
}

// monthStarts 回傳含 d 之月起往前 n 個月初（用於指標計算之月份序列）。
func monthStarts(d time.Time, n int) []time.Time {
	out := make([]time.Time, 0, n)
	y, m := d.Year(), d.Month()
	for i := 0; i < n; i++ {
		out = append(out, time.Date(y, m, 1, 0, 0, 0, 0, model.Taipei()))
		m--
		if m == 0 {
			m = 12
			y--
		}
	}
	return out
}

// sortedKeys 回傳 map 之排序鍵（日期序列）。
func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// macdOf 以收盤序列計算 MACD（不足窗口為零值）。
func macdOf(closes []float64) model.MACDPoint {
	m := engine.MACD(closes)
	return model.MACDPoint{MACD: m.MACD, Signal: m.Signal, Hist: m.Hist}
}

// toAny 將 []T 轉為 []any（JSON 序列化兼容）。
func toAny[T any](s []T) []any {
	out := make([]any, 0, len(s))
	for _, v := range s {
		out = append(out, v)
	}
	return out
}

// vals 將 url.Values 轉為 map[string]string（快取鍵參數）。
func vals(v url.Values) map[string]string {
	if len(v) == 0 {
		return nil
	}
	out := make(map[string]string, len(v))
	for k, vs := range v {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}

// ************** 指數工具 **************

// handlerGetTWSEIndex：查詢 TWSE 指數盤後行情與歷史日 K（§10.B）。
// symbol 為指數名稱（省略預設「發行量加權股價指數」）；
// date 省略時為最近交易日。
// 資料路徑 1（單日指數收盤）：TWSE-API MI_INDEX（openapi.twse.com.tw/v1/exchangeReport/MI_INDEX），
//
//	依 IndexName 過濾，輸出 IndexQuoteRow（收盤指數/漲跌/漲跌百分比）。
//
// 資料路徑 2（歷史日 K）：TWSE-WEB MI_5MINS_HIST（www.twse.com.tw/indicesReport/MI_5MINS_HIST?date=YYYYMMDD），
//
//	請求月份資料，回傳該月每日 OHLC（IndexRow）。
//
// 輸出：domain.IndexView，含 _chart_meta（line 型別，history 序列）。
func handlerGetTWSEIndex(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	symbol, _ := args["symbol"].(string)
	if symbol == "" {
		symbol = "發行量加權股價指數" // 預設加權指數
	}
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}

	// 路徑 1：單日指數收盤（TWSE-API MI_INDEX）
	params := url.Values{}
	rowsIdx, cachedIdx, staleIdx, err := fetchNormalize[[]provider.IndexQuoteRow](a, ctx, string(provider.TWSEAPIIndices),
		date, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIIndices), date, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIIndices, params) })
	if err != nil {
		return HandlerResult{}, err
	}

	var quote *provider.IndexQuoteRow
	for i := range rowsIdx {
		if rowsIdx[i].IndexName == symbol {
			quote = &rowsIdx[i]
			break
		}
	}
	if quote == nil {
		return HandlerResult{}, fmt.Errorf("指數 %q 於 %s 無收盤資料", symbol, date)
	}

	// 路徑 2：歷史日 K（TWSE-WEB MI_5MINS_HIST，整月請求）
	// 以 date 所在月份為請求參數
	histDate, _ := model.ParseDate(date)
	monthParam := histDate.Format("200601") + "01" // 該月第一天
	histParams := url.Values{"date": {monthParam}}
	rowsHist, cachedHist, staleHist, err := fetchNormalize[[]provider.IndexRow](a, ctx, string(provider.TWSEWDIndexHistory),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDIndexHistory), date, "", vals(histParams)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDIndexHistory, histParams) })
	if err != nil {
		return HandlerResult{}, err
	}

	// 轉為 domain.IndexDay
	history := make([]domain.IndexDay, 0, len(rowsHist))
	for _, r := range rowsHist {
		history = append(history, domain.IndexDay{
			Date:  r.Date,
			Open:  r.Open,
			High:  r.High,
			Low:   r.Low,
			Close: r.Close,
		})
	}

	cachedAny := cachedIdx || cachedHist
	staleAny := staleIdx || staleHist
	ttl, _ := a.ttlOf(string(provider.TWSEAPIIndices)) // 兩資料集同 TTL 政策

	view := domain.IndexView{
		Name:          quote.IndexName,
		Date:          quote.Date,
		Close:         quote.Close,
		Change:        quote.Change,
		ChangePercent: quote.ChangePercent,
		ChangeDir:     quote.ChangeDir,
		Note:          quote.Note,
		History:       history,
		ChartMeta: &domain.IndexChartMeta{
			Type:   "line",
			Series: []string{"date", "open", "high", "low", "close"},
		},
	}

	lg := postLineage(model.SourceTWSEWeb, date, cachedAny || staleAny, staleAny, ttl)
	lg.DerivedFrom = []string{"TWSE_WEB:index_history", "TWSE_API:indices"}
	lg.SourceRole = model.SourceRoleCanonical

	return HandlerResult{Data: view, Lineage: lg}, nil
}

// handlerGetAfterHoursTrading：盤後定價交易（BFT41U，T040）。
func handlerGetAfterHoursTrading(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["code"].(string)
	limit, offset := 50, 0
	if v, ok := args["limit"]; ok {
		if n, err := asInt(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v, ok := args["offset"]; ok {
		if n, err := asInt(v); err == nil && n >= 0 {
			offset = n
		}
	}
	date, err := a.resolveDate("")
	if err != nil {
		return HandlerResult{}, err
	}
	rows, cached, stale, err := fetchNormalize[[]provider.AfterHoursRow](a, ctx,
		string(provider.TWSEWDAfterHours), date,
		cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDAfterHours), date, code, nil),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDAfterHours, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	if code != "" {
		filtered := rows[:0:0]
		for _, r := range rows {
			if r.Code == code {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	if offset < len(rows) {
		rows = rows[offset:]
	} else {
		rows = rows[:0]
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDAfterHours))
	return HandlerResult{Data: rows, Lineage: postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)}, nil
}

// ── 上櫃市場（T155/T156/T157）──

// otcPaginate 通用分頁（offset 越界回空陣列）。
func otcPaginate[T any](rows []T, offset, limit int) []T {
	if offset < len(rows) {
		rows = rows[offset:]
	} else {
		return []T{}
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows
}

// handlerGetOtcDaily：上櫃市場當日收盤行情（T155）。
func handlerGetOtcDaily(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	limit, offset := listPaging(args)
	stockNo := strVal(args["stock_no"])
	date := a.now().Format("2006-01-02")
	params := url.Values{}
	if stockNo != "" {
		params.Set("stockNo", stockNo)
		offset = 0
	}
	rows, cached, stale, err := fetchNormalize[[]provider.TPExDailyCloseRow](a, ctx,
		string(provider.TPExOtcDaily), date,
		cache.KeyString(model.SourceTPExAPI, string(provider.TPExOtcDaily), date, stockNo, vals(params)),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExOtcDaily, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(cache.DatasetDailyKLine) // TPExOtcDaily 與 daily_kline 同政策（T155）
	lineage := postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)
	return HandlerResult{Data: otcPaginate(rows, offset, limit), Lineage: lineage}, nil
}

// handlerGetOtcIndex：櫃買指數歷史行情（T156；官方恆回歷史序列，依日期新→舊排序）。
func handlerGetOtcIndex(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	limit, offset := listPaging(args)
	date := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]provider.TPExIndexRow](a, ctx,
		string(provider.TPExIndices), date,
		cache.KeyString(model.SourceTPExAPI, string(provider.TPExIndices), date, "", nil),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExIndices, url.Values{}) })
	if err != nil {
		return HandlerResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Date > rows[j].Date })
	ttl, _ := a.ttlOf(cache.DatasetDailyKLine) // 櫃買指數歷史同 daily_kline 政策（T156）
	lineage := postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)
	return HandlerResult{Data: otcPaginate(rows, offset, limit), Lineage: lineage}, nil
}

// handlerGetOtcOddLot：上櫃零股交易行情（T157）。
func handlerGetOtcOddLot(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	limit, offset := listPaging(args)
	stockNo := strVal(args["stock_no"])
	date := a.now().Format("2006-01-02")
	params := url.Values{}
	if stockNo != "" {
		params.Set("stockNo", stockNo)
		offset = 0
	}
	rows, cached, stale, err := fetchNormalize[[]provider.TPExOddLotRow](a, ctx,
		string(provider.TPExOddLot), date,
		cache.KeyString(model.SourceTPExAPI, string(provider.TPExOddLot), date, stockNo, vals(params)),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExOddLot, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(cache.DatasetAlertStock) // 上櫃零股同 alert_stock 政策（T157）
	lineage := postLineage(model.SourceTPExAPI, date, cached || stale, stale, ttl)
	return HandlerResult{Data: otcPaginate(rows, offset, limit), Lineage: lineage}, nil
}

// listPaging 解析 limit/offset 分頁參數（預設 50/0）。
func listPaging(args map[string]any) (int, int) {
	limit, offset := 50, 0
	if v, ok := args["limit"]; ok {
		if n, err := asInt(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v, ok := args["offset"]; ok {
		if n, err := asInt(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// handlerGetTwseEvents：證交所活動訊息（news/eventList，T191）。
// top 為回傳筆數上限（預設 10；0 表全部，對齊遠端同名工具語意）。
func handlerGetTwseEvents(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	top := 10
	if v, ok := args["top"]; ok {
		if n, err := asInt(v); err == nil && n >= 0 {
			top = n
		}
	}
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]map[string]any](a, ctx,
		string(provider.TWSEAPITwseEvents), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPITwseEvents), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPITwseEvents, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPITwseEvents))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)
	if rows == nil {
		rows = []map[string]any{}
	}
	if top > 0 && len(rows) > top {
		rows = rows[:top]
	}
	return HandlerResult{Data: rows, Lineage: lineage}, nil
}

// handlerGetAllStocksDailyClose：指定日期全市場逐檔收盤行情（T192）。
// 「單一日期 × 全市場」快照，與 get_stock_daily_quote（個股跨日）互補。
// 上游 MI_INDEX type=ALLBUT0999 之「每日收盤行情」表；stock_no/name 為
// 本地端過濾；date 相容 YYYYMMDD 與 YYYY-MM-DD（本機慣例）。
func handlerGetAllStocksDailyClose(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	rawDate := strVal(args["date"])
	if len(rawDate) == 8 && !strings.Contains(rawDate, "-") {
		rawDate = rawDate[:4] + "-" + rawDate[4:6] + "-" + rawDate[6:]
	}
	date, err := a.resolveDate(rawDate)
	if err != nil {
		return HandlerResult{}, err
	}
	limit, offset := listPaging(args)
	stockNo, nameArg := strVal(args["stock_no"]), strVal(args["name"])

	params := url.Values{"date": {dateYMD(date)}, "type": {"ALLBUT0999"}}
	rows, cached, stale, err := fetchNormalize[[]provider.MarketCloseRow](a, ctx,
		string(provider.TWSEWDMarketClose), date,
		cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDMarketClose), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDMarketClose, params) })
	if err != nil {
		return HandlerResult{}, err
	}

	ttl, _ := a.ttlOf(string(provider.TWSEWDMarketClose))
	lineage := postLineage(model.SourceTWSEWeb, date, cached || stale, stale, ttl)

	out := make([]provider.MarketCloseRow, 0, len(rows))
	for _, r := range rows {
		if stockNo != "" && r.Code != stockNo {
			continue
		}
		if nameArg != "" && !strings.Contains(r.Name, nameArg) {
			continue
		}
		out = append(out, r)
	}
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []provider.MarketCloseRow{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: lineage}, nil
}

// handlerGetAbnormalAccumulatedNoticeStocks：集中市場公布注意累計次數異常
// 資訊（announcement/notetrans，T193）。與 get_attention_disposition_stocks
// （當日注意/處置清單）互補：本工具揭露「近期符合注意處理標準」之累計紀錄。
// 清單含權證（Code 為 6 碼），原樣回傳不靜默丟棄；kind 選填可供過濾
//（"stock"=4 碼普通股、"warrant"=6 碼權證）。
func handlerGetAbnormalAccumulatedNoticeStocks(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	limit, offset := listPaging(args)
	nameArg, kind := strVal(args["name"]), strVal(args["kind"])

	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]map[string]any](a, ctx,
		string(provider.TWSEAPINoteTrans), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPINoteTrans), dataDate, nameArg+kind, nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPINoteTrans, nil) })
	if err != nil {
		return HandlerResult{}, err
	}

	ttl, _ := a.ttlOf(string(provider.TWSEAPINoteTrans))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)

	out := make([]any, 0, len(rows))
	for _, r := range rows {
		code := rowCode(r)
		if code == "" {
			continue // 官方偶有無代號之空列
		}
		switch kind {
		case "stock":
			if len(code) > 4 {
				continue
			}
		case "warrant":
			if len(code) <= 4 {
				continue
			}
		}
		if nameArg != "" && !strings.Contains(rowName(r), nameArg) {
			continue
		}
		out = append(out, r)
	}
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: lineage}, nil
}
