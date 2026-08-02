package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tw-quant-mcp/pkg/chart"
)

func TestLineageMarshalFull(t *testing.T) {
	got := time.Date(2026, 7, 31, 18, 0, 5, 0, taipei)
	lg := Lineage{
		Source:      SourceTWSEWeb,
		SourceRole:  SourceRoleCanonical,
		DerivedFrom: []string{"kline_daily"},
		FetchedAt:   NewTaipeiTime(got),
		DataDate:    "2026-07-31",
		Freshness:   FreshnessPostMarket,
		SamplingSec: 0,
		IsCached:    true,
		CacheTTL:    60,
		CacheAgeSec: 300,
		LatencyMS:   42,
		SourceURL:   "https://www.twse.com.tw/exchangeReport/STOCK_DAY",
		Grade:       GradeAvailable,
	}
	b, err := json.Marshal(lg)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	// 正式 JSON：source_role 輸出新值；derived_from/cache_ttl/source_url 一律不出；
	// 新增 cache_age_sec 與 grade。
	want := `{"source":"TWSE_WEB","source_role":"CANONICAL",` +
		`"fetched_at":"2026-07-31T18:00:05+08:00","data_date":"2026-07-31","freshness":"POST_MARKET",` +
		`"sampling_sec":0,"is_cached":true,"cache_age_sec":300,"latency_ms":42,"grade":"AVAILABLE"}`
	if string(b) != want {
		t.Errorf("marshal 結果不符\n got: %s\nwant: %s", b, want)
	}
	// 舊三欄不得出現於正式 JSON
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	for _, absent := range []string{"derived_from", "cache_ttl", "source_url"} {
		if _, ok := m[absent]; ok {
			t.Errorf("正式 JSON 不應輸出欄位 %q", absent)
		}
	}
}

func TestLineageOmitempty(t *testing.T) {
	lg := Lineage{
		Source:     SourceTWSEMIS,
		SourceRole: SourceRoleRealtime,
		FetchedAt:  NewTaipeiTime(time.Date(2026, 7, 31, 9, 0, 0, 0, taipei)),
		DataDate:   "2026-07-31",
		Freshness:  FreshnessRealtimeIntraday,
	}
	b, err := json.Marshal(lg)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	for _, absent := range []string{"derived_from", "cache_ttl", "source_url", "cache_age_sec", "grade"} {
		if _, ok := m[absent]; ok {
			t.Errorf("欄位 %q 為空值時不應輸出", absent)
		}
	}
	for _, present := range []string{"source", "source_role", "fetched_at", "data_date", "freshness", "sampling_sec", "is_cached", "latency_ms"} {
		if _, ok := m[present]; !ok {
			t.Errorf("欄位 %q 應輸出", present)
		}
	}
}

// TestLineageNewFields 驗證 v2.1 新欄位（cache_age_sec / grade）之序列化與預設值。
func TestLineageNewFields(t *testing.T) {
	base := Lineage{
		Source:     SourceTAIFEXDL,
		SourceRole: SourceRoleFallback,
		FetchedAt:  NewTaipeiTime(time.Date(2026, 7, 30, 8, 0, 0, 0, taipei)),
		DataDate:   "2026-07-30",
		Freshness:  FreshnessPostMarket,
	}
	// 預設值：cache_age_sec=0、grade="" → 均不出 JSON
	b, err := json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	if err := json.Unmarshal(b, &map[string]any{}); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	// 設定後輸出
	base.CacheAgeSec = 3600
	base.Grade = GradePreview
	b, err = json.Marshal(base)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if v, ok := m["cache_age_sec"]; !ok || v.(float64) != 3600 {
		t.Errorf("cache_age_sec 應輸出 3600，實際 %v", m["cache_age_sec"])
	}
	if v, ok := m["grade"]; !ok || v.(string) != "PREVIEW" {
		t.Errorf("grade 應輸出 PREVIEW，實際 %v", m["grade"])
	}
}

