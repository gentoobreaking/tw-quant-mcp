// ETF e添富平台資料源 Adapter（spec §30.1 L1）。
//
// 2026-08 實測：TWSE 舊版 NAV 端點（etfEstimateNAV/etfNav/etfDailyNAV/
// etfPremiumDiscount 等）已全部 404/302；e添富平台
// （www.twse.com.tw/zh/ETFortune/）為現行官方 ETF 資訊入口，其商品頁
// （etfInfo/<code>）以 POST ajaxEtfInfoChart 取得歷史淨值與折溢價：
//   - type=fundPric → {"netPrice":[{date,count}...],"atmps":[{date,count}...]}
//     netPrice 為每單位淨值；atmps 為折溢價率（%）。
//   - type=close → [{date,count}...] 歷史市價。
//
// 僅上市 ETF（含主動 ETF 00400A 等）；上櫃 ETF（00679B 等）不在本平台。
// 債券型等部分 ETF 可能無 netPrice 資料（回傳空陣列，Handler 需處理）。
package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// ETFChartType 為 ajaxEtfInfoChart 之 type 參數。
type ETFChartType string

const (
	ETFChartClose    ETFChartType = "close"    // 歷史市價
	ETFChartFundPric ETFChartType = "fundPric" // 歷史淨值 + 折溢價
)

// ETFortuneSource 為 TWSE ETF e添富平台資料源（www.twse.com.tw/zh/ETFortune）。
type ETFortuneSource struct {
	client *BaseClient
	base   string
}

// NewETFortuneSource 建立 e添富資料源（Rate Limit 1 req/s）。
func NewETFortuneSource(opts ...Option) *ETFortuneSource {
	return &ETFortuneSource{
		client: NewBaseClient("www.twse.com.tw", opts...),
		base:   "https://www.twse.com.tw/zh/ETFortune",
	}
}

// ID 回傳資料源 ID（TWSE_WEB 之一部分，共用主機）。
func (s *ETFortuneSource) ID() string { return "TWSE_ETFORTUNE" }

// ChartURL 建構 ajaxEtfInfoChart 之請求 URL。
func (s *ETFortuneSource) ChartURL(code string, chartType ETFChartType, start, end string) string {
	form := url.Values{}
	form.Set("id", code)
	form.Set("startDate", start) // YYYY/MM/DD
	form.Set("endDate", end)
	form.Set("type", string(chartType))
	return s.base + "/ajaxEtfInfoChart"
}

// ChartBody 回傳 POST form-urlencoded body。
func ChartBody(code string, chartType ETFChartType, start, end string) []byte {
	form := url.Values{}
	form.Set("id", code)
	form.Set("startDate", start)
	form.Set("endDate", end)
	form.Set("type", string(chartType))
	return []byte(form.Encode())
}

// ChartFetch 執行 chart 請求並回傳原始 body。
func (s *ETFortuneSource) ChartFetch(ctx context.Context, code string, chartType ETFChartType, start, end string) ([]byte, error) {
	u := s.ChartURL(code, chartType, start, end)
	req := RawRequest{
		Method: "POST",
		URL:    u,
		Body:   ChartBody(code, chartType, start, end),
		Headers: httpHeader(map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
			"Referer":      s.base + "/etfInfo/" + strings.TrimSpace(code),
		}),
	}
	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider: ETFortune ajaxEtfInfoChart 回傳 %d", resp.StatusCode)
	}
	return resp.Body, nil
}

// httpHeader 由 map 建立 http.Header。
func httpHeader(kv map[string]string) http.Header {
	h := make(http.Header, len(kv))
	for k, v := range kv {
		h.Set(k, v)
	}
	return h
}
