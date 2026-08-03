// Package foreign 實作「外資持股」情境（v2.1 §9.2）。
// 對應工具：get_foreign_holdings（AVAILABLE）/ get_foreign_shareholding_history /
// get_foreign_industry_holdings / get_stock_holdings_by_broker。
// T026 只建立入口骨架；實際引擎由後續 T 系列任務接線。
package foreign

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("foreign: 引擎尚未實作（骨架）")

// GetForeignHoldings 為「外資持股」業務入口（§9.2）。
// 骨架：待 TWSE/FINM 資料源接線後實作。
func GetForeignHoldings(symbol string) error {
	return ErrNotImplemented
}

// GetForeignShareholdingHistory 為「外資持股變動歷史」入口（§9.2）。
// 骨架：待接線後實作。
func GetForeignShareholdingHistory(symbol string) error {
	return ErrNotImplemented
}
