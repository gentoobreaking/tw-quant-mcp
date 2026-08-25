// Package model 之 TAIFEX 資料契約（規格書 §5.1 單位：口/元，日期 YYYY-MM-DD）。
// TAIFEX-API（openapi.taifex.com.tw，最新交易日）與 TAIFEX-DL
// （www.taifex.com.tw 下載頁，歷史回溯）兩路徑皆為 canonical，於 Normalize
// 階段統一為下列模型。單位換算（§5.1）：法人金額欄位官方以「千元」計，
// 一律 ×1000 → 元；成交量/未沖銷契約數即「口」不換算。
package model

// TAIFEXDataset 為 TAIFEX 資料集 ID（API 與 DL 共用同一組）。
type TAIFEXDataset string

// TAIFEX 資料集（§2 TAIFEX-API / §9.2 TAIFEX-DL）。
const (
	TAFuturesDaily   TAIFEXDataset = "futures_daily"    // 期貨每日 OHLC
	TAOptionsDaily   TAIFEXDataset = "options_daily"    // 選擇權每日 OHLC
	TAInstiFutures   TAIFEXDataset = "insti_futures"    // 三大法人期貨部位
	TAInstiOptions   TAIFEXDataset = "insti_options"    // 三大法人選擇權部位
	TAInstiDivided   TAIFEXDataset = "insti_divided"    // 三大法人期貨與選擇權合計每日交易資訊（T126）
	TAInstiGeneral   TAIFEXDataset = "insti_general"    // 三大法人整體交易總表（CSV，T129）
	TAInstiCallsPuts TAIFEXDataset = "insti_calls_puts" // 三大法人選擇權買賣權分計明細（T134）
	TAInstiFutOptSplit TAIFEXDataset = "insti_fut_opt_split" // 三大法人期貨/選擇權分計歷史（僅 DL，T128）
	TAInstiTotal       TAIFEXDataset = "insti_total"        // 三大法人期貨+選擇權合計總表歷史（僅 DL，T130）
	TALargeTraderFut TAIFEXDataset = "large_trader_fut" // 大額交易人期貨未沖銷部位
	TALargeTraderOpt TAIFEXDataset = "large_trader_opt" // 大額交易人選擇權未沖銷部位
	TAPutCallRatio   TAIFEXDataset = "put_call_ratio"   // 買賣權比（PCR）
	TAMargin         TAIFEXDataset = "margin"           // 保證金（僅 API）
	TAFAnnualVolume  TAIFEXDataset = "annual_volume"    // 年成交量統計（僅 API，T041）
	TAFMonthlyStats  TAIFEXDataset = "monthly_stats_futures" // 期貨各類交易人月統計（僅 API，T148）
)

// FuturesDailyRow 為單一期貨契約之日交易行情。
type FuturesDailyRow struct {
	Date          string  `json:"date"`                   // YYYY-MM-DD
	Contract      string  `json:"contract"`               // 契約代碼（TX、MXF…）
	ContractMonth string  `json:"contract_month"`         // 到期月份(週別)（202608 / 202608W1）
	Session       string  `json:"session"`                // 交易時段（一般 / 盤後）
	Open          float64 `json:"open"`                   // 開盤價（點）
	High          float64 `json:"high"`                   // 最高價（點）
	Low           float64 `json:"low"`                    // 最低價（點）
	Close         float64 `json:"close"`                  // 收盤價（點；API 欄名 Last）
	Change        float64 `json:"change"`                 // 漲跌價（點）
	ChangePct     float64 `json:"change_pct"`             // 漲跌%（如 2.84）
	Volume        int64   `json:"volume"`                 // 成交量（口）
	Settlement    float64 `json:"settlement"`             // 結算價（點）
	OpenInterest  int64   `json:"open_interest"`          // 未沖銷契約數（口）
	BestBid       float64 `json:"best_bid,omitempty"`     // 最後最佳買價
	BestAsk       float64 `json:"best_ask,omitempty"`     // 最後最佳賣價
	TradingHalt   bool    `json:"trading_halt,omitempty"` // 是否因訊息面暫停交易
}

