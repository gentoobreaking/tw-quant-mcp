package provider

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// 預設瀏覽器樣式 User-Agent（§8.3：Session 預熱需維持瀏覽器樣式）。
const defaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"

const (
	defaultTimeout     = 30 * time.Second
	defaultBackoffCap  = 30 * time.Second // §4.4：退避上限
	defaultMaxRetries  = 8                // 退避序列 1,2,4,8,16,30,30,30
	firstBackoff       = 1 * time.Second  // §4.4：指數退避起點
	keepAlive          = 30 * time.Second
	idleConnTimeout    = 90 * time.Second
	tlsHandshake       = 10 * time.Second
	responseHeaderWait = 15 * time.Second
)

// HTTPStatusError 表示收到非預期之 HTTP 狀態（非 2xx，且非 403/429）。
type HTTPStatusError struct {
	StatusCode int
	Body       []byte
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("provider: 非預期 HTTP 狀態 %d", e.StatusCode)
}

// BaseClient 是 Resilient HTTP Client 基類（§6、§12.3）：
// 每主機獨立連線池 + Cookie Session 維持 + 請求級 Rate Limit（含 Jitter）+
// 403/429 指數退避 + 熔斷。所有官方來源 Adapter 皆以之為底層。
type BaseClient struct {
	host         string
	http         *http.Client
	limiter      *HostLimiter
	breaker      *CircuitBreaker
	userAgent    string
	timeout      time.Duration
	backoffCap   time.Duration
	maxRetries   int
	rateInterval time.Duration
	jitterRatio  float64
	sleep        sleepFunc
	logger       *slog.Logger
}

// Option 為 BaseClient 建置選項。
type Option func(*BaseClient)

// WithTimeout 設定單一請求整體 timeout（TAIFEX-DL 大 CSV 建議 60s，§T003 備註）。
func WithTimeout(d time.Duration) Option {
	return func(c *BaseClient) { c.timeout = d }
}

// WithUserAgent 覆寫預設 User-Agent。
func WithUserAgent(ua string) Option {
	return func(c *BaseClient) { c.userAgent = ua }
}

// WithRateInterval 覆寫主機之請求間隔（環境變數 RATE_LIMIT_*_EVERY 優先）。
func WithRateInterval(d time.Duration) Option {
	return func(c *BaseClient) { c.rateInterval = d }
}

// WithJitterRatio 覆寫 jitter 比例（預設 MIS 12.5%、其餘 20%）。
func WithJitterRatio(r float64) Option {
	return func(c *BaseClient) { c.jitterRatio = r }
}

// WithMaxRetries 覆寫 403/429 重試次數。
func WithMaxRetries(n int) Option {
	return func(c *BaseClient) { c.maxRetries = n }
}

// WithBackoffCap 覆寫退避上限。
func WithBackoffCap(d time.Duration) Option {
	return func(c *BaseClient) { c.backoffCap = d }
}

// WithLogger 注入 slog logger（預設 discard）。
func WithLogger(l *slog.Logger) Option {
	return func(c *BaseClient) { c.logger = l }
}

// WithBreakerNow 注入熔斷器時鐘（僅測試用）。
func WithBreakerNow(fn func() time.Time) Option {
	return func(c *BaseClient) { c.breaker.SetNowFn(fn) }
}

// NewBaseClient 建立主機之 Resilient Client。
// host 需為登錄於 §4.4 之主機（如 "www.twse.com.tw"），
// rate interval 未指定時採用 §4.4 預設（可被 RATE_LIMIT_*_EVERY 環境變數覆寫）。
func NewBaseClient(host string, opts ...Option) *BaseClient {
	c := &BaseClient{
		host:       host,
		userAgent:  defaultUserAgent,
		timeout:    defaultTimeout,
		backoffCap: defaultBackoffCap,
		maxRetries: defaultMaxRetries,
		sleep:      sleepCtx,
		logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	c.breaker = NewCircuitBreaker()
	for _, o := range opts {
		o(c)
	}

	interval := c.rateInterval
	if d, ok := envRateInterval(host); ok {
		interval = d
	}
	c.limiter = NewHostLimiter(host, interval, c.jitterRatio)
	c.http = &http.Client{
		// 每主機獨立 Transport（§12.3）：Keep-Alive、MaxIdleConnsPerHost=8、HTTP/2、gzip 自動解壓
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: keepAlive}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          32,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       idleConnTimeout,
			TLSHandshakeTimeout:   tlsHandshake,
			ExpectContinueTimeout: time.Second,
			ResponseHeaderTimeout: responseHeaderWait,
		},
		// Cookie Session 維持（§8.3：MIS 需 session cookie；cookiejar 自動儲存/回送）
		Jar:     mustCookieJar(),
		Timeout: c.timeout,
	}
	return c
}

// mustCookieJar 建立 cookiejar；nil PublicSuffixList 表示不檢查公眾後綴
// （官方主機域明確，無需 x/net/publicsuffix 依賴）。
func mustCookieJar() http.CookieJar {
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(fmt.Sprintf("provider: cookiejar.New 失敗: %v", err))
	}
	return jar
}

// Host 回傳主機名。
func (c *BaseClient) Host() string { return c.host }

// RateInterval 回傳目前請求間隔（測試/debug 用）。
func (c *BaseClient) RateInterval() time.Duration { return c.limiter.Interval() }

// Do 執行完整防護管線（§4.4）：
// 熔斷檢查 → Rate Limit 等待（+Jitter，請求前）→ HTTP 請求 →
// 403/429 指數退避重試（1s→2s→4s…上限 30s）→ 結果記錄至熔斷器。
func (c *BaseClient) Do(ctx context.Context, req RawRequest) (*RawResponse, error) {
	if err := c.breaker.Allow(); err != nil {
		return nil, err
	}
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	backoff := firstBackoff
	for attempt := 0; ; attempt++ {
		resp, err := c.doOnce(ctx, method, req.URL, req.Headers, req.Body)
		if err != nil {
			c.breaker.Record(false)
			return nil, err
		}
		if resp.StatusCode < 400 {
			c.breaker.Record(true)
			return resp, nil
		}

		// 403/429 → 指數退避重試（§4.4）
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			if attempt >= c.maxRetries {
				c.breaker.Record(false)
				return nil, fmt.Errorf("provider: %s 連續 %d 次收到 %d: %w",
					c.host, attempt+1, resp.StatusCode, &HTTPStatusError{StatusCode: resp.StatusCode})
			}
			c.logger.Debug("收到 403/429，指數退避後重試", "host", c.host,
				"status", resp.StatusCode, "attempt", attempt+1, "backoff", backoff)
			if err := c.sleep(ctx, backoff); err != nil {
				return nil, err
			}
			backoff = min(backoff*2, c.backoffCap)
			continue
		}

		// 其他非預期狀態（5xx 等）
		c.breaker.Record(false)
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Body: resp.Body}
	}
}

func (c *BaseClient) doOnce(ctx context.Context, method, url string, headers http.Header, body []byte) (*RawResponse, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", c.userAgent)
	httpReq.Header.Set("Accept", "application/json, text/plain, */*")
	// 不手動設定 Accept-Encoding：由 http.Transport 自動送 gzip 並解壓縮
	for k, vs := range headers {
		for _, v := range vs {
			httpReq.Header.Set(k, v)
		}
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	return newRawResponse(resp, url)
}
