// Package registry 實作 §5.2 Symbol Registry 之載入管線：
// TWSE 上市清單 + TPEx 上櫃清單官方 OpenAPI → model.Symbol（經 Symbol.Validate
// 檢查）→ 24h TTL 快取（§4.2「交易日曆 / 公司代碼表」）並每日預熱入 L2。
// 盤中引擎與預熱排程（T018）皆以本套件之 Registry 判定市場別（§5.2）：
// MIS ex_ch 組裝一律經 Registry，禁止猜測市場別（v1.2 已知缺失）。
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// 官方資料源（§5.2：TWSE/TPEx 官方上市上櫃公司清單 openapi）。
// 以變數而非 const 提供，供測試以 httptest 注入。
var (
	twseListURL = "https://openapi.twse.com.tw/v1/opendata/t187ap05_L"           // 上市股票公司基本資料
	tpexListURL = "https://www.tpex.org.tw/openapi/v1/tpex_mainboard_daily_close_quotes"
	twseETFListURL = "https://openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL" // 全市場收盤（含 ETF）
)

// SetListURLs 覆寫官方清單 URL（僅測試用：httptest 注入，T018 預熱排程測試）。
func SetListURLs(twse, tpex string) { twseListURL, tpexListURL = twse, tpex }

// SetETFListURL 覆寫 ETF 清單 URL（僅測試用）。
func SetETFListURL(url string) { twseETFListURL = url }

// registryTTL 對應 §4.2「交易日曆 / 公司代碼表」24h 盤中/盤後 TTL。
const registryTTL = 24 * time.Hour

// Loader 自官方清單載入 Registry，並以 cache.GetOrFetch（§12.2 Single-flight）
// 合流：24h 內同鍵僅一次上游呼叫，資料落入 L2（24h ≥ l2WriteMinTTL）。
type Loader struct {
	client *provider.BaseClient
	cache  *cache.Cache
}

// NewLoader 建立 Registry 載入器。client 為必要（官方清單抓取）；
// cache 為選用（nil 時直接抓取、不快取）。
func NewLoader(client *provider.BaseClient, c *cache.Cache) *Loader {
	return &Loader{client: client, cache: c}
}

// Load 回傳最新 Registry 快照（上市 + 上櫃 + ETF 合併）。任一市場清單載入失敗即回傳錯誤：
// Registry 為核心基礎設施，缺市場別可能導致 ex_ch 路由錯誤，寧可快速失敗。
// ETF 清單載入失敗不阻擋整體 Registry（記 warning，log 輸出）——避免 ETF 端點異常時影響既有股票工具。
func (l *Loader) Load(ctx context.Context) (*model.Registry, error) {
	if l.client == nil {
		return nil, fmt.Errorf("registry: client 為 nil")
	}
	date := model.FormatDate(model.Now().Time)

	tse, err := l.loadMarket(ctx, twseListURL, date, parseTWSEList)
	if err != nil {
		return nil, fmt.Errorf("registry: TWSE 上市清單載入失敗: %w", err)
	}
	otc, err := l.loadMarket(ctx, tpexListURL, date, parseTPExList)
	if err != nil {
		return nil, fmt.Errorf("registry: TPEx 上櫃清單載入失敗: %w", err)
	}

	// 載入 ETF 清單（失敗不阻擋整體 Registry）
	etfs, err := l.loadMarket(ctx, twseETFListURL, date, parseTWSEETFList)
	if err != nil {
		// 僅記錄警告，不回傳錯誤
		fmt.Printf("registry: ETF 清單載入失敗（不影響既有工具）: %v\n", err)
		etfs = nil
	}

	all := append(tse, otc...)
	if len(etfs) > 0 {
		all = append(all, etfs...)
	}

	reg := model.NewRegistry()
	if err := reg.Set(all); err != nil {
		return nil, err
	}
	return reg, nil
}

// loadMarket 載入單一市場清單；24h TTL 快取鍵依 §4.3 建構（dataset=calendar）。
func (l *Loader) loadMarket(ctx context.Context, url, date string,
	parse func([]byte) ([]model.Symbol, error)) ([]model.Symbol, error) {
	fetch := func(ctx context.Context) ([]model.Symbol, error) {
		resp, err := l.client.Do(ctx, provider.RawRequest{URL: url})
		if err != nil {
			return nil, err
		}
		return parse(resp.Body)
	}
	if l.cache == nil {
		return fetch(ctx)
	}
	key := cache.KeyString(sourceIDFor(url), cache.DatasetCalendar, date, "", map[string]string{"url": url})
	opts := []cache.FetchOption{cache.WithDataset(cache.DatasetCalendar, date)}
	symbols, _, err := cache.GetOrFetch(ctx, l.cache, key, registryTTL, fetch, opts...)
	return symbols, err
}

// sourceIDFor 對應 §2 資料來源登錄：TWSE/TPEx 清單之來源 ID（快取鍵用）。
func sourceIDFor(url string) string {
	if url == tpexListURL {
		return model.SourceTPExAPI
	}
	return model.SourceTWSEAPI
}

