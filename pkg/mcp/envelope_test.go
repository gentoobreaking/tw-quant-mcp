package mcp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tw-quant-mcp/pkg/chart"
	"tw-quant-mcp/pkg/model"
)

// §12.7 JSON 最小化驗收：chart=false 不輸出 _chart_meta；empty 欄位由 omitempty 省略；
// 結果直接以 Envelope 序列化（無中間 map 結構）。
func TestEnvelopeJSONMinimal(t *testing.T) {
	env := &model.Envelope{
		Data: []model.Candle{
			{Open: 900, High: 910, Low: 895, Close: 905, Volume: 1000},
		},
		Lineage: model.Lineages{Lineage: model.Lineage{
			Source:      model.SourceTWSEMIS,
			SourceRole:  model.SourceRoleRealtime,
			DataDate:    "2026-07-31",
			Freshness:   model.FreshnessRealtimeIntraday,
			SamplingSec: 8,
			FetchedAt:   model.NewTaipeiTime(time.Date(2026, 7, 31, 9, 30, 0, 0, model.Taipei())),
		}},
		HTTPCalls: 0,
	}
	// chart=false：ChartMeta 為 nil → omitempty 應省略
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, absent := range []string{`"_chart_meta"`, `"derived_from"`, `"source_url"`} {
		if strings.Contains(s, absent) {
			t.Errorf("chart=false/無來源時不應輸出 %s，實際: %s", absent, s)
		}
	}
	// http_calls 為 instrumentation 欄位：即使為 0 亦輸出（驗收基準）
	if !strings.Contains(s, `"http_calls":0`) {
		t.Errorf("應輸出 http_calls:0（instrumentation），實際: %s", s)
	}
	// 序列化結果為 Normalized Model 直接輸出（無中間 map 鍵）
	for _, want := range []string{`"open"`, `"high"`, `"close"`, `"volume"`} {
		if !strings.Contains(s, want) {
			t.Errorf("資料應為 Normalized Candle 欄位，缺少 %s: %s", want, s)
		}
	}

	// chart=true：注入 Meta 後省略其 empty 子欄位（x_axis/axes/series…）
	meta := &chart.Meta{RecommendedType: "candlestick"}
	env.ChartMeta = meta
	raw, err = json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	s = string(raw)
	if !strings.Contains(s, `"_chart_meta"`) {
		t.Error("chart=true 應輸出 _chart_meta")
	}
	for _, absent := range []string{`"x_axis"`, `"axes"`, `"series"`, `"annotations"`, `"note"`} {
		if strings.Contains(s, absent) {
			t.Errorf("Meta 空子欄位應由 omitempty 省略，實際含 %s: %s", absent, s)
		}
	}
}
