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

// etfFixture 為 TWSE openapi STOCK_DAY_ALL 之欄位格式樣本（含 ETF/ETN/股票/特別股混雜）。
// 依 2026-08-18 實測：上市 ETF 代碼為 4 碼（0050）、5 碼（00636）、6 碼（006208/00400A/00679B）
// 且均以 00 開頭；4 碼一般股票（2330/1101）、6 碼非 00 開頭（02000L ETN、2887Z1 特別股、910322 DR）
// 應被 parseTWSEETFList 排除。00899 為 STOCK_DAY_ALL 中唯一可能非 ETF 之 00 開頭列（官方未提供
// 類型欄位，規則先保留，見 TestParseTWSEETFList）。
const etfFixture = `[
	{"Code":"0050","Name":"元大台灣50","Date":"1150818","ClosingPrice":"104.90"},
	{"Code":"0056","Name":"元大高股息","Date":"1150818","ClosingPrice":"32.10"},
	{"Code":"006208","Name":"富邦台50","Date":"1150818","ClosingPrice":"125.00"},
	{"Code":"00636","Name":"國泰中國A50","Date":"1150818","ClosingPrice":"21.30"},
	{"Code":"2330","Name":"台積電","Date":"1150818","ClosingPrice":"920.00"},
	{"Code":"00679B","Name":"元大美債20年","Date":"1150818","ClosingPrice":"30.20"},
	{"Code":"00400A","Name":"國泰台灣高股息","Date":"1150818","ClosingPrice":"50.00"},
	{"Code":"1101","Name":"台泥","Date":"1150818","ClosingPrice":"40.50"},
	{"Code":"02000L","Name":"富邦蘋果正二N","Date":"1150818","ClosingPrice":"12.30"},
	{"Code":"2887Z1","Name":"台新新光己特","Date":"1150818","ClosingPrice":"50.00"},
	{"Code":"910322","Name":"康師傅-DR","Date":"1150818","ClosingPrice":"35.00"},
	{"Code":"00899","Name":"FT潔淨能源","Date":"1150818","ClosingPrice":"18.50"}
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

// newListServers 建立 TWSE/TPEx/ETF 三個官方清單 httptest server。
func newListServers(t *testing.T) (*httptest.Server, *listServer, *httptest.Server, *listServer, *httptest.Server, *listServer) {
	t.Helper()
	twse := &listServer{body: []byte(twseFixture)}
	tpex := &listServer{body: []byte(tpexFixture)}
	etf := &listServer{body: []byte(etfFixture)}
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
	s3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		etf.calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(etf.body)
	}))
	return s1, twse, s2, tpex, s3, etf
}

// withURLs 以 httptest URL 覆寫官方清單端點（測試用），回傳還原函式。
func withURLs(t *testing.T, twseURL, tpexURL, etfURL string) func() {
	t.Helper()
	otwse, otpex, oetf := twseListURL, tpexListURL, twseETFListURL
	twseListURL, tpexListURL, twseETFListURL = twseURL, tpexURL, etfURL
	return func() { twseListURL, tpexListURL, twseETFListURL = otwse, otpex, oetf }
}

func testClient() *provider.BaseClient {
	return provider.NewBaseClient("openapi.twse.com.tw",
		provider.WithRateInterval(time.Microsecond), provider.WithJitterRatio(0))
}

// 載入後：上市/上櫃/ETF 判定正確、ex_ch 組裝、未知代碼 miss。
func TestLoaderLoad(t *testing.T) {
	s1, _, s2, _, s3, _ := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()
	defer withURLs(t, s1.URL, s2.URL, s3.URL)()

	l := NewLoader(testClient(), nil)
	reg, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("Load 失敗: %v", err)
	}

	// 上市 3 + 上櫃 3 + ETF 8 (0050, 0056, 006208, 00636, 00679B, 00400A, 00899, 006201)
	if reg.Len() != 13 {
		t.Errorf("應載入 13 檔（3 上市 + 3 上櫃 + 7 上市 ETF + 1 上櫃 ETF），實際 %d", reg.Len())
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
	// ETF 驗證（4/5/6 碼 00 開頭皆應入列，含上櫃 6 碼）
	if s, ok := reg.Lookup("0050"); !ok || s.Market != model.MarketTSE || s.Name != "元大台灣50" {
		t.Errorf("0050 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("0056"); !ok || s.Market != model.MarketTSE || s.Name != "元大高股息" {
		t.Errorf("0056 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("006208"); !ok || s.Market != model.MarketTSE || s.Name != "富邦台50" {
		t.Errorf("006208 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("00636"); !ok || s.Market != model.MarketTSE || s.Name != "國泰中國A50" {
		t.Errorf("00636 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("00400A"); !ok || s.Market != model.MarketTSE || s.Name != "國泰台灣高股息" {
		t.Errorf("00400A = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("00679B"); !ok || s.Market != model.MarketTSE || s.Name != "元大美債20年" {
		t.Errorf("00679B = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("00899"); !ok || s.Market != model.MarketTSE || s.Name != "FT潔淨能源" {
		t.Errorf("00899 = %+v ok=%v", s, ok)
	}
	if s, ok := reg.Lookup("006201"); !ok || s.Market != model.MarketOTC || s.Name != "元大富櫃50" {
		t.Errorf("006201 上櫃 ETF = %+v ok=%v", s, ok)
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
	s1, twse, s2, tpex, s3, etf := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()
	defer withURLs(t, s1.URL, s2.URL, s3.URL)()

	dir := t.TempDir()
	cch, err := cache.New(cache.WithDataDir(dir))
	if err != nil {
		t.Fatal(err)
	}

	l := NewLoader(testClient(), cch)
	if _, err := l.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 || etf.calls.Load() != 1 {
		t.Fatalf("首次應各 1 次上游呼叫，實際 twse=%d tpex=%d etf=%d", twse.calls.Load(), tpex.calls.Load(), etf.calls.Load())
	}
	// 24h 內第二次：L1 命中。
	if _, err := l.Load(context.Background()); err != nil {
		t.Fatal(err)
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 || etf.calls.Load() != 1 {
		t.Errorf("24h 內第二次應命中快取，實際 twse=%d tpex=%d etf=%d", twse.calls.Load(), tpex.calls.Load(), etf.calls.Load())
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
	if reg.Len() != 13 {
		t.Errorf("重啟後應自 L2 回復 13 檔，實際 %d", reg.Len())
	}
	if twse.calls.Load() != 1 || tpex.calls.Load() != 1 || etf.calls.Load() != 1 {
		t.Errorf("重啟後應命中 L2，實際 twse=%d tpex=%d etf=%d", twse.calls.Load(), tpex.calls.Load(), etf.calls.Load())
	}
}

// 無 cache：每次 Load 直接抓取。
func TestLoaderNoCache(t *testing.T) {
	s1, twse, s2, tpex, s3, etf := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()
	defer withURLs(t, s1.URL, s2.URL, s3.URL)()

	l := NewLoader(testClient(), nil)
	for i := 0; i < 2; i++ {
		if _, err := l.Load(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if twse.calls.Load() != 2 || tpex.calls.Load() != 2 || etf.calls.Load() != 2 {
		t.Errorf("無 cache 應每次抓取，實際 twse=%d tpex=%d etf=%d", twse.calls.Load(), tpex.calls.Load(), etf.calls.Load())
	}
}

// 官方清單格式變更（全數無效）：回傳明確錯誤。
func TestLoaderParseAllInvalid(t *testing.T) {
	s1, twse, s2, tpex, s3, etf := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	defer s3.Close()
	defer withURLs(t, s1.URL, s2.URL, s3.URL)()

	twse.body = []byte(`[{"公司代號":"","公司名稱":"","產業別":""}]`)
	tpex.body = []byte(`[{"SecuritiesCompanyCode":"abc","CompanyName":""}]`)
	etf.body = []byte(`[{"Code":"0050","Name":""}]`) // ETF 名稱為空，會被過濾

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
// ETF 清單失敗不阻擋整體 Registry。
func TestLoaderPartialFailure(t *testing.T) {
	s1, _, _, _, _, _ := newListServers(t)
	defer s1.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	defer withURLs(t, s1.URL, bad.URL, bad.URL)()

	l := NewLoader(testClient(), nil)
	_, err := l.Load(context.Background())
	if err == nil {
		t.Error("TPEx 失敗時 Load 應回傳錯誤")
	} else {
		t.Logf("預期錯誤：%v", err)
	}
}

// ETF 端點失敗時，整體 Registry 仍成功（僅缺 ETF）。
func TestLoaderETFFailureDoesNotBlock(t *testing.T) {
	s1, _, s2, _, _, _ := newListServers(t)
	defer s1.Close()
	defer s2.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()
	defer withURLs(t, s1.URL, s2.URL, bad.URL)()

	l := NewLoader(testClient(), nil)
	reg, err := l.Load(context.Background())
	if err != nil {
		t.Fatalf("ETF 失敗不應阻擋 Registry: %v", err)
	}
	// 應有 6 檔（3 上市 + 3 上櫃），無 ETF
	if reg.Len() != 6 {
		t.Errorf("ETF 失敗時應載入 6 檔（3 上市 + 3 上櫃），實際 %d", reg.Len())
	}
}

// client 為 nil 時直接回傳錯誤。
func TestLoaderNilClient(t *testing.T) {
	l := NewLoader(nil, nil)
	if _, err := l.Load(context.Background()); err == nil {
		t.Error("client 為 nil 應回傳錯誤")
	}
}

// parseTWSEETFList 測試：僅 6 碼 00 開頭入列、4 碼股票/權證排除、名稱正確。
func TestParseTWSEETFList(t *testing.T) {
	symbols, err := parseTWSEETFList([]byte(etfFixture))
	if err != nil {
		t.Fatal(err)
	}
	// 00 開頭 4/5/6 碼共 7 檔入列（0050, 0056, 006208, 00636, 00679B, 00400A, 00899；
	// 006201 為上櫃 ETF 由 tpexFixture 處理，不在本 fixture）
	if len(symbols) != 7 {
		t.Errorf("應解析 7 檔 ETF，實際 %d", len(symbols))
	}
	found := map[string]bool{}
	for _, s := range symbols {
		found[s.Code] = true
		if s.Market != model.MarketTSE {
			t.Errorf("%s 市場別應為 tse，實際 %s", s.Code, s.Market)
		}
		if s.Category != "" {
			t.Errorf("%s 產業別應為空，實際 %q", s.Code, s.Category)
		}
	}
	want := []string{"0050", "0056", "006208", "00636", "00679B", "00400A", "00899"}
	for _, c := range want {
		if !found[c] {
			t.Errorf("缺少 ETF %s", c)
		}
	}
	// 非 ETF 代碼不應入列
	for _, c := range []string{"2330", "1101"} {
		if found[c] {
			t.Errorf("非 ETF 代碼（4 碼股票）%s 不應入列", c)
		}
	}
	for _, c := range []string{"02000L", "2887Z1", "910322"} {
		if found[c] {
			t.Errorf("非 00 開頭 6 碼（ETN/特別股/DR）%s 不應入列", c)
		}
	}
	// 名稱正確
	for _, s := range symbols {
		switch s.Code {
		case "0050":
			if s.Name != "元大台灣50" {
				t.Errorf("0050 名稱錯誤: %s", s.Name)
			}
		case "0056":
			if s.Name != "元大高股息" {
				t.Errorf("0056 名稱錯誤: %s", s.Name)
			}
		case "006208":
			if s.Name != "富邦台50" {
				t.Errorf("006208 名稱錯誤: %s", s.Name)
			}
		case "00636":
			if s.Name != "國泰中國A50" {
				t.Errorf("00636 名稱錯誤: %s", s.Name)
			}
		case "00400A":
			if s.Name != "國泰台灣高股息" {
				t.Errorf("00400A 名稱錯誤: %s", s.Name)
			}
		case "00679B":
			if s.Name != "元大美債20年" {
				t.Errorf("00679B 名稱錯誤: %s", s.Name)
			}
		case "00899":
			if s.Name != "FT潔淨能源" {
				t.Errorf("00899 名稱錯誤: %s", s.Name)
			}
		}
	}
}

func TestSourceIDFor(t *testing.T) {
	if got := sourceIDFor(twseListURL); got != model.SourceTWSEAPI {
		t.Errorf("sourceIDFor(twse) = %s", got)
	}
	if got := sourceIDFor(tpexListURL); got != model.SourceTPExAPI {
		t.Errorf("sourceIDFor(tpex) = %s", got)
	}
	if got := sourceIDFor(twseETFListURL); got != model.SourceTWSEAPI {
		t.Errorf("sourceIDFor(etf) = %s", got)
	}
}
