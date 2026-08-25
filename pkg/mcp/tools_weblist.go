package mcp

// tools_weblist.go：TWSE-WEB 報表清單型工具之共用框架（parity 任務 T042+）。
// 模式：fetchWebRaw → ParseWebReport → rowMap → code/name 過濾 → limit/offset 分頁。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// webListSpec 描述一個報表清單型工具。
type webListSpec struct {
	ds       provider.TWSEWebDataset
	withDate bool // 端點接受 date 參數並帶入 query
}

// handler 產生共用 handler：
// 可用過濾參數：code（完全比對輸出列之 "code"）、name（部分比對 "name"）；
// 分頁參數 limit（預設 50）/offset（預設 0）。
func (s webListSpec) handler() func(*App, map[string]any) (HandlerResult, error) {
	return func(a *App, args map[string]any) (HandlerResult, error) {
		ctx := context.Background()
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
		codeArg, nameArg := strVal(args["code"]), strVal(args["name"])

		var params url.Values
		dataDate := a.now().Format("2006-01-02")
		if s.withDate {
			d, err := a.resolveDate(strVal(args["date"]))
			if err != nil {
				return HandlerResult{}, err
			}
			dataDate = d
			params = url.Values{"date": {dateYMD(d)}}
		}

		rows, cached, stale, err := fetchNormalize[[]map[string]any](a, ctx,
			string(s.ds), dataDate,
			cache.KeyString(model.SourceTWSEWeb, string(s.ds), dataDate,
				codeArg+nameArg, vals(params)),
			func() ([]byte, error) { return a.fetchWebRaw(ctx, s.ds, params) })
		if err != nil {
			return HandlerResult{}, err
		}

		ttl, _ := a.ttlOf(string(s.ds))
		lineage := postLineage(model.SourceTWSEWeb, dataDate, cached || stale, stale, ttl)

		if rows == nil {
			return HandlerResult{Data: []any{}, Lineage: lineage}, nil
		}
		return HandlerResult{Data: paginateRows(rows, codeArg, nameArg, offset, limit), Lineage: lineage}, nil
	}
}

// apiListSpec 描述 TWSE-API（openapi）報表清單型工具（無 date 參數，
// 官方恆回全量最新資料；T056 起）。
type apiListSpec struct {
	ds provider.TWSEAPIDataset
}

// handler 產生共用 handler：同 webListSpec 之過濾/分頁語意。
func (s apiListSpec) handler() func(*App, map[string]any) (HandlerResult, error) {
	return func(a *App, args map[string]any) (HandlerResult, error) {
		ctx := context.Background()
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
		codeArg, nameArg := strVal(args["code"]), strVal(args["name"])

		dataDate := a.now().Format("2006-01-02")
		rows, cached, stale, err := fetchNormalize[[]map[string]any](a, ctx,
			string(s.ds), dataDate,
			cache.KeyString(model.SourceTWSEAPI, string(s.ds), dataDate, codeArg+nameArg, nil),
			func() ([]byte, error) { return a.fetchAPIRaw(ctx, s.ds, nil) })
		if err != nil {
			return HandlerResult{}, err
		}

		ttl, _ := a.ttlOf(string(s.ds))
		lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)

		if rows == nil {
			return HandlerResult{Data: []any{}, Lineage: lineage}, nil
		}
		return HandlerResult{Data: paginateRows(rows, codeArg, nameArg, offset, limit), Lineage: lineage}, nil
	}
}

// rowField 取列中多個候選鍵之第一個非空值（中文/英文欄名相容）。
func rowField(r map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := r[k]; ok {
			s := fmt.Sprint(v)
			if s != "" {
				return s
			}
		}
	}
	return ""
}

// rowCode / rowName：過濾用之代碼/名稱欄位（相容官方中文欄名）。
func rowCode(r map[string]any) string {
	return rowField(r, "code", "公司代號", "證券代號", "債券代號")
}

func rowName(r map[string]any) string {
	return rowField(r, "name", "公司名稱", "證券名稱", "債券簡稱")
}

