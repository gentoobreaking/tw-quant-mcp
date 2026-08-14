package mcp

// tools_trend_test.go：T029 驗收 — get_stock_trend_composite（v2.1 §9.1
// 唯一 C 組缺口）。驗證：
//  1. 跨來源聚合：技術面（TWSE Web 日K）+ 基本面（估值 + MOPS EPS YoY）
//     + 籌碼面（法人）組合正確；
//  2. _lineage 為 []Lineage 陣列（v2.1 §4 設計規則 2），多來源逐一標註；
//  3. Grade 標註：新工具 PREVIEW、既有工具 AVAILABLE；
//  4. horizon 參數（short/mid/long）與預設值；
//  5. OTC 標的技術面從缺（note 語意）不影響其餘面向。

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/model/domain"
)

// TestTrendCompositeTSE 上市標的（2330）short horizon 完整研判。
func TestTrendCompositeTSE(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	stubTrendTSE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_stock_trend_composite", map[string]any{"symbol": "2330", "horizon": "short"})
	tc, ok := env.Data.(domain.TrendComposite)
	if !ok {
		t.Fatalf("Data 應為 domain.TrendComposite，實際 %T", env.Data)
	}
	if tc.Stock.Symbol != "2330" || tc.Stock.Market != model.MarketTSE {
		t.Errorf("stock 身分錯誤: %+v", tc.Stock)
	}
	if tc.Horizon != "short" {
		t.Errorf("horizon 錯誤: %q", tc.Horizon)
	}
	// 技術面：MA5/MA20/RSI14 皆 > 0（short 窗口僅 1 月 K 線，MA60 容許 0）
	if tc.Technical.MA5 <= 0 || tc.Technical.MA20 <= 0 {
		t.Errorf("技術面 MA5/MA20 應全 > 0: %+v", tc.Technical)
	}
	if tc.Technical.RSI14 < 0 || tc.Technical.RSI14 > 100 {
		t.Errorf("RSI14 應在 0-100: %+v", tc.Technical)
	}
	switch tc.Technical.TrendSignal {
	case "BULLISH", "BEARISH", "NEUTRAL":
	default:
		t.Errorf("trend_signal 非法: %q", tc.Technical.TrendSignal)
	}
	// 基本面：PE/PB 來自 stub（pe=20, pb=5.2, yield=2.1）；EPS YoY=(14.5-13)/13
	if tc.Fundamental.PE != 20 || tc.Fundamental.PB != 5.2 {
		t.Errorf("估值錯誤: %+v", tc.Fundamental)
	}
	if tc.Fundamental.DividendYieldPct != 2.1 {
		t.Errorf("殖利率錯誤: %+v", tc.Fundamental)
	}
	if tc.Fundamental.EPSGrowthYoYPct < 10 || tc.Fundamental.EPSGrowthYoYPct > 12 {
		t.Errorf("EPS YoY 應約 11.5%%，實際 %v", tc.Fundamental.EPSGrowthYoYPct)
	}
	// 籌碼面：short 回溯自前一交易日（7/29）起 5 日：29/28/27/24/23
	// 各外資 200（stubTrendTSE）→ 1000；投信 50 → 250
	if tc.Chip.ForeignNetShares5D != 1000 {
		t.Errorf("外資 5 日淨買賣錯誤: %d", tc.Chip.ForeignNetShares5D)
	}
	if tc.Chip.TrustNetShares5D != 250 {
		t.Errorf("投信 5 日淨買賣錯誤: %d", tc.Chip.TrustNetShares5D)
	}
	// _lineage 陣列：技術/估值/EPS/籌碼 ≥ 3 筆
	if len(tc.Lineage) < 3 {
		t.Fatalf("_lineage 應 ≥ 3 筆，實際 %d: %+v", len(tc.Lineage), tc.Lineage)
	}
	// Envelope 層級：Multi lineage 陣列 + Grade PREVIEW
	if len(env.Lineage.Multi) != len(tc.Lineage) {
		t.Errorf("Envelope Multi lineage 數量應與 Data._lineage 一致: %d vs %d", len(env.Lineage.Multi), len(tc.Lineage))
	}
	for i, sub := range env.Lineage.Multi {
		if sub.Grade != model.GradePreview {
			t.Errorf("lineage[%d] grade 應為 PREVIEW（新工具），實際 %q", i, sub.Grade)
		}
		if sub.FetchedAt.Time.IsZero() {
			t.Errorf("lineage[%d] fetched_at 不得為零值", i)
		}
	}
	if env.ChartMeta == nil || env.ChartMeta.RecommendedType != "line" {
		t.Errorf("_chart_meta 應為 line 型別，實際 %+v", env.ChartMeta)
	}
}

