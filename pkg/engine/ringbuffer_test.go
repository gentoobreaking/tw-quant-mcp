package engine

import (
	"sync"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

func sn(code string, h, m, s int, last float64, tv int64) model.Snapshot {
	return model.Snapshot{
		Code:      code,
		Time:      model.NewTaipeiTime(time.Date(2026, 7, 31, h, m, s, 0, model.Taipei())),
		Last:      last,
		MinuteVol: tv,
	}
}

// RingBuffer：O(1) append、容量滿覆寫最舊、順序由舊至新。
func TestRingBufferOrder(t *testing.T) {
	r := NewRingBuffer(4)
	for i := 1; i <= 4; i++ {
		r.Append(sn("2330", 9, 0, i*8, float64(i), int64(i)))
	}
	if r.Len() != 4 {
		t.Fatalf("Len 應為 4，實際 %d", r.Len())
	}
	snaps := r.Snapshots()
	if len(snaps) != 4 || snaps[0].Last != 1 || snaps[3].Last != 4 {
		t.Errorf("順序應由舊至新 1..4，實際 %+v", snaps)
	}

	// 覆寫最舊 3 筆：1,2,3 被 5,6,7 取代 → 4,5,6,7
	r.Append(sn("2330", 9, 0, 33, 5, 5))
	r.Append(sn("2330", 9, 0, 41, 6, 6))
	r.Append(sn("2330", 9, 0, 49, 7, 7))
	snaps = r.Snapshots()
	if len(snaps) != 4 || snaps[0].Last != 4 || snaps[3].Last != 7 {
		t.Errorf("覆寫後應為 4..7，實際 %+v", snaps)
	}
	if r.Len() != 4 || r.Cap() != 4 {
		t.Errorf("容量與長度不應超過 4，Len=%d Cap=%d", r.Len(), r.Cap())
	}
}

func TestRingBufferDefaultCapacity(t *testing.T) {
	r := NewRingBuffer(0)
	if r.Cap() != RingCapacity {
		t.Errorf("預設容量應為 RingCapacity=%d，實際 %d", RingCapacity, r.Cap())
	}
}

func TestRingBufferReset(t *testing.T) {
	r := NewRingBuffer(4)
	r.Append(sn("2330", 9, 0, 8, 1, 1))
	r.Reset()
	if r.Len() != 0 || len(r.Snapshots()) != 0 {
		t.Error("Reset 後應為空")
	}
	r.Append(sn("2330", 9, 0, 8, 9, 9))
	if snaps := r.Snapshots(); len(snaps) != 1 || snaps[0].Last != 9 {
		t.Errorf("Reset 後 append 應正常，實際 %+v", snaps)
	}
}

// RingStore：per-symbol 隔離（§8.4 sharded map）。
func TestRingStoreIsolation(t *testing.T) {
	rs := NewRingStore()
	rs.Append(sn("2330", 9, 0, 8, 100, 10))
	rs.Append(sn("6547", 9, 0, 8, 45, 3))
	rs.Append(sn("2330", 9, 0, 16, 105, 15))

	got := rs.Snapshots("2330")
	if len(got) != 2 || got[1].Last != 105 {
		t.Errorf("2330 應有 2 筆且最後 105，實際 %+v", got)
	}
	if got := rs.Snapshots("6547"); len(got) != 1 || got[0].Last != 45 {
		t.Errorf("6547 應有 1 筆 45，實際 %+v", got)
	}
	if got := rs.Snapshots("9999"); len(got) != 0 {
		t.Errorf("無資料代碼應為空，實際 %+v", got)
	}
}

// 併發：寫入與讀取並行（-race 驗證 per-symbol 鎖）。
func TestRingStoreConcurrent(t *testing.T) {
	rs := NewRingStore()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				rs.Append(sn("2330", 9, 0, 8, float64(j), int64(j)))
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = rs.Snapshots("2330")
				_ = rs.Snapshots("6547")
			}
		}()
	}
	wg.Wait()
	if n := rs.Snapshots("2330"); len(n) == 0 {
		t.Error("併發寫入後應有資料")
	}
}