// TestLineageDebugJSON 驗證 debug/log 模式輸出：舊三欄
// （derived_from/cache_ttl/source_url）僅於 DebugJSON 輸出，正式 JSON 無。
func TestLineageDebugJSON(t *testing.T) {
	lg := Lineage{
		Source:      SourceTWSEWeb,
		SourceRole:  SourceRoleCanonical,
		DerivedFrom: []string{"kline_daily", "TWSE_API:esg"},
		FetchedAt:   NewTaipeiTime(time.Date(2026, 7, 31, 18, 0, 0, 0, taipei)),
		DataDate:    "2026-07-31",
		Freshness:   FreshnessPostMarket,
		IsCached:    true,
		CacheTTL:    60,
		LatencyMS:   42,
		SourceURL:   "https://www.twse.com.tw/exchangeReport/STOCK_DAY",
	}
	b, err := lg.DebugJSON()
	if err != nil {
		t.Fatalf("DebugJSON 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	for _, present := range []string{"derived_from", "cache_ttl", "source_url"} {
		if _, ok := m[present]; !ok {
			t.Errorf("debug 模式應輸出欄位 %q", present)
		}
	}
	if v, ok := m["source_role"]; !ok || v.(string) != string(SourceRoleCanonical) {
		t.Errorf("debug 模式 source_role 應為 CANONICAL，實際 %v", m["source_role"])
	}
	// 正式 JSON 不含三欄
	b2, err := json.Marshal(lg)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m2 map[string]any
	if err := json.Unmarshal(b2, &m2); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	for _, absent := range []string{"derived_from", "cache_ttl", "source_url"} {
		if _, ok := m2[absent]; ok {
			t.Errorf("正式 JSON 不應輸出欄位 %q", absent)
		}
	}
}

func TestLineageUnmarshalRoundTrip(t *testing.T) {
	in := `{"source":"TAIFEX_DL","source_role":"FALLBACK",` +
		`"fetched_at":"2026-07-30T08:01:02+08:00","data_date":"2026-07-30","freshness":"POST_MARKET",` +
		`"sampling_sec":0,"is_cached":false,"cache_age_sec":1200,"latency_ms":7,"grade":"AVAILABLE"}`
	var lg Lineage
	if err := json.Unmarshal([]byte(in), &lg); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if lg.Source != SourceTAIFEXDL || lg.DataDate != "2026-07-30" || lg.Freshness != FreshnessPostMarket {
		t.Errorf("unmarshal 欄位不符: %+v", lg)
	}
	if lg.SourceRole != SourceRoleFallback {
		t.Errorf("source_role 應為 FALLBACK，實際 %q", lg.SourceRole)
	}
	if lg.CacheAgeSec != 1200 {
		t.Errorf("cache_age_sec 應為 1200，實際 %d", lg.CacheAgeSec)
	}
	if lg.Grade != GradeAvailable {
		t.Errorf("grade 應為 AVAILABLE，實際 %q", lg.Grade)
	}
	if want := "2026-07-30T08:01:02+08:00"; lg.FetchedAt.Format(time.RFC3339) != want {
		t.Errorf("fetched_at 應為 %q，實際 %q", want, lg.FetchedAt.Format(time.RFC3339))
	}
	if _, off := lg.FetchedAt.Zone(); off != 8*3600 {
		t.Errorf("時區偏移應為 +08:00，實際 %d", off)
	}
}

// TestLineagesUnionMarshal 驗證 `_lineage` union（單一物件 / 陣列）輸出。
func TestLineagesUnionMarshal(t *testing.T) {
	single := Lineage{
		Source: SourceTPExAPI, SourceRole: SourceRoleCanonical,
		FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 13, 30, 0, 0, taipei)),
		DataDate:  "2026-07-31", Freshness: FreshnessPostMarket,
	}
	env := Envelope{Data: "{}", Lineage: Lineages{Lineage: single}}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	raw := string(m["_lineage"])
	if raw == "" || raw[0] != '{' {
		t.Errorf("單一來源時 _lineage 應為物件，實際 %s", raw)
	}
	if !strings.Contains(raw, `"source_role":"CANONICAL"`) {
		t.Errorf("_lineage 應含新 source_role，實際 %s", raw)
	}

	multi := Lineages{Multi: []Lineage{
		{Source: SourceTWSEWeb, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 14, 0, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessPostMarket},
		{Source: SourceMOPS, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 14, 5, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessMonthly},
	}}
	env2 := Envelope{Data: "{}", Lineage: multi}
	b2, err := json.Marshal(env2)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	if err := json.Unmarshal(b2, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	raw2 := string(m["_lineage"])
	if raw2 == "" || raw2[0] != '[' {
		t.Errorf("多來源聚合時 _lineage 應為陣列，實際 %s", raw2)
	}
}

