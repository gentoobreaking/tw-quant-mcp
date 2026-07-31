package cache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

func newTestCache(t *testing.T, opts ...Option) *Cache {
	t.Helper()
	c, err := New(opts...)
	if err != nil {
		t.Fatalf("New 失敗: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// 命中/未命中：首次 miss 呼叫 fetch，第二次命中不呼叫 fetch。
func TestGetOrFetchHitMiss(t *testing.T) {
	c := newTestCache(t)
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "tw", nil
	}

	v, fromCache, err := GetOrFetch(ctx(t), c, "k1", time.Minute, fetch)
	if err != nil || v != "tw" || fromCache {
		t.Fatalf("首次應 miss 並回傳值，實際 v=%q fromCache=%v err=%v", v, fromCache, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("首次應僅 1 次上游呼叫，實際 %d", calls.Load())
	}

	v, fromCache, err = GetOrFetch(ctx(t), c, "k1", time.Minute, fetch)
	if err != nil || v != "tw" || !fromCache {
		t.Fatalf("第二次應命中，實際 v=%q fromCache=%v err=%v", v, fromCache, err)
	}
	if calls.Load() != 1 {
		t.Fatalf("命中時不得再呼叫上游，實際 %d", calls.Load())
	}
}

// 不同鍵不互相干擾。
func TestGetOrFetchDistinctKeys(t *testing.T) {
	c := newTestCache(t)
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "v", nil
	}
	for _, k := range []string{"k-a", "k-b"} {
		if _, _, err := GetOrFetch(ctx(t), c, k, time.Minute, fetch); err != nil {
			t.Fatalf("key=%s 失敗: %v", k, err)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("兩鍵應各 1 次上游呼叫，實際 %d", calls.Load())
	}
}

// TTL 過期：過期後重新 miss 並呼叫上游。
func TestGetOrFetchTTLExpiry(t *testing.T) {
	c := newTestCache(t)
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "tw", nil
	}

	if _, _, err := GetOrFetch(ctx(t), c, "k", 60*time.Millisecond, fetch); err != nil {
		t.Fatal(err)
	}
	if _, fromCache, err := GetOrFetch(ctx(t), c, "k", 60*time.Millisecond, fetch); err != nil || !fromCache {
		t.Fatalf("未過期應命中，實際 fromCache=%v err=%v", fromCache, err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, fromCache, err := GetOrFetch(ctx(t), c, "k", 60*time.Millisecond, fetch); err != nil || fromCache {
		t.Fatalf("過期後應重新 miss，實際 fromCache=%v err=%v", fromCache, err)
	}
	if calls.Load() != 2 {
		t.Errorf("過期後應重新呼叫上游，實際 %d 次", calls.Load())
	}
}

// 併發同鍵：Single-flight 合流，僅一次上游呼叫（§12.2 驗收：計數器驗證）。
func TestGetOrFetchConcurrentDedup(t *testing.T) {
	c := newTestCache(t)
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond)
		return "tw", nil
	}

	const n = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			v, _, err := GetOrFetch(ctx(t), c, "hot", time.Minute, fetch)
			errs[i] = err
			if v != "tw" {
				errs[i] = errors.New("回傳值不符")
			}
		}(i)
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d 失敗: %v", i, err)
		}
	}
	if calls.Load() != 1 {
		t.Errorf("20 併發同鍵應僅 1 次上游呼叫，實際 %d", calls.Load())
	}
}

// fetch 失敗：錯誤不進快取，下次呼叫會重試。
func TestGetOrFetchErrorNotCached(t *testing.T) {
	c := newTestCache(t)
	var calls atomic.Int32
	boom := errors.New("upstream down")
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "", boom
	}

	if _, _, err := GetOrFetch(ctx(t), c, "k", time.Minute, fetch); !errors.Is(err, boom) {
		t.Fatalf("應回傳上游錯誤，實際 %v", err)
	}
	if _, _, err := GetOrFetch(ctx(t), c, "k", time.Minute, fetch); !errors.Is(err, boom) {
		t.Fatalf("第二次應重試並再次失敗，實際 %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("失敗不應被快取（可重試），實際 %d 次", calls.Load())
	}
}

// 空鍵拒絕。
func TestGetOrFetchEmptyKey(t *testing.T) {
	c := newTestCache(t)
	if _, _, err := GetOrFetch(ctx(t), c, "", time.Minute,
		func(context.Context) (string, error) { return "x", nil }); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("空鍵應回傳 ErrEmptyKey，實際 %v", err)
	}
}

// 命中標記注入：MarkCacheHit 於 _lineage 標記 is_cached + cache_ttl（§3.2）。
func TestMarkCacheHit(t *testing.T) {
	env := &model.Envelope{Data: "x", Lineage: model.Lineage{Source: model.SourceTWSEAPI}}
	MarkCacheHit(env, 90*time.Second)
	if !env.Lineage.IsCached {
		t.Error("is_cached 應為 true")
	}
	if env.Lineage.CacheTTL != 90 {
		t.Errorf("cache_ttl 應為 90 秒，實際 %d", env.Lineage.CacheTTL)
	}
	if env.Lineage.Source != model.SourceTWSEAPI {
		t.Error("MarkCacheHit 不得覆寫既有 lineage 欄位")
	}
}

