// Package trend 實作「趨勢與股價歷史」情境（v2.1 §9.1）。
// 對應工具：get_stock_daily_kline / get_stock_daily_quote /
// get_stock_trend_composite（§9 現列為 PREVIEW）。
// T026 只建立入口骨架；實際引擎由後續 T 系列任務接線。
package trend

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("trend: 引擎尚未實作（骨架）")

// GetTrendComposite 為綜合趨勢分數之業務入口（§9.1，規格現列 PREVIEW）。
// 骨架：待 K 線/報價資料源接線後實作。
func GetTrendComposite(symbol string) error {
	return ErrNotImplemented
}
