package mcp

// registry_bc.go 登錄 §10.B（盤後行情・籌碼）與 §10.C（重大訊息與風險）
// 之工具（parity 任務持續新增）。全部為 POST_MARKET 資料，lineage 由 handler 回報。

import "tw-quant-mcp/pkg/provider"

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
		Symbol:      "get_block_trades_daily",
		Name:        "get_block_trades_daily",
		Description: "查詢集中市場鉅額交易日成交量值統計（TWSE-WEB BFIAUU_d，T042）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDBlockTrades}.handler(),
	})
	r.Register(ToolDef{
		Symbol: "get_block_trades_detail",
		Name:   "get_block_trades_detail",
		Description: "查詢集中市場鉅額交易逐筆明細（含配對交易、盤後鉅額等交易別；TWSE-WEB BFIAUU_d date 查詢，T043）。" +
			"stock_no/name 為本地端過濾；limit 預設 50。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":     map[string]any{"type": "string", "description": "查詢日期 YYYY-MM-DD（需為交易日）"},
				"stock_no": map[string]any{"type": "string", "description": "股票代號（選填）"},
				"name":     map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":    map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset":   map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
			"required": []string{"date"},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDBlockTrades, withDate: true}.handler(),
	})
	r.Register(ToolDef{
		Symbol:      "get_block_trades_monthly",
		Name:        "get_block_trades_monthly",
		Description: "查詢集中市場鉅額交易月成交量值統計（TWSE-WEB BFIAUU_m，T044）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDBlockMonthly}.handler(),
	})
	r.Register(ToolDef{
		Symbol:      "get_block_trades_yearly",
		Name:        "get_block_trades_yearly",
		Description: "查詢集中市場鉅額交易年成交量值統計（TWSE-WEB BFIAUU_y，T045）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDBlockYearly}.handler(),
	})
	r.Register(ToolDef{
		Symbol:      "get_cross_market_trading_info",
		Name:        "get_cross_market_trading_info",
		Description: "查詢每日上市上櫃跨市場成交資訊（T115）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDCrossMarket}.handler(),
	}) // T115
	r.Register(ToolDef{
		Symbol:      "get_daily_day_trading_targets",
		Name:        "get_daily_day_trading_targets",
		Description: "查詢上市股票每日當日沖銷交易標的及統計（T116）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDDayTradeTargets}.handler(),
	}) // T116
	r.Register(ToolDef{
		Symbol: "get_financial_program_abnormal_recommendations",
		Name:   "get_financial_program_abnormal_recommendations",
		Description: "查詢投資理財節目異常推介個股（TWSE-WEB Announcement/BFZFZU_T，T121）。可選 name 過濾；" +
			"無異常推介時官方回「本日無」佔位列。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDFinProgAbn}.handler(),
	}) // T121
	r.Register(ToolDef{
		Symbol:      "get_foreign_companies_applying_for_listing",
		Name:        "get_foreign_companies_applying_for_listing",
		Description: "查詢外國公司向證交所申請第一上市之公司（TWSE-API company/applylistingForeign，T123）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIForeignApply}.handler(),
	}) // T123
	r.Register(ToolDef{
		Symbol:      "get_local_companies_applying_for_listing",
		Name:        "get_local_companies_applying_for_listing",
		Description: "查詢申請上市之本國公司（TWSE-API company/applylistingLocal，T138）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPILocalApply}.handler(),
	}) // T138
	r.Register(ToolDef{
		Symbol:      "get_recently_listed_companies",
		Name:        "get_recently_listed_companies",
		Description: "查詢最近上市公司（TWSE-API company/newlisting，T162）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPINewListing}.handler(),
	}) // T162
	r.Register(ToolDef{
		Symbol:      "get_suspended_listed_companies",
		Name:        "get_suspended_listed_companies",
		Description: "查詢終止上市公司（TWSE-API company/suspendListingCsvAndHtml，T178）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPISuspListing}.handler(),
	}) // T178
	r.Register(ToolDef{
		Symbol:      "get_daily_securities_lending_volume",
		Name:        "get_daily_securities_lending_volume",
		Description: "查詢集中市場借券賣出每日量（T119）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSBLVolume}.handler(),
	}) // T119
	r.Register(ToolDef{
		Symbol:      "get_first_listed_foreign_stocks_daily",
		Name:        "get_first_listed_foreign_stocks_daily",
		Description: "查詢每日第一上市外國股票成交量值（T122）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDFirstForeign}.handler(),
	}) // T122
	r.Register(ToolDef{
		Symbol:      "get_margin_loan_restrictions_announcement",
		Name:        "get_margin_loan_restrictions_announcement",
		Description: "查詢集中市場停資停券預告表（T139）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDMarginRestrict}.handler(),
	}) // T139
	r.Register(ToolDef{
		Symbol:      "get_odd_lot_trading_quotes",
		Name:        "get_odd_lot_trading_quotes",
		Description: "查詢集中市場盤後零股交易行情單（T149）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDOddLot}.handler(),
	}) // T149
	r.Register(ToolDef{
		Symbol:      "get_securities_trading_changes",
		Name:        "get_securities_trading_changes",
		Description: "查詢集中市場證券變更交易（T163）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDTradingChanges}.handler(),
	}) // T163
	r.Register(ToolDef{
		Symbol:      "get_stock_price_changes",
		Name:        "get_stock_price_changes",
		Description: "查詢上市個股股價升降幅（漲跌停參考價；T172）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDPriceChangeLim}.handler(),
	}) // T172
	r.Register(ToolDef{
		Symbol:      "get_stocks_no_price_change_first_five_days",
		Name:        "get_stocks_no_price_change_first_five_days",
		Description: "查詢上市個股首五日無漲跌幅（T175）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDNewList5D}.handler(),
	}) // T175
	r.Register(ToolDef{
		Symbol:      "get_suspended_day_trading_announcement",
		Name:        "get_suspended_day_trading_announcement",
		Description: "查詢暫停先賣後買當日沖銷標的預告表（T176）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSuspDayTradeAnn}.handler(),
	}) // T176
	r.Register(ToolDef{
		Symbol:      "get_suspended_day_trading_history",
		Name:        "get_suspended_day_trading_history",
		Description: "查詢暫停先賣後買當日沖銷交易歷史（T177）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSuspDayTradeHis}.handler(),
	}) // T177
	r.Register(ToolDef{
		Symbol:      "get_suspended_trading_stocks",
		Name:        "get_suspended_trading_stocks",
		Description: "查詢集中市場暫停交易證券（T179）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSuspended}.handler(),
	}) // T179
	r.Register(ToolDef{
		Symbol:      "get_top_20_volume_stocks",
		Name:        "get_top_20_volume_stocks",
		Description: "查詢當日成交量 Top20（TWSE-WEB MI_INDEX20，T184）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDTopVolume}.handler(),
	}) // T184
	r.Register(ToolDef{
		Symbol: "get_etf_regular_investment_ranking",
		Name:   "get_etf_regular_investment_ranking",
		Description: "查詢定期定額交易戶數統計排行月報表（TWSE-WEB ETFReport/ETFRank，T120）。" +
			"每列含排名、股票與 ETF 之代碼/名稱/交易戶數。可選 code/name 過濾（比對股票欄）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":   map[string]any{"type": "string", "description": "股票代號過濾（選填）"},
				"name":   map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDEtfRegInv}.handler(),
	}) // T120
	r.Register(ToolDef{
		Symbol:      "get_market_institutional_amounts_history",
		Name:        "get_market_institutional_amounts_history",
		Description: "查詢外資及陸資/投信/自營商買賣超金額彙總歷史（TWSE-WEB BFI82U，T146）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"date": map[string]any{"type": "string", "description": "查詢日 YYYY-MM-DD（預設最近交易日）"}, "code": map[string]any{"type": "string", "description": "股票代號（選填）"}, "limit": map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"}, "offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"}},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDInstiAmounts, withDate: true}.handler(),
	}) // T146
	r.Register(ToolDef{
		Symbol:      "get_market_turnover_history",
		Name:        "get_market_turnover_history",
		Description: "查詢集中市場每日成交資訊（含週轉率）歷史（TWSE-WEB FMTQIK，T147）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"date": map[string]any{"type": "string", "description": "查詢日 YYYY-MM-DD（預設最近交易日）"}, "code": map[string]any{"type": "string", "description": "股票代號（選填）"}, "limit": map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"}, "offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"}},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDTurnoverHistory, withDate: true}.handler(),
	}) // T147
	r.Register(ToolDef{
		Symbol:      "get_short_sale_lending_balance_history",
		Name:        "get_short_sale_lending_balance_history",
		Description: "查詢信用交易融資融券餘額歷史（TWSE-WEB TWT93U，T164）。可選 code/name 過濾。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"date": map[string]any{"type": "string", "description": "查詢日 YYYY-MM-DD（預設最近交易日）"}, "code": map[string]any{"type": "string", "description": "股票代號（選填）"}, "limit": map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"}, "offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"}},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSLSBalanceHis, withDate: true}.handler(),
	}) // T164
	r.Register(ToolDef{
		Symbol:      "get_short_sale_lending_trades_history",
		Name:        "get_short_sale_lending_trades_history",
		Description: "查詢借券賣出及借券賣出價量歷史（TWSE-WEB TWTASU，T165）。可選 code/name 過濾。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"date": map[string]any{"type": "string", "description": "查詢日 YYYY-MM-DD（預設最近交易日）"}, "code": map[string]any{"type": "string", "description": "股票代號（選填）"}, "limit": map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"}, "offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"}},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDSBLTradesHis, withDate: true}.handler(),
	}) // T165

	r.Register(ToolDef{
		Symbol:      "get_central_depository_bond_redemption",
		Name:        "get_central_depository_bond_redemption",
		Description: "查詢中央登錄公債補息資料表（TWSE-WEB BFI61U，T055）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDBondRedemption}.handler(),
	}) // T055

	r.Register(ToolDef{
		Symbol: "get_companies_cumulative_voting",
		Name:   "get_companies_cumulative_voting",
		Description: "查詢上市公司採累積投票制、全額連記法、候選人提名制選任董監事及當選資料彙總表" +
			"（TWSE-API t187ap34_L，T056）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPICumVoting}.handler(),
	}) // T056

	r.Register(ToolDef{
		Symbol: "get_companies_ownership_changes_business_scope",
		Name:   "get_companies_ownership_changes_business_scope",
		Description: "查詢上市公司經營權及營業範圍異(變)動專區-經營權異動且營業範圍重大變更停止買賣公司" +
			"（TWSE-API t187ap26_L，T057）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIOwnScopeHalt}.handler(),
	}) // T057

	r.Register(ToolDef{
		Symbol: "get_companies_ownership_changes_business_scope_trading",
		Name:   "get_companies_ownership_changes_business_scope_trading",
		Description: "查詢上市公司經營權及營業範圍異(變)動專區-經營權異動且營業範圍重大變更列為變更交易公司" +
			"（TWSE-API t187ap27_L，T058）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIOwnScopeTrade}.handler(),
	}) // T058

	r.Register(ToolDef{
		Symbol: "get_companies_with_business_scope_changes",
		Name:   "get_companies_with_business_scope_changes",
		Description: "查詢上市公司經營權及營業範圍異(變)動專區-營業範圍重大變更公司" +
			"（TWSE-API t187ap25_L，T060）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIScopeChanges}.handler(),
	}) // T060

	r.Register(ToolDef{
		Symbol:      "get_companies_with_independent_directors",
		Name:        "get_companies_with_independent_directors",
		Description: "查詢上市公司獨立董監事兼任情形彙總表（TWSE-API t187ap30_L，T063）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIIndepDirectors}.handler(),
	}) // T063

	r.Register(ToolDef{
		Symbol:      "get_companies_with_ownership_changes",
		Name:        "get_companies_with_ownership_changes",
		Description: "查詢上市公司經營權及營業範圍異(變)動專區-經營權異動公司（TWSE-API t187ap24_L，T064）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "公司名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIOwnershipChange}.handler(),
	}) // T064

	r.Register(ToolDef{
		Symbol: "get_after_hours_trading",
		Name:   "get_after_hours_trading",
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
		Symbol:      "get_company_major_news",
		Name:        "get_company_major_news",
		Description: "查詢上市公司每日重大訊息（MOPS t187ap04_L，T096）。code 選填，指定則僅回傳該公司。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號（選填），例如 \"2330\""},
			},
		},
		ReadOnly: true,
		Handler: func(a *App, args map[string]any) (HandlerResult, error) {
			return handlerGetMajorAnnouncements(a, map[string]any{"symbol": strVal(args["code"])})
		},
	}) // T096

	// ── 行情歷史與指數補齊（T140/T141/T143-T145/T161/T180-T183）──
	listSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		}
	}
	r.Register(ToolDef{
		Symbol:      "get_margin_trading_info",
		Name:        "get_margin_trading_info",
		Description: "查詢信用交易統計（融資融券餘額；TWSE-WEB MI_MARGN tables 型，T140）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDMarginInfo}.handler(),
	}) // T140
	r.Register(ToolDef{
		Symbol:      "get_market_disposal_stocks",
		Name:        "get_market_disposal_stocks",
		Description: "查詢集中市場公布處置股票（TWSE-API announcement/punish 正規化模型，T141）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIPunish}.handler(),
	}) // T141
	for _, e := range []struct{ symbol, desc string }{
		{"get_market_historical_index", "查詢加權指數歷史資料（每 5 分鐘軌跡；TWSE-WEB MI_5MINS_HIST，T143）"}, // T143
		{"get_taiex_index_history", "查詢發行量加權股價指數歷史資料（TWSE-WEB MI_5MINS_HIST，T180）"},         // T180
	} {
		r.Register(ToolDef{
			Symbol:      e.symbol,
			Name:        e.symbol,
			Description: e.desc + "。",
			Schema:      listSchema(),
			ReadOnly:    true,
			Handler:     webListSpec{ds: provider.TWSEWDIndexHistory}.handler(),
		})
	}
	r.Register(ToolDef{
		Symbol:      "get_market_holiday_schedule",
		Name:        "get_market_holiday_schedule",
		Description: "查詢有價證券集中交易市場開（休）市日期（TWSE-WEB holidaySchedule，T144）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDHoliday}.handler(),
	}) // T144
	r.Register(ToolDef{
		Symbol:      "get_market_index_info",
		Name:        "get_market_index_info",
		Description: "查詢每日市場各類指數行情明細（TWSE-API MI_INDEX 正規化模型，T145）。可選 name 過濾。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "指數名稱關鍵字（選填）"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIIndices}.handler(),
	}) // T145

	codeSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"code"},
		}
	}
	r.Register(ToolDef{
		Symbol:      "get_stock_daily_trading",
		Name:        "get_stock_daily_trading",
		Description: "根據股票代號查詢個股日成交資訊（TWSE-API STOCK_DAY_ALL 正規化模型，T166）。",
		Schema:      codeSchema(),
		ReadOnly:    true,
		Handler:     apiCompanySpec{ds: provider.TWSEAPIDailyClose}.handler(),
	}) // T166
	webCodeSchema := func(withDate bool) map[string]any {
		props := map[string]any{
			"code":   map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1},
			"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0},
		}
		if withDate {
			props["date"] = map[string]any{"type": "string", "description": "查詢日期 YYYY-MM-DD（預設今日）"}
		}
		return map[string]any{"type": "object", "properties": props}
	}
	r.Register(ToolDef{
		Symbol:      "get_stock_monthly_average",
		Name:        "get_stock_monthly_average",
		Description: "根據股票代號過濾個股日收盤價及月平均價全市場報表（TWSE-WEB STOCK_DAY_AVG_ALL，T168）。",
		Schema:      webCodeSchema(false),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDMonthlyAvgAll}.handler(),
	}) // T168
	r.Register(ToolDef{
		Symbol:      "get_stock_monthly_avg_history",
		Name:        "get_stock_monthly_avg_history",
		Description: "查詢個股月平均價歷史（指定年度逐月；TWSE-WEB STOCK_DAY_AVG，T169）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stock_no": map[string]any{"type": "string", "description": "股票代號"},
				"date":     map[string]any{"type": "string", "description": "任意日期 YYYYMMDD（僅年份有效）"},
			},
			"required": []string{"stock_no"},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDMonthlyAvg, withDate: true}.handler(),
	}) // T169
	r.Register(ToolDef{
		Symbol:      "get_stock_monthly_history",
		Name:        "get_stock_monthly_history",
		Description: "查詢個股月 K 歷史（指定年度逐月行情；TWSE-WEB STOCK_DAY，T170）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stock_no": map[string]any{"type": "string", "description": "股票代號"},
				"date":     map[string]any{"type": "string", "description": "任意日期 YYYYMMDD（僅年份有效）"},
			},
			"required": []string{"stock_no"},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDDailyK, withDate: true}.handler(),
	}) // T170
	r.Register(ToolDef{
		Symbol: "get_stock_monthly_trading",
		Name:   "get_stock_monthly_trading",
		Description: "根據股票代號查詢個股月成交資訊（TWSE-WEB FMSRFK，T171）。" +
			"code 必填：上游以單一個股報表提供，不支援全市場查詢。",
		Schema: func() map[string]any {
			s := webCodeSchema(true)
			s["required"] = []string{"code"}
			return s
		}(),
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDStockMonTrade, withDate: true}.handler(),
	}) // T171
	r.Register(ToolDef{
		Symbol:      "get_stock_yearly_history",
		Name:        "get_stock_yearly_history",
		Description: "查詢個股歷年成交資訊彙總（每年一筆長期彙總；TWSE-WEB FMNPTK，T173）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stock_no": map[string]any{"type": "string", "description": "股票代號"},
			},
			"required": []string{"stock_no"},
		},
		ReadOnly: true,
		Handler:  webListSpec{ds: provider.TWSEWDStockYearHis, withDate: true}.handler(),
	}) // T173
	r.Register(ToolDef{
		Symbol:      "get_stock_yearly_trading",
		Name:        "get_stock_yearly_trading",
		Description: "根據股票代號過濾年度成交資訊全市場報表（TWSE-WEB FMNPTK_ALL，T174）。",
		Schema:      webCodeSchema(false),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDStockYearTrade}.handler(),
	}) // T174
	r.Register(ToolDef{
		Symbol:      "get_top_foreign_holdings",
		Name:        "get_top_foreign_holdings",
		Description: "查詢外資持股前 20 名上市公司（TWSE-API MI_QFIIS_sort_20 passthrough，T185）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPITopForeign}.handler(),
	}) // T185
	r.Register(ToolDef{
		Symbol:      "get_twse_news",
		Name:        "get_twse_news",
		Description: "查詢證交所新聞清單（TWSE-API news/newsList passthrough，T186）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"start_date": map[string]any{"type": "string", "description": "起始日期 YYYYMMDD（選填）"},
				"end_date":   map[string]any{"type": "string", "description": "結束日期 YYYYMMDD（選填）"},
			},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPITwseNews}.handler(),
	}) // T186
	warrantCodeSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "權證或標的代號（選填）"},
			},
		}
	}
	warrantBasicSchema := func() map[string]any {
		return map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code":   map[string]any{"type": "string", "description": "權證代號或標的證券代號（選填；如 2330 查其標的權證）"},
				"limit":  map[string]any{"type": "integer", "default": 100, "minimum": 1, "description": "回傳筆數上限（code 省略時預設 100）"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		}
	}
	r.Register(ToolDef{
		Symbol:      "get_warrant_basic_info",
		Name:        "get_warrant_basic_info",
		Description: "查詢權證基本資料（TWSE-API t187ap37_L passthrough，T187）。code 選填：可為權證代號或標的證券代號（經名稱比對）；省略時分頁回傳全部。",
		Schema:      warrantBasicSchema(),
		ReadOnly:    true,
		Handler:     handlerGetWarrantBasicInfo,
	}) // T187
	r.Register(ToolDef{
		Symbol:      "get_warrant_daily_trading",
		Name:        "get_warrant_daily_trading",
		Description: "根據股票代號查詢權證每日成交資訊（TWSE-API t187ap42_L 正規化模型，T188）。code 選填過濾。",
		Schema:      warrantCodeSchema(),
		ReadOnly:    true,
		Handler:     handlerGetWarrantActivity,
	}) // T188
	r.Register(ToolDef{
		Symbol:      "get_warrant_trader_count",
		Name:        "get_warrant_trader_count",
		Description: "查詢權證流動量提供者報價方式統計（TWSE-API t187ap43_L passthrough，T189）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIWarrantTrader}.handler(),
	}) // T189
	r.Register(ToolDef{
		Symbol:      "get_warrant_yearly_issuance_statistics",
		Name:        "get_warrant_yearly_issuance_statistics",
		Description: "查詢權證年度發行統計（TWSE-API t187ap36_L passthrough，T190）。",
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		ReadOnly: true,
		Handler:  apiListSpec{ds: provider.TWSEAPIWarrantIssue}.handler(),
	}) // T190
	r.Register(ToolDef{
		Symbol: "get_twse_events",
		Name:   "get_twse_events",
		Description: "查詢證交所活動訊息（業績發表會、產業講座等活動公告；TWSE-API news/eventList，T191）。" +
			"top 為回傳筆數上限（預設 10，填 0 回傳全部）。每筆含 No（序號）、Title（標題）、Details（詳情連結）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"top": map[string]any{"type": "integer", "minimum": 0, "default": 10, "description": "回傳筆數上限（預設 10；填 0 則回傳全部）"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetTwseEvents,
	}) // T191
	r.Register(ToolDef{
		Symbol: "get_all_stocks_daily_close",
		Name:   "get_all_stocks_daily_close",
		Description: "查詢指定日期全部上市股票的每日收盤行情（開高低收、成交量、本益比；TWSE-WEB MI_INDEX，T192）。" +
			"與 get_stock_daily_quote（單一股票跨日）互補：此工具是「單一日期查全市場」快照。" +
			"date 需為交易日；stock_no/name 為本地端過濾；limit/offset 分頁（預設 50）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"date":     map[string]any{"type": "string", "description": "查詢日期 YYYY-MM-DD 或 YYYYMMDD（需為交易日），例如 \"2026-08-25\""},
				"stock_no": map[string]any{"type": "string", "description": "股票代號（選填），指定則只回傳該股票"},
				"name":     map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"limit":    map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限（預設 50）"},
				"offset":   map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
			"required": []string{"date"},
		},
		ReadOnly: true,
		Handler:  handlerGetAllStocksDailyClose,
	}) // T192
	r.Register(ToolDef{
		Symbol: "get_abnormal_accumulated_notice_stocks",
		Name:   "get_abnormal_accumulated_notice_stocks",
		Description: "查詢集中市場公布注意累計次數異常資訊（TWSE-API announcement/notetrans，T193）。" +
			"與 get_attention_disposition_stocks（當日注意/處置清單）互補：本工具揭露近期符合注意處理標準之累計紀錄，適合風險掃描與短線避雷。" +
			"清單含權證（kind 可過濾 stock/warrant）；name 關鍵字過濾；limit 預設 50、offset 分頁。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "description": "股票名稱關鍵字（選填）"},
				"kind":   map[string]any{"type": "string", "enum": []string{"stock", "warrant"}, "description": "標的類型過濾（選填）：stock=普通股、warrant=權證；省略回傳全部"},
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限（預設 50）"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetAbnormalAccumulatedNoticeStocks,
	}) // T193

	r.Register(ToolDef{
		Symbol:      "get_company_sec_regulatory_penalties",
		Name:        "get_company_sec_regulatory_penalties",
		Description: "根據股票代號查詢上市公司金管會證券期貨局裁罰案件專區（TWSE-API t187ap22_L passthrough，T106）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"code": map[string]any{"type": "string", "description": "股票代號，例如 \"2330\""},
			},
			"required": []string{"code"},
		},
		ReadOnly: true,
		Handler:  apiCompanySpec{ds: provider.TWSEAPISecPenalty, skipRegistryCheck: true}.handler(),
	}) // T106
	r.Register(ToolDef{
		Symbol:      "get_real_time_trading_stats",
		Name:        "get_real_time_trading_stats",
		Description: "查詢每 5 秒委託成交統計（盤中即時；TWSE-WEB MI_5MINS，T161）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDRealTimeStats}.handler(),
	}) // T161
	r.Register(ToolDef{
		Symbol:      "get_taiwan_50_index_history",
		Name:        "get_taiwan_50_index_history",
		Description: "查詢臺灣50指數歷史資料（TWSE-WEB TAI50I，T181）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDTaiwan50}.handler(),
	}) // T181
	r.Register(ToolDef{
		Symbol:      "get_taiwan_island_index_history",
		Name:        "get_taiwan_island_index_history",
		Description: "查詢寶島股價指數歷史資料（TWSE-WEB FRMSA，T182）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDIslandIndex}.handler(),
	}) // T182
	r.Register(ToolDef{
		Symbol:      "get_taiwan_total_return_index",
		Name:        "get_taiwan_total_return_index",
		Description: "查詢發行量加權股價報酬指數歷史資料（TWSE-WEB MFI94U，T183）。",
		Schema:      listSchema(),
		ReadOnly:    true,
		Handler:     webListSpec{ds: provider.TWSEWDTotalReturn}.handler(),
	}) // T183
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

	// ── 上櫃市場（T155/T156/T157）──
	r.Register(ToolDef{
		Symbol: "get_otc_daily",
		Name:   "get_otc_daily",
		Description: "查詢上櫃（OTC）市場當日所有股票收盤行情（TPEx-API tpex_mainboard_daily_close_quotes，T155）。" +
			"stock_no 選填，指定則只回傳該股票。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stock_no": map[string]any{"type": "string", "description": "股票代號（選填），若指定則只回傳該股票的收盤行情"},
				"limit":    map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset":   map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetOtcDaily,
	}) // T155
	r.Register(ToolDef{
		Symbol:      "get_otc_index",
		Name:        "get_otc_index",
		Description: "查詢櫃買市場（上櫃）指數歷史行情，包含開高低收、漲跌幅（TPEx-API tpex_index，T156）。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetOtcIndex,
	}) // T156
	r.Register(ToolDef{
		Symbol: "get_otc_odd_lot",
		Name:   "get_otc_odd_lot",
		Description: "查詢上櫃零股（不足一張）交易行情，包含零股成交價、成交量、成交金額（" +
			"TPEx-API tpex_odd_stock，T157）。stock_no 選填，指定則只回傳該股票。",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"stock_no": map[string]any{"type": "string", "description": "股票代號（選填），若指定則只回傳該股票的零股資料"},
				"limit":    map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
				"offset":   map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
			},
		},
		ReadOnly: true,
		Handler:  handlerGetOtcOddLot,
	}) // T157
}