// twseETFListRow 對應 TWSE openapi STOCK_DAY_ALL 之欄位（ETF 篩選用）。
type twseETFListRow struct {
	Code  string `json:"Code"`
	Name  string `json:"Name"`
	Date  string `json:"Date"`  // 民國年日期
	Close string `json:"ClosingPrice"` // 收盤價（用於判斷是否有效交易）
}

// parseTWSEETFList 解析 TWSE 全市場收盤清單（STOCK_DAY_ALL），
// 保留以 00 開頭之上市 ETF/ETN，涵蓋 4 碼（0050）、5 碼（00636）與 6 碼（006208、
// 00400A 主動 ETF、00631L 槓反）。4 碼一般股票、權證、2 碼代號等一律排除；
// 6 碼且非 00 開頭（020000 系列 ETN、01001T 不動產投資信託、2887Z1 特別股、
// 910322 DR）排除（另由 TWSE 上市清單處理）。產業別留空（官方未提供）。
// 已知限制：00899 FT潔淨能源為 STOCK_DAY_ALL 中唯一可能非 ETF 之 00 開頭列，
// 官方未提供類型欄位，先保留（見 pkg/registry/loader_test.go TestParseTWSEETFList）。
func parseTWSEETFList(body []byte) ([]model.Symbol, error) {
	var rows []twseETFListRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("registry: STOCK_DAY_ALL JSON 解析失敗: %w", err)
	}
	symbols := make([]model.Symbol, 0, len(rows))
	for _, r := range rows {
		code := strings.TrimSpace(r.Code)
		name := strings.TrimSpace(r.Name)
		// 僅保留以 00 開頭之代號（上市 ETF/ETN：4/5/6 碼皆有可能）
		if len(code) < 4 || len(code) > 6 || code[:2] != "00" {
			continue
		}
		if name == "" {
			continue
		}
		s := model.Symbol{Code: code, Market: model.MarketTSE, Name: name, Category: ""}
		if err := s.Validate(); err != nil {
			continue
		}
		symbols = append(symbols, s)
	}
	return symbols, nil
}

// twseListRow 對應 TWSE openapi t187ap05_L 之欄位（§5.2 Registry 來源）。
type twseListRow struct {
	Code     string `json:"公司代號"`
	Name     string `json:"公司名稱"`
	Category string `json:"產業別"`
}

// parseTWSEList 解析 TWSE 上市清單；不合契約之記錄略過
// （Symbol.Validate），全部無效視為官方格式變更並回傳錯誤。
func parseTWSEList(body []byte) ([]model.Symbol, error) {
	var rows []twseListRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("registry: TWSE 清單 JSON 解析失敗: %w", err)
	}
	symbols := make([]model.Symbol, 0, len(rows))
	for _, r := range rows {
		s := model.Symbol{Code: r.Code, Market: model.MarketTSE, Name: r.Name, Category: r.Category}
		if err := s.Validate(); err != nil {
			continue
		}
		symbols = append(symbols, s)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("registry: TWSE 清單解析後無有效記錄（官方格式可能變更）")
	}
	return symbols, nil
}

// tpexListRow 對應 TPEx openapi tpex_mainboard_daily_close_quotes 之欄位。
// 註：TPEx OpenAPI 無公司主清單端點（§T005 查證 swagger 全表），
// 以每日收盤行情清單為官方來源，涵蓋上櫃股票與上櫃 ETF 等有價證券；
// 產業別（category）官方未提供機器可讀欄位，留空。
type tpexListRow struct {
	Code string `json:"SecuritiesCompanyCode"`
	Name string `json:"CompanyName"`
}

// parseTPExList 解析 TPEx 上櫃清單（同 parseTWSEList 之跳過/全滅規則）。
// 資料源（tpex_mainboard_daily_close_quotes）為全市場收盤行情，含大量
// 權證（6 碼 7xx 開頭，約 9,600 檔）與 ETN（6 碼 02 開頭）；此處僅保留
// 可交易標的：
//   - 4 碼：上櫃股票（含興櫃股票代號區間）
//   - 5 碼：上櫃 ETF（00858 等）與特別股（8349A 等）
//   - 6 碼 00/02 開頭：上櫃 ETF/ETN（006201、00679B、020001 等）
// 排除 6 碼 7xx 開頭之權證（約 9,600 檔）。
func parseTPExList(body []byte) ([]model.Symbol, error) {
	var rows []tpexListRow
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("registry: TPEx 清單 JSON 解析失敗: %w", err)
	}
	symbols := make([]model.Symbol, 0, len(rows))
	for _, r := range rows {
		code := strings.TrimSpace(r.Code)
		if len(code) == 6 && strings.HasPrefix(code, "7") {
			// 6 碼 7xx 開頭：權證，排除
			continue
		}
		s := model.Symbol{Code: code, Market: model.MarketOTC, Name: r.Name}
		if err := s.Validate(); err != nil {
			continue
		}
		symbols = append(symbols, s)
	}
	if len(symbols) == 0 {
		return nil, fmt.Errorf("registry: TPEx 清單解析後無有效記錄（官方格式可能變更）")
	}
	return symbols, nil
}
