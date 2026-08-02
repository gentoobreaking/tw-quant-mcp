// Package model 定義 tw-quant-mcp 對外之統一資料契約（規格書 §3、§5）。
// 此為全專案共用契約：欄位一經定義不可隨意變更，所有 Handler 不得直接回傳 raw payload。
package model

import "encoding/json"

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

// SourceRole 對應 v2.1 §3 的三種資料來源角色（取代 v1.3 的
// canonical/helper/fallback 字串）：所有 Lineage 之 source_role 僅允許此三值。
type SourceRole string

const (
	SourceRoleCanonical SourceRole = "CANONICAL"              // 正式公開、有文件、JSON 結構化的官方 API（production 主路徑）
	SourceRoleRealtime  SourceRole = "SEMI_OFFICIAL_REALTIME" // 官方網域提供、未列入 OpenAPI 文件目錄之即時端點（如 TWSE MIS）
	SourceRoleFallback  SourceRole = "FALLBACK"               // CANONICAL 端點在特定維度（如歷史深度）不足時之官方替代管道
)

// DataGrade 標註該筆資料背後 Tool 目前的成熟度（v2.1 §4、§9、§13），
// 呼應 twmarketdata.com 之 available-now / preview / not-yet-available 分級。
type DataGrade string

const (
	GradeAvailable   DataGrade = "AVAILABLE"         // 已上線，可直接依賴
	GradePreview     DataGrade = "PREVIEW"           // 已可查詢，但欄位/準確度仍可能調整
	GradeUnavailable DataGrade = "NOT_YET_AVAILABLE" // Roadmap 中，尚未實作
)

// 新鮮度分級（v2.1 §4）。僅允許下列五值；STALE_FALLBACK 供
// stale-if-error（快取過期回退）情境使用（§5 失敗處理、T024）。
const (
	FreshnessRealtimeIntraday = "REALTIME_INTRADAY"
	FreshnessPostMarket       = "POST_MARKET"
	FreshnessMonthly          = "MONTHLY"
	FreshnessQuarterly        = "QUARTERLY"
	FreshnessStaleFallback    = "STALE_FALLBACK"
)

// ValidFreshness 檢查新鮮度分級是否為 v2.1 §4 允許之五值。
func ValidFreshness(f string) bool {
	switch f {
	case FreshnessRealtimeIntraday, FreshnessPostMarket, FreshnessMonthly,
		FreshnessQuarterly, FreshnessStaleFallback:
		return true
	}
	return false
}

// Lineage 是每筆回傳資料之血統資訊（v2.1 §4），由 response shaping 階段統一注入。
// 所有 `_lineage` 之建立與注入皆集中於 model 層，任何 Handler 不得自行偽造。
//
// 欄位決策（T021，2026-08-01 已與使用者確認）：`derived_from` / `cache_ttl` /
// `source_url` 三個欄位**保留於 struct 但標 `json:"-"`**——正式 JSON 不輸出，
// 僅 debug/log 模式（見 DebugJSON）可輸出；既有內部組裝點（設定父資料集 /
// TTL / 來源 URL 之處）仍可直接寫入，序列化輸出不受影響。
type Lineage struct {
	Source      string     `json:"source"`                  // 來源機構 ID，見上方 Source* 常數
	SourceRole  SourceRole `json:"source_role"`             // CANONICAL | SEMI_OFFICIAL_REALTIME | FALLBACK
	DerivedFrom []string   `json:"-"`                       // 派生資料的父資料集 ID（僅 debug/log 模式輸出，§附錄A）
	FetchedAt   TaipeiTime `json:"fetched_at"`              // 抓取時間（RFC3339，Asia/Taipei）
	DataDate    string     `json:"data_date"`               // 資料歸屬日期 YYYY-MM-DD
	Freshness   string     `json:"freshness"`               // REALTIME_INTRADAY | POST_MARKET | MONTHLY | QUARTERLY | STALE_FALLBACK
	SamplingSec int        `json:"sampling_sec"`            // 採樣間隔（秒）；非採樣資料為 0
	IsCached    bool       `json:"is_cached"`               // 是否命中快取
	CacheTTL    int        `json:"-"`                       // 本次快取 TTL（秒，僅 debug/log 模式輸出）
	CacheAgeSec int64      `json:"cache_age_sec,omitempty"` // 若命中快取，資料已存活多久（秒）
	LatencyMS   int64      `json:"latency_ms"`              // 端到端耗時（含 cache 命中時仍計算）
	SourceURL   string     `json:"-"`                       // 實際請求的官方 URL（僅 debug/log 模式輸出，§附錄A）
	Grade       DataGrade  `json:"grade,omitempty"`         // Tool 成熟度分級（AVAILABLE | PREVIEW | NOT_YET_AVAILABLE）
}

// DebugJSON 輸出含 derived_from / cache_ttl / source_url 之完整血統資訊，
// 僅供 debug / log 模式使用（§附錄A：source_url 僅 debug 模式輸出）。
// 正式 JSON 一律不含此三欄。
func (l Lineage) DebugJSON() ([]byte, error) {
	type lineageDebug struct {
		Source      string     `json:"source"`
		SourceRole  SourceRole `json:"source_role"`
		DerivedFrom []string   `json:"derived_from,omitempty"`
		FetchedAt   TaipeiTime `json:"fetched_at"`
		DataDate    string     `json:"data_date"`
		Freshness   string     `json:"freshness"`
		SamplingSec int        `json:"sampling_sec"`
		IsCached    bool       `json:"is_cached"`
		CacheTTL    int        `json:"cache_ttl"`
		CacheAgeSec int64      `json:"cache_age_sec,omitempty"`
		LatencyMS   int64      `json:"latency_ms"`
		SourceURL   string     `json:"source_url,omitempty"`
		Grade       DataGrade  `json:"grade,omitempty"`
	}
	return json.Marshal(lineageDebug{
		Source:      l.Source,
		SourceRole:  l.SourceRole,
		DerivedFrom: l.DerivedFrom,
		FetchedAt:   l.FetchedAt,
		DataDate:    l.DataDate,
		Freshness:   l.Freshness,
		SamplingSec: l.SamplingSec,
		IsCached:    l.IsCached,
		CacheTTL:    l.CacheTTL,
		CacheAgeSec: l.CacheAgeSec,
		LatencyMS:   l.LatencyMS,
		SourceURL:   l.SourceURL,
		Grade:       l.Grade,
	})
}
