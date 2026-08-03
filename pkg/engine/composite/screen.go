// Package composite 實作 §10.D/E 複合分析引擎（T017）：
// 篩選（screen_stocks / screen_high_yield）與五面向評分
// （get_financial_health_check，見 health.go）。
//
// 本包為 §7 模組化邊界之「domain 層下層」引擎（T026 對齊）：業務入口
// 由 pkg/domain/screener、pkg/domain/fundamental 承載，composite 僅被
// 下層/設定層引用（domain/screener、domain/fundamental、pkg/config），
// 不屬於「已知九情境」，不重複產生業務入口。
//
// 引擎僅接受「已由呼叫端透過快取取得」之 raw 資料輸入，禁止直接呼叫
// Adapter（§12.4 記憶體計算、§6 架構圖）。輸出為 helper 資料，
// 由呼叫端以 source_role=helper + derived_from 標明父資料集。
package composite

import "sort"

// ValuationMetrics 為單一候選股之估值/成長指標（篩選輸入）。
// 無該指標資料時以 0 + 對應 available=false 表示（如虧損公司無本益比）。
type ValuationMetrics struct {
	Code             string
	Name             string
	Market           string
	PE               float64
	PEAvailable      bool
	PB               float64
	DividendYield    float64
	DividendShare    float64
	RevenueGrowth    float64
	HasGrowth        bool // 是否具營收成長資料（新股/新上市可能無 YoY）
	ProfitGrowth     float64
	HasProfitGrowth  bool // 是否具淨利成長資料（需去年同期財報）
	ConsecutiveYears int  // 連年配息年數（配息穩定性，§10.E）
}

// ValueCriterion 為 screen_stocks 之價值/成長條件（§10.D）。
// 欄位為 0 時表示不限制；MinGrowth/MinProfitGrowth 僅在對應 Has* 時判定。
type ValueCriterion struct {
	MaxPE           float64    // 低本益比上限（需 PEAvailable）
	MaxPB           float64    // 低股價淨值比上限
	MinYield        float64    // 最低現金殖利率（%）
	MinGrowth       float64    // 最低營收 YoY（%）
	MinProfitGrowth float64    // 最低淨利 YoY（%）
	MinPB           float64    // 股價淨值比下限（0=不限制）
	AllowETFs       bool       // 是否納入 ETF/指數型（預設排除 00 開頭）
	Sort            ScreenSort // 排序（預設 ScreenSortNone＝候選順序）
	TopN            int        // 回傳上限（0=不限）
}

// HighYieldCriterion 為 screen_high_yield 之條件（§10.E）。
type HighYieldCriterion struct {
	MinYield       float64 // 最低現金殖利率（%）
	MinDividend    float64 // 最低每股現金股利（元/股）
	MaxPE          float64 // 本益比上限（0=不限制）
	MinConsecutive int     // 最低連年配息年數（配息穩定性，0=不限制）
	TopN           int     // 回傳上限（0=不限）
}

// ScreenSort 定義篩選結果排序方式（T017：排序、top_n）。
type ScreenSort int

const (
	// ScreenSortNone 保留候選順序（預設）。
	ScreenSortNone ScreenSort = iota
	// ScreenSortPE 依本益比升冪（無本益比者置後）。
	ScreenSortPE
	// ScreenSortYield 依殖利率遞減。
	ScreenSortYield
	// ScreenSortPB 依股價淨值比升冪（0 者置後）。
	ScreenSortPB
	// ScreenSortGrowth 依營收成長遞減（無成長資料者置後）。
	ScreenSortGrowth
)

// Match 為單一命中列（含命中條件說明）。
type Match struct {
	Code             string
	Name             string
	Market           string
	PE               float64
	PEAvailable      bool
	PB               float64
	DividendYield    float64
	DividendShare    float64
	RevenueGrowth    float64
	HasGrowth        bool
	ProfitGrowth     float64
	HasProfitGrowth  bool
	ConsecutiveYears int
	Matched          []string
}

func (m Match) metrics() ValuationMetrics {
	return ValuationMetrics{
		Code: m.Code, Name: m.Name, Market: m.Market,
		PE: m.PE, PEAvailable: m.PEAvailable, PB: m.PB,
		DividendYield: m.DividendYield, DividendShare: m.DividendShare,
		RevenueGrowth: m.RevenueGrowth, HasGrowth: m.HasGrowth,
		ConsecutiveYears: m.ConsecutiveYears,
	}
}

