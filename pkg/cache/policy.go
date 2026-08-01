// Package cache 實作規格書 §4 三層快取策略與 §12.2 Single-flight：
// L1 Ristretto（TinyLFU，<1ms 熱資料）、L2 SQLite（WAL mode、prepared statement，
// §12.8 之 (dataset, date) 索引）、TTL 政策表（§4.2）與快取鍵設計（§4.3）。
package cache

import "time"

// 資料類別（§4.2 TTL 政策表之資料類別欄位）。各 Handler 以 WithDataset 標註。
const (
	DatasetMISSnapshot    = "mis_snapshot"    // MIS Snapshot / 即時 K 線
	DatasetDailyKLine     = "daily_kline"     // 日線/週線/月線 K 線
	DatasetInstitutional  = "institutional"   // 三大法人買賣超
	DatasetMargin         = "margin"          // 融資融券
	DatasetAlertStock     = "alert_stock"     // 注意/處置股
	DatasetMonthlyRevenue = "monthly_revenue" // 月營收
	DatasetFinancials     = "financials"      // 財報三表
	DatasetMaterialNews   = "material_news"   // 重大訊息
	DatasetCalendar       = "calendar"        // 交易日曆 / 公司代碼表
	DatasetTAIFEXHistory  = "taifex_history"  // TAIFEX 歷史回溯（download 解析結果）
	DatasetForeignHold    = "foreign_holding" // 外資持股（個股歷史/類股，T011）
	DatasetWarrants       = "warrants"        // 權證每日成交（T011）
	DatasetValuation      = "valuation"       // 估值指標（PE/PB/殖利率，T014）
	DatasetESG            = "esg"             // ESG 揭露/公司治理（T014）
	DatasetExDivCalendar  = "ex_div_calendar" // 除權除息行事曆（T014，§4.1 明列 L2 資格）
	DatasetDividend       = "dividend"        // 股利分派資料（t187ap45_L，T014）
)

// 特殊 TTL 值（§4.2）。
const (
	// ForeverTTL 表示永久快取（TAIFEX 歷史回溯，存 L2；§4.2「永久」欄）。
	ForeverTTL time.Duration = 0

	// PostUntilNext8AM 標記盤後 TTL 為「至隔日 08:00」（§4.2 盤後欄）。
	PostUntilNext8AM time.Duration = -1 * time.Hour

	// PostNotCached 標記該資料類別盤後不查詢、不快取（如 MIS Snapshot，§4.2「—」欄）。
	PostNotCached time.Duration = -2 * time.Hour
)

// Policy 是 §4.2 政策表中單一資料類別之 TTL 政策。
type Policy struct {
	Intraday time.Duration // 盤中 TTL
	Post     time.Duration // 盤後（16:30 後）TTL；PostUntilNext8AM / PostNotCached / ForeverTTL 為特殊值
	AllowL2  bool          // 是否允許落入 L2（§4.1 L2 用途）
}

// policies 為 §4.2 快取 TTL 政策表之唯一真值來源。此表新增/修改一律同步本 map。
var policies = map[string]Policy{
	DatasetMISSnapshot:    {Intraday: 4 * time.Second, Post: PostNotCached, AllowL2: false},
	DatasetDailyKLine:     {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetInstitutional:  {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetMargin:         {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetAlertStock:     {Intraday: 30 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetMonthlyRevenue: {Intraday: 12 * time.Hour, Post: 12 * time.Hour, AllowL2: true},
	DatasetFinancials:     {Intraday: 12 * time.Hour, Post: 12 * time.Hour, AllowL2: true},
	DatasetMaterialNews:   {Intraday: 5 * time.Minute, Post: 5 * time.Minute, AllowL2: true},
	DatasetCalendar:       {Intraday: 24 * time.Hour, Post: 24 * time.Hour, AllowL2: true},
	DatasetTAIFEXHistory:  {Intraday: ForeverTTL, Post: ForeverTTL, AllowL2: true},
	DatasetForeignHold:    {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetWarrants:       {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	// T014：估值/除權息行事曆為日級資料（同 daily_kline 盤後 TTL）；行事曆
	// L2 持久（§4.2「L2 持久」），L1 24h 內重取以免遺漏新公告事件。
	DatasetValuation:     {Intraday: 60 * time.Second, Post: PostUntilNext8AM, AllowL2: true},
	DatasetESG:           {Intraday: 24 * time.Hour, Post: 24 * time.Hour, AllowL2: true},
	DatasetExDivCalendar: {Intraday: 24 * time.Hour, Post: 24 * time.Hour, AllowL2: true},
	DatasetDividend:      {Intraday: 12 * time.Hour, Post: 12 * time.Hour, AllowL2: true},
}

// TTLFor 依 §4.2 政策表與目前時間回傳資料類別之有效期間。
// now 應為台北時間（model.TaipeiNow()）；盤後定義為 16:30（含）之後。
// 回傳之 cacheable=false 表示該時段不應快取（未登錄資料類別，或如 MIS 之盤後「不查」）。
func TTLFor(dataset string, now time.Time) (ttl time.Duration, cacheable bool) {
	p, ok := policies[dataset]
	if !ok {
		return 0, false
	}
	if !postMarket(now) {
		return p.Intraday, true
	}
	switch p.Post {
	case PostUntilNext8AM:
		return nextDay8AM(now).Sub(now), true
	case PostNotCached:
		return 0, false
	case ForeverTTL:
		return ForeverTTL, true
	default:
		return p.Post, true
	}
}

// AllowL2 回傳資料類別是否允許落入 L2（§4.1：L2 僅收 TAIFEX 歷史回溯、
// 日線盤後快照、交易日曆、除權息行事曆、公司代碼表；未登錄視同不允許）。
func AllowL2(dataset string) bool {
	p, ok := policies[dataset]
	return ok && p.AllowL2
}

// postMarket 判定 16:30（含）後是否為盤後（§4.2 表頭「盤後（16:30 後）」）。
func postMarket(now time.Time) bool {
	return now.Hour()*60+now.Minute() >= 16*60+30
}

// nextDay8AM 回傳隔日 08:00（「至隔日 08:00 永久」之到期點；§4.2）。
func nextDay8AM(now time.Time) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day()+1, 8, 0, 0, 0, now.Location())
}
