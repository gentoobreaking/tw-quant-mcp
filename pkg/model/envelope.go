package model

import "tw-quant-mcp/pkg/chart"

// Envelope 是統一 Response Envelope（§3.3）。所有 MCP Tool 回傳一律包裹此結構，
// raw payload 僅在內部暫存，絕不回傳給 Client。
type Envelope struct {
	Data      interface{} `json:"data"`                  // 業務資料（Normalized Model 或 []）
	Lineage   Lineage     `json:"_lineage"`              // 血統資訊（由 response shaping 注入）
	ChartMeta *chart.Meta `json:"_chart_meta,omitempty"` // 圖表渲染描述（§11），請求含 chart=true（預設）時輸出
	// HTTPCalls 為本次查詢實際對上游之 HTTP 請求數（§12.9 效能 instrumentation：
	// 盤中 K 線查詢路徑零 HTTP，此欄位應為 0；miss 時等於上游呼叫次數）。
	HTTPCalls int64 `json:"http_calls"`
	// Disclaimer 為附錄 A 法遵免責欄位：僅供研究參考，不構成投資建議。
	// 正式 Response 一律附加（固定文字，omitempty 僅為序列化最小化之保險）。
	Disclaimer string `json:"disclaimer,omitempty"`
}

// DisclaimerText 為附錄 A 之統一免責聲明（所有 Tool 回傳附加）。
const DisclaimerText = "僅供研究參考，不構成投資建議"
