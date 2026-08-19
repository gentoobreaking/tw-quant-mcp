// Package domain 定義 v2.1 §6 六大共用正規化 Schema（pkg/model/domain/）。
//
// 此為 Domain 模組（§7）與 MCP Tool Handler（§9）唯一操作之資料型別：
// 所有 Schema 共用 §4 之 model.Lineage；上游原始欄位一律不得穿透至
// 此層——轉換集中於 pkg/model/normalize（唯一「知道上游欄位」之處）。
//
// 欄位與 json tag 依 v2.1 §6 規範（含 omitempty 規則），一旦定義不可隨意變更。
package domain

import "tw-quant-mcp/pkg/model"

// StockIdentity 為所有個股相關 Schema 共用的識別資訊。
type StockIdentity struct {
	Symbol   string `json:"symbol"` // "2330"
	Name     string `json:"name"`   // "台積電"
	Market   string `json:"market"` // "TSE" | "OTC"
	Industry string `json:"industry,omitempty"`
}

// ---------- 個股趨勢研判（§9.1） ----------

// TrendComposite 為短中長期技術面、基本面、籌碼面綜合分析之聚合 Schema。
type TrendComposite struct {
	Stock       StockIdentity   `json:"stock"`
	Technical   TechnicalView   `json:"technical"`
	Fundamental FundamentalView `json:"fundamental"`
	Chip        ChipView        `json:"chip"`
	Horizon     string          `json:"horizon"` // "short" | "mid" | "long"
	Lineage     []model.Lineage `json:"_lineage"`
	ChartData   interface{}     `json:"_chart_meta,omitempty"`
}

// TechnicalView 為技術面視圖（MA/RSI/趨勢訊號）。
type TechnicalView struct {
	MA5         float64 `json:"ma5"`
	MA20        float64 `json:"ma20"`
	MA60        float64 `json:"ma60"`
	RSI14       float64 `json:"rsi_14"`
	TrendSignal string  `json:"trend_signal"` // "BULLISH" | "BEARISH" | "NEUTRAL"
}

// FundamentalView 為基本面視圖（估值/殖利率/成長）。
type FundamentalView struct {
	PE               float64 `json:"pe"`
	PB               float64 `json:"pb"`
	DividendYieldPct float64 `json:"dividend_yield_pct"`
	EPSGrowthYoYPct  float64 `json:"eps_growth_yoy_pct"`
}

// ChipView 為籌碼面視圖（法人 5 日淨買賣）。
type ChipView struct {
	ForeignNetShares5D int64 `json:"foreign_net_shares_5d"`
	TrustNetShares5D   int64 `json:"trust_net_shares_5d"`
}

// ---------- 三大法人籌碼流向 / 外資投資解讀（§9.2/§9.7） ----------

// InstitutionalFlow 為單日三大法人買賣超（股）。
type InstitutionalFlow struct {
	Stock             StockIdentity `json:"stock"`
	Date              string        `json:"date"`
	Market            string        `json:"market"` // "TSE" | "OTC"
	ForeignNetShares  int64         `json:"foreign_net_shares"`
	TrustNetShares    int64         `json:"investment_trust_net_shares"`
	DealerNetShares   int64         `json:"dealer_net_shares"`
	ForeignHoldingPct float64       `json:"foreign_holding_pct,omitempty"`
	Lineage           model.Lineage `json:"_lineage"`
}

// ---------- 股利投資規劃（§9.4） ----------

// DividendRecord 為單一股票單年度配息紀錄（含穩定度評分）。
type DividendRecord struct {
	Stock                StockIdentity `json:"stock"`
	FiscalYear           string        `json:"fiscal_year"`
	CashDividend         float64       `json:"cash_dividend"`
	StockDividend        float64       `json:"stock_dividend"`
	ExDividendDate       string        `json:"ex_dividend_date,omitempty"`
	ExRightDate          string        `json:"ex_right_date,omitempty"`
	DividendYieldPct     float64       `json:"dividend_yield_pct"`
	PayoutStabilityScore float64       `json:"payout_stability_score,omitempty"` // 0-100，近 5 年配息穩定度
	Lineage              model.Lineage `json:"_lineage"`
}

