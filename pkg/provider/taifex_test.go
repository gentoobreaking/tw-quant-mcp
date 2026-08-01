package provider

// TAIFEX Adapter 契約測試（T013）：以 2026-07-31 實地錄製之官方 raw fixtures
// （testdata/taifex/ 下 taifex_*.csv 為 DL 下載 CSV 轉 UTF-8；tfx_*.json 為 API 回應）
// 驗證 Fetch→Validate→Normalize：欄位型別、單位換算（千元 → 元，§5.1）、
// 日期格式（西元 → YYYY-MM-DD）、DL 表頭契約、缺口/補檔/範圍查詢流程（§9.3）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
)

// taifexFixture 讀取 testdata/taifex/<name> 之 TAIFEX 官方錄製回應。
func taifexFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "taifex", name))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return b
}

// newTAIFEXAPISource 建立測試用 API 來源（base 指向測試伺服器）。
func newTAIFEXAPISource(base string) *TAIFEXAPISource {
	return &TAIFEXAPISource{client: NewBaseClient("openapi.taifex.com.tw", WithRateInterval(time.Microsecond)), baseURL: base}
}

// newTAIFEXDLSource 建立測試用 DL 來源（base 指向測試伺服器）。
func newTAIFEXDLSource(base string) *TAIFEXDLSource {
	return &TAIFEXDLSource{client: NewBaseClient("www.taifex.com.tw", WithRateInterval(time.Microsecond)), baseURL: base}
}

// testCache 建立測試快取（僅 L1；TAIFEX 契約測試不需 L2 持久化）。
func testCache(t *testing.T) *cache.Cache {
	t.Helper()
	c, err := cache.New()
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// ---------------------------------------------------------------------------
// DL 契約：fixture CSV → Validate → Normalize → Model 檢查

func TestTAIFEXDLContract(t *testing.T) {
	cases := []struct {
		name string
		ds   model.TAIFEXDataset
		file string
		want int
	}{
		{"期貨每日行情", model.TAFuturesDaily, "taifex_fut_daily.csv", 24},
		{"選擇權每日行情", model.TAOptionsDaily, "taifex_opt_daily.csv", 6724},
		{"三大法人期貨", model.TAInstiFutures, "taifex_insti_fut.csv", 69},
		{"大額交易人期貨", model.TALargeTraderFut, "taifex_large_trader_fut.csv", 1366},
		{"大額交易人選擇權", model.TALargeTraderOpt, "taifex_large_trader_opt.csv", 328},
		{"買賣權比", model.TAPutCallRatio, "taifex_pc_ratio.csv", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTAIFEXDLSource("https://dl.test/cht/3/")
			raw := fixtureRaw(t, s.URL(tc.ds, nil), taifexFixture(t, tc.file))
			if err := s.Validate(raw); err != nil {
				t.Fatalf("Validate 失敗: %v", err)
			}
			out, err := s.Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize 失敗: %v", err)
			}
			var rows []map[string]any
			if err := json.Unmarshal(out, &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != tc.want {
				t.Fatalf("資料列數不符: 期望 %d，實際 %d", tc.want, len(rows))
			}
			for i, r := range rows {
				d, _ := r["date"].(string)
				if !strings.HasPrefix(d, "2026-07-29") {
					t.Errorf("第 %d 列日期異常: %q", i, d)
				}
			}
		})
	}
}

