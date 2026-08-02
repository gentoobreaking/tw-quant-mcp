package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/sync/singleflight"

	"tw-quant-mcp/pkg/model"
)

// ErrEmptyKey 表示以空字串快取鍵呼叫 GetOrFetch。
var ErrEmptyKey = errors.New("cache: 快取鍵不得為空")

// ErrServedStale 表示上游 fetch 失敗，已回退「過期但仍存在」之 L2 值
// （v2.1 §5.2 stale-if-error）。回傳值（含 fromCache=true）仍有效；
// 呼叫端以 errors.Is 辨識，並將 _lineage.freshness 標記為 STALE_FALLBACK。
var ErrServedStale = errors.New("cache: 上游失敗，回退過期 L2 值（STALE_FALLBACK）")

// l2WriteMinTTL 是 L2 寫入之最短 TTL：短 TTL（盤中 4s/30s/60s）資料無持久化價值，
// 且避免盤中磁碟 I/O（§4.2 備註：盤中 K 線查詢路徑不可進入 L2；§4.1 L2 僅收
// 盤後快照/歷史/行事曆/代碼表等長期資料）。
const l2WriteMinTTL = 10 * time.Minute

// Cache 是 §4.1 之三層快取引擎（L1 Ristretto / L2 SQLite / §12.2 Single-flight）。
// L2 為可選：未以 WithDataDir 設定資料目錄時僅運作 L1。
type Cache struct {
	l1 *l1
	l2 *l2
	sf singleflight.Group
}

// Option 調整 Cache 初始化行為。
type Option func(*config)

type config struct {
	dataDir       string
	l2Path        string
	l1MaxEntries  int
	l1MaxMemoryMB int
}

// WithDataDir 設定 L2 資料目錄（對應 config.DataDir / DATA_DIR），資料庫檔為 <dir>/cache.db。
func WithDataDir(dir string) Option {
	return func(c *config) { c.dataDir = dir }
}

// WithSQLitePath 直接指定 L2 SQLite 資料庫檔路徑（對應 config.L2SQLitePath /
// CACHE_L2_SQLITE_PATH）；與 WithDataDir 同時使用時以此為準。
func WithSQLitePath(path string) Option {
	return func(c *config) { c.l2Path = path }
}

// WithL1Config 設定 L1 容量（對應 CACHE_L1_MAX_ENTRIES / CACHE_L1_MAX_MEMORY_MB）；
// 未設定時沿用內建預設值。maxEntries <= 0 或 maxMemoryMB <= 0 時沿用預設。
func WithL1Config(maxEntries, maxMemoryMB int) Option {
	return func(c *config) { c.l1MaxEntries, c.l1MaxMemoryMB = maxEntries, maxMemoryMB }
}

