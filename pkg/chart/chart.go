// Package chart 實作 §11「圖表化設計」：`_chart_meta` 標準產生器與
// §11.3 圖表類型對應（T016）。
//
// 本套件為 §11.3 對應表之唯一真值：新增資料型別時須於此同步補類型
// 對應（見 ForTool）。`_chart_meta` 僅為渲染描述，不重複編碼資料
// （§11.1）；所有時間序列工具之 data 本身即為可直接繪圖的 Series，
// 時間欄位格式一致（timestamp / date，見 x_axis.format）。
package chart

import "fmt"

// Meta 為 §11.2 `_chart_meta` 標準結構。ChartMeta 請求含 chart=true
// （預設）時輸出；chart=false 時以 nil 省略（omitempty，§12.7）。
type Meta struct {
	RecommendedType string       `json:"recommended_type"`  // candlestick/line/bar/heatmap/table/pie/scatter/radar
	XAxis           *Axis        `json:"x_axis,omitempty"`  // 橫軸（時間序列必填）
	YAxis           *YAxis       `json:"y_axis,omitempty"`  // 縱軸（radar 以 Axes 表示）
	Axes            []string     `json:"axes,omitempty"`    // radar 之面向軸
	Series          []Series     `json:"series,omitempty"`  // 輔助序列描述（volume/分色…）
	Columns         []Column     `json:"columns,omitempty"` // table 型別之欄位描述（§11.3）
	Annotations     []Annotation `json:"annotations,omitempty"`
	Note            string       `json:"note,omitempty"`
}

// Axis 為座標軸描述。Type 為 datetime / category / value；
// Format 僅 datetime 使用（HH:mm、YYYY-MM-DD）。
type Axis struct {
	Key    string `json:"key"`
	Type   string `json:"type,omitempty"`
	Format string `json:"format,omitempty"`
}

// YAxis 為縱軸描述。Keys 為多序列（OHLC）；Key 為單序列。
// RightAxis 為輔助軸欄位（§11.2 volume）。
type YAxis struct {
	Keys      []string `json:"keys,omitempty"`
	Key       string   `json:"key,omitempty"`
	Title     string   `json:"title,omitempty"`
	RightAxis []string `json:"right_axis,omitempty"`
}

// Series 描述輔助/次要序列之渲染方式。
// Style 為 diverging（正負分色，§11.3 bar）或 volume（K 線成交量）；
// Type 為 bar/line/bubble/radar/table（table 型別之系列標記）。
type Series struct {
	Key   string `json:"key,omitempty"`
	Type  string `json:"type,omitempty"` // bar/line/bubble/radar/table
	Style string `json:"style,omitempty"`
}

// Column 為 table 型別之欄位描述（§11.3 table：除權息行事曆、風險旗標）。
// Key 對應資料列之物件欄位名；Label 為欄位標題（可省略，前端可用 Key 直出）。
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label,omitempty"`
}

// Annotation 為圖表註記（§11.2 annotations）。
// Type 為 hline / vline；Value 為座標值（hline 之數值）。
type Annotation struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
	Label string `json:"label,omitempty"`
}

// HLine 產出水平分界線註記（如 PCR 多空分界線 1.0，§11.3）。
func HLine(value float64, label string) Annotation {
	return Annotation{Type: "hline", Value: value, Label: label}
}

// Option 為 builder 之客製選項。
type Option func(*Meta)

// WithNote 覆寫/設定 note。
func WithNote(note string) Option { return func(m *Meta) { m.Note = note } }

// WithAnnotations 追加註記。
func WithAnnotations(a ...Annotation) Option {
	return func(m *Meta) { m.Annotations = append(m.Annotations, a...) }
}

// WithXKey 覆寫橫軸資料欄位（如期貨 K 線之 date）。
func WithXKey(key string) Option {
	return func(m *Meta) {
		if m.XAxis == nil {
			m.XAxis = &Axis{}
		}
		m.XAxis.Key = key
	}
}

// WithXFormat 覆寫橫軸 datetime 格式。
func WithXFormat(format string) Option {
	return func(m *Meta) {
		if m.XAxis == nil {
			m.XAxis = &Axis{}
		}
		m.XAxis.Type = "datetime"
		m.XAxis.Format = format
	}
}