// TestTrendCompositeDefaultHorizon 省略 horizon → 預設 mid。
func TestTrendCompositeDefaultHorizon(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	stubTrendTSE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_stock_trend_composite", map[string]any{"symbol": "2330"})
	tc := env.Data.(domain.TrendComposite)
	if tc.Horizon != "mid" {
		t.Errorf("預設 horizon 應為 mid，實際 %q", tc.Horizon)
	}
}

// TestTrendCompositeInvalidHorizon 非法 horizon → 錯誤。
func TestTrendCompositeInvalidHorizon(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	app := deApp(t, f)
	_, err := app.core.Call(context.Background(), "get_stock_trend_composite",
		map[string]any{"symbol": "2330", "horizon": "weekly"})
	if err == nil {
		t.Fatal("非法 horizon 應回傳錯誤")
	}
}

// TestTrendCompositeOTC 上櫃標的（6147）：技術面從缺（0 值），
// 基本面/籌碼面正常（TPEx 估值 + TPEx 法人）。
func TestTrendCompositeOTC(t *testing.T) {
	f := newFake(t)
	stubDE(f) // 含 pe_valuation（6147）與 income_summary（6147 無 → EPS 選填略過）
	f.stub("institutional", url.Values{"date": {"20260730"}},
		`[{"code":"6147","name":"頎邦","foreign_net":1200,"investment_net":300,"dealer_net":-100,"total_net":1400}]`)
	app := deApp(t, f)

	env := callEnv(t, app, "get_stock_trend_composite", map[string]any{"symbol": "6147", "horizon": "short"})
	tc := env.Data.(domain.TrendComposite)
	if tc.Stock.Market != model.MarketOTC {
		t.Errorf("market 錯誤: %q", tc.Stock.Market)
	}
	if tc.Technical.MA5 != 0 || tc.Technical.RSI14 != 0 {
		t.Errorf("上櫃技術面應從缺（0 值），實際 %+v", tc.Technical)
	}
	if tc.Fundamental.PE != 15 || tc.Fundamental.PB != 2.0 || tc.Fundamental.DividendYieldPct != 4.0 {
		t.Errorf("上櫃估值錯誤: %+v", tc.Fundamental)
	}
	if tc.Chip.ForeignNetShares5D != 1200 || tc.Chip.TrustNetShares5D != 300 {
		t.Errorf("上櫃籌碼面錯誤: %+v", tc.Chip)
	}
	// lineage 中技術面為 TPEx 來源（fallback）
	hasTPEx := false
	for _, sub := range env.Lineage.Multi {
		if sub.Source == model.SourceTPExAPI {
			hasTPEx = true
		}
	}
	if !hasTPEx {
		t.Errorf("lineage 應含 TPEx 來源，實際 %+v", env.Lineage.Multi)
	}
}

// TestDataGradeAllTools 全 38 工具 Envelope 皆有 grade 標註：
// 既有 37 工具 AVAILABLE；get_stock_trend_composite PREVIEW。
func TestDataGradeAllTools(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)
	intraday := newTestApp(t)

	preview := map[string]bool{"get_stock_trend_composite": true}
	for _, p := range allToolProbes() {
		p := p
		t.Run(p.name, func(t *testing.T) {
			target := app
			if intradayTools[p.name] {
				target = intraday
			}
			env, err := target.core.Call(context.Background(), p.name, p.args)
			if err != nil {
				t.Fatalf("%s: Call 失敗: %v", p.name, err)
			}
			e := env.(*model.Envelope)
			if preview[p.name] {
				// PREVIEW 工具：任一 lineage 皆應 PREVIEW
				if len(e.Lineage.Multi) > 0 {
					for i, sub := range e.Lineage.Multi {
						if sub.Grade != model.GradePreview {
							t.Errorf("%s[%d] grade 應為 PREVIEW，實際 %q", p.name, i, sub.Grade)
						}
					}
				} else if e.Lineage.Grade != model.GradePreview {
					t.Errorf("%s grade 應為 PREVIEW，實際 %q", p.name, e.Lineage.Grade)
				}
			} else {
				if e.Lineage.Grade != model.GradeAvailable {
					t.Errorf("%s grade 應為 AVAILABLE，實際 %q", p.name, e.Lineage.Grade)
				}
			}
		})
	}
}

