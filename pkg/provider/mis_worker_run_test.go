package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
)

// fakeClock 為可推進之測試時鐘（worker goroutine 讀取，測試端寫入）。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(h, m, s int) *fakeClock {
	return &fakeClock{t: time.Date(2026, 7, 31, h, m, s, 0, model.Taipei())}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

// 測試 worker：快取節流 + 快速 tick + 可注入端點與熔斷時鐘。
func newTestWorker(t *testing.T, srv *httptest.Server, wl *engine.Watchlist, breakerClk *fakeClock) (*MISWorker, *engine.RingStore) {
	t.Helper()
	client := NewBaseClient("mis.twse.com.tw",
		WithRateInterval(time.Microsecond), WithJitterRatio(0),
		WithBreakerNow(breakerClk.now))
	rings := engine.NewRingStore()
	wk := NewMISWorker(client, wl, rings,
		WithMISURLs(srv.URL+"/index.jsp", srv.URL+"/api"),
		WithMISTick(time.Millisecond),
		WithMISIdleCheck(time.Millisecond),
		WithMISDegradedRetry(time.Millisecond))
	return wk, rings
}

// Run：盤中採樣寫入 RingStore；ctx 取消即結束。
func TestWorkerRunSamples(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if strings.HasSuffix(r.URL.Path, "/index.jsp") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Write(misFixtureBytes(t))
	}))
	defer srv.Close()

	wk, rings := newTestWorker(t, srv, testWatchlist(), newFakeClock(9, 30, 0))
	wk.now = newFakeClock(9, 30, 0).now

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = wk.Run(ctx)
		close(done)
	}()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(rings.Snapshots("2330")) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	<-done

	if len(rings.Snapshots("2330")) == 0 {
		t.Fatal("盤中採樣應已寫入 2330 快照")
	}
	if atomic.LoadInt32(&hits) < 2 {
		t.Errorf("應至少 warmup(1) + 採樣(≥1)，實際 %d 次請求", hits)
	}
	if got := wk.watchlist.Advance(newFakeClock(9, 30, 0).t); got == engine.StateDEGRADED {
		t.Error("正常採樣不應為 DEGRADED")
	}
}

// Run：連續 5 tick 失敗 → DEGRADED（30s 重試）；恢復後回到 SAMPLING。
func TestWorkerRunDegraded(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/index.jsp") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write(misFixtureBytes(t))
	}))
	defer srv.Close()

	// 熔斷時鐘與 worker 時鐘分離：DEGRADED 後推進 60s+ 模擬熔斷窗口結束。
	breakerClk := newFakeClock(9, 30, 0)
	wk, rings := newTestWorker(t, srv, testWatchlist(), breakerClk)
	wk.now = newFakeClock(9, 30, 0).now

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = wk.Run(ctx)
		close(done)
	}()
	defer func() { cancel(); <-done }()

	// 等待轉入 DEGRADED（連續 5 次失敗，同步觸發熔斷）
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if wk.watchlist.Advance(wk.now()) == engine.StateDEGRADED {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if wk.watchlist.Advance(wk.now()) != engine.StateDEGRADED {
		cancel()
		<-done
		t.Fatal("連續失敗後應轉入 DEGRADED")
	}

	// 熔斷開啟期間（60s）所有請求快速失敗，無 HTTP 流量
	fail.Store(false)
	time.Sleep(10 * time.Millisecond)
	if len(rings.Snapshots("2330")) != 0 {
		t.Error("熔斷開啟期間不得寫入資料")
	}

	// 熔斷窗口結束 → 下一次重試放行並恢復採樣
	breakerClk.set(breakerClk.now().Add(61 * time.Second))
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(rings.Snapshots("2330")) > 0 && wk.watchlist.Advance(wk.now()) == engine.StateSAMPLING {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if len(rings.Snapshots("2330")) == 0 {
		t.Error("熔斷結束後應恢復採樣並寫入資料")
	}
	if got := wk.watchlist.Advance(wk.now()); got == engine.StateDEGRADED {
		t.Error("成功採樣後應解除 DEGRADED")
	}
}

// Run：非交易日/盤外 → IDLE，不發任何採樣請求。
func TestWorkerRunIdle(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wk, _ := newTestWorker(t, srv, testWatchlist(), newFakeClock(9, 30, 0))
	wk.now = newFakeClock(14, 0, 0).now // 盤外 → IDLE

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = wk.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	if atomic.LoadInt32(&hits) != 0 {
		t.Errorf("IDLE 狀態不得發出請求，實際 %d", hits)
	}
}

// Run：重啟日清零——跨日後 RingBuffer 清空（§8.4）。
// 新日首筆採樣前應先清空舊日資料；採樣失敗時不重新寫入，
// 使「清空」狀態可被確定性觀測。
func TestWorkerRunDayReset(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/index.jsp") {
			w.WriteHeader(http.StatusOK)
			return
		}
		if fail.Load() {
			w.Write([]byte(`{"rtcode":"5000","msgArray":[]}`))
			return
		}
		w.Write(misFixtureBytes(t))
	}))
	defer srv.Close()

	wk, rings := newTestWorker(t, srv, testWatchlist(), newFakeClock(9, 30, 0))
	clk := newFakeClock(9, 30, 0)
	wk.now = clk.now

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = wk.Run(ctx)
		close(done)
	}()
	defer func() { cancel(); <-done }()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(rings.Snapshots("2330")) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if len(rings.Snapshots("2330")) == 0 {
		t.Fatal("首日應有資料")
	}

	// 推進至次日並令採樣失敗：重置後緩衝應清空且不再被填回
	fail.Store(true)
	clk.set(time.Date(2026, 8, 3, 9, 30, 0, 0, model.Taipei()))
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(rings.Snapshots("2330")) == 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if len(rings.Snapshots("2330")) != 0 {
		t.Error("新交易日首筆採樣前應清空舊日 RingBuffer")
	}
}
