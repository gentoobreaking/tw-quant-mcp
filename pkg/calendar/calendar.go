// Package calendar 實作台股交易日曆（規格書附錄 A：伺服器僅於台灣交易所
// 交易日運作採樣）。資料來源為 TWSE 官方開休市表（holidaySchedule JSON API，
// 僅提供當年資料）；內嵌 2026 年（115 年）官方資料為離線 fallback（標註版本），
// 並支援以官方資料合併更新。盤中引擎與預熱排程（T018）皆以 IsTradingDay
// 判定是否執行。
package calendar

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"tw-quant-mcp/pkg/model"
)

// embeddedVersion 為內嵌開休市表之版本標記（來源：TWSE holidaySchedule
// API 之 date 欄位，2026-01-01 抓取）。官方新年度資料以 LoadFromOfficial 更新。
const embeddedVersion = "TWSE-holidaySchedule-2026-01-01"

// Holiday 為開休市表之單筆記錄（官方欄位：日期/名稱/說明）。
type Holiday struct {
	Date string `json:"date"` // 休市日 YYYY-MM-DD
	Name string `json:"name"` // 節日/事件名稱
	Note string `json:"note,omitempty"`
}

// Calendar 為交易日曆：週末規則 + 官方休市日集合。
type Calendar struct {
	mu       sync.RWMutex
	closures map[string]string // date(YYYY-MM-DD) → 名稱
	version  string
}

// New 建立內嵌 2026 年官方開休市表之行事曆。
func New() *Calendar {
	c := &Calendar{closures: make(map[string]string), version: embeddedVersion}
	c.Merge(embedded2026())
	return c
}

// embedded2026 為 TWSE 官方 115 年（2026）市場開休市日期表之休市日
// （來源：https://www.twse.com.tw/holidaySchedule/holidaySchedule?response=json，
// 2026-01-01 抓取；「開始/最後交易日」與週末非休市日不列入）。
func embedded2026() []Holiday {
	rows := []Holiday{
		{"2026-01-01", "中華民國開國紀念日", "依規定放假1日。"},
		{"2026-02-12", "農曆春節前市場無交易", "市場無交易，僅辦理結算交割作業。"},
		{"2026-02-13", "農曆春節前市場無交易", "市場無交易，僅辦理結算交割作業。"},
		{"2026-02-15", "農曆除夕及春節", "依規定於2月15日至2月19日放假5日。"},
		{"2026-02-16", "農曆除夕及春節", "依規定於2月15日至2月19日放假5日。"},
		{"2026-02-17", "農曆除夕及春節", "依規定於2月15日至2月19日放假5日。"},
		{"2026-02-18", "農曆除夕及春節", "依規定於2月15日至2月19日放假5日。"},
		{"2026-02-19", "農曆除夕及春節", "依規定於2月15日至2月19日放假5日。"},
		{"2026-02-20", "農曆除夕及春節", "2月15日適逢星期日，於2月20日（星期五）補假。"},
		{"2026-02-27", "和平紀念日", "和平紀念日為2月28日適逢星期六，於2月27日（星期五）補假。"},
		{"2026-02-28", "和平紀念日", "依規定放假1日。"},
		{"2026-04-03", "兒童節及民族掃墓節", "兒童節為4月4日適逢星期六，於4月3日（星期五）補假。"},
		{"2026-04-04", "兒童節及民族掃墓節", "依規定放假1日。"},
		{"2026-04-05", "兒童節及民族掃墓節", "依規定放假1日。"},
		{"2026-04-06", "兒童節及民族掃墓節", "民族掃墓節為4月5日適逢星期日，於4月6日（星期一）補假。"},
		{"2026-05-01", "勞動節", "依規定放假1日。"},
		{"2026-06-19", "端午節", "依規定放假1日。"},
		{"2026-09-25", "中秋節", "依規定放假1日。"},
		{"2026-09-28", "孔子誕辰紀念日/教師節", "依規定放假1日。"},
		{"2026-10-09", "國慶日", "國慶日為10月10日適逢星期六，於10月9日（星期五）補假。"},
		{"2026-10-10", "國慶日", "依規定放假1日。"},
		{"2026-10-25", "臺灣光復暨金門古寧頭大捷紀念日", "依規定放假1日。"},
		{"2026-10-26", "臺灣光復暨金門古寧頭大捷紀念日", "10月25日適逢星期日，於10月26日（星期一）補假。"},
		{"2026-12-25", "行憲紀念日", "依規定放假1日。"},
	}
	return rows
}

// IsTradingDay 判定 date 是否為交易日：週六/週日休市；其餘依官方休市表
// （未登錄年份僅依週末規則判定）。
func (c *Calendar) IsTradingDay(date time.Time) bool {
	d := date.In(model.Taipei())
	if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		return false
	}
	c.mu.RLock()
	_, closed := c.closures[d.Format("2006-01-02")]
	c.mu.RUnlock()
	return !closed
}

// IsTradingDate 以 YYYY-MM-DD 字串判定交易日。
func (c *Calendar) IsTradingDate(s string) (bool, error) {
	t, err := model.ParseDate(s)
	if err != nil {
		return false, fmt.Errorf("calendar: 日期格式必須為 YYYY-MM-DD: %w", err)
	}
	return c.IsTradingDay(t), nil
}

// Holidays 回傳指定年份之官方休市清單（依日期排序）。
func (c *Calendar) Holidays(year int) []Holiday {
	prefix := fmt.Sprintf("%04d-", year)
	c.mu.RLock()
	defer c.mu.RUnlock()
	var out []Holiday
	for date, name := range c.closures {
		if len(date) >= 4 && date[:4] == prefix[:4] {
			out = append(out, Holiday{Date: date, Name: name})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out
}

// Version 回傳行事曆資料版本（內嵌或官方更新後之版本標記）。
func (c *Calendar) Version() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.version
}

// Merge 以官方開休市資料併入行事曆（同日期覆寫名稱/說明）。
// 日期須為 YYYY-MM-DD；無效記錄略過。
func (c *Calendar) Merge(holidays []Holiday) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, h := range holidays {
		if _, err := time.Parse("2006-01-02", h.Date); err != nil || h.Name == "" {
			continue
		}
		c.closures[h.Date] = h.Name
	}
}
