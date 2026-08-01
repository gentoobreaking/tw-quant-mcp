package composite

import (
	"fmt"
	"math"

	"tw-quant-mcp/pkg/model"
)

// health.go：五面向評分引擎（§10.D get_financial_health_check，T017）。
// 評分輸入僅依賴 T014 已快取之 raw 資料；評分規則為產品核心邏輯，
// 版本化（ScoringConfig.Version 隨輸出回傳，便於回測）。

// ScoringConfig 為五面向評分之可調規則（計算規則於 config 可調）。
// 各門檻以「≥ 上限→100 分；≤ 下限→0 分；其間線性」之 piecewise 映射。
type ScoringConfig struct {
	// Version 為評分規則版本號（隨輸出回傳，scoring_version）。
	Version string `json:"version"`
	// Weights 為總分加權（五面向，總和應為 1.0）。
	Weights WeightSet `json:"weights"`

	// Profit：獲利能力門檻（%）
	GrossMarginMax     float64 `json:"gross_margin_max_pct"` // 毛利率 ≥ 此值 → 100 分
	OperatingMarginMax float64 `json:"operating_margin_max_pct"`
	NetMarginMax       float64 `json:"net_margin_max_pct"`

	// Growth：成長性門檻（%）
	RevenueGrowthMax   float64 `json:"revenue_growth_max_pct"` // 營收 YoY ≥ 此值 → 100 分
	NetIncomeGrowthMax float64 `json:"net_income_growth_max_pct"`

	// Structure：財務結構（負債比 = 負債 ÷ 資產；越低越好）
	DebtRatioMax float64 `json:"debt_ratio_max"` // 負債比 ≤ 此值 → 100 分
	DebtRatioMin float64 `json:"debt_ratio_min"` // 負債比 ≥ 此值 → 0 分
	// OperatingCFBonus 為營業現金流為正之加分（上限 100）。
	OperatingCFBonus float64 `json:"operating_cf_bonus"`

	// Dividend：配息政策
	ConsecutiveMax int `json:"consecutive_max"` // 連年配息 ≥ 此年數 → 70 分基準滿分
	// DividendRatioMax 為「有配息年度 ÷ 全部年度」之比例滿分門檻。
	DividendRatioMax float64 `json:"dividend_ratio_max"`
	// YieldBonus 為殖利率 ≥ 此值（%）之加分（上限 100）。
	YieldBonus float64 `json:"yield_bonus_pct"`

	// Governance：公司治理（ESG 揭露 / 公司治理規程規則）
	GovernanceBase float64 `json:"governance_base"` // 任一揭露之基礎分
	GovernanceESG  float64 `json:"governance_esg"`  // 具 ESG 揭露之加分
	GovernanceRule float64 `json:"governance_rule"` // 具公司治理規程之加分
}

// WeightSet 為五面向之總分權重。
type WeightSet struct {
	Profit     float64 `json:"profit"`
	Growth     float64 `json:"growth"`
	Structure  float64 `json:"structure"`
	Dividend   float64 `json:"dividend"`
	Governance float64 `json:"governance"`
}

// DefaultScoringConfig 回傳 v1 預設評分規則（config 可覆寫）。
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		Version: "v1",
		Weights: WeightSet{Profit: 0.30, Growth: 0.20, Structure: 0.20, Dividend: 0.15, Governance: 0.15},
		// 獲利能力：毛利率 40% / 營益率 20% / 純益率 15% → 100 分
		GrossMarginMax: 40, OperatingMarginMax: 20, NetMarginMax: 15,
		// 成長性：營收 YoY 20% / 淨利 YoY 25% → 100 分
		RevenueGrowthMax: 20, NetIncomeGrowthMax: 25,
		// 財務結構：負債比 ≤ 40% → 100 分；≥ 80% → 0 分；營業現金流為正 +5 分
		DebtRatioMax: 0.40, DebtRatioMin: 0.80, OperatingCFBonus: 5,
		// 配息政策：連年配息 ≥ 5 年 → 70 分基準滿分；配息年度比例 ≥ 80% → 30 分；殖利率 ≥ 5% +5 分
		ConsecutiveMax: 5, DividendRatioMax: 0.80, YieldBonus: 5,
		// 公司治理：任一揭露 50 分基礎 + ESG +25 + 治理規程 +25
		GovernanceBase: 50, GovernanceESG: 25, GovernanceRule: 25,
	}
}

// DividendYear 為配息政策評分輸入之單一年度。
type DividendYear struct {
	Year string  // 股利年度（官方）
	Cash float64 // 每股現金股利（元/股）
}

