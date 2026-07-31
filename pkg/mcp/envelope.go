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

// UpdateEnvelope 依工具型別注入：
//   - kline：candlestick 描述（limit 依 ChartOption.Limit，handler 已於
//     data 上套用同一限制）
//   - 其餘（報價/指標/掃描）：非時間序列，依 §11.1 原則不注入。
func (defaultChartUpdater) UpdateEnvelope(env *model.Envelope, def *ToolDef, opt ChartOption, data any) error {
	switch def.Name {
	case "get_intraday_kline":
		env.ChartMeta = KlineChartMeta(opt.Limit)
	}
	return nil
}
