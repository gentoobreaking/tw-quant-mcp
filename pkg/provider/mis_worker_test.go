package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
)

// misFixture 為 mis.twse.com.tw/stock/api/getStockInfo.jsp 之真實回應
// （2026-07-31 14:30 收盤後抓取，tse_2330.tw|otc_6547.tw 兩檔）。
const misFixture = `{"msgArray":[{"@":"2330.tw","tv":"4512","ps":"4411","pid":"9.tse.tw|13527","pz":"2425.0000","bp":"0","fv":"156","oa":"2425.0000","ob":"2420.0000","m%":"000000","^":"20260731","key":"tse_2330.tw_20260731","a":"-","b":"2425.0000_2420.0000_2415.0000_2410.0000_2405.0000_","c":"2330","#":"13.tse.tw|1969","d":"20260731","%":"14:30:00","ch":"2330.tw","tlong":"1785479400000","ot":"14:30:00","f":"-","g":"1989_138_237_300_307_","ip":"0","mt":"000000","ov":"93894","h":"2425.0000","i":"24","it":"12","oz":"2425.0000","l":"2345.0000","n":"台積電","o":"2350.0000","p":"0","ex":"tse","s":"4512","t":"13:30:00","u":"2425.0000","v":"56896","w":"1985.0000","nf":"台灣積體電路製造股份有限公司","y":"2205.0000","z":"2425.0000","ts":"0"},{"@":"6547.tw","tv":"93","ps":"92","pid":"9.otc.tw|869","pz":"45.8000","bp":"0","fv":"4","oa":"49.0000","ob":"45.8000","m%":"000000","^":"20260731","key":"otc_6547.tw_20260731","a":"45.8000_45.8500_45.9000_45.9500_46.0000_","b":"45.7500_45.7000_45.6500_45.6000_45.5500_","c":"6547","#":"13.otc.tw|1403","d":"20260731","%":"14:30:00","ch":"6547.tw","tlong":"1785479400000","ot":"14:30:00","f":"3_3_3_8_9_","g":"5_12_11_23_8_","ip":"0","mt":"000000","ov":"100","h":"47.2000","i":"22","it":"12","oz":"46.0500","l":"45.7000","n":"高端疫苗","o":"46.2500","p":"0","ex":"otc","s":"93","t":"13:30:00","u":"50.1000","v":"1584","w":"41.0500","nf":"高端疫苗生物製劑股份有限公司","y":"45.6000","z":"45.8000","ts":"0"}],"referer":"","userDelay":5000,"rtcode":"0000","queryTime":{"sysDate":"20260731","stockInfoItem":13493,"stockInfo":4497,"sessionStr":"UserSession","sysTime":"18:51:24","showChart":false,"sessionFromTime":-1,"sessionLatestTime":-1},"rtmessage":"OK"}`

