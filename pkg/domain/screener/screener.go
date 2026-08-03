// Package screener 實作「投資標的篩選」情境（v2.1 §9.5、§10）。
// 提供 screen_dividend_yield 等高殖利率/價值篩選之業務入口，為
// §7 模組化邊界中 pkg/domain/screener 之薄層：篩選規則實作
// 下沈至 pkg/engine/composite（T017 下層引擎），本包不重複邏輯、
// 僅對齊介面並以類型別名維持與下層之型別相等。
package screener

import (
	"tw-quant-mcp/pkg/engine/composite"
)

// 以下型別別名對齊 composite 下層引擎（`=` 表示同一型別，可直接互轉）。
type (
	// ValuationMetrics 為單一候選股之估值/成長指標（篩選輸入）。
	ValuationMetrics = composite.ValuationMetrics
	// ValueCriterion 為價值股/成長股篩選條件（§10.D）。
	ValueCriterion = composite.ValueCriterion
	// HighYieldCriterion 為高殖利率篩選條件（§10.E）。
	HighYieldCriterion = composite.HighYieldCriterion
	// Match 為單一候選股之命中結果。
	Match = composite.Match
	// ScreenSort 為篩選結果排序規則。
	ScreenSort = composite.ScreenSort
)

// 排序規則常數（與 composite 同型別同值）。
const (
	SortByPE     = composite.ScreenSortPE
	SortByPB     = composite.ScreenSortPB
	SortByYield  = composite.ScreenSortYield
	SortByGrowth = composite.ScreenSortGrowth
)

// ScreenValue 篩選價值股/成長股（§10.D screen_stocks / screen_value_growth_stocks）。
// 薄層入口，直接委託下層 composite.ScreenValue。
func ScreenValue(rows []ValuationMetrics, c ValueCriterion) []Match {
	return composite.ScreenValue(rows, c)
}

// ScreenHighYield 篩選高殖利率股票（§9.4/§10.E screen_high_dividend_yield）。
// 薄層入口，直接委託下層 composite.ScreenHighYield。
func ScreenHighYield(rows []ValuationMetrics, c HighYieldCriterion) []Match {
	return composite.ScreenHighYield(rows, c)
}
