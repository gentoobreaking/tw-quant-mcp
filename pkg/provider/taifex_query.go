package provider

// TAIFEX 查詢層（T013，§9.3）：統一入口，串接 TAIFEX-API（最新交易日）與
// TAIFEX-DL（歷史回溯）兩 canonical 路徑，並落地 L2 永久 TTL 快取。
//
// 查詢流程（§9.3）：
//
//	查詢 (dataset, date)
//	  → 檢查 L2（命中→回傳，key 以 (dataset, date) 為鍵）
//	  → 若 date == 最新交易日 → TAIFEX-API（hot tier）
//	  → 否則 → TAIFEX-DL 下載 CSV（rate limit 1 req/5s，範圍 [date-3, date]）
//	  → 解析 → 驗證（欄位數、數值、日期）→ Normalize → 寫入 L2（永久 TTL）
//
// 缺口處理（§9.3）：單日下載失敗（HTTP/解析錯誤）時退而求其次由鄰近交易日
// 補檔（標註 derived_from），否則以 null 回傳並註明缺口（Note）。缺口結果
// 不寫入 L2（避免永久鎖死可能晚到的資料）。
//
// 快取鍵慣例：所有 TAIFEX 資料統一以 sourceID="TAIFEX" 建鍵
// （cache.KeyString("TAIFEX", <dataset>, <date>, <contract>, nil)），
// 使 API/DL 兩路徑共享同一 L2 項目（資料等價，§2.1 兩路徑皆 canonical）。
// mcp 層（T015）沿用此慣例即命中既有 L2。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
)

// taifexKeySource 為 TAIFEX 資料於快取鍵之統一來源 ID（見檔頭說明）。
const taifexKeySource = "TAIFEX"

// gapRetryWindow 為 DL 單日下載之回溯補檔視窗（含當日共 4 個曆日）。
const gapRetryWindow = 3

// TAIFEXQueryResult 為單一 (dataset, date) 之查詢結果。
// Data 為 Normalized Model 之 JSON 陣列；缺口時為 nil（配合 Note 註明）。
// DerivedFrom 非空表示以鄰近交易日補檔（§9.3 缺口處理）。
type TAIFEXQueryResult struct {
	Data        json.RawMessage `json:"data"`
	Source      string          `json:"source"`       // model.SourceTAIFEXAPI / SourceTAIFEXDL
	DerivedFrom string          `json:"derived_from"` // YYYY-MM-DD；空=無補檔
	Note        string          `json:"note"`         // 缺口註記；空=正常
}

// gapError 承載缺口結果，經 GetOrFetch 錯誤路徑回傳（缺口不寫入 L2）。
type gapError struct{ res TAIFEXQueryResult }

func (e *gapError) Error() string { return "taifex gap: " + e.res.Note }

// TAIFEXQuery 為 TAIFEX 統一查詢入口（§9.3）。
type TAIFEXQuery struct {
	api   *TAIFEXAPISource
	dl    *TAIFEXDLSource
	cache *cache.Cache
	now   func() time.Time

	mu        sync.Mutex
	latestAt  string // 最新交易日快取之台北日期
	latestDay string // 最新交易日 YYYY-MM-DD
}

// NewTAIFEXQuery 建立查詢層；cache 為 nil 時回傳錯誤。
func NewTAIFEXQuery(api *TAIFEXAPISource, dl *TAIFEXDLSource, c *cache.Cache, now func() time.Time) (*TAIFEXQuery, error) {
	if api == nil || dl == nil {
		return nil, fmt.Errorf("provider: TAIFEXQuery 需 api 與 dl 來源")
	}
	if now == nil {
		now = model.TaipeiNow
	}
	return &TAIFEXQuery{api: api, dl: dl, cache: c, now: now}, nil
}

// queryKey 建構 (dataset, date, contract) 之統一快取鍵。
func (q *TAIFEXQuery) queryKey(ds model.TAIFEXDataset, date, contract string) string {
	return cache.KeyString(taifexKeySource, string(ds), date, contract, nil)
}

