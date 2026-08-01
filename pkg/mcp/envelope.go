package mcp

import (
	"fmt"
	"strconv"

	"tw-quant-mcp/pkg/model"
)

// ChartOption 控制 §11 圖表注入行為。
type ChartOption struct {
	// Chart 是否注入 _chart_meta（預設 true）。
	Chart bool
	// Limit 時間序列長度上限（預設 200）。
	Limit int
}

// DefaultChartOption 回傳 chart=true、limit=200。
func DefaultChartOption() ChartOption {
	return ChartOption{Chart: true, Limit: 200}
}

// ParseChartOption 自工具參數解析 chart/limit（兩者皆選填）。
// chart 接受布林（true/false）；limit 接受正整數。
func ParseChartOption(args map[string]any) (ChartOption, error) {
	opt := DefaultChartOption()
	if v, ok := args["chart"]; ok {
		b, ok := v.(bool)
		if !ok {
			return opt, fmt.Errorf("mcp: 參數 chart 必須為布林")
		}
		opt.Chart = b
	}
	if v, ok := args["limit"]; ok {
		n, err := asInt(v)
		if err != nil || n < 1 {
			return opt, fmt.Errorf("mcp: 參數 limit 必須為正整數")
		}
		opt.Limit = n
	}
	return opt, nil
}

// asInt 將 JSON number/string 轉為 int。
func asInt(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case string:
		return strconv.Atoi(n)
	}
	return 0, fmt.Errorf("非數字型別 %T", v)
}

// ChartUpdater 於 response shaping 階段為 Envelope 注入 _chart_meta。
// 各工具型別實作注入規則（§11）；不適用之資料型別可保留 ChartMeta 為 nil。
type ChartUpdater interface {
	// UpdateEnvelope 依工具與回傳資料更新 env.ChartMeta。
	UpdateEnvelope(env *model.Envelope, def *ToolDef, opt ChartOption, data any) error
}

// KlineChartMeta 產出 K 線圖表之 §11.2 標準描述（candlestick）。
func KlineChartMeta(limit int) map[string]any {
	return map[string]any{
		"recommended_type": "candlestick",
		"x_axis": map[string]any{
			"key":    "timestamp",
			"type":   "datetime",
			"format": "HH:mm",
		},
		"y_axis": map[string]any{
			"keys":       []string{"open", "high", "low", "close"},
			"title":      "價格 (元)",
			"right_axis": []string{"volume"},
		},
		"series": []map[string]any{
			{"key": "volume", "type": "bar", "style": "volume"},
		},
		"annotations": []any{},
		"note":        fmt.Sprintf("限 %d 根；Candle[] 之 timestamp 格式 HH:mm:00", limit),
	}
}

// defaultChartUpdater 提供 §11 內建之圖表描述。
type defaultChartUpdater struct{}

// UpdateEnvelope 依工具型別注入（§11.3）：
//   - 盤中 K 線：candlestick（limit 依 ChartOption.Limit，handler 已於
//     data 上套用同一限制）
//   - 盤後日 K：candlestick（datetime 為 YYYY-MM-DD）
//   - 報價/指標：line（單日值亦提供，x=date）
//   - 法人/籌碼：bar（正負分色）
//   - 外資產業配置：pie
//   - 外資持股歷史：line（時間趨勢）
//   - 權證活躍度：bar
//   - 市場彙總：bar（漲跌家數）
//   - 其餘（掃描/清單）：非時間序列，依 §11.1 原則不注入。
func (defaultChartUpdater) UpdateEnvelope(env *model.Envelope, def *ToolDef, opt ChartOption, data any) error {
	switch def.Name {
	case "get_intraday_kline":
		env.ChartMeta = KlineChartMeta(opt.Limit)
	case "get_stock_daily_kline":
		env.ChartMeta = DailyKlineChartMeta()
	case "get_stock_daily_quote":
		env.ChartMeta = lineChart("收盤價 (元)", "date", "close")
	case "get_institutional_investors":
		env.ChartMeta = barChart("三大法人買賣超 (股)", "code", "foreign_net")
	case "get_margin_trading":
		env.ChartMeta = barChart("融資融券餘額", "code", "margin_balance")
	case "get_foreign_industry_holdings":
		env.ChartMeta = pieChart("外資產業配置", "industry", "foreign_share")
	case "get_foreign_shareholding_history":
		env.ChartMeta = lineChart("外資持股比率 (%)", "date", "foreign_percent")
	case "get_warrant_activity":
		env.ChartMeta = barChart("權證成交金額 (元)", "code", "amount")
	case "get_market_summary":
		env.ChartMeta = barChart("漲跌家數", "tse|otc", "advancers")
	case "get_monthly_revenue":
		env.ChartMeta = barChart("月營收 (元)", "data_year_month", "revenue")
	case "get_dividend_history":
		env.ChartMeta = barChart("每股現金股利 (元/股)", "dividend_year", "cash_dividend")
	case "screen_stocks":
		env.ChartMeta = scatterChart("PE / 殖利率散佈", "pe", "dividend_yield_pct", "pb")
	case "screen_high_yield":
		env.ChartMeta = scatterChart("殖利率 / PE 散佈", "pe", "dividend_yield_pct", "dividend_per_share")
	case "get_financial_health_check":
		env.ChartMeta = radarChart("財務健康五面向", []string{"profit", "growth", "structure", "dividend", "governance"})
	case "get_futures_daily_ohlc", "get_futures_history":
		env.ChartMeta = FuturesKlineChartMeta()
	case "get_put_call_ratio":
		env.ChartMeta = PCRLineChartMeta()
	case "get_institutional_futures_positions", "get_institutional_options_positions":
		env.ChartMeta = barChart("三大法人淨口數 (口)", "investor", "net_volume")
	case "get_institutional_futures_history":
		env.ChartMeta = lineChart("三大法人期貨淨口數 (口)", "date", "net_volume")
	case "get_large_trader_positions":
		env.ChartMeta = barChart("大額交易人前五大買方 (口)", "contract", "top5_long")
	}
	return nil
}