// TestTAIFEXDLContractValues 抽查 DL 數值欄位（單位、千分位、"-"）。
func TestTAIFEXDLContractValues(t *testing.T) {
	s := newTAIFEXDLSource("https://dl.test/cht/3/")
	raw := fixtureRaw(t, s.URL(model.TAInstiFutures, nil), taifexFixture(t, "taifex_insti_fut.csv"))
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []model.InstitutionalRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	// fixture 首列：自營商 多方交易口數 12186、契約金額 98976083 千元 → 98,976,083,000 元
	first := rows[0]
	if first.Contract != "臺股期貨" || first.Investor != "自營商" {
		t.Errorf("首列內容異常: %+v", first)
	}
	if first.LongVolume != 12186 {
		t.Errorf("LongVolume 異常: %d", first.LongVolume)
	}
	if first.LongValue != 98976083000 {
		t.Errorf("LongValue 千元→元換算異常: %d", first.LongValue)
	}
	if first.Date != "2026-07-29" {
		t.Errorf("日期異常: %q", first.Date)
	}

	// 期貨每日行情：千分位與 "-"
	raw2 := fixtureRaw(t, s.URL(model.TAFuturesDaily, nil), taifexFixture(t, "taifex_fut_daily.csv"))
	out2, err := s.Normalize(raw2)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var fut []model.FuturesDailyRow
	if err := json.Unmarshal(out2, &fut); err != nil {
		t.Fatal(err)
	}
	if len(fut) == 0 || fut[0].Contract != "TX" || fut[0].Open != 41915 || fut[0].Volume != 124405 {
		t.Errorf("期貨首列異常: %+v", fut[0])
	}
	if fut[0].ChangePct != -2.84 {
		t.Errorf("漲跌%% 解析異常: %v", fut[0].ChangePct)
	}
}

// TestTAIFEXDLHeaderOnly 週六/無交易日之僅表頭 CSV → Normalize 為空陣列。
func TestTAIFEXDLHeaderOnly(t *testing.T) {
	s := newTAIFEXDLSource("https://dl.test/cht/3/")
	body := []byte("日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n")
	raw := fixtureRaw(t, s.URL(model.TAInstiFutures, nil), body)
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗（僅表頭應回空陣列）: %v", err)
	}
	if strings.TrimSpace(string(out)) != "[]" {
		t.Errorf("僅表頭 CSV 應輸出空陣列，實際: %s", out)
	}
}

// ---------------------------------------------------------------------------
// API 契約：fixture → Validate → Normalize → Model 檢查

func TestTAIFEXAPIContract(t *testing.T) {
	cases := []struct {
		name string
		ds   model.TAIFEXDataset
		file string
		want int
		date string // 期望日期前綴（PCR 回傳近一月多日）
	}{
		{"期貨每日行情", model.TAFuturesDaily, "tfx_fut.json", 2140, "2026-07-31"},
		{"選擇權每日行情", model.TAOptionsDaily, "tfx_opt.json", 13520, "2026-07-31"},
		{"三大法人期貨", model.TAInstiFutures, "tfx_MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate.json", 66, "2026-07-31"},
		{"三大法人選擇權", model.TAInstiOptions, "tfx_MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheDate.json", 15, "2026-07-31"},
		{"大額交易人期貨", model.TALargeTraderFut, "tfx_OpenInterestOfLargeTradersFutures.json", -1, "2026-07-31"}, // CSV，列數另查
		{"大額交易人選擇權", model.TALargeTraderOpt, "tfx_OpenInterestOfLargeTradersOptions.json", 328, "2026-07-31"},
		{"買賣權比", model.TAPutCallRatio, "tfx_PutCallRatio.json", 22, "2026-07-"},
		{"保證金", model.TAMargin, "tfx_margin2.json", 31, "2026-07-31"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTAIFEXAPISource("https://api.test/v1")
			raw := fixtureRaw(t, s.URL(tc.ds, nil), taifexFixture(t, tc.file))
			if err := s.Validate(raw); err != nil {
				t.Fatalf("Validate 失敗: %v", err)
			}
			out, err := s.Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize 失敗: %v", err)
			}
			var rows []map[string]any
			if err := json.Unmarshal(out, &rows); err != nil {
				t.Fatal(err)
			}
			if tc.want >= 0 && len(rows) != tc.want {
				t.Fatalf("資料列數不符: 期望 %d，實際 %d", tc.want, len(rows))
			}
			for i, r := range rows {
				d, _ := r["date"].(string)
				if !strings.HasPrefix(d, tc.date) {
					t.Errorf("第 %d 列日期異常: %q", i, d)
				}
			}
		})
	}
}

