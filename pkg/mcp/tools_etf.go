// tools_etf.go 實作 §30.1 ETF Data Adapter L1 之歷史 NAV/折溢價工具
// （get_etf_nav）與 ETF 分配收益工具（get_etf_dividend）。
// 資料源：TWSE ETF e添富平台（provider.ETFortuneSource）與
// TWSE ETF 分配收益 API（provider.ETFDividendSource）。
//
// 2026-08 實測：TWSE 舊版 NAV 端點全 404；e添富平台為現行官方入口，
// 以 POST ajaxEtfInfoChart（type=fundPric）取得歷史淨值（netPrice）與
// 折溢價率（atmps，%）。僅上市 ETF；上櫃 ETF 不在本平台。
// ETF 分配收益：TWSE rwd/zh/ETF/etfDiv 提供完整歷史配息資料。
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// etfChartResp 為 ajaxEtfInfoChart type=fundPric 之回應結構。
type etfChartResp struct {
	NetPrice []etfPoint `json:"netPrice"` // 歷史淨值
	ATMPS    []etfPoint `json:"atmps"`    // 折溢價率（%）
}

// etfChartClose 為 ajaxEtfInfoChart type=close 之回應結構（陣列）。
type etfChartClose []etfPoint

// etfPoint 為圖表資料點（date: YYYY/MM/DD, count: 數值）。
type etfPoint struct {
	Date  string  `json:"date"`
	Count float64 `json:"count"`
}

// etfFetcher 為 e添富資料源之 handler 視界（測試可注入 fake）。
type etfFetcher interface {
	ChartFetch(ctx context.Context, code string, chartType provider.ETFChartType, start, end string) ([]byte, error)
}

// etfDivFetcher 為 ETF 分配收益資料源之 handler 視界（測試可注入 fake）。
type etfDivFetcher interface {
	FetchDividend(ctx context.Context, code, startDate, endDate string) ([]model.ETFDividendPoint, error)
}

// ************** get_etf_nav（§30.1 L1 歷史 NAV/折溢價） **************

