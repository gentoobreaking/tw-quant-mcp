package mcp

// registry_fg.go 登錄 §10.F（期貨與選擇權，7 工具）與 §10.G（基礎設施，
// 2 工具）（T015）。F 組資料源：T013 TAIFEX 查詢層（API hot tier /
// DL cold tier，§9.3）；G 組：T005 Symbol Registry 與交易日曆。

func registerFGTools(r *Registry) {
	r.Register(ToolDef{
		Symbol: "get_futures_daily_ohlc",
		Name:   "get_futures_daily_ohlc",
		Description: "查詢期貨契約每日 OHLC（TAIFEX，openapi 最新交易日 hot tier）。" +
			"date 省略時為最新交易日；回傳該日該契約全到期月份/時段之行情（價格單位：點）。" +
			"契約代號限白名單（TX/MTX/GTX/G2F/G1F/G9F/E4F/XIF/GXF/T5F）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "description": "期貨契約代號（如 TX、MTX）"},
				"date":     map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
			},
			"required": []string{"contract"},
		},
		ReadOnly: true,
		Handler:  handlerGetFuturesDailyOHLC,
	})
	r.Register(ToolDef{
		Symbol: "get_futures_history",
		Name:   "get_futures_history",
		Description: "查詢期貨 OHLC 歷史（TAIFEX-DL 下載頁回溯，§9.3；L2 永久快取）。" +
			"start/end 跨度 ≤ 366 日；回傳依日期/到期月份排序之行情。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "description": "期貨契約代號（如 TX、MTX）"},
				"start":    map[string]any{"type": "string", "description": "起始日 YYYY-MM-DD"},
				"end":      map[string]any{"type": "string", "description": "結束日 YYYY-MM-DD"},
			},
			"required": []string{"contract", "start", "end"},
		},
		ReadOnly: true,
		Handler:  handlerGetFuturesHistory,
	})
	r.Register(ToolDef{
		Symbol: "get_put_call_ratio",
		Name:   "get_put_call_ratio",
		Description: "查詢買賣權比（Put/Call Ratio，成交量/未平倉比；§10.F）。" +
			"單日（date，省略為最新交易日）或範圍（start/end，支援歷史回溯）；" +
			"多空分界線 1.0 由 _chart_meta 標示。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":  map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
				"start": map[string]any{"type": "string", "description": "範圍起日 YYYY-MM-DD（與 end 成對）"},
				"end":   map[string]any{"type": "string", "description": "範圍迄日 YYYY-MM-DD（與 start 成對）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetPutCallRatio,
	})
	r.Register(ToolDef{
		Symbol: "get_large_trader_positions",
		Name:   "get_large_trader_positions",
		Description: "查詢大額交易人未沖銷部位（期貨 + 選擇權合併；§10.F）。" +
			"單日（date，省略為最新交易日）或範圍（start/end）；" +
			"回傳前五大/前十大交易人買賣方口數與全市場未沖銷部位。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":  map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
				"start": map[string]any{"type": "string", "description": "範圍起日 YYYY-MM-DD（與 end 成對）"},
				"end":   map[string]any{"type": "string", "description": "範圍迄日 YYYY-MM-DD（與 start 成對）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetLargeTraderPositions,
	})
	r.Register(ToolDef{
		Symbol: "get_institutional_futures_positions",
		Name:   "get_institutional_futures_positions",
		Description: "查詢三大法人期貨部位（自營/投信/外資之多方、空方、未平倉口數與金額，" +
			"§10.F；TAIFEX API 最新交易日，date 省略為最新交易日）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalFuturesPositions,
	})
	r.Register(ToolDef{
		Symbol: "get_institutional_options_positions",
		Name:   "get_institutional_options_positions",
		Description: "查詢三大法人選擇權部位（自營/投信/外資之多方、空方、未平倉口數與金額，" +
			"§10.F；TAIFEX API 最新交易日，date 省略為最新交易日）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalOptionsPositions,
	})
	r.Register(ToolDef{
		Symbol: "get_institutional_futures_history",
		Name:   "get_institutional_futures_history",
		Description: "查詢三大法人期貨部位歷史（TAIFEX-DL 回溯，§9.3；L2 永久快取）。" +
			"start/end 跨度 ≤ 366 日；回傳依日期/身份別排序之部位。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start": map[string]any{"type": "string", "description": "起始日 YYYY-MM-DD"},
				"end":   map[string]any{"type": "string", "description": "結束日 YYYY-MM-DD"},
			},
			"required": []string{"start", "end"},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalFuturesHistory,
	})
	r.Register(ToolDef{
		Symbol: "get_symbol_list",
		Name:   "get_symbol_list",
		Description: "查詢上市/上櫃代碼表（§10.G；Symbol Registry，§5.2）。" +
			"來源為 TWSE/TPEx 官方清單（24h 快取每日預熱）；" +
			"market 省略時回傳全部（依代碼排序）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market": map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別（省略為全部）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetSymbolList,
	})
	r.Register(ToolDef{
		Symbol: "get_trading_calendar",
		Name:   "get_trading_calendar",
		Description: "查詢交易日曆（§10.G；TWSE 官方開休市表，內嵌 2026 年資料）。" +
			"year/month 省略時為今年/全年；回傳交易日清單與官方休市日（含名稱）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"year":  map[string]any{"type": "integer", "minimum": 2000, "maximum": 2100, "description": "西元年（預設今年）"},
				"month": map[string]any{"type": "integer", "minimum": 1, "maximum": 12, "description": "月份（預設全年）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetTradingCalendar,
	})
}
