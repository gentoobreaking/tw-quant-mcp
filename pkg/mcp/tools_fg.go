package mcp

// tools_fg.go 實作 §10.F（期貨與選擇權，7 工具）與 §10.G（基礎設施，2 工具）
// （T015）。F 組資料經 T013 TAIFEX 查詢層（taifex_query.go，§9.3）：
// date==最新交易日 → API（hot tier）；其餘 → DL 下載 CSV（cold tier，L2 永久
// TTL，命中不重複下載）。G 組取自 Symbol Registry（§5.2）與交易日曆（§附錄 A）。

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// TAIFEXQuerier 為 §10.F 工具之 TAIFEX 查詢層視界（§9.3）。
// App 預設以 provider.TAIFEXQuery 實作；測試可注入替身。
type TAIFEXQuerier interface {
	// Fetch 取得單一 (dataset, date, contract) 之 Normalized 資料
	//（回傳是否命中快取，§3.2 is_cached）。
	Fetch(ctx context.Context, ds model.TAIFEXDataset, date, contract string) (provider.TAIFEXQueryResult, bool, error)
	// FetchRange 依範圍取得多日資料（一次 DL 請求覆蓋範圍）。
	FetchRange(ctx context.Context, ds model.TAIFEXDataset, start, end, contract string) (map[string]provider.TAIFEXQueryResult, error)
	// LatestTradingDay 回傳最新交易日（YYYY-MM-DD）。
	LatestTradingDay(ctx context.Context) (string, error)
}

var _ TAIFEXQuerier = (*provider.TAIFEXQuery)(nil)

// futuresContractWhitelist 為期貨契約代號白名單（§T015 備註：避免注入）。
// 涵蓋指數/商品類主要期貨契約；個股期貨依官方公告代碼另行增補。
var futuresContractWhitelist = map[string]bool{
	"TX": true, "MTX": true, // 台股期貨 / 小型台指期貨
	"GTX": true, "G2F": true, // 富櫃200期貨 / 電子期貨
	"G1F": true, "G9F": true, // 小型電子期貨 / 小型金融期貨
	"E4F": true, "XIF": true, // 資訊科技期貨 / 非金電期貨
	"GXF": true, "T5F": true, // 金融期貨 / 中型100期貨
}

// futuresContract 解析 contract 參數並驗證白名單。
func futuresContract(args map[string]any) (string, error) {
	c, _ := args["contract"].(string)
	c = strings.TrimSpace(c)
	if !futuresContractWhitelist[c] {
		return "", fmt.Errorf("期貨契約代號 %q 不在白名單（TX/MTX/GTX/G2F/G1F/G9F/E4F/XIF/GXF/T5F）", c)
	}
	return c, nil
}

// taifexRangeCap 為範圍查詢之最長跨度（1 年，防單次 DL 過大）。
const taifexRangeCap = 366

// validateRange 解析並驗證 start/end（YYYY-MM-DD；順序合法；跨度 ≤ 366 日）。
func validateRange(start, end string) (string, string, error) {
	s, err := model.ParseDate(start)
	if err != nil {
		return "", "", fmt.Errorf("參數 start 格式必須為 YYYY-MM-DD: %w", err)
	}
	e, err := model.ParseDate(end)
	if err != nil {
		return "", "", fmt.Errorf("參數 end 格式必須為 YYYY-MM-DD: %w", err)
	}
	if e.Before(s) {
		return "", "", fmt.Errorf("參數 start（%s）不得晚於 end（%s）", start, end)
	}
	if days := int(e.Sub(s).Hours() / 24); days > taifexRangeCap {
		return "", "", fmt.Errorf("範圍跨度 %d 日超過上限 %d 日", days, taifexRangeCap)
	}
	return model.FormatDate(s), model.FormatDate(e), nil
}

// querier 回傳 TAIFEX 查詢層（未接線時明確錯誤）。
func (a *App) querier() (TAIFEXQuerier, error) {
	if a.taifex == nil {
		return nil, fmt.Errorf("TAIFEX 查詢層尚未接線")
	}
	return a.taifex, nil
}

// taifexDate 解析 date 參數：省略時以最新交易日（API 判定）補齊。
func taifexDate(a *App, q TAIFEXQuerier, ctx context.Context, date string) (string, error) {
	if date != "" {
		t, err := model.ParseDate(date)
		if err != nil {
			return "", fmt.Errorf("參數 date 格式必須為 YYYY-MM-DD: %w", err)
		}
		return model.FormatDate(t), nil
	}
	latest, err := q.LatestTradingDay(ctx)
	if err != nil {
		return "", fmt.Errorf("判定最新交易日失敗: %w", err)
	}
	return latest, nil
}

