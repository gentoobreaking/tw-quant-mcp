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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"tw-quant-mcp/pkg/model"
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

// ---------------------------------------------------------------------------
// ETF 分配收益資料源（TWSE rwd/zh/ETF/etfDiv）

// ETFDividendSource 為 TWSE ETF 分配收益資料源（rwd/zh/ETF/etfDiv）。
type ETFDividendSource struct {
	client *BaseClient
}

// NewETFDividendSource 建立 ETF 分配收益資料源（Rate Limit 1 req/s）。
func NewETFDividendSource(opts ...Option) *ETFDividendSource {
	return &ETFDividendSource{
		client: NewBaseClient("www.twse.com.tw", opts...),
	}
}

// ID 回傳資料源 ID。
func (s *ETFDividendSource) ID() string { return "TWSE_ETF_DIVIDEND" }

// etfDivResp 為 etfDiv 端點回應結構。
type etfDivResp struct {
	Status string     `json:"status"`
	Title  string     `json:"title"`
	Data   [][]string `json:"data"`
	Fields []string   `json:"fields"`
}

// FetchDividend 取得 ETF 分配收益歷史。
func (s *ETFDividendSource) FetchDividend(ctx context.Context, code, startDate, endDate string) ([]model.ETFDividendPoint, error) {
	params := url.Values{}
	params.Set("response", "json")
	params.Set("stkNo", code)
	params.Set("startDate", startDate)
	params.Set("endDate", endDate)
	u := "https://www.twse.com.tw/rwd/zh/ETF/etfDiv?" + params.Encode()

	req := RawRequest{
		Method: "GET",
		URL:    u,
		Headers: httpHeader(map[string]string{
			"Referer": "https://www.twse.com.tw/zh/ETFortune/dividendList",
		}),
	}
	resp, err := s.client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider: ETFDividend etfDiv 回傳 %d", resp.StatusCode)
	}

	var raw struct {
		Status string          `json:"status"`
		Title  string          `json:"title"`
		Data   [][]interface{} `json:"data"`
		Fields []string        `json:"fields"`
	}
	if err := json.Unmarshal(resp.Body, &raw); err != nil {
		return nil, fmt.Errorf("provider: ETFDividend JSON 解析失敗: %w", err)
	}
	if raw.Status != "ok" {
		return nil, fmt.Errorf("provider: ETFDividend 官方回應異常 status=%q", raw.Status)
	}

	points := make([]model.ETFDividendPoint, 0, len(raw.Data))
	for _, row := range raw.Data {
		if len(row) < 8 {
			continue
		}
		// 將 interface{} 轉為字串
		getStr := func(v interface{}) string {
			switch val := v.(type) {
			case string:
				return val
			case float64:
				return fmt.Sprintf("%.0f", val)
			case int:
				return fmt.Sprintf("%d", val)
			case json.Number:
				return val.String()
			default:
				return ""
			}
		}
		exDate := parseROCDateToISO(getStr(row[2]))
		recordDate := parseROCDateToISO(getStr(row[3]))
		payDate := parseROCDateToISO(getStr(row[4]))
		amount := parseFloatOrZero(getStr(row[5]))

		p := model.ETFDividendPoint{
			ExDate:       exDate,
			RecordDate:   recordDate,
			PayDate:      payDate,
			Amount:       amount,
			Standard:     getStr(row[6]),
			AnnounceYear: getStr(row[7]),
		}
		points = append(points, p)
	}
	return points, nil
}

// parseROCDateToISO 將民國年日期（如 "115年07月21日"）轉為 ISO 格式（YYYY-MM-DD）。
func parseROCDateToISO(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// 移除「年」「月」「日」
	s = strings.ReplaceAll(s, "年", "/")
	s = strings.ReplaceAll(s, "月", "/")
	s = strings.ReplaceAll(s, "日", "")
	parts := strings.Split(s, "/")
	if len(parts) != 3 {
		return ""
	}
	year, err1 := strconv.Atoi(parts[0])
	month, err2 := strconv.Atoi(parts[1])
	day, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return ""
	}
	// 民國年轉西元年
	year += 1911
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

// parseFloatOrZero 解析浮點數，失敗回傳 0。
func parseFloatOrZero(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// httpHeader 由 map 建立 http.Header。
func httpHeader(kv map[string]string) http.Header {
	h := make(http.Header, len(kv))
	for k, v := range kv {
		h.Set(k, v)
	}
	return h
}
