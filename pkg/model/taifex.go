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
	TAFuturesDaily     TAIFEXDataset = "futures_daily"         // 期貨每日 OHLC
	TAOptionsDaily     TAIFEXDataset = "options_daily"         // 選擇權每日 OHLC
	TAInstiFutures     TAIFEXDataset = "insti_futures"         // 三大法人期貨部位
	TAInstiOptions     TAIFEXDataset = "insti_options"         // 三大法人選擇權部位
	TAInstiDivided     TAIFEXDataset = "insti_divided"         // 三大法人期貨與選擇權合計每日交易資訊（T126）
	TAInstiGeneral     TAIFEXDataset = "insti_general"         // 三大法人整體交易總表（CSV，T129）
	TAInstiCallsPuts   TAIFEXDataset = "insti_calls_puts"      // 三大法人選擇權買賣權分計明細（T134）
	TAInstiFutOptSplit TAIFEXDataset = "insti_fut_opt_split"   // 三大法人期貨/選擇權分計歷史（僅 DL，T128）
	TAInstiTotal       TAIFEXDataset = "insti_total"           // 三大法人期貨+選擇權合計總表歷史（僅 DL，T130）
	TAOptionsDelta     TAIFEXDataset = "options_delta"         // 選擇權每日 Delta（僅 API，T151）
	TAOIChange         TAIFEXDataset = "oi_change"             // 台指選擇權未平倉量增減（僅 API，T154）
	TAOptInstiByCont   TAIFEXDataset = "opt_insti_by_contract" // 三大法人各選擇權契約交易歷史（僅 DL，T152）
	TAInstiCPHist      TAIFEXDataset = "insti_cp_hist"         // 三大法人選擇權買賣權分計歷史（僅 DL，T153）
	TAStockMargin      TAIFEXDataset = "stock_margin"          // 股票期貨保證金一覽表（僅 API，T167）
	TATickFutures      TAIFEXDataset = "tick_futures"          // 期貨逐筆成交（僅 API，T207）
	TATickOptions      TAIFEXDataset = "tick_options"          // 選擇權逐筆成交（僅 API，T207）
	TAInstiGenWeek     TAIFEXDataset = "insti_general_week"    // 三大法人總表-依週別（僅 API，T204）
	TAInstiDivWeek     TAIFEXDataset = "insti_divided_week"    // 三大法人區分期貨與選擇權-依週別（僅 API，T204）
	TAInstiFutContWeek TAIFEXDataset = "insti_fut_cont_week"   // 三大法人區分各期貨契約-依週別（僅 API，T204）
	TAInstiOptContWeek TAIFEXDataset = "insti_opt_cont_week"   // 三大法人區分各選擇權契約-依週別（僅 API，T204）
	TAInstiCPWeek      TAIFEXDataset = "insti_cp_week"         // 三大法人買賣權分計-依週別（僅 API，T204）
	TAFSPAll           TAIFEXDataset = "fsp_all"               // 最後結算價-全部商品（僅 API，T205）
	TAFSPFutures       TAIFEXDataset = "fsp_futures"           // 最後結算價-期貨商品（僅 API，T205）
	TAFSPIdxFut        TAIFEXDataset = "fsp_index_futures"     // 最後結算價-股價指數類期貨（僅 API，T205）
	TAFSPSSf           TAIFEXDataset = "fsp_ssf"               // 最後結算價-股票期貨（僅 API，T205）
	TAFSPIdxOpt        TAIFEXDataset = "fsp_index_options"     // 最後結算價-指數選擇權（僅 API，T205）
	TAFSPFx            TAIFEXDataset = "fsp_fx"                // 最後結算價-匯率類（僅 API，T205）
	TAFSPGold          TAIFEXDataset = "fsp_gold"              // 最後結算價-商品類（僅 API，T205）
	TAFSPIR            TAIFEXDataset = "fsp_ir"                // 最後結算價-利率類（僅 API，T205）
	TASPOptions        TAIFEXDataset = "fsp_options"           // 最後結算價-選擇權商品（僅 API，T205）
	TAFSPSSO           TAIFEXDataset = "fsp_sso"               // 最後結算價-股票選擇權（僅 API，T205）
	TASPAll            TAIFEXDataset = "sp_all"                // 到期履約交割-全部（僅 API，T206）
	TASFutures         TAIFEXDataset = "sp_futures"            // 到期履約交割-期貨商品（僅 API，T206）
	TASPIdxOpt         TAIFEXDataset = "sp_index_options"      // 到期履約交割-指數選擇權（僅 API，T206）
	TASPFx             TAIFEXDataset = "sp_fx"                 // 到期履約交割-匯率選擇權（僅 API，T206）
	TASPFxFut          TAIFEXDataset = "sp_fx_futures"         // 到期履約交割-匯率期貨（僅 API，T206）
	TASPGold           TAIFEXDataset = "sp_gold"               // 到期履約交割-商品類（僅 API，T206）
	TASPIR             TAIFEXDataset = "sp_ir"                 // 到期履約交割-利率類（僅 API，T206）
	TASPIdxFut         TAIFEXDataset = "sp_index_futures"      // 到期履約交割-指數期貨（僅 API，T206）
	TASPOpt            TAIFEXDataset = "sp_options"            // 到期履約交割-選擇權商品（僅 API，T206）
	TASPSSF            TAIFEXDataset = "sp_ssf"                // 到期履約交割-股票期貨（僅 API，T206）
	TASPSSO            TAIFEXDataset = "sp_sso"                // 到期履約交割-股票選擇權（僅 API，T206）
	TABlockTrade       TAIFEXDataset = "block_trade"           // 鉅額交易各商品成交資訊（僅 API，T208）
	TABTFutInfo        TAIFEXDataset = "bt_fut_info"           // 鉅額交易成交資訊-期貨（僅 API，T208）
	TABTOptInfo        TAIFEXDataset = "bt_opt_info"           // 鉅額交易成交資訊-選擇權（僅 API，T208）
	TABTFutSummary     TAIFEXDataset = "bt_fut_summary"        // 鉅額交易成交量統計-期貨（僅 API，T208）
	TABTOptSummary     TAIFEXDataset = "bt_opt_summary"        // 鉅額交易成交量統計-選擇權（僅 API，T208）
	TAMarginFx         TAIFEXDataset = "margin_fx"             // 保證金一覽表-匯率類（僅 API，T209）
	TAMarginIR         TAIFEXDataset = "margin_ir"             // 保證金一覽表-利率類（僅 API，T209）
	TAMarginGold       TAIFEXDataset = "margin_gold"           // 保證金一覽表-商品類（僅 API，T209）
	TAMarginETF        TAIFEXDataset = "margin_etf"            // 保證金一覽表-股票類 ETF（僅 API，T209）
	TAFCMLists         TAIFEXDataset = "fcm_lists"             // 期貨商總公司名冊（僅 API，T230）
	TAFCMBranchLists   TAIFEXDataset = "fcm_branch_lists"      // 期貨商分公司名冊（僅 API，T230）
	TAFCMNetValue      TAIFEXDataset = "fcm_net_value"         // 期貨商每股淨值明細表（僅 API，T230）
	TAFCMIncome        TAIFEXDataset = "fcm_income"            // 專營期貨商稅前累計損益彙總表（僅 API，T230）
	TAFCMAccIncome     TAIFEXDataset = "fcm_acc_income"        // 專營期貨商累計損益明細表（僅 API，T230）
	TAPosLimitEquity   TAIFEXDataset = "pos_limit_equity"      // 交易人部位限制-個股類（僅 API，T231）
	TAPosLimitNonEq    TAIFEXDataset = "pos_limit_non_equity"  // 交易人部位限制-非個股類（僅 API，T231）
	TAContractAdj      TAIFEXDataset = "contract_adj"          // 股票期貨/選擇權契約調整一覽事項（僅 API，T231）
	TASSFAdjustedInfo  TAIFEXDataset = "ssf_adjusted_info"     // 股票期貨/選擇權調整型契約資訊（僅 API，T231）
	TAFeeSchedule      TAIFEXDataset = "fee_schedule"          // 期貨及選擇權收費標準表（僅 API，T231）
	TACollStock        TAIFEXDataset = "coll_stock"            // 可抵繳標的-股票含ETF（僅 API，T232）
	TACollGovBond      TAIFEXDataset = "coll_gov_bond"         // 可抵繳標的-公債（僅 API，T232）
	TACollIntlBond     TAIFEXDataset = "coll_intl_bond"        // 可抵繳標的-國際債（僅 API，T232）
	TACollLogStock     TAIFEXDataset = "coll_log_stock"        // 可抵繳標的增刪紀錄（僅 API，T232）
	TAStockOptOID      TAIFEXDataset = "stock_opt_oi_delta"    // 每日個股選擇權未平倉量增減（僅 API，T210）
	TAStockFutStatsD   TAIFEXDataset = "stock_fut_stats_d"     // 每日個股期貨交易量統計（僅 API，T210）
	TAStockFutStatsM   TAIFEXDataset = "stock_fut_stats_m"     // 每月個股期貨交易量統計（僅 API，T210）
	TAStockFutStatsY   TAIFEXDataset = "stock_fut_stats_y"     // 每年個股期貨交易量統計（僅 API，T210）
	TASSFLists         TAIFEXDataset = "ssf_lists"             // 股票期貨交易標的（僅 API，T211）
	TASTFTop10         TAIFEXDataset = "stf_top10"             // 每日股票期貨交易量前十大（僅 API，T211）
	TASSOLists         TAIFEXDataset = "sso_lists"             // 股票選擇權交易標的（僅 API，T211）
	TALargeTraderFut   TAIFEXDataset = "large_trader_fut"      // 大額交易人期貨未沖銷部位
	TALargeTraderOpt   TAIFEXDataset = "large_trader_opt"      // 大額交易人選擇權未沖銷部位
	TAPutCallRatio     TAIFEXDataset = "put_call_ratio"        // 買賣權比（PCR）
	TAMargin           TAIFEXDataset = "margin"                // 保證金（僅 API）
	TAFAnnualVolume    TAIFEXDataset = "annual_volume"         // 年成交量統計（僅 API，T041）
	TAFMonthlyStats    TAIFEXDataset = "monthly_stats_futures" // 期貨各類交易人月統計（僅 API，T148）
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
	Date          string  `json:"date"`             // YYYY-MM-DD
	Investor      string  `json:"investor"`         // 身份別（自營商 / 投信 / 外資及陸資）
	FutLongVol    int64   `json:"fut_long_vol"`     // 期貨多方交易口數
	OptLongVol    int64   `json:"opt_long_vol"`     // 選擇權多方交易口數
	FutLongValue  float64 `json:"fut_long_value"`   // 期貨多方交易契約金額（千元）
	OptLongValue  float64 `json:"opt_long_value"`   // 選擇權多方交易契約金額（千元）
	FutShortVol   int64   `json:"fut_short_vol"`    // 期貨空方交易口數
	OptShortVol   int64   `json:"opt_short_vol"`    // 選擇權空方交易口數
	FutShortValue float64 `json:"fut_short_value"`  // 期貨空方交易契約金額（千元）
	OptShortValue float64 `json:"opt_short_value"`  // 選擇權空方交易契約金額（千元）
	FutNetVol     int64   `json:"fut_net_vol"`      // 期貨多空交易口數淨額
	OptNetVol     int64   `json:"opt_net_vol"`      // 選擇權多空交易口數淨額
	FutNetValue   float64 `json:"fut_net_value"`    // 期貨多空交易契約金額淨額（千元）
	OptNetValue   float64 `json:"opt_net_value"`    // 選擇權多空交易契約金額淨額（千元）
	FutOILong     int64   `json:"fut_oi_long"`      // 期貨多方未平倉口數
	OptOILong     int64   `json:"opt_oi_long"`      // 選擇權多方未平倉口數
	FutOILongVal  float64 `json:"fut_oi_long_val"`  // 期貨多方未平倉契約金額（千元）
	OptOILongVal  float64 `json:"opt_oi_long_val"`  // 選擇權多方未平倉契約金額（千元）
	FutOIShort    int64   `json:"fut_oi_short"`     // 期貨空方未平倉口數
	OptOIShort    int64   `json:"opt_oi_short"`     // 選擇權空方未平倉口數
	FutOIShortVal float64 `json:"fut_oi_short_val"` // 期貨空方未平倉契約金額（千元）
	OptOIShortVal float64 `json:"opt_oi_short_val"` // 選擇權空方未平倉契約金額（千元）
	FutOINet      int64   `json:"fut_oi_net"`       // 期貨多空未平倉口數淨額
	OptOINet      int64   `json:"opt_oi_net"`       // 選擇權多空未平倉口數淨額
	FutOINetVal   float64 `json:"fut_oi_net_val"`   // 期貨多空未平倉契約金額淨額（千元）
	OptOINetVal   float64 `json:"opt_oi_net_val"`   // 選擇權多空未平倉契約金額淨額（千元）
}

