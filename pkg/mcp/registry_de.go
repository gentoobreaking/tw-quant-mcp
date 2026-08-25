package mcp

// registry_de.go 登錄 §10.D（基本面與篩選）與 §10.E（股利）之 10 個工具
// （T014）。資料源：MOPS（T012）、TWSE-API/TPEx（T008/T009）；五面向
// 評分（get_financial_health_check）由 T017 composite engine 提供。

import "tw-quant-mcp/pkg/provider"

func registerDETools(r *Registry) {
	r.Register(ToolDef{
		Symbol: "get_financial_statements",
		Name:   "get_financial_statements",
		Description: "查詢個股財報三表（MOPS）。period 支援 \"2026Q1\"（或 \"2026\" 年度，" +
			"省略時為最新一季）；statement 為 income（損益表摘要+獲利能力）/balance/cashflow，" +
			"省略時回傳全部。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol":    map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"period":    map[string]any{"type": "string", "description": "期間 \"2026Q1\" 或 \"2026\"（預設最新一季）"},
				"statement": map[string]any{"type": "string", "enum": []string{"income", "balance", "cashflow"}, "description": "報表別（預設全部）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetFinancialStatements,
	})
	r.Register(ToolDef{
		Symbol: "get_monthly_revenue",
		Name:   "get_monthly_revenue",
		Description: "查詢個股月營收與成長率（MOPS t187ap05_L，含 YoY/MoM/累計）。" +
			"years 指定回傳年數（預設 2，上限 10），列由近至遠。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"years":  map[string]any{"type": "integer", "minimum": 1, "maximum": 10, "description": "回傳年數（預設 2）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetMonthlyRevenue,
	})
	r.Register(ToolDef{
		Symbol: "get_financial_health_check",
		Name:   "get_financial_health_check",
		Description: "查詢個股財務健康五面向評分（獲利/成長/結構/配息/治理，各 0-100）。" +
			"評分輸入來自 T014 已快取之官方資料（MOPS 財報/TWSE 估值・股利・ESG/TPEx 估值）；" +
			"評分規則版本化（scoring_version，config 可調）；輸出為 helper 資料（_lineage.source_role=helper）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetFinancialHealthCheck,
	})
	r.Register(ToolDef{
		Symbol: "get_valuation_ratios",
		Name:   "get_valuation_ratios",
		Description: "查詢個股估值指標（PE/PB/殖利率/ROE/每股股利）。" +
			"上市 TWSE-API BWIBBU_ALL + MOPS；上櫃 TPEx 本益比/殖利率/淨值比。" +
			"ROE 為 MOPS 損益表摘要年化 ÷ 權益之年化估計（官方無直接端點）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetValuationRatios,
	})
	r.Register(ToolDef{
		Symbol: "get_esg_report",
		Name:   "get_esg_report",
		Description: "查詢個股 ESG 揭露完整報告（T037 雙來源：TWSE OpenAPI / MOPS CSV " +
			"t187ap46_L_1~8 八主題——溫室氣體排放/再生能源/用水/廢棄物/員工薪資福利/" +
			"董事會組成/法說會/TCFD，另附 t187ap32_L 公司治理規程）。" +
			"首次呼叫自動速度選源（快者為主來源），主來源失敗自動 fallback。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"topics": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "integer", "minimum": 1, "maximum": 8},
					"description": "揭露主題（1=溫室氣體 2=再生能源 3=用水 4=廢棄物 5=員工薪資福利 6=董事會 7=法說會 8=TCFD），省略回傳全部",
				},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetESGReport,
	})
	r.Register(ToolDef{
		Symbol:      "get_companies_with_refineries_in_populated_areas",
		Name:        "get_companies_with_refineries_in_populated_areas",
		Description: "查詢所有已申報在人口密集區設有煉油廠的上市公司（排除零值及 N/A；" +
			"TWSE-API ESG t187ap46_L_15，T065）。", 
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  handlerGetRefineriesPopulatedAreas,
	}) // T065

	r.Register(ToolDef{
		Symbol:      "get_company_balance_sheet",
		Name:        "get_company_balance_sheet",
		Description: "根據股票代號查詢上市公司資產負債表（TWSE-API t187ap07_L，T067）。" +
			"自動偵測公司所屬產業並使用對應的財務報表格式（一般業、金融業、證券期貨業、金控業、保險業、異業）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"code"},
		},
		ReadOnly: true,
		Handler:  handlerGetCompanyBalanceSheet,
	}) // T067

	r.Register(ToolDef{
		Symbol:      "get_company_board_info",
		Name:        "get_company_board_info",
		Description: "根據股票代號查詢上市公司董事會資訊（ESG 揭露 t187ap46_L_6，T068）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"code"},
		},
		ReadOnly: true,
		Handler:  esgCompanySpec{topic: 6}.handler(),
	}) // T068

	r.Register(ToolDef{
		Symbol:      "get_company_board_insufficient_shares",
		Name:        "get_company_board_insufficient_shares",
		Description: "查詢上市公司董事、監察人持股不足法定成數彙總表（TWSE-API t187ap08_L，T069）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIBoardInsuff}.handler(),
	}) // T069

	r.Register(ToolDef{
		Symbol:      "get_company_board_insufficient_shares_consecutive",
		Name:        "get_company_board_insufficient_shares_consecutive",
		Description: "查詢上市公司董事、監察人持股連續不足月數彙總表（TWSE-API t187ap10_L，T070）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIBoardInsuffCon}.handler(),
	}) // T070

	r.Register(ToolDef{
		Symbol:      "get_company_board_pledged_shares",
		Name:        "get_company_board_pledged_shares",
		Description: "查詢上市公司董事、監察人質權設定占董事及監察人實際持有股數彙總表（TWSE-API t187ap09_L，T071）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIBoardPledged}.handler(),
	}) // T071

	r.Register(ToolDef{
		Symbol:      "get_company_board_shareholdings",
		Name:        "get_company_board_shareholdings",
		Description: "根據股票代號查詢上市公司董監事持股餘額明細資料（TWSE-API t187ap11_L，T072）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"code"},
		},
		ReadOnly: true,
		Handler:  apiCompanySpec{ds: provider.TWSEAPIBoardHoldings}.handler(),
	}) // T072

	r.Register(ToolDef{
		Symbol:      "get_company_ceo_dual_role",
		Name:        "get_company_ceo_dual_role",
		Description: "查詢上市公司董事長是否兼任總經理資訊彙總表（TWSE-API t187ap33_L，T073）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPICEODualRole}.handler(),
	}) // T073
	r.Register(ToolDef{
		Symbol: "get_company_profile",
		Name:   "get_company_profile",
		Description: "查詢公司基本資料（MOPS t187ap03_L：董事長、資本額、上市日期、" +
			"產業別、發言人、過戶機構等）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetCompanyProfile,
	})
	r.Register(ToolDef{
		Symbol: "screen_stocks",
		Name:   "screen_stocks",
		Description: "價值/成長篩選全市場股票（§10.D；T017 composite 引擎批次過濾，" +
			"整批快取 + 記憶體計算，§12.4）。條件：max_pe（低本益比）、max_pb（低股價淨值比）、" +
			"min_yield（高殖利率）、min_growth（月營收 YoY）、min_profit_growth（淨利 YoY）、" +
			"require_esg（具 ESG 揭露）。排序 sort（pe 預設|yield|pb|growth）；limit 即 top_n 回傳上限。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market":            map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別（省略為全部）"},
				"max_pe":            map[string]any{"type": "number", "description": "本益比上限"},
				"max_pb":            map[string]any{"type": "number", "description": "股價淨值比上限"},
				"min_yield":         map[string]any{"type": "number", "description": "最低現金殖利率（%）"},
				"min_growth":        map[string]any{"type": "number", "description": "最低月營收 YoY（%）"},
				"min_profit_growth": map[string]any{"type": "number", "description": "最低淨利 YoY（最新季 vs 去年同期，%）"},
				"require_esg":       map[string]any{"type": "boolean", "description": "僅保留具 ESG 揭露之公司"},
				"sort":              map[string]any{"type": "string", "enum": []string{"pe", "yield", "pb", "growth"}, "description": "排序（預設 pe 升冪；yield/growth 遞減）"},
				"limit":             map[string]any{"type": "integer", "minimum": 1, "maximum": 200, "description": "top_n 回傳筆數上限（預設 50）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerScreenStocks,
	})
	r.Register(ToolDef{
		Symbol: "get_dividend_history",
		Name:   "get_dividend_history",
		Description: "查詢個股配息歷史與穩定性（上市 t187ap45_L 股利分派；上櫃 TPEx 最新年度）。" +
			"輸出：各股利年度現金/股票股利、連續配息年數、平均每股現金股利、最新殖利率。" +
			"官方 Open API 僅提供現行年度分派資料，歷史深度有限（note 說明）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetDividendHistory,
	})
	r.Register(ToolDef{
		Symbol: "get_exdividend_calendar",
		Name:   "get_exdividend_calendar",
		Description: "查詢除權除息行事曆（上市 TWT48U_ALL + 上櫃 TPEx 預告；§10.E）。" +
			"start/end 省略時為今日起 6 個月；事件依日期排序。" +
			"資料 L2 持久（§4.1/4.2），L1 24h 內重取以納入新公告。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start": map[string]any{"type": "string", "description": "起始日 YYYY-MM-DD（預設今日）"},
				"end":   map[string]any{"type": "string", "description": "結束日 YYYY-MM-DD（預設 start+6 個月）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetExdividendCalendar,
	})
	r.Register(ToolDef{
		Symbol: "screen_high_yield",
		Name:   "screen_high_yield",
		Description: "高殖利率排行（§10.E；T017 composite 引擎批次過濾）。" +
			"條件：min_yield（預設 3%）、min_dividend（每股現金股利下限）、max_pe、" +
			"min_consecutive（最低連年配息年數，配息穩定性）。" +
			"結果依殖利率遞減；整批快取 + 記憶體計算（§12.4）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"market":          map[string]any{"type": "string", "enum": []string{"tse", "otc"}, "description": "市場別（省略為全部）"},
				"min_yield":       map[string]any{"type": "number", "description": "最低現金殖利率 %（預設 3）"},
				"min_dividend":    map[string]any{"type": "number", "description": "最低每股現金股利（元/股）"},
				"max_pe":          map[string]any{"type": "number", "description": "本益比上限（0=不限制）"},
				"min_consecutive": map[string]any{"type": "integer", "minimum": 0, "description": "最低連年配息年數（0=不限制）"},
				"limit":           map[string]any{"type": "integer", "minimum": 1, "maximum": 100, "description": "回傳筆數上限（預設 20）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerScreenHighYield,
	})
	r.Register(ToolDef{
		Symbol: "get_stock_trend_composite",
		Name:   "get_stock_trend_composite",
		Description: "短中長期「技術面+基本面+籌碼面」綜合研判（v2.1 §9.1，Grade PREVIEW）。" +
			"horizon 為 short（近 1 月 MA5/MA20 + 法人 5 日）/mid（近 3 月 MA20/MA60 + " +
			"法人 20 日）/long（近 6 月 MA20/MA60 + 法人 60 日）。跨來源聚合（TWSE Web API " +
			"日K/法人 + TWSE-API/TPEx 估值 + MOPS 損益表），_lineage 為 []Lineage 陣列；" +
			"上櫃無歷史 K 線，技術面從缺（note 標註）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol":  map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
				"horizon": map[string]any{"type": "string", "enum": []string{"short", "mid", "long"}, "description": "研判窗口（預設 mid）"},
			},
			"required": []string{"symbol"},
		},
		ReadOnly: true,
		Handler:  handlerGetStockTrendComposite,
	})
}
