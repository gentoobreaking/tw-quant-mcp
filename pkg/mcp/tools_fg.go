package mcp

// tools_fg.go 實作 §10.F（期貨與選擇權，7 工具）與 §10.G（基礎設施，2 工具）
// （T015）。F 組資料經 T013 TAIFEX 查詢層（taifex_query.go，§9.3）：
// date==最新交易日 → API（hot tier）；其餘 → DL 下載 CSV（cold tier，L2 永久
// TTL，命中不重複下載）。G 組取自 Symbol Registry（§5.2）與交易日曆（§附錄 A）。

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
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

// handlerGetDailyFuturesMarketReport：期貨每日交易行情（TAIFEX-API
// DailyMarketReportFut，T117）。contract 省略時預設 TX；明確傳空字串
// 則列出最新交易日所有可用契約代碼。
func handlerGetDailyFuturesMarketReport(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	raw, hasArg := args["contract"]
	contract := strings.TrimSpace(strVal(raw))
	date, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	if hasArg && contract == "" {
		// 列出所有可用契約代碼（全市場查詢，本地去重排序）
		res, fromCache, err := q.Fetch(ctx, model.TAFuturesDaily, date, "")
		if err != nil {
			return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAFuturesDaily, err)
		}
		rows, err := taifexRows[model.FuturesDailyRow](model.TAFuturesDaily, date, res)
		if err != nil {
			return HandlerResult{}, err
		}
		seen := map[string]bool{}
		out := make([]map[string]any, 0)
		for _, r := range rows {
			if seen[r.Contract] {
				continue
			}
			seen[r.Contract] = true
			out = append(out, map[string]any{"contract": r.Contract})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["contract"].(string) < out[j]["contract"].(string) })
		return HandlerResult{Data: out, Lineage: taifexLineage(res, date, fromCache, a.taifexTTL())}, nil
	}
	if contract == "" {
		contract = "TX" // 預設 TX（對齊遠端 inputSchema default）
	}
	if !futuresContractWhitelist[contract] {
		return HandlerResult{}, fmt.Errorf("期貨契約代號 %q 不在白名單（TX/MTX/GTX/G2F/G1F/G9F/E4F/XIF/GXF/T5F）", contract)
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

// optionsContractRe 驗證選擇權契約代碼格式（全大寫英數字 2-6 碼，防注入）。
var optionsContractRe = regexp.MustCompile(`^[A-Z0-9]{2,6}$`)

// handlerGetDailyOptionsMarketReport：選擇權每日交易行情（TAIFEX-API
// DailyMarketReportOpt，T118）。篩選有成交量之序列，按成交量由大到小排序；
// contract 省略時預設 TXO；明確傳空字串則列出所有可用契約代碼。
func handlerGetDailyOptionsMarketReport(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	raw, hasArg := args["contract"]
	contract := strings.TrimSpace(strVal(raw))
	date, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	callPut := strings.TrimSpace(strVal(args["call_put"]))
	if callPut != "" && callPut != "買權" && callPut != "賣權" {
		return HandlerResult{}, fmt.Errorf("參數 call_put 僅接受「買權」或「賣權」，實際 %q", callPut)
	}
	limit := 30
	if v, ok := args["limit"]; ok {
		if n, err := asInt(v); err == nil && n > 0 {
			limit = n
		}
	}
	listMode := hasArg && contract == ""
	fetchContract := contract
	if !listMode {
		if fetchContract == "" {
			fetchContract = "TXO" // 預設 TXO（對齊遠端 inputSchema default）
		} else if !optionsContractRe.MatchString(fetchContract) {
			return HandlerResult{}, fmt.Errorf("選擇權契約代碼 %q 格式不合法（應為 2-6 碼大寫英數字，如 TXO）", contract)
		}
	}
	res, fromCache, err := q.Fetch(ctx, model.TAOptionsDaily, date, fetchContract)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAOptionsDaily, err)
	}
	rows, err := taifexRows[model.OptionsDailyRow](model.TAOptionsDaily, date, res)
	if err != nil {
		return HandlerResult{}, err
	}
	if listMode {
		// 列出所有可用契約代碼（本地去重排序）
		seen := map[string]bool{}
		out := make([]map[string]any, 0)
		for _, r := range rows {
			if seen[r.Contract] {
				continue
			}
			seen[r.Contract] = true
			out = append(out, map[string]any{"contract": r.Contract})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["contract"].(string) < out[j]["contract"].(string) })
		return HandlerResult{Data: out, Lineage: taifexLineage(res, date, fromCache, a.taifexTTL())}, nil
	}
	// 有成交量之序列，按成交量由大到小；call_put / limit 過濾
	filtered := make([]model.OptionsDailyRow, 0, len(rows))
	for _, r := range rows {
		if r.Volume <= 0 {
			continue
		}
		if callPut != "" && r.CallPut != callPut {
			continue
		}
		filtered = append(filtered, r)
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Volume > filtered[j].Volume })
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return HandlerResult{Data: filtered, Lineage: taifexLineage(res, date, fromCache, a.taifexTTL())}, nil
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

