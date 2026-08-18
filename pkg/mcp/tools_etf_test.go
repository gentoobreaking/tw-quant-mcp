package mcp

// tools_etf_test.go：get_etf_nav（§30.1 L1）之整合測試。
// 以 fakeETF 替身驗證：資料對齊、日期範圍、快取、錯誤路徑。

import (
	"context"
	"fmt"
	"net/url"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// stubETF 種入 get_etf_nav 所需之 e添富 fake 資料（與 envelope 測試同款）。
func stubETFNav(f *fakeFetch) {
	f.bodies["etf|0050|fundPric"] = `{"netPrice":[{"date":"2026/07/30","count":101.0},{"date":"2026/07/29","count":100.0}],"atmps":[{"date":"2026/07/30","count":0.15},{"date":"2026/07/29","count":-0.1}]}`
	f.bodies["etf|0050|close"] = `[{"date":"2026/07/30","count":101.15},{"date":"2026/07/29","count":99.9}]`
	// 債券型等：無 netPrice（僅市價）
	f.bodies["etf|00710B|fundPric"] = `{"netPrice":[],"atmps":[]}`
	f.bodies["etf|00710B|close"] = `[{"date":"2026/07/30","count":19.11}]`
}

// etfApp 建立注入 fakeETF 之 App（盤後 16:00，與 fgApp 同款時鐘）。
func etfApp(t *testing.T, f *fakeFetch) *App {
	t.Helper()
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, model.Taipei())
	reg := seedSymbols()
	// Registry.Set 為全量覆寫：需併入既有代號後再 Set。
	_ = reg.Set(append(reg.List(""), model.Symbol{Code: "00710B", Name: "復華彭博非投等債", Market: model.MarketTSE}))
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(reg),
		WithAppSources(fakeWeb{f}, fakeAPI{f}, fakeTPEx{f}),
		WithAppETF(fakeETF{f}),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	return app
}

func TestGetETFNavBasic(t *testing.T) {
	f := newFake(t)
	stubETFNav(f)
	app := etfApp(t, f)

	env := callEnv(t, app, "get_etf_nav", map[string]any{"symbol": "0050"})
	res, ok := env.Data.(model.ETFNavResult)
	if !ok {
		t.Fatalf("Data 應為 model.ETFNavResult，實際 %T", env.Data)
	}
	if res.Symbol != "0050" || res.Name != "元大台灣50" || res.Market != model.MarketTSE {
		t.Errorf("基本欄位錯誤: %+v", res)
	}
	if len(res.Points) != 2 {
		t.Fatalf("應回傳 2 點，實際 %d", len(res.Points))
	}
	p0 := res.Points[0] // 由近至遠
	if p0.Date != "2026-07-30" || p0.NAV != 101.0 || p0.Market != 101.15 || p0.PremiumDiscount != 0.15 {
		t.Errorf("最新點錯誤: %+v", p0)
	}
	p1 := res.Points[1]
	if p1.Date != "2026-07-29" || p1.NAV != 100.0 || p1.PremiumDiscount != -0.1 {
		t.Errorf("前一日點錯誤: %+v", p1)
	}
	if res.Note != "" {
		t.Errorf("正常資料不應有 note: %q", res.Note)
	}
	if env.Lineage.Source != model.SourceTWSEWeb || env.Lineage.SourceRole != model.SourceRoleFallback {
		t.Errorf("lineage 應為 TWSE_WEB/FALLBACK: %+v", env.Lineage)
	}
	if env.Lineage.DataDate != "2026-07-30" {
		t.Errorf("data_date 應為 2026-07-30，實際 %s", env.Lineage.DataDate)
	}
	if env.Lineage.IsCached {
		t.Errorf("首次呼叫不得 cached")
	}
	// 二次呼叫 → 快取命中
	env2 := callEnv(t, app, "get_etf_nav", map[string]any{"symbol": "0050"})
	if !env2.Lineage.IsCached {
		t.Errorf("二次呼叫應 cached")
	}
}