// InstiGeneralRow 為三大法人整體交易總表（期貨+選擇權合計，T129）。
// 金額單位百萬元。
type InstiGeneralRow struct {
	Date         string  `json:"date"`           // YYYY-MM-DD
	Investor     string  `json:"investor"`       // 身份別
	LongVolume   int64   `json:"long_volume"`    // 多方交易口數
	LongValue    float64 `json:"long_value"`     // 多方交易契約金額（百萬元）
	ShortVolume  int64   `json:"short_volume"`   // 空方交易口數
	ShortValue   float64 `json:"short_value"`    // 空方交易契約金額（百萬元）
	NetVolume    int64   `json:"net_volume"`     // 多空交易口數淨額
	NetValue     float64 `json:"net_value"`      // 多空交易契約金額淨額（百萬元）
	OILong       int64   `json:"oi_long"`        // 多方未平倉口數
	OILongValue  float64 `json:"oi_long_value"`  // 多方未平倉契約金額（百萬元）
	OIShort      int64   `json:"oi_short"`       // 空方未平倉口數
	OIShortValue float64 `json:"oi_short_value"` // 空方未平倉契約金額（百萬元）
	OINet        int64   `json:"oi_net"`         // 多空未平倉口數淨額
	OINetValue   float64 `json:"oi_net_value"`   // 多空未平倉契約金額淨額（百萬元）
}

