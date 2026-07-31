// Package model 定義 tw-quant-mcp 對外之統一資料契約（規格書 §3、§5）。
// 此為全專案共用契約：欄位一經定義不可隨意變更，所有 Handler 不得直接回傳 raw payload。
package model

// 資料來源 ID（§2 資料來源登錄表之 ID，對應 Lineage.Source）。
const (
	SourceTWSEAPI   = "TWSE_API"
	SourceTWSEWeb   = "TWSE_WEB"
	SourceTWSEMIS   = "TWSE_MIS"
	SourceTPExAPI   = "TPEX_API"
	SourceMOPS      = "MOPS"
	SourceTAIFEXAPI = "TAIFEX_API"
	SourceTAIFEXDL  = "TAIFEX_DL"
)

// 來源角色（§2.1）。
const (
	SourceRoleCanonical = "canonical" // 直接來自官方機構之原始資料，唯一真值來源
	SourceRoleHelper    = "helper"    // 由 canonical 派生之計算結果，需標明 derived_from
	SourceRoleFallback  = "fallback"  // 官方 A 來源缺資料時改用官方 B 來源
)

// 新鮮度分級（§3.2）。僅允許下列三值。
const (
	FreshnessRealtimeIntraday = "REALTIME_INTRADAY"
	FreshnessPostMarketToday  = "POST_MARKET_TODAY"
	FreshnessHistorical       = "HISTORICAL"
)

// ValidFreshness 檢查新鮮度分級是否為 §3.2 允許之三值。
func ValidFreshness(f string) bool {
	switch f {
	case FreshnessRealtimeIntraday, FreshnessPostMarketToday, FreshnessHistorical:
		return true
	}
	return false
}

// Lineage 是每筆回傳資料之血統資訊（§3.2），由 response shaping 階段統一注入。
// 所有 `_lineage` 之建立與注入皆集中於 model 層，任何 Handler 不得自行偽造。
type Lineage struct {
	Source      string     `json:"source"`                 // 來源機構 ID，見上方 Source* 常數
	SourceRole  string     `json:"source_role"`            // canonical | helper | fallback
	DerivedFrom []string   `json:"derived_from,omitempty"` // helper 資料的父資料集 ID
	FetchedAt   TaipeiTime `json:"fetched_at"`             // 抓取時間（RFC3339，Asia/Taipei）
	DataDate    string     `json:"data_date"`              // 資料歸屬日期 YYYY-MM-DD
	Freshness   string     `json:"freshness"`              // REALTIME_INTRADAY | POST_MARKET_TODAY | HISTORICAL
	SamplingSec int        `json:"sampling_sec"`           // 採樣間隔（秒）；非採樣資料為 0
	IsCached    bool       `json:"is_cached"`              // 是否命中快取
	CacheTTL    int        `json:"cache_ttl"`              // 本次快取 TTL（秒）
	LatencyMS   int64      `json:"latency_ms"`             // 端到端耗時（含 cache 命中時仍計算）
	SourceURL   string     `json:"source_url,omitempty"`   // 實際請求的官方 URL（僅 debug/log 模式輸出，§附錄A）
}
