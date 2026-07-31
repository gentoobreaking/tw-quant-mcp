package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// twseFixture 為 TWSE openapi t187ap05_L 之欄位格式樣本（公司代號/名稱/產業別）。
const twseFixture = `[
	{"公司代號":"1101","公司名稱":"台泥","產業別":"水泥工業"},
	{"公司代號":"2330","公司名稱":"台積電","產業別":"半導體業"},
	{"公司代號":"3045","公司名稱":"台灣大哥大","產業別":"通信網路業"}
]`

// tpexFixture 為 TPEx openapi tpex_mainboard_daily_close_quotes 之欄位格式樣本
// （SecuritiesCompanyCode/CompanyName；含 6 碼 ETF 與 4 碼股票）。
const tpexFixture = `[
	{"SecuritiesCompanyCode":"006201","CompanyName":"元大富櫃50"},
	{"SecuritiesCompanyCode":"6547","CompanyName":"高端疫苗"},
	{"SecuritiesCompanyCode":"3226","CompanyName":"至寶電"}
]`

type listServer struct {
	body  []byte
	calls atomic.Int32
}

// newListServers 建立 TWSE/TPEx 兩個官方清單 httptest server。
func newListServers(t *testing.T) (*httptest.Server, *listServer, *httptest.Server, *listServer) {
	t.Helper()
	twse := &listServer{body: []byte(twseFixture)}
	tpex := &listServer{body: []byte(tpexFixture)}
	s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		twse.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(twse.body)
	}))
	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tpex.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(tpex.body)
	}))
	return s1, twse, s2, tpex
}

// withURLs 以 httptest URL 覆寫官方清單端點（測試用），回傳還原函式。
func withURLs(t *testing.T, twseURL, tpexURL string) func() {
	t.Helper()
	otwse, otpex := twseListURL, tpexListURL
	twseListURL, tpexListURL = twseURL, tpexURL
	return func() { twseListURL, tpexListURL = otwse, otpex }
}

func testClient() *provider.BaseClient {
	return provider.NewBaseClient("openapi.twse.com.tw",
		provider.WithRateInterval(time.Microsecond), provider.WithJitterRatio(0))
}

// 載入後：上市/上櫃判定正確、ex_ch 組裝、未知代碼 miss。
func TestLoaderLoad(t *testing.T) {
	s1, _, s2, _ := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer withURLs(t, s1.URL, s2.URL)()

	l := NewLoader(testClient(), nil)
	reg, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}

	if reg.Len() != 6 {
		t.Errorf("應載入 6 檔（3 上市 + 3 上櫃），實際 %d", reg.Len())
	}
	if s, ok := reg.Lookup("2330"); !ok || s.Market != model.MarketTSE || s.Name != "台積電" {
		t.Errorf("2330 = %+v", s)
	}
	if s, ok := reg.Lookup("2330"); ok && s.Category != "半導體業" {
		t.Errorf("2330 產業別 = %q", s.Category)
	}
	if s, ok := reg.Lookup("6547"); !ok || s.Market != model.MarketOTC || s.Exch() != "otc_6547.tw" {
		t.Errorf("6547 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("006201"); !ok || s.Market != model.MarketOTC || s.Exch() != "otc_006201.tw" {
		t.Errorf("6 碼上櫃 ETF = %+v ok=%v", s, ok)
	}
	if _, ok := reg.Lookup("9999"); ok {
		t.Error("未知代碼應 miss")
	}
	if m, ok := reg.Market("1101"); !ok || m != model.MarketTSE {
		t.Errorf("Market(1101) = %s/%v", m, ok)
	}
}

