package screener

import (
	"reflect"
	"testing"

	"tw-quant-mcp/pkg/engine/composite"
)

// TestScreenValueDelegates 驗證 domain/screener 薄層與下層 composite 行為一致
// （不重複邏輯；型別別名維持型別相等，§7/§9.5 對齊）。
func TestScreenValueDelegates(t *testing.T) {
	rows := []ValuationMetrics{
		{Code: "2330", Name: "台積電", Market: "TSE", PE: 10, PEAvailable: true, DividendYield: 1.2, RevenueGrowth: 25, HasGrowth: true, ConsecutiveYears: 10},
		{Code: "0056", Name: "某ETF", Market: "TSE", PE: 22, DividendYield: 5.0}, // ETF 應被排除
		{Code: "2414", Name: "中華電", Market: "TSE", PE: 30, PEAvailable: true, DividendYield: 4.0, ConsecutiveYears: 20},
	}
	crit := ValueCriterion{MaxPE: 25, MinYield: 1.0, Sort: SortByPE, TopN: 10}

	got := ScreenValue(rows, crit)
	want := composite.ScreenValue(rows, composite.ValueCriterion(crit))
	if len(got) != len(want) {
		t.Fatalf("len(ScreenValue)=%d，下層應為 %d", len(got), len(want))
	}
	for i := range got {
		if !reflect.DeepEqual(got[i], Match(want[i])) {
			t.Errorf("第 %d 項結果不一致：got %+v want %+v", i, got[i], want[i])
		}
	}
}

// TestScreenHighYieldDelegates 驗證高殖利率篩選薄層與下層一致。
func TestScreenHighYieldDelegates(t *testing.T) {
	rows := []ValuationMetrics{
		{Code: "2414", Name: "中華電", DividendShare: 4.2, DividendYield: 4.2, ConsecutiveYears: 20},
		{Code: "0056", Name: "某ETF", DividendYield: 5.0}, // ETF 應被排除
		{Code: "2330", Name: "台積電", DividendYield: 1.5, DividendShare: 5.0, ConsecutiveYears: 15},
	}
	crit := HighYieldCriterion{MinYield: 3.0, MinDividend: 1.0, TopN: 10}

	got := ScreenHighYield(rows, crit)
	want := composite.ScreenHighYield(rows, composite.HighYieldCriterion(crit))
	if len(got) != len(want) {
		t.Fatalf("len(ScreenHighYield)=%d，下層=%d", len(got), len(want))
	}
	for i := range got {
		if !reflect.DeepEqual(got[i], Match(want[i])) {
			t.Errorf("第 %d 項不一致：got %+v want %+v", i, got[i], want[i])
		}
	}
}