// FuturesKlineChartMeta 產出期貨 K 線之 §11.2 標準描述（§11.3 期貨 candlestick）。
func FuturesKlineChartMeta() map[string]any {
	return map[string]any{
		"recommended_type": "candlestick",
		"x_axis": map[string]any{
			"key":    "timestamp",
			"type":   "datetime",
			"format": "YYYY-MM-DD",
		},
		"y_axis": map[string]any{
			"keys":       []string{"open", "high", "low", "close"},
			"title":      "價格 (點)",
			"right_axis": []string{"volume"},
		},
		"series": []map[string]any{
			{"key": "volume", "type": "bar", "style": "volume"},
		},
		"annotations": []any{},
		"note":        "FuturesDailyRow 之 date 欄位為 YYYY-MM-DD（契約月份見 contract_month）",
	}
}

// PCRLineChartMeta 產出 Put/Call Ratio 之 §11.2 標準描述
// （§11.3 line + 多空分界線 1.0 annotation，§T015 備註）。
func PCRLineChartMeta() map[string]any {
	return map[string]any{
		"recommended_type": "line",
		"x_axis": map[string]any{
			"key":  "date",
			"type": "category",
		},
		"y_axis": map[string]any{
			"keys":  []string{"volume_ratio"},
			"title": "買賣權成交量比 (%)",
		},
		"annotations": []any{
			map[string]any{"type": "hline", "value": 1.0, "label": "多空分界"},
		},
		"note": "volume_ratio 單位 %（100 = 多空均衡；annotation 1.0 為對數刻度之分界）",
	}
}

// scatterChart 產出散佈圖之 §11.2 標準描述（§11.3 篩選結果）。
func scatterChart(title, xKey, yKey, sizeKey string) map[string]any {
	return map[string]any{
		"recommended_type": "scatter",
		"x_axis": map[string]any{
			"key":  xKey,
			"type": "value",
		},
		"y_axis": map[string]any{
			"key":   yKey,
			"title": title,
		},
		"series": []map[string]any{
			{"key": sizeKey, "type": "bubble"},
		},
		"annotations": []any{},
	}
}

// radarChart 產出雷達圖之 §11.2 標準描述（§11.3 財報五面向）。
func radarChart(title string, keys []string) map[string]any {
	return map[string]any{
		"recommended_type": "radar",
		"axes":             keys,
		"series": []map[string]any{
			{"type": "radar"},
		},
		"title":       title,
		"annotations": []any{},
	}
}

// DailyKlineChartMeta 產出盤後日 K 之 §11.2 標準描述（candlestick）。
func DailyKlineChartMeta() map[string]any {
	return map[string]any{
		"recommended_type": "candlestick",
		"x_axis": map[string]any{
			"key":    "timestamp",
			"type":   "datetime",
			"format": "YYYY-MM-DD",
		},
		"y_axis": map[string]any{
			"keys":       []string{"open", "high", "low", "close"},
			"title":      "價格 (元)",
			"right_axis": []string{"volume"},
		},
		"series": []map[string]any{
			{"key": "volume", "type": "bar", "style": "volume"},
		},
		"annotations": []any{},
		"note":        "Candle[] 之 timestamp 格式 YYYY-MM-DD（盤後日/週/月 K）",
	}
}

// lineChart 產出線圖之 §11.2 標準描述。
func lineChart(title, xKey, yKey string) map[string]any {
	return map[string]any{
		"recommended_type": "line",
		"x_axis": map[string]any{
			"key":  xKey,
			"type": "category",
		},
		"y_axis": map[string]any{
			"keys":  []string{yKey},
			"title": title,
		},
		"annotations": []any{},
	}
}

// barChart 產出長條圖之 §11.2 標準描述。
func barChart(title, xKey, yKey string) map[string]any {
	return map[string]any{
		"recommended_type": "bar",
		"x_axis": map[string]any{
			"key":  xKey,
			"type": "category",
		},
		"y_axis": map[string]any{
			"keys":  []string{yKey},
			"title": title,
		},
		"annotations": []any{},
	}
}

// pieChart 產出圓餅圖之 §11.2 標準描述。
func pieChart(title, nameKey, valueKey string) map[string]any {
	return map[string]any{
		"recommended_type": "pie",
		"series": map[string]any{
			"name_key":  nameKey,
			"value_key": valueKey,
			"title":     title,
			"aggregate": "sum",
		},
		"annotations": []any{},
	}
}