// InstiCPRow 為三大法人選擇權買賣權（CALL/PUT）分計歷史之單列（T153）。
// 金額單位千元。
type InstiCPRow struct {
	Date        string  `json:"date"`          // YYYY-MM-DD
	Contract    string  `json:"contract"`      // 商品名稱
	CallPut     string  `json:"call_put"`      // CALL / PUT
	Investor    string  `json:"investor"`      // 身份別
	BuyVolume   int64   `json:"buy_volume"`    // 買方交易口數
	BuyValue    float64 `json:"buy_value"`     // 買方交易契約金額（千元）
	SellVolume  int64   `json:"sell_volume"`   // 賣方交易口數
	SellValue   float64 `json:"sell_value"`    // 賣方交易契約金額（千元）
	NetVolume   int64   `json:"net_volume"`    // 交易口數買賣淨額
	NetValue    float64 `json:"net_value"`     // 交易契約金額買賣淨額（千元）
	OIBuy       int64   `json:"oi_buy"`        // 買方未平倉口數
	OIBuyValue  float64 `json:"oi_buy_value"`  // 買方未平倉契約金額（千元）
	OISell      int64   `json:"oi_sell"`       // 賣方未平倉口數
	OISellValue float64 `json:"oi_sell_value"` // 賣方未平倉契約金額（千元）
	OINetBuy    int64   `json:"oi_net_buy"`    // 未平倉口數買賣淨額
	OINetValue  float64 `json:"oi_net_value"`  // 未平倉契約金額買賣淨額（千元）
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