// New 建立快取引擎；L2 開啟失敗時回傳錯誤（資料目錄不可寫入即視為致命）。
func New(opts ...Option) (*Cache, error) {
	cfg := config{}
	for _, o := range opts {
		o(&cfg)
	}
	c := &Cache{}
	var err error
	if c.l1, err = newL1(cfg.l1MaxEntries, cfg.l1MaxMemoryMB); err != nil {
		return nil, fmt.Errorf("cache: L1 初始化失敗: %w", err)
	}
	l2Path := cfg.l2Path
	if l2Path == "" && cfg.dataDir != "" {
		l2Path = filepath.Join(cfg.dataDir, "cache.db")
	}
	if l2Path != "" {
		if c.l2, err = openL2(l2Path); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Close 釋放資源：L2（若存在）與 L1 背景 goroutine（§12 效能原則：無 Leak）。
func (c *Cache) Close() error {
	if c.l1 != nil {
		c.l1.close()
	}
	if c.l2 != nil {
		return c.l2.close()
	}
	return nil
}

// FetchOption 調整 GetOrFetch 行為。
type FetchOption func(*fetchConfig)

type fetchConfig struct {
	dataset       string
	dataDate      string
	skipL2        bool
	staleFallback bool
}

// WithDataset 標註資料類別與資料日期（§4.2 資料類別欄），供 L2 資格判定與
// (dataset, date) 索引（§12.8）使用。
func WithDataset(dataset, dataDate string) FetchOption {
	return func(f *fetchConfig) {
		f.dataset, f.dataDate = dataset, dataDate
	}
}

// SkipL2 強制略過 L2（盤中即時路徑使用；§4.2 備註：盤中 K 線查詢路徑不可進入 L2）。
func SkipL2() FetchOption {
	return func(f *fetchConfig) { f.skipL2 = true }
}

// WithStaleFallback 啟用 v2.1 §5.2 stale-if-error：上游 fetch 失敗時，
// 回退「已過期但仍存在」之 L2 值並以 ErrServedStale 標記（連同有效回傳值）。
// 未啟用時，上游失敗直接回傳錯誤。
func WithStaleFallback() FetchOption {
	return func(f *fetchConfig) { f.staleFallback = true }
}

// allowL2 判定本請求是否可進入 L2：政策允許、非 SkipL2、且已設定資料目錄。
func (c *Cache) allowL2(cfg fetchConfig) bool {
	return !cfg.skipL2 && c.l2 != nil && cfg.dataset != "" && AllowL2(cfg.dataset)
}

// GetOrFetch 是 §12.2 之 Single-flight 讀穿介面，查詢路徑依序為：
//  1. L1（命中直接回傳，<1ms）；
//  2. L2（僅政策允許之資料類別，命中回傳並回填 L1）；
//  3. miss 時經 singleflight 合流：同鍵併發請求僅一個 goroutine 呼叫 fetch，
//     其餘等待共享結果；成功後回填 L1（及允許之 L2）。
//
// fromCache 為 true 表示資料來自快取（L1/L2），供 Handler 注入 _lineage
// is_cached / cache_ttl（§3.2）。ttl <= 0 表示永久（ForeverTTL）。
func GetOrFetch[T any](ctx context.Context, c *Cache, key string, ttl time.Duration,
	fetch func(context.Context) (T, error), opts ...FetchOption) (T, bool, error) {
	var zero T
	if key == "" {
		return zero, false, ErrEmptyKey
	}
	cfg := fetchConfig{}
	for _, o := range opts {
		o(&cfg)
	}

	if v, ok := l1Get[T](c, key); ok {
		return v, true, nil
	}
	if v, ok, err := l2Get[T](ctx, c, key, cfg); err != nil {
		return zero, false, err
	} else if ok {
		return v, true, nil
	}

	res, err, _ := c.sf.Do(key, func() (any, error) {
		// 併發視窗內可能已被其他 goroutine 回填（§12.2 同鍵僅一次上游呼叫）。
		if v, ok := l1Get[T](c, key); ok {
			return cacheHit[T]{value: v}, nil
		}
		if v, ok, err := l2Get[T](ctx, c, key, cfg); err != nil {
			return nil, err
		} else if ok {
			return cacheHit[T]{value: v}, nil
		}
		v, err := fetch(ctx)
		if err != nil {
			// §5.2 stale-if-error：上游失敗時回退過期 L2 值（需已存在且可反序列化）。
			if cfg.staleFallback && c.allowL2(cfg) {
				if stale, ok, serr := c.l2.get(ctx, key); serr == nil && ok && stale.expired {
					var sv T
					if uerr := json.Unmarshal(stale.value, &sv); uerr == nil {
						return cacheHit[T]{value: sv, cached: true, stale: true}, nil
					}
				}
			}
			return nil, err
		}
		c.store(key, v, ttl, cfg)
		return cacheHit[T]{value: v}, nil
	})
	if err != nil {
		return zero, false, err
	}
	hit := res.(cacheHit[T])
	if hit.stale {
		return hit.value, true, ErrServedStale
	}
	return hit.value, hit.cached, nil
}

// Get 執行唯讀快取查詢（L1 → L2），不觸發上游 fetch（§12.2 讀穿之唯讀端）。
// 用於批次查詢前之快取探測（如 TAIFEX FetchRange 先探測哪些日期已在 L2）。
func Get[T any](ctx context.Context, c *Cache, key string, opts ...FetchOption) (T, bool, error) {
	var zero T
	if key == "" {
		return zero, false, ErrEmptyKey
	}
	cfg := fetchConfig{}
	for _, o := range opts {
		o(&cfg)
	}
	if v, ok := l1Get[T](c, key); ok {
		return v, true, nil
	}
	return l2Get[T](ctx, c, key, cfg)
}

// L2Count 回傳指定資料類別於 L2 之列數（含過期列）。用於觀測與守門測試
// （如確認 RingBuffer 即時資料不落入快取，§5.1）。未啟用 L2 時回傳 0。
func (c *Cache) L2Count(ctx context.Context, dataset string) (int, error) {
	if c.l2 == nil {
		return 0, nil
	}
	return c.l2.count(ctx, dataset)
}

// cacheHit 承載 singleflight 之結果與來源（避免以 error 通道回傳標記）。
type cacheHit[T any] struct {
	value  T
	cached bool
	stale  bool // §5.2：stale-if-error 回退之過期 L2 值
}

// l1Get 讀取 L1；型別不符視為失效並清除（同鍵跨型別誤用之防護）。
func l1Get[T any](c *Cache, key string) (T, bool) {
	var zero T
	raw, ok := c.l1.get(key)
	if !ok {
		return zero, false
	}
	if v, ok := raw.(T); ok {
		return v, true
	}
	c.l1.del(key)
	return zero, false
}

// l2Get 讀取 L2（僅 policy 允許之資料類別）；命中時回填 L1 並回傳剩餘 TTL。
// 過期項目視為 miss（保留於 L2 供 §5.2 stale-if-error 回退）。
func l2Get[T any](ctx context.Context, c *Cache, key string, cfg fetchConfig) (T, bool, error) {
	var zero T
	if !c.allowL2(cfg) {
		return zero, false, nil
	}
	e, ok, err := c.l2.get(ctx, key)
	if err != nil || !ok || e.expired {
		return zero, false, err
	}
	var v T
	if err := json.Unmarshal(e.value, &v); err != nil {
		return zero, false, fmt.Errorf("cache: L2 反序列化失敗: %w", err)
	}
	c.l1.set(key, v, ttlFromExpires(e.expiresAt))
	return v, true, nil
}

// store 回填 L1，並於政策允許且 TTL 夠長時回填 L2（L2 為 best-effort，失敗不影響回應）。
func (c *Cache) store(key string, v any, ttl time.Duration, cfg fetchConfig) {
	c.l1.set(key, v, ttl)
	if c.allowL2(cfg) && (ttl <= 0 || ttl >= l2WriteMinTTL) {
		raw, err := json.Marshal(v)
		if err == nil {
			_ = c.l2.set(context.Background(), key, cfg.dataset, cfg.dataDate, raw, ttl)
		}
	}
}

// ttlFromExpires 將 L2 之到期時間轉為剩餘 TTL；zero 表示永久。
func ttlFromExpires(expiresAt time.Time) time.Duration {
	if expiresAt.IsZero() {
		return 0
	}
	return time.Until(expiresAt)
}

// MarkCacheHit 將快取命中資訊注入 Envelope 之 _lineage（v2.1 §4）：
// is_cached=true、cache_ttl=<秒>（cache_ttl 為內部欄位，僅 debug/log 模式
// 輸出）。各 Handler 於 GetOrFetch 回傳 fromCache=true 時呼叫。
func MarkCacheHit(env *model.Envelope, ttl time.Duration) {
	env.Lineage.IsCached = true
	env.Lineage.CacheTTL = int(ttl.Seconds())
}