func matchOf(v ValuationMetrics, matched []string) Match {
	return Match{
		Code: v.Code, Name: v.Name, Market: v.Market,
		PE: v.PE, PEAvailable: v.PEAvailable, PB: v.PB,
		DividendYield: v.DividendYield, DividendShare: v.DividendShare,
		RevenueGrowth: v.RevenueGrowth, HasGrowth: v.HasGrowth,
		ProfitGrowth: v.ProfitGrowth, HasProfitGrowth: v.HasProfitGrowth,
		ConsecutiveYears: v.ConsecutiveYears,
		Matched:          matched,
	}
}

// isETF 判斷是否為 ETF（代號 00 開頭）——篩選預設排除。
func isETF(code string) bool { return len(code) >= 2 && code[:2] == "00" }

// ScreenValue 依價值/成長條件篩選（§10.D screen_stocks）：
// 全量記憶體過濾（§12.4），依 Sort 排序並套用 TopN 限制。
func ScreenValue(rows []ValuationMetrics, c ValueCriterion) []Match {
	out := make([]Match, 0)
	for _, v := range rows {
		if !c.AllowETFs && isETF(v.Code) {
			continue
		}
		var matched []string
		if c.MaxPE > 0 {
			if !v.PEAvailable || v.PE > c.MaxPE {
				continue
			}
			matched = append(matched, "低本益比")
		}
		if c.MaxPB > 0 {
			if v.PB == 0 || v.PB > c.MaxPB {
				continue
			}
			matched = append(matched, "低股價淨值比")
		}
		if c.MinPB > 0 && (v.PB == 0 || v.PB < c.MinPB) {
			continue
		}
		if c.MinYield > 0 {
			if v.DividendYield < c.MinYield {
				continue
			}
			matched = append(matched, "高殖利率")
		}
		if c.MinGrowth > 0 {
			if !v.HasGrowth || v.RevenueGrowth < c.MinGrowth {
				continue
			}
			matched = append(matched, "營收成長")
		}
		if c.MinProfitGrowth > 0 {
			if !v.HasProfitGrowth || v.ProfitGrowth < c.MinProfitGrowth {
				continue
			}
			matched = append(matched, "獲利成長")
		}
		out = append(out, matchOf(v, matched))
	}
	sortMatches(out, c.Sort)
	return limitMatches(out, c.TopN)
}

// ScreenHighYield 依殖利率/股利條件篩選（§10.E screen_high_yield）：
// 命中列依殖利率遞減排序，套用 TopN；MinConsecutive 過濾配息穩定性。
func ScreenHighYield(rows []ValuationMetrics, c HighYieldCriterion) []Match {
	out := make([]Match, 0)
	for _, v := range rows {
		if isETF(v.Code) {
			continue
		}
		var matched []string
		if v.DividendYield < c.MinYield {
			continue
		}
		matched = append(matched, "高殖利率")
		if c.MinDividend > 0 {
			if v.DividendShare < c.MinDividend {
				continue
			}
			matched = append(matched, "高現金股利")
		}
		if c.MaxPE > 0 {
			if !v.PEAvailable || v.PE > c.MaxPE {
				continue
			}
			matched = append(matched, "低本益比")
		}
		if c.MinConsecutive > 0 {
			if v.ConsecutiveYears < c.MinConsecutive {
				continue
			}
			matched = append(matched, "連年配息")
		}
		out = append(out, matchOf(v, matched))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DividendYield > out[j].DividendYield
	})
	return limitMatches(out, c.TopN)
}

// sortMatches 依 Sort 排序命中列（不指定時保留候選順序）。
func sortMatches(ms []Match, s ScreenSort) {
	switch s {
	case ScreenSortPE:
		sort.SliceStable(ms, func(i, j int) bool {
			if ms[i].PEAvailable != ms[j].PEAvailable {
				return ms[i].PEAvailable
			}
			return ms[i].PE < ms[j].PE
		})
	case ScreenSortYield:
		sort.SliceStable(ms, func(i, j int) bool { return ms[i].DividendYield > ms[j].DividendYield })
	case ScreenSortPB:
		sort.SliceStable(ms, func(i, j int) bool {
			if (ms[i].PB == 0) != (ms[j].PB == 0) {
				return ms[i].PB != 0
			}
			return ms[i].PB < ms[j].PB
		})
	case ScreenSortGrowth:
		sort.SliceStable(ms, func(i, j int) bool {
			if ms[i].HasGrowth != ms[j].HasGrowth {
				return ms[i].HasGrowth
			}
			return ms[i].RevenueGrowth > ms[j].RevenueGrowth
		})
	}
}

// limitMatches 套用 TopN 限制（0=不限）。
func limitMatches(ms []Match, n int) []Match {
	if n <= 0 || len(ms) <= n {
		return ms
	}
	return ms[:n]
}
