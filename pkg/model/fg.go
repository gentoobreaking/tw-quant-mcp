// Package model 之 §10.F（期貨與選擇權）與 §10.G（基礎設施）工具輸出契約
// （T015）。F 組資料行型別（FuturesDailyRow / InstitutionalRow /
// LargeTraderRow / PCRow）定義於 taifex.go；本檔僅含組合輸出型別。
package model

// LargeTraderPositions 為大額交易人未沖銷部位之組合輸出
// （§10.F get_large_trader_positions；期貨 + 選擇權合併回傳）。
// 單日查詢填 Date；範圍查詢填 RangeStart/RangeEnd（依日期排序合併）。
type LargeTraderPositions struct {
	Date       string           `json:"date,omitempty"`        // 單日查詢之日期 YYYY-MM-DD
	RangeStart string           `json:"range_start,omitempty"` // 範圍查詢起日
	RangeEnd   string           `json:"range_end,omitempty"`   // 範圍查詢迄日
	Futures    []LargeTraderRow `json:"futures"`               // 期貨未沖銷部位
	Options    []LargeTraderRow `json:"options"`               // 選擇權未沖銷部位
	Note       string           `json:"note,omitempty"`        // 缺口/深度限制註記
}

// TradingCalendar 為交易日曆輸出（§10.G get_trading_calendar）。
// TradingDays 依日期排序；Holidays 為官方休市清單（含名稱）。
type TradingCalendar struct {
	Year        int          `json:"year"`            // 西元年
	Month       int          `json:"month,omitempty"` // 月份（1..12；省略=全年）
	TradingDays []string     `json:"trading_days"`    // 交易日清單 YYYY-MM-DD
	Holidays    []HolidayRow `json:"holidays"`        // 官方休市清單
	Note        string       `json:"note,omitempty"`  // 行事曆資料版本等說明
}

// HolidayRow 為休市日之單筆記錄。
type HolidayRow struct {
	Date string `json:"date"` // YYYY-MM-DD
	Name string `json:"name"` // 節日/事件名稱
}