// handlerGetETFNav：ETF 歷史淨值與折溢價（spec §30.1 L1）。
// 以 ajaxEtfInfoChart type=fundPric 取 netPrice（NAV）+ atmps（折溢價率），
// type=close 取市價；三序列以日期對齊輸出。僅上市 ETF（上櫃 ETF 不在
// e添富平台，回傳錯誤說明）。
func handlerGetETFNav(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	if !sym.IsETF() {
		return HandlerResult{}, fmt.Errorf("代碼 %s（%s）非 ETF，get_etf_nav 僅供 ETF 使用", code, sym.Name)
	}
	if sym.Market != model.MarketTSE {
		return HandlerResult{}, fmt.Errorf("上櫃 ETF（%s）暫無 NAV 資料源：e添富平台僅涵蓋上市 ETF（T012a 待接上櫃源）", code)
	}
	if a.etf == nil {
		return HandlerResult{}, fmt.Errorf("e添富資料源尚未接線")
	}

	// 日期範圍（預設近 3 個月）
	end := a.now()
	if end.Hour()*3600+end.Minute()*60 < 15*3600 || !a.calendar.IsTradingDay(end) {
		if p, err := a.prevTradingDay(end, 1); err == nil {
			end = p
		}
	}
	start := end.AddDate(0, -3, 0)
	if s, err := strValDate(args["start"]); err != nil {
		return HandlerResult{}, err
	} else if s != "" {
		start, _ = model.ParseDate(s)
	}
	if e, err := strValDate(args["end"]); err != nil {
		return HandlerResult{}, err
	} else if e != "" {
		end, _ = model.ParseDate(e)
	}
	if end.Before(start) {
		return HandlerResult{}, fmt.Errorf("start 不得晚於 end")
	}
	startStr := start.Format("2006/01/02")
	endStr := end.Format("2006/01/02")
	dataDate := end.Format("2006-01-02")

	// 快取鍵：fundPric + close 各一筆，快取類別 daily_kline（24h TTL）
	keyP := cache.KeyString(model.SourceTWSEWeb, "etf_nav_fundPric", dataDate, code, map[string]string{"s": startStr, "e": endStr})
	keyC := cache.KeyString(model.SourceTWSEWeb, "etf_nav_close", dataDate, code, map[string]string{"s": startStr, "e": endStr})

	// etf_nav dataset 未登錄政策表（無法由 provider dataset 對映），
	// 改以 daily_kline 政策（60s 盤中 / 至隔日 08:00 盤後）並明確指定
	// cacheDataset（fetchRaw 對 dataset 參數不做 policyDataset 對映）。
	const etfNavDataset = cache.DatasetDailyKLine

	var fundResp etfChartResp
	cachedP, staleP, err := a.etfFetchJSON(ctx, etfNavDataset, keyP, func() ([]byte, error) {
		return a.etf.ChartFetch(ctx, code, provider.ETFChartFundPric, startStr, endStr)
	}, &fundResp)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("淨值資料取得失敗: %w", err)
	}
	var closes etfChartClose
	cachedC, staleC, err := a.etfFetchJSON(ctx, etfNavDataset, keyC, func() ([]byte, error) {
		return a.etf.ChartFetch(ctx, code, provider.ETFChartClose, startStr, endStr)
	}, &closes)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("市價資料取得失敗: %w", err)
	}

	// 對齊：以 netPrice 為主軸，市價/折溢價按日期對應
	byNav := make(map[string]float64, len(fundResp.NetPrice))
	for _, p := range fundResp.NetPrice {
		byNav[p.Date] = p.Count
	}
	byAtmp := make(map[string]float64, len(fundResp.ATMPS))
	for _, p := range fundResp.ATMPS {
		byAtmp[p.Date] = p.Count
	}
	byClose := make(map[string]float64, len(closes))
	for _, p := range closes {
		byClose[p.Date] = p.Count
	}
	// 若 fundPric 無資料（債券型等），退回以市價序列為主軸
	dates := make([]string, 0, len(byNav))
	navSource := true
	if len(byNav) == 0 && len(byClose) > 0 {
		for d := range byClose {
			dates = append(dates, d)
		}
		navSource = false
	} else {
		for d := range byNav {
			dates = append(dates, d)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	points := make([]model.ETFNavPoint, 0, len(dates))
	for _, d := range dates {
		ymd := d
		if len(ymd) == 10 && ymd[4] == '/' {
			ymd = ymd[:4] + "-" + ymd[5:7] + "-" + ymd[8:10]
		}
		p := model.ETFNavPoint{
			Date:            ymd,
			NAV:             byNav[d],
			Market:          byClose[d],
			PremiumDiscount: byAtmp[d],
		}
		points = append(points, p)
	}
	if len(points) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s~%s 無 NAV 資料（e添富平台無此 ETF 或期間無資料）", code, startStr, endStr)
	}

	note := ""
	if !navSource {
		note = "淨值資料從缺（此 ETF 於 e添富平台無 netPrice），僅提供市價序列"
	}
	out := model.ETFNavResult{
		Symbol: sym.Code, Name: sym.Name, Market: sym.Market,
		Start: start.Format("2006-01-02"), End: end.Format("2006-01-02"),
		Points: points, Note: note,
	}
	lg := postLineage(model.SourceTWSEWeb, dataDate, cachedP || staleP || cachedC || staleC, staleP || staleC, 24*time.Hour)
	lg.SourceRole = model.SourceRoleFallback // 官方網域網頁端點（未列入 OpenAPI 目錄）：FALLBACK 角色
	lg.DerivedFrom = []string{"TWSE_ETFORTUNE:ajaxEtfInfoChart"}
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// etfFetchJSON 執行快取讀穿並解碼 JSON（dataset 為 §4.2 政策類別）。
func (a *App) etfFetchJSON(ctx context.Context, dataset, key string, fetch func() ([]byte, error), v any) (cached, stale bool, err error) {
	if a.cache == nil {
		raw, err := fetch()
		if err != nil {
			return false, false, err
		}
		return false, false, json.Unmarshal(raw, v)
	}
	cached, stale, raw, err := a.fetchRaw(ctx, dataset, a.now().Format("2006-01-02"), key, fetch)
	if err != nil {
		return false, false, err
	}
	return cached, stale, json.Unmarshal(raw, v)
}

// strValDate 回傳參數日期字串（YYYY-MM-DD，可空）。
func strValDate(v any) (string, error) {
	s, _ := v.(string)
	if s == "" {
		return "", nil
	}
	if _, err := model.ParseDate(s); err != nil {
		return "", fmt.Errorf("mcp: 參數 date 格式必須為 YYYY-MM-DD: %w", err)
	}
	return s, nil
}

// ************** get_etf_dividend（ETF 分配收益歷史） **************

// handlerGetETFDividend：ETF 歷史分配收益（收益分配/配息）。
// 資料源：TWSE rwd/zh/ETF/etfDiv（官方 ETF 分配收益查詢 API）。
// 僅上市 ETF；上櫃 ETF 需確認是否有資料。
func handlerGetETFDividend(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	if !sym.IsETF() {
		return HandlerResult{}, fmt.Errorf("代碼 %s（%s）非 ETF，get_etf_dividend 僅供 ETF 使用", code, sym.Name)
	}
	if a.etfDiv == nil {
		return HandlerResult{}, fmt.Errorf("ETF 分配收益資料源尚未接線")
	}

	// 日期範圍（預設近 2 年）
	end := a.now()
	start := end.AddDate(-2, 0, 0)
	if s, err := strValDate(args["start"]); err != nil {
		return HandlerResult{}, err
	} else if s != "" {
		start, _ = model.ParseDate(s)
	}
	if e, err := strValDate(args["end"]); err != nil {
		return HandlerResult{}, err
	} else if e != "" {
		end, _ = model.ParseDate(e)
	}
	if end.Before(start) {
		return HandlerResult{}, fmt.Errorf("start 不得晚於 end")
	}
	startStr := start.Format("20060102")
	endStr := end.Format("20060102")
	dataDate := end.Format("2006-01-02")

	// 快取鍵
	key := cache.KeyString(model.SourceTWSEWeb, "etf_dividend", dataDate, code, map[string]string{"s": startStr, "e": endStr})

	// 使用 daily_kline 政策（24h TTL）
	const etfDivDataset = cache.DatasetDailyKLine

	var points []model.ETFDividendPoint
	cached, stale, points, err := a.etfDivFetch(ctx, etfDivDataset, key, func() ([]model.ETFDividendPoint, error) {
		return a.etfDiv.FetchDividend(ctx, code, startStr, endStr)
	})
	if err != nil {
		return HandlerResult{}, fmt.Errorf("分配收益資料取得失敗: %w", err)
	}

	if len(points) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 於 %s~%s 無分配收益資料（TWSE etfDiv 無此 ETF 或期間無資料）", code, startStr, endStr)
	}

	// 依除息日由近至遠排序
	sort.Slice(points, func(i, j int) bool { return points[i].ExDate > points[j].ExDate })

	out := model.ETFDividendResult{
		Symbol: sym.Code, Name: sym.Name, Market: sym.Market,
		Points: points,
	}
	lg := postLineage(model.SourceTWSEWeb, dataDate, cached || stale, stale, 24*time.Hour)
	lg.SourceRole = model.SourceRoleCanonical // TWSE 官方 Open Data 端點
	lg.DerivedFrom = []string{"TWSE_ETF_DIVIDEND:etfDiv"}
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// etfDivFetch 執行快取讀穿並解碼 ETF 分配收益資料（dataset 為 §4.2 政策類別）。
func (a *App) etfDivFetch(ctx context.Context, dataset, key string, fetch func() ([]model.ETFDividendPoint, error)) (cached, stale bool, points []model.ETFDividendPoint, err error) {
	if a.cache == nil {
		points, err = fetch()
		return false, false, points, err
	}
	// 將 ETFDividendPoint 序列化為 JSON 快取
	fetchWrapper := func() ([]byte, error) {
		data, err := fetch()
		if err != nil {
			return nil, err
		}
		return json.Marshal(data)
	}
	cached, stale, raw, err := a.fetchRaw(ctx, dataset, a.now().Format("2006-01-02"), key, fetchWrapper)
	if err != nil {
		return false, false, nil, err
	}
	if raw == nil {
		return cached, stale, nil, nil
	}
	return cached, stale, points, json.Unmarshal(raw, &points)
}
