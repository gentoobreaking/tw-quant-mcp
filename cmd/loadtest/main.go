// tw-quant-mcp 壓力測試工具（T019 驗收 #5）。
//
// 用法：
//
//	go run ./cmd/loadtest [flags]
//
// 模擬 20 個併發 Client 對同一熱門股（2330）重複查詢 get_stock_daily_quote，
// 驗證 §12 效能原則：Single-flight 併流、L1/L2 快取命中率（目標 ≥ 80%）、
// 延遲分位數（P50/P90/P95/P99）。預設離線（注入 fake 資料源），
// 不連網、不觸發 Rate Limit，可直接於 CI 執行（§13 測試策略）。
//
// 旗標：
//
//	-concurrency N   併發 Client 數（預設 20，§13 壓力測試基準）
//	-requests N      每 Client 查詢次數（預設 10）
//	-warmup N        預熱查詢次數（預設 5，暖 L1/L2 以量測真實命中）
//	-symbol CODE     熱門股代碼（預設 2330）
//	-v               輸出每次查詢之延遲（預設僅彙總）
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"net/url"
	"os"
	"sort"
	"sync"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/mcp"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// loadFetcher 為壓力測試之離線資料替身（與 pkg/mcp 測試替身同構）：
// 依「ds|params」鍵回傳固定日 K 資料，並統計上游呼叫次數（Single-flight 驗證用）。
type loadFetcher struct {
	mu    sync.Mutex
	calls map[string]int
	// latency 模擬上游網路延遲（僅 miss 時），預設 50ms。
	latency time.Duration
}

func newLoadFetcher() *loadFetcher {
	return &loadFetcher{calls: map[string]int{}, latency: 50 * time.Millisecond}
}

func (f *loadFetcher) key(ds string, params url.Values) string { return ds + "|" + params.Encode() }

func (f *loadFetcher) doFetch(key string) *provider.RawResponse {
	f.mu.Lock()
	f.calls[key]++
	f.mu.Unlock()
	if f.latency > 0 {
		time.Sleep(f.latency) // 模擬上游延遲（單 flight 併流時只發生一次）
	}
	// 三個月日 K（mkDailyMonth 同款：收盤 100+i 線性上升）
	rows := make([]model.Candle, 0, 30)
	for i := 0; i < 30; i++ {
		rows = append(rows, model.Candle{
			Timestamp: fmt.Sprintf("2026-07-%02d", i+1),
			Open:      100 + float64(i), High: 101 + float64(i),
			Low: 99 + float64(i), Close: 100 + float64(i),
			Volume: 1000, Amount: 100000,
		})
	}
	b, _ := json.Marshal(rows)
	return &provider.RawResponse{Body: b, SourceURL: key, StatusCode: 200}
}

// ── 以下為 WebFetcher 介面實作（URL 回傳 key 供 fake 路由）──

type lfWeb struct{ f *loadFetcher }

func (w lfWeb) URL(ds provider.TWSEWebDataset, params url.Values) string {
	return w.f.key(fmt.Sprint(ds), params)
}
func (w lfWeb) Fetch(_ context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.doFetch(req.URL), nil
}
func (w lfWeb) Validate(*provider.RawResponse) error                { return nil }
func (w lfWeb) Normalize(raw *provider.RawResponse) ([]byte, error) { return raw.Body, nil }

type lfAPI struct{ f *loadFetcher }

func (w lfAPI) URL(ds provider.TWSEAPIDataset, params url.Values) string {
	return w.f.key(fmt.Sprint(ds), params)
}
func (w lfAPI) Fetch(_ context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.doFetch(req.URL), nil
}
func (w lfAPI) Validate(*provider.RawResponse) error                { return nil }
func (w lfAPI) Normalize(raw *provider.RawResponse) ([]byte, error) { return raw.Body, nil }

type lfTPEx struct{ f *loadFetcher }

func (w lfTPEx) URL(ds provider.TPExDataset, params url.Values) string {
	return w.f.key(fmt.Sprint(ds), params)
}
func (w lfTPEx) Fetch(_ context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.doFetch(req.URL), nil
}
func (w lfTPEx) Validate(*provider.RawResponse) error                { return nil }
func (w lfTPEx) Normalize(raw *provider.RawResponse) ([]byte, error) { return raw.Body, nil }

type lfMOPS struct{ f *loadFetcher }

func (w lfMOPS) URL(ds provider.MOPSDataset, params url.Values) string {
	return w.f.key(fmt.Sprint(ds), params)
}
func (w lfMOPS) Fetch(_ context.Context, req provider.RawRequest) (*provider.RawResponse, error) {
	return w.f.doFetch(req.URL), nil
}
func (w lfMOPS) Validate(*provider.RawResponse) error                   { return nil }
func (w lfMOPS) Normalize(raw *provider.RawResponse) ([]byte, error)    { return raw.Body, nil }
func (w lfMOPS) RawNormalize(raw *provider.RawResponse) ([]byte, error) { return raw.Body, nil }

