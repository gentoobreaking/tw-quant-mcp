//go:build live

// live_smoke_test.go：T019 驗收 #4 — Live Smoke 測試。
//
// 僅於 CI 開盤時段（週一至週五 09:00–13:30 Asia/Taipei）執行：
//   - build tag `live`（`go test -tags=live ./pkg/mcp/ -run LiveSmoke`）
//   - 以真實 App（真實資料源 + 真實快取）對官方來源發少量真請求
//     （§13：Live smoke 限開盤時段執行，避免觸發 Rate Limit）
//
// 驗證項目（最小煙霧）：
//  1. get_intraday_quote 盤中行情（TWSE_MIS，§12.9 零 HTTP 或少量）
//  2. get_stock_daily_quote 盤後日報價（TWSE_WEB，§4.4 Rate Limit 內）
//  3. get_symbol_list 代碼表（TWSE-API）
//
// 非開盤時段自動 Skip（return early，exit 0），不發任何請求。
package mcp

import (
	"context"
	"os"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// liveNow 為 Live smoke 之時鐘（真實時間）。
func liveNow() time.Time { return model.Now().Time }

// liveSessionOpen 判斷目前是否為開盤時段（09:00–13:30，週一至週五）。
func liveSessionOpen(now time.Time) bool {
	if now.Weekday() == time.Saturday || now.Weekday() == time.Sunday {
		return false
	}
	hm := now.Hour()*100 + now.Minute()
	return hm >= 900 && hm <= 1330
}

// TestLiveSmokeIntradayQuote 盤中行情 Live smoke（僅開盤時段）。
func TestLiveSmokeIntradayQuote(t *testing.T) {
	now := liveNow()
	if !liveSessionOpen(now) {
		t.Skipf("非開盤時段（%s），Live smoke 僅於 09:00–13:30 執行", now.Format("15:04"))
	}
	if os.Getenv("TW_QUANT_LIVE") == "" {
		t.Skip("未設定 TW_QUANT_LIVE=1，跳過真實請求（CI 開盤時段排程時設定）")
	}

	app := newLiveApp(t)
	defer app.Close()

	// 加入 watchlist（開盤時段 gate 通過）
	if _, err := app.Core().Call(context.Background(), "set_active_watchlist",
		map[string]any{"symbols": []any{"2330"}}); err != nil {
		t.Fatalf("set_active_watchlist 失敗: %v", err)
	}
	env, err := app.Core().Call(context.Background(), "get_intraday_quote",
		map[string]any{"symbol": "2330"})
	if err != nil {
		t.Fatalf("get_intraday_quote 失敗: %v", err)
	}
	e := env.(*model.Envelope)
	if e.Lineage.Source != model.SourceTWSEMIS {
		t.Errorf("盤中行情 source 應為 TWSE_MIS，實際 %s", e.Lineage.Source)
	}
	if e.Lineage.Freshness != model.FreshnessRealtimeIntraday {
		t.Errorf("freshness 應為 REALTIME_INTRADAY，實際 %s", e.Lineage.Freshness)
	}
	t.Logf("盤中報價 OK: %s 高=%v 低=%v（http_calls=%d）",
		e.Lineage.DataDate, quoteHigh(e), quoteLow(e), e.HTTPCalls)
}

// TestLiveSmokeDailyQuote 盤後日報價 Live smoke（僅開盤時段；盤後資料為前一交易日）。
func TestLiveSmokeDailyQuote(t *testing.T) {
	now := liveNow()
	if !liveSessionOpen(now) {
		t.Skipf("非開盤時段（%s），Live smoke 僅於 09:00–13:30 執行", now.Format("15:04"))
	}
	if os.Getenv("TW_QUANT_LIVE") == "" {
		t.Skip("未設定 TW_QUANT_LIVE=1，跳過真實請求（CI 開盤時段排程時設定）")
	}

	app := newLiveApp(t)
	defer app.Close()

	env, err := app.Core().Call(context.Background(), "get_stock_daily_quote",
		map[string]any{"symbol": "2330"})
	if err != nil {
		t.Fatalf("get_stock_daily_quote 失敗: %v", err)
	}
	e := env.(*model.Envelope)
	if e.Lineage.Source != model.SourceTWSEWeb {
		t.Errorf("日報價 source 應為 TWSE_WEB，實際 %s", e.Lineage.Source)
	}
	if e.Data == nil {
		t.Fatal("日報價 data 不得為 nil")
	}
	t.Logf("日報價 OK: %s（http_calls=%d）", e.Lineage.DataDate, e.HTTPCalls)
}

// TestLiveSmokeSymbolList 代碼表 Live smoke（TWSE-API，任何時段皆可但限開盤執行）。
func TestLiveSmokeSymbolList(t *testing.T) {
	now := liveNow()
	if !liveSessionOpen(now) {
		t.Skipf("非開盤時段（%s），Live smoke 僅於 09:00–13:30 執行", now.Format("15:04"))
	}
	if os.Getenv("TW_QUANT_LIVE") == "" {
		t.Skip("未設定 TW_QUANT_LIVE=1，跳過真實請求（CI 開盤時段排程時設定）")
	}

	app := newLiveApp(t)
	defer app.Close()

	env, err := app.Core().Call(context.Background(), "get_symbol_list", nil)
	if err != nil {
		t.Fatalf("get_symbol_list 失敗: %v", err)
	}
	e := env.(*model.Envelope)
	syms, ok := e.Data.([]model.Symbol)
	if !ok || len(syms) == 0 {
		t.Fatalf("代碼表應回傳非空 []Symbol，實際 %T", e.Data)
	}
	t.Logf("代碼表 OK: %d 檔（http_calls=%d）", len(syms), e.HTTPCalls)
}

// newLiveApp 建立真實資料源之 App（Live smoke 用，真實 HTTP + 真實快取）。
// 注意：不注入任何替身；預設快取為 L1-only（避免污染 CI 機器之 L2 DB）。
func newLiveApp(t *testing.T) *App {
	t.Helper()
	app, err := NewApp(nil)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	return app
}

// quoteHigh/quoteLow 由 Data 斷言型別取出（DailyQuote 或 Snapshot）。
func quoteHigh(e *model.Envelope) any {
	switch d := e.Data.(type) {
	case model.DailyQuote:
		return d.High
	case model.Snapshot:
		return d.High
	}
	return "?"
}

func quoteLow(e *model.Envelope) any {
	switch d := e.Data.(type) {
	case model.DailyQuote:
		return d.Low
	case model.Snapshot:
		return d.Low
	}
	return "?"
}
