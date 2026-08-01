package calendar

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

// holidayScheduleURL 為 TWSE 官方開休市表 JSON API（僅提供當年資料）。
// 以變數而非 const 提供，供測試以 httptest 注入。
var holidayScheduleURL = "https://www.twse.com.tw/holidaySchedule/holidaySchedule?response=json"

// SetScheduleURL 覆寫開休市表 URL（僅測試用：httptest 注入，T018 預熱排程測試）。
func SetScheduleURL(u string) { holidayScheduleURL = u }

// scheduleTTL 對應 §4.2「交易日曆 / 公司代碼表」24h TTL。
const scheduleTTL = 24 * time.Hour

// scheduleResponse 為 holidaySchedule API 之回應結構。
type scheduleResponse struct {
	Stat  string     `json:"stat"`
	Date  string     `json:"date"` // 資料版本（YYYYMMDD）
	Title string     `json:"title"`
	Data  [][]string `json:"data"` // [日期, 名稱, 說明]
}

// LoadFromOfficial 自 TWSE 官方開休市表抓取當年資料，以 24h TTL 快取
// （§4.2 行事曆，落入 L2）後合併入 Calendar。client 為必要；cache 為選用
// （nil 時直接抓取）。
func (c *Calendar) LoadFromOfficial(ctx context.Context, client *provider.BaseClient, cch *cache.Cache) error {
	if client == nil {
		return fmt.Errorf("calendar: client 為 nil")
	}
	date := model.FormatDate(model.Now().Time)
	fetch := func(ctx context.Context) ([]Holiday, error) {
		resp, err := client.Do(ctx, provider.RawRequest{URL: holidayScheduleURL})
		if err != nil {
			return nil, err
		}
		return parseSchedule(resp.Body)
	}

	var (
		holidays []Holiday
		err      error
	)
	if cch == nil {
		holidays, err = fetch(ctx)
	} else {
		key := cache.KeyString(model.SourceTWSEWeb, cache.DatasetCalendar, date, "", nil)
		holidays, _, err = cache.GetOrFetch(ctx, cch, key, scheduleTTL, fetch,
			cache.WithDataset(cache.DatasetCalendar, date))
	}
	if err != nil {
		return err
	}
	c.Merge(holidays)
	return nil
}

// parseSchedule 解析官方開休市表 JSON 並回傳休市日清單。
// 名稱含「交易日」者為交易日標記（如「國曆新年開始交易日」、
// 「農曆春節前最後交易日」）而非休市日，予以排除。
func parseSchedule(body []byte) ([]Holiday, error) {
	var resp scheduleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("calendar: 開休市表 JSON 解析失敗: %w", err)
	}
	if resp.Stat != "ok" {
		return nil, fmt.Errorf("calendar: 開休市表回應異常（stat=%q）", resp.Stat)
	}
	var out []Holiday
	for _, row := range resp.Data {
		if len(row) < 2 {
			continue
		}
		if strings.Contains(row[1], "交易日") {
			continue
		}
		note := ""
		if len(row) > 2 {
			note = row[2]
		}
		out = append(out, Holiday{Date: row[0], Name: row[1], Note: note})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("calendar: 開休市表解析後無休市日（官方格式可能變更）")
	}
	return out, nil
}
