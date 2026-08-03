package chart

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// TestCandlestickMeta 驗證 K 線結構（§11.2 + §11.3 candlestick）。
func TestCandlestickMeta(t *testing.T) {
	m := Candlestick(WithNote("限 200 根"))
	if m.RecommendedType != "candlestick" {
		t.Errorf("recommended_type 應為 candlestick，實際 %s", m.RecommendedType)
	}
	if m.XAxis == nil || m.XAxis.Key != "timestamp" || m.XAxis.Type != "datetime" || m.XAxis.Format != "HH:mm" {
		t.Errorf("x_axis 應為 timestamp/datetime/HH:mm，實際 %+v", m.XAxis)
	}
	if m.YAxis == nil || len(m.YAxis.Keys) != 4 {
		t.Fatalf("y_axis.keys 應為 OHLC 四欄，實際 %+v", m.YAxis)
	}
	if m.YAxis.Keys[0] != "open" || m.YAxis.Keys[3] != "close" {
		t.Errorf("y_axis.keys 應為 open/high/low/close，實際 %v", m.YAxis.Keys)
	}
	if len(m.YAxis.RightAxis) != 1 || m.YAxis.RightAxis[0] != "volume" {
		t.Errorf("volume 應置於 right_axis 輔助軸，實際 %v", m.YAxis.RightAxis)
	}
	if len(m.Series) != 1 || m.Series[0].Key != "volume" || m.Series[0].Style != "volume" {
		t.Errorf("series 應為 volume bar，實際 %+v", m.Series)
	}
	if m.Note != "限 200 根" {
		t.Errorf("note 應為 限 200 根，實際 %s", m.Note)
	}
}

// TestCandlestickOptions 驗證 WithXKey/WithXFormat/WithYTitle（期貨 K 線）。
func TestCandlestickOptions(t *testing.T) {
	m := Candlestick(WithXKey("date"), WithXFormat("YYYY-MM-DD"), WithYTitle("價格 (點)"))
	if m.XAxis.Key != "date" || m.XAxis.Format != "YYYY-MM-DD" {
		t.Errorf("x_axis 應覆寫為 date/YYYY-MM-DD，實際 %+v", m.XAxis)
	}
	if m.YAxis.Title != "價格 (點)" {
		t.Errorf("y_axis.title 應覆寫為 價格 (點)，實際 %s", m.YAxis.Title)
	}
}

// TestBarDiverging 驗證 bar 之正負分色描述（§11.3 法人/融資融券/營收）。
func TestBarDiverging(t *testing.T) {
	m := Bar("月營收 (元)", "data_year_month", "revenue")
	if m.RecommendedType != "bar" {
		t.Errorf("recommended_type 應為 bar，實際 %s", m.RecommendedType)
	}
	if len(m.Series) != 1 || m.Series[0].Style != "diverging" {
		t.Errorf("bar 應標記 diverging（正負分色），實際 %+v", m.Series)
	}
	if m.XAxis == nil || m.XAxis.Key != "data_year_month" || m.XAxis.Type != "category" {
		t.Errorf("x_axis 應為 data_year_month/category，實際 %+v", m.XAxis)
	}
}

// TestLineAndAnnotation 驗證 line + hline annotation（§11.3 PCR 分界線）。
func TestLineAndAnnotation(t *testing.T) {
	m := Line("買賣權成交量比 (%)", "date", "volume_ratio",
		WithAnnotations(HLine(1.0, "多空分界")))
	if m.RecommendedType != "line" {
		t.Errorf("recommended_type 應為 line，實際 %s", m.RecommendedType)
	}
	if len(m.Annotations) != 1 {
		t.Fatalf("應有 1 個 annotation，實際 %v", m.Annotations)
	}
	a := m.Annotations[0]
	if a.Type != "hline" || a.Value != float64(1) || a.Label != "多空分界" {
		t.Errorf("annotation 應為 hline 1.0 多空分界，實際 %+v", a)
	}
}

// TestHeatmapAndPie 驗證產業配置兩類型（§11.3 heatmap 或 pie）。
func TestHeatmapAndPie(t *testing.T) {
	hm := Heatmap("產業權重", "industry", "weight")
	if hm.RecommendedType != "heatmap" || hm.XAxis == nil || hm.XAxis.Key != "industry" {
		t.Errorf("heatmap 結構錯誤: %+v", hm)
	}
	if hm.YAxis == nil || hm.YAxis.Key != "weight" {
		t.Errorf("heatmap y_axis 應為 weight，實際 %+v", hm.YAxis)
	}
	pie := Pie("外資產業配置", "industry", "foreign_share")
	if pie.RecommendedType != "pie" || pie.YAxis == nil || pie.YAxis.Key != "foreign_share" {
		t.Errorf("pie 結構錯誤: %+v", pie)
	}
	if !strings.Contains(pie.Note, "加總") {
		t.Errorf("pie note 應描述 aggregate=sum，實際 %s", pie.Note)
	}
}

