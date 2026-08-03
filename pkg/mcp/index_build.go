package mcp

// index_build.go 實作 §10.3 Materialized Screener Index 之每日重建：
// 每交易日 15:00 以既有整批/快取路徑（§12.4）抓取全市場估值、股利、
// 營收成長、財報摘要，並逐檔計算 FinancialHealthReport.OverallScore
// （無批次端點之逐檔情境以 ScanUniverse bounded concurrency 執行，§10.2），
// 彙總寫入 SQLite 索引（pkg/domain/screener.Store，L2）。
//
// 索引為「今日資料之快照」：查詢端（queryHighYieldIndex）於索引存在時
// 直接 SELECT，零即時 Adapter 請求；freshness 以索引建立時間標註（§10.3）。

import (
	"context"
	"fmt"
	"sync"

	"tw-quant-mcp/pkg/domain/fundamental"
	"tw-quant-mcp/pkg/domain/screener"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// rebuildScreenerIndex 重建今日（app 時鐘）之 Materialized Screener Index。
// 資料來源全數為既有整批路徑（valuationTSE/valuationOTC/apiRows/mopsRows，
// §12.4：整批快取 + 記憶體計算），不新增逐檔上游呼叫：
//   - 估值（PE/PB/殖利率）：上市 BWIBBU_ALL、上櫃 TPEx 本益比/殖利率/淨值比
//   - 股利（每股現金股利 + 連年配息年數）：上市 t187ap45_L、上櫃最新年度
//   - 成長（月營收 YoY、淨利 YoY）：MOPS 月營收 / 損益表摘要（整批）
//   - 財報體檢 OverallScore：MOPS 財報三表（逐檔 AJAX，經 ScanUniverse
//     bounded concurrency，§10.2；失敗之標的以 0 分（缺資料）寫入）
//
// 索引寫入失敗（如 SQLite 鎖定）回傳錯誤，由排程器記錄（不阻塞其餘階段）。
func (a *App) rebuildScreenerIndex(ctx context.Context) error {
	if a.index == nil {
		return nil // 未啟用（測試/無資料目錄）
	}
	now := a.now()
	date := now.Format("2006-01-02")

	// 1. 整批快照資料（§12.4；與 screenMetrics 共用同一快取鍵，天然熱）。
	metrics, _, err := a.screenMetrics(ctx, "")
	if err != nil {
		return fmt.Errorf("索引重建：整批指標取得失敗: %w", err)
	}
	if len(metrics) == 0 {
		return fmt.Errorf("索引重建：全市場候選數為 0（資料源未就緒）")
	}

	// 2. 逐檔 OverallScore（財報體檢；無批次端點之逐檔情境，§10.2）。
	//    以 bounded concurrency 掃描，任一標的失敗僅記錄（該檔 0 分），
	//    不中斷其餘標的（§12.9 失敗不阻塞原則）。
	cfg, err := a.scoringConfig()
	if err != nil {
		return fmt.Errorf("索引重建：評分規則取得失敗: %w", err)
	}
	scores := a.scoreUniverse(ctx, metrics, cfg)

	// 3. 組 IndexRow 並寫入（單 transaction 重建）。
	rows := make([]screener.IndexRow, 0, len(metrics))
	for _, m := range metrics {
		rows = append(rows, screener.IndexRow{
			Symbol:               m.Code,
			Name:                 m.Name,
			Market:               model.MarketTSE,
			DividendYieldPct:     m.DividendYield,
			CashDividend:         m.DividendShare,
			PayoutStability:      stabilityFromYears(m.ConsecutiveYears),
			ConsecutiveYears:     m.ConsecutiveYears,
			PE:                   m.PE,
			PEAvailable:          m.PE > 0,
			PB:                   m.PB,
			RevenueGrowthPct:     m.RevenueGrowth,
			ProfitGrowthPct:      m.ProfitGrowth,
			FinancialHealthScore: scores[m.Code],
		})
		if m.Market == model.MarketOTC {
			rows[len(rows)-1].Market = model.MarketOTC
		}
	}
	return a.index.Replace(ctx, date, rows, now)
}

// scoreUniverse 對全市場候選逐檔計算財報體檢 OverallScore。
// 輸入與 handlerGetFinancialHealthCheck 同源（整批 MOPS 資料），
// 僅財報三表（balance/cash_flow）為逐檔 AJAX：以 ScanUniverse
// （errgroup.SetLimit=RATE_LIMIT_BULK_CONCURRENCY，§10.2）執行。
// 回傳 map[代碼]OverallScore；失敗/缺資料之標的為 0（不臆測）。
func (a *App) scoreUniverse(ctx context.Context, metrics []screener.ValuationMetrics, cfg fundamental.ScoringConfig) map[string]float64 {
	out := make(map[string]float64, len(metrics))
	concurrency := 8
	if a.cfg != nil && a.cfg.RateLimitBulkConcurrency > 0 {
		concurrency = a.cfg.RateLimitBulkConcurrency
	}
	// 整批父資料（與 screenMetrics 共用快取）：損益表摘要 + 獲利能力 + 股利。
	income, _, _, ierr := mopsRows[model.IncomeStatementRow](a, ctx, provider.MOPSIncomeSummary)
	profit, _, _, perr := mopsRows[model.ProfitabilityRatio](a, ctx, provider.MOPSProfitRatios)
	divRows, _, _, derr := apiRows[provider.DividendRow](a, ctx, provider.TWSEAPIDividend)
	esgSet, esgErr := a.esgCodes(ctx)

	latest := map[string]model.IncomeStatementRow{}
	if ierr == nil {
		latest, _ = latestIncomeOf(income), incomeAgoOf(income)
	}
	profitBy := map[string][]model.ProfitabilityRatio{}
	if perr == nil {
		for _, p := range profit {
			profitBy[p.Code] = append(profitBy[p.Code], p)
		}
	}
	divBy := map[string][]provider.DividendRow{}
	if derr == nil {
		for _, d := range divRows {
			divBy[d.Code] = append(divBy[d.Code], d)
		}
	}

	symbols := make([]string, 0, len(metrics))
	for _, m := range metrics {
		symbols = append(symbols, m.Code)
	}
	var mu sync.Mutex
	_ = screener.ScanUniverse(ctx, symbols, concurrency, func(code string) error {
		li, ok := latest[code]
		if !ok {
			mu.Lock()
			out[code] = 0
			mu.Unlock()
			return nil
		}
		in := fundamental.HealthInput{
			Code: code, Name: nameOf(metrics, code), Market: marketOf(metrics, code),
			Profit: profitBy[code],
			Income: incomeOf(income, code),
		}
		if li.Year > 0 {
			if bs, _, _, berr := mopsStatement[model.BalanceSheet](a, ctx, provider.MOPSBalanceSheet, code, li.Year, li.Quarter); berr == nil {
				in.Balance = &bs
			}
			if cf, _, _, cerr := mopsStatement[model.CashFlowStatement](a, ctx, provider.MOPSCashFlow, code, li.Year, li.Quarter); cerr == nil {
				in.CashFlow = &cf
			}
		}
		if dv, ok := divBy[code]; ok {
			for _, r := range dv {
				in.DividendYears = append(in.DividendYears, fundamental.DividendYear{Year: r.DividendYear, Cash: r.CashDividend})
			}
		}
		in.Yield = yieldOf(metrics, code)
		if esgErr == nil {
			in.ESGDisclosed = esgSet[code]
		}
		score := fundamental.ScoreHealth(in, cfg)
		mu.Lock()
		out[code] = score.Total
		mu.Unlock()
		return nil
	})
	return out
}

// stabilityFromYears 以連年配息年數導出配息穩定度（0-100）：
// 0 年=0；1-5 年線性 20..100（每年 +20）；≥5 年 100。
// 與 DividendRecord.PayoutStabilityScore（§9.4 0-100）同語意。
func stabilityFromYears(n int) float64 {
	switch {
	case n <= 0:
		return 0
	case n >= 5:
		return 100
	}
	return float64(n) * 20
}

// nameOf / marketOf / yieldOf 自候選清單查詢單檔屬性（O(n) 小集合）。
func nameOf(metrics []screener.ValuationMetrics, code string) string {
	for _, m := range metrics {
		if m.Code == code {
			return m.Name
		}
	}
	return ""
}

func marketOf(metrics []screener.ValuationMetrics, code string) string {
	for _, m := range metrics {
		if m.Code == code {
			return m.Market
		}
	}
	return ""
}

func yieldOf(metrics []screener.ValuationMetrics, code string) float64 {
	for _, m := range metrics {
		if m.Code == code {
			return m.DividendYield
		}
	}
	return 0
}
