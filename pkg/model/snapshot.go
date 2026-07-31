package model

// Snapshot 是 MIS 即時報價之正規化快照（§8.3）：MIS 原生欄位
// （z/v/tv/tlong/o/h/l/y）由 pkg/provider/mis_worker.go（MIS Adapter）轉換，
// 單位依 §5.1：價格「元」、成交量「股」。
//
// MIS 原生語意（2026-07-31 實測 + 多來源交叉驗證）：
//   - v  = 當日累積成交量（張）→ CumulativeVol ×1000 為股
//   - tv = 當分鐘內累積成交量（張，每分鐘重置）→ MinuteVol ×1000 為股；
//     重採樣規則「桶末 tv − 桶初 tv」即分鐘量（§8.4）
//   - c  = 股票代號（非漲跌）；漲跌 = z − y
//   - p/q/a/w = 五檔買價/買量/賣價/賣量（張）→ Book（T010 起解析）
type Snapshot struct {
	Code          string     `json:"code"`           // 代碼 "2330"
	Exch          string     `json:"exch"`           // MIS ex_ch "tse_2330.tw"
	Time          TaipeiTime `json:"time"`           // tlong（毫秒）→ Asia/Taipei
	TradeTime     string     `json:"trade_time"`     // 最近成交時刻 "HH:MM:SS"
	Last          float64    `json:"last"`           // z 成交價（元）
	Open          float64    `json:"open"`           // o 開盤價（元）
	High          float64    `json:"high"`           // h 最高價（元）
	Low           float64    `json:"low"`            // l 最低價（元）
	PrevClose     float64    `json:"prev_close"`     // y 昨收（元）
	Change        float64    `json:"change"`         // 漲跌（元）= z − y
	MinuteVol     int64      `json:"minute_vol"`     // tv 當分鐘內累積量（股）
	CumulativeVol int64      `json:"cumulative_vol"` // v 當日累積量（股）
	Book          *LevelBook `json:"book,omitempty"` // 五檔買賣價量（p/q/a/w）
}

// LevelBook 為五檔買賣價量（MIS 原生 p/q/a/w，T010 起解析；
// 缺檔或官方未提供時為 nil/空，價格 元、數量 股）。
type LevelBook struct {
	Bids []PriceLevel `json:"bids,omitempty"` // 買價由高至低（最佳買價在前）
	Asks []PriceLevel `json:"asks,omitempty"` // 賣價由低至高（最佳賣價在前）
}
