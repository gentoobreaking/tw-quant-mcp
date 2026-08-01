package mcp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

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

	env := &model.Envelope{Data: hr.Data, Lineage: lg, HTTPCalls: a.httpCalls.Load(), Disclaimer: model.DisclaimerText}
	if opt.Chart && c.chart != nil {
		if err := c.chart.UpdateEnvelope(env, def, opt, hr.Data); err != nil {
			return nil, fmt.Errorf("mcp: 工具 %s chart 注入失敗: %w", name, err)
		}
	}
	return env, nil
}

// lineageFor 合併 ToolDef.Response 與 HandlerResult.Lineage：
//   - HandlerResult.Lineage 優先（欄位級覆寫）；
//   - 其餘由 ToolDef.Response 補齊；
//   - 兩者皆無 → 盤中預設（§10.A：TWSE_MIS / REALTIME_INTRADAY / 8s 採樣）。
func lineageFor(def *ToolDef, hr HandlerResult) model.Lineage {
	if def.Response == nil && hr.Lineage == nil {
		return model.Lineage{
			Source:      model.SourceTWSEMIS,
			SourceRole:  model.SourceRoleCanonical,
			Freshness:   model.FreshnessRealtimeIntraday,
			SamplingSec: 8,
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
		lg.SourceURL = o.SourceURL
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
