package model

// bc.go 定義 §10.B/C（盤後行情・籌碼・風險）工具之輸出契約。
// 單檔明細類輸出直接使用 provider 之 Normalized Row（欄位即契約）；
// 複合/彙總/指標類輸出定義於本檔。

// MACDPoint 為 MACD 於單一期之三值（§10.B helper）。
type MACDPoint struct {
	MACD   float64 `json:"macd"`   // DIF（12-26 EMA 差）
	Signal float64 `json:"signal"` // DEA（DIF 之 9 EMA）
	Hist   float64 `json:"hist"`   // MACD Histogram（DIF − DEA）
}

// DailyIndicators 為 get_stock_daily_quote 之 helper 技術指標（§10.B）。
// 全部為 helper 資料，_lineage 以 derived_from 標明父資料集（§3.2）。
type DailyIndicators struct {
	MA20  float64   `json:"ma20,omitempty"`  // 20 日均線（收盤）
	MA60  float64   `json:"ma60,omitempty"`  // 60 日均線（收盤）
	RSI14 float64   `json:"rsi14,omitempty"` // RSI(14)
	MACD  MACDPoint `json:"macd,omitempty"`  // MACD(12,26,9)
}

// DailyQuote 對應 §10.B get_stock_daily_quote 之 data：
// 單日報價 + helper 技術指標。
type DailyQuote struct {
	Symbol     string          `json:"symbol"` // 代碼 "2330"
	Name       string          `json:"name"`
	Market     string          `json:"market"`           // tse | otc
	Date       string          `json:"date"`             // YYYY-MM-DD
	Open       float64         `json:"open"`             // 開盤價（元）
	High       float64         `json:"high"`             // 最高價（元）
	Low        float64         `json:"low"`              // 最低價（元）
	Close      float64         `json:"close"`            // 收盤價（元）
	Change     float64         `json:"change,omitempty"` // 漲跌（元）
	Volume     int64           `json:"volume"`           // 成交股數（股）
	Amount     int64           `json:"amount,omitempty"` // 成交金額（元）
	Indicators DailyIndicators `json:"indicators"`       // helper 指標（不足窗口為 0）
	Note       string          `json:"note,omitempty"`   // 上櫃無歷史指標等說明
}

// MarketStats 為單一市場之收盤彙總（§10.B get_market_summary）。
type MarketStats struct {
	Advancers   int   `json:"advancers"`    // 上漲家數
	Decliners   int   `json:"decliners"`    // 下跌家數
	Unchanged   int   `json:"unchanged"`    // 平盤家數
	LimitUp     int   `json:"limit_up"`     // 漲停家數
	LimitDown   int   `json:"limit_down"`   // 跌停家數
	TotalVolume int64 `json:"total_volume"` // 總成交股數（股）
	TotalAmount int64 `json:"total_amount"` // 總成交金額（元）
}

// MarketSummary 對應 get_market_summary 之 data。
type MarketSummary struct {
	Date string      `json:"date"` // YYYY-MM-DD
	TSE  MarketStats `json:"tse"`  // 上市
	OTC  MarketStats `json:"otc"`  // 上櫃
}

// InstitutionalSummary 為單一市場之三大法人買賣超（個股 + 彙總）。
type InstitutionalSummary struct {
	Market   string `json:"market"`         // tse | otc
	Date     string `json:"date"`           // YYYY-MM-DD
	Rows     any    `json:"rows"`           // []InstitutionalRow / []TPExInstitutionalRow
	TotalNet int64  `json:"total_net"`      // 市場彙總三大法人買賣超（股）
	Note     string `json:"note,omitempty"` // 15:00 前資料可能未齊（§T011）
}

// ForeignHoldingPoint 為個股一日之外資持股（get_foreign_shareholding_history）。
type ForeignHoldingPoint struct {
	Date           string  `json:"date"`            // YYYY-MM-DD
	ForeignShares  int64   `json:"foreign_shares"`  // 外資及陸資持有股數（股）
	ForeignPercent float64 `json:"foreign_percent"` // 持股比率（%）
}

// ForeignShareholdingHistory 對應 get_foreign_shareholding_history 之 data。
type ForeignShareholdingHistory struct {
	Symbol string                `json:"symbol"` // 代碼 "2330"
	Name   string                `json:"name"`
	Range  int                   `json:"range"`  // 請求交易日數
	Series []ForeignHoldingPoint `json:"series"` // 由近至遠（最新在前）
}

// AbnormalTrade 為一檔異常交易/注意股（get_abnormal_trading）。
type AbnormalTrade struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	NoticeCount int64  `json:"notice_count,omitempty"` // 累計次數（上市注意股）
	Info        string `json:"info,omitempty"`         // 異常資訊/注意交易資訊
}

// AttentionStock 為一檔注意股（get_attention_disposition_stocks）。
type AttentionStock struct {
	Code string `json:"code"`
	Name string `json:"name"`
	Info string `json:"info,omitempty"` // 注意交易資訊
}

// DispositionStock 為一檔處置股（get_attention_disposition_stocks）。
type DispositionStock struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Period  string `json:"period,omitempty"`  // 處置期間（官方原文）
	Reason  string `json:"reason,omitempty"`  // 處置條件
	Measure string `json:"measure,omitempty"` // 處置措施
	Detail  string `json:"detail,omitempty"`  // 處置內容（原文）
}

// AttentionDispositionList 對應 get_attention_disposition_stocks 之 data。
type AttentionDispositionList struct {
	Market      string             `json:"market"`         // tse | otc
	Date        string             `json:"date"`           // 名單日期 YYYY-MM-DD
	Attention   []AttentionStock   `json:"attention"`      // 注意股清單
	Disposition []DispositionStock `json:"disposition"`    // 處置股清單
	Note        string             `json:"note,omitempty"` // 上市處置股資料源說明等
}

// WarrantSummary 對應 get_warrant_activity 之 data（權證活躍度）。
type WarrantSummary struct {
	Date      string `json:"date"`       // YYYY-MM-DD
	AmountTop []any  `json:"amount_top"` // 成交金額 Top N（WarrantRow）
	VolumeTop []any  `json:"volume_top"` // 成交張數 Top N（WarrantRow）
}

// ChartMeta 標準（§11.2）之便利建構。
func BarChartMeta(title, key string, rightAxis []string) map[string]any {
	return map[string]any{
		"recommended_type": "bar",
		"x_axis":           map[string]any{"key": key, "type": "category"},
		"y_axis": map[string]any{
			"keys":       []string{"value"},
			"title":      title,
			"right_axis": rightAxis,
		},
		"annotations": []any{},
	}
}
