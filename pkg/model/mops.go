package model

// mops.go 定義 MOPS（公開資訊觀測站）資料之歸一化模型（§5、§10.D）。
// 所有欄位已依 §5.1 統一為元/股/%；MOPS CSV 原生單位（千元）已於 provider 換算。

// CompanyProfile 為公司基本資料（t187ap03_L）。
type CompanyProfile struct {
	TableDate     string `json:"table_date"`                 // 出表日期 YYYY-MM-DD
	Code          string `json:"code"`                       // 公司代號
	Name          string `json:"name"`                       // 公司名稱
	ShortName     string `json:"short_name"`                 // 公司簡稱
	ForeignReg    string `json:"foreign_reg,omitempty"`      // 外國企業註冊地國（缺值省略，§5.1）
	Industry      string `json:"industry"`                   // 產業別
	Address       string `json:"address"`                    // 住址
	TaxID         string `json:"tax_id"`                     // 營利事業統一編號
	Chairman      string `json:"chairman"`                   // 董事長
	President     string `json:"president"`                  // 總經理
	Spokesman     string `json:"spokesman"`                  // 發言人
	SpokesTitle   string `json:"spokesman_title"`            // 發言人職稱
	DepSpokes     string `json:"deputy_spokesman,omitempty"` // 代理發言人（缺值省略）
	Phone         string `json:"phone"`                      // 總機電話
	Established   string `json:"established"`                // 成立日期 YYYY-MM-DD
	Listed        string `json:"listed"`                     // 上市日期 YYYY-MM-DD
	ParValue      string `json:"par_value"`                  // 普通股每股面額（原文）
	Capital       int64  `json:"capital"`                    // 實收資本額（元）
	PrivateStock  int64  `json:"private_stock"`              // 私募股數（股）
	Preferred     int64  `json:"preferred_stock"`            // 特別股（股）
	FinType       string `json:"fin_report_type,omitempty"`  // 編制財務報表類型
	Transfer      string `json:"transfer_agent,omitempty"`   // 股票過戶機構
	TransferPhone string `json:"transfer_phone,omitempty"`   // 過戶電話
	TransferAddr  string `json:"transfer_address,omitempty"` // 過戶地址
	AuditorFirm   string `json:"auditor_firm,omitempty"`     // 簽證會計師事務所
	Auditor1      string `json:"auditor_1,omitempty"`        // 簽證會計師1
	Auditor2      string `json:"auditor_2,omitempty"`        // 簽證會計師2
	EngName       string `json:"english_name,omitempty"`     // 英文簡稱
	EngAddr       string `json:"english_address,omitempty"`  // 英文通訊地址
	Fax           string `json:"fax,omitempty"`              // 傳真機號碼
	Email         string `json:"email,omitempty"`            // 電子郵件信箱
	Website       string `json:"website,omitempty"`          // 網址
	SharesOut     int64  `json:"shares_outstanding"`         // 已發行普通股數（股）
}

// MajorAnnouncement 為一筆重大訊息（t187ap04_L）。
type MajorAnnouncement struct {
	TableDate    string `json:"table_date"`    // 出表日期 YYYY-MM-DD
	AnnounceDate string `json:"announce_date"` // 發言日期 YYYY-MM-DD
	AnnounceTime string `json:"announce_time"` // 發言時間 HH:MM:SS
	Code         string `json:"code"`          // 公司代號
	Name         string `json:"name"`          // 公司名稱
	Subject      string `json:"subject"`       // 主旨
	Clause       string `json:"clause"`        // 符合條款
	FactDate     string `json:"fact_date"`     // 事實發生日 YYYY-MM-DD
	Description  string `json:"description"`   // 說明
}

// MonthlyRevenueRow 為一筆月營收（t187ap05_L）。營收金額已由千元換算為元（§5.1）。
// YoY/MoM 成長率由官方 CSV 保留（已為百分比），另提供 helper 計算供對照。
type MonthlyRevenueRow struct {
	TableDate        string  `json:"table_date"`         // 出表日期 YYYY-MM-DD
	DataYearMonth    string  `json:"data_year_month"`    // 資料年月 YYYYMM
	Code             string  `json:"code"`               // 公司代號
	Name             string  `json:"name"`               // 公司名稱
	Industry         string  `json:"industry"`           // 產業別
	Revenue          int64   `json:"revenue"`            // 當月營收（元）
	LastMonthRevenue int64   `json:"last_month_revenue"` // 上月營收（元）
	LastYearRevenue  int64   `json:"last_year_revenue"`  // 去年當月營收（元）
	MoMChange        float64 `json:"mom_change_pct"`     // 上月比較增減（%）
	YoYChange        float64 `json:"yoy_change_pct"`     // 去年同月增減（%）
	CumRevenue       int64   `json:"cum_revenue"`        // 當月累計營收（元）
	CumLastYear      int64   `json:"cum_last_year"`      // 去年累計營收（元）
	CumChange        float64 `json:"cum_change_pct"`     // 累計增減（%）
	Note             string  `json:"note,omitempty"`     // 備註
}

