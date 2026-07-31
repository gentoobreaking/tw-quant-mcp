package model

import (
	"fmt"
	"sort"
	"sync"
)

// Registry 是 §5.2 之 Symbol Registry：上市/上櫃代碼表之記憶體快照。
// 資料由 pkg/registry 之 Loader 自 TWSE/TPEx 官方清單每日預熱（§4.2 24h TTL）；
// 本型別僅負責存取與查詢，不含任何 I/O。MIS ex_ch 組裝一律經本 Registry
// 判定市場別（§5.2），禁止簡易猜測。
type Registry struct {
	mu      sync.RWMutex
	symbols map[string]Symbol
}

// NewRegistry 建立空 Registry。
func NewRegistry() *Registry {
	return &Registry{symbols: make(map[string]Symbol)}
}

// Set 以全量覆寫方式載入 Symbol 清單（每日預熱）。任一記錄未通過
// Symbol.Validate 即回傳錯誤：官方清單格式漂移須立即發現，而非靜默缺檔。
func (r *Registry) Set(symbols []Symbol) error {
	next := make(map[string]Symbol, len(symbols))
	for _, s := range symbols {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("model: registry 載入失敗: %w", err)
		}
		next[s.Code] = s
	}
	r.mu.Lock()
	r.symbols = next
	r.mu.Unlock()
	return nil
}

// Lookup 依代碼查詢 Symbol；未註冊回傳 (zero, false)，
// 供各 Tool handler 對未知代碼回覆明確錯誤。
func (r *Registry) Lookup(code string) (Symbol, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.symbols[code]
	return s, ok
}

// Market 判定代碼之市場別（"tse" | "otc"，§5.2）；未註冊回傳 (zero, false)。
func (r *Registry) Market(code string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.symbols[code]
	if !ok {
		return "", false
	}
	return s.Market, true
}

// List 回傳指定市場別之所有 Symbol（依代碼排序）；market 為空字串時回傳全部。
func (r *Registry) List(market string) []Symbol {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Symbol, 0, len(r.symbols))
	for _, s := range r.symbols {
		if market == "" || s.Market == market {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// Len 回傳已註冊之代碼數量。
func (r *Registry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.symbols)
}