func main() {
	concurrency := flag.Int("concurrency", 20, "併發 Client 數（§13 基準 20）")
	requests := flag.Int("requests", 10, "每 Client 查詢次數")
	warmup := flag.Int("warmup", 5, "預熱查詢次數（暖快取）")
	symbol := flag.String("symbol", "2330", "熱門股代碼")
	verbose := flag.Bool("v", false, "輸出每次查詢延遲")
	upLatency := flag.Duration("upstream-latency", 50*time.Millisecond, "模擬上游延遲（miss 時）")
	flag.Parse()

	if *concurrency <= 0 || *requests <= 0 {
		fmt.Fprintln(os.Stderr, "concurrency/requests 需 > 0")
		os.Exit(2)
	}

	ctx := context.Background()
	fetcher := newLoadFetcher()
	fetcher.latency = *upLatency

	// App：注入 fake 資料源 + 記憶體快取（含 L2？僅 L1 記憶體即可量測命中）。
	// 壓力測試目標為 Single-flight 併流與 L1 命中率；L2 SQLite 為另一測試（cache 套件）。
	c, err := cache.New() // L1 Ristretto + L2 SQLite（DATA_DIR 預設，可用環境變數隔離）
	if err != nil {
		fmt.Fprintf(os.Stderr, "快取初始化失敗: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	symbols := model.NewRegistry()
	_ = symbols.Set([]model.Symbol{{Code: *symbol, Name: "熱門股", Market: model.MarketTSE}})
	now := time.Date(2026, 7, 30, 16, 0, 0, 0, model.Taipei()) // 盤後（B/C 工具時段）

	app, err := mcp.NewApp(nil,
		mcp.WithAppClock(func() time.Time { return now }),
		mcp.WithAppSymbols(symbols),
		mcp.WithAppSources(lfWeb{fetcher}, lfAPI{fetcher}, lfTPEx{fetcher}),
		mcp.WithAppMOPS(lfMOPS{fetcher}),
		mcp.WithAppCache(c),
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "App 初始化失敗: %v\n", err)
		os.Exit(1)
	}
	defer app.Close()

	args := map[string]any{"symbol": *symbol, "date": "2026-07-30"}

	// 預熱：單一 Client 連續查詢暖 L1/L2
	for i := 0; i < *warmup; i++ {
		if _, err := app.Core().Call(ctx, "get_stock_daily_quote", args); err != nil {
			fmt.Fprintf(os.Stderr, "預熱失敗: %v\n", err)
			os.Exit(1)
		}
	}

	// 量測：N Client × M 次（同時起跑，製造 Single-flight 併流）
	var mu sync.Mutex
	var lats []time.Duration
	hits, misses := 0, 0
	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func(seed int64) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed))
			for j := 0; j < *requests; j++ {
				t0 := time.Now()
				env, err := app.Core().Call(ctx, "get_stock_daily_quote", args)
				lat := time.Since(t0)
				if err != nil {
					fmt.Fprintf(os.Stderr, "查詢失敗: %v\n", err)
					continue
				}
				e, ok := env.(*model.Envelope)
				if !ok {
					fmt.Fprintln(os.Stderr, "回傳非 Envelope")
					continue
				}
				mu.Lock()
				lats = append(lats, lat)
				if e.Lineage.IsCached {
					hits++
				} else {
					misses++
				}
				mu.Unlock()
				if *verbose {
					fmt.Printf("client=%02d cached=%v latency=%s\n", i, e.Lineage.IsCached, lat.Round(time.Microsecond))
				}
				// 微隨機間隔模擬真實 Client 行為（避免完全同步）
				time.Sleep(time.Duration(rng.Intn(200)) * time.Microsecond)
			}
		}(int64(i))
	}
	wg.Wait()
	elapsed := time.Since(start)

	total := hits + misses
	if total == 0 {
		fmt.Fprintln(os.Stderr, "無有效查詢")
		os.Exit(1)
	}
	hitRate := float64(hits) / float64(total) * 100
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })

	pct := func(p float64) time.Duration {
		if len(lats) == 0 {
			return 0
		}
		idx := int(float64(len(lats)-1) * p)
		return lats[idx]
	}

	// 上游呼叫次數：Single-flight 併流後應遠小於總查詢次數（理想 ≈ warmup + 各月 1 次）
	var upstream int
	fetcher.mu.Lock()
	for _, n := range fetcher.calls {
		upstream += n
	}
	fetcher.mu.Unlock()

	fmt.Printf("\n===== tw-quant-mcp 壓力測試彙總（symbol=%s）=====\n", *symbol)
	fmt.Printf("併發 Client : %d × %d 次/Client = %d 總查詢\n", *concurrency, *requests, total)
	fmt.Printf("總耗時      : %s（平均吞吐 %.0f req/s）\n", elapsed.Round(time.Millisecond), float64(total)/elapsed.Seconds())
	fmt.Printf("快取命中率  : %.1f%%（%d hit / %d miss；§13 目標 ≥ 80%%）\n", hitRate, hits, misses)
	fmt.Printf("上游呼叫    : %d（Single-flight 併流，遠小於 %d 查詢）\n", upstream, total)
	fmt.Printf("延遲分位數  : P50=%s  P90=%s  P95=%s  P99=%s  max=%s\n",
		pct(0.50).Round(time.Microsecond), pct(0.90).Round(time.Microsecond),
		pct(0.95).Round(time.Microsecond), pct(0.99).Round(time.Microsecond),
		lats[len(lats)-1].Round(time.Microsecond))

	ok := hitRate >= 80
	fmt.Printf("\n結果: %s（命中率目標 ≥ 80%%）\n", map[bool]string{true: "PASS", false: "FAIL"}[ok])
	if !ok {
		os.Exit(1)
	}
}
