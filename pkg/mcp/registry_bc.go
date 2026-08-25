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
		Handler: webListSpec{ds: provider.TWSEWDBlockTrades}.handler(),
	})
	r.Register(ToolDef{
		Symbol:      "get_block_trades_detail",
		Name:        "get_block_trades_detail",
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
		Symbol:      "get_market_institutional_amounts_history",
		Name:        "get_market_institutional_amounts_history",
		Description: "查詢外資及陸資/投信/自營商買賣超金額彙總歷史（TWSE-WEB BFI82U，T146）。",
		Schema: map[string]any{
			"type": "object",
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
			"type": "object",
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
			"type": "object",
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
			"type": "object",
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
		Symbol:      "get_companies_cumulative_voting",
		Name:        "get_companies_cumulative_voting",
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
		Symbol:      "get_companies_ownership_changes_business_scope",
		Name:        "get_companies_ownership_changes_business_scope",
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
		Symbol:      "get_companies_ownership_changes_business_scope_trading",
		Name:        "get_companies_ownership_changes_business_scope_trading",
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
		Symbol:      "get_companies_with_business_scope_changes",
		Name:        "get_companies_with_business_scope_changes",
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