// TestLineagesUnmarshalUnion 驗證 union 反序列化：物件→單一、陣列→Multi。
func TestLineagesUnmarshalUnion(t *testing.T) {
	singleJSON := `{"_lineage":{"source":"TWSE_WEB","source_role":"CANONICAL","fetched_at":"2026-07-31T14:00:00+08:00","data_date":"2026-07-31","freshness":"POST_MARKET","sampling_sec":0,"is_cached":false,"latency_ms":3},"http_calls":0}`
	var env Envelope
	if err := json.Unmarshal([]byte(singleJSON), &env); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if env.Lineage.Multi != nil || env.Lineage.Lineage.Source != SourceTWSEWeb {
		t.Errorf("物件型別應填入單一 Lineage: %+v", env.Lineage)
	}
	if env.Lineage.First().Source != SourceTWSEWeb {
		t.Errorf("First() 應回傳單一 Lineage 之來源，實際 %q", env.Lineage.First().Source)
	}
	if env.Lineage.Len() != 1 {
		t.Errorf("Len() 應為 1，實際 %d", env.Lineage.Len())
	}

	multiJSON := `{"_lineage":[{"source":"TWSE_WEB","source_role":"CANONICAL","fetched_at":"2026-07-31T14:00:00+08:00","data_date":"2026-07-31","freshness":"POST_MARKET","sampling_sec":0,"is_cached":false,"latency_ms":3},{"source":"MOPS","source_role":"CANONICAL","fetched_at":"2026-07-31T14:05:00+08:00","data_date":"2026-07-31","freshness":"MONTHLY","sampling_sec":0,"is_cached":false,"latency_ms":4}],"http_calls":0}`
	var env2 Envelope
	if err := json.Unmarshal([]byte(multiJSON), &env2); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if len(env2.Lineage.Multi) != 2 {
		t.Fatalf("陣列型別應填入 2 筆 Multi，實際 %d", len(env2.Lineage.Multi))
	}
	if env2.Lineage.Multi[1].Source != SourceMOPS || env2.Lineage.Multi[1].Freshness != FreshnessMonthly {
		t.Errorf("Multi[1] 欄位不符: %+v", env2.Lineage.Multi[1])
	}
	if env2.Lineage.Len() != 2 {
		t.Errorf("Len() 應為 2，實際 %d", env2.Lineage.Len())
	}
	if env2.Lineage.First().Source != SourceTWSEWeb {
		t.Errorf("First() 應回傳陣列首筆，實際 %q", env2.Lineage.First().Source)
	}
	// round trip：反序列化後再序列化，仍應輸出陣列
	b, err := json.Marshal(env2)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	if !strings.Contains(string(b), `"_lineage":[`) {
		t.Errorf("round trip 後 _lineage 應仍為陣列: %s", b)
	}
}