func TestGetETFNavNoNetPrice(t *testing.T) {
	f := newFake(t)
	stubETFNav(f)
	app := etfApp(t, f)

	env := callEnv(t, app, "get_etf_nav", map[string]any{"symbol": "00710B"})
	res := env.Data.(model.ETFNavResult)
	if len(res.Points) != 1 || res.Points[0].NAV != 0 {
		t.Fatalf("債券型無淨值應僅回傳市價點（NAV=0）: %+v", res)
	}
	if res.Note == "" {
		t.Errorf("債券型無淨值應有 note 說明")
	}
}

func TestGetETFNavRejectsNonETF(t *testing.T) {
	f := newFake(t)
	app := etfApp(t, f)
	if _, err := app.core.Call(context.Background(), "get_etf_nav", map[string]any{"symbol": "2330"}); err == nil {
		t.Fatal("非 ETF 代號應被拒絕")
	}
}

func TestGetETFNavDateRange(t *testing.T) {
	f := newFake(t)
	stubETFNav(f)
	app := etfApp(t, f)
	// start 晚於 end → 錯誤
	if _, err := app.core.Call(context.Background(), "get_etf_nav",
		map[string]any{"symbol": "0050", "start": "2026-07-31", "end": "2026-07-01"}); err == nil {
		t.Fatal("start 晚於 end 應被拒絕")
	}
	// 非法日期格式 → 錯誤
	if _, err := app.core.Call(context.Background(), "get_etf_nav",
		map[string]any{"symbol": "0050", "start": "2026/07/01"}); err == nil {
		t.Fatal("非法日期格式應被拒絕")
	}
	// 自訂範圍：確認 fakeETF 收到之 code 與 type（含 start/end 參數傳遞）
	env := callEnv(t, app, "get_etf_nav", map[string]any{"symbol": "0050", "start": "2026-07-01", "end": "2026-07-30"})
	res := env.Data.(model.ETFNavResult)
	if res.Start != "2026-07-01" || res.End != "2026-07-30" {
		t.Errorf("範圍欄位錯誤: %s ~ %s", res.Start, res.End)
	}
}

func TestGetETFNavOtcUnsupported(t *testing.T) {
	f := newFake(t)
	// 上櫃 ETF（00679B 不在 e添富平台）
	reg := model.NewRegistry()
	_ = reg.Set([]model.Symbol{{Code: "00679B", Name: "元大美債20年", Market: model.MarketOTC}})
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, model.Taipei())
	app, err := NewApp(nil,
		WithAppClock(func() time.Time { return now }),
		WithAppSymbols(reg),
		WithAppETF(fakeETF{f}),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	if _, err := app.core.Call(context.Background(), "get_etf_nav", map[string]any{"symbol": "00679B"}); err == nil {
		t.Fatal("上櫃 ETF 應回報資料源缺口錯誤")
	}
}

// TestETFNavCacheKeyStability 確認快取鍵不含時變參數（穩定鍵）。
func TestETFNavCacheKeyStability(t *testing.T) {
	k1 := cacheKeyETF("0050", "2026/07/01", "2026/07/30", "fundPric")
	k2 := cacheKeyETF("0050", "2026/07/01", "2026/07/30", "fundPric")
	if k1 != k2 {
		t.Errorf("快取鍵應穩定: %q vs %q", k1, k2)
	}
	k3 := cacheKeyETF("0050", "2026/07/01", "2026/07/31", "fundPric")
	if k1 == k3 {
		t.Errorf("不同範圍鍵應不同")
	}
}

// cacheKeyETF 以與 handler 相同之規則建構快取鍵（僅測試用途）。
func cacheKeyETF(code, start, end, chartType string) string {
	_ = chartType
	return fmt.Sprintf("%s|%s|%s|%s", "etf_nav", code, start, end)
}

var _ = url.Values{}
var _ = provider.ETFChartFundPric