// ---------- 個股財報體檢（§9.8，五面向） ----------

// FinancialHealthReport 為五面向財報體檢之聚合 Schema。
type FinancialHealthReport struct {
	Stock              StockIdentity   `json:"stock"`
	Profitability      DimensionScore  `json:"profitability"`
	Growth             DimensionScore  `json:"growth"`
	FinancialStructure DimensionScore  `json:"financial_structure"`
	DividendPolicy     DimensionScore  `json:"dividend_policy"`
	Governance         DimensionScore  `json:"governance"`
	OverallScore       float64         `json:"overall_score"` // 五面向加權平均
	Lineage            []model.Lineage `json:"_lineage"`
}

// DimensionScore 為單一面向之評分與指標。
type DimensionScore struct {
	Score   float64            `json:"score"` // 0-100
	Metrics map[string]float64 `json:"metrics"`
}

// ---------- 買前風險掃描（§9.9） ----------

// RiskFlags 為處置/注意/當沖限制/停資停券之風險旗標。
type RiskFlags struct {
	Stock                  StockIdentity `json:"stock"`
	IsDisposition          bool          `json:"is_disposition"`           // 處置股
	IsAttention            bool          `json:"is_attention"`             // 注意股
	DayTradingRestricted   bool          `json:"day_trading_restricted"`   // 當沖限制
	MarginTradingSuspended bool          `json:"margin_trading_suspended"` // 停資
	ShortSellingSuspended  bool          `json:"short_selling_suspended"`  // 停券
	Lineage                model.Lineage `json:"_lineage"`
}

// ---------- 期貨籌碼與選擇權分析（§9.6） ----------

// DerivativesSnapshot 為期貨/選擇權籌碼之聚合 Schema。
type DerivativesSnapshot struct {
	Product              string            `json:"product"` // "TX"（台指期）等
	Date                 string            `json:"date"`
	PutCallRatio         float64           `json:"put_call_ratio"`
	LargeTraderNetOI     map[string]int64  `json:"large_trader_net_oi"` // key: 特定/一般法人分類
	InstitutionalFutures InstitutionalFlow `json:"institutional_futures"`
	Lineage              model.Lineage     `json:"_lineage"`
}

// IndexView 為 TWSE 指數（加權/寶島/台灣50 等）之盤後行情與歷史日 K。
type IndexView struct {
	Name          string          `json:"name"`                 // 指數名稱（如「發行量加權股價指數」）
	Date          string          `json:"date"`                 // 交易日 YYYY-MM-DD
	Close         float64         `json:"close"`                // 收盤指數
	Change        float64         `json:"change"`               // 漲跌點數
	ChangePercent float64         `json:"change_percent"`       // 漲跌百分比（%）
	ChangeDir     string          `json:"change_dir,omitempty"` // 漲跌(+/-)
	Note          string          `json:"note,omitempty"`       // 特殊處理註記
	History       []IndexDay      `json:"history,omitempty"`    // 歷史日 K（月份每日 OHLC）
	Lineage       model.Lineage   `json:"_lineage"`
	ChartMeta     *IndexChartMeta `json:"_chart_meta,omitempty"` // 圖表中繼資料
}

// IndexDay 為指數歷史之單日 OHLC。
type IndexDay struct {
	Date  string  `json:"date"`  // YYYY-MM-DD
	Open  float64 `json:"open"`  // 開盤指數
	High  float64 `json:"high"`  // 最高指數
	Low   float64 `json:"low"`   // 最低指數
	Close float64 `json:"close"` // 收盤指數
}

// IndexChartMeta 為指數圖表中繼資料（line 型別）。
type IndexChartMeta struct {
	Type   string   `json:"type"`   // "line"
	Series []string `json:"series"` // 欄位序列：["date", "open", "high", "low", "close"]
}
