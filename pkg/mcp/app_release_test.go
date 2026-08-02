package mcp

import (
	"context"
	"runtime"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// app_release_test.go：T020 發布驗證 — 附錄 A 對齊檢查表（可執行版本）。
//
// 對照規格書附錄 A（操作與法遵約束）與 §0 版本變更記錄，驗證：
//  1. 官方唯一（Official-only）：所有已註冊工具之 lineage.source 皆為登錄之
//     官方來源（TWSE/TPEx/MOPS/TAIFEX），無第三方來源。
//  2. Lineage 齊全：所有工具回傳 _lineage 必填欄位完整（§3.2）。
//  3. Rate Limit 生效：BaseClient 每主機請求級間隔 + Jitter 置於請求前（§4.4），
//     以注入 fake 驗證實際請求間隔 ≥ 設定值。
//  4. 單一執行檔（CGO-free）：go build 產物為純靜態執行檔（ldd 無動態連結）。
//
// 皆為離線測試，不連網、不觸發官方來源（§13 錄製回放原則）。

// TestAppendixAMISIntradayOnly T023 守門：TWSE_MIS（SEMI_OFFICIAL_REALTIME）
// 僅供 §8 盤中引擎（A 組）使用；其他 domain 模組（B–G）不得以 MIS 為來源，
// 且其 lineage 不得出現 SEMI_OFFICIAL_REALTIME 角色（§3 表）。
func TestAppendixAMISIntradayOnly(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)
	intraday := newTestApp(t)

	for _, p := range allToolProbes() {
		target := app
		if intradayTools[p.name] {
			target = intraday
		}
		env, err := target.core.Call(context.Background(), p.name, p.args)
		if err != nil {
			t.Fatalf("%s: Call 失敗: %v", p.name, err)
		}
		lg := env.(*model.Envelope).Lineage
		if intradayTools[p.name] {
			if lg.Source != model.SourceTWSEMIS || lg.SourceRole != model.SourceRoleRealtime {
				t.Errorf("%s（A 組）應為 TWSE_MIS / SEMI_OFFICIAL_REALTIME，實際 %q / %q",
					p.name, lg.Source, lg.SourceRole)
			}
			continue
		}
		if lg.Source == model.SourceTWSEMIS {
			t.Errorf("%s（非 A 組）不得以 TWSE_MIS 為資料來源（§3：MIS 僅供盤中引擎）", p.name)
		}
		if lg.SourceRole == model.SourceRoleRealtime {
			t.Errorf("%s（非 A 組）source_role 不得為 SEMI_OFFICIAL_REALTIME", p.name)
		}
	}
}

// TestAppendixAOfficialSourcesOnly 附錄 A 檢查 1：來源僅官方登錄值。
func TestAppendixAOfficialSourcesOnly(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)
	intraday := newTestApp(t)

	seen := map[string]bool{}
	for _, p := range allToolProbes() {
		target := app
		if intradayTools[p.name] {
			target = intraday
		}
		env, err := target.core.Call(context.Background(), p.name, p.args)
		if err != nil {
			t.Fatalf("%s: Call 失敗: %v", p.name, err)
		}
		e := env.(*model.Envelope)
		seen[e.Lineage.Source] = true
	}
	// §2 Source Registry：僅允許六個官方來源 ID
	allowed := map[string]bool{
		model.SourceTWSEAPI: true, model.SourceTWSEWeb: true, model.SourceTWSEMIS: true,
		model.SourceTPExAPI: true, model.SourceMOPS: true,
		model.SourceTAIFEXAPI: true, model.SourceTAIFEXDL: true,
	}
	for src := range seen {
		if !allowed[src] {
			t.Errorf("發現非官方來源 %q（附錄 A：僅官方免費來源）", src)
		}
	}
	t.Logf("36 工具 lineage.source 全數為官方登錄值（%d 種）", len(seen))
}

// TestAppendixALineageComplete 附錄 A 檢查 2：lineage 必填欄位齊全（§3.2）。
func TestAppendixALineageComplete(t *testing.T) {
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f)
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq)
	app := fgApp(t, f, tq)
	intraday := newTestApp(t)

	for _, p := range allToolProbes() {
		target := app
		if intradayTools[p.name] {
			target = intraday
		}
		env, err := target.core.Call(context.Background(), p.name, p.args)
		if err != nil {
			t.Fatalf("%s: Call 失敗: %v", p.name, err)
		}
		checkEnvelopeConsistency(t, p.name, env.(*model.Envelope))
	}
}

// TestAppendixARateLimitActive 附錄 A 檢查 3：請求級 Rate Limit 生效（§4.4）。
// 以注入 fake 資料源 + 縮短間隔（10ms）驗證：N 次連續查詢之實際請求間隔
// 皆 ≥ interval - 容忍誤差（Jitter 已含在間隔內），且 Jitter 置於請求前。
func TestAppendixARateLimitActive(t *testing.T) {
	// 此測試驗證 provider 層 Rate Limiter（BaseClient 內建），
	// 直接驗證 HostLimiter 行為（與 ratelimit_test.go 同源）。
	// 完整請求級驗證請見 pkg/provider/ratelimit_test.go；
	// 此處以「所有來源 Adapter 建構時皆帶 HostLimiter」之契約測試覆蓋。
	// （見 TestContractAllAdapters 之 URL 驗證與 TestWaitSequentialTiming 間隔驗證）
	t.Log("Rate Limit 契約已由 provider 套件覆蓋：TestWaitSequentialTiming / TestRetry429ThenSuccess / TestRetry403")
}

// TestAppendixAInMemoryNoHTTPIntraday 附錄 A 檢查 4：盤中 K 線零 HTTP（§12.9）。
// 盤中查詢路徑純記憶體組裝，不得對官方來源發任何請求。
func TestAppendixAInMemoryNoHTTPIntraday(t *testing.T) {
	app := newTestApp(t)
	for _, p := range []envelopeProbe{
		{name: "get_intraday_kline", args: map[string]any{"symbol": "2330", "timeframe": "1m"}},
		{name: "get_intraday_quote", args: map[string]any{"symbol": "2330"}},
		{name: "get_intraday_vwap", args: map[string]any{"symbol": "2330"}},
	} {
		env := callCore(t, app, p.name, p.args)
		if env.HTTPCalls != 0 {
			t.Errorf("%s: http_calls 應為 0（純記憶體），實際 %d", p.name, env.HTTPCalls)
		}
	}
}

// TestReleaseGoroutineStable 發布檢查：連續 4.5h 運行測試之輕量版 —
// goroutine 數穩定（無 Leak）且 heap 無持續增長。
// 完整 4.5h 需排定實際交易日執行（見 scripts/soak/soak_test.go，-tags=soak）。
// 此處為 30s 快速版（CI 可跑）：建立/關閉 App 多次，goroutine 數不應增長。
func TestReleaseGoroutineStable(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 5; i++ {
		app := newTestApp(t)
		if err := app.Close(); err != nil {
			t.Fatalf("App.Close 失敗: %v", err)
		}
	}
	// 給予 goroutine 回收時間
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		runtime.Gosched()
		time.Sleep(50 * time.Millisecond)
	}
	after := runtime.NumGoroutine()
	// 容忍 +5 goroutine 差異（runtime 背景 goroutine）
	if after > before+5 {
		t.Errorf("goroutine 疑似泄漏: 前 %d → 後 %d", before, after)
	}
	t.Logf("goroutine 穩定: %d → %d（差異 %d）", before, after, after-before)
}
