// Package normalize 實作 v2.1 §6 之正規化轉換層（pkg/model/normalize/）。
//
// 本層是唯一「知道上游原始欄位」的地方：每個官方來源（§3 七個 Adapter）
// 對應一組 From<Source>() 函式，負責把上游原始格式（JSON / HTML Table /
// CSV）轉換為 pkg/model/domain 之正規化 Schema（或 §4 之 KlineBar）。
// Domain 模組與 MCP Tool Handler 只操作正規化後的型別，永遠不直接解析
// Adapter 的原始回應——欄位一旦統一，新增/替換資料來源就不會波及下游。
//
// 簽名約定：From<Source>(raw []byte)（T, error）——raw 為官方原始 payload，
// 語意參數（日期/代碼等）一律由 raw 內容自帶；轉換失敗回傳明確錯誤。
// 各來源對應之輸出 Schema 註記於函式 doc；同一來源可能對應多個 Schema
// （如 TWSE Web 同時餵養 InstitutionalFlow 與 TrendComposite 之輸入），
// 視後續任務（T023/T026/T027）逐步填實。
package normalize

import (
	"errors"
	"fmt"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/model/domain"
)

// ErrNotImplemented 表示該來源轉換路徑尚未實作（T022 先立骨架，
// 依 T023/T026/T027 依序填實）。呼叫端應視為「該 Tool 尚未上線」。
var ErrNotImplemented = errors.New("normalize: 該來源轉換路徑尚未實作")

// ---------- 已實作路徑（fixture 驅動） ----------

// FromMIS 將 MIS 原始回應（getStockInfo.jsp JSON）轉為 []KlineBar（tick bar，
// 供 §8 盤中引擎讀取）。單位換算：tv/v 張 ×1000 → 股、價格 2 位小數。
// 見 mis.go。
func FromMIS(raw []byte) ([]model.KlineBar, error) { return fromMIS(raw) }

// FromTWSEWeb 將 TWSE Web 三大法人買賣超（T86 日報 JSON）轉為
// []InstitutionalFlow（§9.7）。見 twse_web.go。
func FromTWSEWeb(raw []byte) ([]domain.InstitutionalFlow, error) { return fromTWSEWeb(raw) }

// ---------- 骨架（未實作，待 T023/T026/T027 填實） ----------

// FromTWSEOpenAPI 將 TWSE OpenAPI 原始回應轉為 domain Schema
// （預期輸出：RiskFlags 等）。尚未實作。
func FromTWSEOpenAPI(raw []byte) (domain.RiskFlags, error) {
	return domain.RiskFlags{}, fmt.Errorf("%w: FromTWSEOpenAPI（TWSE OpenAPI）", ErrNotImplemented)
}

// FromTPEx 將 TPEx OpenAPI 原始回應轉為 domain Schema
// （預期輸出：DividendRecord 等）。尚未實作。
func FromTPEx(raw []byte) (domain.DividendRecord, error) {
	return domain.DividendRecord{}, fmt.Errorf("%w: FromTPEx（TPEx OpenAPI）", ErrNotImplemented)
}

// FromMOPS 將 MOPS AJAX/HTML Table 原始回應轉為 domain Schema
// （預期輸出：FinancialHealthReport 等）。尚未實作。
func FromMOPS(raw []byte) (domain.FinancialHealthReport, error) {
	return domain.FinancialHealthReport{}, fmt.Errorf("%w: FromMOPS（MOPS）", ErrNotImplemented)
}

// FromTAIFEXOpenAPI 將 TAIFEX OpenAPI 原始回應轉為 domain Schema
// （預期輸出：DerivativesSnapshot）。尚未實作。
func FromTAIFEXOpenAPI(raw []byte) (domain.DerivativesSnapshot, error) {
	return domain.DerivativesSnapshot{}, fmt.Errorf("%w: FromTAIFEXOpenAPI（TAIFEX OpenAPI）", ErrNotImplemented)
}

// FromTAIFEXDownload 將 TAIFEX 網站下載（CSV）原始回應轉為 domain Schema
// （預期輸出：DerivativesSnapshot，歷史回溯）。尚未實作。
func FromTAIFEXDownload(raw []byte) (domain.DerivativesSnapshot, error) {
	return domain.DerivativesSnapshot{}, fmt.Errorf("%w: FromTAIFEXDownload（TAIFEX 網站下載）", ErrNotImplemented)
}
