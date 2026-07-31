package mcp

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/calendar"
	"tw-quant-mcp/pkg/config"
	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// WebFetcher 為 TWSE-WEB 資料源之 handler 視界（URL 建構 + 請求 +
// Validate + Normalize）。測試可注入 fake 實作（回傳 fixture），免真實 HTTP。
type WebFetcher interface {
	URL(ds provider.TWSEWebDataset, params url.Values) string
	Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error)
	Validate(raw *provider.RawResponse) error
	Normalize(raw *provider.RawResponse) ([]byte, error)
}

// APIFetcher 為 TWSE-API（openapi.twse.com.tw）資料源之 handler 視界。
type APIFetcher interface {
	URL(ds provider.TWSEAPIDataset, params url.Values) string
	Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error)
	Validate(raw *provider.RawResponse) error
	Normalize(raw *provider.RawResponse) ([]byte, error)
}

// TPExFetcher 為 TPEx-API 資料源之 handler 視界。
type TPExFetcher interface {
	URL(ds provider.TPExDataset, params url.Values) string
	Fetch(ctx context.Context, req provider.RawRequest) (*provider.RawResponse, error)
	Validate(raw *provider.RawResponse) error
	Normalize(raw *provider.RawResponse) ([]byte, error)
}

// App 是 MCP Engine Layer 之組裝根（§6）：
// Symbol Registry / 交易日曆 / 盤中引擎（Watchlist + RingStore +
// Aggregator + IntradayStore）/ 風險掃描器 / 盤後資料源（TWSE-WEB /
// TWSE-API / TPEx-API，T011）/ 快取層 / Tool Registry。
type App struct {
	cfg       *config.Config
	symbols   *model.Registry
	calendar  *calendar.Calendar
	watchlist *engine.Watchlist
	rings     *engine.RingStore
	agg       *engine.Aggregator
	intraday  *engine.IntradayStore
	risk      *DaytradeScanner
	twseWeb   WebFetcher
	twseAPI   APIFetcher
	tpex      TPExFetcher
	cache     *cache.Cache
	core      *Core
	registry  *Registry
	now       func() time.Time
	logger    *slog.Logger
}

// AppOption 為 App 建置選項（測試用注入）。
type AppOption func(*App)

// WithAppClock 注入時鐘（測試用）。
func WithAppClock(now func() time.Time) AppOption {
	return func(a *App) { a.now = now }
}

// WithAppSymbols 注入 Symbol Registry（測試用；預設為空表）。
func WithAppSymbols(s *model.Registry) AppOption {
	return func(a *App) { a.symbols = s }
}

// WithAppCalendar 注入交易日曆（測試用；預設內建行事曆）。
func WithAppCalendar(c *calendar.Calendar) AppOption {
	return func(a *App) { a.calendar = c }
}

// WithAppEngine 注入盤中引擎組（測試用；預設組全新引擎）。
func WithAppEngine(w *engine.Watchlist, rings *engine.RingStore, agg *engine.Aggregator, intraday *engine.IntradayStore) AppOption {
	return func(a *App) {
		a.watchlist = w
		a.rings = rings
		a.agg = agg
		a.intraday = intraday
	}
}

// WithAppScanner 注入風險掃描器（測試用；預設以 symbols 建立）。
func WithAppScanner(s *DaytradeScanner) AppOption {
	return func(a *App) { a.risk = s }
}

// WithAppSources 注入盤後資料源（測試用；預設建立真實 TWSE/TPEx sources）。
func WithAppSources(web WebFetcher, api APIFetcher, tpex TPExFetcher) AppOption {
	return func(a *App) {
		a.twseWeb = web
		a.twseAPI = api
		a.tpex = tpex
	}
}

// WithAppCache 注入快取層（測試用；預設 L1-only 快取）。
func WithAppCache(c *cache.Cache) AppOption {
	return func(a *App) { a.cache = c }
}

// WithAppLogger 注入 slog logger（預設 discard）。
func WithAppLogger(l *slog.Logger) AppOption {
	return func(a *App) { a.logger = l }
}

// NewApp 建立 MCP Engine Layer 組裝根。cfg 可為 nil（預設零值設定）。
func NewApp(cfg *config.Config, opts ...AppOption) (*App, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	a := &App{
		cfg:       cfg,
		symbols:   model.NewRegistry(),
		calendar:  calendar.New(),
		watchlist: nil, // 見下
		rings:     engine.NewRingStore(),
		agg:       nil, // 見下
		intraday:  engine.NewIntradayStore(),
		twseWeb:   provider.NewTWSEWebSource(),
		twseAPI:   provider.NewTWSEAPISource(),
		tpex:      provider.NewTPExSource(),
		now:       func() time.Time { return model.Now().Time },
		logger:    slog.New(slog.NewTextHandler(discard, nil)),
	}
	for _, o := range opts {
		o(a)
	}
	if a.cache == nil {
		var err error
		if a.cache, err = cache.New(); err != nil {
			return nil, fmt.Errorf("mcp: 快取層初始化失敗: %w", err)
		}
	}
	if a.watchlist == nil {
		a.watchlist = engine.NewWatchlist(a.calendar.IsTradingDay)
	}
	if a.agg == nil {
		a.agg = engine.NewAggregator(a.rings)
	}
	if a.risk == nil {
		a.risk = NewDaytradeScanner(a.symbols)
	}
	a.registry = buildRegistry()
	a.core = NewCore(a, a.registry,
		WithCoreClock(a.now),
		WithCoreLogger(a.logger),
	)
	return a, nil
}

