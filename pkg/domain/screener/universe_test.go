package screener

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestScanUniverseConcurrencyLimit 驗證 §10.2：併發數不超過上限。
// mock 逐檔 fn 同時 sleep，記錄同時在飛之 goroutine 數與總執行數。
func TestScanUniverseConcurrencyLimit(t *testing.T) {
	const (
		n     = 100
		limit = 8
	)
	symbols := make([]string, n)
	for i := range symbols {
		symbols[i] = "T" + string(rune('A'+i%26))
	}

	var (
		inflight   int64
		maxConcurr int64
		ran        int64
	)
	fn := func(string) error {
		cur := atomic.AddInt64(&inflight, 1)
		// 記錄高峰併發
		for {
			peak := atomic.LoadInt64(&maxConcurr)
			if cur <= peak || atomic.CompareAndSwapInt64(&maxConcurr, peak, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt64(&inflight, -1)
		atomic.AddInt64(&ran, 1)
		return nil
	}

	if err := ScanUniverse(context.Background(), symbols, limit, fn); err != nil {
		t.Fatalf("不應有錯誤: %v", err)
	}
	if got := atomic.LoadInt64(&maxConcurr); got > limit {
		t.Errorf("併發突破限制：最大同時=%d，上限=%d", got, limit)
	}
	if got := atomic.LoadInt64(&ran); got != n {
		t.Errorf("應掃描 %d 檔，實際 %d", n, got)
	}
}

// TestScanUniverseFirstErrorPropagates 驗證錯誤傳播：任一 fn 失敗
// 回傳該錯誤（errgroup.WithContext 會取消其餘工作）。
func TestScanUniverseFirstErrorPropagates(t *testing.T) {
	wantErr := errors.New("mock 上游")
	var ran int64
	err := ScanUniverse(context.Background(),
		[]string{"a", "b", "c", "d", "e", "f", "g", "h", "I", "j"}, 4,
		func(sym string) error {
			atomic.AddInt64(&ran, 1)
			time.Sleep(5 * time.Millisecond)
			if sym == "c" {
				return wantErr
			}
			return nil
		})
	if !errors.Is(err, wantErr) {
		t.Fatalf("應回傳首錯 %v，實際 %v", wantErr, err)
	}
	if ran == 0 {
		t.Fatal("應執行部分工作後才取消")
	}
}
