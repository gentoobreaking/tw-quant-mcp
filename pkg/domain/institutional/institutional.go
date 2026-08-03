// Package institutional 實作「法人進出」情境（v2.1 §9.7）。
// 對應工具：get_institutional_investors（三大法人）/ 本外資買賣超、
// get_institutional_futures_history / get_institutional_options_positions（法人期權回頭）。
// T026 只建立骨架；實際引擎由後續 T 系列任務接線。
package institutional

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（T026 骨架）。
var ErrNotImplemented = errors.New("institutional: 引擎尚未實作（骨架）")

// GetInstitutionalInvestors 為三大法人買賣超（含本外資）入口（§9.7）。
// 骨架：待證交所三大法人資料源接線後實作。
func GetInstitutionalInvestors(symbol string) error {
	return ErrNotImplemented
}

// GetInstitutionalFuturesHistory 為法人期貨回顧圖入口（§9.7/§9.10）。
// 骨架：待接線後實作。
func GetInstitutionalFuturesHistory(symbol string) error {
	return ErrNotImplemented
}
