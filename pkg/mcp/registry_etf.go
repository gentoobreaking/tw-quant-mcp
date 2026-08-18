package mcp

// registry_etf.go 登錄 §30.1 ETF Data Adapter L1 工具（get_etf_nav）。
// 資料源：TWSE ETF e添富平台（www.twse.com.tw/zh/ETFortune，
// provider.ETFortuneSource）；2026-08 實測為官方現行 NAV 入口。

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
}
