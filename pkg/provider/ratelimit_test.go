package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// TestHostLimiterDefaultInterval 驗證 §4.4 預設節奏與 jitter 後備比例。
func TestHostLimiterDefaultInterval(t *testing.T) {
	l := NewHostLimiter("mis.twse.com.tw", 0, 0)
	if got := l.Interval(); got != 8*time.Second {
		t.Errorf("MIS 間隔 = %v, want 8s", got)
	}
	if l.jitterRatio != 0.125 {
		t.Errorf("MIS jitter 比例 = %v, want 0.125（§4.4 ±1s/8s）", l.jitterRatio)
	}
	if min, max := l.MISJitterWindow(); min != 7*time.Second || max != 9*time.Second {
		t.Errorf("MIS jitter 區間 = [%v, %v], want [7s, 9s]（§5.3 預設）", min, max)
	}

	l2 := NewHostLimiter("www.twse.com.tw", 0, 0)
	if got := l2.Interval(); got != 2*time.Second {
		t.Errorf("TWSE-WEB 間隔 = %v, want 2s", got)
	}
	if l2.jitterRatio != 0.2 {
		t.Errorf("TWSE-WEB jitter 比例 = %v, want 0.2", l2.jitterRatio)
	}
}

func TestHostLimiterUnknownHostDefaults(t *testing.T) {
	l := NewHostLimiter("unknown.example.com", 0, 0)
	if got := l.Interval(); got != time.Second {
		t.Errorf("未登錄主機間隔 = %v, want 1s", got)
	}
	if l.Source() != "" {
		t.Errorf("未登錄主機 Source 應為空，實際 %q", l.Source())
	}
}

// TestPerSourceRateLimits 驗證七個來源（v2.1 §3）各自獨立 token bucket，
// 數值完全採 v1.3 §4.4 保守值（§5.3 架構、不採較寬鬆之 burst>1）。
func TestPerSourceRateLimits(t *testing.T) {
	cases := []struct {
		host   string
		source string
		want   time.Duration
	}{
		{"openapi.twse.com.tw", SourceTWSEOpenAPI, 1 * time.Second},
		{"www.twse.com.tw", SourceTWSEWebAPI, 2 * time.Second},
		{"mis.twse.com.tw", SourceTWSEMIS, 8 * time.Second},
		{"www.tpex.org.tw", SourceTPExOpenAPI, 1 * time.Second},
		{"mops.twse.com.tw", SourceMOPS, 2 * time.Second},
		{"openapi.taifex.com.tw", SourceTAIFEXOpenAPI, 1 * time.Second},
		{"www.taifex.com.tw", SourceTAIFEXDownload, 5 * time.Second},
	}
	for _, tc := range cases {
		l := NewHostLimiter(tc.host, 0, 0)
		if l.Source() != tc.source {
			t.Errorf("%s Source = %q, want %q", tc.host, l.Source(), tc.source)
		}
		if got := l.Interval(); got != tc.want {
			t.Errorf("%s 間隔 = %v, want %v", tc.host, got, tc.want)
		}
		if b := l.Burst(); b != 1 {
			t.Errorf("%s burst = %d, want 1（保守值，不採 v2.1 burst>1）", tc.host, b)
		}
	}
}

// TestJitterRanges 驗證比例 jitter 等待量於 (0, interval×ratio] 範圍內。
func TestJitterRanges(t *testing.T) {
	const interval = 20 * time.Millisecond
	l := NewHostLimiter("test.host", interval, 0.5)

	var jitters []time.Duration
	l.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		jitters = append(jitters, d)
		return nil
	})

	for i := 0; i < 6; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait 失敗: %v", err)
		}
	}
	if len(jitters) == 0 {
		t.Fatal("jitter 等待從未被呼叫")
	}
	maxAllowed := interval / 2
	for _, j := range jitters {
		if j <= 0 || j > maxAllowed {
			t.Errorf("jitter = %v，應於 (0, %v] 內", j, maxAllowed)
		}
	}
}

// TestMISJitterWindowRange 驗證 MIS 絕對 jitter 區間（§5.3 預設
// MIS_JITTER_MIN_MS=7000 / MAX_MS=9000，即「每 8 秒 ±1 秒」）。
func TestMISJitterWindowRange(t *testing.T) {
	l := NewHostLimiter("mis.twse.com.tw", 0, 0)
	var got []time.Duration
	l.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		return nil
	})
	for i := 0; i < 20; i++ {
		if err := l.jitter(context.Background()); err != nil {
			t.Fatalf("jitter 失敗: %v", err)
		}
	}
	if len(got) == 0 {
		t.Fatal("MIS jitter 等待從未被呼叫")
	}
	for _, d := range got {
		if d < 7*time.Second || d > 9*time.Second {
			t.Errorf("MIS jitter = %v，應於 [7s, 9s] 內（§5.3 區間）", d)
		}
	}
}

