package mcp

// registry_bc.go 登錄 §10.B（盤後行情・籌碼）與 §10.C（重大訊息與風險）
// 之 11 個工具。全部為 POST_MARKET 資料，lineage 由 handler 回報。

// registerBCTools 將 B/C 組工具登錄至 r。
func registerBCTools(r *Registry) {
	r.Register(ToolDef{
		Symbol: "get_stock_daily_quote",
		Name:   "get_stock_daily_quote",
		Description: "查詢個股盤後日收盤報價（含 MA20/MA60、RSI14、MACD helper 指標）。" +
			"上市以 TWSE-WEB 日 K（近 3 個月）計算指標；上櫃以 TPEx 收盤行情（指標暫缺）。" +
			"date 省略時回傳最近交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetStockDailyQuote,
	})
	r.Register(ToolDef{
		Symbol: "get_stock_daily_kline",
		Name:   "get_stock_daily_kline",
		Description: "查詢個股盤後日/週/月 K 線（TWSE-WEB STOCK_DAY，period/adjust 官方參數）。" +
			"date 為月份起點，省略時為最近交易日。上櫃資料源未接線（錯誤）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
				"period": map[string]any{"type": "string", "enum": []string{"day", "week", "month"}, "description": "K 線週期（預設 day）"},
				"adjust": map[string]any{"type": "boolean", "description": "是否還原權值（adjust=Y，預設 false）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetStockDailyKline,
	})
	r.Register(ToolDef{
		Symbol: "get_market_summary",
		Name:   "get_market_summary",
		Description: "查詢全市場盤後漲跌家數/成交量/漲跌停（上市 TWSE-WEB 收盤行情 + " +
			"上櫃 TPEx 收盤行情）。date 省略時為最近交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetMarketSummary,
	})
	r.Register(ToolDef{
		Symbol: "get_institutional_investors",
		Name:   "get_institutional_investors",
		Description: "查詢三大法人買賣超（個股 + 市場彙總）。" +
			"上市 TWSE-WEB T86 / 上櫃 TPEx 三大法人明細。15:00 前資料可能未齊全（lineage 註記）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market": map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別"},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"market"},
		},
		ReadOnly: true,
		Handler:  handlerGetInstitutionalInvestors,
	})
	r.Register(ToolDef{
		Symbol: "get_foreign_industry_holdings",
		Name:   "get_foreign_industry_holdings",
		Description: "查詢外資產業配置（TWSE-API 類股外資持股比率，chart pie）。" +
			"date 省略時為最近交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date": map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetForeignIndustryHoldings,
	})
	r.Register(ToolDef{
		Symbol: "get_foreign_shareholding_history",
		Name:   "get_foreign_shareholding_history",
		Description: "查詢個股外資及陸資持股歷史（TWSE-WEB MI_QFIIS 逐日快照，T-1 翌日釋出）。" +
			"僅上市股票。series 由近至遠。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"range":  map[string]any{"type": "integer", "minimum": 1, "maximum": 30, "description": "回傳交易日數（預設 5）"},
				"date":   map[string]any{"type": "string", "description": "結束日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetForeignShareholdingHistory,
	})
	r.Register(ToolDef{
		Symbol: "get_margin_trading",
		Name:   "get_margin_trading",
		Description: "查詢個股盤後融資融券（上市 TWSE-WEB MI_MARGN / 上櫃 TPEx 融資融券）。" +
			"date 省略時為最近交易日。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetMarginTrading,
	})
	r.Register(ToolDef{
		Symbol: "get_abnormal_trading",
		Name:   "get_abnormal_trading",
		Description: "查詢異常成交量（注意股）排名（上市 TWSE-WEB notice / 上櫃 TPEx 注意股）。" +
			"top_n 預設 20，最大 100。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market": map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別"},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
				"top_n":  map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "回傳筆數（預設 20）"},
			},
			"required": []string{"market"},
		},
		ReadOnly: true,
		Handler:  handlerGetAbnormalTrading,
	})
	r.Register(ToolDef{
		Symbol: "get_warrant_activity",
		Name:   "get_warrant_activity",
		Description: "查詢權證活躍度（TWSE-API 權證每日成交：成交金額/張數 Top N）。" +
			"top_n 預設 10，最大 50。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":  map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
				"top_n": map[string]any{"type": "integer", "minimum": 1, "maximum": 50, "description": "回傳筆數（預設 10）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetWarrantActivity,
	})
	r.Register(ToolDef{
		Symbol:      "get_after_hours_trading",
		Name:        "get_after_hours_trading",
		Description: "查詢集中市場盤後定價交易（TWSE-WEB BFT41U，T040）。" +
			"code 選填（單檔查詢）；limit 預設 50；offset 分頁。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":   map[string]any{"type": "string", "description": "股票代號（選填，預設全部）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetAfterHoursTrading,
	})
	r.Register(ToolDef{
		Symbol:      "get_major_announcements",
		Name:        "get_major_announcements",
		Description: "查詢上市/上櫃重大訊息（MOPS 公開資訊觀測站 Open Data，T012）。支援依日期、股票代號、關鍵字過濾。資料來源：mopsfin.twse.com.tw",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":    map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD"},
				"symbol":  map[string]any{"type": "string", "description": "股票代號（選填）"},
				"keyword": map[string]any{"type": "string", "description": "關鍵字（選填）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetMajorAnnouncements,
	})
	r.Register(ToolDef{
		Symbol: "get_twse_index",
		Name:   "get_twse_index",
		Description: "查詢 TWSE 指數盤後行情與歷史日 K（加權指數、寶島、臺灣50 等）。" +
			"symbol 為指數名稱（省略預設「發行量加權股價指數」）；date 省略時為最近交易日。" +
			"資料來源：TWSE-API MI_INDEX（單日收盤）+ TWSE-WEB MI_5MINS_HIST（歷史日 K）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "指數名稱（如：發行量加權股價指數、寶島股價指數、臺灣50指數），預設發行量加權股價指數"},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetTWSEIndex,
	})
	r.Register(ToolDef{
		Symbol: "get_attention_disposition_stocks",
		Name:   "get_attention_disposition_stocks",
		Description: "查詢注意股/處置股清單（買前風險掃描）。" +
			"上市：TWSE-WEB notice + TWSE-API punish；上櫃：TPEx 注意/處置。" +
			"結果同步注入 scan_daytrade_eligibility 名單。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market": map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別"},
				"date":   map[string]any{"type": "string", "description": "交易日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"market"},
		},
		ReadOnly: true,
		Handler:  handlerGetAttentionDispositionStocks,
	})
}
