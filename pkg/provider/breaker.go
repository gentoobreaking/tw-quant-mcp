package provider

import (
	"errors"
	"sync"
	"time"
)

// ErrCircuitOpen 於熔斷開啟期間快速失敗（§4.4：主機暫停 60s）。
var ErrCircuitOpen = errors.New("provider: circuit breaker open（主機暫停中）")

// 熔斷參數（§4.4）：連續 5 次失敗 → 開啟 60s。
const (
	breakerMaxFailures = 5
	breakerOpenFor     = 60 * time.Second
)

// CircuitBreaker 是單一主機之熔斷器。
// 狀態：closed（計數失敗）→ 連續 5 次失敗 → open（Allow 快速失敗）→
// 60s 後自動恢復 closed 並重置計數。
type CircuitBreaker struct {
	mu        sync.Mutex
	failures  int
	openUntil time.Time
	nowFn     func() time.Time
}

// NewCircuitBreaker 建立熔斷器。
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{nowFn: time.Now}
}

// SetNowFn 注入時鐘（僅測試用）。
func (b *CircuitBreaker) SetNowFn(fn func() time.Time) { b.nowFn = fn }

// Allow 檢查請求是否放行；熔斷開啟期間回傳 ErrCircuitOpen。
func (b *CircuitBreaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.nowFn()
	if now.Before(b.openUntil) {
		return ErrCircuitOpen
	}
	if now.After(b.openUntil) && !b.openUntil.IsZero() {
		// 熔斷時間已過，恢復 closed
		b.openUntil = time.Time{}
		b.failures = 0
	}
	return nil
}

// Record 記錄一次請求結果：成功重置計數，失敗累加，
// 達 5 次連續失敗後開啟熔斷。
func (b *CircuitBreaker) Record(ok bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ok {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= breakerMaxFailures {
		b.openUntil = b.nowFn().Add(breakerOpenFor)
	}
}