// 真實 fixture 解析：欄位轉換與單位（§8.3/§5.1）。
func TestParseMISReal(t *testing.T) {
	snaps, err := parseMIS([]byte(misFixture))
	if err != nil {
		t.Fatalf("parseMIS 失敗: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("應解析 2 檔，實際 %d", len(snaps))
	}

	a := snaps[0]
	if a.Code != "2330" || a.Exch != "tse_2330.tw" {
		t.Errorf("代碼/Exch 錯誤: %s/%s", a.Code, a.Exch)
	}
	if a.Last != 2425 || a.Open != 2350 || a.High != 2425 || a.Low != 2345 || a.PrevClose != 2205 {
		t.Errorf("價格轉換錯誤: z=%v o=%v h=%v l=%v y=%v", a.Last, a.Open, a.High, a.Low, a.PrevClose)
	}
	if a.Change != 220 {
		t.Errorf("漲跌 = z−y 應為 220，實際 %v", a.Change)
	}
	// tv=4512 張 → 4,512,000 股；v=56896 張 → 56,896,000 股
	if a.MinuteVol != 4512000 {
		t.Errorf("MinuteVol(tv) 應為 4,512,000 股，實際 %d", a.MinuteVol)
	}
	if a.CumulativeVol != 56896000 {
		t.Errorf("CumulativeVol(v) 應為 56,896,000 股，實際 %d", a.CumulativeVol)
	}
	want := time.Date(2026, 7, 31, 14, 30, 0, 0, model.Taipei())
	if !a.Time.Time.Equal(want) {
		t.Errorf("tlong 轉換應為 2026-07-31 14:30:00，實際 %s", a.Time.Format("15:04:05"))
	}
	if a.TradeTime != "13:30:00" {
		t.Errorf("TradeTime 應為 13:30:00，實際 %q", a.TradeTime)
	}

	b := snaps[1]
	if b.Code != "6547" || b.Exch != "otc_6547.tw" {
		t.Errorf("代碼/Exch 錯誤: %s/%s", b.Code, b.Exch)
	}
	if b.Last != 45.8 || b.MinuteVol != 93000 {
		t.Errorf("6547 轉換錯誤: z=%v tv=%d", b.Last, b.MinuteVol)
	}
}

func TestParseMISErrors(t *testing.T) {
	if _, err := parseMIS([]byte("{bad json")); err == nil {
		t.Error("非法 JSON 應回傳錯誤")
	}
	if _, err := parseMIS([]byte(`{"rtcode":"5000","msgArray":[]}`)); err == nil {
		t.Error("rtcode!=0000 應回傳錯誤")
	}
	if _, err := parseMIS([]byte(`{"rtcode":"0000","msgArray":[]}`)); err == nil {
		t.Error("空 msgArray 應回傳錯誤")
	}
	// 全部無效記錄（z 為 "-"）→ 錯誤
	bad := `{"rtcode":"0000","msgArray":[{"c":"2330","ch":"2330.tw","ex":"tse","z":"-","tv":"1","tlong":"1785479400000"}]}`
	if _, err := parseMIS([]byte(bad)); err == nil {
		t.Error("全滅無效記錄應回傳錯誤")
	}
	// 部分無效：略過無效記錄、保留有效（容錯）
	mixed := `{"rtcode":"0000","msgArray":[
		{"c":"2330","ch":"2330.tw","ex":"tse","z":"2425.0000","tv":"4512","tlong":"1785479400000","o":"2350.0000","h":"2425.0000","l":"2345.0000","y":"2205.0000","v":"56896","t":"13:30:00"},
		{"c":"bad","ch":"bad.tw","ex":"tse","z":"-","tv":"1","tlong":"1"}]}`
	snaps, err := parseMIS([]byte(mixed))
	if err != nil {
		t.Fatalf("部分無效應略過並回傳有效記錄: %v", err)
	}
	if len(snaps) != 1 || snaps[0].Code != "2330" {
		t.Errorf("應僅保留有效記錄，實際 %d 筆", len(snaps))
	}
}

// 價格 4 位小數 → 2 位；"-" 視為無效。
func TestNormalizeUnits(t *testing.T) {
	if v, ok := parsePrice("2425.0000"); !ok || v != 2425 {
		t.Errorf("2425.0000 → 2425，實際 %v/%v", v, ok)
	}
	if v, ok := parsePrice("45.8000"); !ok || v != 45.8 {
		t.Errorf("45.8000 → 45.8，實際 %v/%v", v, ok)
	}
	if _, ok := parsePrice("-"); ok {
		t.Error("'-' 應無效")
	}
	if v, ok := parseVol("4512"); !ok || v != 4512000 {
		t.Errorf("4512 張 → 4,512,000 股，實際 %v", v)
	}
}

// 建立測試用 watchlist（2026-07-31 交易日）。
func testWatchlist() *engine.Watchlist {
	w := engine.NewWatchlist(func(time.Time) bool { return true })
	_ = w.Set([]model.Symbol{
		{Code: "2330", Market: model.MarketTSE, Name: "台積電"},
		{Code: "6547", Market: model.MarketOTC, Name: "高端疫苗"},
	})
	return w
}

// pollAndStore：httptest 注入，ex_ch 批次參數正確、快照寫入 RingStore。
func TestMISPollAndStore(t *testing.T) {
	var gotQuery atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery.Store(r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(misFixture))
	}))
	defer srv.Close()

	client := NewBaseClient("mis.twse.com.tw",
		WithRateInterval(time.Microsecond), WithJitterRatio(0))
	rings := engine.NewRingStore()
	wk := NewMISWorker(client, testWatchlist(), rings,
		WithMISURLs(srv.URL+"/stock/index.jsp", srv.URL+"/api/getStockInfo.jsp"))

	snaps, err := wk.pollAndStore(context.Background())
	if err != nil {
		t.Fatalf("poll 失敗: %v", err)
	}
	if len(snaps) != 2 {
		t.Fatalf("應 2 筆快照，實際 %d", len(snaps))
	}
	q, _ := gotQuery.Load().(string)
	if !strings.Contains(q, "ex_ch=") || !strings.Contains(q, "tse_2330.tw") || !strings.Contains(q, "otc_6547.tw") {
		t.Errorf("ex_ch 應含兩檔且市場別正確，實際 %q", q)
	}
	if len(rings.Snapshots("2330")) != 1 || len(rings.Snapshots("6547")) != 1 {
		t.Error("快照應已寫入 RingStore")
	}
}

