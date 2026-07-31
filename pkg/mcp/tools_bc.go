package mcp

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// tools_bc.go 實作 §10.B（盤後行情・籌碼）與 §10.C（重大訊息與風險）工具。
// 全部為 POST_MARKET_TODAY 資料：統一經 fetchRaw/fetchNormalize
//（§4.2 快取 TTL + §12.2 讀穿），並於 lineage 標明資料源（§3.2）。

// postLineage 建構盤後工具之 lineage 預設值；cached/ttl 由 handler 依快取
// 結果填入。
func postLineage(source, dataDate string, cached bool, ttl time.Duration) *model.Lineage {
	return &model.Lineage{
		Source:     source,
		SourceRole: model.SourceRoleCanonical,
		DataDate:   dataDate,
		Freshness:  model.FreshnessPostMarketToday,
		IsCached:   cached,
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
	for _, ms := range months {
		params := url.Values{"date": {ms.Format("20060102")}, "stockNo": {sym.Code}}
		rows, cached, err := fetchNormalize[[]model.Candle](a, ctx, string(provider.TWSEWDDailyK),
			date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDDailyK), date, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDDailyK, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		cachedAny = cachedAny || cached
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
	lg := postLineage(model.SourceTWSEWeb, date, cachedAny, ttl)
	lg.DerivedFrom = []string{"TWSE_WEB:daily_k"}
	lg.SourceRole = model.SourceRoleCanonical
	return HandlerResult{Data: q, Lineage: lg}, nil
}

// quoteOTC 以上櫃收盤行情取單日報價。
func (a *App) quoteOTC(ctx context.Context, sym model.Symbol, date string) (HandlerResult, error) {
	params := url.Values{"date": {dateYMD(date)}}
	rows, _, err := fetchNormalize[[]provider.TPExDailyCloseRow](a, ctx, string(provider.TPExDailyClose),
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
	return HandlerResult{Data: q, Lineage: postLineage(model.SourceTPExAPI, date, false, ttl)}, nil
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
	rows, cached, err := fetchNormalize[[]model.Candle](a, ctx, string(provider.TWSEWDDailyK),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDDailyK), date, sym.Code, vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDDailyK, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	if len(rows) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無日 K 資料（非交易日或無成交）", sym.Code, date)
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDDailyK))
	lg := postLineage(model.SourceTWSEWeb, date, cached, ttl)
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
	tse, err := a.marketStatsTSE(ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	otc, err := a.marketStatsOTC(ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEWDMarketClose))
	lg := postLineage(model.SourceTWSEWeb, date, false, ttl)
	lg.SourceRole = model.SourceRoleCanonical
	return HandlerResult{Data: model.MarketSummary{Date: date, TSE: tse, OTC: otc}, Lineage: lg}, nil
}

func (a *App) marketStatsTSE(ctx context.Context, date string) (model.MarketStats, error) {
	params := url.Values{"date": {dateYMD(date)}}
	rows, _, err := fetchNormalize[[]provider.MarketCloseRow](a, ctx, string(provider.TWSEWDMarketClose),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDMarketClose), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDMarketClose, params) })
	if err != nil {
		return model.MarketStats{}, err
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
	return st, nil
}

