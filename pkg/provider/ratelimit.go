package provider

import (
	"context"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// defaultRateInterval 對應規格書 §4.4 之請求級 Rate Limit 表格（唯一真值）。
// 任何新來源加入前必須先更新規格書 §4.4 再更新本表。
var defaultRateInterval = map[string]time.Duration{
	"mis.twse.com.tw":       8 * time.Second,
	"www.twse.com.tw":       2 * time.Second,
	"openapi.twse.com.tw":   1 * time.Second,
	"www.tpex.org.tw":       1 * time.Second,
	"mops.twse.com.tw":      2 * time.Second,
	"openapi.taifex.com.tw": 1 * time.Second,
	"www.taifex.com.tw":     5 * time.Second,
}

// defaultJitterRatio 對應 §4.4：MIS 明定 ±1s（8s 之 12.5%），其餘主機 ±20%。
var defaultJitterRatio = map[string]float64{
	"mis.twse.com.tw": 0.125,
}

const defaultJitter = 0.2

// DefaultRateInterval 回傳主機之預設請求間隔；未登錄之主機回傳 0。
func DefaultRateInterval(host string) time.Duration { return defaultRateInterval[host] }

// rateLimitEnvKey 回傳主機之環境變數覆寫鍵：
// `RATE_LIMIT_<HOST_SLUG>_EVERY`（主機名大寫、`.`/`-` 換底線），值為秒數（可小數）。
//
//	例：RATE_LIMIT_MIS_TWSE_COM_TW_EVERY=5.5
func rateLimitEnvKey(host string) string {
	slug := strings.ToUpper(strings.NewReplacer(".", "_", "-", "_").Replace(host))
	return "RATE_LIMIT_" + slug + "_EVERY"
}

// envRateInterval 讀取環境變數覆寫；未設定或非法值回傳 ok=false。
func envRateInterval(host string) (time.Duration, bool) {
	v := os.Getenv(rateLimitEnvKey(host))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 {
		return 0, false
	}
	return time.Duration(f * float64(time.Second)), true
}

// sleepFunc 為可注入之等待實作（測試用），ctx 取消時回傳 ctx.Err()。
type sleepFunc func(ctx context.Context, d time.Duration) error

// HostLimiter 是單一主機之請求級 Rate Limiter（§4.4）：
// 以 x/time/rate 保證兩次請求間隔 ≥ interval，並於取得權杖後、
// 請求**發送前**追加 ±jitterRatio 之隨機等待（v1.2 已知錯誤：
// sleep 置於請求後，本實作不得復發）。
type HostLimiter struct {
	host        string
	interval    time.Duration
	jitterRatio float64
	limiter     *rate.Limiter
	sleep       sleepFunc
	rng         *rand.Rand
}

// NewHostLimiter 建立主機之 Limiter。
// interval <= 0 時採用 §4.4 預設（可被環境變數覆寫）；再無登錄則 1s。
// jitterRatio <= 0 時採用主機預設（MIS 12.5%，其餘 20%）。
func NewHostLimiter(host string, interval time.Duration, jitterRatio float64) *HostLimiter {
	if d, ok := envRateInterval(host); ok {
		interval = d
	} else if interval <= 0 {
		interval = defaultRateInterval[host]
		if interval <= 0 {
			interval = time.Second
		}
	}
	if jitterRatio <= 0 {
		jitterRatio = defaultJitterRatio[host]
		if jitterRatio <= 0 {
			jitterRatio = defaultJitter
		}
	}
	return &HostLimiter{
		host:        host,
		interval:    interval,
		jitterRatio: jitterRatio,
		limiter:     rate.NewLimiter(rate.Every(interval), 1),
		sleep:       sleepCtx,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Interval 回傳目前請求間隔（測試/debug 用）。
func (l *HostLimiter) Interval() time.Duration { return l.interval }

// SetSleepFunc 注入等待實作（僅測試用）。
func (l *HostLimiter) SetSleepFunc(fn sleepFunc) { l.sleep = fn }

// Wait 於請求前等待：先取得 rate 權杖，再追加 jitter 等待。
// 保證兩次請求間隔 ≥ interval × (1 - jitterRatio)。
func (l *HostLimiter) Wait(ctx context.Context) error {
	if err := l.limiter.Wait(ctx); err != nil {
		return err
	}
	j := time.Duration(float64(l.interval) * l.jitterRatio * (2*l.rng.Float64() - 1))
	if j <= 0 {
		return nil
	}
	return l.sleep(ctx, j)
}

// sleepCtx 是預設等待實作；ctx 取消時回傳 ctx.Err()。
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