// TestMISJitterEnvOverride 驗證 MIS_JITTER_MIN_MS / MIS_JITTER_MAX_MS 覆寫；
// 非法區間（min ≥ max）整組退回預設。
func TestMISJitterEnvOverride(t *testing.T) {
	t.Setenv("MIS_JITTER_MIN_MS", "3000")
	t.Setenv("MIS_JITTER_MAX_MS", "4000")
	l := NewHostLimiter("mis.twse.com.tw", 0, 0)
	if min, max := l.MISJitterWindow(); min != 3*time.Second || max != 4*time.Second {
		t.Errorf("覆寫後區間 = [%v, %v], want [3s, 4s]", min, max)
	}

	t.Setenv("MIS_JITTER_MIN_MS", "5000")
	t.Setenv("MIS_JITTER_MAX_MS", "2000")
	l2 := NewHostLimiter("mis.twse.com.tw", 0, 0)
	if min, max := l2.MISJitterWindow(); min != 7*time.Second || max != 9*time.Second {
		t.Errorf("非法區間應退回預設 [7s, 9s]，實際 [%v, %v]", min, max)
	}
}

// TestMISJitterFallbackWhenIntervalOverridden 驗證：MIS 節奏被
// WithRateInterval 覆寫時，jitter 退回 interval × ratio 之比例式
// （絕對區間僅於 §4.4 預設 8s 節奏下套用）。
func TestMISJitterFallbackWhenIntervalOverridden(t *testing.T) {
	l := NewHostLimiter("mis.twse.com.tw", 100*time.Millisecond, 0.5)
	if l.misWindow {
		t.Error("節奏覆寫時 misWindow 應為 false")
	}
	var got []time.Duration
	l.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		got = append(got, d)
		return nil
	})
	for i := 0; i < 6; i++ {
		if err := l.jitter(context.Background()); err != nil {
			t.Fatalf("jitter 失敗: %v", err)
		}
	}
	for _, d := range got {
		if d <= 0 || d > 50*time.Millisecond {
			t.Errorf("比例 jitter = %v，應於 (0, 50ms] 內（100ms×0.5）", d)
		}
	}
}

// TestRateLimitIntervalEnvOverride 驗證 RATE_LIMIT_<HOST>_EVERY 覆寫。
func TestRateLimitIntervalEnvOverride(t *testing.T) {
	t.Setenv("RATE_LIMIT_WWW_TWSE_COM_TW_EVERY", "0.5")
	l := NewHostLimiter("www.twse.com.tw", 0, 0)
	if got := l.Interval(); got != 500*time.Millisecond {
		t.Errorf("環境覆寫後間隔 = %v, want 500ms", got)
	}
}

// TestRateLimitDisabled 驗證 RATE_LIMIT_ENABLED=false：Wait 立即回傳，
// 不執行 token bucket 與 jitter（§5.3）。
func TestRateLimitDisabled(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "false")
	l := NewHostLimiter("www.twse.com.tw", 0, 0)
	slept := false
	l.SetSleepFunc(func(_ context.Context, _ time.Duration) error {
		slept = true
		return nil
	})
	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait 失敗: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("停用限流後 Wait 應立即回傳，實際耗時 %v", elapsed)
	}
	if slept {
		t.Error("停用限流後不得執行 jitter 等待")
	}
}

// TestWaitSequentialTiming 驗證 Wait 保證間隔 ≥ interval（token bucket 層）。
// 首個 Wait 持有初始權杖可立即回傳，故間隔自第 2 個起檢查。
// 注入 no-op sleep 以排除 jitter 實際等待（避免時序抖動造成 flaky）。
func TestWaitSequentialTiming(t *testing.T) {
	const interval = 30 * time.Millisecond
	l := NewHostLimiter("test.host", interval, 0)
	l.SetSleepFunc(func(_ context.Context, _ time.Duration) error { return nil })

	var starts []time.Time
	for i := 0; i < 4; i++ {
		start := time.Now()
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait 失敗: %v", err)
		}
		starts = append(starts, start)
	}
	for i := 2; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < interval-5*time.Millisecond {
			t.Errorf("間隔 %v 應 ≥ %v", gap, interval)
		}
	}
}

// TestJitterBeforeRequest 驗證「Jitter 一律置於請求發出之前」：
// 每筆請求抵達伺服器時，jitter 等待必已完成（v1.2 sleep-after 為已知
// 錯誤；v2.1 §8.2 範例亦為錯誤寫法，本實作以請求前為準，§4.4）。
// 比例 jitter 為 ±ratio 隨機（負值略過），故以多輪取樣確保至少一輪
// 實際觸發 jitter，且任何觸發輪之順序皆須為 jitter → request。
func TestJitterBeforeRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	var mu sync.Mutex
	var orders [][]string
	var fires int
	c := NewBaseClient("test.host",
		WithRateInterval(time.Millisecond), WithJitterRatio(1))
	c.limiter.SetSleepFunc(func(_ context.Context, _ time.Duration) error {
		mu.Lock()
		defer mu.Unlock()
		fires++
		orders[len(orders)-1] = append(orders[len(orders)-1], "jitter")
		return nil
	})

	for i := 0; i < 16; i++ {
		mu.Lock()
		orders = append(orders, nil)
		mu.Unlock()
		if _, err := c.Do(context.Background(), RawRequest{URL: srv.URL}); err != nil {
			t.Fatalf("Do 失敗: %v", err)
		}
		mu.Lock()
		orders[len(orders)-1] = append(orders[len(orders)-1], "request")
		mu.Unlock()
	}

	mu.Lock()
	defer mu.Unlock()
	if fires == 0 {
		t.Fatal("測試樣本未觸發 jitter，無法驗證時序")
	}
	for i, o := range orders {
		if len(o) == 2 && (o[0] != "jitter" || o[1] != "request") {
			t.Errorf("第 %d 輪：jitter 應發生於請求之前，實際順序 %v", i+1, o)
		}
	}
}