// TestTrendCompositeJSONContract 驗證 Envelope JSON 輸出契約：
// _lineage 為陣列（首字元 `[`）、每筆含 grade=PREVIEW、fetched_at 非空；
// _chart_meta.recommended_type=line；data 含 stock/technical/fundamental/chip。
func TestTrendCompositeJSONContract(t *testing.T) {
	f := newFake(t)
	stubDE(f)
	stubTrendTSE(f)
	app := deApp(t, f)

	env := callEnv(t, app, "get_stock_trend_composite", map[string]any{"symbol": "2330", "horizon": "mid"})
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("Envelope JSON 序列化失敗: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Envelope JSON 解析失敗: %v", err)
	}
	// _lineage 為陣列
	lgRaw, ok := m["_lineage"]
	if !ok {
		t.Fatal("缺 _lineage")
	}
	if len(lgRaw) == 0 || lgRaw[0] != '[' {
		t.Errorf("_lineage 應為陣列（多來源聚合），實際: %s", lgRaw)
	}
	var subs []map[string]any
	if err := json.Unmarshal(lgRaw, &subs); err != nil {
		t.Fatalf("_lineage 陣列解析失敗: %v", err)
	}
	if len(subs) < 3 {
		t.Errorf("_lineage 應 ≥ 3 筆（技術/估值/EPS/籌碼），實際 %d", len(subs))
	}
	for i, s := range subs {
		if s["grade"] != "PREVIEW" {
			t.Errorf("_lineage[%d].grade 應為 PREVIEW，實際 %v", i, s["grade"])
		}
		if s["fetched_at"] == nil || s["fetched_at"] == "" {
			t.Errorf("_lineage[%d] 缺 fetched_at", i)
		}
		if s["source"] == nil {
			t.Errorf("_lineage[%d] 缺 source", i)
		}
	}
	// _chart_meta
	cmRaw, ok := m["_chart_meta"]
	if !ok {
		t.Fatal("缺 _chart_meta")
	}
	var cm map[string]any
	if err := json.Unmarshal(cmRaw, &cm); err != nil {
		t.Fatalf("_chart_meta 解析失敗: %v", err)
	}
	if cm["recommended_type"] != "line" {
		t.Errorf("_chart_meta.recommended_type 應為 line，實際 %v", cm["recommended_type"])
	}
	// data 結構
	var data map[string]any
	if err := json.Unmarshal(m["data"], &data); err != nil {
		t.Fatalf("data 解析失敗: %v", err)
	}
	for _, k := range []string{"stock", "technical", "fundamental", "chip", "horizon"} {
		if _, ok := data[k]; !ok {
			t.Errorf("data 缺 %s", k)
		}
	}
	if data["horizon"] != "mid" {
		t.Errorf("data.horizon 應為 mid，實際 %v", data["horizon"])
	}
}

// stubTrendTSE 補 get_stock_trend_composite 上市標的所需日 K / 法人回溯 stub。
func stubTrendTSE(f *fakeFetch) {
	// 日 K：近 3 個月（mid 需 MA20/MA60；short 需 MA5/MA20）
	for _, d := range []string{"20260501", "20260601", "20260701"} {
		f.stub("daily_k", url.Values{"date": {d}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", d[4:6], 0, 22)))
	}
	// 法人回溯（2026-07-30 為 stubDE 既有；補 29/28/27/24/23）
	for _, d := range []string{"20260729", "20260728", "20260727", "20260724", "20260723"} {
		f.stub("institutional", url.Values{"date": {d}},
			`[{"code":"2330","name":"台積電","foreign_buy":500,"foreign_sell":300,"foreign_net":200,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":100,"investment_sell":50,"investment_net":50}]`)
	}
}
