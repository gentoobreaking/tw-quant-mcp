package engine

import (
	"sort"
)

// composite.go：D/E 組篩選引擎（T014）。
// 純記憶體批次過濾（§12.4：篩選類工具整批透過快取+記憶體計算，
// 避免逐股打上游）；五面向評分屬 T017 composite engine，本檔不做評分。

// ValuationMetrics 為單一候選股之估值/成長指標（篩選輸入）。
// 無該指標資料時以 0 + 對應 available=false 表示（如虧損公司無本益比）。
type ValuationMetrics struct {
	Code          string
	Name          string
	Market        string
	PE            float64
	PEAvailable   bool
	PB            float64
	DividendYield float64
	DividendShare float64
	RevenueGrowth float64
	HasGrowth     bool // 是否具營收成長資料（新股/新上市可能無 YoY）
}

// ValueCriterion 為 screen_stocks 之價值/成長條件（§10.D）。
// 欄位為 0 時表示不限制；MinGrowth 僅在 HasGrowth 時判定。
type ValueCriterion struct {
	MaxPE     float64 // 低本益比上限（需 PEAvailable）
	MaxPB     float64 // 低股價淨值比上限
	MinYield  float64 // 最低現金殖利率（%）
	MinGrowth float64 // 最低營收 YoY（%）
	MinPB     float64 // 股價淨值比下限（0=不限制）
	AllowETFs bool    // 是否納入 ETF/指數型（預設排除 00 開頭）
}

// HighYieldCriterion 為 screen_high_yield 之條件（§10.E）。
type HighYieldCriterion struct {
	MinYield    float64 // 最低現金殖利率（%）
	MinDividend float64 // 最低每股現金股利（元/股）
	MaxPE       float64 // 本益比上限（0=不限制）
}

// Match 為單一命中列（含命中條件說明）。
type Match struct {
	Code          string
	Name          string
	Market        string
	PE            float64
	PEAvailable   bool
	PB            float64
	DividendYield float64
	DividendShare float64
	RevenueGrowth float64
	HasGrowth     bool
	Matched       []string
}

func (m Match) metrics() ValuationMetrics {
	return ValuationMetrics{
		Code: m.Code, Name: m.Name, Market: m.Market,
		PE: m.PE, PEAvailable: m.PEAvailable, PB: m.PB,
		DividendYield: m.DividendYield, DividendShare: m.DividendShare,
		RevenueGrowth: m.RevenueGrowth, HasGrowth: m.HasGrowth,
	}
}

func matchOf(v ValuationMetrics, matched []string) Match {
	return Match{
		Code: v.Code, Name: v.Name, Market: v.Market,
		PE: v.PE, PEAvailable: v.PEAvailable, PB: v.PB,
		DividendYield: v.DividendYield, DividendShare: v.DividendShare,
		RevenueGrowth: v.RevenueGrowth, HasGrowth: v.HasGrowth,
		Matched: matched,
	}
}

// isETF 判斷是否為 ETF（代號 00 開頭）——篩選預設排除。
func isETF(code string) bool { return len(code) >= 2 && code[:2] == "00" }

// ScreenValue 依價值/成長條件篩選（§10.D screen_stocks）：
// 全量記憶體過濾，回傳命中列（保留候選原始順序）。
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
		out = append(out, matchOf(v, matched))
	}
	return out
}

// ScreenHighYield 依殖利率/股利條件篩選（§10.E screen_high_yield）：
// 命中列依殖利率遞減排序。
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
		out = append(out, matchOf(v, matched))
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].DividendYield > out[j].DividendYield
	})
	return out
}
