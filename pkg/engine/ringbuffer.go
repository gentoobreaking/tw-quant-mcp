package engine

import (
	"sync"
	"time"

	"tw-quant-mcp/pkg/model"
)

// 盤中引擎常數（§8.4）。
const (
	SamplingInterval = 8 * time.Second // 採樣間隔（§8.1：8s ± 1s Jitter，Jitter 由 HostLimiter 置於請求前）
	RingCapacity     = 2025            // 4.5h × 450 samples/h
	WatchlistMax     = 15              // §8.2 硬上限，不可放大
)

// RingBuffer 是固定容量之快照緩衝（§8.4）：O(1) append/overwrite，
// 不可擴張；容量滿時覆寫最舊資料。依代碼以獨立實例儲存（sharded map，
// 各自 RWMutex，不同代碼無鎖競爭）。
type RingBuffer struct {
	mu   sync.RWMutex
	buf  []model.Snapshot
	head int // 下次寫入位置
	n    int // 目前筆數（≤ cap）
}

// NewRingBuffer 建立容量為 capacity 之 RingBuffer（capacity ≤ 0 時採
// §8.4 預設 RingCapacity）。
func NewRingBuffer(capacity int) *RingBuffer {
	if capacity <= 0 {
		capacity = RingCapacity
	}
	return &RingBuffer{buf: make([]model.Snapshot, capacity)}
}

// Append O(1) 追加一筆快照；容量滿時覆寫最舊一筆。
func (r *RingBuffer) Append(s model.Snapshot) {
	r.mu.Lock()
	r.buf[r.head] = s
	r.head = (r.head + 1) % len(r.buf)
	if r.n < len(r.buf) {
		r.n++
	}
	r.mu.Unlock()
}

// Len 回傳目前筆數。
func (r *RingBuffer) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.n
}

// Cap 回傳容量。
func (r *RingBuffer) Cap() int { return len(r.buf) }

// Reset 清空緩衝（重啟日清零，§8.4）。
func (r *RingBuffer) Reset() {
	r.mu.Lock()
	r.n = 0
	r.head = 0
	r.mu.Unlock()
}

// Snapshots 回傳由舊至新之快照複本（聚合器唯讀路徑用）。
func (r *RingBuffer) Snapshots() []model.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Snapshot, r.n)
	for i := 0; i < r.n; i++ {
		out[i] = r.buf[(r.head-r.n+i+len(r.buf))%len(r.buf)]
	}
	return out
}

// RingStore 是 per-symbol 之 RingBuffer 集合（§8.4 sharded map）：
// 每代碼獨立 RingBuffer（獨立 RWMutex），寫入與讀取互不阻塞。
type RingStore struct {
	mu    sync.Mutex // 僅守護 map 本身（短暫持有）
	rings map[string]*RingBuffer
}

// NewRingStore 建立空 RingStore。
func NewRingStore() *RingStore {
	return &RingStore{rings: make(map[string]*RingBuffer)}
}

// Append 將快照追加至對應代碼之 RingBuffer（首次自動建立）。
func (s *RingStore) Append(sn model.Snapshot) {
	r := s.ring(sn.Code)
	r.Append(sn)
}

func (s *RingStore) ring(code string) *RingBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rings[code]
	if !ok {
		r = NewRingBuffer(0)
		s.rings[code] = r
	}
	return r
}

// Reset 清空所有代碼之緩衝（重啟日清零）。
func (s *RingStore) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.rings {
		r.Reset()
	}
}

// Snapshots 回傳指定代碼由舊至新之快照複本；無資料回傳 nil。
func (s *RingStore) Snapshots(code string) []model.Snapshot {
	r := s.ring(code)
	return r.Snapshots()
}

// Codes 回傳已有資料之代碼清單（依代碼排序）。
func (s *RingStore) Codes() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.rings))
	for c := range s.rings {
		out = append(out, c)
	}
	return out
}