// HealthInput 為五面向評分之輸入（呼叫端以快取 raw 資料填充，T014）。
// 缺資料之面向以 0 + Available=false + Note 標記（不臆測）。
type HealthInput struct {
	Code   string
	Name   string
	Market string

	// Profit：最新一季獲利能力指標（0..3 列）
	Profit []model.ProfitabilityRatio
	// Growth：該代碼全期間損益表摘要（計算營收/淨利 YoY）
	Income []model.IncomeStatementRow
	// Structure：最新一季資產負債表與現金流量表
	Balance  *model.BalanceSheet
	CashFlow *model.CashFlowStatement
	// Dividend：官方提供之股利年度（由新至舊）與最新殖利率
	DividendYears []DividendYear
	Yield         float64
	// Governance：ESG / 公司治理揭露與否
	ESGDisclosed        bool
	GovernanceDisclosed bool
}

// ScoreDimension 為單一面向之評分結果。
type ScoreDimension struct {
	Score     float64 `json:"score"`          // 0-100（無資料 = 0）
	Available bool    `json:"available"`      // 是否具評分輸入
	Note      string  `json:"note,omitempty"` // 評分依據/缺資料說明
}

// HealthScore 為 get_financial_health_check 之 data（§10.D）。
// ScoringVersion 隨輸出回傳（便於回測）；Total 為加權總分（0-100）。
type HealthScore struct {
	Code           string         `json:"code"`
	Name           string         `json:"name"`
	Market         string         `json:"market"`
	ScoringVersion string         `json:"scoring_version"`
	DataDate       string         `json:"date,omitempty"` // 資料歸屬日期（呼叫端填）
	Profit         ScoreDimension `json:"profit"`
	Growth         ScoreDimension `json:"growth"`
	Structure      ScoreDimension `json:"structure"`
	Dividend       ScoreDimension `json:"dividend"`
	Governance     ScoreDimension `json:"governance"`
	Total          float64        `json:"total"` // 加權總分（0-100）
	Note           string         `json:"note,omitempty"`
}

// ScoreHealth 依輸入計算五面向評分（§10.D）。純記憶體計算，不呼叫 Adapter。
func ScoreHealth(in HealthInput, cfg ScoringConfig) HealthScore {
	out := HealthScore{
		Code: in.Code, Name: in.Name, Market: in.Market,
		ScoringVersion: cfg.Version,
	}
	out.Profit = scoreProfit(in, cfg)
	out.Growth = scoreGrowth(in, cfg)
	out.Structure = scoreStructure(in, cfg)
	out.Dividend = scoreDividend(in, cfg)
	out.Governance = scoreGovernance(in, cfg)
	w := cfg.Weights
	out.Total = round1(out.Profit.Score*w.Profit + out.Growth.Score*w.Growth +
		out.Structure.Score*w.Structure + out.Dividend.Score*w.Dividend + out.Governance.Score*w.Governance)
	return out
}

// scoreProfit：獲利能力（毛利率/營益率/純益率之平均）。
func scoreProfit(in HealthInput, cfg ScoringConfig) ScoreDimension {
	if len(in.Profit) == 0 {
		return ScoreDimension{Note: "無獲利能力指標（MOPS profit_ratios）"}
	}
	r := in.Profit[0]
	gross := clamp01(r.GrossMargin/cfg.GrossMarginMax) * 100
	oper := clamp01(r.OperatingMargin/cfg.OperatingMarginMax) * 100
	net := clamp01(r.NetMargin/cfg.NetMarginMax) * 100
	avg := round1((gross + oper + net) / 3)
	return ScoreDimension{
		Score:     avg,
		Available: true,
		Note: fmt.Sprintf("毛利率 %.1f%%（滿分 %g%%）、營益率 %.1f%%、純益率 %.1f%%",
			r.GrossMargin, cfg.GrossMarginMax, r.OperatingMargin, r.NetMargin),
	}
}

// scoreGrowth：成長性（營收/淨利 YoY 之平均；無去年同期不計）。
func scoreGrowth(in HealthInput, cfg ScoringConfig) ScoreDimension {
	latest, ok := latestIncome(in.Income)
	if !ok {
		return ScoreDimension{Note: "無損益表摘要（MOPS income_summary）"}
	}
	prev, ok := incomeAgo(in.Income, latest.Year, latest.Quarter)
	if !ok {
		return ScoreDimension{
			Note: fmt.Sprintf("無 %dQ%d 去年同期財報（僅 %dQ%d），成長性暫不計", latest.Year-1, latest.Quarter, latest.Year, latest.Quarter),
		}
	}
	var scores []float64
	var parts []string
	if prev.Revenue > 0 {
		revGrowth := (float64(latest.Revenue) - float64(prev.Revenue)) / float64(prev.Revenue) * 100
		scores = append(scores, clamp01(revGrowth/cfg.RevenueGrowthMax)*100)
		parts = append(parts, fmt.Sprintf("營收 YoY %+.1f%%", revGrowth))
	}
	if prev.NetIncome > 0 {
		niGrowth := (float64(latest.NetIncome) - float64(prev.NetIncome)) / float64(prev.NetIncome) * 100
		scores = append(scores, clamp01(niGrowth/cfg.NetIncomeGrowthMax)*100)
		parts = append(parts, fmt.Sprintf("淨利 YoY %+.1f%%", niGrowth))
	}
	if len(scores) == 0 {
		return ScoreDimension{Note: "去年同期無營收/淨利可比較"}
	}
	avg := 0.0
	for _, s := range scores {
		avg += s
	}
	return ScoreDimension{
		Score:     round1(avg / float64(len(scores))),
		Available: true,
		Note:      fmt.Sprintf("%s（%dQ%d 對 %dQ%d）", joinParts(parts), latest.Year, latest.Quarter, latest.Year-1, latest.Quarter),
	}
}

