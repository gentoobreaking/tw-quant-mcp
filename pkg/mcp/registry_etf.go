package mcp

// registry_etf.go 登錄 §30.1 ETF Data Adapter L1 工具（get_etf_nav, get_etf_dividend）。
// 資料源：TWSE ETF e添富平台（www.twse.com.tw/zh/ETFortune，
// provider.ETFortuneSource）；2026-08 實測為官方現行 NAV 入口。
// ETF 分配收益：TWSE rwd/zh/ETF/etfDiv（provider.ETFDividendSource）。

func registerETFTools(r *Registry) {
	r.Register(ToolDef{
		Symbol: "get_etf_nav",
		Name:   "get_etf_nav",
		Description: "查詢 ETF 歷史淨值（NAV）與折溢價（spec §30.1 L1）。" +
			"資料源：TWSE ETF e添富平台（ajaxEtfInfoChart）；回傳期間內逐日 NAV/市價/折溢價率。" +
			"僅上市 ETF（上櫃 ETF 暫無資料源）。start/end 省略時為近 3 個月。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "ETF 代號，例如 \"0050\""},
				"start":  map[string]any{"type": "string", "description": "起始日 YYYY-MM-DD（預設 3 個月前）"},
				"end":    map[string]any{"type": "string", "description": "迄日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetETFNav,
	})

	r.Register(ToolDef{
		Symbol: "get_etf_dividend",
		Name:   "get_etf_dividend",
		Description: "查詢 ETF 歷史分配收益（配息/收益分配）。" +
			"資料源：TWSE ETF 分配收益 API（etfDiv）；回傳期間內每筆除息日、基準日、發放日、配息金額、分配標準。" +
			"僅上市 ETF。start/end 省略時為近 2 年。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "ETF 代號，例如 \"0056\""},
				"start":  map[string]any{"type": "string", "description": "起始日 YYYY-MM-DD（預設 2 年前）"},
				"end":    map[string]any{"type": "string", "description": "迄日 YYYY-MM-DD（預設最近交易日）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetETFDividend,
	})
	r.Register(ToolDef{
		Symbol: "get_etf_performance",
		Name:   "get_etf_performance",
		Description: "查詢 ETF 報酬率績效序列（e添富 ajaxPerformance，T243）。" +
			"回傳 dates＋performanceA/B 兩組數列（官方未標明 A/B 語意，原樣保留，推測為兩種期間報酬或含/不含配息）。" +
			"symbol 必填；limit 回傳最後 N 點。對稱既有 get_etf_nav（NAV 折溢價）工具。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "ETF 代號（必填），如 0050"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳最後 N 點"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetETFPerformance,
	}) // T243
	r.Register(ToolDef{
		Symbol: "get_etf_dividend_detail",
		Name:   "get_etf_dividend_detail",
		Description: "查詢 ETF 配息明細與收益分配政策全文（e添富 ajaxDividendData，T243）。" +
			"含「尚未發生」之預定除息日/預定發放日（可提前得知下一期配息日程）與政策全文。" +
			"symbol 必填；upcoming_only 僅回未發放之預定配息。對稱既有 get_etf_dividend（僅歷史已發放配息）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol":        map[string]any{"type": "string", "description": "ETF 代號（必填），如 0056"},
				"upcoming_only": map[string]any{"type": "boolean", "default": false, "description": "僅回未發放之預定配息"},
				"limit":         map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset":        map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetETFDividendDetail,
	}) // T243
}
