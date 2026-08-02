package cache

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

// l1 是 §4.1 L1 記憶體快取：Ristretto（LFU / TinyLFU，存取 <1ms）。
type l1 struct {
	c *ristretto.Cache
}

// l1Defaults 為 L1 內建容量預設值（CACHE_L1_MAX_ENTRIES / CACHE_L1_MAX_MEMORY_MB
// 未設定或非正值時使用）。NumCounters 取 10 倍目標條目數（TinyLFU 計數器；
// 10k 條目 → 1e5 計數器，約 0.4MB）；MaxCost 以 maxMemoryMB 換算位元組。
const (
	l1DefaultMaxEntries  = 10000
	l1DefaultMaxMemoryMB = 256
)

// newL1 建立 Ristretto L1。maxEntries/maxMemoryMB <= 0 時沿用內建預設值
// （10000 條目 / 256MB）。
func newL1(maxEntries, maxMemoryMB int) (*l1, error) {
	if maxEntries <= 0 {
		maxEntries = l1DefaultMaxEntries
	}
	if maxMemoryMB <= 0 {
		maxMemoryMB = l1DefaultMaxMemoryMB
	}
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: int64(maxEntries) * 10,
		MaxCost:     int64(maxMemoryMB) * 1024 * 1024,
		BufferItems: 64,
	})
	if err != nil {
		return nil, err
	}
	return &l1{c: c}, nil
}

// get 回傳快取值與是否命中。
func (l *l1) get(key string) (any, bool) {
	return l.c.Get(key)
}

// set 寫入快取；ttl <= 0 表示不設到期（永久）。
// ristretto 之 Set 為非同步 buffered 寫入，故於寫入後 Wait() 確保後續 Get 可見
// （read-through 語義：GetOrFetch 回傳後，同鍵之後續查詢必須命中）。
func (l *l1) set(key string, v any, ttl time.Duration) {
	if ttl > 0 {
		l.c.SetWithTTL(key, v, 1, ttl)
	} else {
		l.c.Set(key, v, 1)
	}
	l.c.Wait()
}

// del 刪除快取鍵。
func (l *l1) del(key string) {
	l.c.Del(key)
}

// close 釋放 L1 資源（停止 Ristretto 背景 goroutine：processItems / policy / ticker）。
func (l *l1) close() {
	if l != nil && l.c != nil {
		l.c.Close()
	}
}