// WarmupSession：GET index.jsp 取 Cookie；官方改版 404 時回傳錯誤但不阻斷
// （Run 內僅記錄）。
func TestMISWarmupSession(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := NewBaseClient("mis.twse.com.tw",
		WithRateInterval(time.Microsecond), WithJitterRatio(0))
	wk := NewMISWorker(client, testWatchlist(), engine.NewRingStore(),
		WithMISURLs(srv.URL+"/index.jsp", srv.URL+"/api"))

	if err := wk.warmupSession(context.Background()); err != nil {
		t.Fatalf("warmup 失敗: %v", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Errorf("warmup 應 1 次 GET index.jsp，實際 %d", hits)
	}

	// 404：回傳錯誤（Run 會記錄後繼續）
	srv404 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv404.Close()
	wk404 := NewMISWorker(client, testWatchlist(), engine.NewRingStore(),
		WithMISURLs(srv404.URL+"/index.jsp", srv404.URL+"/api"))
	if err := wk404.warmupSession(context.Background()); err == nil {
		t.Error("404 warmup 應回傳錯誤（由 Run 記錄處理）")
	}
}

// 重採樣正確性端到端：真實 fixture 寫入 RingStore 後聚合出含影線 K 線
// （收盤競價 tv 落入 13:30 桶）。
func TestMISToKlineEndToEnd(t *testing.T) {
	rings := engine.NewRingStore()
	client := NewBaseClient("mis.twse.com.tw",
		WithRateInterval(time.Microsecond), WithJitterRatio(0))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(misFixture))
	}))
	defer srv.Close()
	wk := NewMISWorker(client, testWatchlist(), rings,
		WithMISURLs(srv.URL, srv.URL))
	if _, err := wk.pollAndStore(context.Background()); err != nil {
		t.Fatal(err)
	}
	bars, err := engine.NewAggregator(rings).Klines("2330", "1m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Timestamp != "14:30:00" {
		t.Fatalf("快照 tlong 14:30 應落入 14:30:00 桶，實際 %+v", bars)
	}
	if bars[0].Open != 2425 || bars[0].High != 2425 || bars[0].Low != 2425 || bars[0].Close != 2425 {
		t.Errorf("單筆桶 OHLC 應皆為 z=2425，實際 %+v", bars[0])
	}
	if bars[0].Volume != 4512000 {
		t.Errorf("單筆桶 Volume 應為 tv=4,512,000 股，實際 %d", bars[0].Volume)
	}
}
