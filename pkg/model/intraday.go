package model

// FibLevel 是 Fibonacci 回檔位（§8.5：0.382 / 0.5 / 0.618）。
type FibLevel struct {
	Ratio float64 `json:"ratio"` // 回檔比例 0.382 | 0.5 | 0.618
	Price float64 `json:"price"` // 價格（元）
}

// IntradayVWAP 對應 §10.A get_intraday_vwap 之 data：
// 當日累計 VWAP、當日高低點與 Fibonacci 支撐/壓力位（全為記憶體計算，零 HTTP）。
type IntradayVWAP struct {
	Symbol      string     `json:"symbol"`                // 代碼 "2330"
	Date        string     `json:"date"`                  // 資料日期 YYYY-MM-DD
	Time        string     `json:"time"`                  // 計算基準 "HH:MM:SS"
	VWAP        float64    `json:"vwap"`                  // Σ(p×v)/Σv（元）
	Volume      int64      `json:"volume"`                // 累計成交量（股）
	High        float64    `json:"high"`                  // 當日最高（元）
	Low         float64    `json:"low"`                   // 當日最低（元）
	Last        float64    `json:"last"`                  // 最新成交價（元）
	PrevClose   float64    `json:"prev_close"`            // 昨收（元）
	Supports    []FibLevel `json:"supports,omitempty"`    // 支撐位（價格低於 Last，由低至高）
	Resistances []FibLevel `json:"resistances,omitempty"` // 壓力位（價格高於 Last，由低至高）
}

// VolumeSurge 對應 §10.A detect_volume_surge 之 data：
// 前 20 分鐘均量滑動窗口之爆量偵測結果。
type VolumeSurge struct {
	Symbol          string  `json:"symbol"`            // 代碼 "2330"
	Date            string  `json:"date"`              // 資料日期 YYYY-MM-DD
	Time            string  `json:"time"`              // 偵測基準（最後一根 K 線）"HH:MM:00"
	Minutes         int     `json:"minutes"`           // 近 N 分鐘（請求參數）
	RecentVolume    int64   `json:"recent_volume"`     // 近 N 分鐘總量（股）
	WindowAvgVolume float64 `json:"window_avg_volume"` // 前 20 分鐘均量（股/分）
	VolumeRatio     float64 `json:"volume_ratio"`      // 近 N 分鐘均量 / 窗口均量
	SurgeType       string  `json:"surge_type"`        // BULLISH_BREAKOUT | BEARISH_BREAKDOWN | NONE
	Open            float64 `json:"open"`              // 近 N 分鐘首根開盤（元）
	Close           float64 `json:"close"`             // 最後一根收盤（元）
}