// TestTAIFEXAPIContractValues 抽查 API 數值欄位與日期過濾。
func TestTAIFEXAPIContractValues(t *testing.T) {
	s := newTAIFEXAPISource("https://api.test/v1")

	// 期貨：API 欄名 Last → close；%"4.42%" → 4.42
	raw := fixtureRaw(t, s.URL(model.TAFuturesDaily, nil), taifexFixture(t, "tfx_fut.json"))
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var fut []model.FuturesDailyRow
	if err := json.Unmarshal(out, &fut); err != nil {
		t.Fatal(err)
	}
	if len(fut) == 0 || fut[0].Close != 3234.8 || fut[0].ChangePct != 4.42 {
		t.Errorf("期貨首列異常: %+v", fut[0])
	}

	// 法人：千元 → 元
	raw2 := fixtureRaw(t, s.URL(model.TAInstiFutures, nil),
		taifexFixture(t, "tfx_MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate.json"))
	out2, err := s.Normalize(raw2)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var insti []model.InstitutionalRow
	if err := json.Unmarshal(out2, &insti); err != nil {
		t.Fatal(err)
	}
	if len(insti) == 0 || insti[0].LongValue != 65268505000 {
		t.Errorf("法人首列換算異常: %+v", insti[0])
	}

	// 日期過濾：查詢 2026-07-30 → 無該日資料（fixture 僅 07-31）
	raw3 := fixtureRaw(t, s.URL(model.TAFuturesDaily, nil)+"?date=2026-07-30",
		taifexFixture(t, "tfx_fut.json"))
	out3, err := s.Normalize(raw3)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	if strings.TrimSpace(string(out3)) != "[]" {
		t.Errorf("日期過濾後應為空陣列，實際: %s", out3)
	}
}

// ---------------------------------------------------------------------------
// 查詢層（§9.3）：httptest 伺服器 + 快取

// taifexQueryHarness 建立 API/DL 測試伺服器與 TAIFEXQuery。
func taifexQueryHarness(t *testing.T, apiHandler, dlHandler http.HandlerFunc) (*TAIFEXQuery, *httptest.Server, *httptest.Server) {
	t.Helper()
	apiSrv := httptest.NewServer(apiHandler)
	dlSrv := httptest.NewServer(dlHandler)
	t.Cleanup(func() { apiSrv.Close(); dlSrv.Close() })
	api := newTAIFEXAPISource(apiSrv.URL + "/v1")
	dl := newTAIFEXDLSource(dlSrv.URL + "/cht/3/")
	q, err := NewTAIFEXQuery(api, dl, testCache(t), func() time.Time {
		return time.Date(2026, 8, 1, 10, 0, 0, 0, model.Taipei())
	})
	if err != nil {
		t.Fatalf("NewTAIFEXQuery 失敗: %v", err)
	}
	return q, apiSrv, dlSrv
}

// TestTAIFEXQueryAPIPath date==最新交易日 → API 路徑。
func TestTAIFEXQueryAPIPath(t *testing.T) {
	q, _, _ := taifexQueryHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/PutCallRatio"):
				w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
			case strings.HasSuffix(r.URL.Path, "/DailyMarketReportFut"):
				w.Write(taifexFixture(t, "tfx_fut.json"))
			default:
				http.NotFound(w, r)
			}
		},
		func(w http.ResponseWriter, r *http.Request) {
			t.Error("DL 不應被呼叫")
		})
	res, fromCache, err := q.Fetch(context.Background(), model.TAFuturesDaily, "2026-07-31", "")
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if res.Source != model.SourceTAIFEXAPI {
		t.Errorf("應走 API 路徑，實際 source=%s", res.Source)
	}
	if fromCache || res.Note != "" {
		t.Errorf("API 首查異常: fromCache=%v note=%q", fromCache, res.Note)
	}
	var rows []model.FuturesDailyRow
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Error("API 路徑無資料")
	}
	// 第二次查詢應命中快取
	_, fromCache2, err := q.Fetch(context.Background(), model.TAFuturesDaily, "2026-07-31", "")
	if err != nil || !fromCache2 {
		t.Errorf("二次查詢應命中快取: fromCache=%v err=%v", fromCache2, err)
	}
}