func paginateRows(rows []map[string]any, code, name string, offset, limit int) []any {
	out := make([]any, 0, len(rows))
	for _, r := range rows {
		if code != "" && rowCode(r) != code {
			continue
		}
		if name != "" && !strings.Contains(rowName(r), name) {
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
	return out
}

// webFetchDate 由 raw 頂層 date（YYYYMMDD）轉 YYYY-MM-DD；失敗回空字串。
func webFetchDate(raw []byte) string {
	var meta struct {
		Date string `json:"date"`
	}
	_ = json.Unmarshal(raw, &meta)
	if ts, err := time.Parse("20060102", meta.Date); err == nil {
		return ts.Format("2006-01-02")
	}
	return ""
}

// commaInt64 / commaFloat：解析含千分位逗號之數字（解析失敗回 0）。
func commaInt64(s string) int64 {
	n, _ := strconv.ParseInt(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 10, 64)
	return n
}

func commaFloat(s string) float64 {
	f, _ := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	return f
}

// rocToDate：「115/08/03」→「2026-08-03」（無法解析時原樣回傳）。
func rocToDate(s string) string {
	p := strings.Split(strings.TrimSpace(s), "/")
	if len(p) != 3 {
		return s
	}
	y, err := strconv.Atoi(p[0])
	if err != nil {
		return s
	}
	mm, err2 := strconv.Atoi(p[1])
	if err2 != nil {
		return s
	}
	return fmt.Sprintf("%04d-%02d-%s", y+1911, mm, p[2])
}

// ---------------------------------------------------------------------------
// 財報類（t187ap07_L{suffix}，T067；後續 t187ap06_L{suffix} 等共用）

// financialSuffixDatasets 將遠端產業別 suffix 對應至本機 API 資料集。
type financialSuffixSpec struct {
	datasets   map[string]provider.TWSEAPIDataset
	pathPrefix string // 供錯誤訊息標示
}

// financialSuffix 由產業別關鍵字推導遠端 API 之 suffix（順序：保險 → 金控 →
// 證券期貨 → 金融 → 異業，預設一般業 _ci）。
func financialSuffix(category string) string {
	c := strings.TrimSpace(category)
	switch {
	case strings.Contains(c, "保險"):
		return "_ins"
	case strings.Contains(c, "金控") || strings.Contains(c, "金融控股"):
		return "_fh"
	case strings.Contains(c, "證券"), strings.Contains(c, "期貨"):
		return "_bd"
	case strings.Contains(c, "金融"):
		return "_basi"
	case strings.Contains(c, "異業"):
		return "_mim"
	default:
		return "_ci"
	}
}

// fetchFinancialRowsForCode 取得指定資料集全量並過濾出公司代號相符之列
//（官方端點恆回全量最新，code 過濾於本地端進行）。
func fetchFinancialRowsForCode(a *App, ctx context.Context, ds provider.TWSEAPIDataset,
	dataDate, code string) ([]map[string]any, bool, bool, error) {
	return fetchNormalize[[]map[string]any](a, ctx,
		string(ds), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(ds), dataDate, code, nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, ds, nil) })
}

// financialSuffixOrder 回傳以推導 suffix 優先、其餘依序嘗試之完整順序。
func financialSuffixOrder(category string) []string {
	primary := financialSuffix(category)
	all := []string{"_ci", "_basi", "_bd", "_fh", "_ins", "_mim"}
	out := []string{primary}
	for _, s := range all {
		if s != primary {
			out = append(out, s)
		}
	}
	return out
}

// fetchFinancialRowsFallback 依 suffix 順序逐一取得並過濾 code，
// 任一格式命中即回傳；全部落空才回錯（cached/stale 取最後一次結果）。
func fetchFinancialRowsFallback(a *App, ctx context.Context, datasets map[string]provider.TWSEAPIDataset,
	category, dataDate, code string) ([]map[string]any, bool, bool, provider.TWSEAPIDataset, error) {
	var lastErr error
	cachedAny, staleAny := false, false
	for _, suf := range financialSuffixOrder(category) {
		ds, ok := datasets[suf]
		if !ok {
			continue
		}
		rows, cached, stale, err := fetchFinancialRowsForCode(a, ctx, ds, dataDate, code)
		if err != nil {
			lastErr = err
			continue
		}
		cachedAny = cachedAny || cached
		staleAny = staleAny || stale
		out := make([]map[string]any, 0)
		for _, r := range rows {
			if rowField(r, "公司代號", "code") == code {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return out, cachedAny, staleAny, ds, nil
		}
	}
	if lastErr != nil {
		return nil, false, false, "", lastErr
	}
	return nil, false, false, "", fmt.Errorf("查無 %s 之資料（已嘗試全部財報格式）", code)
}

// handlerGetCompanyBalanceSheet：上市公司資產負債表（依產業別選端點，T067）。
func handlerGetCompanyBalanceSheet(a *App, args map[string]any) (HandlerResult, error) {
	code := strVal(args["code"])
	if code == "" {
		return HandlerResult{}, fmt.Errorf("code 為必填參數")
	}
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	ctx := context.Background()
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, ds, err := fetchFinancialRowsFallback(a, ctx, balanceSheetDatasets,
		financialSuffix(sym.Category), dataDate, code)
	if len(rows) == 0 {
		return HandlerResult{}, fmt.Errorf("查無 %s（%s）之資產負債表資料", code, sym.Name)
	}
	ttl, _ := a.ttlOf(string(ds))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)
	return HandlerResult{Data: rows, Lineage: lineage}, nil
}

