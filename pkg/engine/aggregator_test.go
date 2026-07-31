package engine

import (
	"testing"
)

// 單桶多 snapshot：OHLC 影線由桶內所有 z 計算（§8.4 規則表）。
func TestResampleSingleBucket(t *testing.T) {
	rs := NewRingStore()
	// 09:30 桶內 5 筆：z=100, 105, 95, 102, 104（tv 為當分鐘累積，遞增）
	rs.Append(sn("2330", 9, 30, 8, 100, 1000))
	rs.Append(sn("2330", 9, 30, 16, 105, 3000))
	rs.Append(sn("2330", 9, 30, 24, 95, 6000))
	rs.Append(sn("2330", 9, 30, 32, 102, 8000))
	rs.Append(sn("2330", 9, 30, 40, 104, 9000))

	bars, err := NewAggregator(rs).Klines("2330", "1m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("應 1 根 1m K 線，實際 %d", len(bars))
	}
	b := bars[0]
	if b.Timestamp != "09:30:00" {
		t.Errorf("桶鍵應為 09:30:00，實際 %s", b.Timestamp)
	}
	if b.Open != 100 || b.High != 105 || b.Low != 95 || b.Close != 104 {
		t.Errorf("OHLC 應為 100/105/95/104，實際 %v/%v/%v/%v", b.Open, b.High, b.Low, b.Close)
	}
	// Volume = 桶末 tv − 桶初 tv
	if b.Volume != 8000 {
		t.Errorf("Volume 應為 9000-1000=8000，實際 %d", b.Volume)
	}
}

// 跨分鐘桶：依 tlong 分桶、各自 Volume。
func TestResampleCrossBucket(t *testing.T) {
	rs := NewRingStore()
	rs.Append(sn("2330", 9, 31, 8, 100, 2000))   // 09:31
	rs.Append(sn("2330", 9, 31, 56, 102, 9000))  // 09:31
	rs.Append(sn("2330", 9, 32, 8, 101, 1500))   // 09:32（tv 每分鐘重置）
	rs.Append(sn("2330", 9, 32, 56, 103, 12000)) // 09:32

	bars, err := NewAggregator(rs).Klines("2330", "1m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("應 2 根，實際 %d", len(bars))
	}
	if bars[0].Timestamp != "09:31:00" || bars[0].Volume != 7000 {
		t.Errorf("09:31 桶 Volume 應為 7000，實際 %s/%d", bars[0].Timestamp, bars[0].Volume)
	}
	if bars[1].Timestamp != "09:32:00" || bars[1].Volume != 10500 {
		t.Errorf("09:32 桶 Volume 應為 10500，實際 %s/%d", bars[1].Timestamp, bars[1].Volume)
	}
}

// 單筆桶：以該筆 tv 近似（啟動首分鐘）。
func TestResampleSingleSnapshotBucket(t *testing.T) {
	rs := NewRingStore()
	rs.Append(sn("2330", 9, 33, 8, 100, 5000))
	bars, err := NewAggregator(rs).Klines("2330", "1m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Volume != 5000 {
		t.Errorf("單筆桶 Volume 應為該筆 tv=5000，實際 %d", bars[0].Volume)
	}
}

// 5m：由 1m 二次聚合（§8.4），含部分桶與桶對齊（09:00 起每 5 分）。
func TestResample5m(t *testing.T) {
	rs := NewRingStore()
	// 09:00–09:04 五個完整 1m 桶
	for m := 0; m < 5; m++ {
		rs.Append(sn("2330", 9, m, 8, 100+float64(m), int64(1000*(m+1))))
		rs.Append(sn("2330", 9, m, 56, 102+float64(m), int64(2000*(m+1))))
	}
	// 09:05 部分桶（進行中）
	rs.Append(sn("2330", 9, 5, 8, 110, 500))

	a := NewAggregator(rs)
	bars, err := a.Klines("2330", "5m", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("應 2 根 5m（09:00 + 09:05 部分），實際 %d", len(bars))
	}
	b0 := bars[0]
	if b0.Timestamp != "09:00:00" {
		t.Errorf("首桶應對齊 09:00:00，實際 %s", b0.Timestamp)
	}
	if b0.Open != 100 || b0.High != 106 || b0.Low != 100 || b0.Close != 106 {
		t.Errorf("5m OHLC 應為 100/106/100/106，實際 %v/%v/%v/%v", b0.Open, b0.High, b0.Low, b0.Close)
	}
	var wantVol int64
	for m := 0; m < 5; m++ {
		wantVol += int64(1000 * (m + 1))
	}
	if b0.Volume != wantVol {
		t.Errorf("5m Volume 應為 1m 加總 %d，實際 %d", wantVol, b0.Volume)
	}
	if bars[1].Timestamp != "09:05:00" {
		t.Errorf("部分桶應為 09:05:00，實際 %s", bars[1].Timestamp)
	}
}

// limit 只回傳最後 N 根。
func TestResampleLimit(t *testing.T) {
	rs := NewRingStore()
	for m := 0; m < 10; m++ {
		rs.Append(sn("2330", 9, m, 8, 100, 1000))
	}
	bars, err := NewAggregator(rs).Klines("2330", "1m", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 3 || bars[0].Timestamp != "09:07:00" {
		t.Errorf("limit=3 應回傳 09:07..09:09，實際 %d 根，首筆 %s", len(bars), bars[0].Timestamp)
	}
}

// 無資料與非法參數。
func TestResampleErrors(t *testing.T) {
	a := NewAggregator(NewRingStore())
	if _, err := a.Klines("2330", "1m", 0); err == nil {
		t.Error("無資料應回傳錯誤")
	}
	rs := NewRingStore()
	rs.Append(sn("2330", 9, 0, 8, 100, 1000))
	if _, err := a.Klines("2330", "30m", 0); err == nil {
		t.Error("非法 timeframe 應回傳錯誤")
	}
}

// 重啟日清零（§8.4）：Reset 後讀取路徑回傳無資料錯誤。
func TestResampleDayReset(t *testing.T) {
	rs := NewRingStore()
	rs.Append(sn("2330", 9, 0, 8, 100, 1000))
	a := NewAggregator(rs)
	if _, err := a.Klines("2330", "1m", 0); err != nil {
		t.Fatalf("Reset 前應有資料: %v", err)
	}
	rs.Reset()
	if _, err := a.Klines("2330", "1m", 0); err == nil {
		t.Error("Reset 後應回傳無資料")
	}
}
