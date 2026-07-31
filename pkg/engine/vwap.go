package engine

import (
	"fmt"
	"sort"
	"sync"
	"time"

	"tw-quant-mcp/pkg/model"
)

// VWAPTracker 是單一代碼之增量 VWAP 累計器（§8.5）：
//   - VWAP = Σ(p×v) / Σv，每 tick O(1) 更新（p = Snapshot.Last、v = CumulativeVol 之增量）
//   - 當日高低點 / 昨收隨 tick 維護
//   - 跨日自動重置（新交易日首筆快照歸零重算）
//
// 增量累計與全量重算之結果一致（fixture 驗證，見 vwap_test.go）。
type VWAPTracker struct {
	mu        sync.RWMutex
	day       string // model.FormatDate 之交易日
	sumPV     float64
	vol       int64 // Σv（股）
	prevCum   int64 // 上一 tick 之當日累積量
	last      float64
	high, low float64
	prevClose float64
	lastTime  time.Time
}

// NewVWAPTracker 建立空白累計器。
func NewVWAPTracker() *VWAPTracker {
	return &VWAPTracker{}
}

// Update 以單筆快照增量更新（O(1)）；快照日期與累計器不同日時自動重置。
func (t *VWAPTracker) Update(s model.Snapshot) {
	day := model.FormatDate(s.Time.Time)
	t.mu.Lock()
	defer t.mu.Unlock()
	if day != t.day {
		t.sumPV = 0
		t.vol = 0
		t.prevCum = 0
		t.high = s.Last
		t.low = s.Last
		t.prevClose = s.PrevClose
		t.day = day
	}
	dv := s.CumulativeVol - t.prevCum
	if t.prevCum == 0 {
		// 當日首筆：自開盤起累計
		dv = s.CumulativeVol
	}
	if dv < 0 {
		dv = 0 // 官方量能回檔之防禦
	}
	t.sumPV += s.Last * float64(dv)
	t.vol += dv
	t.prevCum = s.CumulativeVol
	t.last = s.Last
	if s.Last > t.high {
		t.high = s.Last
	}
	if s.Last < t.low {
		t.low = s.Last
	}
	if s.PrevClose != 0 {
		t.prevClose = s.PrevClose
	}
	t.lastTime = s.Time.Time
}

// VWAP 回傳目前累計 VWAP（元）；尚無量能時為 0。
func (t *VWAPTracker) VWAP() float64 {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.vol == 0 {
		return 0
	}
	return t.sumPV / float64(t.vol)
}

func (t *VWAPTracker) snapshot() model.IntradayVWAP {
	t.mu.RLock()
	defer t.mu.RUnlock()
	supports, resistances := fibLevels(t.high, t.low, t.last)
	base := model.IntradayVWAP{
		Date:      t.day,
		Time:      t.lastTime.In(model.Taipei()).Format("15:04:05"),
		High:      t.high,
		Low:       t.low,
		Last:      t.last,
		PrevClose: t.prevClose,
	}
	if t.vol > 0 {
		base.VWAP = t.sumPV / float64(t.vol)
		base.Volume = t.vol
	}
	base.Supports = supports
	base.Resistances = resistances
	return base
}

// IntradayStore 是 per-symbol VWAPTracker 之登錄（§8.5 記憶體計算）。
type IntradayStore struct {
	mu       sync.RWMutex
	trackers map[string]*VWAPTracker
}

// NewIntradayStore 建立盤中衍生計算登錄。
func NewIntradayStore() *IntradayStore {
	return &IntradayStore{trackers: make(map[string]*VWAPTracker)}
}

// Update 以單筆快照增量更新對應代碼（計算失敗不影響 Poller 寫入：
// 純記憶體、無錯誤回傳，且呼叫端於寫入 RingBuffer 後才呼叫）。
func (s *IntradayStore) Update(sn model.Snapshot) {
	s.mu.RLock()
	t := s.trackers[sn.Code]
	s.mu.RUnlock()
	if t == nil {
		s.mu.Lock()
		t = s.trackers[sn.Code]
		if t == nil {
			t = NewVWAPTracker()
			s.trackers[sn.Code] = t
		}
		s.mu.Unlock()
	}
	t.Update(sn)
}

// UpdateAll 批次更新。
func (s *IntradayStore) UpdateAll(snaps []model.Snapshot) {
	for _, sn := range snaps {
		s.Update(sn)
	}
}

// VWAP 回傳指定代碼之盤中衍生計算（§10.A get_intraday_vwap 讀取路徑，
// 純記憶體、零 HTTP）；無資料時回傳錯誤（供 Tool handler 對 Client 回覆）。
func (s *IntradayStore) VWAP(code string) (model.IntradayVWAP, error) {
	s.mu.RLock()
	t := s.trackers[code]
	s.mu.RUnlock()
	if t == nil {
		return model.IntradayVWAP{}, fmt.Errorf("engine: 代碼 %s 目前無盤中衍生資料（請先加入 watchlist 並等待採樣）", code)
	}
	v := t.snapshot()
	v.Symbol = code
	return v, nil
}

// fibLevels 計算當日高低點之 Fibonacci 回檔位（§8.5：0.382/0.5/0.618）。
// 回檔位 = high − ratio×(high−low)；依 ref（最新價）分類：低於 ref 為支撐、
// 高於 ref 為壓力（等於視為支撐）；各列表由低價至高價排序。
func fibLevels(high, low, ref float64) (supports, resistances []model.FibLevel) {
	r := high - low
	if r <= 0 || high == 0 {
		return nil, nil
	}
	ratios := []float64{0.382, 0.5, 0.618}
	for _, k := range ratios {
		p := high - k*r
		lv := model.FibLevel{Ratio: k, Price: p}
		if p <= ref {
			supports = append(supports, lv)
		} else {
			resistances = append(resistances, lv)
		}
	}
	sort.Slice(supports, func(i, j int) bool { return supports[i].Price < supports[j].Price })
	sort.Slice(resistances, func(i, j int) bool { return resistances[i].Price < resistances[j].Price })
	return supports, resistances
}

// recomputeVWAP 為驗證用全量重算（與 VWAPTracker 增量結果一致，fixture 測試）。
func recomputeVWAP(snaps []model.Snapshot) (float64, int64) {
	var sumPV float64
	var vol int64
	var day string
	var prevCum int64
	for _, s := range snaps {
		d := model.FormatDate(s.Time.Time)
		if d != day {
			sumPV, vol, prevCum = 0, 0, 0
			day = d
		}
		dv := s.CumulativeVol - prevCum
		if prevCum == 0 {
			dv = s.CumulativeVol
		}
		if dv < 0 {
			dv = 0
		}
		sumPV += s.Last * float64(dv)
		vol += dv
		prevCum = s.CumulativeVol
	}
	if vol == 0 {
		return 0, 0
	}
	return sumPV / float64(vol), vol
}
