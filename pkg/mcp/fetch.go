package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// fetch.go 提供盤後工具（T011）共用之資料取得輔助：
// 快取讀穿（§12.2 GetOrFetch + §4.2 TTL 政策）與交易日歷導航。

// cacheDataset 將 provider 資料集 ID 對映至 §4.2 政策表資料類別
// （新增資料集於此登錄；未登錄資料類別視為不納管快取）。
var cacheDataset = map[string]string{
	string(provider.TWSEWDDailyK):           cache.DatasetDailyKLine,
	string(provider.TWSEWDMarketClose):      cache.DatasetDailyKLine,
	string(provider.TWSEWDInstitutional):    cache.DatasetInstitutional, // 與 TPEx institutional 同名
	string(provider.TWSEWDMargin):           cache.DatasetMargin,        // 與 TPEx margin 同名
	string(provider.TWSEWDAbnormal):         cache.DatasetAlertStock,
	string(provider.TWSEWDForeignQFIIS):     cache.DatasetForeignHold,
	string(provider.TWSEAPIForeignHoldings): cache.DatasetForeignHold,
	string(provider.TWSEAPIWarrants):        cache.DatasetWarrants,
	string(provider.TWSEAPIPunish):          cache.DatasetAlertStock,
	string(provider.TWSEAPIESG):             cache.DatasetESG,
	string(provider.TWSEAPIGovernance):      cache.DatasetESG,
	string(provider.TPExDailyClose):         cache.DatasetDailyKLine,
	string(provider.TPExAttention):          cache.DatasetAlertStock,
	string(provider.TPExDisposition):        cache.DatasetAlertStock,
	// MOPS Open Data（T012）
	string(provider.MOPSMonthlyRevenue):  cache.DatasetMonthlyRevenue,
	string(provider.MOPSIncomeSummary):   cache.DatasetFinancials,
	string(provider.MOPSProfitRatios):    cache.DatasetFinancials,
	string(provider.MOPSBalanceSheet):    cache.DatasetFinancials,
	string(provider.MOPSIncomeStatement): cache.DatasetFinancials,
	string(provider.MOPSCashFlow):        cache.DatasetFinancials,
	string(provider.MOPSAnnouncements):   cache.DatasetMaterialNews,
	string(provider.MOPSCompanyProfile):  cache.DatasetCalendar, // 24h TTL，類似行事曆
	// T014：TWSE-API 估值/除權息/股利分派資料
	string(provider.TWSEAPIValuation): cache.DatasetValuation,
	string(provider.TWSEAPIExDiv):     cache.DatasetExDivCalendar,
	string(provider.TWSEAPIDividend):  cache.DatasetDividend,
	string(provider.TPExPEValuation):  cache.DatasetValuation,
	string(provider.TPExExRights):     cache.DatasetExDivCalendar,
}

// policyDataset 回傳資料集對應之政策類別；未登錄時回傳原字串（cacheable=false）。
func policyDataset(dataset string) string {
	if c, ok := cacheDataset[dataset]; ok {
		return c
	}
	return dataset
}

// fetchNormalize 執行「快取讀穿 → Fetch → Validate → Normalize →
// Unmarshal」之標準路徑，回傳資料與是否命中快取（§3.2 is_cached）。
//   - srcID：資料源 ID（model.Source*，供快取鍵與 lineage）
//   - dataset：資料類別（§4.2，供 TTL 政策與 L2 資格）
//   - dataDate：資料歸屬日期（§4.2 索引，YYYY-MM-DD）
//   - key：快取鍵（含日期/代碼等識別，見 cache.KeyString）
//   - fetch：上游呼叫（URL 建構 + Fetch）；nil 時僅做快取讀穿
func fetchNormalize[T any](a *App, ctx context.Context, dataset, dataDate, key string,
	fetch func() ([]byte, error)) (T, bool, error) {
	var zero T
	if a.cache == nil {
		return zero, false, fmt.Errorf("mcp: 快取層未初始化")
	}
	dataset = policyDataset(dataset)
	ttl, cacheable := cache.TTLFor(dataset, a.now())
	fn := func(ctx context.Context) (T, error) {
		a.httpCalls.Add(1) // §12.9 instrumentation：快取 miss 即為一次上游 HTTP 請求
		raw, err := fetch()
		if err != nil {
			return zero, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return zero, fmt.Errorf("mcp: Normalize 結果解析失敗: %w", err)
		}
		return v, nil
	}
	if !cacheable {
		v, err := fn(ctx)
		return v, false, err
	}
	return cache.GetOrFetch(ctx, a.cache, key, ttl, fn,
		cache.WithDataset(dataset, dataDate))
}

// fetchRaw 為 fetchNormalize 之原始版：fetch 端即做 Fetch+Validate+
// Normalize（回傳 normalizer 產出之 []byte），僅快取鍵與 TTL 在此層管理。
func (a *App) fetchRaw(ctx context.Context, dataset, dataDate, key string,
	fetch func() ([]byte, error)) (cached bool, raw []byte, err error) {
	if a.cache == nil {
		return false, nil, fmt.Errorf("mcp: 快取層未初始化")
	}
	dataset = policyDataset(dataset)
	ttl, cacheable := cache.TTLFor(dataset, a.now())
	fn := func(ctx context.Context) ([]byte, error) {
		a.httpCalls.Add(1) // §12.9 instrumentation
		return fetch()
	}
	if !cacheable {
		b, err := fn(ctx)
		return false, b, err
	}
	b, fromCache, err := cache.GetOrFetch(ctx, a.cache, key, ttl, fn,
		cache.WithDataset(dataset, dataDate))
	return fromCache, b, err
}

// prevTradingDay 回傳 d 之前第 n 個交易日（n≥1）；搜尋上限 60 日。
func (a *App) prevTradingDay(d time.Time, n int) (time.Time, error) {
	if n < 1 {
		n = 1
	}
	day := d.AddDate(0, 0, -1)
	for i := 0; i < 60 && n > 0; i++ {
		if a.calendar.IsTradingDay(day) {
			n--
			if n == 0 {
				return day, nil
			}
		}
		day = day.AddDate(0, 0, -1)
	}
	return d, fmt.Errorf("mcp: 60 日內找不到第 %d 個交易日", n)
}

// resolveDate 解析工具之 date 參數（YYYY-MM-DD，可空）：
//   - 給定：驗證格式並回傳；
//   - 空：回傳最近交易日（盤後時段 ≥15:00 為今日，否則前一日）。
func (a *App) resolveDate(date string) (string, error) {
	if date != "" {
		t, err := model.ParseDate(date)
		if err != nil {
			return "", fmt.Errorf("mcp: 參數 date 格式必須為 YYYY-MM-DD: %w", err)
		}
		return model.FormatDate(t), nil
	}
	now := a.now()
	day := now
	if now.Hour()*3600+now.Minute()*60 < 15*3600 || !a.calendar.IsTradingDay(now) {
		prev, err := a.prevTradingDay(now, 1)
		if err != nil {
			return "", err
		}
		day = prev
	}
	return model.FormatDate(day), nil
}

// symbolOf 解析 symbol 參數並查 Symbol Registry。
func (a *App) symbolOf(code string) (model.Symbol, error) {
	sym, ok := a.symbols.Lookup(code)
	if !ok {
		return model.Symbol{}, fmt.Errorf("非法代號 %q（未註冊於 Symbol Registry）", code)
	}
	return sym, nil
}
