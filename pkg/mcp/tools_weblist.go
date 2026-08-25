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