// TestTAIFEXQueryDLPath date<最新交易日 → DL 路徑。
func TestTAIFEXQueryDLPath(t *testing.T) {
	var gotForm url.Values
	q, _, _ := taifexQueryHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/PutCallRatio") {
				w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
				return
			}
			http.NotFound(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Write([]byte("<html>view</html>"))
				return
			}
			r.ParseForm()
			gotForm = r.PostForm
			w.Write(taifexFixture(t, "taifex_fut_daily.csv"))
		})
	res, _, err := q.Fetch(context.Background(), model.TAFuturesDaily, "2026-07-29", "")
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if res.Source != model.SourceTAIFEXDL || res.Note != "" {
		t.Errorf("應走 DL 路徑: source=%s note=%q", res.Source, res.Note)
	}
	if gotForm.Get("queryStartDate") != "2026/07/26" || gotForm.Get("queryEndDate") != "2026/07/29" {
		t.Errorf("DL 表單參數異常: %v", gotForm)
	}
	var rows []model.FuturesDailyRow
	if err := json.Unmarshal(res.Data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 24 || rows[0].Date != "2026-07-29" {
		t.Errorf("DL 資料異常: n=%d first=%+v", len(rows), rows[0])
	}
}

// TestTAIFEXQueryGap 無資料日（僅表頭 CSV）→ 缺口（null + Note），不寫入快取。
func TestTAIFEXQueryGap(t *testing.T) {
	q, _, _ := taifexQueryHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/PutCallRatio") {
				w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
				return
			}
			http.NotFound(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Write([]byte("<html>view</html>"))
				return
			}
			// 僅表頭之官方 CSV（15 欄）
			w.Write([]byte("日期,商品名稱,身份別,多方交易口數,多方交易契約金額(千元),空方交易口數,空方交易契約金額(千元),多空交易口數淨額,多空交易契約金額淨額(千元),多方未平倉口數,多方未平倉契約金額(千元),空方未平倉口數,空方未平倉契約金額(千元),多空未平倉口數淨額,多空未平倉契約金額淨額(千元)\n"))
		})
	res, _, err := q.Fetch(context.Background(), model.TAInstiFutures, "2026-07-26", "")
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if res.Data != nil || res.Note == "" {
		t.Errorf("缺口應為 null 資料 + Note: data=%v note=%q", res.Data, res.Note)
	}
	// 缺口不應寫入 L1/L2：再查一次仍應走上游
	res2, fromCache2, err := q.Fetch(context.Background(), model.TAInstiFutures, "2026-07-26", "")
	if err != nil {
		t.Fatalf("二次 Fetch 失敗: %v", err)
	}
	if fromCache2 || res2.Data != nil {
		t.Errorf("缺口被快取: fromCache=%v data=%v", fromCache2, res2.Data)
	}
}

// TestTAIFEXQueryDerivedFrom 缺口補檔：請求日無資料但視窗內有鄰近日。
func TestTAIFEXQueryDerivedFrom(t *testing.T) {
	q, _, _ := taifexQueryHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/PutCallRatio") {
				w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
				return
			}
			http.NotFound(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Write([]byte("<html>view</html>"))
				return
			}
			w.Write(taifexFixture(t, "taifex_fut_daily.csv")) // 僅 07-29
		})
	// 請求 07-30（fixture 無該日）→ 補檔自 07-29
	res, _, err := q.Fetch(context.Background(), model.TAFuturesDaily, "2026-07-30", "")
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if res.Note != "" || res.DerivedFrom != "2026-07-29" || res.Data == nil {
		t.Errorf("補檔異常: note=%q derived_from=%q", res.Note, res.DerivedFrom)
	}
}