// 24h TTL 快取：同鍵僅一次上游呼叫；L2 持久化（重啟 cache 後仍命中）。
func TestLoaderCacheAndL2Persistence(t *testing.T) {
	s1, twse, s2, tpex := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer withURLs(t, s1.URL, s2.URL)()

	dir := t.TempDir()
	cch, err := cache.New(cache.WithDataDir(dir))
	if err != nil {
		t.Fatal(err)
	}

	l := NewLoader(testClient(), cch)
	if _, err := l.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 {
		t.Fatalf("首次應各 1 次上游呼叫，實際 twse=%d tpex=%d", twse.calls.Load(), tpex.calls.Load())
	}
	// 24h 內第二次：L1 命中。
	if _, err := l.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 {
		t.Errorf("24h 內第二次應命中快取，實際 twse=%d tpex=%d", twse.calls.Load(), tpex.calls.Load())
	}
	// 模擬進程重啟（L1 消失、L2 仍在）：仍不觸發上游。
	cch.Close()
	cch2, err := cache.New(cache.WithDataDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer cch2.Close()
	l2 := NewLoader(testClient(), cch2)
	reg, err := l2.Load(context.Background())
	if err != nil {
		t.Fatalf("重啟後 Load 失敗: %v", err)
	}
	if reg.Len() != 6 {
		t.Errorf("重啟後應自 L2 回復 6 檔，實際 %d", reg.Len())
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 {
		t.Errorf("重啟後應命中 L2，實際 twse=%d tpex=%d", twse.calls.Load(), tpex.calls.Load())
	}
}

// 無 cache：每次 Load 直接抓取。
func TestLoaderNoCache(t *testing.T) {
	s1, twse, s2, tpex := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer withURLs(t, s1.URL, s2.URL)()

	l := NewLoader(testClient(), nil)
	for i := 0; i < 2; i++ {
		if _, err := l.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if twse.calls.Load() != 2 || tpex.calls.Load() != 2 {
		t.Errorf("無 cache 應每次抓取，實際 twse=%d tpex=%d", twse.calls.Load(), tpex.calls.Load())
	}
}

// 官方清單格式變更（全數無效）：回傳明確錯誤。
func TestLoaderParseAllInvalid(t *testing.T) {
	s1, twse, s2, tpex := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer withURLs(t, s1.URL, s2.URL)()

	twse.body = []byte(`[{"公司代號":"","公司名稱":"","產業別":""}]`)
	tpex.body = []byte(`[{"SecuritiesCompanyCode":"abc","CompanyName":""}]`)

	l := NewLoader(testClient(), nil)
	if _, err := l.Load(context.Background()); err == nil {
		t.Error("全數無效之官方清單應回傳錯誤")
	}
}

// 個別無效記錄略過，不影響其餘。
func TestParseSkipsInvalid(t *testing.T) {
	symbols, err := parseTWSEList([]byte(`[
		{"公司代號":"1101","公司名稱":"台泥","產業別":"水泥工業"},
		{"公司代號":"xyz","公司名稱":"壞紀錄","產業別":"X"},
		{"公司代號":"2330","公司名稱":"台積電","產業別":"半導體業"}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 2 {
		t.Errorf("應略過無效記錄，實際 %d", len(symbols))
	}
}

// 任一市場失敗即 Load 失敗（Registry 為核心基礎設施，缺市場別可能導致路由錯誤）。
func TestLoaderPartialFailure(t *testing.T) {
	s1, _, _, _ := newListServers(t)
	defer s1.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	defer withURLs(t, s1.URL, bad.URL)()

	l := NewLoader(testClient(), nil)
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("TPEx 失敗時 Load 應回傳錯誤")
	} else {
		t.Logf("預期錯誤：%v", err)
	}
}

// client 為 nil 時直接回傳錯誤。
func TestLoaderNilClient(t *testing.T) {
	l := NewLoader(nil, nil)
	if _, err := l.Load(context.Background()); err == nil {
		t.Error("client 為 nil 應回傳錯誤")
	}
}

func TestSourceIDFor(t *testing.T) {
	if got := sourceIDFor(twseListURL); got != model.SourceTWSEAPI {
		t.Errorf("sourceIDFor(twse) = %s", got)
	}
	if got := sourceIDFor(tpexListURL); got != model.SourceTPExAPI {
		t.Errorf("sourceIDFor(tpex) = %s", got)
	}
}
