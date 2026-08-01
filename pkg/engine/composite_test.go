package engine

import "testing"

// composite_test.go：T014 篩選引擎契約測試。
// 邊界案例：虧損（無本益比）、無營收成長資料（新股）、ETF 排除、無命中。

func valRow(code, name string, pe float64, peOK bool, pb, yield, div, growth float64, hasGrowth bool) ValuationMetrics {
	return ValuationMetrics{
		Code: code, Name: name, PE: pe, PEAvailable: peOK, PB: pb,
		DividendYield: yield, DividendShare: div, RevenueGrowth: growth, HasGrowth: hasGrowth,
	}
}

func TestScreenValue(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
		valRow("2317", "鴻海", 12, true, 1.5, 3.5, 7.2, 8.0, true),
		valRow("1101", "台泥", 0, false, 0.77, 3.29, 0.8, -5.0, true), // 虧損：無 PE
		valRow("6147", "頎邦", 15, true, 2.0, 4.0, 4.0, 6.0, true),
		valRow("6547", "高端疫苗", 40, true, 8.0, 0, 0, 0, false),         // 新股：無成長資料
		valRow("0050", "元大台灣50", 18, true, 1.8, 3.0, 5.0, 10.0, true), // ETF：預設排除
	}

	got := ScreenValue(rows, ValueCriterion{MaxPE: 15, MaxPB: 2.0, MinYield: 3.0, MinGrowth: 5.0})
	if len(got) != 2 {
		t.Fatalf("應命中 2 列（2317、6147），實際 %d", len(got))
	}
	// 2317：PE 12 ≤ 15、PB 1.5 ≤ 2、殖利率 3.5 ≥ 3、成長 8 ≥ 5
	if got[0].Code != "2317" {
		t.Errorf("首列應為 2317，實際 %s", got[0].Code)
	}
	if len(got[0].Matched) != 4 {
		t.Errorf("2317 應命中 4 條件，實際 %v", got[0].Matched)
	}
	// 虧損（1101）無 PE → 不命中低本益比條件
	for _, m := range got {
		if m.Code == "1101" {
			t.Error("虧損公司 1101 不應命中")
		}
	}
	// 無成長資料（6547）：MinGrowth 條件下不命中
	for _, m := range got {
		if m.Code == "6547" {
			t.Error("無成長資料 6547 不應命中 MinGrowth")
		}
	}
}

func TestScreenValueNoCondition(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
		valRow("0050", "元大台灣50", 18, true, 1.8, 3.0, 5.0, 10.0, true),
	}
	// 無條件：回傳全部（排除 ETF）
	got := ScreenValue(rows, ValueCriterion{})
	if len(got) != 1 || got[0].Code != "2330" {
		t.Fatalf("無條件應僅回傳 2330（排除 ETF），實際 %+v", got)
	}
	// AllowETFs 開啟時包含 ETF
	got = ScreenValue(rows, ValueCriterion{AllowETFs: true})
	if len(got) != 2 {
		t.Fatalf("AllowETFs 應回傳 2 列，實際 %d", len(got))
	}
}

func TestScreenValueNoMatch(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
	}
	// 邊界：篩選無結果
	got := ScreenValue(rows, ValueCriterion{MaxPE: 5})
	if len(got) != 0 {
		t.Fatalf("無命中應為空，實際 %d", len(got))
	}
}

func TestScreenHighYield(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
		valRow("2317", "鴻海", 12, true, 1.5, 3.5, 7.2, 8.0, true),
		valRow("1101", "台泥", 0, false, 0.77, 3.29, 0.8, -5.0, true),
		valRow("0050", "元大台灣50", 18, true, 1.8, 6.0, 5.0, 10.0, true), // ETF：排除
		valRow("6547", "高端疫苗", 40, true, 8.0, 0.5, 0, 0, false),       // 低殖利率
	}
	got := ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0})
	if len(got) != 2 {
		t.Fatalf("應命中 2 列（2317、1101），實際 %d", len(got))
	}
	// 依殖利率遞減排序：2317(3.5) → 1101(3.29)
	if got[0].Code != "2317" || got[1].Code != "1101" {
		t.Errorf("排序錯誤: %s(%%) > %s(%%)", got[0].Code, got[1].Code)
	}
	// 每股股利門檻
	got = ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0, MinDividend: 5.0})
	if len(got) != 1 || got[0].Code != "2317" {
		t.Fatalf("MinDividend 5.0 應僅命中 2317（7.2），實際 %+v", got)
	}
	// 虧損公司（無 PE）：MaxPE 條件不命中
	got = ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0, MaxPE: 15})
	if len(got) != 1 || got[0].Code != "2317" {
		t.Fatalf("MaxPE 15 應僅命中 2317（1101 無 PE），實際 %+v", got)
	}
}

func TestScreenHighYieldNoMatch(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
	}
	got := ScreenHighYield(rows, HighYieldCriterion{MinYield: 10})
	if len(got) != 0 {
		t.Fatalf("無命中應為空，實際 %d", len(got))
	}
}