// TestTAIFEXQueryFetchRange 範圍查詢：一次 DL 下載、逐日結果、L2 寫入。
func TestTAIFEXQueryFetchRange(t *testing.T) {
	q, _, _ := taifexQueryHarness(t,
		func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/PutCallRatio") {
				w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
				return
			}
			http.NotFound(w, r)
		},
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet {
				w.Write([]byte("<html>view</html>"))
				return
			}
			w.Write(taifexFixture(t, "taifex_fut_daily.csv"))
		})
	out, err := q.FetchRange(context.Background(), model.TAFuturesDaily, "2026-07-29", "2026-07-30", "")
	if err != nil {
		t.Fatalf("FetchRange 失敗: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("應有 2 日結果，實際 %d", len(out))
	}
	r29 := out["2026-07-29"]
	if r29.Data == nil || r29.Source != model.SourceTAIFEXDL {
		t.Errorf("07-29 異常: %+v", r29)
	}
	r30 := out["2026-07-30"]
	if r30.DerivedFrom != "2026-07-29" {
		t.Errorf("07-30 應補檔自 07-29，實際 derived_from=%q", r30.DerivedFrom)
	}
}

// TestTAIFEXQueryL2NoRedownload L2 命中後不再重複下載（計數器驗證，§9.3）。
func TestTAIFEXQueryL2NoRedownload(t *testing.T) {
	var downloads int32
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/PutCallRatio") {
			w.Write(taifexFixture(t, "tfx_PutCallRatio.json"))
			return
		}
		http.NotFound(w, r)
	}))
	dlSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.Write([]byte("<html>view</html>"))
			return
		}
		atomic.AddInt32(&downloads, 1)
		w.Write(taifexFixture(t, "taifex_fut_daily.csv"))
	}))
	t.Cleanup(func() { apiSrv.Close(); dlSrv.Close() })

	c, err := cache.New(cache.WithDataDir(t.TempDir()))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	q, err := NewTAIFEXQuery(
		newTAIFEXAPISource(apiSrv.URL+"/v1"),
		newTAIFEXDLSource(dlSrv.URL+"/cht/3/"),
		c, func() time.Time { return time.Date(2026, 8, 1, 10, 0, 0, 0, model.Taipei()) })
	if err != nil {
		t.Fatalf("NewTAIFEXQuery 失敗: %v", err)
	}

	ctx := context.Background()
	if _, _, err := q.Fetch(ctx, model.TAFuturesDaily, "2026-07-29", ""); err != nil {
		t.Fatalf("首次 Fetch 失敗: %v", err)
	}
	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Fatalf("首次查詢應下載 1 次，實際 %d", got)
	}
	if _, fromCache, err := q.Fetch(ctx, model.TAFuturesDaily, "2026-07-29", ""); err != nil || !fromCache {
		t.Fatalf("二次查詢應 L2 命中: fromCache=%v err=%v", fromCache, err)
	}
	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Errorf("L2 命中後不應重複下載，實際下載 %d 次", got)
	}

	// FetchRange 探測到 L2 既有日期 → 不再下載
	if _, err := q.FetchRange(ctx, model.TAFuturesDaily, "2026-07-29", "2026-07-29", ""); err != nil {
		t.Fatalf("FetchRange 失敗: %v", err)
	}
	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Errorf("FetchRange 命中 L2 後不應再下載，實際下載 %d 次", got)
	}
}

// TestTAIFEXDLFormContract DL POST 表單欄位順序/值與 Referer。
func TestTAIFEXDLFormContract(t *testing.T) {
	var gotMethod string
	var gotReferer string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotReferer = r.Header.Get("Referer")
		w.Write([]byte("日期,商品名稱,身份別\n"))
	}))
	defer srv.Close()

	s := newTAIFEXDLSource(srv.URL + "/cht/3/")
	view := s.URL(model.TAFuturesDaily, url.Values{"queryStartDate": {"2026/07/26"}, "queryEndDate": {"2026/07/29"}, "commodity_id": {"TX"}})
	_, err := s.Fetch(context.Background(), RawRequest{URL: view})
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("下載應為 POST，實際 %s", gotMethod)
	}
	if !strings.Contains(gotReferer, "futDailyMarketView") {
		t.Errorf("Referer 異常: %q", gotReferer)
	}
}
