package provider

// mis_quote_test.go：T194 單發直查之離線測試（httptest 注入 + fixtures 回放）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// loadMISFixture 讀取 testdata/mis fixture 並回傳原始 bytes。
func loadMISFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/mis/" + name)
	if err != nil {
		t.Fatalf("讀取 fixture 失敗: %v", err)
	}
	return b
}

// newMISServer 建立 httptest server，回傳 fixture 內容。
func newMISServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func marketOfTSE(string) string { return "tse" }
func marketOfOTC(string) string { return "otc" }

func mustQuotes(t *testing.T, raw []byte) []RealtimeQuote {
	t.Helper()
	var qs []RealtimeQuote
	if err := json.Unmarshal(raw, &qs); err != nil {
		t.Fatalf("解析 RealtimeQuote 失敗: %v", err)
	}
	return qs
}

// TestFetchRealtimeQuotesFixture 以 fixtures/mis/tick_01.json 驗證：
// 上市/上櫃混合查詢、欄位正規化、五檔與 price_source。
func TestFetchRealtimeQuotesFixture(t *testing.T) {
	raw := loadMISFixture(t, "tick_01.json")
	srv := newMISServer(t, raw)
	old := misQuoteURL
	SetMISQuoteURL(srv.URL + "/stock/api/getStockInfo.jsp")
	defer SetMISQuoteURL(old)

	client := NewBaseClient("localhost")
	qs, reqs, err := FetchRealtimeQuotes(context.Background(), client, nil,
		[]string{"2330", "6547"})
	if err != nil {
		t.Fatalf("FetchRealtimeQuotes 失敗: %v", err)
	}
	if reqs != 1 {
		t.Errorf("單批請求數應為 1，實際 %d", reqs)
	}
	if len(qs) == 0 {
		t.Fatal("應至少回傳一筆報價")
	}
	for _, q := range qs {
		if q.Symbol == "" || q.Last <= 0 {
			t.Errorf("%s: 報價欄位不完整: %+v", q.Symbol, q)
		}
		if q.PriceSource != "trade" && q.PriceSource != "prev_close_fallback" {
			t.Errorf("%s: price_source 非法 %q", q.Symbol, q.PriceSource)
		}
	}
}

// TestFetchRealtimeQuotesOTCRetry：tse_ 無資料時以 otc_ 重試取得。
func TestFetchRealtimeQuotesOTCRetry(t *testing.T) {
	fixture := struct {
		MsgArray []map[string]any `json:"msgArray"`
	}{}
	if err := json.Unmarshal(loadMISFixture(t, "tick_01.json"), &fixture); err != nil {
		t.Fatalf("fixture 解析失敗: %v", err)
	}
	// 第一輪只回 otc 標的（模擬 tse_6547 查無）；重試輪回全部
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		exCh := r.URL.Query().Get("ex_ch")
		out := map[string]any{"rtcode": "0000", "msgArray": []any{}}
		for _, m := range fixture.MsgArray {
			key, _ := m["key"].(string)
			isOTCQuery := strings.HasPrefix(exCh, "otc_")
			if (isOTCQuery && strings.Contains(key, "otc_")) ||
				(!isOTCQuery && strings.Contains(key, "tse_2330")) {
				out["msgArray"] = append(out["msgArray"].([]any), m)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	old := misQuoteURL
	SetMISQuoteURL(srv.URL + "/stock/api/getStockInfo.jsp")
	defer SetMISQuoteURL(old)

	client := NewBaseClient("localhost")
	qs, reqs, err := FetchRealtimeQuotes(context.Background(), client, nil,
		[]string{"2330", "6547"})
	if err != nil {
		t.Fatalf("FetchRealtimeQuotes 失敗: %v", err)
	}
	if reqs != 2 {
		t.Errorf("缺漏重試後請求數應為 2，實際 %d", reqs)
	}
	found := map[string]bool{}
	for _, q := range qs {
		found[q.Symbol] = true
	}
	if !found["2330"] || !found["6547"] {
		t.Errorf("重試後應同時取得 2330 與 6547，實際 %v", found)
	}
}

// TestRealtimeQuotePrevCloseFallback：z="-"（無成交）時以昨收 fallback。
func TestRealtimeQuotePrevCloseFallback(t *testing.T) {
	body := `{"rtcode":"0000","msgArray":[{"c":"3037","n":"欣興","ex":"tse",
		"z":"-","o":"-","h":"-","l":"-","y":"210.0000","v":"0","tv":"-",
		"tlong":"","t":"13:30:00","b":"-","g":"-","a":"-","f":"-"}]}`
	srv := newMISServer(t, []byte(body))
	old := misQuoteURL
	SetMISQuoteURL(srv.URL + "/stock/api/getStockInfo.jsp")
	defer SetMISQuoteURL(old)

	client := NewBaseClient("localhost")
	qs, _, err := FetchRealtimeQuotes(context.Background(), client, marketOfTSE,
		[]string{"3037"})
	if err != nil {
		t.Fatalf("FetchRealtimeQuotes 失敗: %v", err)
	}
	if len(qs) != 1 {
		t.Fatalf("應回傳 1 筆，實際 %d", len(qs))
	}
	q := qs[0]
	if q.PriceSource != "prev_close_fallback" {
		t.Errorf("無成交應標記 prev_close_fallback，實際 %q", q.PriceSource)
	}
	if q.Last != 210.0 {
		t.Errorf("fallback 價應為昨收 210，實際 %v", q.Last)
	}
}

// TestRealtimeQuoteFromEntryRoundTrip：realtimeQuoteFromEntry 與 JSON 往返一致
// （確保 handler 快取路徑 json.Marshal/Unmarshal 不失真）。
func TestRealtimeQuoteFromEntryRoundTrip(t *testing.T) {
	e := misEntry{Code: "2330", Ex: "tse", Z: "2425.0000", O: "2350.0000",
		H: "2425.0000", L: "2345.0000", Y: "2400.0000", V: "37500",
		Tlong: "1785479400000", T: "14:30:00",
		B: "2424.0000_2423.0000", G: "100_200",
		A: "2425.0000_2426.0000", F: "300_400"}
	q, ok := realtimeQuoteFromEntry(e)
	if !ok {
		t.Fatal("應可正規化")
	}
	raw, err := json.Marshal([]RealtimeQuote{q})
	if err != nil {
		t.Fatal(err)
	}
	var back []RealtimeQuote
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if len(back) != 1 || back[0].Last != q.Last ||
		len(back[0].Bids) != len(q.Bids) || len(back[0].Asks) != len(q.Asks) {
		t.Errorf("JSON 往返失真: %+v vs %+v", back, q)
	}
	if q.Date != "2026-07-31" || q.Time == "" {
		t.Errorf("日期時間應由 tlong 導出，實際 %q/%q", q.Date, q.Time)
	}
}