func TestEnvelopeMarshal(t *testing.T) {
	env := Envelope{
		Data: []Candle{{Timestamp: "09:01:00", Open: 100, High: 102, Low: 99, Close: 101, Volume: 3000}},
		Lineage: Lineages{Lineage: Lineage{
			Source: SourceTPExAPI, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 13, 30, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessPostMarket,
		}},
		ChartMeta: chart.Candlestick(),
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if _, ok := m["data"]; !ok {
		t.Error("data 應存在")
	}
	if _, ok := m["_lineage"]; !ok {
		t.Error("_lineage 應存在")
	}
	if _, ok := m["_chart_meta"]; !ok {
		t.Error("_chart_meta 設定時應輸出")
	}
}

func TestEnvelopeChartMetaOmitempty(t *testing.T) {
	env := Envelope{
		Data: "{}",
		Lineage: Lineages{Lineage: Lineage{
			Source: SourceMOPS, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 15, 0, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessPostMarket,
		}},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if _, ok := m["_chart_meta"]; ok {
		t.Error("_chart_meta 為 nil 時不應輸出")
	}
}

func TestCandleOmitempty(t *testing.T) {
	c := Candle{Timestamp: "2026-07-31", Open: 1, High: 2, Low: 0.5, Close: 1.5, Volume: 1000}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if _, ok := m["amount"]; ok {
		t.Error("amount 為 0 時不應輸出")
	}
	c.Amount = 12345000
	b, err = json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if v, ok := m["amount"]; !ok || v.(float64) != 12345000 {
		t.Errorf("amount 設定後應輸出 12345000，實際 %v", m["amount"])
	}
}

func TestSymbolMarshalUnmarshal(t *testing.T) {
	sym := Symbol{Code: "2330", Market: MarketTSE, Name: "台積電", Category: "半導體"}
	b, err := json.Marshal(sym)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	var back Symbol
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if back != sym {
		t.Errorf("round trip 不符: got %+v want %+v", back, sym)
	}
}

func TestSymbolExch(t *testing.T) {
	tests := []struct {
		sym  Symbol
		want string
	}{
		{Symbol{Code: "2330", Market: MarketTSE}, "tse_2330.tw"},
		{Symbol{Code: "6547", Market: MarketOTC}, "otc_6547.tw"},
	}
	for _, tt := range tests {
		if got := tt.sym.Exch(); got != tt.want {
			t.Errorf("Exch(%+v) = %q, want %q", tt.sym, got, tt.want)
		}
	}
}

func TestSymbolValidate(t *testing.T) {
	valid := []Symbol{
		{Code: "2330", Market: MarketTSE, Name: "台積電"},  // 上市 4 碼
		{Code: "6547", Market: MarketOTC, Name: "高端疫苗"}, // 上櫃 4 碼
		{Code: "223321", Market: MarketOTC, Name: "某股"}, // 6 碼
	}
	for _, s := range valid {
		if err := s.Validate(); err != nil {
			t.Errorf("合法 Symbol %+v 不應失敗: %v", s, err)
		}
	}
	invalid := []Symbol{
		{Code: "233", Market: MarketTSE, Name: "x"},   // 不足 4 碼
		{Code: "2330A", Market: MarketTSE, Name: "x"}, // 非數字
		{Code: "2330", Market: "nasdaq", Name: "x"},   // 非法市場
		{Code: "2330", Market: MarketTSE, Name: " "},  // 空名稱
	}
	for _, s := range invalid {
		if err := s.Validate(); err == nil {
			t.Errorf("非法 Symbol %+v 應回報錯誤", s)
		}
	}
}

func TestValidFreshness(t *testing.T) {
	for _, f := range []string{
		FreshnessRealtimeIntraday, FreshnessPostMarket, FreshnessMonthly,
		FreshnessQuarterly, FreshnessStaleFallback,
	} {
		if !ValidFreshness(f) {
			t.Errorf("%q 應為合法 freshness", f)
		}
	}
	for _, f := range []string{"TODAY", "HISTORICAL", "POST_MARKET_TODAY", ""} {
		if ValidFreshness(f) {
			t.Errorf("非法 freshness %q 不應通過", f)
		}
	}
}

func TestTaipeiTimeJSONErrors(t *testing.T) {
	var tt TaipeiTime
	if err := json.Unmarshal([]byte(`"2026-13-99T00:00:00+08:00"`), &tt); err == nil {
		t.Error("非法 RFC3339 應回報錯誤")
	}
	if err := json.Unmarshal([]byte(`12345`), &tt); err == nil {
		t.Error("非字串應回報錯誤")
	}
}
