package mcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"tw-quant-mcp/pkg/chart"
	"tw-quant-mcp/pkg/model"
)

// Core 是 Tool 呼叫之統一入口（§6 MCP Engine Layer）：
// schema 驗證 → handler 執行 → Envelope 注入（lineage 含 latency_ms、
// 依 ChartOption 注入 chart_meta）。所有工具皆經此路徑，任何 Handler
// 不得自行偽造 _lineage。
type Core struct {
	app   *App
	reg   *Registry
	chart ChartUpdater
	now   func() time.Time
	log   *slog.Logger
}

// CoreOption 為 Core 建置選項。
type CoreOption func(*Core)

// WithCoreClock 注入時鐘（測試用）。
func WithCoreClock(now func() time.Time) CoreOption {
	return func(c *Core) { c.now = now }
}

// WithCoreChart 覆寫圖表注入器（測試/客製用；預設 defaultChartUpdater）。
func WithCoreChart(u ChartUpdater) CoreOption {
	return func(c *Core) { c.chart = u }
}

// WithCoreLogger 注入 slog logger（預設 discard）。
func WithCoreLogger(l *slog.Logger) CoreOption {
	return func(c *Core) { c.log = l }
}

// NewCore 建立統一呼叫入口。
func NewCore(app *App, reg *Registry, opts ...CoreOption) *Core {
	c := &Core{
		app:   app,
		reg:   reg,
		chart: defaultChartUpdater{},
		now:   func() time.Time { return model.Now().Time },
		log:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// HandlerResult 為 Handler 回傳契約：業務資料 + 選用之 lineage 覆寫。
// Handler 不得直接回傳 Envelope；lineage 統一由 Core 注入（機制欄位
// FetchedAt/LatencyMS 僅 Core 可填）。Handler 僅可指定來源/角色/
// 新鮮度/資料日期等語意欄位（§3.2）。
type HandlerResult struct {
	Data any
	// Lineage 非 nil 時覆寫 ToolDef.Response 之 lineage 語意欄位
	//（source/source_role/derived_from/freshness/data_date/sampling_sec）。
	// 空欄位由 Core 以 ToolDef.Response（或盤中預設）補齊。
	Lineage *model.Lineage
	// MultiLineage 非 nil 時輸出 _lineage 為 []Lineage（多來源聚合工具，
	// v2.1 §4 設計規則 2：例如 get_stock_trend_composite 同時使用
	// TWSE Web API 與 MOPS）。優先於單一 Lineage；機制欄位
	//（FetchedAt/LatencyMS/DataDate 缺省）仍由 Core 補齊。
	MultiLineage []model.Lineage
	// ChartMeta 非 nil 時直接覆寫 _chart_meta（handler 依資料內容組建，
	// 例如趨勢研判之複合線圖）。nil 時由 Core 依 §11.3 ForTool 注入。
	ChartMeta *chart.Meta
}

// Call 執行單一工具呼叫，回傳 §3.3 Envelope（以 interface{} 承載，
// 由 Wire 層序列化至 StructuredContent）。錯誤一律為明確訊息：
// 未知工具 / schema 驗證失敗 / 業務錯誤（IsError 標記由 Wire 層處理）。
func (c *Core) Call(ctx context.Context, name string, args map[string]any) (interface{}, error) {
	def, ok := c.reg.Get(name)
	if !ok {
		return nil, fmt.Errorf("mcp: 未知工具 %q", name)
	}
	if args == nil {
		args = map[string]any{}
	}
	if err := validateArgs(def, args); err != nil {
		return nil, fmt.Errorf("mcp: 工具 %s 參數驗證失敗: %w", name, err)
	}
	opt, err := ParseChartOption(args)
	if err != nil {
		return nil, fmt.Errorf("mcp: 工具 %s 參數驗證失敗: %w", name, err)
	}

	started := c.now()
	a := c.app
	a.httpCalls.Store(0) // §12.9：每次查詢歸零上游 HTTP 計數器
	hr, err := def.Handler(a, args)
	if err != nil {
		return nil, err
	}

	lg := lineageFor(def, hr)
	lg.FetchedAt = model.NewTaipeiTime(started)
	if lg.DataDate == "" {
		lg.DataDate = model.FormatDate(started)
	}
	lg.LatencyMS = time.Since(started).Milliseconds()

	ls := model.Lineages{Lineage: lg}
	if len(hr.MultiLineage) > 0 {
		// 多來源聚合：逐一補齊機制欄位（v2.1 §4 設計規則 2）。
		multi := make([]model.Lineage, len(hr.MultiLineage))
		for i, sub := range hr.MultiLineage {
			sub.FetchedAt = model.NewTaipeiTime(started)
			if sub.DataDate == "" {
				sub.DataDate = model.FormatDate(started)
			}
			sub.LatencyMS = time.Since(started).Milliseconds()
			multi[i] = sub
		}
		ls.Multi = multi
	}

	env := &model.Envelope{Data: hr.Data, Lineage: ls, HTTPCalls: a.httpCalls.Load(), Disclaimer: model.DisclaimerText}
	if hr.ChartMeta != nil {
		env.ChartMeta = hr.ChartMeta // handler 自組 _chart_meta（優先）
	} else if opt.Chart && c.chart != nil {
		if err := c.chart.UpdateEnvelope(env, def, opt, hr.Data); err != nil {
			return nil, fmt.Errorf("mcp: 工具 %s chart 注入失敗: %w", name, err)
		}
	}
	return env, nil
}

// lineageFor 合併 ToolDef.Response 與 HandlerResult.Lineage：
//   - HandlerResult.Lineage 優先（欄位級覆寫）；
//   - 其餘由 ToolDef.Response 補齊；
//   - 兩者皆無 → 盤中預設（§10.A：TWSE_MIS / SEMI_OFFICIAL_REALTIME /
//     REALTIME_INTRADAY / 8s 採樣；§8 尾註：source_role 固定 SEMI_OFFICIAL_REALTIME、
//     grade 標註 AVAILABLE）。
func lineageFor(def *ToolDef, hr HandlerResult) model.Lineage {
	if def.Response == nil && hr.Lineage == nil {
		return model.Lineage{
			Source:      model.SourceTWSEMIS,
			SourceRole:  model.SourceRoleRealtime,
			Freshness:   model.FreshnessRealtimeIntraday,
			SamplingSec: 8,
			Grade:       model.GradeAvailable,
		}
	}
	var lg model.Lineage
	if def.Response != nil {
		lg = *def.Response
	}
	if hr.Lineage != nil {
		o := hr.Lineage
		if o.Source != "" {
			lg.Source = o.Source
		}
		if o.SourceRole != "" {
			lg.SourceRole = o.SourceRole
		}
		if o.Freshness != "" {
			lg.Freshness = o.Freshness
		}
		if o.DataDate != "" {
			lg.DataDate = o.DataDate
		}
		if o.SamplingSec != 0 {
			lg.SamplingSec = o.SamplingSec
		}
		lg.DerivedFrom = o.DerivedFrom
		lg.IsCached = o.IsCached
		lg.CacheTTL = o.CacheTTL
		lg.CacheAgeSec = o.CacheAgeSec
		lg.SourceURL = o.SourceURL
		if o.Grade != "" {
			lg.Grade = o.Grade
		}
	}
	if lg.Grade == "" {
		// Data Grade 預設 AVAILABLE（T029：36 既有工具全數標註；
		// PREVIEW/NOT_YET_AVAILABLE 由 ToolDef.Response 或 Handler 覆寫）。
		lg.Grade = model.GradeAvailable
	}
	return lg
}

// validateArgs 以登錄時編譯之 JSON Schema 驗證參數。
func validateArgs(def *ToolDef, args map[string]any) error {
	v, err := compileSchema(def.Schema)
	if err != nil {
		return err
	}
	if err := v.Validate(args); err != nil {
		return err
	}
	return nil
}
