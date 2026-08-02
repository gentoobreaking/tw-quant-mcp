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

// 七個實體來源（v2.1 §3）。每個來源各自獨立 token bucket（§5.3），
// 與 §4.4 請求級 Rate Limit 表格之主機 1:1 對應。
const (
	SourceTWSEOpenAPI    = "TWSE_OPENAPI"    // openapi.twse.com.tw
	SourceTWSEWebAPI     = "TWSE_WEB_API"    // www.twse.com.tw
	SourceTWSEMIS        = "TWSE_MIS"        // mis.twse.com.tw
	SourceTPExOpenAPI    = "TPEX_OPENAPI"    // www.tpex.org.tw
	SourceMOPS           = "MOPS"            // mops.twse.com.tw
	SourceTAIFEXOpenAPI  = "TAIFEX_OPENAPI"  // openapi.taifex.com.tw
	SourceTAIFEXDownload = "TAIFEX_DOWNLOAD" // www.taifex.com.tw
)

// hostSource 對映主機 → 來源（1:1，§4.4 表格之唯一真值）。
var hostSource = map[string]string{
	"openapi.twse.com.tw":   SourceTWSEOpenAPI,
	"www.twse.com.tw":       SourceTWSEWebAPI,
	"mis.twse.com.tw":       SourceTWSEMIS,
	"www.tpex.org.tw":       SourceTPExOpenAPI,
	"mops.twse.com.tw":      SourceMOPS,
	"openapi.taifex.com.tw": SourceTAIFEXOpenAPI,
	"www.taifex.com.tw":     SourceTAIFEXDownload,
}

// defaultRateInterval 對應 v1.3 §4.4 之請求級 Rate Limit 表格（唯一真值）。
// 限流數值已確認完全採 v1.3 保守值（2026-08-01），v2.1 §5.3 僅取其
// per-source token bucket 架構與環境變數可調設計。任何新來源加入前
// 必須先更新規格書 §4.4 再更新本表。
var defaultRateInterval = map[string]time.Duration{
	SourceTWSEOpenAPI:    1 * time.Second,
	SourceTWSEWebAPI:     2 * time.Second,
	SourceTWSEMIS:        8 * time.Second,
	SourceTPExOpenAPI:    1 * time.Second,
	SourceMOPS:           2 * time.Second,
	SourceTAIFEXOpenAPI:  1 * time.Second,
	SourceTAIFEXDownload: 5 * time.Second,
}

// defaultJitterRatio 對應 §4.4：MIS 明定 ±1s（8s 之 12.5%），其餘主機 ±20%。
// MIS 自 T025 起在 §4.4 預設節奏下改以 MIS_JITTER_MIN_MS/MAX_MS 絕對區間
// 疊加（見 envMISJitterWindow）；本表僅於 MIS 節奏被 WithRateInterval
// 覆寫時作為比例 jitter 後備。
var defaultJitterRatio = map[string]float64{
	"mis.twse.com.tw": 0.125,
}

const defaultJitter = 0.2

// MIS jitter 絕對區間（v2.1 §5.3：MIS_JITTER_MIN_MS / MIS_JITTER_MAX_MS，
// 預設 7000/9000，即 §8.1「每 8 秒 ± 1 秒隨機擾動」之絕對化）。
const (
	defaultMISJitterMinMS = 7000
	defaultMISJitterMaxMS = 9000
)

// DefaultRateInterval 回傳主機之預設請求間隔；未登錄之主機回傳 0。
func DefaultRateInterval(host string) time.Duration { return defaultRateInterval[hostSource[host]] }

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

// rateLimitEnabled 讀取 RATE_LIMIT_ENABLED（v2.1 §5.3，預設 true）；
// "false"/"0" 時停用限流（權杖無限、略過 jitter）。pkg/config 亦解析
// 同一變數（預設值/文件雙源同鍵，值必須一致）。
func rateLimitEnabled() bool {
	v := strings.TrimSpace(os.Getenv("RATE_LIMIT_ENABLED"))
	if v == "" {
		return true
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return true
	}
	return b
}

// envMISJitterWindow 讀取 MIS_JITTER_MIN_MS / MIS_JITTER_MAX_MS
// （v2.1 §5.3，預設 7000/9000）。任一值非法（非正整數）或 min ≥ max
// 時整組退回預設區間。
func envMISJitterWindow() (min, max time.Duration) {
	mi, ma := defaultMISJitterMinMS, defaultMISJitterMaxMS
	if v := os.Getenv("MIS_JITTER_MIN_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			mi = n
		}
	}
	if v := os.Getenv("MIS_JITTER_MAX_MS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ma = n
		}
	}
	if mi >= ma {
		mi, ma = defaultMISJitterMinMS, defaultMISJitterMaxMS
	}
	return time.Duration(mi) * time.Millisecond, time.Duration(ma) * time.Millisecond
}

// sleepFunc 為可注入之等待實作（測試用），ctx 取消時回傳 ctx.Err()。
type sleepFunc func(ctx context.Context, d time.Duration) error