// taifexRows 解出 Normalize 後之資料行；單日缺口（Data nil）回明確錯誤。
func taifexRows[T any](ds model.TAIFEXDataset, date string, res provider.TAIFEXQueryResult) ([]T, error) {
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return nil, fmt.Errorf("官方無 %s 於 %s 之資料（%s）", ds, date, note)
	}
	var rows []T
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return nil, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	return rows, nil
}

// taifexTTL TAIFEX 資料類別 TTL（§4.2/§5.2：TAIFEX 歷史 7 天；與
// taifex_query.go 之 L2 政策一致）。
func (a *App) taifexTTL() time.Duration {
	ttl, _ := cache.TTLFor(cache.DatasetTAIFEXHistory, a.now())
	return ttl
}

// taifexLineage 依查詢結果建立 lineage（v2.1 §3/§4）：TAIFEX 資料為每日盤後
// 公布，freshness 一律 POST_MARKET；source_role 依實際使用來源標註
// （TAIFEX-API → CANONICAL，TAIFEX-DL → FALLBACK，§3 表）；補檔標
// derived_from（僅 debug/log 輸出）。
func taifexLineage(res provider.TAIFEXQueryResult, date string, fromCache bool, ttl time.Duration) *model.Lineage {
	role := model.SourceRoleCanonical
	if res.Source == model.SourceTAIFEXDL {
		role = model.SourceRoleFallback
	}
	lg := &model.Lineage{
		Source:     res.Source,
		SourceRole: role,
		Freshness:  model.FreshnessPostMarket,
		DataDate:   date,
		IsCached:   fromCache || res.IsCached,
		CacheTTL:   int(ttl.Seconds()),
	}
	if res.DerivedFrom != "" {
		lg.DerivedFrom = []string{res.DerivedFrom}
	}
	return lg
}

// ************** F. 期貨與選擇權 **************

// handlerGetFuturesDailyOHLC：期貨每日 OHLC（openapi 最新日，§10.F）。
func handlerGetFuturesDailyOHLC(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract, err := futuresContract(args)
	if err != nil {
		return HandlerResult{}, err
	}
	date, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAFuturesDaily, date, contract)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAFuturesDaily, err)
	}
	rows, err := taifexRows[model.FuturesDailyRow](model.TAFuturesDaily, date, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, date, fromCache, a.taifexTTL())}, nil
}

// handlerGetFuturesHistory：期貨 OHLC 歷史（TAIFEX-DL 回溯，§10.F）。
func handlerGetFuturesHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract, err := futuresContract(args)
	if err != nil {
		return HandlerResult{}, err
	}
	start, end, err := validateRange(strVal(args["start"]), strVal(args["end"]))
	if err != nil {
		return HandlerResult{}, err
	}
	byDay, err := q.FetchRange(ctx, model.TAFuturesDaily, start, end, contract)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAFuturesDaily, err)
	}
	rows, err := collectRangeRows[model.FuturesDailyRow](model.TAFuturesDaily, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].ContractMonth < rows[j].ContractMonth
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, end, a.taifexTTL())}, nil
}

