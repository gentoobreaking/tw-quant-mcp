package mcp

import (
	"fmt"
	"strconv"

	"tw-quant-mcp/pkg/chart"
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
// 各工具型別之對應為 §11.3 表（唯一真值，pkg/chart.ForTool）。
type ChartUpdater interface {
	// UpdateEnvelope 依工具與回傳資料更新 env.ChartMeta。
	UpdateEnvelope(env *model.Envelope, def *ToolDef, opt ChartOption, data any) error
}

// defaultChartUpdater 依 §11.3 表委派 pkg/chart 產出圖表描述；
// 未對應之工具（非時間序列，§11.1）不注入（ChartMeta 為 nil，omitempty）。
type defaultChartUpdater struct{}

// UpdateEnvelope 依工具型別注入（§11.3，pkg/chart 為唯一真值）：
//   - K 線（盤中/日線/期貨）：candlestick；limit 依 ChartOption.Limit，
//     handler 已於 data 上套用同一限制
//   - 報價/持股/期貨淨部位歷史：line
//   - PCR：line + 多空分界線 1.0 annotation
//   - 法人/籌碼/營收等：bar（正負分色）
//   - 外資產業配置：pie
//   - 篩選：scatter；財報五面向：radar
//   - 其餘（掃描/清單）：非時間序列，依 §11.1 原則不注入。
func (defaultChartUpdater) UpdateEnvelope(env *model.Envelope, def *ToolDef, opt ChartOption, _ any) error {
	env.ChartMeta = chart.ForTool(def.Name, opt.Limit)
	return nil
}
