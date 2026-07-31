package model

import "math"

// 單位換算工具（§5.1）：TWSE/TPEx 原生單位一律於 Adapter 內換算，
// 對外欄位統一為「元」「股」「%」，此處為唯一換算真值。

// ThousandToYuan 仟元→元（TWSE 成交值原生單位）。
func ThousandToYuan(thousand int64) int64 { return thousand * 1000 }

// LotsToShares 張→股（TWSE 成交量原生單位，1 張 = 1000 股）。
func LotsToShares(lots int64) int64 { return lots * 1000 }

// RoundPrice 價格保留 2 位小數（元）。
func RoundPrice(v float64) float64 { return math.Round(v*100) / 100 }

// RatioToPercent 小數比例→百分比（0.0148 → 1.48）。
func RatioToPercent(ratio float64) float64 { return ratio * 100 }

// PercentToRatio 百分比→小數比例（1.48 → 0.0148）。
func PercentToRatio(pct float64) float64 { return pct / 100 }
