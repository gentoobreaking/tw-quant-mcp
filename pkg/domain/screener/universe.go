package screener

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// ScanUniverse 以 bounded concurrency 掃描全市場個股（§10.2）：
// concurrency 對應對應 RATE_LIMIT_BULK_CONCURRENCY（預設 8）；
// fn 於個別 goroutine 執行，任一失敗即取消其餘並回傳首錯。
// fn 內部應套用 §5 per-source rate limiter 與快取。
func ScanUniverse(ctx context.Context, symbols []string, concurrency int, fn func(string) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, sym := range symbols {
		sym := sym
		g.Go(func() error {
			return fn(sym)
		})
	}
	return g.Wait()
}