// Close 釋放 App 資源（快取層 L2 連線）。
func (a *App) Close() error {
	if a.cache != nil {
		return a.cache.Close()
	}
	return nil
}

// Registry 回傳 Tool 登錄表。
func (a *App) Registry() *Registry { return a.registry }

// Core 回傳統一呼叫入口。
func (a *App) Core() *Core { return a.core }

// Calendar 回傳交易日曆（非交易時段 gate 用）。
func (a *App) Calendar() *calendar.Calendar { return a.calendar }

// Symbols 回傳 Symbol Registry。
func (a *App) Symbols() *model.Registry { return a.symbols }

// Watchlist 回傳盤中觀察清單引擎。
func (a *App) Watchlist() *engine.Watchlist { return a.watchlist }

// Rings 回傳快照 RingStore。
func (a *App) Rings() *engine.RingStore { return a.rings }

// IntradayStore 回傳盤中衍生計算登錄（§8.5）。
func (a *App) IntradayStore() *engine.IntradayStore { return a.intraday }

// RiskScanner 回傳風險掃描器。
func (a *App) RiskScanner() *DaytradeScanner { return a.risk }

// Wire 將登錄表內所有 Tool 註冊至 MCP Server（低階 AddTool，
// schema 驗證與 Envelope 注入統一經 Core）。
func (a *App) Wire(srv *mcp.Server) {
	for _, def := range a.registry.Tools() {
		d := def
		srv.AddTool(d, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := map[string]any{}
			if len(req.Params.Arguments) > 0 && string(req.Params.Arguments) != "null" {
				if err := jsonUnmarshal(req.Params.Arguments, &args); err != nil {
					return errorResult(fmt.Errorf("mcp: 工具 %s 參數無法解析: %w", d.Name, err)), nil
				}
			}
			env, err := a.core.Call(ctx, d.Name, args)
			if err != nil {
				return errorResult(err), nil
			}
			return successResult(env), nil
		})
	}
}

// buildRegistry 登錄 §10.A 之 6 個盤中工具。
func buildRegistry() *Registry {
	r := NewRegistry()
	r.Register(ToolDef{
		Symbol: "set_active_watchlist",
		Name:   "set_active_watchlist",
		Description: "設定盤中即時監控的股票觀察清單（最多 15 檔）。" +
			"呼叫後 background worker 每 8 秒進行快照輪詢，為其餘盤中工具提供記憶體資料。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbols": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"minItems":    1,
					"maxItems":    15,
					"description": "股票代號陣列（1~15 檔），例如 [\"2330\", \"2317\"]",
				},
			},
			"required": []string{"symbols"},
		},
		Handler: handlerSetActiveWatchlist,
	})
	r.Register(ToolDef{
		Symbol: "get_intraday_kline",
		Name:   "get_intraday_kline",
		Description: "查詢指定股票當日盤中即時 1 分 K / 5 分 K 線（純記憶體重採樣，零 HTTP）。" +
			"回傳 Candle[]（timestamp/open/high/low/close/volume）+ _chart_meta（candlestick）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol":    map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"timeframe": map[string]any{"type": "string", "enum": []string{"1m", "5m"}, "description": "K 線週期（預設 1m）"},
				"limit":     map[string]any{"type": "integer", "minimum": 1, "description": "回傳最後 N 根（預設 200）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetIntradayKline,
	})
	r.Register(ToolDef{
		Symbol: "get_intraday_quote",
		Name:   "get_intraday_quote",
		Description: "查詢指定股票最新即時報價 + 五檔買賣價量（純記憶體讀取，零 HTTP）。" +
			"回傳報價欄位與 bids/asks（price/volume）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetIntradayQuote,
	})
	r.Register(ToolDef{
		Symbol: "get_intraday_vwap",
		Name:   "get_intraday_vwap",
		Description: "查詢指定股票當日累計 VWAP、當日高低點與 Fibonacci 支撐/壓力位" +
			"（§8.5 記憶體計算，零 HTTP）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetIntradayVWAP,
	})
	r.Register(ToolDef{
		Symbol: "detect_volume_surge",
		Name:   "detect_volume_surge",
		Description: "偵測指定股票近 N 分鐘爆量/急拉訊號（前 20 分鐘均量滑動窗口比對，" +
			"§8.5 記憶體計算，零 HTTP）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol":  map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"minutes": map[string]any{"type": "integer", "minimum": 1, "maximum": 120, "description": "近 N 分鐘（預設 5）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerDetectVolumeSurge,
	})
	r.Register(ToolDef{
		Symbol: "scan_daytrade_eligibility",
		Name:   "scan_daytrade_eligibility",
		Description: "買前風險掃描：比對當日注意股/處置股名單與停資停券狀態，" +
			"回傳當沖資格、風險摘要（名單來源 TWSE-WEB / TPEx 盤後名單）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerScanDaytradeEligibility,
	})
	registerBCTools(r)
	return r
}
