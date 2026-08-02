package model

// KlineBar 代表單根 K 線（v2.1 §4）：盤中 1 分 K 由引擎（pkg/engine）依
// Snapshot 重採樣，或由 normalize.FromMIS 由 MIS 原始快照轉出（tick bar）。
// 欄位單位依 §5.1：價格為「元」、成交量為「股」。
type KlineBar struct {
	Timestamp string  `json:"timestamp"` // 盤中 "HH:MM:00"；tick bar 為 "HH:MM:SS"
	Open      float64 `json:"open"`      // 開（元）
	High      float64 `json:"high"`      // 高（元）
	Low       float64 `json:"low"`       // 低（元）
	Close     float64 `json:"close"`     // 收（元）
	Volume    int64   `json:"volume"`    // 成交量（股）
}
