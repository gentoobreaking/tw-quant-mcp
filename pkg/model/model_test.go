package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLineageMarshalFull(t *testing.T) {
	got := time.Date(2026, 7, 31, 18, 0, 5, 0, taipei)
	lg := Lineage{
		Source:      SourceTWSEWeb,
		SourceRole:  SourceRoleCanonical,
		DerivedFrom: []string{"kline_daily"},
		FetchedAt:   NewTaipeiTime(got),
		DataDate:    "2026-07-31",
		Freshness:   FreshnessPostMarketToday,
		SamplingSec: 0,
		IsCached:    true,
		CacheTTL:    60,
		LatencyMS:   42,
		SourceURL:   "https://www.twse.com.tw/exchangeReport/STOCK_DAY",
	}
	b, err := json.Marshal(lg)
	if err != nil {
		t.Fatalf("marshal 失敗: %v", err)
	}
	want := `{"source":"TWSE_WEB","source_role":"canonical","derived_from":["kline_daily"],` +
		`"fetched_at":"2026-07-31T18:00:05+08:00","data_date":"2026-07-31","freshness":"POST_MARKET_TODAY",` +
		`"sampling_sec":0,"is_cached":true,"cache_ttl":60,"latency_ms":42,"source_url":"https://www.twse.com.tw/exchangeReport/STOCK_DAY"}`
	if string(b) != want {
		t.Errorf("marshal 結果不符\n got: %s\nwant: %s", b, want)
	}
}

func TestLineageOmitempty(t *testing.T) {
	lg := Lineage{
		Source:     SourceTWSEMIS,
		SourceRole: SourceRoleCanonical,
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
	for _, absent := range []string{"derived_from", "source_url"} {
		if _, ok := m[absent]; ok {
			t.Errorf("欄位 %q 為空值時不應輸出", absent)
		}
	}
	for _, present := range []string{"source", "source_role", "fetched_at", "data_date", "freshness", "sampling_sec", "is_cached", "cache_ttl", "latency_ms"} {
		if _, ok := m[present]; !ok {
			t.Errorf("欄位 %q 應輸出", present)
		}
	}
}

func TestLineageUnmarshalRoundTrip(t *testing.T) {
	in := `{"source":"TAIFEX_DL","source_role":"canonical","derived_from":null,` +
		`"fetched_at":"2026-07-30T08:01:02+08:00","data_date":"2026-07-30","freshness":"HISTORICAL",` +
		`"sampling_sec":0,"is_cached":false,"cache_ttl":0,"latency_ms":7}`
	var lg Lineage
	if err := json.Unmarshal([]byte(in), &lg); err != nil {
		t.Fatalf("unmarshal 失敗: %v", err)
	}
	if lg.Source != SourceTAIFEXDL || lg.DataDate != "2026-07-30" || lg.Freshness != FreshnessHistorical {
		t.Errorf("unmarshal 欄位不符: %+v", lg)
	}
	if want := "2026-07-30T08:01:02+08:00"; lg.FetchedAt.Format(time.RFC3339) != want {
		t.Errorf("fetched_at 應為 %q，實際 %q", want, lg.FetchedAt.Format(time.RFC3339))
	}
	if _, off := lg.FetchedAt.Zone(); off != 8*3600 {
		t.Errorf("時區偏移應為 +08:00，實際 %d", off)
	}
}

func TestEnvelopeMarshal(t *testing.T) {
	env := Envelope{
		Data: []Candle{{Timestamp: "09:01:00", Open: 100, High: 102, Low: 99, Close: 101, Volume: 3000}},
		Lineage: Lineage{
			Source: SourceTPExAPI, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 13, 30, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessPostMarketToday,
		},
		ChartMeta: map[string]any{"recommended_type": "candlestick"},
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
		Lineage: Lineage{
			Source: SourceMOPS, SourceRole: SourceRoleCanonical,
			FetchedAt: NewTaipeiTime(time.Date(2026, 7, 31, 15, 0, 0, 0, taipei)),
			DataDate:  "2026-07-31", Freshness: FreshnessPostMarketToday,
		},
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
	for _, f := range []string{FreshnessRealtimeIntraday, FreshnessPostMarketToday, FreshnessHistorical} {
		if !ValidFreshness(f) {
			t.Errorf("%q 應為合法 freshness", f)
		}
	}
	if ValidFreshness("TODAY") || ValidFreshness("") {
		t.Error("非法 freshness 不應通過")
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
