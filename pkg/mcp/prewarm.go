package mcp

// prewarm.go 實作 §12.9 預熱排程（T018）：
//
//	交易日 08:00   → 交易日曆 + 公司代碼表（入 L2，24h TTL）
//	交易日 08:45   → 開盤前 MIS Session 重取（Cookie 維持，§8.3）
//	交易日 16:45   → 當日盤後資料（全市場彙總/名單，經既有 Handler 路徑入快取）
//
// 設計原則：
//   - 非交易日（T005 行事曆 IsTradingDay）：僅執行基礎設施預熱（交易日曆 +
//     公司代碼表，24h TTL 之官方基礎資料），跳過盤中相關階段（MIS Session、
//     盤後彙總）；
//   - 單一階段失敗僅記錄 Log，不阻塞其餘階段與服務啟動；
//   - Rate Limit 遵守（§12.9 備註）：所有請求皆經各主機 BaseClient
//     （HostLimiter 保證兩請求間隔 ≥ 對應 limiter 間隔），預熱佇列
//     間距天然合規，無需額外節流；
//   - 長駐 goroutine：Run(ctx) 隨 ctx 取消（Server lifecycle）結束。
//
// 各階段為「每日一次」：跨日重置旗標；進程於時段後啟動時立即補執行。

import (
	"context"
	"log/slog"
	"time"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// 預熱時段（§12.9）。以變數供測試覆寫。
var (
	prewarmMorningSecs = 8 * 3600         // 08:00
	prewarmPreOpenSecs = 8*3600 + 45*60   // 08:45 開盤前
	prewarmIndexSecs   = 15 * 3600        // 15:00 §10.3 Materialized Screener Index 重建
	prewarmEODSecs     = 16*3600 + 45*60  // 16:45 盤後
	prewarmTick        = 30 * time.Second // 排程器檢查間隔
)

// PrewarmScheduler 是 §12.9 預熱排程器。
type PrewarmScheduler struct {
	app  *App
	now  func() time.Time
	tick time.Duration
	log  *slog.Logger

	day         string
	morningDone bool
	preOpenDone bool
	indexDone   bool // §10.3 每日 15:00 索引重建
	eodDone     bool
}

// PrewarmOption 調整排程器行為（測試用）。
type PrewarmOption func(*PrewarmScheduler)

// WithPrewarmClock 注入時鐘（測試用）。
func WithPrewarmClock(now func() time.Time) PrewarmOption {
	return func(s *PrewarmScheduler) { s.now = now }
}

// WithPrewarmTick 覆寫檢查間隔（測試用）。
func WithPrewarmTick(d time.Duration) PrewarmOption {
	return func(s *PrewarmScheduler) { s.tick = d }
}

// WithPrewarmLogger 注入 slog logger（預設 discard）。
func WithPrewarmLogger(l *slog.Logger) PrewarmOption {
	return func(s *PrewarmScheduler) { s.log = l }
}

// NewPrewarmScheduler 建立預熱排程器（綁定 App）。
func NewPrewarmScheduler(a *App, opts ...PrewarmOption) *PrewarmScheduler {
	s := &PrewarmScheduler{
		app:  a,
		now:  a.now,
		tick: prewarmTick,
		log:  a.logger,
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Run 為長駐主迴圈：先執行一次檢查，之後每 tick 檢查（ctx 取消時結束）。
func (s *PrewarmScheduler) Run(ctx context.Context) error {
	for {
		s.TickOnce(ctx, s.now())
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.tick):
		}
	}
}

// TickOnce 執行單次階段檢查（可測試：注入任意時鐘時間）。
func (s *PrewarmScheduler) TickOnce(ctx context.Context, now time.Time) {
	day := model.FormatDate(now)
	if day != s.day {
		s.day = day
		s.morningDone, s.preOpenDone, s.indexDone, s.eodDone = false, false, false, false
	}
	// 非交易日（T005 行事曆）：僅執行基礎設施預熱（交易日曆 + 公司代碼表，
	// 兩者皆 24h TTL 之官方基礎資料，週末/假日亦應可載入供查詢使用）；
	// 跳過盤中相關階段（MIS Session 重取、當日盤後彙總）。
	if !s.app.calendar.IsTradingDay(now) {
		if !s.morningDone {
			s.morningDone = true
			s.prewarmMorning(ctx)
		}
		return
	}
	sec := now.Hour()*3600 + now.Minute()*60 + now.Second()
	if !s.morningDone && sec >= prewarmMorningSecs {
		s.morningDone = true
		s.prewarmMorning(ctx)
	}
	if !s.preOpenDone && sec >= prewarmPreOpenSecs {
		s.preOpenDone = true
		s.prewarmPreOpen(ctx)
	}
	if !s.indexDone && sec >= prewarmIndexSecs {
		s.indexDone = true
		s.prewarmIndex(ctx)
	}
	if !s.eodDone && sec >= prewarmEODSecs {
		s.eodDone = true
		s.prewarmEOD(ctx)
	}
}

// prewarmMorning：交易日曆（TWSE 官方開休市表）＋ 公司代碼表（TWSE/TPEx 清單）。
// 兩者皆 24h TTL 並入 L2（§4.2「交易日曆 / 公司代碼表」）；代碼表同時
// 載入 App 之 Symbol Registry（§5.2，供 watchlist/查詢使用）。
func (s *PrewarmScheduler) prewarmMorning(ctx context.Context) {
	if s.app.calClient == nil || s.app.cache == nil {
		s.log.Warn("預熱失敗：交易日曆（client/cache 未接線）")
	} else if err := s.app.calendar.LoadFromOfficial(ctx, s.app.calClient, s.app.cache); err != nil {
		s.log.Warn("預熱失敗：交易日曆（不阻塞啟動，沿用內嵌/既有快取）", "err", err)
	}
	if s.app.regLoader != nil {
		reg, err := s.app.regLoader.Load(ctx)
		if err != nil {
			s.log.Warn("預熱失敗：公司代碼表（不阻塞啟動，沿用既有快取）", "err", err)
			return
		}
		if err := s.app.symbols.Set(reg.List("")); err != nil {
			s.log.Warn("預熱失敗：代碼表載入 Symbol Registry", "err", err)
		}
	}
}

// prewarmPreOpen：開盤前 MIS Session 重取（§8.3：維持 index.jsp Cookie）。
// 失敗僅記錄：MIS 無 cookie 時仍可正常回應。
func (s *PrewarmScheduler) prewarmPreOpen(ctx context.Context) {
	if s.app.misClient == nil {
		return
	}
	if err := provider.WarmupMISSession(ctx, s.app.misClient); err != nil {
		s.log.Warn("預熱失敗：MIS Session（cookie 未取得，繼續採樣）", "err", err)
	}
}

// prewarmIndex：每交易日 15:00 重建 §10.3 Materialized Screener Index。
// 與 16:45 盤後預熱（prewarmEOD）併存：索引早於盤後彙總，使用同一批
// 整批快取路徑（估值/股利/月營收），財報三表逐檔以 bounded concurrency
// 掃描（§10.2）。失敗僅記錄，不阻塞其餘階段與查詢（查詢自動退回
// T017 引擎即時整批路徑）。
func (s *PrewarmScheduler) prewarmIndex(ctx context.Context) {
	if s.app.index == nil {
		return // 未啟用（無資料目錄）
	}
	fn := s.app.indexBuilder
	if fn == nil {
		fn = s.app.rebuildScreenerIndex
	}
	if err := fn(ctx); err != nil {
		s.log.Warn("預熱失敗：Materialized Screener Index 重建", "err", err)
	}
}

// eodTask 為 16:45 盤後預熱之單一全市場任務（§12.4 批次化：
// 僅整批/彙總端點，不做逐股迴圈）。
type eodTask struct {
	name string
	run  func(context.Context) error
}

// prewarmEOD：當日盤後資料（15:00 後釋出，16:45 已齊全）經既有 Handler
// 路徑抓取並入快取（L1/L2 依政策），後續查詢直接命中（is_cached=true）。
func (s *PrewarmScheduler) prewarmEOD(ctx context.Context) {
	tasks := []eodTask{
		{"market_summary", s.taskMarketSummary},
		{"institutional_tse", s.taskInstitutional(model.MarketTSE)},
		{"institutional_otc", s.taskInstitutional(model.MarketOTC)},
		{"foreign_industry_holdings", s.taskForeignIndustryHoldings},
		{"abnormal_trading_tse", s.taskAbnormalTrading(model.MarketTSE)},
		{"abnormal_trading_otc", s.taskAbnormalTrading(model.MarketOTC)},
		{"attention_disposition", s.taskAttentionDisposition},
		{"twse_index", s.taskTWSEIndex},
	}
	for _, t := range tasks {
		if err := t.run(ctx); err != nil {
			s.log.Warn("預熱失敗（盤後資料，略過該項）", "task", t.name, "err", err)
		}
	}
}

func (s *PrewarmScheduler) taskMarketSummary(ctx context.Context) error {
	_, err := handlerGetMarketSummary(s.app, map[string]any{})
	return err
}

func (s *PrewarmScheduler) taskInstitutional(market string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := handlerGetInstitutionalInvestors(s.app, map[string]any{"market": market})
		return err
	}
}

func (s *PrewarmScheduler) taskForeignIndustryHoldings(ctx context.Context) error {
	_, err := handlerGetForeignIndustryHoldings(s.app, map[string]any{})
	return err
}

func (s *PrewarmScheduler) taskAbnormalTrading(market string) func(context.Context) error {
	return func(ctx context.Context) error {
		_, err := handlerGetAbnormalTrading(s.app, map[string]any{"market": market})
		return err
	}
}

func (s *PrewarmScheduler) taskAttentionDisposition(ctx context.Context) error {
	_, err := handlerGetAttentionDispositionStocks(s.app, map[string]any{})
	return err
}

func (s *PrewarmScheduler) taskTWSEIndex(ctx context.Context) error {
	// 預熱加權指數單日收盤（使用預設 symbol）
	_, err := handlerGetTWSEIndex(s.app, map[string]any{})
	return err
}