// scoreStructure：財務結構（負債比 + 營業現金流加分）。
func scoreStructure(in HealthInput, cfg ScoringConfig) ScoreDimension {
	if in.Balance == nil || in.Balance.TotalAssets <= 0 {
		return ScoreDimension{Note: "無資產負債表（MOPS balance_sheet）"}
	}
	ratio := float64(in.Balance.TotalLiabilities) / float64(in.Balance.TotalAssets)
	// 負債比 ≤ DebtRatioMax → 100 分；≥ DebtRatioMin → 0 分；其間線性
	score := (cfg.DebtRatioMin - ratio) / (cfg.DebtRatioMin - cfg.DebtRatioMax) * 100
	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}
	note := fmt.Sprintf("負債比 %.1f%%（門檻 %g%%–%g%%）", ratio*100, cfg.DebtRatioMax*100, cfg.DebtRatioMin*100)
	if in.CashFlow != nil && in.CashFlow.OperatingCashFlow > 0 {
		score += cfg.OperatingCFBonus
		note += "；營業現金流為正"
	}
	return ScoreDimension{Score: round1(clamp100(score)), Available: true, Note: note}
}

// scoreDividend：配息政策（連年配息 + 配息年度比例 + 殖利率加分）。
func scoreDividend(in HealthInput, cfg ScoringConfig) ScoreDimension {
	years := in.DividendYears
	if len(years) == 0 {
		return ScoreDimension{Note: "無配息資料（TWSE dividend / TPEx 估值）"}
	}
	consecutive := 0
	paying := 0
	for i, y := range years {
		if y.Cash > 0 {
			paying++
			if consecutive == i { // 由新至舊連續
				consecutive++
			}
		}
	}
	base := clamp01(float64(consecutive)/float64(cfg.ConsecutiveMax)) * 70
	payRatio := float64(paying) / float64(len(years))
	ratio := clamp01(payRatio/cfg.DividendRatioMax) * 30
	score := base + ratio
	if in.Yield >= cfg.YieldBonus {
		score += cfg.YieldBonus
	}
	note := fmt.Sprintf("連年配息 %d 年（滿分基準 %d 年）、有配息 %d/%d 年度",
		consecutive, cfg.ConsecutiveMax, paying, len(years))
	if in.Yield > 0 {
		note += fmt.Sprintf("、殖利率 %.2f%%", in.Yield)
	}
	return ScoreDimension{Score: round1(clamp100(score)), Available: true, Note: note}
}

// scoreGovernance：公司治理（ESG 揭露 + 治理規程規則）。
func scoreGovernance(in HealthInput, cfg ScoringConfig) ScoreDimension {
	if !in.ESGDisclosed && !in.GovernanceDisclosed {
		return ScoreDimension{Note: "無 ESG/公司治理揭露（TWSE esg / company_governance）"}
	}
	score := cfg.GovernanceBase
	note := "具 "
	if in.ESGDisclosed {
		score += cfg.GovernanceESG
		note += "ESG"
	}
	if in.GovernanceDisclosed {
		score += cfg.GovernanceRule
		if in.ESGDisclosed {
			note += "、"
		}
		note += "公司治理規程"
	}
	return ScoreDimension{Score: round1(clamp100(score)), Available: true, Note: note + " 揭露"}
}

// latestIncome 回傳該代碼最新（年, 季）之損益表摘要。
func latestIncome(rows []model.IncomeStatementRow) (model.IncomeStatementRow, bool) {
	var best model.IncomeStatementRow
	found := false
	for _, r := range rows {
		if !found || r.Year > best.Year || (r.Year == best.Year && r.Quarter > best.Quarter) {
			best = r
			found = true
		}
	}
	return best, found
}

// incomeAgo 回傳（year-1, quarter）同期列。
func incomeAgo(rows []model.IncomeStatementRow, year, quarter int) (model.IncomeStatementRow, bool) {
	for _, r := range rows {
		if r.Year == year-1 && r.Quarter == quarter {
			return r, true
		}
	}
	return model.IncomeStatementRow{}, false
}

// clamp01 / clamp100 / round1 為數值輔助。
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clamp100(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func joinParts(parts []string) string {
	s := ""
	for i, p := range parts {
		if i > 0 {
			s += "、"
		}
		s += p
	}
	return s
}