// L2 重啟持久化：進程重啟（Close/重新 New）後歷史資料仍在（§4.2 驗收）。
func TestL2RestartPersistence(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "tw", nil
	}

	c1 := newTestCache(t, WithDataDir(dir))
	if _, _, err := GetOrFetch(ctx(t), c1, "k", time.Hour, fetch, WithDataset(DatasetDailyKLine, "2026-07-31")); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := newTestCache(t, WithDataDir(dir))
	v, fromCache, err := GetOrFetch(ctx(t), c2, "k", time.Hour, fetch, WithDataset(DatasetDailyKLine, "2026-07-31"))
	if err != nil {
		t.Fatal(err)
	}
	if !fromCache || v != "tw" {
		t.Fatalf("重啟後應自 L2 命中，實際 v=%q fromCache=%v", v, fromCache)
	}
	if calls.Load() != 1 {
		t.Errorf("重啟後不得再呼叫上游，實際 %d 次", calls.Load())
	}
}

// TAIFEX 歷史永久 TTL（ForeverTTL）：僅存 L2，重啟後仍在（§4.2 主要消費者）。
func TestL2ForeverPersistence(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "hist", nil
	}

	c1 := newTestCache(t, WithDataDir(dir))
	if _, _, err := GetOrFetch(ctx(t), c1, "k", ForeverTTL, fetch,
		WithDataset(DatasetTAIFEXHistory, "2025-01-15")); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := newTestCache(t, WithDataDir(dir))
	v, fromCache, err := GetOrFetch(ctx(t), c2, "k", ForeverTTL, fetch, WithDataset(DatasetTAIFEXHistory, "2025-01-15"))
	if err != nil || !fromCache || v != "hist" {
		t.Fatalf("永久 TTL 重啟後應自 L2 命中，實際 v=%q fromCache=%v err=%v", v, fromCache, err)
	}
	if calls.Load() != 1 {
		t.Errorf("重啟後不得再呼叫上游，實際 %d 次", calls.Load())
	}
}

// 短 TTL（60s 盤中）不落入 L2（l2WriteMinTTL 門檻，§4.1/§4.2 備註）。
func TestL2ShortTTLNotPersisted(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "tw", nil
	}

	c1 := newTestCache(t, WithDataDir(dir))
	if _, _, err := GetOrFetch(ctx(t), c1, "k", 60*time.Second, fetch, WithDataset(DatasetDailyKLine, "2026-07-31")); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := newTestCache(t, WithDataDir(dir))
	if _, fromCache, err := GetOrFetch(ctx(t), c2, "k", 60*time.Second, fetch, WithDataset(DatasetDailyKLine, "2026-07-31")); err != nil || fromCache {
		t.Fatalf("盤中短 TTL 不應寫入 L2（重啟後應 miss），實際 fromCache=%v err=%v", fromCache, err)
	}
	if calls.Load() != 2 {
		t.Errorf("短 TTL 資料不應落入 L2，實際上游呼叫 %d 次", calls.Load())
	}
}

// SkipL2：即使長 TTL 亦略過 L2（盤中即時路徑，§4.2 備註）。
func TestSkipL2(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "live", nil
	}

	c1 := newTestCache(t, WithDataDir(dir))
	if _, _, err := GetOrFetch(ctx(t), c1, "k", time.Hour, fetch, WithDataset(DatasetDailyKLine, "2026-07-31"), SkipL2()); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := newTestCache(t, WithDataDir(dir))
	if _, fromCache, err := GetOrFetch(ctx(t), c2, "k", time.Hour, fetch, WithDataset(DatasetDailyKLine, "2026-07-31")); err != nil || fromCache {
		t.Fatalf("SkipL2 不應寫入 L2（重啟後應 miss），實際 fromCache=%v err=%v", fromCache, err)
	}
}

// MIS Snapshot 政策即不允許入 L2（AllowL2=false），重啟後資料不應殘留。
func TestMISNeverInL2(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "snap", nil
	}

	c1 := newTestCache(t, WithDataDir(dir))
	if _, _, err := GetOrFetch(ctx(t), c1, "k", 4*time.Second, fetch, WithDataset(DatasetMISSnapshot, "2026-07-31")); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := newTestCache(t, WithDataDir(dir))
	if _, fromCache, err := GetOrFetch(ctx(t), c2, "k", 4*time.Second, fetch, WithDataset(DatasetMISSnapshot, "2026-07-31")); err != nil || fromCache {
		t.Fatalf("MIS 快照不得入 L2，實際 fromCache=%v err=%v", fromCache, err)
	}
}

// L2 命中回填 L1：第二次查詢（同進程）不需再觸碰 L2。
func TestL2HitRefillsL1(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	fetch := func(ctx context.Context) (string, error) {
		calls.Add(1)
		return "tw", nil
	}

	c := newTestCache(t, WithDataDir(dir))
	opts := []FetchOption{WithDataset(DatasetDailyKLine, "2026-07-31")}
	if _, _, err := GetOrFetch(ctx(t), c, "k", time.Hour, fetch, opts...); err != nil {
		t.Fatal(err)
	}
	// 手動清除 L1，模擬 L1 逐出後之 L2 命中。
	c.l1.del("k")
	if _, fromCache, err := GetOrFetch(ctx(t), c, "k", time.Hour, fetch, opts...); err != nil || !fromCache {
		t.Fatalf("L1 逐出後應自 L2 命中，實際 fromCache=%v err=%v", fromCache, err)
	}
	if calls.Load() != 1 {
		t.Errorf("L2 命中不得呼叫上游，實際 %d 次", calls.Load())
	}
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	return context.Background()
}
