package cache

import (
	"time"

	"github.com/dgraph-io/ristretto"
)

// l1 是 §4.1 L1 記憶體快取：Ristretto（LFU / TinyLFU，存取 <1ms）。
type l1 struct {
	c *ristretto.Cache
}

// newL1 建立 Ristretto L1。NumCounters 1e7 約佔 4MB 計數器；MaxCost 256MB。
func newL1() (*l1, error) {
	c, err := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,
		MaxCost:     1 << 28,
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