// Fetch 依 §9.3 流程取得 (dataset, date) 之 Normalized 資料。
// fromCache=true 表示資料來自快取（L1/L2）。
func (q *TAIFEXQuery) Fetch(ctx context.Context, ds model.TAIFEXDataset, date, contract string) (TAIFEXQueryResult, bool, error) {
	if q.cache == nil {
		return TAIFEXQueryResult{}, false, fmt.Errorf("provider: 快取層未初始化")
	}
	key := q.queryKey(ds, date, contract)
	res, fromCache, err := cache.GetOrFetch(ctx, q.cache, key, cache.ForeverTTL,
		q.loadFn(ctx, ds, date, contract),
		cache.WithDataset(cache.DatasetTAIFEXHistory, date))
	if err != nil {
		var ge *gapError
		if errors.As(err, &ge) {
			return ge.res, fromCache, nil // 缺口：null 資料 + Note
		}
		return TAIFEXQueryResult{}, fromCache, err
	}
	return res, fromCache, nil
}

// loadFn 建構 GetOrFetch 之上游載入函式（回傳 gapError 使缺口不寫入 L2）。
func (q *TAIFEXQuery) loadFn(ctx context.Context, ds model.TAIFEXDataset, date, contract string) func(context.Context) (TAIFEXQueryResult, error) {
	return func(ctx context.Context) (TAIFEXQueryResult, error) {
		res, err := q.load(ctx, ds, date, contract)
		if err != nil {
			return res, err
		}
		if res.Note != "" {
			return res, &gapError{res: res}
		}
		return res, nil
	}
}

// load 為上游載入：date==最新交易日 → API；否則 → DL（含缺口補檔）。
func (q *TAIFEXQuery) load(ctx context.Context, ds model.TAIFEXDataset, date, contract string) (TAIFEXQueryResult, error) {
	latest, err := q.latestTradingDay(ctx)
	if err == nil && date == latest && apiSupported(ds) {
		if res, err := q.loadAPI(ctx, ds, date, contract); err == nil {
			return res, nil
		}
		// API 失敗 → 退回 DL（cold tier 兜底，兩路徑皆 canonical）
	}
	if dlSupported(ds) {
		return q.loadDL(ctx, ds, date, contract)
	}
	return TAIFEXQueryResult{}, fmt.Errorf("provider: 資料集 %q 之歷史查詢需 DL 支援", ds)
}

// apiSupported 判定資料集是否由 API 提供。
func apiSupported(ds model.TAIFEXDataset) bool {
	_, ok := taifexAPIPaths[ds]
	return ok
}

// loadAPI 走 TAIFEX-API（最新交易日）：URL 帶 date/contract 過濾參數。
func (q *TAIFEXQuery) loadAPI(ctx context.Context, ds model.TAIFEXDataset, date, contract string) (TAIFEXQueryResult, error) {
	params := queryOfParams(date, contract)
	url := q.api.URL(ds, params)
	raw, err := q.api.Fetch(ctx, RawRequest{URL: url})
	if err != nil {
		return TAIFEXQueryResult{}, err
	}
	if err := q.api.Validate(raw); err != nil {
		return TAIFEXQueryResult{}, err
	}
	data, err := q.api.Normalize(raw)
	if err != nil {
		return TAIFEXQueryResult{}, err
	}
	// 過濾後無資料列（如 API 僅回最新日而請求日不符）→ 視為未命中
	if emptyJSONArray(data) {
		return TAIFEXQueryResult{}, fmt.Errorf("provider: API 無 %s %s 之資料", ds, date)
	}
	return TAIFEXQueryResult{Data: data, Source: model.SourceTAIFEXAPI}, nil
}

