// Package derivatives 實作「期貨/衍生品」情境（v2.1 §9.6）。
// 對應工具：get_put_call_ratio / get_large_trader_positions /
// get_institutional_options_positions / get_warrant_activity / get_futures_history /
// get_futures_daily_ohlc（§9.10 法人回溯）。
// T026 只建立骨架；實際引擎由後續 T 系列任務接線。
package derivatives

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("derivatives: 引擎尚未實作（骨架）")

// GetPutCallRatio 為 台指選擇權 P/C 比（多空訊號）入口（§9.6）。
// 骨架：待 TAIFEX 資料源接線後實作。
func GetPutCallRatio() error {
	return ErrNotImplemented
}

// GetFuturesPositions 為期貨/選擇權未平倉與法人部位入口（§9.6/§9.10）。
// 骨架：待接線後實作。
func GetFuturesPositions() error {
	return ErrNotImplemented
}
