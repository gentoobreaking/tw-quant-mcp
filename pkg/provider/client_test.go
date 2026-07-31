package provider

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fastClient 建立間隔極短之 client（測試專用，避免 §4.4 預設 1s 拖慢測試）。
func fastClient() *BaseClient {
	return NewBaseClient("test.host", WithRateInterval(time.Microsecond))
}

// jsonServer 回傳固定 JSON。
func jsonServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	}))
}

func TestBaseClientGetOK(t *testing.T) {
	srv := jsonServer()
	defer srv.Close()

	c := fastClient()
	raw, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do 失敗: %v", err)
	}
	if raw.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", raw.StatusCode)
	}
	if got := string(raw.Body); got != `{"ok":true}` {
		t.Errorf("Body = %q", got)
	}
	wantHash := sha256.Sum256([]byte(`{"ok":true}`))
	if raw.BodyHash != hex.EncodeToString(wantHash[:]) {
		t.Errorf("BodyHash 不符: got %q", raw.BodyHash)
	}
	if raw.FetchedAt.IsZero() {
		t.Error("FetchedAt 不應為零值")
	}
	if raw.SourceURL != srv.URL {
		t.Errorf("SourceURL = %q, want %q", raw.SourceURL, srv.URL)
	}
	if _, off := raw.FetchedAt.Zone(); off != 8*3600 {
		t.Errorf("FetchedAt 應為 +08:00，實際 %d", off)
	}
}

func TestUserAgentInjected(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.UserAgent()
		fmt.Fprint(w, "{}")
	}))
	defer srv.Close()

	c := NewBaseClient("test.host", WithRateInterval(time.Microsecond), WithUserAgent("tw-quant-mcp-test/1.0"))
	if _, err := c.Do(context.Background(), RawRequest{URL: srv.URL}); err != nil {
		t.Fatalf("Do 失敗: %v", err)
	}
	if ua := <-got; ua != "tw-quant-mcp-test/1.0" {
		t.Errorf("User-Agent = %q，與設定不符", ua)
	}
}

func TestGzipAutoDecompress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			t.Error("應送出 Accept-Encoding: gzip")
		}
		w.Header().Set("Content-Encoding", "gzip")
		body, _ := gzipBytes(`{"gzip":true}`)
		w.Write(body)
	}))
	defer srv.Close()

	c := fastClient()
	raw, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do 失敗: %v", err)
	}
	if got := string(raw.Body); got != `{"gzip":true}` {
		t.Errorf("gzip 未自動解壓: %q", got)
	}
}

func TestCookieSessionMaintained(t *testing.T) {
	reqCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		if ck, err := r.Cookie("session"); err == nil && ck.Value == "abc123" {
			w.Header().Set("X-Session-Seen", "yes")
		}
		if reqCount == 1 {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
		}
		fmt.Fprint(w, "{}")
	}))
	defer srv.Close()

	c := fastClient()
	raw1, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do 1 失敗: %v", err)
	}
	if raw1.Header.Get("X-Session-Seen") == "yes" {
		t.Error("第一次請求不應帶 cookie")
	}
	raw2, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do 2 失敗: %v", err)
	}
	if raw2.Header.Get("X-Session-Seen") != "yes" {
		t.Error("第二次請求應帶回 session cookie")
	}
}

func TestRateLimitGap(t *testing.T) {
	srv := jsonServer()
	defer srv.Close()

	const interval = 50 * time.Millisecond
	c := NewBaseClient("test.host", WithRateInterval(interval), WithJitterRatio(0.2))

	var starts []time.Time
	for i := 0; i < 5; i++ {
		start := time.Now()
		if _, err := c.Do(context.Background(), RawRequest{URL: srv.URL}); err != nil {
			t.Fatalf("Do %d 失敗: %v", i, err)
		}
		starts = append(starts, start)
	}
	// 最小間隔保證 = interval × (1 - jitterRatio)，扣除計時誤差。
	// 首個請求持有初始權杖可立即發出，故從第 2 個間隔起檢查。
	minGap := time.Hour
	for i := 2; i < len(starts); i++ {
		gap := starts[i].Sub(starts[i-1])
		if gap < minGap {
			minGap = gap
		}
	}
	wantMin := interval * 8 / 10 // 0.8 × interval
	if minGap < wantMin-10*time.Millisecond {
		t.Errorf("請求間隔最小 %v，應 ≥ %v（含誤差）", minGap, wantMin)
	}
}

func TestRetry429ThenSuccess(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits <= 2 {
			http.Error(w, "too many", http.StatusTooManyRequests)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	var sleeps []time.Duration
	c := fastClient()
	c.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	raw, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do 應在重試後成功: %v", err)
	}
	if raw.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", raw.StatusCode)
	}
	if hits != 3 {
		t.Errorf("伺服器應被請求 3 次，實際 %d", hits)
	}
	want := []time.Duration{time.Second, 2 * time.Second}
	if len(sleeps) != 2 || sleeps[0] != want[0] || sleeps[1] != want[1] {
		t.Errorf("退避時序 = %v, want %v", sleeps, want)
	}
}

func TestRetry403(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	c := fastClient()
	c.sleep = func(_ context.Context, _ time.Duration) error { return nil }

	if _, err := c.Do(context.Background(), RawRequest{URL: srv.URL}); err != nil {
		t.Fatalf("403 應退避重試後成功: %v", err)
	}
	if hits != 2 {
		t.Errorf("伺服器應被請求 2 次，實際 %d", hits)
	}
}

func TestBackoffSequenceAndCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "too many", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var sleeps []time.Duration
	c := NewBaseClient("test.host", WithRateInterval(time.Microsecond), WithMaxRetries(7), WithBackoffCap(30*time.Second))
	c.sleep = func(_ context.Context, d time.Duration) error {
		sleeps = append(sleeps, d)
		return nil
	}

	_, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("持續 429 應回報錯誤")
	}
	want := []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second,
		8 * time.Second, 16 * time.Second, 30 * time.Second, 30 * time.Second}
	if len(sleeps) != len(want) {
		t.Fatalf("退避序列長度 = %d, want %d（序列 %v）", len(sleeps), len(want), sleeps)
	}
	for i := range want {
		if sleeps[i] != want[i] {
			t.Errorf("退避[%d] = %v, want %v", i, sleeps[i], want[i])
		}
	}
}

func TestUnexpectedStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := fastClient()
	_, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("502 應回報錯誤")
	}
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("錯誤應為 *HTTPStatusError，實際 %T: %v", err, err)
	}
	if statusErr.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want 502", statusErr.StatusCode)
	}
}

func TestCircuitBreakerOpenAfter5Failures(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := fastClient()

	for i := 0; i < 5; i++ {
		if _, err := c.Do(context.Background(), RawRequest{URL: srv.URL}); err == nil {
			t.Fatalf("第 %d 次應失敗", i+1)
		}
	}
	if hits != 5 {
		t.Fatalf("5 次失敗後伺服器 hit 數應為 5，實際 %d", hits)
	}

	// 第 6 次：熔斷開啟，快速失敗（不打 HTTP）
	_, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("熔斷開啟期間應回 ErrCircuitOpen，實際 %v", err)
	}
	if hits != 5 {
		t.Errorf("熔斷期間不應打 HTTP，hit 數 = %d", hits)
	}

	// 60s 後恢復
	c.breaker.SetNowFn(func() time.Time { return time.Now().Add(61 * time.Second) })
	_, err = c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("恢復後 500 仍應失敗")
	}
	if hits != 6 {
		t.Errorf("恢復後應再打 HTTP，hit 數 = %d", hits)
	}
}

func TestCircuitBreakerSuccessResets(t *testing.T) {
	status := http.StatusInternalServerError
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status == http.StatusOK {
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		http.Error(w, "boom", status)
	}))
	defer srv.Close()

	c := fastClient()

	fail := func() error {
		_, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
		return err
	}

	// 4 次失敗 + 1 次成功（重設計數）
	for i := 0; i < 4; i++ {
		if err := fail(); err == nil {
			t.Fatal("應失敗")
		}
	}
	status = http.StatusOK
	if err := fail(); err != nil {
		t.Fatalf("成功請求不應失敗: %v", err)
	}

	// 再 4 次失敗 → 尚未熔斷
	status = http.StatusInternalServerError
	for i := 0; i < 4; i++ {
		if err := fail(); err == nil {
			t.Fatal("應失敗")
		}
	}
	if err := c.breaker.Allow(); err != nil {
		t.Fatalf("4 次失敗（成功已重置）不應熔斷: %v", err)
	}

	// 第 5 次失敗 → 熔斷
	if err := fail(); err == nil {
		t.Fatal("應失敗")
	}
	if err := c.breaker.Allow(); !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("5 次連續失敗後應熔斷: %v", err)
	}
}

func TestClientTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		fmt.Fprint(w, "{}")
	}))
	defer srv.Close()

	c := NewBaseClient("test.host", WithRateInterval(time.Microsecond), WithTimeout(50*time.Millisecond))
	start := time.Now()
	_, err := c.Do(context.Background(), RawRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("超過 timeout 應回報錯誤")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Errorf("timeout 應於 ~50ms 生效，實際 %v", elapsed)
	}
}

func TestRateLimitEnvOverride(t *testing.T) {
	t.Setenv("RATE_LIMIT_TEST_HOST_EVERY", "0.05")
	c := NewBaseClient("test.host")
	if got := c.RateInterval(); got != 50*time.Millisecond {
		t.Errorf("環境變數覆寫後間隔 = %v, want 50ms", got)
	}
}

func TestDefaultRateIntervalPerHost(t *testing.T) {
	tests := []struct {
		host string
		want time.Duration
	}{
		{"mis.twse.com.tw", 8 * time.Second},
		{"www.twse.com.tw", 2 * time.Second},
		{"openapi.twse.com.tw", time.Second},
		{"www.tpex.org.tw", time.Second},
		{"mops.twse.com.tw", 2 * time.Second},
		{"openapi.taifex.com.tw", time.Second},
		{"www.taifex.com.tw", 5 * time.Second},
	}
	for _, tt := range tests {
		if got := NewBaseClient(tt.host).RateInterval(); got != tt.want {
			t.Errorf("%s 間隔 = %v, want %v", tt.host, got, tt.want)
		}
	}
}

func TestInvalidEnvValueFallsBackToDefault(t *testing.T) {
	t.Setenv("RATE_LIMIT_WWW_TWSE_COM_TW_EVERY", "abc")
	c := NewBaseClient("www.twse.com.tw")
	if got := c.RateInterval(); got != 2*time.Second {
		t.Errorf("非法環境變數應 fallback 預設 2s，實際 %v", got)
	}
}

func gzipBytes(s string) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(s)); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
