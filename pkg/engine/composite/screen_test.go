package composite

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

// TestScreenValueSortAndTopN：排序與 top_n 限制（T017）。
func TestScreenValueSortAndTopN(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
		valRow("2317", "鴻海", 12, true, 1.5, 3.5, 7.2, 8.0, true),
		valRow("1101", "台泥", 0, false, 0.77, 3.29, 0.8, -5.0, true),
		valRow("6147", "頎邦", 15, true, 2.0, 4.0, 4.0, 6.0, true),
		valRow("6547", "高端疫苗", 40, true, 8.0, 0.5, 0, 0, false),
	}
	// 無條件 + yield 排序：6147(4.0) → 2317(3.5) → 1101(3.29) → 2330(2.1) → 6547(0.5)
	got := ScreenValue(rows, ValueCriterion{Sort: ScreenSortYield})
	if len(got) != 5 {
		t.Fatalf("應回傳 5 列，實際 %d", len(got))
	}
	wantOrder := []string{"6147", "2317", "1101", "2330", "6547"}
	for i, c := range wantOrder {
		if got[i].Code != c {
			t.Fatalf("yield 排序第 %d 應為 %s，實際 %s", i, c, got[i].Code)
		}
	}
	// top_n 限制
	got = ScreenValue(rows, ValueCriterion{Sort: ScreenSortYield, TopN: 2})
	if len(got) != 2 || got[0].Code != "6147" || got[1].Code != "2317" {
		t.Fatalf("TopN=2 應回傳 6147/2317，實際 %+v", got)
	}
	// PB 升冪（0 者置後）＋growth 遞減
	got = ScreenValue(rows, ValueCriterion{Sort: ScreenSortPB})
	if got[0].Code != "1101" || got[0].PB != 0.77 {
		t.Fatalf("PB 排序首列應為 1101（0.77），實際 %+v", got[0])
	}
	got = ScreenValue(rows, ValueCriterion{Sort: ScreenSortGrowth})
	if got[0].Code != "2330" || got[0].RevenueGrowth != 15.0 {
		t.Fatalf("growth 排序首列應為 2330（15%%），實際 %+v", got[0])
	}
}

// TestScreenValueGrowthCondition：value + growth 條件組合（T017）。
func TestScreenValueGrowthCondition(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2330", "台積電", 20, true, 5.2, 2.1, 6.0, 15.0, true),
		valRow("2317", "鴻海", 12, true, 1.5, 3.5, 7.2, 8.0, true),
		valRow("1101", "台泥", 0, false, 0.77, 3.29, 0.8, -5.0, true),
	}
	// 僅 growth 條件：2330（15%）
	got := ScreenValue(rows, ValueCriterion{MinGrowth: 10})
	if len(got) != 1 || got[0].Code != "2330" {
		t.Fatalf("MinGrowth 10 應僅命中 2330，實際 %+v", got)
	}
	if len(got[0].Matched) != 1 || got[0].Matched[0] != "營收成長" {
		t.Errorf("命中條件標記錯誤: %v", got[0].Matched)
	}
	// 邊界：無成長資料之個股（HasGrowth=false）在 MinGrowth 下被跳過
	noGrowth := valRow("6547", "高端疫苗", 40, true, 8.0, 0.5, 0, 0, false)
	got = ScreenValue(append(rows, noGrowth), ValueCriterion{MinGrowth: 10})
	for _, m := range got {
		if m.Code == "6547" {
			t.Error("無成長資料之個股不應命中 MinGrowth 條件")
		}
	}
	// 獲利成長（淨利 YoY）條件：僅命中有去年同期財報者
	rows[0].ProfitGrowth, rows[0].HasProfitGrowth = 18.0, true // 2330
	rows[1].ProfitGrowth, rows[1].HasProfitGrowth = 5.0, true  // 2317
	got = ScreenValue(rows, ValueCriterion{MinProfitGrowth: 10})
	if len(got) != 1 || got[0].Code != "2330" {
		t.Fatalf("MinProfitGrowth 10 應僅命中 2330（18%%），實際 %+v", got)
	}
	if len(got[0].Matched) != 1 || got[0].Matched[0] != "獲利成長" {
		t.Errorf("命中條件標記錯誤: %v", got[0].Matched)
	}
	// 邊界：無去年同期財報（HasProfitGrowth=false）在 MinProfitGrowth 下被跳過
	got = ScreenValue(append(rows, noGrowth), ValueCriterion{MinProfitGrowth: 10})
	for _, m := range got {
		if m.Code == "6547" {
			t.Error("無財報資料之個股不應命中 MinProfitGrowth 條件")
		}
	}
}

// TestScreenHighYieldMinConsecutive：配息穩定性過濾（T017）。
func TestScreenHighYieldMinConsecutive(t *testing.T) {
	rows := []ValuationMetrics{
		valRow("2317", "鴻海", 12, true, 1.5, 3.5, 7.2, 8.0, true),
		valRow("1101", "台泥", 0, false, 0.77, 3.29, 0.8, -5.0, true),
	}
	rows[0].ConsecutiveYears = 3
	rows[1].ConsecutiveYears = 1
	got := ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0, MinConsecutive: 2})
	if len(got) != 1 || got[0].Code != "2317" {
		t.Fatalf("MinConsecutive=2 應僅命中 2317（3 年），實際 %+v", got)
	}
	if got[0].ConsecutiveYears != 3 {
		t.Errorf("ConsecutiveYears 應透出為 3，實際 %d", got[0].ConsecutiveYears)
	}
	// MinConsecutive=0 不限制
	got = ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0})
	if len(got) != 2 {
		t.Fatalf("未設定穩定性條件應回傳 2 列，實際 %d", len(got))
	}
	// top_n 限制（依殖利率排序後截斷）
	got = ScreenHighYield(rows, HighYieldCriterion{MinYield: 3.0, TopN: 1})
	if len(got) != 1 || got[0].Code != "2317" {
		t.Fatalf("TopN=1 應回傳殖利率最高之 2317，實際 %+v", got)
	}
}