// handlerGetFuturesDailyHistory：期貨每日 OHLC 歷史（T125，與
// handlerGetFuturesHistory 同資料層；contract 省略預設 TX，日期相容 YYYYMMDD）。
func handlerGetFuturesDailyHistory(a *App, args map[string]any) (HandlerResult, error) {
	contract := strings.TrimSpace(strVal(args["contract"]))
	if contract == "" {
		contract = "TX" // 對齊遠端 inputSchema default
	}
	start := strVal(args["start"])
	if start == "" {
		start = strVal(args["start_date"])
	}
	end := strVal(args["end"])
	if end == "" {
		end = strVal(args["end_date"])
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	return handlerGetFuturesHistory(a, map[string]any{
		"contract": contract,
		"start":    flexDateArg(start),
		"end":      flexDateArg(end),
	})
}

// flexDateArg 日期參數相容 YYYYMMDD（轉 YYYY-MM-DD）與 YYYY-MM-DD（原樣）。
func flexDateArg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 8 {
		if t, err := time.ParseInLocation("20060102", s, model.Taipei()); err == nil {
			return model.FormatDate(t)
		}
	}
	return s
}

// handlerGetInstitutionalTotalHistory：三大法人期貨+選擇權合計總表歷史
// （TAIFEX-DL totalTableDateDown，T130；區間 ≤ 92 日）。
func handlerGetInstitutionalTotalHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	sT, errT := model.ParseDate(sd)
	eT, _ := model.ParseDate(ed)
	if errT == nil && int(eT.Sub(sT).Hours()/24) > instiSplitRangeCap {
		return HandlerResult{}, fmt.Errorf("查詢區間不可超過 %d 天，請縮小範圍", instiSplitRangeCap)
	}
	byDay, err := q.FetchRange(ctx, model.TAInstiTotal, sd, ed, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAInstiTotal, err)
	}
	rows, err := collectRangeRows[model.InstiGeneralRow](model.TAInstiTotal, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].Investor < rows[j].Investor
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// handlerGetLargeTradersFuturesOI：期貨大額交易人未沖銷部位（最新日，T136）。
// contract 精確比對契約代碼（如 TX），留空列出所有可用契約代碼。
func handlerGetLargeTradersFuturesOI(a *App, args map[string]any) (HandlerResult, error) {
	return largeTradersOI(a, args, model.TALargeTraderFut)
}

// handlerGetLargeTradersOptionsOI：選擇權大額交易人未沖銷部位（最新日，T137）。
// contract 精確比對契約代碼（如 TXO），留空列出所有可用契約代碼；call_put 過濾。
func handlerGetLargeTradersOptionsOI(a *App, args map[string]any) (HandlerResult, error) {
	return largeTradersOI(a, args, model.TALargeTraderOpt)
}

