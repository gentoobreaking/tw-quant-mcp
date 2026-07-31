package engine

import (
	"math"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// vsn 為 VWAP 測試用快照：依 sn() 附加當日累積量（v，股）。
func vsn(code string, h, m, sec int, last float64, tv, cv int64) model.Snapshot {
	s := sn(code, h, m, sec, last, tv)
	s.CumulativeVol = cv
	s.PrevClose = 2205
	return s
}

// 增量 VWAP：重放快照後與全量重算一致（fixture 驗證，§8.5）。
func TestVWAPIncrementalConsistency(t *testing.T) {
	var snaps []model.Snapshot
	var cv int64 = 0
	price := 100.0
	for i := 1; i <= 30; i++ {
		m, s := 0, i*8
		if s >= 60 {
			m, s = 1, s-60
		}
		price += 0.25
		cv += int64(1000 * i)
		snaps = append(snaps, vsn("2330", 9, m, s, price, int64(1000*i), cv))
	}

	tr := NewVWAPTracker()
	for _, s := range snaps {
		tr.Update(s)
	}
	want, wantVol := recomputeVWAP(snaps)
	if got := tr.VWAP(); math.Abs(got-want) > 1e-9 {
		t.Errorf("增量 VWAP %.6f 應與全量重算 %.6f 一致", got, want)
	}
	if tr.snapshot().Volume != wantVol {
		t.Errorf("累計量應 %d，實際 %d", wantVol, tr.snapshot().Volume)
	}
}

// 跨日自動重置：新交易日累計器歸零重算（僅新日量能計入）。
func TestVWAPDayReset(t *testing.T) {
	var snaps []model.Snapshot
	cv := int64(0)
	for i := 1; i <= 4; i++ {
		cv += 2000
		snaps = append(snaps, vsn("2330", 9, 0, i*8, 100, 2000, cv)) // 2026-07-31
	}
	// 次日 2026-08-03：v 重啟（sn() 固定 07-31，須手動建跨日快照）
	next := func(sec int, last float64, cv int64) model.Snapshot {
		s := sn("2330", 9, 1, sec, last, 500)
		s.Time = model.NewTaipeiTime(time.Date(2026, 8, 3, 9, 1, sec, 0, model.Taipei()))
		s.CumulativeVol = cv
		s.PrevClose = 2205
		return s
	}
	snaps = append(snaps,
		next(0, 105, 500),
		next(8, 106, 1000),
	)

	tr := NewVWAPTracker()
	for _, s := range snaps {
		tr.Update(s)
	}
	want, wantVol := recomputeVWAP(snaps)
	if got := tr.VWAP(); math.Abs(got-want) > 1e-9 {
		t.Errorf("跨日後 VWAP %.6f 應等於新日全量重算 %.6f", got, want)
	}
	if wantVol != 1000 {
		t.Fatalf("新日累計量應 1000（舊日 8000 不計入），實際 %d", wantVol)
	}
	v := tr.snapshot()
	if v.Date != "2026-08-03" {
		t.Errorf("資料日期應為新日 2026-08-03，實際 %s", v.Date)
	}
}

// 高低點 / 昨收 / 最新價追蹤。
func TestVWAPHighLowPrevClose(t *testing.T) {
	tr := NewVWAPTracker()
	tr.Update(vsn("2330", 9, 0, 8, 100, 1000, 1000))
	tr.Update(vsn("2330", 9, 0, 16, 105, 500, 1500))
	tr.Update(vsn("2330", 9, 0, 24, 99, 500, 2000))
	v := tr.snapshot()
	if v.High != 105 || v.Low != 99 {
		t.Errorf("當日高低點應 105/99，實際 %.1f/%.1f", v.High, v.Low)
	}
	if v.Last != 99 || v.PrevClose != 2205 {
		t.Errorf("最新價/昨收應 99/2205，實際 %.1f/%.1f", v.Last, v.PrevClose)
	}
	if v.Time != "09:00:24" {
		t.Errorf("計算基準時間應 09:00:24，實際 %s", v.Time)
	}
}

// Fibonacci 支撐/壓力：high=100、low=90、ref=96 →
// 位階 96.18/95/93.82；96.18 為壓力、95 與 93.82 為支撐。
func TestFibonacciLevels(t *testing.T) {
	supports, resistances := fibLevels(100, 90, 96)
	if len(resistances) != 1 || math.Abs(resistances[0].Price-96.18) > 1e-9 || resistances[0].Ratio != 0.382 {
		t.Errorf("壓力應僅 96.18(0.382)，實際 %+v", resistances)
	}
	if len(supports) != 2 ||
		math.Abs(supports[0].Price-93.82) > 1e-9 || supports[0].Ratio != 0.618 ||
		math.Abs(supports[1].Price-95) > 1e-9 || supports[1].Ratio != 0.5 {
		t.Errorf("支撐應 93.82(0.618)、95(0.5) 由低至高，實際 %+v", supports)
	}

	// 無高低點（無資料）→ 空列表
	if s, r := fibLevels(0, 0, 0); len(s) != 0 || len(r) != 0 {
		t.Error("無高低點時支撐/壓力應為空")
	}
}

// IntradayStore：per-symbol 隔離、VWAP 讀取路徑、未知代碼錯誤。
func TestIntradayStore(t *testing.T) {
	store := NewIntradayStore()
	store.UpdateAll([]model.Snapshot{
		vsn("2330", 9, 0, 8, 100, 1000, 1000),
		vsn("2330", 9, 0, 16, 104, 1000, 2000),
		vsn("2330", 9, 0, 24, 102, 1000, 3000),
		vsn("6547", 9, 0, 8, 45.8, 500, 500),
	})

	v, err := store.VWAP("2330")
	if err != nil {
		t.Fatal(err)
	}
	// VWAP = (100×1000 + 104×1000 + 102×1000)/3000 = 102
	if math.Abs(v.VWAP-102) > 1e-9 || v.Symbol != "2330" {
		t.Errorf("2330 VWAP 應 102，實際 %.6f（symbol=%s）", v.VWAP, v.Symbol)
	}
	// high=104、low=100、ref=102 → 位階 102.472/102/101.528：
	// 102.472 為壓力，102 與 101.528 為支撐
	if len(v.Supports) != 2 || len(v.Resistances) != 1 {
		t.Errorf("支撐/壓力應依 Fibonacci 分類（2 支撐 1 壓力），實際 S=%+v R=%+v", v.Supports, v.Resistances)
	}

	v65, err := store.VWAP("6547")
	if err != nil || v65.VWAP != 45.8 {
		t.Errorf("6547 VWAP 應 45.8，實際 %v（err=%v）", v65.VWAP, err)
	}

	if _, err := store.VWAP("9999"); err == nil {
		t.Error("未知代碼應回傳錯誤")
	}
}

// 無量能時 VWAP 為 0 而非除零。
func TestVWAPNoVolume(t *testing.T) {
	tr := NewVWAPTracker()
	if tr.VWAP() != 0 {
		t.Error("無量能時 VWAP 應為 0")
	}
}