// HostLimiter 是單一主機（= 單一來源，§5.3 per-source token bucket）之
// 請求級 Rate Limiter：以 x/time/rate 保證兩次請求間隔 ≥ interval（burst 1），
// 並於取得權杖後、請求**發送前**追加 jitter 等待（v1.2 已知錯誤：
// sleep 置於請求後，本實作不得復發）：
//   - MIS 於 §4.4 預設 8s 節奏下以 MIS_JITTER_MIN_MS/MAX_MS 絕對區間疊加
//     （v2.1 §5.3 區間可調；以 WithRateInterval 覆寫節奏時退回比例 jitter）；
//   - 其餘來源為 interval × ratio 之 ±ratio 抖動（預設 ±20%）。
type HostLimiter struct {
	source       string
	host         string
	interval     time.Duration
	jitterRatio  float64
	misJitterMin time.Duration // MIS 絕對 jitter 區間下限（§5.3）
	misJitterMax time.Duration // MIS 絕對 jitter 區間上限（§5.3）
	misWindow    bool          // MIS 是否套用絕對 jitter 區間（預設節奏下）
	disabled     bool          // RATE_LIMIT_ENABLED=false
	limiter      *rate.Limiter
	sleep        sleepFunc
	rng          *rand.Rand
}

// NewHostLimiter 建立主機之 Limiter。
// interval <= 0 時採用 §4.4 預設（可被環境變數覆寫）；再無登錄則 1s。
// jitterRatio <= 0 時採用主機預設（MIS 12.5%，其餘 20%）。
// RATE_LIMIT_ENABLED=false 時建立無作用 Limiter（§5.3）。
func NewHostLimiter(host string, interval time.Duration, jitterRatio float64) *HostLimiter {
	source := hostSource[host]

	explicitInterval := interval > 0 // 呼叫端（WithRateInterval）或環境變數覆寫節奏
	if d, ok := envRateInterval(host); ok {
		interval = d
		explicitInterval = true
	} else if interval <= 0 {
		interval = defaultRateInterval[source]
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

	// §5.3：每來源獨立 token bucket（burst=1，保守值不採用 v2.1 較寬鬆
	// 之 burst>1）。RATE_LIMIT_ENABLED=false 時以 rate.Inf 停用。
	lim := rate.NewLimiter(rate.Every(interval), 1)
	disabled := false
	if !rateLimitEnabled() {
		lim = rate.NewLimiter(rate.Inf, 1)
		disabled = true
	}

	// MIS 絕對 jitter 區間僅於 §4.4 預設 8s 節奏下套用（區間即「每 8 秒
	// ±1 秒」之絕對化，§5.3）；以 WithRateInterval / 環境變數覆寫節奏時
	// 節奏已由呼叫端接管，改用比例 jitter 後備。
	misWindow := source == SourceTWSEMIS && !explicitInterval
	min, max := envMISJitterWindow()

	return &HostLimiter{
		source:       source,
		host:         host,
		interval:     interval,
		jitterRatio:  jitterRatio,
		misJitterMin: min,
		misJitterMax: max,
		misWindow:    misWindow,
		disabled:     disabled,
		limiter:      lim,
		sleep:        sleepCtx,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Interval 回傳目前請求間隔（測試/debug 用）。
func (l *HostLimiter) Interval() time.Duration { return l.interval }

// Burst 回傳 token bucket 之 burst（§5.3 保守設計下恆為 1）。
func (l *HostLimiter) Burst() int { return l.limiter.Burst() }

// Source 回傳來源 ID（v2.1 §3；未登錄主機為空字串）。
func (l *HostLimiter) Source() string { return l.source }

// MISJitterWindow 回傳 MIS jitter 絕對區間（測試/debug 用；僅
// misWindow 為 true 時套用）。
func (l *HostLimiter) MISJitterWindow() (time.Duration, time.Duration) {
	return l.misJitterMin, l.misJitterMax
}

// SetSleepFunc 注入等待實作（僅測試用）。
func (l *HostLimiter) SetSleepFunc(fn sleepFunc) { l.sleep = fn }

// Wait 於請求前等待：先取得 per-source token bucket 權杖，再追加
// jitter 等待（Jitter 一律置於請求發出之前，§4.4/v1.2 修正）。
// RATE_LIMIT_ENABLED=false 時立即回傳。
func (l *HostLimiter) Wait(ctx context.Context) error {
	if l.disabled {
		return nil
	}
	if err := l.limiter.Wait(ctx); err != nil {
		return err
	}
	return l.jitter(ctx)
}

// jitter 回傳本次請求前之 jitter 等待量；MIS 絕對區間或 ±ratio 比例。
func (l *HostLimiter) jitter(ctx context.Context) error {
	if l.misWindow {
		j := l.misJitterMin + time.Duration(l.rng.Int63n(int64(l.misJitterMax-l.misJitterMin)+1))
		return l.sleep(ctx, j)
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
