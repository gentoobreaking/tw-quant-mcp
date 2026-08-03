// Package risk 實作「風險評估」情境（v2.1 §9.9）。
// 對應工具：get_margin_trading（資券）/ get_abnormal_trading /
// get_attention_disposition_stocks（處置/警示股）。日內風險計測
// （DaytradeScanner，§9.11 盤中即時）仍由 pkg/mcp/risk.go 提供。
// T026 只建立骨架；實際引擎由後續 T 系列任務接線。
package risk

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("risk: 引擎尚未實作（骨架）")

// GetMarginTrading 為資券餘額/信用交易指標入口（§9.9，融資餘額高低
// 常作為當沖/風險訊號）。
// 骨架：待證交所資料源接線後實作。
func GetMarginTrading(symbol string) error {
	return ErrNotImplemented
}

// GetAttentionStock 為警示/處置股風險入口（§9.9）。
// 骨架：待接線後實作。
func GetAttentionStock(symbol string) error {
	return ErrNotImplemented
}
