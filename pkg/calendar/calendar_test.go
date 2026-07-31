package calendar

import (
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

func tp(y, m, d int) time.Time {
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, model.Taipei())
}

// 內嵌 2026 官方開休市表：節日休市日。
func TestIsTradingDayHolidays(t *testing.T) {
	c := New()
	cases := []struct {
		date time.Time
		name string
	}{
		{tp(2026, 1, 1), "元旦"},
		{tp(2026, 2, 16), "春節（初一）"},
		{tp(2026, 2, 20), "春節（補假）"},
		{tp(2026, 2, 27), "和平紀念日補假"},
		{tp(2026, 4, 6), "清明節補假"},
		{tp(2026, 5, 1), "勞動節"},
		{tp(2026, 6, 19), "端午節"},
		{tp(2026, 9, 25), "中秋節"},
		{tp(2026, 9, 28), "教師節"},
		{tp(2026, 10, 9), "國慶日補假"},
		{tp(2026, 10, 26), "光復節補假"},
		{tp(2026, 12, 25), "行憲紀念日"},
	}
	for _, tc := range cases {
		if c.IsTradingDay(tc.date) {
			t.Errorf("%s（%s）應為休市日", tc.name, tc.date.Format("2006-01-02"))
		}
	}
}

// 週末與補班日：週六/週日一律休市（補班日股市不開盤，§T005 查證）。
func TestIsTradingDayWeekend(t *testing.T) {
	c := New()
	cases := []struct {
		date time.Time
		name string
	}{
		{tp(2026, 2, 21), "週六補班日（2026 唯一補班日）"},
		{tp(2026, 6, 20), "一般週六"},
		{tp(2026, 6, 21), "週日"},
	}
	for _, tc := range cases {
		if c.IsTradingDay(tc.date) {
			t.Errorf("%s（%s）應為休市日", tc.name, tc.date.Format("2006-01-02"))
		}
	}
}

// 交易日：官方「開始交易日」標記與一般平日。
func TestIsTradingDayOpen(t *testing.T) {
	c := New()
	cases := []struct {
		date time.Time
		name string
	}{
		{tp(2026, 1, 2), "國曆新年開始交易日"},
		{tp(2026, 2, 11), "農曆春節前最後交易日"},
		{tp(2026, 2, 23), "農曆春節後開始交易日"},
		{tp(2026, 7, 31), "一般平日"},
	}
	for _, tc := range cases {
		if !c.IsTradingDay(tc.date) {
			t.Errorf("%s（%s）應為交易日", tc.name, tc.date.Format("2006-01-02"))
		}
	}
}

// 未登錄年份：僅週末規則（2027 元旦為週五 → 交易日）。
func TestIsTradingDayUnlistedYear(t *testing.T) {
	c := New()
	if !c.IsTradingDay(tp(2027, 1, 1)) {
		t.Error("2027-01-01 無官方資料，依週末規則應為交易日")
	}
	if c.IsTradingDay(tp(2027, 1, 2)) {
		t.Error("2027-01-02 為週六，應為休市日")
	}
}

// IsTradingDate 字串介面與錯誤處理。
func TestIsTradingDate(t *testing.T) {
	c := New()
	if ok, err := c.IsTradingDate("2026-02-16"); err != nil || ok {
		t.Errorf("2026-02-16 應為休市日，實際 ok=%v err=%v", ok, err)
	}
	if ok, err := c.IsTradingDate("2026-07-31"); err != nil || !ok {
		t.Errorf("2026-07-31 應為交易日，實際 ok=%v err=%v", ok, err)
	}
	if _, err := c.IsTradingDate("2026/02/16"); err == nil {
		t.Error("非 YYYY-MM-DD 格式應回傳錯誤")
	}
}

// Holidays(year) 回傳該年官方休市清單（依日期排序）。
func TestHolidays(t *testing.T) {
	c := New()
	list := c.Holidays(2026)
	if len(list) != 24 {
		t.Fatalf("2026 年休市日應為 24 筆（官方表），實際 %d", len(list))
	}
	if list[0].Date != "2026-01-01" {
		t.Errorf("休市清單應依日期排序，首筆 %s", list[0].Date)
	}
	if list[1].Date != "2026-02-12" {
		t.Errorf("休市清單次筆應為 02-12，實際 %s", list[1].Date)
	}
	if got := c.Holidays(2025); len(got) != 0 {
		t.Errorf("2025 未登錄應為空清單，實際 %d", len(got))
	}
}

// Merge 併入官方資料（如臨時颱風假）後 IsTradingDay 生效。
func TestMerge(t *testing.T) {
	c := New()
	if !c.IsTradingDay(tp(2026, 7, 10)) {
		t.Fatal("前置：2026-07-10 原本應為交易日")
	}
	c.Merge([]Holiday{{Date: "2026-07-10", Name: "颱風假", Note: "天然災害停止上班及上課"}})
	if c.IsTradingDay(tp(2026, 7, 10)) {
		t.Error("合併颱風假後 2026-07-10 應為休市日")
	}
	if !c.IsTradingDay(tp(2026, 7, 13)) {
		t.Error("合併不得影響其他交易日")
	}
}