func (a *App) marketStatsOTC(ctx context.Context, date string) (model.MarketStats, error) {
	params := url.Values{"date": {dateYMD(date)}}
	rows, _, err := fetchNormalize[[]provider.TPExDailyCloseRow](a, ctx, string(provider.TPExDailyClose),
		date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExDailyClose), date, "", vals(params)),
		func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExDailyClose, params) })
	if err != nil {
		return model.MarketStats{}, err
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
	return st, nil
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
		rows, cached, err := fetchNormalize[[]provider.InstitutionalRow](a, ctx, string(provider.TWSEWDInstitutional),
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
		}, Lineage: postLineage(model.SourceTWSEWeb, date, cached, ttl)}, nil
	case model.MarketOTC:
		params := url.Values{"date": {dateYMD(date)}}
		rows, cached, err := fetchNormalize[[]provider.TPExInstitutionalRow](a, ctx, string(provider.TPExInstitutional),
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
		}, Lineage: postLineage(model.SourceTPExAPI, date, cached, ttl)}, nil
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
	rows, cached, err := fetchNormalize[[]provider.ForeignHoldingRow](a, ctx, string(provider.TWSEAPIForeignHoldings),
		date, cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIForeignHoldings), date, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIForeignHoldings, nil) })
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIForeignHoldings))
	return HandlerResult{Data: rows, Lineage: postLineage(model.SourceTWSEAPI, date, cached, ttl)}, nil
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
	cachedAny := false
	day, _ := model.ParseDate(date)
	for i := 0; i < rnge; i++ {
		d := model.FormatDate(day)
		params := url.Values{"dayDate": {dateYMD(d)}}
		rows, cached, err := fetchNormalize[[]provider.ForeignHoldingPointRow](a, ctx, string(provider.TWSEWDForeignQFIIS),
			d, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDForeignQFIIS), d, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDForeignQFIIS, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		cachedAny = cachedAny || cached
		for _, r := range rows {
			if r.Code == sym.Code {
				series = append(series, model.ForeignHoldingPoint{
					Date: r.Date, ForeignShares: r.ForeignShares, ForeignPercent: r.ForeignPercent,
				})
			}
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
	lg := postLineage(model.SourceTWSEWeb, date, cachedAny, ttl)
	return HandlerResult{Data: model.ForeignShareholdingHistory{
		Symbol: sym.Code, Name: sym.Name, Range: rnge, Series: series,
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
		rows, cached, err := fetchNormalize[[]provider.TPExMarginRow](a, ctx, string(provider.TPExMargin),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExMargin), date, sym.Code, vals(params)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExMargin, params) })
		if err != nil {
			return HandlerResult{}, err
		}
		for _, r := range rows {
			if r.Code == sym.Code {
				ttl, _ := a.ttlOf(string(provider.TPExMargin))
				return HandlerResult{Data: r, Lineage: postLineage(model.SourceTPExAPI, date, cached, ttl)}, nil
			}
		}
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s 無上櫃融資融券資料", sym.Code, date)
	}
	params := url.Values{"date": {dateYMD(date)}}
	rows, cached, err := fetchNormalize[[]provider.MarginRow](a, ctx, string(provider.TWSEWDMargin),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDMargin), date, sym.Code, vals(params)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDMargin, params) })
	if err != nil {
		return HandlerResult{}, err
	}
	for _, r := range rows {
		if r.Code == sym.Code {
			ttl, _ := a.ttlOf(string(provider.TWSEWDMargin))
			return HandlerResult{Data: r, Lineage: postLineage(model.SourceTWSEWeb, date, cached, ttl)}, nil
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
		rows, cached, err := fetchNormalize[[]provider.TPExAttentionRow](a, ctx, string(provider.TPExAttention),
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
		return HandlerResult{Data: out, Lineage: postLineage(model.SourceTPExAPI, date, cached, ttl)}, nil
	}
	params := url.Values{"date": {dateYMD(date)}}
	rows, cached, err := fetchNormalize[[]provider.AbnormalVolumeRow](a, ctx, string(provider.TWSEWDAbnormal),
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
	return HandlerResult{Data: out, Lineage: postLineage(model.SourceTWSEWeb, date, cached, ttl)}, nil
}

// handlerGetWarrantActivity：權證活躍度（成交金額/張數排名，§10.B）。
func handlerGetWarrantActivity(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	date, err := a.resolveDate(strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	rows, cached, err := fetchNormalize[[]provider.WarrantRow](a, ctx, string(provider.TWSEAPIWarrants),
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
	}, Lineage: postLineage(model.SourceTWSEAPI, date, cached, ttl)}, nil
}

// ************** C. 重大訊息與風險 **************

// handlerGetMajorAnnouncements：MOPS 重大訊息（§10.C）。
// 資料源（MOPS-API）屬 T012 範圍，尚未接線時回傳明確錯誤。
func handlerGetMajorAnnouncements(a *App, args map[string]any) (HandlerResult, error) {
	return HandlerResult{}, fmt.Errorf("重大訊息資料源（MOPS-API）尚未接線（屬 T012 範圍），請先完成 T012")
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
		att, cached, err := fetchNormalize[[]provider.TPExAttentionRow](a, ctx, string(provider.TPExAttention),
			date, cache.KeyString(model.SourceTPExAPI, string(provider.TPExAttention), date, "", vals(ap)),
			func() ([]byte, error) { return a.fetchTPExRaw(ctx, provider.TPExAttention, ap) })
		if err != nil {
			return HandlerResult{}, err
		}
		dp := url.Values{"date": {dateYMD(date)}}
		disp, dCached, err := fetchNormalize[[]provider.TPExDispositionRow](a, ctx, string(provider.TPExDisposition),
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
		return HandlerResult{Data: out, Lineage: postLineage(model.SourceTPExAPI, date, cached || dCached, ttl)}, nil
	}
	ap := url.Values{"date": {dateYMD(date)}}
	att, cached, err := fetchNormalize[[]provider.AbnormalVolumeRow](a, ctx, string(provider.TWSEWDAbnormal),
		date, cache.KeyString(model.SourceTWSEWeb, string(provider.TWSEWDAbnormal), date, "", vals(ap)),
		func() ([]byte, error) { return a.fetchWebRaw(ctx, provider.TWSEWDAbnormal, ap) })
	if err != nil {
		return HandlerResult{}, err
	}
	dp := url.Values{"date": {dateYMD(date)}}
	disp, dCached, err := fetchNormalize[[]provider.PunishRow](a, ctx, string(provider.TWSEAPIPunish),
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
	return HandlerResult{Data: out, Lineage: postLineage(model.SourceTWSEWeb, date, cached || dCached, ttl)}, nil
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