var balanceSheetDatasets = map[string]provider.TWSEAPIDataset{
	"_ci":   provider.TWSEAPIBalCI,
	"_basi": provider.TWSEAPIBalBASI,
	"_bd":   provider.TWSEAPIBalBD,
	"_fh":   provider.TWSEAPIBalFH,
	"_ins":  provider.TWSEAPIBalINS,
	"_mim":  provider.TWSEAPIBalMIM,
}

// apiCompanySpec 為「依公司代號查詢」之 TWSE-API 報表工具（fetch t187apXX_L
// 全量 → 本地過濾 公司代號/code == code；T072 起）。
type apiCompanySpec struct {
	ds provider.TWSEAPIDataset
}

func (s apiCompanySpec) handler() func(*App, map[string]any) (HandlerResult, error) {
	return func(a *App, args map[string]any) (HandlerResult, error) {
		code := strVal(args["code"])
		if code == "" {
			return HandlerResult{}, fmt.Errorf("code 為必填參數")
		}
		if _, err := a.symbolOf(code); err != nil {
			return HandlerResult{}, err
		}
		ctx := context.Background()
		dataDate := a.now().Format("2006-01-02")
		rows, cached, stale, err := fetchFinancialRowsForCode(a, ctx, s.ds, dataDate, code)
		if err != nil {
			return HandlerResult{}, err
		}
		ttl, _ := a.ttlOf(string(s.ds))
		lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)
		out := make([]map[string]any, 0)
		for _, r := range rows {
			if rowField(r, "公司代號", "code") == code {
				out = append(out, r)
			}
		}
		if len(out) == 0 {
			return HandlerResult{}, fmt.Errorf("查無 %s 之資料", code)
		}
		return HandlerResult{Data: out, Lineage: lineage}, nil
	}
}

// esgCompanySpec 為「依公司代號查詢 ESG 主題揭露」之工具（t187ap46_L_<topic>，
// 取報告年度最新一列之 Fields；T068 起公司治理/ESG 細項任務共用）。
type esgCompanySpec struct {
	topic int
}

func (s esgCompanySpec) handler() func(*App, map[string]any) (HandlerResult, error) {
	return func(a *App, args map[string]any) (HandlerResult, error) {
		code := strVal(args["code"])
		if code == "" {
			return HandlerResult{}, fmt.Errorf("code 為必填參數")
		}
		sym, err := a.symbolOf(code)
		if err != nil {
			return HandlerResult{}, err
		}
		ctx := context.Background()
		dataDate := a.now().Format("2006-01-02")
		rows, cached, stale, err := fetchNormalize[[]provider.ESGRow](a, ctx,
			string(provider.TWSEAPIESG), dataDate,
			cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIESG), dataDate, "",
				map[string]string{"topic": strconv.Itoa(s.topic)}),
			func() ([]byte, error) {
				return a.fetchAPIRaw(ctx, provider.TWSEAPIESG, url.Values{"topic": {strconv.Itoa(s.topic)}})
			})
		if err != nil {
			return HandlerResult{}, err
		}
		ttl, _ := a.ttlOf(string(provider.TWSEAPIESG))
		lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)

		var latest *provider.ESGRow
		for i := range rows {
			r := &rows[i]
			if r.Code != code {
				continue
			}
			if latest == nil || r.Year > latest.Year {
				latest = r
			}
		}
		if latest == nil {
			return HandlerResult{}, fmt.Errorf("查無 %s（%s）之該主題揭露資料", code, sym.Name)
		}
		return HandlerResult{Data: map[string]any{
			"code": latest.Code, "name": latest.Name, "year": latest.Year,
			"report_date": latest.ReportDate, "fields": latest.Fields,
		}, Lineage: lineage}, nil
	}
}

// incomeStatementDatasets 對應綜合損益表六種產業格式（T092）。
var incomeStatementDatasets = map[string]provider.TWSEAPIDataset{
	"_ci":   provider.TWSEAPIIncCI,
	"_basi": provider.TWSEAPIIncBASI,
	"_bd":   provider.TWSEAPIIncBD,
	"_fh":   provider.TWSEAPIIncFH,
	"_ins":  provider.TWSEAPIIncINS,
	"_mim":  provider.TWSEAPIIncMIM,
}

// handlerGetCompanyIncomeStatement：上市公司綜合損益表（依產業別選端點，T092）。
func handlerGetCompanyIncomeStatement(a *App, args map[string]any) (HandlerResult, error) {
	code := strVal(args["code"])
	if code == "" {
		return HandlerResult{}, fmt.Errorf("code 為必填參數")
	}
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	ctx := context.Background()
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, ds, err := fetchFinancialRowsFallback(a, ctx, incomeStatementDatasets,
		financialSuffix(sym.Category), dataDate, code)
	if len(rows) == 0 {
		return HandlerResult{}, fmt.Errorf("查無 %s（%s）之綜合損益表資料", code, sym.Name)
	}
	ttl, _ := a.ttlOf(string(ds))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)
	return HandlerResult{Data: rows, Lineage: lineage}, nil
}