func largeTradersOI(a *App, args map[string]any, ds model.TAIFEXDataset) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strings.TrimSpace(strVal(args["contract"])))
	callPut := strings.TrimSpace(strVal(args["call_put"]))
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	rows, err := taifexRows[model.LargeTraderRow](ds, d, res)
	if err != nil {
		return HandlerResult{}, err
	}
	if contract == "" {
		seen := map[string]bool{}
		out := make([]map[string]any, 0)
		for _, r := range rows {
			if seen[r.Contract] {
				continue
			}
			seen[r.Contract] = true
			out = append(out, map[string]any{"contract": r.Contract})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["contract"].(string) < out[j]["contract"].(string) })
		return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	filtered := make([]model.LargeTraderRow, 0, len(rows))
	for _, r := range rows {
		if r.Contract != contract {
			continue
		}
		if callPut != "" && r.CallPut != callPut {
			continue
		}
		filtered = append(filtered, r)
	}
	if len(filtered) == 0 {
		seen := map[string]bool{}
		names := make([]string, 0)
		for _, r := range rows {
			if !seen[r.Contract] {
				seen[r.Contract] = true
				names = append(names, r.Contract)
			}
		}
		sort.Strings(names)
		return HandlerResult{}, fmt.Errorf("查無契約 %s 的資料。可用代碼：%s", contract, strings.Join(names, ", "))
	}
	return HandlerResult{Data: filtered, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// largeTraderFutHistoryCap 為 T135 期貨大額交易人歷史之跨度上限（遠端對齊 31 日）。
const largeTraderFutHistoryCap = 31

// handlerGetLargeTradersFuturesHistory：期貨大額交易人未沖銷部位歷史
// （TAIFEX-DL largeTraderFutDown，T135；端點不支援契約過濾，本地端篩選；
// contract 必填，區間 ≤ 31 日）。
func handlerGetLargeTradersFuturesHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strings.TrimSpace(strVal(args["contract"])))
	if contract == "" {
		return HandlerResult{}, fmt.Errorf("參數 contract 為必填（例如 TX、MTX、TE、TF）")
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	sT, errT := model.ParseDate(sd)
	eT, _ := model.ParseDate(ed)
	if errT == nil && int(eT.Sub(sT).Hours()/24) > largeTraderFutHistoryCap {
		return HandlerResult{}, fmt.Errorf("查詢區間不可超過 %d 天，請縮小範圍", largeTraderFutHistoryCap)
	}
	byDay, err := q.FetchRange(ctx, model.TALargeTraderFut, sd, ed, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TALargeTraderFut, err)
	}
	all, err := collectRangeRows[model.LargeTraderRow](model.TALargeTraderFut, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	rows := make([]model.LargeTraderRow, 0, len(all))
	for _, r := range all {
		if r.Contract == contract {
			rows = append(rows, r)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].ContractMonth < rows[j].ContractMonth
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// optionsHistoryRowCap 為 T150 未指定 contract_month 時之資料量門檻，超過改列到期月份。
const optionsHistoryRowCap = 500

// handlerGetOptionsDailyHistory：選擇權每日 OHLC 歷史（TAIFEX-DL dlOptDataDown，
// T150）。contract 預設 TXO；contract_month/call_put 過濾；未指定 contract_month
// 且資料量過大時改列可用到期月份。
func handlerGetOptionsDailyHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strings.TrimSpace(strVal(args["contract"])))
	if contract == "" {
		contract = "TXO"
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	callPut := strings.TrimSpace(strVal(args["call_put"]))
	if callPut != "" && callPut != "買權" && callPut != "賣權" {
		return HandlerResult{}, fmt.Errorf("參數 call_put 僅接受「買權」或「賣權」，實際 %q", callPut)
	}
	contractMonth := strings.TrimSpace(strVal(args["contract_month"]))
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	byDay, err := q.FetchRange(ctx, model.TAOptionsDaily, sd, ed, contract)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAOptionsDaily, err)
	}
	all, err := collectRangeRows[model.OptionsDailyRow](model.TAOptionsDaily, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	months := map[string]bool{}
	for _, r := range all {
		if r.Contract == contract {
			months[r.ContractMonth] = true
		}
	}
	if contractMonth == "" && len(all) > optionsHistoryRowCap {
		list := make([]string, 0, len(months))
		for m := range months {
			list = append(list, m)
		}
		sort.Strings(list)
		out := make([]map[string]any, 0, len(list))
		for _, m := range list {
			out = append(out, map[string]any{"contract_month": m})
		}
		lg := taifexLineage(byDay[ed], ed, false, a.taifexTTL())
		lg.DerivedFrom = []string{"contract_months_list"}
		return HandlerResult{Data: out, Lineage: lg}, nil
	}
	rows := make([]model.OptionsDailyRow, 0, len(all))
	for _, r := range all {
		if r.Contract != contract {
			continue
		}
		if contractMonth != "" && r.ContractMonth != contractMonth {
			continue
		}
		if callPut != "" && r.CallPut != callPut {
			continue
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		if rows[i].Strike != rows[j].Strike {
			return rows[i].Strike < rows[j].Strike
		}
		return rows[i].CallPut < rows[j].CallPut
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// handlerGetOptionsDelta：選擇權每日 Delta（TAIFEX-API DailyOptionsDelta，T151）。
// contract 留空列所有契約；contract_month 留空列該契約可用月份；兩者皆給則過濾。
func handlerGetOptionsDelta(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strings.TrimSpace(strVal(args["contract"])))
	month := strings.TrimSpace(strVal(args["contract_month"]))
	callPut := strings.TrimSpace(strVal(args["call_put"]))
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAOptionsDelta, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAOptionsDelta, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", model.TAOptionsDelta, err)
	}
	// 過濾函式：Contract 精確、月份精確、買賣權精確
	keep := func(r map[string]any) bool {
		c, _ := r["Contract"].(string)
		m, _ := r["ContractMonth(Week)"].(string)
		cp, _ := r["CallPut"].(string)
		return (contract == "" || c == contract) &&
			(month == "" || m == month) &&
			(callPut == "" || cp == callPut)
	}
	if contract == "" {
		return HandlerResult{Data: distinctField(rows, "Contract", keep), Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	if month == "" {
		return HandlerResult{Data: distinctField(rows, "ContractMonth(Week)", keep), Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if keep(r) {
			out = append(out, r)
		}
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// distinctField 取 rows 中指定欄位之去重排序清單（僅保留符合 filter 之列）。
func distinctField(rows []map[string]any, field string, filter func(map[string]any) bool) []map[string]any {
	seen := map[string]bool{}
	vals := make([]string, 0)
	for _, r := range rows {
		if !filter(r) {
			continue
		}
		v, _ := r[field].(string)
		if v != "" && !seen[v] {
			seen[v] = true
			vals = append(vals, v)
		}
	}
	sort.Strings(vals)
	out := make([]map[string]any, 0, len(vals))
	for _, v := range vals {
		out = append(out, map[string]any{fieldToKey(field): v})
	}
	return out
}

// fieldToKey 將官方欄名轉為輸出鍵名。
func fieldToKey(field string) string {
	switch field {
	case "Contract":
		return "contract"
	case "ContractMonth(Week)":
		return "contract_month"
	default:
		return field
	}
}

// handlerGetOptionsOIChange：台指選擇權每日未平倉量增減（TAIFEX-API va01，T154；
// 今日與前一日未平倉及變化量）。
func handlerGetOptionsOIChange(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAOIChange, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAOIChange, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", model.TAOIChange, err)
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetStockFuturesMargin：股票期貨保證金一覽表（TAIFEX-API
// SingleStockFuturesMargining，T167）。stock_code 可為股票代號（2330）或
// 期貨契約代碼（CAF），留空回全部。
func handlerGetStockFuturesMargin(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	code := strings.TrimSpace(strVal(args["stock_code"]))
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAStockMargin, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAStockMargin, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", model.TAStockMargin, err)
	}
	if code == "" {
		return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		uc, _ := r["UnderlyingSecurityCode"].(string)
		ct, _ := r["Contract"].(string)
		if uc == code || strings.EqualFold(ct, code) {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return HandlerResult{}, fmt.Errorf("查無 %s 之股票期貨保證金資料", code)
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetInstitutionalTradersByFuturesHistory：三大法人期貨部位歷史
// （TAIFEX-DL futContractsDateDown，T132；contract 為 TXF 型代碼，預設 TXF，
// 伺服器端以 commodityId 過濾；區間 ≤ 92 日）。
func handlerGetInstitutionalTradersByFuturesHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.ToUpper(strings.TrimSpace(strVal(args["contract"])))
	if contract == "" {
		contract = "TXF" // 對齊遠端 default
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	sT, errT := model.ParseDate(sd)
	eT, _ := model.ParseDate(ed)
	if errT == nil && int(eT.Sub(sT).Hours()/24) > instiSplitRangeCap {
		return HandlerResult{}, fmt.Errorf("查詢區間不可超過 %d 天，請縮小範圍", instiSplitRangeCap)
	}
	byDay, err := q.FetchRange(ctx, model.TAInstiFutures, sd, ed, contract)
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
		return rows[i].Investor < rows[j].Investor
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// instiHistRangeHandler：T152/T153 共用之 DL 歷史查詢路徑（區間 ≤ 92 日，
// contract 為中文契約名子字串或留空全部；伺服器端以 commodityId 過濾）。
func instiHistByContract[T any](a *App, args map[string]any, ds model.TAIFEXDataset,
	defaultContract string, fetchContract func(string) string) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	if contract == "" {
		contract = defaultContract
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	sT, errT := model.ParseDate(sd)
	eT, _ := model.ParseDate(ed)
	if errT == nil && int(eT.Sub(sT).Hours()/24) > instiSplitRangeCap {
		return HandlerResult{}, fmt.Errorf("查詢區間不可超過 %d 天，請縮小範圍", instiSplitRangeCap)
	}
	fc := fetchContract(contract)
	byDay, err := q.FetchRange(ctx, ds, sd, ed, fc)
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", ds, err)
	}
	rows, err := collectRangeRows[T](ds, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// handlerGetOptionsInstiByContractHistory：三大法人各選擇權契約交易歷史
// （CALL+PUT 合計，TAIFEX-DL optContractsDateDown，T152）。
func handlerGetOptionsInstiByContractHistory(a *App, args map[string]any) (HandlerResult, error) {
	return instiHistByContract[model.InstitutionalRow](a, args, model.TAOptInstiByCont, "TXO",
		func(c string) string { return strings.ToUpper(c) })
}

// handlerGetOptionsInstiCallsPutsHistory：三大法人選擇權買賣權分計歷史
// （TAIFEX-DL callsAndPutsDateDown，T153）。
func handlerGetOptionsInstiCallsPutsHistory(a *App, args map[string]any) (HandlerResult, error) {
	return instiHistByContract[model.InstiCPRow](a, args, model.TAInstiCPHist, "TXO",
		func(c string) string { return strings.ToUpper(c) })
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

// handlerGetFuturesInstitutional：三大法人期貨與選擇權合計每日交易資訊
// （TAIFEX-API DividedByFuturesAndOptions，T126；date 省略為最新交易日，
// 直通保留官方欄位）。
func handlerGetFuturesInstitutional(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAInstiDivided, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAInstiDivided, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 於 %s 之資料（%s）", model.TAInstiDivided, d, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", model.TAInstiDivided, err)
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// instiWeeklyTypes 為 T204 週別 type → 資料集對應。
var instiWeeklyTypes = map[string]model.TAIFEXDataset{
	"general":       model.TAInstiGenWeek,
	"fut_opt":       model.TAInstiDivWeek,
	"fut_contracts": model.TAInstiFutContWeek,
	"opt_contracts": model.TAInstiOptContWeek,
	"calls_puts":    model.TAInstiCPWeek,
}

// handlerGetInstiWeekly：三大法人依週別系列（TAIFEX-API *BytheWeek，T204）。
// type 切換五型：general 總表／fut_opt 區分期貨與選擇權／fut_contracts 各期貨
// 契約／opt_contracts 各選擇權契約／calls_puts 買賣權分計；passthrough。
// 官方端點不接受日期過濾（恆回近期各週），limit/offset 分頁由本地端處理。
func handlerGetInstiWeekly(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	typeArg := strVal(args["type"])
	if typeArg == "" {
		typeArg = "general"
	}
	ds, ok := instiWeeklyTypes[typeArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("type 僅接受 general/fut_opt/fut_contracts/opt_contracts/calls_puts，得到 %q", typeArg)
	}
	// 週別端點不接受日期過濾（恆回近期各週），但仍需解析最新交易日作為
	// 快取鍵（空日期會走 DL 歷史查詢路徑，週別無 DL 支援）。
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if contract != "" && !strings.Contains(rowField(r, "ContractCode"), contract) {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// fspCategories 為 T205 category → 資料集對應。
var fspCategories = map[string]model.TAIFEXDataset{
	"all":           model.TAFSPAll,
	"futures":       model.TAFSPFutures,
	"index_futures": model.TAFSPIdxFut,
	"ssf":           model.TAFSPSSf,
	"index_options": model.TAFSPIdxOpt,
	"fx":            model.TAFSPFx,
	"gold":          model.TAFSPGold,
	"ir":            model.TAFSPIR,
	"options":       model.TASPOptions,
	"sso":           model.TAFSPSSO,
}

// handlerGetFinalSettlementPrice：最後結算價系列（TAIFEX-API
// FinalSettlementPrice*，T205）。category 切換商品類別；date 指定到期日
// （本地端過濾 TheFinalSettlementDay，省略回全部）；passthrough。
func handlerGetFinalSettlementPrice(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	catArg := strVal(args["category"])
	if catArg == "" {
		catArg = "all"
	}
	ds, ok := fspCategories[catArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("category 僅接受 all/futures/index_futures/ssf/index_options/fx/gold/ir/options/sso，得到 %q", catArg)
	}
	dateArg := strVal(args["date"])
	// 端點恆回近期各到期日全量，不接受日期參數；快取鍵固定用最新交易日
	// （傳入過去日期會誤走 DL 歷史查詢路徑），date 僅供本地端過濾。
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if dateArg != "" && rowField(r, "TheFinalSettlementDay") != strings.ReplaceAll(dateArg, "-", "") {
			continue
		}
		if contract != "" && !strings.Contains(rowField(r, "Contract"), contract) && !strings.Contains(rowField(r, "ContractName"), contract) {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// spCategories 為 T206 category → 資料集對應。
var spCategories = map[string]model.TAIFEXDataset{
	"all":           model.TASPAll,
	"futures":       model.TASFutures,
	"index_options": model.TASPIdxOpt,
	"fx":            model.TASPFx,
	"fx_futures":    model.TASPFxFut,
	"gold":          model.TASPGold,
	"ir":            model.TASPIR,
	"index_futures": model.TASPIdxFut,
	"options":       model.TASPOpt,
	"ssf":           model.TASPSSF,
	"sso":           model.TASPSSO,
}

// handlerGetSettledPositions：到期契約履約交割系列（TAIFEX-API
// SettledPositions*，T206）。category 切換商品類別；date 指定到期日
// （本地端過濾 TheFinalSettlementDay，省略回全部）；passthrough。
func handlerGetSettledPositions(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	catArg := strVal(args["category"])
	if catArg == "" {
		catArg = "all"
	}
	ds, ok := spCategories[catArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("category 僅接受 all/futures/index_options/fx/fx_futures/gold/ir/index_futures/options/ssf/sso，得到 %q", catArg)
	}
	dateArg := strVal(args["date"])
	// 端點恆回近期各到期日全量；快取鍵固定用最新交易日（傳入過去日期會
	// 誤走 DL 歷史查詢路徑），date 僅供本地端過濾。
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if dateArg != "" && rowField(r, "TheFinalSettlementDay") != strings.ReplaceAll(dateArg, "-", "") {
			continue
		}
		if contract != "" && !strings.Contains(rowField(r, "Contract"), contract) && !strings.Contains(rowField(r, "ContractName"), contract) {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// btKinds 為 T208 kind → 資料集對應。
var btKinds = map[string]model.TAIFEXDataset{
	"all":             model.TABlockTrade,
	"futures":         model.TABTFutInfo,
	"options":         model.TABTOptInfo,
	"summary_futures": model.TABTFutSummary,
	"summary_options": model.TABTOptSummary,
}

// handlerGetTaifexBlockTrade：鉅額交易系列（TAIFEX-API BlockTrade*，T208）。
// kind 切換五型：all 成交資訊全部／futures 期貨成交／options 選擇權成交／
// summary_futures 期貨量統計／summary_options 選擇權量統計；passthrough。
func handlerGetTaifexBlockTrade(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	kindArg := strVal(args["kind"])
	if kindArg == "" {
		kindArg = "all"
	}
	ds, ok := btKinds[kindArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("kind 僅接受 all/futures/options/summary_futures/summary_options，得到 %q", kindArg)
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if contract != "" && !strings.Contains(rowField(r, "Contract"), contract) && !strings.Contains(rowField(r, "ProductCategory"), contract) {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// sfsPeriods 為 T210 period → 資料集對應。
var sfsPeriods = map[string]model.TAIFEXDataset{
	"daily":    model.TAStockFutStatsD,
	"monthly":  model.TAStockFutStatsM,
	"yearly":   model.TAStockFutStatsY,
	"oi_daily": model.TAStockOptOID,
}

// handlerGetStockFuturesStats：個股期貨/選擇權交易統計 va 系列（TAIFEX-API，
// T210）。period 切換 daily（va12）/monthly（va13）/yearly（va14）/
// oi_daily（每日個股選擇權未平倉量增減，va02）；passthrough。
func handlerGetStockFuturesStats(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	period := strVal(args["period"])
	if period == "" {
		period = "daily"
	}
	ds, ok := sfsPeriods[period]
	if !ok {
		return HandlerResult{}, fmt.Errorf("period 僅接受 daily/monthly/yearly/oi_daily，得到 %q", period)
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	limit, offset := listPaging(args)
	out := make([]any, 0, len(rows))
	for _, r := range rows {
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
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// ssfKinds 為 T211 kind → 資料集對應。
var ssfKinds = map[string]model.TAIFEXDataset{
	"ssf_list": model.TASSFLists,
	"top10":    model.TASTFTop10,
	"sso_list": model.TASSOLists,
}

// handlerGetSSFOverview：股票期貨標的與前十大量（TAIFEX-API SSFLists/
// STFTop10/SSOLists，T211）。kind 切換三型；code 過濾 StockCode 或
// Contract；passthrough。
func handlerGetSSFOverview(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	kindArg := strVal(args["kind"])
	if kindArg == "" {
		kindArg = "ssf_list"
	}
	ds, ok := ssfKinds[kindArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("kind 僅接受 ssf_list/top10/sso_list，得到 %q", kindArg)
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	code := strings.TrimSpace(strVal(args["code"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if code != "" && rowField(r, "StockCode") != code && rowField(r, "Contract") != code {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerMarginTable：保證金一覽表四類別共用路徑（T209）。passthrough；
// contract 過濾（Contracts 或 Contract 欄位），查無時列出可用商品。
func handlerMarginTable(a *App, args map[string]any, ds model.TAIFEXDataset) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if contract == "" || strings.Contains(rowField(r, "Contract"), contract) || strings.Contains(rowField(r, "Contracts"), contract) {
			out = append(out, r)
		}
	}
	if len(out) == 0 && contract != "" {
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, rowField(r, "Contract", "Contracts"))
		}
		return HandlerResult{}, fmt.Errorf("查無「%s」。可用商品：%s", contract, strings.Join(names, "、"))
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetFxMargin／GetIrMargin／GetGoldMargin／GetEtfMargin：保證金一覽表
// 四類別（TAIFEX-API *Margining，T209）。
func handlerGetFxMargin(a *App, args map[string]any) (HandlerResult, error) {
	return handlerMarginTable(a, args, model.TAMarginFx)
}

func handlerGetIrMargin(a *App, args map[string]any) (HandlerResult, error) {
	return handlerMarginTable(a, args, model.TAMarginIR)
}

func handlerGetGoldMargin(a *App, args map[string]any) (HandlerResult, error) {
	return handlerMarginTable(a, args, model.TAMarginGold)
}

func handlerGetEtfMargin(a *App, args map[string]any) (HandlerResult, error) {
	return handlerMarginTable(a, args, model.TAMarginETF)
}

// fcmKinds 為 T230 kind → 資料集對應。
var fcmKinds = map[string]model.TAIFEXDataset{
	"lists":       model.TAFCMLists,
	"branch":      model.TAFCMBranchLists,
	"netvalue":    model.TAFCMNetValue,
	"income":      model.TAFCMIncome,
	"accumulated": model.TAFCMAccIncome,
}

// handlerGetFcmProfiles：期貨商名冊與財務概況（TAIFEX-API FCMLists 等，
// T230）。kind 切換 lists/branch/netvalue/income/accumulated；code 過濾
// FCMCode；passthrough。
func handlerGetFcmProfiles(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	kindArg := strVal(args["kind"])
	if kindArg == "" {
		kindArg = "lists"
	}
	ds, ok := fcmKinds[kindArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("kind 僅接受 lists/branch/netvalue/income/accumulated，得到 %q", kindArg)
	}
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	code := strings.TrimSpace(strVal(args["code"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if code != "" && rowField(r, "FCMCode") != code {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// posLimitCats 為 T231 category → 資料集對應。
var posLimitCats = map[string]model.TAIFEXDataset{
	"equity":     model.TAPosLimitEquity,
	"non_equity": model.TAPosLimitNonEq,
}

// handlerGetPositionLimits：交易人部位限制（TAIFEX-API PositionLimit*，
// T231）。category 切換 equity 個股類／non_equity 非個股類；contract 過濾；
// passthrough。
func handlerGetPositionLimits(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	catArg := strVal(args["category"])
	if catArg == "" {
		catArg = "equity"
	}
	ds, ok := posLimitCats[catArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("category 僅接受 equity 或 non_equity，得到 %q", catArg)
	}
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if contract != "" && rowField(r, "Contract") != contract {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// contractAdjViews 為 T231 view → 資料集對應。
var contractAdjViews = map[string]model.TAIFEXDataset{
	"adjust":   model.TAContractAdj,
	"adjusted": model.TASSFAdjustedInfo,
	"fee":      model.TAFeeSchedule,
}

// handlerGetContractAdjust：契約調整與收費標準（TAIFEX-API ContractAdj 等，
// T231）。view 切換 adjust 調整一覽事項／adjusted 調整型契約資訊／fee 收費
// 標準表；contract 過濾；passthrough。
func handlerGetContractAdjust(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	viewArg := strVal(args["view"])
	if viewArg == "" {
		viewArg = "adjust"
	}
	ds, ok := contractAdjViews[viewArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("view 僅接受 adjust/adjusted/fee，得到 %q", viewArg)
	}
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if contract != "" && rowField(r, "Contract") != contract && !strings.Contains(rowField(r, "StockId"), contract) {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// collKinds 為 T232 kind → 資料集對應。
var collKinds = map[string]model.TAIFEXDataset{
	"stock":         model.TACollStock,
	"bond":          model.TACollGovBond,
	"international": model.TACollIntlBond,
	"log":           model.TACollLogStock,
}

// collCodeKeys 為各 kind 之標的代號欄位。
var collCodeKeys = map[string]string{
	"stock":         "StockId",
	"bond":          "Code",
	"international": "InternationalBondCode",
	"log":           "StockId",
}

// handlerGetAcceptableCollateral：保證金可抵繳標的（TAIFEX-API
// AcceptableCollateral*，T232）。kind 切換 stock/bond/international/log；
// code 過濾標的代號；passthrough。
func handlerGetAcceptableCollateral(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	kindArg := strVal(args["kind"])
	if kindArg == "" {
		kindArg = "stock"
	}
	ds, ok := collKinds[kindArg]
	if !ok {
		return HandlerResult{}, fmt.Errorf("kind 僅接受 stock/bond/international/log，得到 %q", kindArg)
	}
	d, err := taifexDate(a, q, ctx, "")
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 之資料（%s）", ds, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	code := strings.TrimSpace(strVal(args["code"]))
	codeKey := collCodeKeys[kindArg]
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if code != "" && rowField(r, codeKey) != code {
			continue
		}
		out = append(out, r)
	}
	limit, offset := listPaging(args)
	if offset < len(out) {
		out = out[offset:]
	} else {
		out = []any{}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return HandlerResult{Data: out, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetInstitutionalGeneral：三大法人整體交易總表（期貨+選擇權合計，
// TAIFEX-API GeneralBytheDate，T129；端點回 CSV，date 省略為最新交易日）。
func handlerGetInstitutionalGeneral(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAInstiGeneral, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAInstiGeneral, err)
	}
	rows, err := taifexRows[model.InstiGeneralRow](model.TAInstiGeneral, d, res)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// instiSplitRangeCap 為 T128 分計歷史之最長跨度（遠端對齊 92 日）。
const instiSplitRangeCap = 92

// handlerGetInstitutionalFutOptSplitHistory：三大法人期貨/選擇權分計歷史
// （TAIFEX-DL futAndOptDateDown，T128；僅 DL 提供，區間 ≤ 92 日）。
func handlerGetInstitutionalFutOptSplitHistory(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	start := flexDateArg(strVal(args["start"]))
	if start == "" {
		start = flexDateArg(strVal(args["start_date"]))
	}
	end := flexDateArg(strVal(args["end"]))
	if end == "" {
		end = flexDateArg(strVal(args["end_date"]))
	}
	if start == "" || end == "" {
		return HandlerResult{}, fmt.Errorf("參數 start（start_date）與 end（end_date）為必填")
	}
	sd, ed, err := validateRange(start, end)
	if err != nil {
		return HandlerResult{}, err
	}
	sT, errT := model.ParseDate(sd)
	eT, _ := model.ParseDate(ed)
	if errT == nil && int(eT.Sub(sT).Hours()/24) > instiSplitRangeCap {
		return HandlerResult{}, fmt.Errorf("查詢區間不可超過 %d 天，請縮小範圍", instiSplitRangeCap)
	}
	byDay, err := q.FetchRange(ctx, model.TAInstiFutOptSplit, sd, ed, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 範圍查詢失敗: %w", model.TAInstiFutOptSplit, err)
	}
	rows, err := collectRangeRows[model.InstiSplitRow](model.TAInstiFutOptSplit, byDay)
	if err != nil {
		return HandlerResult{}, err
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date < rows[j].Date
		}
		return rows[i].Investor < rows[j].Investor
	})
	return HandlerResult{Data: rows, Lineage: rangeLineage(byDay, ed, a.taifexTTL())}, nil
}

// instiTradersByContract：三大法人依契約分類明細共用路徑（T131 期貨 / T133 選擇權）。
// contract_code 為中文契約名子字串過濾；留空回全部，查無時列出可用契約。
func instiTradersByContract(a *App, args map[string]any, ds model.TAIFEXDataset) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	code := strings.TrimSpace(strVal(args["contract_code"]))
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
	if code == "" {
		return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	filtered := make([]model.InstitutionalRow, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(r.Contract, code) {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		seen := map[string]bool{}
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			if !seen[r.Contract] {
				seen[r.Contract] = true
				names = append(names, r.Contract)
			}
		}
		sort.Strings(names)
		return HandlerResult{}, fmt.Errorf("查無契約「%s」。可用契約：%s", code, strings.Join(names, "、"))
	}
	return HandlerResult{Data: filtered, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetInstitutionalTradersByFutures：三大法人依各期貨契約分類（T131）。
func handlerGetInstitutionalTradersByFutures(a *App, args map[string]any) (HandlerResult, error) {
	return instiTradersByContract(a, args, model.TAInstiFutures)
}

// handlerGetInstitutionalTradersByOptions：三大法人依各選擇權契約分類（T133）。
func handlerGetInstitutionalTradersByOptions(a *App, args map[string]any) (HandlerResult, error) {
	return instiTradersByContract(a, args, model.TAInstiOptions)
}

// handlerGetInstitutionalTradersCallsPuts：三大法人選擇權買賣權分計明細
// （TAIFEX-API DetailsOfCallsAndPuts，T134；直通官方欄位，含 CallPut CALL/PUT）。
func handlerGetInstitutionalTradersCallsPuts(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	code := strings.TrimSpace(strVal(args["contract_code"]))
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAInstiCallsPuts, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAInstiCallsPuts, err)
	}
	if len(res.Data) == 0 {
		note := res.Note
		if note == "" {
			note = "無資料"
		}
		return HandlerResult{}, fmt.Errorf("官方無 %s 於 %s 之資料（%s）", model.TAInstiCallsPuts, d, note)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", model.TAInstiCallsPuts, err)
	}
	if code != "" {
		filtered := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			if s, _ := r["ContractCode"].(string); strings.Contains(s, code) {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}

// handlerGetIndexFuturesMargin：股價指數類期貨與選擇權保證金一覽表
// （TAIFEX-API IndexFuturesAndOptionsMargining，T127）。contract 為中文商品名
// 子字串過濾（如「臺股期貨」），留空回全部；查無時列出可用商品。
func handlerGetIndexFuturesMargin(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	contract := strings.TrimSpace(strVal(args["contract"]))
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	res, fromCache, err := q.Fetch(ctx, model.TAMargin, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", model.TAMargin, err)
	}
	rows, err := taifexRows[model.MarginRow](model.TAMargin, d, res)
	if err != nil {
		return HandlerResult{}, err
	}
	if contract == "" {
		return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
	}
	filtered := make([]model.MarginRow, 0, len(rows))
	for _, r := range rows {
		if strings.Contains(r.Contract, contract) {
			filtered = append(filtered, r)
		}
	}
	if len(filtered) == 0 {
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, r.Contract)
		}
		return HandlerResult{}, fmt.Errorf("查無「%s」。可用商品：%s", contract, strings.Join(names, "、"))
	}
	return HandlerResult{Data: filtered, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
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

// handlerGetTimeAndSales：期貨/選擇權逐筆成交（TAIFEX-API TimeAndSalesData /
// OptionsTimeAndSalesData，T207）。market 參數 futures（預設）/options；
// date 省略為最新交易日。tick 級資料量大，limit 上限 1000。
func handlerGetTimeAndSales(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	q, err := a.querier()
	if err != nil {
		return HandlerResult{}, err
	}
	market := strings.ToLower(strVal(args["market"]))
	if market == "" {
		market = "futures"
	}
	ds := model.TATickFutures
	if market == "options" {
		ds = model.TATickOptions
	}
	d, err := taifexDate(a, q, ctx, strVal(args["date"]))
	if err != nil {
		return HandlerResult{}, err
	}
	limit, offset := 50, 0
	if v, ok := args["limit"]; ok {
		if n, e := asInt(v); e == nil && n > 0 {
			limit = n
			if limit > 1000 {
				limit = 1000
			}
		}
	}
	if v, ok := args["offset"]; ok {
		if n, e := asInt(v); e == nil && n >= 0 {
			offset = n
		}
	}
	res, fromCache, err := q.Fetch(ctx, ds, d, "")
	if err != nil {
		return HandlerResult{}, fmt.Errorf("%s 取得失敗: %w", ds, err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		return HandlerResult{}, fmt.Errorf("mcp: %s 解析失敗: %w", ds, err)
	}
	if offset < len(rows) {
		rows = rows[offset:]
	} else {
		rows = []map[string]any{}
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return HandlerResult{Data: rows, Lineage: taifexLineage(res, d, fromCache, a.taifexTTL())}, nil
}