// OptionsDailyRow 為單一選擇權序列之日交易行情。
type OptionsDailyRow struct {
	Date          string  `json:"date"`           // YYYY-MM-DD
	Contract      string  `json:"contract"`       // 契約代碼（TXO…）
	ContractMonth string  `json:"contract_month"` // 到期月份(週別)（202608 / 202607W5）
	Strike        float64 `json:"strike"`         // 履約價（點）
	CallPut       string  `json:"call_put"`       // 買權 / 賣權
	Session       string  `json:"session"`        // 交易時段
	Open          float64 `json:"open"`
	High          float64 `json:"high"`
	Low           float64 `json:"low"`
	Close         float64 `json:"close"`         // 收盤價
	Volume        int64   `json:"volume"`        // 成交量（口）
	Settlement    float64 `json:"settlement"`    // 結算價
	OpenInterest  int64   `json:"open_interest"` // 未沖銷契約數（口）
	BestBid       float64 `json:"best_bid,omitempty"`
	BestAsk       float64 `json:"best_ask,omitempty"`
	Change        float64 `json:"change,omitempty"`      // 漲跌價（點）
	ChangePct     float64 `json:"change_pct,omitempty"`  // 漲跌%
	ExpiryDate    string  `json:"expiry_date,omitempty"` // 契約到期日（YYYY-MM-DD，DL 提供）
}

// InstitutionalRow 為單一商品之單一法人類別交易/未平倉部位（期貨與選擇權共用）。
// 金額欄位單位皆為「元」（千元 ×1000）。
type InstitutionalRow struct {
	Date         string `json:"date"`           // YYYY-MM-DD
	Contract     string `json:"contract"`       // 商品名稱（臺股期貨 / 臺指選擇權…）
	Investor     string `json:"investor"`       // 身份別（自營商 / 投信 / 外資及陸資…）
	LongVolume   int64  `json:"long_volume"`    // 多方交易口數
	LongValue    int64  `json:"long_value"`     // 多方交易契約金額（元）
	ShortVolume  int64  `json:"short_volume"`   // 空方交易口數
	ShortValue   int64  `json:"short_value"`    // 空方交易契約金額（元）
	NetVolume    int64  `json:"net_volume"`     // 多空交易口數淨額
	NetValue     int64  `json:"net_value"`      // 多空交易契約金額淨額（元）
	OILong       int64  `json:"oi_long"`        // 多方未平倉口數
	OILongValue  int64  `json:"oi_long_value"`  // 多方未平倉契約金額（元）
	OIShort      int64  `json:"oi_short"`       // 空方未平倉口數
	OIShortValue int64  `json:"oi_short_value"` // 空方未平倉契約金額（元）
	OINet        int64  `json:"oi_net"`         // 多空未平倉口數淨額
	OINetValue   int64  `json:"oi_net_value"`   // 多空未平倉契約金額淨額（元）
}

// InstiSplitRow 為三大法人期貨/選擇權分計之單日單身份別列（僅 DL，T128）。
// 期貨與選擇權並列，金額單位千元。
type InstiSplitRow struct {
	Date          string  `json:"date"`            // YYYY-MM-DD
	Investor      string  `json:"investor"`        // 身份別（自營商 / 投信 / 外資及陸資）
	FutLongVol    int64   `json:"fut_long_vol"`    // 期貨多方交易口數
	OptLongVol    int64   `json:"opt_long_vol"`    // 選擇權多方交易口數
	FutLongValue  float64 `json:"fut_long_value"`  // 期貨多方交易契約金額（千元）
	OptLongValue  float64 `json:"opt_long_value"`  // 選擇權多方交易契約金額（千元）
	FutShortVol   int64   `json:"fut_short_vol"`   // 期貨空方交易口數
	OptShortVol   int64   `json:"opt_short_vol"`   // 選擇權空方交易口數
	FutShortValue float64 `json:"fut_short_value"` // 期貨空方交易契約金額（千元）
	OptShortValue float64 `json:"opt_short_value"` // 選擇權空方交易契約金額（千元）
	FutNetVol     int64   `json:"fut_net_vol"`     // 期貨多空交易口數淨額
	OptNetVol     int64   `json:"opt_net_vol"`     // 選擇權多空交易口數淨額
	FutNetValue   float64 `json:"fut_net_value"`   // 期貨多空交易契約金額淨額（千元）
	OptNetValue   float64 `json:"opt_net_value"`   // 選擇權多空交易契約金額淨額（千元）
	FutOILong     int64   `json:"fut_oi_long"`     // 期貨多方未平倉口數
	OptOILong     int64   `json:"opt_oi_long"`     // 選擇權多方未平倉口數
	FutOILongVal  float64 `json:"fut_oi_long_val"` // 期貨多方未平倉契約金額（千元）
	OptOILongVal  float64 `json:"opt_oi_long_val"` // 選擇權多方未平倉契約金額（千元）
	FutOIShort    int64   `json:"fut_oi_short"`    // 期貨空方未平倉口數
	OptOIShort    int64   `json:"opt_oi_short"`    // 選擇權空方未平倉口數
	FutOIShortVal float64 `json:"fut_oi_short_val"` // 期貨空方未平倉契約金額（千元）
	OptOIShortVal float64 `json:"opt_oi_short_val"` // 選擇權空方未平倉契約金額（千元）
	FutOINet      int64   `json:"fut_oi_net"`      // 期貨多空未平倉口數淨額
	OptOINet      int64   `json:"opt_oi_net"`      // 選擇權多空未平倉口數淨額
	FutOINetVal   float64 `json:"fut_oi_net_val"`  // 期貨多空未平倉契約金額淨額（千元）
	OptOINetVal   float64 `json:"opt_oi_net_val"`  // 選擇權多空未平倉契約金額淨額（千元）
}

