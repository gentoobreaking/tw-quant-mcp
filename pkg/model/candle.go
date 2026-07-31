package model

// Candle 是共通 K 線模型（§5.3），盤中/日線/期貨共用。
// 欄位單位依 §5.1：價格為「元」、成交量為「股」、成交值為「元」。
type Candle struct {
	Timestamp string  `json:"timestamp"`        // 盤中 "HH:MM:00"；盤後/期貨 "YYYY-MM-DD"
	Open      float64 `json:"open"`             // 開（元）
	High      float64 `json:"high"`             // 高（元）
	Low       float64 `json:"low"`              // 低（元）
	Close     float64 `json:"close"`            // 收（元）
	Volume    int64   `json:"volume"`           // 成交量（股）
	Amount    int64   `json:"amount,omitempty"` // 成交值（元）；無資料時省略
}
