package engine

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"tw-quant-mcp/pkg/model"
)

// State 是盤中引擎之狀態（§8.2 狀態機）：
//
//	IDLE → WARMUP → SAMPLING → FLUSH → IDLE
//
// 失敗狀態 DEGRADED（§8.3：連續 5 tick 失敗）為 SAMPLING/FLUSH 之
// 正交旗標：Degrade 後 Advance 於盤中時段回傳 DEGRADED。
type State int

const (
	StateIDLE State = iota
	StateWARMUP
	StateSAMPLING
	StateFLUSH
	StateDEGRADED
)

// String 輸出狀態名稱（log 用）。
func (s State) String() string {
	switch s {
	case StateWARMUP:
		return "WARMUP"
	case StateSAMPLING:
		return "SAMPLING"
	case StateFLUSH:
		return "FLUSH"
	case StateDEGRADED:
		return "DEGRADED"
	default:
		return "IDLE"
	}
}

// 時段邊界（§8.2）：WARMUP 為開盤前 30 秒（09:00 ±30s 之開盤前窗口）、
// SAMPLING 09:00–13:30、FLUSH 13:30–13:35（收盤競價補齊最後一根 K 線）。
const (
	warmupStart = 8*3600 + 59*60 + 30 // 08:59:30
	marketOpen  = 9 * 3600            // 09:00:00
	marketClose = 13*3600 + 30*60     // 13:30:00
	flushEnd    = 13*3600 + 35*60     // 13:35:00
)

// Watchlist 是動態觀察清單管理器（§8.2）：容量硬性上限 WatchlistMax（15 檔）、
// 覆寫式更新；市場別一律由 Symbol Registry 判定（Set 輸入即 Registry 產出之
// Symbol）。狀態機由 Advance 依交易日曆與時刻推進，非交易日恆為 IDLE。
type Watchlist struct {
	mu           sync.RWMutex
	items        map[string]model.Symbol
	isTradingDay func(time.Time) bool
	degraded     bool
}

// NewWatchlist 建立 Watchlist。isTradingDay 由呼叫者注入交易日曆判定
// （如 calendar.Calendar.IsTradingDay），nil 時視為恆非交易日。
func NewWatchlist(isTradingDay func(time.Time) bool) *Watchlist {
	return &Watchlist{
		items:        make(map[string]model.Symbol),
		isTradingDay: isTradingDay,
	}
}

// Set 以覆寫方式設定觀察清單（§8.2）。symbols 須為 1..WatchlistMax 檔
// 且通過 Symbol.Validate；超過上限直接回傳錯誤（硬限制，不可放大）。
func (w *Watchlist) Set(symbols []model.Symbol) error {
	if len(symbols) == 0 {
		return fmt.Errorf("engine: watchlist 至少 1 檔")
	}
	if len(symbols) > WatchlistMax {
		return fmt.Errorf("engine: watchlist 上限 %d 檔，收到 %d 檔", WatchlistMax, len(symbols))
	}
	next := make(map[string]model.Symbol, len(symbols))
	for _, s := range symbols {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("engine: watchlist 含不合法 Symbol: %w", err)
		}
		next[s.Code] = s
	}
	w.mu.Lock()
	w.items = next
	w.mu.Unlock()
	return nil
}

// Len 回傳目前觀察檔數。
func (w *Watchlist) Len() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.items)
}

// Symbols 回傳目前觀察清單（依代碼排序）。
func (w *Watchlist) Symbols() []model.Symbol {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make([]model.Symbol, 0, len(w.items))
	for _, s := range w.items {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Code < out[j].Code })
	return out
}

// ExCh 組裝 MIS 批次參數（§8.3）：單一請求 ex_ch=tse_2330.tw|otc_6547.tw|...
// 市場別一律來自 Symbol（Symbol.Exch），禁止猜測。
func (w *Watchlist) ExCh() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	parts := make([]string, 0, len(w.items))
	for _, s := range w.items {
		parts = append(parts, s.Exch())
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// Lookup 依代碼查詢觀察清單內之 Symbol。
func (w *Watchlist) Lookup(code string) (model.Symbol, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	s, ok := w.items[code]
	return s, ok
}

// MarkDegraded 記錄連續失敗已達 §8.3 門檻（由 MISWorker 於 5 次連續失敗時呼叫）。
func (w *Watchlist) MarkDegraded() {
	w.mu.Lock()
	w.degraded = true
	w.mu.Unlock()
}

// MarkHealthy 記錄最近一次採樣成功（解除 DEGRADED）。
func (w *Watchlist) MarkHealthy() {
	w.mu.Lock()
	w.degraded = false
	w.mu.Unlock()
}

// Advance 依 now 與交易日曆推進狀態機並回傳目前狀態。
// 非交易日恆為 IDLE（Poller 不啟動）；盤中時段且 degraded 時回傳 DEGRADED。
func (w *Watchlist) Advance(now time.Time) State {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t := now.In(model.Taipei())
	if w.isTradingDay == nil || !w.isTradingDay(t) {
		return StateIDLE
	}
	sec := t.Hour()*3600 + t.Minute()*60 + t.Second()
	st := StateIDLE
	switch {
	case sec < warmupStart:
		st = StateIDLE
	case sec < marketOpen:
		st = StateWARMUP
	case sec < marketClose:
		st = StateSAMPLING
	case sec < flushEnd:
		st = StateFLUSH
	default:
		st = StateIDLE
	}
	if st != StateIDLE && w.degraded {
		return StateDEGRADED
	}
	return st
}
