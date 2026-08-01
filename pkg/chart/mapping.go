package chart

import "fmt"

// ForTool 依 §11.3「圖表類型對應」回傳工具之 _chart_meta（唯一真值，
// T016）。limit 為時間序列長度上限（K 線工具之 note）。
// 未對應之工具回傳 nil（§11.1 非時間序列不注入）。
func ForTool(tool string, limit int) *Meta {
	switch tool {
	// candlestick：任何 K 線（盤中/日線/期貨）
	case "get_intraday_kline":
		return Candlestick(WithNote(fmtNoteKlineLimit(limit)))
	case "get_stock_daily_kline":
		return Candlestick(WithXFormat("YYYY-MM-DD"),
			WithNote("Candle[] 之 timestamp 格式 YYYY-MM-DD（盤後日/週/月 K）"))
	case "get_futures_daily_ohlc", "get_futures_history":
		return Candlestick(WithXKey("date"), WithXFormat("YYYY-MM-DD"),
			WithYTitle("價格 (點)"),
			WithNote("FuturesDailyRow 之 date 欄位為 YYYY-MM-DD（契約月份見 contract_month）"))

	// line：指數/股價趨勢
	case "get_stock_daily_quote":
		return Line("收盤價 (元)", "date", "close")
	case "get_foreign_shareholding_history":
		return Line("外資持股比率 (%)", "date", "foreign_percent")
	case "get_institutional_futures_history":
		return Line("三大法人期貨淨口數 (口)", "date", "net_volume")

	// line + annotation：Put/Call Ratio 多空分界線 1.0（§11.3）
	case "get_put_call_ratio":
		return Line("買賣權成交量比 (%)", "date", "volume_ratio",
			WithAnnotations(HLine(1.0, "多空分界")),
			WithNote("volume_ratio 單位 %（100 = 多空均衡；annotation 1.0 為對數刻度之分界）"))

	// bar（正負分色）：法人/融資融券/營收等
	case "get_institutional_investors":
		return Bar("三大法人買賣超 (股)", "code", "foreign_net")
	case "get_margin_trading":
		return Bar("融資融券餘額", "code", "margin_balance")
	case "get_warrant_activity":
		return Bar("權證成交金額 (元)", "code", "amount")
	case "get_market_summary":
		return Bar("漲跌家數", "tse|otc", "advancers")
	case "get_monthly_revenue":
		return Bar("月營收 (元)", "data_year_month", "revenue")
	case "get_dividend_history":
		return Bar("每股現金股利 (元/股)", "dividend_year", "cash_dividend")
	case "get_institutional_futures_positions", "get_institutional_options_positions":
		return Bar("三大法人淨口數 (口)", "investor", "net_volume")
	case "get_large_trader_positions":
		return Bar("大額交易人前五大買方 (口)", "contract", "top5_long")

	// pie：產業配置/權重（§11.3 heatmap 或 pie）
	case "get_foreign_industry_holdings":
		return Pie("外資產業配置", "industry", "foreign_share")

	// scatter：篩選結果（PE/PB/殖利率）
	case "screen_stocks":
		return Scatter("PE / 殖利率散佈", "pe", "dividend_yield_pct", "pb")
	case "screen_high_yield":
		return Scatter("殖利率 / PE 散佈", "pe", "dividend_yield_pct", "dividend_per_share")

	// radar：個股財報五面向
	case "get_financial_health_check":
		return Radar("財務健康五面向",
			[]string{"profit", "growth", "structure", "dividend", "governance"})
	}
	return nil
}

// fmtNoteKlineLimit 產出 K 線 limit 之說明文字。
func fmtNoteKlineLimit(limit int) string {
	return fmt.Sprintf("限 %d 根；Candle[] 之 timestamp 格式 HH:mm:00", limit)
}
