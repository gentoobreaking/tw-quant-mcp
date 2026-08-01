package model

import "tw-quant-mcp/pkg/chart"

// Envelope 是統一 Response Envelope（§3.3）。所有 MCP Tool 回傳一律包裹此結構，
// raw payload 僅在內部暫存，絕不回傳給 Client。
type Envelope struct {
	Data      interface{} `json:"data"`                  // 業務資料（Normalized Model 或 []）
	Lineage   Lineage     `json:"_lineage"`              // 血統資訊（由 response shaping 注入）
	ChartMeta *chart.Meta `json:"_chart_meta,omitempty"` // 圖表渲染描述（§11），請求含 chart=true（預設）時輸出
}