// handlerGetPutCallRatio：買賣權比（date 或 range，支援歷史，§10.F）。
func handlerGetPutCallRatio(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	date, start, end := strVal(args["date"]), strVal(args["start"]), strVal(args["end"])
	if start != "" || end != "" {
		s, e, err := validateRange(start, end)
		if err != nil {
			return HandlerResult{}, err
		}
		byDay, err := q.FetchRange(ctx, model.TAPutCallRatio, s, e, "")
		if err != nil {
			return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAPutCallRatio, err)
		}
		rows, err := collectRangeRows[model.PCRow](model.TAPutCallRatio, byDay)
		if err != nil {
			return HandlerResult{}, err
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].Date < rows[j].Date })
		return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, e, a.taifexTTL())}, nil
	}
	d, err := taifexDate(a, q, ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAPutCallRatio, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAPutCallRatio, err)
	}
	rows, err := taifexRows[model.PCRow](model.TAPutCallRatio, d, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetLargeTraderPositions：大額交易人未沖銷部位（期貨+選擇權，§10.F）。
func handlerGetLargeTraderPositions(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	date, start, end := strVal(args["date"]), strVal(args["start"]), strVal(args["end"])
	out := model.LargeTraderPositions{}

	if start != "" || end != "" {
		s, e, err := validateRange(start, end)
		if err != nil {
			return HandlerResult{}, err
		}
		out.RangeStart, out.RangeEnd = s, e
		fut, err := q.FetchRange(ctx, model.TALargeTraderFut, s, e, "")
		if err != nil {
			return HandlerResult{}, err
		}
		opt, err := q.FetchRange(ctx, model.TALargeTraderOpt, s, e, "")
		if err != nil {
			return HandlerResult{}, err
		}
		if out.Futures, err = collectRangeRows[model.LargeTraderRow](model.TALargeTraderFut, fut); err != nil {
			return HandlerResult{}, err
		}
		if out.Options, err = collectRangeRows[model.LargeTraderRow](model.TALargeTraderOpt, opt); err != nil {
			return HandlerResult{}, err
		}
		sort.Slice(out.Futures, func(i, j int) bool { return out.Futures[i].Date < out.Futures[j].Date })
		sort.Slice(out.Options, func(i, j int) bool { return out.Options[i].Date < out.Options[j].Date })
		return HandlerResult{Data: out, Lineage: rangeLineage(fut, e, a.taifexTTL())}, nil
	}

	d, err := taifexDate(a, q, ctx, date)
	if err != nil {
		return HandlerResult{}, err
	}
	out.Date = d
	futRes, futCached, err := q.Fetch(ctx, model.TALargeTraderFut, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TALargeTraderFut, err)
	}
	optRes, _, err := q.Fetch(ctx, model.TALargeTraderOpt, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TALargeTraderOpt, err)
	}
	if out.Futures, err = taifexRows[model.LargeTraderRow](model.TALargeTraderFut, d, futRes); err != nil {
		return HandlerResult{}, err
	}
	if out.Options, err = taifexRows[model.LargeTraderRow](model.TALargeTraderOpt, d, optRes); err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(futRes, d, futCached, a.taifexTTL())}, nil
}

// handlerGetInstitutionalFuturesPositions：三大法人期貨部位（§10.F）。
func handlerGetInstitutionalFuturesPositions(a *App, args map[string]any) (HandlerResult, error) {
	return instiPositions(a, args, model.TAInstiFutures)
}

// handlerGetInstitutionalOptionsPositions：三大法人選擇權部位（§10.F）。
func handlerGetInstitutionalOptionsPositions(a *App, args map[string]any) (HandlerResult, error) {
	return instiPositions(a, args, model.TAInstiOptions)
}

// instiPositions 三大法人部位（期貨/選擇權共用路徑）。
func instiPositions(a *App, args map[string]any, ds model.TAIFEXDataset) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	rows, err := taifexRows[model.InstitutionalRow](ds, d, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetInstitutionalFuturesHistory：三大法人期貨部位歷史（DL 回溯，§10.F）。
func handlerGetInstitutionalFuturesHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	start, end, err := validateRange(strVal(args["start"]), strVal(args["end"]))
	if err != nil {
		return HandlerResult{}, err
	}
	byDay, err := q.FetchRange(ctx, model.TAInstiFutures, start, end, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAInstiFutures, err)
	}
	rows, err := collectRangeRows[model.InstitutionalRow](model.TAInstiFutures, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		if rows[i].Investor != rows[j].Investor {
			return rows[i].Investor < rows[j].Investor
		}
		return rows[i].Contract < rows[j].Contract
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, end, a.taifexTTL())}, nil
}

// collectRangeRows 合併範圍內各日結果；全範圍無資料時回明確錯誤。
// 範圍內個別缺口（Note）以字串串接至返回錯誤訊息（或 Note 欄位）。
func collectRangeRows[T any](ds model.TAIFEXDataset, byDay map[string]provider.TAIFEXQueryResult) ([]T, error) {
	var rows []T
	gaps := []string{}
	for _, d := range sortedKeys(byDay) {
		res := byDay[d]
		if len(res.Data) == 0 {
			gaps = append(gaps, d)
			continue
		}
		var dayRows []T
		if err := json.Unmarshal(res.Data, &dayRows); err != nil {
			return nil, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
		}
		rows = append(rows, dayRows...)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("官方於所查範圍內無 %s 資料（含缺口日 %v）", ds, gaps)
	}
	return rows, nil
}

// rangeLineage 範圍查詢之 lineage：DL 歷史資料（freshness=POST_MARKET，
// source_role=FALLBACK，§3 表）。
func rangeLineage(byDay map[string]provider.TAIFEXQueryResult, end string, ttl time.Duration) *model.Lineage {
	source := model.SourceTAIFEXDL
	derived := []string{}
	cached := false
	for _, d := range sortedKeys(byDay) {
		if byDay[d].DerivedFrom != "" {
			derived = append(derived, d+"←"+byDay[d].DerivedFrom)
		}
		if byDay[d].IsCached {
			cached = true
		}
	}
	lg := &model.Lineage{
		Source:     source,
		SourceRole: model.SourceRoleFallback,
		Freshness:  model.FreshnessPostMarket,
		DataDate:   end,
		IsCached:   cached,
		CacheTTL:   int(ttl.Seconds()),
	}
	if len(derived) > 0 {
		lg.DerivedFrom = derived
	}
	return lg
}