// loadDL 走 TAIFEX-DL：下載 [date-3, date] 視窗，取指定日資料；該日無資料
// 而視窗內有鄰近日資料時補檔（derived_from），否則缺口（null + Note）。
func (q *TAIFEXQuery) loadDL(ctx context.Context, ds model.TAIFEXDataset, date, contract string) (TAIFEXQueryResult, error) {
	start := addDays(date, -gapRetryWindow)
	raw, err := q.downloadDL(ctx, ds, start, date, contract)
	if err != nil {
		return TAIFEXQueryResult{}, fmt.Errorf("provider: %s 下載失敗: %w", ds, err)
	}
	groups, err := normalizeDLByDate(raw)
	if err != nil {
		return TAIFEXQueryResult{}, err
	}
	if rows, ok := groups[date]; ok {
		return TAIFEXQueryResult{Data: rows, Source: model.SourceTAIFEXDL}, nil
	}
	// 該日無資料：視窗內找最近之鄰近日補檔
	for i := gapRetryWindow; i >= 1; i-- {
		if rows, ok := groups[addDays(date, -i)]; ok {
			return TAIFEXQueryResult{
				Data:        rows,
				Source:      model.SourceTAIFEXDL,
				DerivedFrom: addDays(date, -i),
			}, nil
		}
	}
	return TAIFEXQueryResult{
		Note: fmt.Sprintf("官方無 %s 之 %s 資料（可能為非交易日或缺口）", date, ds),
	}, nil
}

