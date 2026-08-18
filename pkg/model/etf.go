// ETF 資料契約（spec §30.1 ETF Data Adapter L1 部分實作）。
//
// 資料源：TWSE ETF e添富平台（www.twse.com.tw/zh/ETFortune）之
// ajaxEtfInfoChart（type=fundPric / type=close）。
//   - fundPric：歷史淨值（netPrice）與折溢價率（atmps）
//   - close：歷史市價
//
// 此端點為官方網域提供之未列入 OpenAPI 文件目錄之網頁端點，
// lineage source_role 採用 SEMI_OFFICIAL_REALTIME（與 MIS 同級）。
package model

// ETFNavPoint 為單日 ETF 淨值/折溢價資料點。
type ETFNavPoint struct {
	Date   string  `json:"date"`   // 交易日 YYYY-MM-DD
	NAV    float64 `json:"nav"`    // 每單位淨值（元）
	Market float64 `json:"market"` // 市價（元，收盤）
	// PremiumDiscount 為折溢價率（%），正為溢價、負為折價；
	// 官方 atmps 欄位即為（市價-淨值）/淨值*100。
	PremiumDiscount float64 `json:"premium_discount"`
}

// ETFNavResult 對應 get_etf_nav 之 data（§30.1 L1 歷史 NAV/折溢價）。
type ETFNavResult struct {
	Symbol string        `json:"symbol"` // 代碼 "0050"
	Name   string        `json:"name"`
	Market string        `json:"market"` // tse | otc
	Start  string        `json:"start"`  // 查詢起始日 YYYY-MM-DD
	End    string        `json:"end"`    // 查詢迄日 YYYY-MM-DD
	Points []ETFNavPoint `json:"points"` // 依日期由近至遠
	Note   string        `json:"note,omitempty"`
}