// InstiGeneralRow 為三大法人整體交易總表（期貨+選擇權合計，T129）。
// 金額單位百萬元。
type InstiGeneralRow struct {
	Date         string  `json:"date"`          // YYYY-MM-DD
	Investor     string  `json:"investor"`      // 身份別
	LongVolume   int64   `json:"long_volume"`   // 多方交易口數
	LongValue    float64 `json:"long_value"`    // 多方交易契約金額（百萬元）
	ShortVolume  int64   `json:"short_volume"`  // 空方交易口數
	ShortValue   float64 `json:"short_value"`   // 空方交易契約金額（百萬元）
	NetVolume    int64   `json:"net_volume"`    // 多空交易口數淨額
	NetValue     float64 `json:"net_value"`     // 多空交易契約金額淨額（百萬元）
	OILong       int64   `json:"oi_long"`       // 多方未平倉口數
	OILongValue  float64 `json:"oi_long_value"` // 多方未平倉契約金額（百萬元）
	OIShort      int64   `json:"oi_short"`      // 空方未平倉口數
	OIShortValue float64 `json:"oi_short_value"` // 空方未平倉契約金額（百萬元）
	OINet        int64   `json:"oi_net"`        // 多空未平倉口數淨額
	OINetValue   float64 `json:"oi_net_value"`  // 多空未平倉契約金額淨額（百萬元）
}

// LargeTraderRow 為單一商品/月份/交易人類別之大額交易人未沖銷部位。
// TraderType 為官方交易人類別代碼（期貨：0=期貨自營、1=期貨經紀、2=期貨交易；
// 選擇權同）。單位皆為口。
type LargeTraderRow struct {
	Date          string `json:"date"`               // YYYY-MM-DD
	Contract      string `json:"contract"`           // 契約代碼（BRF、CA…）
	ContractName  string `json:"contract_name"`      // 商品名稱（布蘭特原油期貨、南亞…）
	ContractMonth string `json:"contract_month"`     // 到期月份(週別)（202609 / 666666 彙總）
	CallPut       string `json:"call_put,omitempty"` // 買權 / 賣權（選擇權）
	TraderType    string `json:"trader_type"`        // 交易人類別
	Top5Long      int64  `json:"top5_long"`          // 前五大交易人買方
	Top5Short     int64  `json:"top5_short"`         // 前五大交易人賣方
	Top10Long     int64  `json:"top10_long"`         // 前十大交易人買方
	Top10Short    int64  `json:"top10_short"`        // 前十大交易人賣方
	MarketOI      int64  `json:"market_oi"`          // 全市場未沖銷部位數
}

// PCRow 為單一交易日之 Put/Call Ratio。
type PCRow struct {
	Date        string  `json:"date"`         // YYYY-MM-DD
	CallVolume  int64   `json:"call_volume"`  // 買權成交量（口）
	PutVolume   int64   `json:"put_volume"`   // 賣權成交量（口）
	VolumeRatio float64 `json:"volume_ratio"` // 買賣權成交量比率（%，如 100.21）
	CallOI      int64   `json:"call_oi"`      // 買權未平倉量（口）
	PutOI       int64   `json:"put_oi"`       // 賣權未平倉量（口）
	OIRatio     float64 `json:"oi_ratio"`     // 買賣權未平倉量比率（%）
}

// MarginRow 為單一契約之保證金（元/口）。
type MarginRow struct {
	Date              string `json:"date"`               // YYYY-MM-DD
	Contract          string `json:"contract"`           // 商品名稱（臺股期貨、小型臺指…）
	ClearingMargin    int64  `json:"clearing_margin"`    // 結算保證金（元）
	MaintenanceMargin int64  `json:"maintenance_margin"` // 維持保證金（元）
	InitialMargin     int64  `json:"initial_margin"`     // 原始保證金（元）
}
