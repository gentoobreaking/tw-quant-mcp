package model

// de.go 定義 D/E 組工具（T014）之歸一化模型（§10.D/E、§5）。
// 估值與股利欄位單位：元/股、%（§5.1）。

// ValuationRatios 為 get_valuation_ratios（§10.D）之 data。
// ROE 為年化估計值（官方無直接 ROE 端點，見 roe_method）。
type ValuationRatios struct {
	Symbol           string  `json:"symbol"`
	Name             string  `json:"name"`
	Market           string  `json:"market"`
	Date             string  `json:"date"`               // 估值資料日期 YYYY-MM-DD
	PE               float64 `json:"pe"`                 // 本益比（虧損為 0）
	PEAvailable      bool    `json:"pe_available"`       // 本益比是否適用（虧損/不適用 false）
	PB               float64 `json:"pb"`                 // 股價淨值比
	DividendYield    float64 `json:"dividend_yield_pct"` // 現金殖利率（%）
	ROE              float64 `json:"roe_pct"`            // ROE（%）
	ROEMethod        string  `json:"roe_method"`         // ROE 計算方式說明
	DividendPerShare float64 `json:"dividend_per_share"` // 每股現金股利（元/股，最新年度）
	Note             string  `json:"note,omitempty"`
}

// DividendYear 為單一年度之股利分派決議（t187ap45_L）。
type DividendYear struct {
	DividendYear  string  `json:"dividend_year"`  // 股利年度（官方民國年）
	Progress      string  `json:"progress"`       // 決議（擬議）進度
	CashDividend  float64 `json:"cash_dividend"`  // 每股現金股利合計（盈餘+公積+法定，元/股）
	StockDividend float64 `json:"stock_dividend"` // 每股股票股利合計（元/股）
	CashTotal     int64   `json:"cash_total"`     // 現金股利總金額（元）
	NetIncome     int64   `json:"net_income"`     // 本期淨利（元）
	Retained      int64   `json:"retained"`       // 可分配盈餘（元）
}

// DividendHistory 為 get_dividend_history（§10.E）之 data。
type DividendHistory struct {
	Symbol           string         `json:"symbol"`
	Name             string         `json:"name"`
	Market           string         `json:"market"`
	Years            []DividendYear `json:"years"`
	TotalYears       int            `json:"total_years"`       // 官方提供之股利年度數
	ConsecutiveYears int            `json:"consecutive_years"` // 連續配發現金股利年數
	AvgCashDividend  float64        `json:"avg_cash_dividend"` // 平均每股現金股利（元/股）
	LastYield        float64        `json:"last_yield_pct"`    // 最新殖利率（%，估值資料）
	Note             string         `json:"note,omitempty"`
}

// ExDivEvent 為單一除權除息事件（TWT48U_ALL / TPEx 除權除息）。
type ExDivEvent struct {
	Date          string  `json:"date"` // 除權息日 YYYY-MM-DD
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Market        string  `json:"market"`
	Kind          string  `json:"kind"`           // 息 / 權 / 權息
	CashDividend  float64 `json:"cash_dividend"`  // 現金股利（元/股）
	StockDividend float64 `json:"stock_dividend"` // 股票股利（元/股）
}

// ExDivCalendar 為 get_exdividend_calendar（§10.E）之 data。
type ExDivCalendar struct {
	RangeStart string       `json:"range_start"` // YYYY-MM-DD
	RangeEnd   string       `json:"range_end"`   // YYYY-MM-DD
	Events     []ExDivEvent `json:"events"`
}

// ESGTopic 為單一 ESG 題材之揭露資料（t187ap46_L_<topic>）。
type ESGTopic struct {
	Topic      string            `json:"topic"`       // 題材名（如 溫室氣體排放）
	Year       string            `json:"year"`        // 報告年度
	ReportDate string            `json:"report_date"` // 出表日期 YYYY-MM-DD
	Fields     map[string]string `json:"fields"`      // 該題材之欄位（原值）
}

// ESGReport 為 get_esg_report（§10.D）之 data。
type ESGReport struct {
	Symbol string     `json:"symbol"`
	Name   string     `json:"name"`
	Market string     `json:"market"`
	Topics []ESGTopic `json:"topics"`
}

// ScreenStock 為 screen_stocks / screen_high_yield（§10.D/E）篩選結果列。
type ScreenStock struct {
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	Market        string   `json:"market"`
	PE            float64  `json:"pe"`
	PEAvailable   bool     `json:"pe_available"`
	PB            float64  `json:"pb"`
	DividendYield float64  `json:"dividend_yield_pct"`
	DividendShare float64  `json:"dividend_per_share"` // 每股現金股利（元/股，高殖利率篩選）
	RevenueGrowth float64  `json:"revenue_growth_pct"` // 月營收 YoY（最近月份，%）
	Matched       []string `json:"matched"`            // 命中之條件說明
}

// ScreenResult 為篩選工具（§10.D/E）之 data。
type ScreenResult struct {
	Total   int           `json:"total"`   // 全市場候選數
	Matched int           `json:"matched"` // 命中數
	Limit   int           `json:"limit"`   // 回傳上限
	Rows    []ScreenStock `json:"rows"`
	Note    string        `json:"note,omitempty"`
}