// WithYTitle 覆寫縱軸標題。
func WithYTitle(title string) Option { return func(m *Meta) { m.YAxis.Title = title } }

// Candlestick 產出 K 線之 §11.2 描述（§11.3 任何 K 線，含期貨）。
// 預設 x=timestamp（HH:mm）；volume 置於 y 軸 right_axis（§11.2 輔助軸）。
func Candlestick(opts ...Option) *Meta {
	m := &Meta{
		RecommendedType: "candlestick",
		XAxis:           &Axis{Key: "timestamp", Type: "datetime", Format: "HH:mm"},
		YAxis: &YAxis{
			Keys:      []string{"open", "high", "low", "close"},
			Title:     "價格 (元)",
			RightAxis: []string{"volume"},
		},
		Series: []Series{{Key: "volume", Type: "bar", Style: "volume"}},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Line 產出線圖之 §11.2 描述（§11.3 指數/股價趨勢、PCR 歷史）。
func Line(title, xKey, yKey string, opts ...Option) *Meta {
	m := &Meta{
		RecommendedType: "line",
		XAxis:           &Axis{Key: xKey, Type: "category"},
		YAxis:           &YAxis{Keys: []string{yKey}, Title: title},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Bar 產出長條圖之 §11.2 描述（§11.3 法人/融資融券/營收）。
// series.style=diverging 表示正值/負值分色呈現。
func Bar(title, xKey, yKey string, opts ...Option) *Meta {
	m := &Meta{
		RecommendedType: "bar",
		XAxis:           &Axis{Key: xKey, Type: "category"},
		YAxis:           &YAxis{Keys: []string{yKey}, Title: title},
		Series:          []Series{{Key: yKey, Type: "bar", Style: "diverging"}},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Table 產出表格型 _chart_meta（§11.3 table：除權息行事曆、風險旗標比對）。
// 表格資料無座標軸語意：XAxis/YAxis 省略，columns 描述欄位順序與標題，
// 供前端依資料結構直接渲染（SeriesData 之 Values map 鍵與 columns.key 對應）。
func Table(columns []Column, opts ...Option) *Meta {
	m := &Meta{
		RecommendedType: "table",
		Series:          []Series{{Type: "table"}},
	}
	for _, o := range opts {
		o(m)
	}
	if len(columns) > 0 {
		m.Columns = columns
	}
	return m
}

// Heatmap 產出熱力圖之 §11.2 描述（§11.3 產業配置/權重）。
func Heatmap(title, nameKey, valueKey string) *Meta {
	return &Meta{
		RecommendedType: "heatmap",
		XAxis:           &Axis{Key: nameKey, Type: "category"},
		YAxis:           &YAxis{Key: valueKey, Title: title},
		Note:            fmt.Sprintf("依 %s 對應 %s 之權重著色", nameKey, valueKey),
	}
}

// Pie 產出圓餅圖之 §11.2 描述（§11.3 產業配置/權重）。
func Pie(title, nameKey, valueKey string) *Meta {
	return &Meta{
		RecommendedType: "pie",
		XAxis:           &Axis{Key: nameKey, Type: "category"},
		YAxis:           &YAxis{Key: valueKey, Title: title},
		Note:            fmt.Sprintf("依 %s 加總 %s（aggregate=sum）", nameKey, valueKey),
	}
}

// Scatter 產出散佈圖之 §11.2 描述（§11.3 篩選結果）；sizeKey 為氣泡大小。
func Scatter(title, xKey, yKey, sizeKey string) *Meta {
	return &Meta{
		RecommendedType: "scatter",
		XAxis:           &Axis{Key: xKey, Type: "value"},
		YAxis:           &YAxis{Key: yKey, Title: title},
		Series:          []Series{{Key: sizeKey, Type: "bubble"}},
	}
}

// Radar 產出雷達圖之 §11.2 描述（§11.3 財報五面向）。
func Radar(title string, axes []string) *Meta {
	return &Meta{
		RecommendedType: "radar",
		Axes:            axes,
		YAxis:           &YAxis{Title: title},
		Series:          []Series{{Type: "radar"}},
	}
}
