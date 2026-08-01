package composite

import (
	"testing"

	"tw-quant-mcp/pkg/model"
)

// health_test.go：五面向評分引擎契約測試（T017）。
// 評分規則正確性、缺資料邊界（不臆測）、config 覆寫。

// fullInput 建構資料齊全之評分輸入（類似 2330 測試情境）。
func fullInput() HealthInput {
	return HealthInput{
		Code: "2330", Name: "台積電", Market: "tse",
		Profit: []model.ProfitabilityRatio{{
			Year: 2026, Quarter: 1, GrossMargin: 59.0, OperatingMargin: 41.4, NetMargin: 40.6,
		}},
		Income: []model.IncomeStatementRow{
			{Year: 2026, Quarter: 1, Revenue: 1134103440000, NetIncome: 460000000000},
			{Year: 2025, Quarter: 1, Revenue: 1000000000000, NetIncome: 390000000000},
		},
		Balance:  &model.BalanceSheet{TotalAssets: 6600000000000, TotalLiabilities: 1500000000000},
		CashFlow: &model.CashFlowStatement{OperatingCashFlow: 350000000000},
		DividendYears: []DividendYear{
			{Year: "115", Cash: 7.0},
			{Year: "114", Cash: 6.0},
		},
		Yield:               2.1,
		ESGDisclosed:        true,
		GovernanceDisclosed: true,
	}
}

func TestScoreHealthFull(t *testing.T) {
	s := ScoreHealth(fullInput(), DefaultScoringConfig())
	if s.ScoringVersion != "v1" {
		t.Errorf("scoring_version 應為 v1，實際 %s", s.ScoringVersion)
	}
	if s.Profit.Score != 100 || !s.Profit.Available {
		t.Errorf("獲利能力應為 100，實際 %+v", s.Profit)
	}
	if s.Growth.Score != 69.4 || !s.Growth.Available {
		t.Errorf("成長性應為 69.4，實際 %+v", s.Growth)
	}
	if s.Structure.Score != 100 || !s.Structure.Available {
		t.Errorf("結構應為 100（負債比 22.7%% + 現金流加分），實際 %+v", s.Structure)
	}
	if s.Dividend.Score != 58 || !s.Dividend.Available {
		t.Errorf("配息應為 58（28+30，殖利率 2.1 無加分），實際 %+v", s.Dividend)
	}
	if s.Governance.Score != 100 || !s.Governance.Available {
		t.Errorf("治理應為 100（50+25+25），實際 %+v", s.Governance)
	}
	if s.Total != 87.6 {
		t.Errorf("總分應為 87.6，實際 %v", s.Total)
	}
	if s.Note != "" {
		t.Errorf("note 應為空（由 handler 填），實際 %s", s.Note)
	}
}

func TestScoreHealthMissingData(t *testing.T) {
	s := ScoreHealth(HealthInput{Code: "6547", Name: "高端疫苗"}, DefaultScoringConfig())
	// 邊界：完全無資料 → 各面向 0 分 + available=false + 註記，不臆測
	for name, d := range map[string]ScoreDimension{
		"profit": s.Profit, "growth": s.Growth, "structure": s.Structure,
		"dividend": s.Dividend, "governance": s.Governance,
	} {
		if d.Available {
			t.Errorf("%s 無資料應 available=false，實際 %+v", name, d)
		}
		if d.Score != 0 {
			t.Errorf("%s 無資料應 0 分，實際 %+v", name, d)
		}
		if d.Note == "" {
			t.Errorf("%s 無資料應有註記，實際 %+v", name, d)
		}
	}
	if s.Total != 0 {
		t.Errorf("無資料總分應為 0，實際 %v", s.Total)
	}
}

func TestScoreHealthGrowthNoPrevYear(t *testing.T) {
	in := HealthInput{
		Income: []model.IncomeStatementRow{{Year: 2026, Quarter: 1, Revenue: 100, NetIncome: 10}},
	}
	s := ScoreHealth(in, DefaultScoringConfig())
	if s.Growth.Available {
		t.Error("無去年同期財報應不評分")
	}
	if s.Growth.Note == "" {
		t.Error("應有缺去年同期之註記")
	}
}

func TestScoreHealthDividendBreak(t *testing.T) {
	// 連年配息中斷：115/114 有配、113 未配 → consecutive=2（由新至舊）
	in := HealthInput{
		DividendYears: []DividendYear{
			{Year: "115", Cash: 5.0},
			{Year: "114", Cash: 4.0},
			{Year: "113", Cash: 0},
		},
		Yield: 6.0,
	}
	s := ScoreHealth(in, DefaultScoringConfig())
	// 2/5×70=28；2/3 有配 → 0.667/0.8=0.833 → 25；殖利率 6 ≥ 5 → +5；共 58
	if s.Dividend.Score != 58 {
		t.Errorf("配息評分應為 58，實際 %+v", s.Dividend)
	}
}

func TestScoreHealthCustomConfig(t *testing.T) {
	cfg := DefaultScoringConfig()
	cfg.Version = "v2"
	cfg.Weights = WeightSet{Profit: 0.5, Growth: 0.1, Structure: 0.1, Dividend: 0.1, Governance: 0.2}
	cfg.GrossMarginMax = 80 // 毛利率 59% → 73.75 分
	cfg.YieldBonus = 0
	s := ScoreHealth(fullInput(), cfg)
	if s.ScoringVersion != "v2" {
		t.Errorf("scoring_version 應為 v2，實際 %s", s.ScoringVersion)
	}
	// profit = (73.75+100+100)/3 = 91.25；dividend 無加分 → 58
	want := 0.5*91.25 + 0.1*69.4 + 0.1*100 + 0.1*58 + 0.2*100
	if s.Total != round1(want) {
		t.Errorf("自訂權重總分應為 %v，實際 %v", round1(want), s.Total)
	}
	if s.Profit.Score != 91.3 {
		t.Errorf("客製門檻後獲利能力應為 91.3，實際 %v", s.Profit.Score)
	}
}

func TestScoreHealthGovernancePartial(t *testing.T) {
	// 僅 ESG 揭露：50+25=75
	in := HealthInput{ESGDisclosed: true}
	s := ScoreHealth(in, DefaultScoringConfig())
	if !s.Governance.Available || s.Governance.Score != 75 {
		t.Errorf("僅 ESG 揭露應為 75，實際 %+v", s.Governance)
	}
	// 僅治理規程：50+25=75
	in = HealthInput{GovernanceDisclosed: true}
	s = ScoreHealth(in, DefaultScoringConfig())
	if !s.Governance.Available || s.Governance.Score != 75 {
		t.Errorf("僅治理規程應為 75，實際 %+v", s.Governance)
	}
}
