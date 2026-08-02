package model

import (
	"encoding/json"

	"tw-quant-mcp/pkg/chart"
)

// Lineages 為 `_lineage` 之 union 型別（v2.1 §4 設計規則 2）：
// 預設以內嵌 Lineage 輸出單一物件；多來源聚合（如 trend composite 同時
// 使用 TWSE Web API 與 MOPS）時以 Multi 輸出 `[]Lineage` 陣列，逐一標註
// 每個子資料的來源與新鮮度。MarshalJSON 依 Multi 是否非空決定輸出型別。
type Lineages struct {
	Lineage
	// Multi 非 nil 時輸出為陣列（多來源聚合）；nil 時輸出單一 Lineage。
	Multi []Lineage
}

// MarshalJSON 實作 union 輸出：Multi 非 nil → 陣列；否則單一物件。
func (ls Lineages) MarshalJSON() ([]byte, error) {
	if ls.Multi != nil {
		return json.Marshal(ls.Multi)
	}
	return json.Marshal(ls.Lineage)
}

// UnmarshalJSON 依輸入型別填入：物件 → 內嵌 Lineage；陣列 → Multi。
func (ls *Lineages) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '[' {
		var multi []Lineage
		if err := json.Unmarshal(b, &multi); err != nil {
			return err
		}
		ls.Multi = multi
		ls.Lineage = Lineage{}
		return nil
	}
	var single Lineage
	if err := json.Unmarshal(b, &single); err != nil {
		return err
	}
	*ls = Lineages{Lineage: single}
	return nil
}

// First 回傳第一筆血統（單一或多來源陣列之首；空值時回傳零值 Lineage）。
func (ls Lineages) First() Lineage {
	if len(ls.Multi) > 0 {
		return ls.Multi[0]
	}
	return ls.Lineage
}

// Len 回傳血統筆數（Multi 非 nil 時為陣列長度，否則為 1）。
func (ls Lineages) Len() int {
	if ls.Multi != nil {
		return len(ls.Multi)
	}
	return 1
}

// Envelope 是統一 Response Envelope（§3.3）。所有 MCP Tool 回傳一律包裹此結構，
// raw payload 僅在內部暫存，絕不回傳給 Client。
type Envelope struct {
	Data      interface{} `json:"data"`                  // 業務資料（Normalized Model 或 []）
	Lineage   Lineages    `json:"_lineage"`              // 血統資訊（由 response shaping 注入；多來源時為 []Lineage）
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
