// Package dividend 實作「股利殖利率」情境（v2.1 §9.4）。
// 對應工具：get_dividend_history / get_exdividend_calendar。
// 高殖利率候選（screen_high_dividend_yield）之規則引擎屬
// pkg/domain/screener（§9.5），本包負責股利本金資料。
// T026 只建立骨架；實際引擎由後續 T 系列任務接線。
package dividend

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("dividend: 引擎尚未實作（骨架）")

// GetDividendHistory 為個股歷年股利與除息資訊入口（§9.4）。
// 骨架：待 TWSE 現金殖利率資料源接線後實作。
func GetDividendHistory(symbol string) error {
	return ErrNotImplemented
}

// GetExDividendCalendar 為除權息行事曆入口（§9.4）。
// 骨架：待接線後實作。
func GetExDividendCalendar() error {
	return ErrNotImplemented
}