// TestScatterAndRadar 驗證篩選散佈圖與財報五面向雷達圖（§11.3）。
func TestScatterAndRadar(t *testing.T) {
	sc := Scatter("PE / 殖利率散佈", "pe", "dividend_yield_pct", "pb")
	if sc.RecommendedType != "scatter" || sc.XAxis == nil || sc.XAxis.Type != "value" {
		t.Errorf("scatter 結構錯誤: %+v", sc)
	}
	if len(sc.Series) != 1 || sc.Series[0].Type != "bubble" || sc.Series[0].Key != "pb" {
		t.Errorf("scatter series 應為 bubble(pb)，實際 %+v", sc.Series)
	}
	rd := Radar("財務健康五面向", []string{"profit", "growth", "structure", "dividend", "governance"})
	if rd.RecommendedType != "radar" {
		t.Errorf("radar recommended_type 錯誤: %s", rd.RecommendedType)
	}
	if len(rd.Axes) != 5 || rd.Axes[0] != "profit" || rd.Axes[4] != "governance" {
		t.Errorf("radar axes 應為五面向，實際 %v", rd.Axes)
	}
	if len(rd.Series) != 1 || rd.Series[0].Type != "radar" {
		t.Errorf("radar series 應為 radar，實際 %+v", rd.Series)
	}
}

