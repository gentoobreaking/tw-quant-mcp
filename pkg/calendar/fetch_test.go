package calendar

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/provider"
)

// parseSchedule：官方回應解析（含交易日標記排除），以真實官方 JSON 為 fixture。
func TestParseSchedule(t *testing.T) {
	holidays, err := parseSchedule([]byte(scheduleFixture))
	if err != nil {
		t.Fatalf("parseSchedule 失敗: %v", err)
	}
	if len(holidays) != 24 {
		t.Fatalf("官方 2026 表應解析出 24 筆休市日，實際 %d", len(holidays))
	}
	for _, h := range holidays {
		if strings.Contains(h.Name, "交易日") {
			t.Errorf("休市清單不得包含交易日標記：%s", h.Name)
		}
		if _, err := time.Parse("2006-01-02", h.Date); err != nil {
			t.Errorf("日期格式錯誤：%s", h.Date)
		}
	}
	if holidays[0].Name != "中華民國開國紀念日" {
		t.Errorf("首筆應為開國紀念日，實際 %s", holidays[0].Name)
	}
}

func TestParseScheduleErrors(t *testing.T) {
	if _, err := parseSchedule([]byte("{bad json")); err == nil {
		t.Error("非法 JSON 應回傳錯誤")
	}
	if _, err := parseSchedule([]byte(`{"stat":"fail","data":[]}`)); err == nil {
		t.Error("stat!=ok 應回傳錯誤")
	}
	if _, err := parseSchedule([]byte(`{"stat":"ok","data":[["2026-01-02","國曆新年開始交易日",""]]}`)); err == nil {
		t.Error("全部為交易日標記時無休市日應回傳錯誤")
	}
}

// LoadFromOfficial：httptest + 24h TTL 快取合流（同鍵僅一次上游呼叫）。
func TestLoadFromOfficial(t *testing.T) {
	srv, calls := newScheduleServer(t)
	defer srv.Close()
	defer withScheduleURL(t, srv.URL)()

	client := provider.NewBaseClient("www.twse.com.tw",
		provider.WithRateInterval(time.Microsecond), provider.WithJitterRatio(0))
	cch, err := cache.New(cache.WithDataDir(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	defer cch.Close()

	cal := New()
	if err := cal.LoadFromOfficial(context.Background(), client, cch); err != nil {
		t.Fatalf("首次載入失敗: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("首次應 1 次上游呼叫，實際 %d", calls.Load())
	}
	if cal.IsTradingDay(tp(2026, 1, 1)) {
		t.Error("官方資料合併後元旦應為休市日")
	}

	// 24h TTL 內第二次載入：快取命中，無上游呼叫。
	if err := cal.LoadFromOfficial(context.Background(), client, cch); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("24h 內第二次載入應命中快取，實際上游呼叫 %d 次", calls.Load())
	}
}

// LoadFromOfficial 未提供 cache：直接抓取（不經快取層）。
func TestLoadFromOfficialNoCache(t *testing.T) {
	srv, calls := newScheduleServer(t)
	defer srv.Close()
	defer withScheduleURL(t, srv.URL)()

	client := provider.NewBaseClient("www.twse.com.tw",
		provider.WithRateInterval(time.Microsecond), provider.WithJitterRatio(0))

	cal := New()
	for i := 0; i < 2; i++ {
		if err := cal.LoadFromOfficial(context.Background(), client, nil); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("無 cache 時每次應直接抓取，實際 %d 次", calls.Load())
	}
}

func TestLoadFromOfficialErrors(t *testing.T) {
	if err := New().LoadFromOfficial(context.Background(), nil, nil); err == nil {
		t.Error("client 為 nil 應回傳錯誤")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	defer withScheduleURL(t, srv.URL)()

	client := provider.NewBaseClient("www.twse.com.tw",
		provider.WithRateInterval(time.Microsecond), provider.WithJitterRatio(0))
	if err := New().LoadFromOfficial(context.Background(), client, nil); err == nil {
		t.Error("官方回應 5xx 應回傳錯誤")
	}
}

// withScheduleURL 暫以 srv.URL 覆寫官方行事曆端點（測試用），回傳還原函式。
func withScheduleURL(t *testing.T, url string) func() {
	t.Helper()
	orig := holidayScheduleURL
	holidayScheduleURL = url
	return func() { holidayScheduleURL = orig }
}

// scheduleServer 建立回傳官方 fixture 之 httptest server，回傳呼叫計數。
func newScheduleServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(scheduleFixture))
	}))
	return srv, &calls
}

// scheduleFixture 為 TWSE holidaySchedule?response=json 之真實回應
// （2026-01-01 抓取，115 年市場開休市日期；含「開始/最後交易日」標記行）。
const scheduleFixture = `{"stat":"ok","date":"20260101","title":"115 年市場開休市日期","fields":["日期","名稱","說明"],"data":[["2026-01-01","中華民國開國紀念日","依規定放假1日。"],["2026-01-02","國曆新年開始交易日","國曆新年開始交易。"],["2026-02-11","農曆春節前最後交易日","農曆春節前最後交易。"],["2026-02-12","市場無交易，僅辦理結算交割作業",""],["2026-02-13","市場無交易，僅辦理結算交割作業",""],["2026-02-15","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-16","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-17","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-18","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-19","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-20","農曆除夕及春節","依規定於2月15日至2月19日放假5日。2月15日適逢星期日，於2月20日（星期五）補假。"],["2026-02-23","農曆春節後開始交易日","農曆春節後開始交易。"],["2026-02-27","和平紀念日","和平紀念日為2月28日適逢星期六，於2月27日（星期五）補假。"],["2026-02-28","和平紀念日","依規定放假1日。"],["2026-04-03","兒童節及民族掃墓節","兒童節為4月4日適逢星期六，於4月3日（星期五）補假。"],["2026-04-04","兒童節及民族掃墓節","依規定放假1日。"],["2026-04-05","兒童節及民族掃墓節","依規定放假1日。"],["2026-04-06","兒童節及民族掃墓節","民族掃墓節為4月5日適逢星期日，於4月6日（星期一）補假。"],["2026-05-01","勞動節","依規定放假1日。"],["2026-06-19","端午節","依規定放假1日。"],["2026-09-25","中秋節","依規定放假1日。"],["2026-09-28","孔子誕辰紀念日/教師節","依規定放假1日。"],["2026-10-09","國慶日","國慶日為10月10日適逢星期六，於10月9日（星期五）補假。"],["2026-10-10","國慶日","依規定放假1日。"],["2026-10-25","臺灣光復暨金門古寧頭大捷紀念日","依規定放假1日。"],["2026-10-26","臺灣光復暨金門古寧頭大捷紀念日","臺灣光復暨金門古寧頭大捷紀念日為10月25日適逢星期日，於10月26日（星期一）補假。"],["2026-12-25","行憲紀念日","依規定放假1日。"]]}`
