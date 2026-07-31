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
	data, err := def.Handler(c.app, args)
	if err != nil {
		return nil, err
	}

	env := &model.Envelope{
		Data: data,
		Lineage: model.Lineage{
			Source:      model.SourceTWSEMIS,
			SourceRole:  model.SourceRoleCanonical,
			FetchedAt:   model.NewTaipeiTime(started),
			DataDate:    model.FormatDate(started),
			Freshness:   model.FreshnessRealtimeIntraday,
			SamplingSec: 8,
			LatencyMS:   time.Since(started).Milliseconds(),
		},
	}
	if opt.Chart && c.chart != nil {
		if err := c.chart.UpdateEnvelope(env, def, opt, data); err != nil {
			return nil, fmt.Errorf("mcp: 工具 %s chart 注入失敗: %w", name, err)
		}
	}
	return env, nil
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
