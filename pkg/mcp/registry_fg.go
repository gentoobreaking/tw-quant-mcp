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
		Symbol:      "get_daily_futures_market_report",
		Name:        "get_daily_futures_market_report",
		Description: "查詢期貨每日交易行情，包含開高低收、成交量、未平倉量等資訊（TAIFEX-API DailyMarketReportFut，T117）。" +
			"常用契約代碼：TX（臺指期貨）、MTX（小型臺指）等白名單契約；contract 留空可列出所有可用契約代碼。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "default": "TX", "description": "期貨契約代碼，例如 TX、MTX。留空則列出所有可用契約代碼"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetDailyFuturesMarketReport,
	}) // T117
	r.Register(ToolDef{
		Symbol:      "get_daily_options_market_report",
		Name:        "get_daily_options_market_report",
		Description: "查詢選擇權每日交易行情，篩選有成交量的履約價資料，按成交量由大到小排序（TAIFEX-API DailyMarketReportOpt，T118）。" +
			"常用契約代碼：TXO（臺指選擇權）、TEO（電子選擇權）、TFO（金融選擇權）；contract 留空可列出所有可用契約代碼。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "default": "TXO", "description": "選擇權契約代碼，例如 TXO。留空則列出所有可用契約代碼"},
				"call_put": map[string]any{"type": "string", "enum": []string{"買權", "賣權"}, "description": "篩選買賣權，留空顯示全部"},
				"limit":    map[string]any{"type": "integer", "default": 30, "minimum": 1, "description": "顯示筆數上限（按成交量由大到小）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetDailyOptionsMarketReport,
	}) // T118
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
		Symbol:      "get_futures_daily_history",
		Name:        "get_futures_daily_history",
		Description: "查詢期貨每日OHLC歷史行情（可回溯查詢，非僅最新一日；TAIFEX-DL 下載頁回溯，T125）。" +
			"contract 省略時預設 TX（臺股期貨）；回傳區間內每個交易日、每個到期月份、一般與盤後時段行情。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract":   map[string]any{"type": "string", "default": "TX", "description": "期貨契約代碼，預設 TX。常用：MTX、E4F（電子）、GXF（金融）"},
				"start":      map[string]any{"type": "string", "description": "起始日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 start_date）"},
				"end":        map[string]any{"type": "string", "description": "結束日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 end_date）；區間跨度上限 366 日"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetFuturesDailyHistory,
	}) // T125
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
		Symbol:      "get_annual_trading_volume",
		Name:        "get_annual_trading_volume",
		Description: "查詢各期貨商品年成交量統計（年度總成交量、交易日數、平均日成交量；TAIFEX-API，T041）。" +
			"contract 省略則回傳全部商品。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "description": "期貨契約代碼（如 TX、MTX），留空顯示全部"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetAnnualTradingVolume,
	})
	r.Register(ToolDef{
		Symbol:      "get_monthly_trading_statistics",
		Name:        "get_monthly_trading_statistics",
		Description: "查詢期貨市場月統計資料，依商品類別（股價指數、利率、商品、股票）分類，" +
			"顯示各類型交易人（自營商、投信、外資、散戶等）的買賣量與月底未平倉量" +
			"（TAIFEX-API MonthlyTradingStatisticsFutures，T148）。", 
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  handlerGetMonthlyTradingStatistics,
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
		Symbol:      "get_futures_institutional",
		Name:        "get_futures_institutional",
		Description: "查詢三大法人期貨與選擇權每日交易資訊（期貨+選擇權合計；多空交易量/金額、未平倉與契約價值；" +
			"TAIFEX-API DividedByFuturesAndOptionsBytheDate，T126）。date 省略為最新交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetFuturesInstitutional,
	}) // T126
	r.Register(ToolDef{
		Symbol:      "get_index_futures_margin",
		Name:        "get_index_futures_margin",
		Description: "查詢股價指數類期貨與選擇權保證金一覽表，包含結算保證金、維持保證金、原始保證金（元；" +
			"TAIFEX-API IndexFuturesAndOptionsMargining，T127）。contract 為中文商品名子字串（如「臺股期貨」），留空顯示全部。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "description": "商品名稱（中文），例如「臺股期貨」。留空則顯示全部"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetIndexFuturesMargin,
	}) // T127
	r.Register(ToolDef{
		Symbol:      "get_institutional_fut_opt_split_history",
		Name:        "get_institutional_fut_opt_split_history",
		Description: "查詢三大法人期貨與選擇權分計交易歷史（期貨、選擇權並列顯示，可回溯查詢；" +
			"TAIFEX-DL futAndOptDateDown，T128）。與 get_institutional_total_history（合計）不同，" +
			"本工具將期貨與選擇權之多空交易口數、契約金額（千元）、未平倉分開列出。區間不可超過 92 日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start": map[string]any{"type": "string", "description": "起始日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 start_date）"},
				"end":   map[string]any{"type": "string", "description": "結束日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 end_date）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalFutOptSplitHistory,
	}) // T128
	r.Register(ToolDef{
		Symbol:      "get_institutional_general",
		Name:        "get_institutional_general",
		Description: "查詢三大法人（自營商、投信、外資）當日期貨與選擇權市場整體交易總表，" +
			"包含交易量、交易金額（百萬元）、未平倉口數及契約價值（TAIFEX-API GeneralBytheDate，T129）。" +
			"date 省略為最新交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日期 YYYY-MM-DD（預設最新交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalGeneral,
	}) // T129
	r.Register(ToolDef{
		Symbol:      "get_institutional_total_history",
		Name:        "get_institutional_total_history",
		Description: "查詢三大法人期貨與選擇權合計總表歷史（可回溯查詢；TAIFEX-DL totalTableDateDown，T130）。" +
			"與 get_institutional_traders_by_futures_history（僅期貨）不同，本工具為期貨+選擇權合計數字。區間不可超過 92 日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start": map[string]any{"type": "string", "description": "起始日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 start_date）"},
				"end":   map[string]any{"type": "string", "description": "結束日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 end_date）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalTotalHistory,
	}) // T130
	r.Register(ToolDef{
		Symbol:      "get_institutional_traders_by_futures",
		Name:        "get_institutional_traders_by_futures",
		Description: "查詢三大法人依各期貨契約分類的交易資料，可觀察各期貨商品的法人買賣情況（" +
			"TAIFEX-API DetailsOfFuturesContracts，T131）。contract_code 為中文契約名子字串（如「臺股期貨」），留空顯示全部。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract_code": map[string]any{"type": "string", "description": "期貨契約名稱（中文），例如「臺股期貨」。留空則顯示全部"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalTradersByFutures,
	}) // T131
	r.Register(ToolDef{
		Symbol:      "get_institutional_traders_by_futures_history",
		Name:        "get_institutional_traders_by_futures_history",
		Description: "查詢三大法人期貨部位歷史資料（可回溯查詢；TAIFEX-DL futContractsDateDown，T132）。" +
			"contract 為期貨契約代碼（TXF/MXF/EXF/FXF/TMF 等，預設 TXF），與日行情之 TX/MTX 為不同代碼系統。區間不可超過 92 日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "default": "TXF", "description": "期貨契約代碼，預設 TXF。留空查詢全部契約（資料量較大）"},
				"start":    map[string]any{"type": "string", "description": "起始日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 start_date）"},
				"end":      map[string]any{"type": "string", "description": "結束日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 end_date）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalTradersByFuturesHistory,
	}) // T132
	r.Register(ToolDef{
		Symbol:      "get_large_traders_futures_history",
		Name:        "get_large_traders_futures_history",
		Description: "查詢期貨大額交易人未沖銷部位歷史資料（可回溯查詢；TAIFEX-DL largeTraderFutDown，T135）。" +
			"contract 為必填契約代碼（如 TX、MTX、TE、TF），由本工具取得資料後本地端篩選。區間不可超過 31 日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "description": "期貨契約代碼（必填），例如 TX、MTX、TE、TF"},
				"start":    map[string]any{"type": "string", "description": "起始日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 start_date）"},
				"end":      map[string]any{"type": "string", "description": "結束日期 YYYY-MM-DD（或 YYYYMMDD；亦可用 end_date）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetLargeTradersFuturesHistory,
	}) // T135
	r.Register(ToolDef{
		Symbol:      "get_large_traders_futures_oi",
		Name:        "get_large_traders_futures_oi",
		Description: "查詢期貨大額交易人（前五大、前十大）未沖銷部位資料，可觀察大戶持倉方向（" +
			"TAIFEX-API OpenInterestOfLargeTradersFutures，T136）。contract 精確比對契約代碼，預設 TX；留空列出所有可用契約代碼。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "default": "TX", "description": "期貨契約代碼，預設 TX。留空則列出所有可用契約代碼"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetLargeTradersFuturesOI,
	}) // T136
	r.Register(ToolDef{
		Symbol:      "get_large_traders_options_oi",
		Name:        "get_large_traders_options_oi",
		Description: "查詢選擇權大額交易人（前五大、前十大）未沖銷部位資料，可觀察大戶選擇權布局（" +
			"TAIFEX-API OpenInterestOfLargeTradersOptions，T137）。contract 精確比對契約代碼，預設 TXO；留空列出所有可用契約代碼。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract": map[string]any{"type": "string", "default": "TXO", "description": "選擇權契約代碼，預設 TXO。留空則列出所有可用契約代碼"},
				"call_put": map[string]any{"type": "string", "enum": []string{"買權", "賣權"}, "description": "篩選買賣權（選填）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetLargeTradersOptionsOI,
	}) // T137
	r.Register(ToolDef{
		Symbol:      "get_institutional_traders_by_options",
		Name:        "get_institutional_traders_by_options",
		Description: "查詢三大法人依各選擇權契約分類的交易資料，可觀察各選擇權商品的法人買賣情況（" +
			"TAIFEX-API DetailsOfOptionsContracts，T133）。contract_code 為中文契約名子字串（如「臺指選擇權」），留空顯示全部。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract_code": map[string]any{"type": "string", "description": "選擇權契約名稱（中文），例如「臺指選擇權」。留空則顯示全部"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalTradersByOptions,
	}) // T133
	r.Register(ToolDef{
		Symbol:      "get_institutional_traders_calls_puts",
		Name:        "get_institutional_traders_calls_puts",
		Description: "查詢三大法人選擇權買賣權分計交易資料，分別顯示 CALL 與 PUT 的法人持倉情況（" +
			"TAIFEX-API DetailsOfCallsAndPuts，T134）。外資偏多時 CALL 淨多單會大幅增加。contract_code 留空顯示全部。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"contract_code": map[string]any{"type": "string", "description": "選擇權契約名稱（中文），例如「臺指選擇權」。留空則顯示全部"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalTradersCallsPuts,
	}) // T134
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