// MonthlyRevenueBundle 為 get_monthly_revenue（§10.D）之完整 data。
type MonthlyRevenueBundle struct {
	Symbol string              `json:"symbol"`
	Name   string              `json:"name"`
	Market string              `json:"market"`
	Rows   []MonthlyRevenueRow `json:"rows"`
}

// IncomeStatementRow 為一筆簡明損益表（t187ap14_L）。
// 金額已由千元換算為元（§5.1）。
type IncomeStatementRow struct {
	TableDate         string  `json:"table_date"`          // 出表日期 YYYY-MM-DD
	Year              int     `json:"year"`                // 年度（西元）
	Quarter           int     `json:"quarter"`             // 季別（1-4）
	Code              string  `json:"code"`                // 公司代號
	Name              string  `json:"name"`                // 公司名稱
	Industry          string  `json:"industry"`            // 產業別
	EPS               float64 `json:"eps"`                 // 基本每股盈餘（元）
	ParValue          string  `json:"par_value"`           // 普通股每股面額
	Revenue           int64   `json:"revenue"`             // 營業收入（元）
	OperatingProfit   int64   `json:"operating_profit"`    // 營業利益（元）
	NonOperatingItems int64   `json:"non_operating_items"` // 營業外收入及支出（元）
	NetIncome         int64   `json:"net_income"`          // 稅後淨利（元）
}

// FinancialStatements 為 get_financial_statements（§10.D）之完整 data。
type FinancialStatements struct {
	Symbol       string               `json:"symbol"`
	Name         string               `json:"name"`
	Year         int                  `json:"year"`
	Quarter      int                  `json:"quarter"`
	Income       []IncomeStatementRow `json:"income"`
	BalanceSheet *BalanceSheet        `json:"balance_sheet,omitempty"`
	CashFlow     *CashFlowStatement   `json:"cash_flow,omitempty"`
	ProfitRatios []ProfitabilityRatio `json:"profit_ratios,omitempty"`
	Note         string               `json:"note,omitempty"`
}

// BalanceSheet 為資產負債表（合併，IFRS）。
// 金額已由 MOPS 仟元換算為元（§5.1）。
type BalanceSheet struct {
	TableDate             string `json:"table_date"`              // 出表日期
	Year                  int    `json:"year"`                    // 年度
	Quarter               int    `json:"quarter"`                 // 季別（1-4）
	TotalAssets           int64  `json:"total_assets"`            // 資產總額（百萬元）
	CurrentAssets         int64  `json:"current_assets"`          // 流動資產
	NonCurrentAssets      int64  `json:"non_current_assets"`      // 非流動資產
	TotalLiabilities      int64  `json:"total_liabilities"`       // 負債總額
	CurrentLiabilities    int64  `json:"current_liabilities"`     // 流動負債
	NonCurrentLiabilities int64  `json:"non_current_liabilities"` // 非流動負債
	TotalEquity           int64  `json:"total_equity"`            // 權益總額（股東權益）
}

// CashFlowStatement 為現金流量表（合併，IFRS）。
// 金額已由 MOPS 仟元換算為元（§5.1）。
type CashFlowStatement struct {
	TableDate         string `json:"table_date"`          // 出表日期
	Year              int    `json:"year"`                // 年度
	Quarter           int    `json:"quarter"`             // 季別（1-4）
	OperatingCashFlow int64  `json:"operating_cash_flow"` // 營業活動淨現金流入（百萬元）
	InvestingCashFlow int64  `json:"investing_cash_flow"` // 投資活動淨現金流出（百萬元）
	FinancingCashFlow int64  `json:"financing_cash_flow"` // 籌資活動淨現金流出（百萬元）
	EndingCashBalance int64  `json:"ending_cash_balance"` // 期末現金及約當現金（百萬元）
}

// ProfitabilityRatio 為一筆獲利能力指標（t187ap17_L）。
// 百分比欄位已為 %（與官方 CSV 一致，不另換算）。
type ProfitabilityRatio struct {
	TableDate       string  `json:"table_date"`           // 出表日期 YYYY-MM-DD
	Year            int     `json:"year"`                 // 年度（西元）
	Quarter         int     `json:"quarter"`              // 季別（1-4）
	Code            string  `json:"code"`                 // 公司代號
	Name            string  `json:"name"`                 // 公司名稱
	RevenueMillion  float64 `json:"revenue_million"`      // 營業收入（百萬元）
	GrossMargin     float64 `json:"gross_margin_pct"`     // 毛利率（%）
	OperatingMargin float64 `json:"operating_margin_pct"` // 營業利益率（%）
	PreTaxMargin    float64 `json:"pretax_margin_pct"`    // 稅前純益率（%）
	NetMargin       float64 `json:"net_margin_pct"`       // 稅後純益率（%）
}
