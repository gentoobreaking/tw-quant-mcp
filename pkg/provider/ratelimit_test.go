package provider

import (
	"context"
	"testing"
	"time"
)

func TestHostLimiterDefaultInterval(t *testing.T) {
	l := NewHostLimiter("mis.twse.com.tw", 0, 0)
	if got := l.Interval(); got != 8*time.Second {
		t.Errorf("MIS 間隔 = %v, want 8s", got)
	}
	if l.jitterRatio != 0.125 {
		t.Errorf("MIS jitter 比例 = %v, want 0.125（§4.4 ±1s/8s）", l.jitterRatio)
	}

	l2 := NewHostLimiter("www.twse.com.tw", 0, 0)
	if got := l2.Interval(); got != 2*time.Second {
		t.Errorf("TWSE-WEB 間隔 = %v, want 2s", got)
	}
	if l2.jitterRatio != 0.2 {
		t.Errorf("TWSE-WEB jitter 比例 = %v, want 0.2", l2.jitterRatio)
	}
}

func TestHostLimiterUnknownHostDefaults(t *testing.T) {
	l := NewHostLimiter("unknown.example.com", 0, 0)
	if got := l.Interval(); got != time.Second {
		t.Errorf("未登錄主機間隔 = %v, want 1s", got)
	}
}

// TestJitterRanges 驗證 jitter 等待量於 [0, interval×ratio) 範圍內且不為負。
func TestJitterRanges(t *testing.T) {
	const interval = 20 * time.Millisecond
	l := NewHostLimiter("test.host", interval, 0.5)

	var jitters []time.Duration
	l.SetSleepFunc(func(_ context.Context, d time.Duration) error {
		jitters = append(jitters, d)
		return nil
	})

	for i := 0; i < 6; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait 失敗: %v", err)
		}
	}
	if len(jitters) == 0 {
		t.Fatal("jitter 等待從未被呼叫")
	}
	maxAllowed := interval / 2
	for _, j := range jitters {
		if j <= 0 || j > maxAllowed {
			t.Errorf("jitter = %v，應於 (0, %v] 內", j, maxAllowed)
		}
	}
}

// TestWaitSequentialTiming 驗證 Wait 保證間隔 ≥ interval（rate 層）。
// 首個 Wait 持有初始權杖可立即回傳，故間隔自第 2 個起檢查。
// 注入 no-op sleep 以排除 jitter 實際等待（避免時序抖動造成 flaky）。
func TestWaitSequentialTiming(t *testing.T) {
	const interval = 30 * time.Millisecond
	l := NewHostLimiter("test.host", interval, 0)
	l.SetSleepFunc(func(_ context.Context, _ time.Duration) error { return nil })

	var starts []time.Time
	for i := 0; i < 4; i++ {
		start := time.Now()
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait 失敗: %v", err)
		}
		starts = append(starts, start)
	}
	for i := 2; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < interval-5*time.Millisecond {
			t.Errorf("間隔 %v 應 ≥ %v", gap, interval)
		}
	}
}