// downloadDL 下載 [start, end] 視窗之 CSV（DL 端點支援範圍參數）。
func (q *TAIFEXQuery) downloadDL(ctx context.Context, ds model.TAIFEXDataset, start, end, contract string) (*RawResponse, error) {
	params := urlValues(map[string]string{
		"queryStartDate": dlDateParam(start),
		"queryEndDate":   dlDateParam(end),
	})
	if contract != "" {
		params.Set("commodity_id", contract)
	}
	url := q.dl.URL(ds, params)
	raw, err := q.dl.Fetch(ctx, RawRequest{URL: url})
	if err != nil {
		return nil, err
	}
	if err := q.dl.Validate(raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// normalizeDLByDate 將 DL 原始 CSV Normalize 後依日期分組（YYYY-MM-DD → JSON 陣列）。
// 週六/無交易日之僅表頭 CSV 視為空分組（各請求日皆為缺口）。
func normalizeDLByDate(raw *RawResponse) (map[string]json.RawMessage, error) {
	data, err := normalizeTAIFEXDL(raw)
	if err != nil {
		if strings.Contains(err.Error(), "無資料列") {
			return map[string]json.RawMessage{}, nil
		}
		return nil, err
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("provider: DL Normalize 結果解析失敗: %w", err)
	}
	groups := map[string][]json.RawMessage{}
	for _, row := range rows {
		var m map[string]any
		if err := json.Unmarshal(row, &m); err != nil {
			return nil, err
		}
		d, _ := m["date"].(string)
		if d != "" {
			groups[d] = append(groups[d], row)
		}
	}
	out := make(map[string]json.RawMessage, len(groups))
	for d, rs := range groups {
		b, err := json.Marshal(rs)
		if err != nil {
			return nil, err
		}
		out[d] = b
	}
	return out, nil
}

// FetchRange 依範圍取得多日資料（§9.2 範圍參數，一次 DL 請求覆蓋 [start, end]）。
// 回傳 map[YYYY-MM-DD]結果；範圍內非交易日以缺口（Note）標記。
// 各日結果皆寫入 L2（永久 TTL）。
func (q *TAIFEXQuery) FetchRange(ctx context.Context, ds model.TAIFEXDataset, start, end, contract string) (map[string]TAIFEXQueryResult, error) {
	if !dlSupported(ds) {
		return nil, fmt.Errorf("provider: 資料集 %q 之範圍查詢需 DL 支援", ds)
	}
	if q.cache == nil {
		return nil, fmt.Errorf("provider: 快取層未初始化")
	}

	out := map[string]TAIFEXQueryResult{}
	missing := []string{}
	for d := start; d <= end; d = addDays(d, 1) {
		key := q.queryKey(ds, d, contract)
		if v, ok, err := cache.Get[TAIFEXQueryResult](ctx, q.cache, key, cache.WithDataset(cache.DatasetTAIFEXHistory, d)); err != nil {
			return nil, err
		} else if ok {
			out[d] = v
			continue
		}
		missing = append(missing, d)
	}
	if len(missing) == 0 {
		return out, nil
	}

	// 一次 DL 下載 [start-3, end]（補檔視窗）
	raw, err := q.downloadDL(ctx, ds, addDays(start, -gapRetryWindow), end, contract)
	if err != nil {
		return nil, err
	}
	groups, err := normalizeDLByDate(raw)
	if err != nil {
		return nil, err
	}
	for _, d := range missing {
		var res TAIFEXQueryResult
		if rows, ok := groups[d]; ok {
			res = TAIFEXQueryResult{Data: rows, Source: model.SourceTAIFEXDL}
		} else {
			// 範圍內無資料：往前找最近鄰近日補檔
			for i := 1; i <= gapRetryWindow; i++ {
				if rows, ok := groups[addDays(d, -i)]; ok {
					res = TAIFEXQueryResult{Data: rows, Source: model.SourceTAIFEXDL, DerivedFrom: addDays(d, -i)}
					break
				}
			}
			if res.Data == nil {
				res = TAIFEXQueryResult{Note: fmt.Sprintf("官方無 %s 之 %s 資料（可能為非交易日或缺口）", d, ds)}
			}
		}
		key := q.queryKey(ds, d, contract)
		_, _, err := cache.GetOrFetch(ctx, q.cache, key, cache.ForeverTTL,
			func(context.Context) (TAIFEXQueryResult, error) { return res, nil },
			cache.WithDataset(cache.DatasetTAIFEXHistory, d))
		if err != nil {
			return nil, err
		}
		out[d] = res
	}
	return out, nil
}

// latestTradingDay 回傳最新交易日（YYYY-MM-DD，記憶體快取，每日刷新）。
// 以 API 之 PutCallRatio（回傳近一個月）或 margin 之最大 Date 判定；
// API 失敗時回傳錯誤，呼叫端退回 DL 路徑。
func (q *TAIFEXQuery) latestTradingDay(ctx context.Context) (string, error) {
	today := model.FormatDate(q.now())
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.latestAt == today {
		return q.latestDay, nil
	}
	latest, err := q.discoverLatest(ctx)
	if err != nil {
		return "", err
	}
	q.latestAt, q.latestDay = today, latest
	return latest, nil
}

// discoverLatest 以 API 判定最新交易日。
func (q *TAIFEXQuery) discoverLatest(ctx context.Context) (string, error) {
	// PutCallRatio 回傳近一月多日；取其最大 Date 為最新交易日
	url := q.api.URL(model.TAPutCallRatio, urlValues(nil))
	raw, err := q.api.Fetch(ctx, RawRequest{URL: url})
	if err != nil {
		return "", fmt.Errorf("provider: 判定最新交易日失敗: %w", err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return "", fmt.Errorf("provider: PutCallRatio 解析失敗: %w", err)
	}
	latest := ""
	for _, r := range rows {
		m := taifexAPIMap(r)
		if d, ok := taifexAPIDate(m["Date"]); ok && d > latest {
			latest = d
		}
	}
	if latest == "" {
		return "", fmt.Errorf("provider: 無法判定最新交易日（PutCallRatio 無有效日期）")
	}
	return latest, nil
}

// ---------------------------------------------------------------------------
// 小工具

// urlValues 建立 url.Values。
func urlValues(m map[string]string) url.Values {
	v := url.Values{}
	for k, val := range m {
		v.Set(k, val)
	}
	return v
}

// queryOfParams 建構 API 過濾參數（date/contract）。
func queryOfParams(date, contract string) url.Values {
	m := map[string]string{}
	if date != "" {
		m["date"] = date
	}
	if contract != "" {
		m["contract"] = contract
	}
	return urlValues(m)
}

// addDays 回傳 YYYY-MM-DD 加減 n 日之日期字串。
func addDays(date string, n int) string {
	t, err := model.ParseDate(date)
	if err != nil {
		return date
	}
	return model.FormatDate(t.AddDate(0, 0, n))
}

// emptyJSONArray 判定 JSON 是否為空陣列（"[]" 或空白後之 "[]"）。
func emptyJSONArray(b []byte) bool {
	return strings.TrimSpace(string(b)) == "[]"
}