// ************** G. 基礎設施 **************

// handlerGetSymbolList：上市/上櫃代碼表（§10.G，Symbol Registry）。
func handlerGetSymbolList(a *App, args map[string]any) (HandlerResult, error) {
	market := strVal(args["market"])
	if market != "" && !model.ValidMarket(market) {
		return HandlerResult{}, fmt.Errorf("參數 market 僅允許 tse|otc")
	}
	if a.symbols.Len() == 0 {
		return HandlerResult{}, fmt.Errorf("Symbol Registry 未載入（請先以官方清單預熱）")
	}
	symbols := a.symbols.List(market)
	return HandlerResult{
		Data: symbols,
		Lineage: &model.Lineage{
			Source:     model.SourceTWSEAPI, // Registry 來源（TWSE+TPEx 官方清單，§5.2）
			SourceRole: model.SourceRoleCanonical,
			Freshness:  model.FreshnessPostMarket,
			DataDate:   model.FormatDate(a.now()),
		},
	}, nil
}

// handlerGetTradingCalendar：交易日曆（§10.G，TWSE 開休市表）。
func handlerGetTradingCalendar(a *App, args map[string]any) (HandlerResult, error) {
	year := a.now().Year()
	if v, ok := args["year"]; ok {
		n, err := asInt(v)
		if err != nil || n < 2000 || n > 2100 {
			return HandlerResult{}, fmt.Errorf("參數 year 必須為 2000~2100 之整數")
		}
		year = n
	}
	month := 0
	if v, ok := args["month"]; ok {
		n, err := asInt(v)
		if err != nil || n < 1 || n > 12 {
			return HandlerResult{}, fmt.Errorf("參數 month 必須為 1~12 之整數")
		}
		month = n
	}

	cal := model.TradingCalendar{Year: year, Month: month}
	start := time.Date(year, time.January, 1, 0, 0, 0, 0, model.Taipei())
	end := start.AddDate(1, 0, 0) // 全年模式：次年 1/1
	if month > 0 {
		start = time.Date(year, time.Month(month), 1, 0, 0, 0, 0, model.Taipei())
		end = start.AddDate(0, 1, 0) // 月份模式：次月 1 日
	}
	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		if a.calendar.IsTradingDay(d) {
			cal.TradingDays = append(cal.TradingDays, model.FormatDate(d))
		}
	}
	for _, h := range a.calendar.Holidays(year) {
		if month == 0 || sameMonth(h.Date, month) {
			cal.Holidays = append(cal.Holidays, model.HolidayRow{Date: h.Date, Name: h.Name})
		}
	}
	cal.Note = "行事曆版本 " + a.calendar.Version()
	return HandlerResult{
		Data: cal,
		Lineage: &model.Lineage{
			Source:     model.SourceTWSEWeb, // TWSE 官方開休市表
			SourceRole: model.SourceRoleCanonical,
			Freshness:  model.FreshnessPostMarket,
			DataDate:   model.FormatDate(a.now()),
		},
	}, nil
}

// sameMonth 判定 YYYY-MM-DD 是否屬於指定月份。
func sameMonth(date string, month int) bool {
	return len(date) >= 7 && date[5:7] == fmt.Sprintf("%02d", month)
}

// handlerGetAnnualTradingVolume：期貨年成交量統計（AnnualTradingVolume，T041）。
func handlerGetAnnualTradingVolume(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strVal(args["contract"]))
	latest, err := q.LatestTradingDay(ctx)
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAFAnnualVolume, latest, contract)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAFAnnualVolume, err)
	}
	rows, err := taifexRows[provider.AnnualVolumeRow](model.TAFAnnualVolume, latest, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, latest, fromCache, a.taifexTTL())}, nil
}

// handlerGetMonthlyTradingStatistics：期貨各類交易人各商品交易量月統計（T148）。
func handlerGetMonthlyTradingStatistics(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	latest, err := q.LatestTradingDay(ctx)
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAFMonthlyStats, latest, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAFMonthlyStats, err)
	}
	rows, err := taifexRows[provider.MonthlyTraderStatsRow](model.TAFMonthlyStats, latest, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, latest, fromCache, a.taifexTTL())}, nil
}