// TestTableMeta 驗證 table 型別（§11.3 除權息行事曆/風險旗標）：
// recommended_type=table、series 標記 table、columns 欄位描述齊全。
func TestTableMeta(t *testing.T) {
	m := Table([]Column{
		{Key: "date", Label: "除權息日"},
		{Key: "code"},
	})
	if m.RecommendedType != "table" {
		t.Errorf("recommended_type 應為 table，實際 %s", m.RecommendedType)
	}
	if len(m.Series) != 1 || m.Series[0].Type != "table" {
		t.Errorf("table series 應標記 type=table，實際 %+v", m.Series)
	}
	if len(m.Columns) != 2 || m.Columns[0].Key != "date" || m.Columns[0].Label != "除權息日" {
		t.Errorf("columns 應含 date/除權息日，實際 %+v", m.Columns)
	}
	if m.Columns[1].Label != "" {
		t.Errorf("無標題之欄位應省略 label，實際 %+v", m.Columns[1])
	}
	if m.XAxis != nil || m.YAxis != nil {
		t.Errorf("table 無座標軸語意，x_axis/y_axis 應為 nil，實際 x=%+v y=%+v", m.XAxis, m.YAxis)
	}
	// 序列化：columns 應輸出；x_axis/y_axis 依 omitempty 省略
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"columns"`) || !strings.Contains(s, `"key":"date"`) {
		t.Errorf("序列化應含 columns，實際 %s", s)
	}
	if strings.Contains(s, "x_axis") || strings.Contains(s, "y_axis") {
		t.Errorf("table 序列化不應含座標軸，實際 %s", s)
	}
}

// TestForTool 驗證 §11.3 全類型對應（pkg/chart 為唯一真值）。
func TestForTool(t *testing.T) {
	cases := []struct {
		tool string
		typ  string
	}{
		{"get_intraday_kline", "candlestick"},
		{"get_stock_daily_kline", "candlestick"},
		{"get_futures_daily_ohlc", "candlestick"},
		{"get_futures_history", "candlestick"},
		{"get_stock_daily_quote", "line"},
		{"get_foreign_shareholding_history", "line"},
		{"get_institutional_futures_history", "line"},
		{"get_put_call_ratio", "line"},
		{"get_institutional_investors", "bar"},
		{"get_margin_trading", "bar"},
		{"get_warrant_activity", "bar"},
		{"get_market_summary", "bar"},
		{"get_monthly_revenue", "bar"},
		{"get_dividend_history", "bar"},
		{"get_institutional_futures_positions", "bar"},
		{"get_institutional_options_positions", "bar"},
		{"get_large_trader_positions", "bar"},
		{"get_foreign_industry_holdings", "pie"},
		{"screen_stocks", "scatter"},
		{"screen_high_yield", "scatter"},
		{"get_financial_health_check", "radar"},
		{"get_exdividend_calendar", "table"},
		{"scan_daytrade_eligibility", "table"},
	}
	for _, c := range cases {
		m := ForTool(c.tool, 200)
		if m == nil {
			t.Errorf("%s 應對應 %s，實際 nil", c.tool, c.typ)
			continue
		}
		if m.RecommendedType != c.typ {
			t.Errorf("%s 應為 %s，實際 %s", c.tool, c.typ, m.RecommendedType)
		}
	}
	if m := ForTool("unknown_tool", 200); m != nil {
		t.Errorf("未知工具應回傳 nil，實際 %+v", m)
	}
}

// TestForToolPCRAnnotation 驗證 PCR 之 hline 1.0 多空分界線（§11.3）。
func TestForToolPCRAnnotation(t *testing.T) {
	m := ForTool("get_put_call_ratio", 200)
	if m == nil || len(m.Annotations) != 1 {
		t.Fatalf("PCR 應有 1 個 annotation，實際 %+v", m)
	}
	if m.Annotations[0].Type != "hline" || m.Annotations[0].Value != float64(1) {
		t.Errorf("PCR annotation 應為 hline 1.0，實際 %+v", m.Annotations[0])
	}
}

// TestForToolTimeSeriesXKey 驗證 §11.1：時間序列工具之 x_axis.key 與
// data 時間欄位一致（可直接繪圖，無需另行解析）。
func TestForToolTimeSeriesXKey(t *testing.T) {
	want := map[string]string{
		"get_intraday_kline":                "timestamp",
		"get_stock_daily_kline":             "timestamp",
		"get_futures_daily_ohlc":            "date",
		"get_futures_history":               "date",
		"get_stock_daily_quote":             "date",
		"get_foreign_shareholding_history":  "date",
		"get_institutional_futures_history": "date",
		"get_put_call_ratio":                "date",
		"get_monthly_revenue":               "data_year_month",
		"get_dividend_history":              "dividend_year",
	}
	for tool, key := range want {
		m := ForTool(tool, 200)
		if m == nil || m.XAxis == nil {
			t.Errorf("%s 應含 x_axis", tool)
			continue
		}
		if m.XAxis.Key != key {
			t.Errorf("%s x_axis.key 應為 %s，實際 %s", tool, key, m.XAxis.Key)
		}
	}
}

// TestForToolTableColumns 驗證 table 型別工具之 columns 欄位描述（§11.3）：
// 除權息行事曆與風險旗標比對之欄位與資料結構一致。
func TestForToolTableColumns(t *testing.T) {
	m := ForTool("get_exdividend_calendar", 200)
	if m == nil || m.RecommendedType != "table" {
		t.Fatalf("除權息行事曆應對應 table，實際 %+v", m)
	}
	keys := map[string]bool{}
	for _, c := range m.Columns {
		keys[c.Key] = true
	}
	for _, want := range []string{"date", "code", "name", "market", "kind", "cash_dividend", "stock_dividend"} {
		if !keys[want] {
			t.Errorf("columns 應含 %s（對應 ExDivEvent.%s），實際 %+v", want, want, m.Columns)
		}
	}

	r := ForTool("scan_daytrade_eligibility", 200)
	if r == nil || r.RecommendedType != "table" {
		t.Fatalf("風險旗標應對應 table，實際 %+v", r)
	}
	keys = map[string]bool{}
	for _, c := range r.Columns {
		keys[c.Key] = true
	}
	for _, want := range []string{"symbol", "name", "daytrade_allowed", "is_attention", "is_disposition", "margin_suspended", "short_suspended", "summary"} {
		if !keys[want] {
			t.Errorf("columns 應含 %s（對應 DaytradeScan.%s），實際 %+v", want, want, r.Columns)
		}
	}
}

// TestSeriesNormalizedXY 驗證 v2.1 §11「series 陣列本身即為正規化 X/Y 資料」
// （呼叫端忽略 recommended_type 也可直接繪圖）：抽查 6 個時間序列工具之
// data 結構皆為「時間欄位 + 數值欄位」陣列，時間欄位與 _chart_meta.x_axis.key
// 一致，數值欄位覆蓋 y_axis 之 keys/right_axis（T028 驗收標準）。
// 此處以 JSON 反射檢查欄位存在性與型別（各工具 handler 之整合資料形狀
// 另由 pkg/mcp 測試以型別斷言覆蓋）。
func TestSeriesNormalizedXY(t *testing.T) {
	cases := []struct {
		name string // 工具名
		row  any    // 單筆資料（結構體含 json tag，與 model 型別欄位一致）
		time string // 時間欄位
		nums []string
	}{
		{"get_intraday_kline", struct {
			Timestamp int64 `json:"timestamp"`
			Open      any   `json:"open"`
			High      any   `json:"high"`
			Low       any   `json:"low"`
			Close     any   `json:"close"`
			Volume    any   `json:"volume"`
		}{}, "timestamp", []string{"open", "high", "low", "close", "volume"}},
		{"get_stock_daily_kline", struct {
			Timestamp int64 `json:"timestamp"`
			Open      any   `json:"open"`
			High      any   `json:"high"`
			Low       any   `json:"low"`
			Close     any   `json:"close"`
			Volume    any   `json:"volume"`
		}{}, "timestamp", []string{"open", "high", "low", "close", "volume"}},
		{"get_futures_daily_ohlc", struct {
			Date   string `json:"date"`
			Open   any    `json:"open"`
			High   any    `json:"high"`
			Low    any    `json:"low"`
			Close  any    `json:"close"`
			Volume any    `json:"volume"`
		}{}, "date", []string{"open", "high", "low", "close", "volume"}},
		{"get_stock_daily_quote", struct {
			Timestamp int64  `json:"timestamp"`
			Date      string `json:"date"`
			Open      any    `json:"open"`
			High      any    `json:"high"`
			Low       any    `json:"low"`
			Close     any    `json:"close"`
			Volume    any    `json:"volume"`
		}{}, "date", []string{"open", "high", "low", "close", "volume"}},
		{"get_foreign_shareholding_history", struct {
			Date           string `json:"date"`
			ForeignShares  any    `json:"foreign_shares"`
			ForeignPercent any    `json:"foreign_percent"`
		}{}, "date", []string{"foreign_shares", "foreign_percent"}},
		{"get_put_call_ratio", struct {
			Date        string `json:"date"`
			CallVolume  any    `json:"call_volume"`
			PutVolume   any    `json:"put_volume"`
			VolumeRatio any    `json:"volume_ratio"`
			CallOI      any    `json:"call_oi"`
			PutOI       any    `json:"put_oi"`
			OIRatio     any    `json:"oi_ratio"`
		}{}, "date", []string{"call_volume", "put_volume", "volume_ratio", "call_oi", "put_oi", "oi_ratio"}},
		{"get_institutional_futures_history", struct {
			Date        string `json:"date"`
			LongVolume  any    `json:"long_volume"`
			ShortVolume any    `json:"short_volume"`
			NetVolume   any    `json:"net_volume"`
		}{}, "date", []string{"long_volume", "short_volume", "net_volume"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			b, err := json.Marshal(c.row)
			if err != nil {
				t.Fatalf("marshal %s 失敗: %v", c.name, err)
			}
			var m map[string]any
			if err := json.Unmarshal(b, &m); err != nil {
				t.Fatalf("unmarshal %s 失敗: %v", c.name, err)
			}
			if _, ok := m[c.time]; !ok {
				t.Errorf("%s 應含時間欄位 %s（x_axis.key），實際欄位 %v", c.name, c.time, keysOf(m))
			}
			for _, n := range c.nums {
				if _, ok := m[n]; !ok {
					t.Errorf("%s 應含數值欄位 %s（y_axis/right_axis），實際欄位 %v", c.name, n, keysOf(m))
				}
			}
			// 與 _chart_meta.x_axis.key 一致（§11.1 直接繪圖保證）
			meta := ForTool(c.name, 200)
			if meta == nil || meta.XAxis == nil || meta.XAxis.Key != c.time {
				t.Errorf("%s _chart_meta.x_axis.key 應為 %s，實際 %+v", c.name, c.time, meta)
			}
		})
	}
}

// keysOf 列出 JSON 物件欄位鍵（測試輔助）。
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestMarshalOmitempty 驗證 chart=false 行為（§12.7）：零值欄位省略、
// note 等選填不輸出；_chart_meta 之序列化不重複編碼資料（§11.1）。
func TestMarshalOmitempty(t *testing.T) {
	b, err := json.Marshal(&Meta{RecommendedType: "line"})
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	s := string(b)
	if strings.Contains(s, "x_axis") || strings.Contains(s, "annotations") {
		t.Errorf("零值欄位應省略，實際 %s", s)
	}
	if !strings.Contains(s, `"recommended_type":"line"`) {
		t.Errorf("recommended_type 應輸出，實際 %s", s)
	}
}

// TestMarshalKline 驗證 K 線 meta 序列化後欄位齊全（§11.2 標準）；
// 空 annotations 依 omitempty 省略。
func TestMarshalKline(t *testing.T) {
	b, err := json.Marshal(Candlestick())
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	for _, k := range []string{"recommended_type", "x_axis", "y_axis", "series"} {
		if _, ok := m[k]; !ok {
			t.Errorf("應含欄位 %s，實際 %s", k, b)
		}
	}
	if _, ok := m["annotations"]; ok {
		t.Errorf("空 annotations 應省略（omitempty），實際 %s", b)
	}
	y := m["y_axis"].(map[string]any)
	ra, _ := y["right_axis"].([]any)
	if len(ra) != 1 || ra[0] != "volume" {
		t.Errorf("right_axis 應含 volume，實際 %v", ra)
	}
}
