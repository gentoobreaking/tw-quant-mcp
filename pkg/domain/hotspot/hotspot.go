// Package hotspot 實作「熱門概念股/漲跌族群」情境（v2.1 §9.3）。
// 對應工具：get_hot_industries_top_gainers / get_trending_stocks 等
// 概念股族群與漲跌幅熱門排行。
// T026 只建立入口骨架；實際引擎由後續 T 系列任務接線。
package hotspot

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("hotspot: 引擎尚未實作（骨架）")

// GetTrendingStocks 為「熱門/當沖當紅個股」入口（§9.3）。
// 骨架：待盤中行情資料源接線後實作。
func GetTrendingStocks() error {
	return ErrNotImplemented
}

// GetTopGainersByIndustry 為「各族群領漲/領跌股」入口（§9.3）。
// 骨架：待各族群成分表接線後實作。
func GetTopGainersByIndustry() error {
	return ErrNotImplemented
}
