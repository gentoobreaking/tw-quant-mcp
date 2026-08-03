package fundamental

import (
	"testing"

	"tw-quant-mcp/pkg/engine/composite"
	"tw-quant-mcp/pkg/model"
)

// TestScoreHealthDelegates 驗證 domain/fundamental 薄層與下層 composite 行為一致
// （§7 對齊；型別別名維持型別相等，app 層既有斷言不受影響）。
func TestScoreHealthDelegates(t *testing.T) {
	in := HealthInput{
		Code: "2330", Name: "台積電", Market: "TSE",
		Balance:  &model.BalanceSheet{TotalAssets: 1_000_000, TotalLiabilities: 400_000},
		CashFlow: &model.CashFlowStatement{OperatingCashFlow: 120_000},
		DividendYears: []DividendYear{
			{Year: "113", Cash: 3.5},
			{Year: "112", Cash: 3.0},
			{Year: "111", Cash: 2.75},
			{Year: "110", Cash: 2.5},
			{Year: "109", Cash: 2.5},
			{Year: "108", Cash: 2.0},
		},
		Yield: 1.5,
	}

	got := ScoreHealth(in, DefaultScoringConfig())
	want := composite.ScoreHealth(composite.HealthInput(in), composite.DefaultScoringConfig())
	if got != HealthScore(want) {
		t.Errorf("ScoreHealth 委託不一致：\ngot  %+v\nwant %+v", got, want)
	}
	if got.ScoringVersion != "v1" {
		t.Errorf("scoring_version 應為 v1，實際 %s", got.ScoringVersion)
	}
	if !got.Structure.Available || got.Structure.Score != 100 {
		t.Errorf("負債比 40%% 應為滿分 100，實際 %+v", got.Structure)
	}
}
