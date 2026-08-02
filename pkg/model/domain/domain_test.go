package domain

import (
	"encoding/json"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// T022 驗收：每 Schema JSON round-trip（欄位與 v2.1 §6 一致，含 omitempty 規則）。

func roundTrip[T any](t *testing.T, v T) T {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var back T
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	return back
}

func TestStockIdentityRoundTrip(t *testing.T) {
	in := StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE", Industry: "半導體"}
	back := roundTrip(t, in)
	if back != in {
		t.Errorf("round trip 不符: got %+v want %+v", back, in)
	}
	// industry 為 omitempty
	var m map[string]any
	b, _ := json.Marshal(StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE"})
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["industry"]; ok {
		t.Error("industry 為空值時不應輸出")
	}
	if v, _ := m["market"].(string); v != "TSE" {
		t.Errorf("market 應為 TSE，實際 %v", m["market"])
	}
}

func TestTrendCompositeRoundTrip(t *testing.T) {
	in := TrendComposite{
		Stock: StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE", Industry: "半導體"},
		Technical: TechnicalView{
			MA5: 900, MA20: 880, MA60: 850, RSI14: 62.5, TrendSignal: "BULLISH",
		},
		Fundamental: FundamentalView{PE: 18.2, PB: 4.1, DividendYieldPct: 2.3, EPSGrowthYoYPct: 12.5},
		Chip:        ChipView{ForeignNetShares5D: 12345, TrustNetShares5D: -678},
		Horizon:     "mid",
		Lineage: []model.Lineage{
			{Source: model.SourceTWSEWeb, SourceRole: model.SourceRoleCanonical, Freshness: model.FreshnessPostMarket},
			{Source: model.SourceMOPS, SourceRole: model.SourceRoleCanonical, Freshness: model.FreshnessMonthly},
		},
		ChartData: map[string]any{"recommended_type": "line"},
	}
	back := roundTrip(t, in)
	if back.Stock != in.Stock || back.Horizon != "mid" || back.Technical.RSI14 != 62.5 {
		t.Errorf("round trip 不符: %+v", back)
	}
	if len(back.Lineage) != 2 || back.Lineage[1].Source != model.SourceMOPS {
		t.Errorf("_lineage 陣列 round trip 不符: %+v", back.Lineage)
	}
}

func TestInstitutionalFlowRoundTrip(t *testing.T) {
	in := InstitutionalFlow{
		Stock:            StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE"},
		Date:             "2026-07-31",
		Market:           "TSE",
		ForeignNetShares: 5000,
		TrustNetShares:   -200,
		DealerNetShares:  100,
		Lineage: model.Lineage{
			Source: model.SourceTWSEWeb, SourceRole: model.SourceRoleCanonical,
			FetchedAt: model.NewTaipeiTime(time.Date(2026, 7, 31, 14, 30, 0, 0, model.Taipei())),
			DataDate:  "2026-07-31", Freshness: model.FreshnessPostMarket,
		},
	}
	back := roundTrip(t, in)
	if back.ForeignNetShares != 5000 || back.Stock.Name != "台積電" || back.Lineage.Source != model.SourceTWSEWeb {
		t.Errorf("round trip 不符: %+v", back)
	}
	// foreign_holding_pct 為 omitempty
	b, _ := json.Marshal(in)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["foreign_holding_pct"]; ok {
		t.Error("foreign_holding_pct=0 時不應輸出")
	}
	in.ForeignHoldingPct = 42.5
	b, _ = json.Marshal(in)
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if v, _ := m["foreign_holding_pct"].(float64); v != 42.5 {
		t.Errorf("foreign_holding_pct 設定後應輸出 42.5，實際 %v", m["foreign_holding_pct"])
	}
}

func TestDividendRecordRoundTrip(t *testing.T) {
	in := DividendRecord{
		Stock:            StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE"},
		FiscalYear:       "2025",
		CashDividend:     8.0,
		StockDividend:    0,
		DividendYieldPct: 2.5,
		Lineage: model.Lineage{
			Source: model.SourceTWSEWeb, SourceRole: model.SourceRoleCanonical,
			Freshness: model.FreshnessPostMarket,
		},
	}
	back := roundTrip(t, in)
	if back.FiscalYear != "2025" || back.CashDividend != 8.0 || back.DividendYieldPct != 2.5 {
		t.Errorf("round trip 不符: %+v", back)
	}
	// ex_dividend_date / ex_right_date / payout_stability_score 為 omitempty
	b, _ := json.Marshal(in)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"ex_dividend_date", "ex_right_date", "payout_stability_score"} {
		if _, ok := m[absent]; ok {
			t.Errorf("欄位 %q 為空值時不應輸出", absent)
		}
	}
	in.ExDividendDate = "2026-07-15"
	in.PayoutStabilityScore = 88
	back = roundTrip(t, in)
	if back.ExDividendDate != "2026-07-15" || back.PayoutStabilityScore != 88 {
		t.Errorf("round trip 不符: %+v", back)
	}
}

func TestFinancialHealthReportRoundTrip(t *testing.T) {
	in := FinancialHealthReport{
		Stock:              StockIdentity{Symbol: "2330", Name: "台積電", Market: "TSE"},
		Profitability:      DimensionScore{Score: 100, Metrics: map[string]float64{"roe": 30.1, "margin": 42.0}},
		Growth:             DimensionScore{Score: 80, Metrics: map[string]float64{"rev_growth": 12.3}},
		FinancialStructure: DimensionScore{Score: 70, Metrics: map[string]float64{"debt_ratio": 45.0}},
		DividendPolicy:     DimensionScore{Score: 60, Metrics: map[string]float64{"payout": 50.0}},
		Governance:         DimensionScore{Score: 90, Metrics: map[string]float64{"esg": 85.0}},
		OverallScore:       87.6,
		Lineage: []model.Lineage{
			{Source: model.SourceMOPS, SourceRole: model.SourceRoleCanonical, Freshness: model.FreshnessQuarterly},
		},
	}
	back := roundTrip(t, in)
	if back.OverallScore != 87.6 || back.Profitability.Score != 100 || back.Profitability.Metrics["roe"] != 30.1 {
		t.Errorf("round trip 不符: %+v", back)
	}
	if len(back.Lineage) != 1 || back.Lineage[0].Freshness != model.FreshnessQuarterly {
		t.Errorf("_lineage round trip 不符: %+v", back.Lineage)
	}
}

func TestRiskFlagsRoundTrip(t *testing.T) {
	in := RiskFlags{
		Stock:                  StockIdentity{Symbol: "2317", Name: "鴻海", Market: "TSE"},
		IsDisposition:          true,
		IsAttention:            true,
		DayTradingRestricted:   true,
		MarginTradingSuspended: false,
		ShortSellingSuspended:  false,
		Lineage: model.Lineage{
			Source: model.SourceTWSEAPI, SourceRole: model.SourceRoleCanonical,
			Freshness: model.FreshnessPostMarket,
		},
	}
	back := roundTrip(t, in)
	if !back.IsDisposition || !back.IsAttention || !back.DayTradingRestricted || back.MarginTradingSuspended {
		t.Errorf("round trip 不符: %+v", back)
	}
	if back.Lineage.Source != model.SourceTWSEAPI {
		t.Errorf("_lineage round trip 不符: %+v", back.Lineage)
	}
}

func TestDerivativesSnapshotRoundTrip(t *testing.T) {
	in := DerivativesSnapshot{
		Product:          "TX",
		Date:             "2026-07-31",
		PutCallRatio:     1.2,
		LargeTraderNetOI: map[string]int64{"特定法人": 12000, "一般法人": -5000},
		InstitutionalFutures: InstitutionalFlow{
			Stock:            StockIdentity{Symbol: "", Market: "TSE"},
			Date:             "2026-07-31",
			Market:           "TSE",
			ForeignNetShares: 3000,
			TrustNetShares:   500,
			DealerNetShares:  100,
			Lineage: model.Lineage{
				Source: model.SourceTAIFEXAPI, SourceRole: model.SourceRoleCanonical,
				Freshness: model.FreshnessPostMarket,
			},
		},
		Lineage: model.Lineage{
			Source: model.SourceTAIFEXAPI, SourceRole: model.SourceRoleCanonical,
			Freshness: model.FreshnessPostMarket,
		},
	}
	back := roundTrip(t, in)
	if back.Product != "TX" || back.PutCallRatio != 1.2 || back.LargeTraderNetOI["特定法人"] != 12000 {
		t.Errorf("round trip 不符: %+v", back)
	}
	if back.InstitutionalFutures.ForeignNetShares != 3000 {
		t.Errorf("institutional_futures round trip 不符: %+v", back.InstitutionalFutures)
	}
}
