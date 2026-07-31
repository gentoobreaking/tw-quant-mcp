package engine

import (
	"math"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// surgeSnaps 建立 baseMinutes 根 1m 快照（每分鐘 1 筆、單筆桶 Volume=tv），
// 分鐘自 09:00 起算、價格 100+i。
func surgeSnaps(code string, baseMinutes int, perMin int64) []model.Snapshot {
	var snaps []model.Snapshot
	for i := 0; i < baseMinutes; i++ {
		snaps = append(snaps, sn(code, 9, i, 0, float64(100+i), perMin))
	}
	return snaps
}

// surgeTail 附加 n 根大量 1m 快照（自分鐘 base 起、每分鐘 perMin 股，
// 價格隨 dir 遞增/遞減）。
func surgeTail(code string, base, n int, perMin int64, dir float64) []model.Snapshot {
	var snaps []model.Snapshot
	for i := 0; i < n; i++ {
		snaps = append(snaps, sn(code, 9, base+i, 0, float64(100+base)+dir*float64(i), perMin))
	}
	return snaps
}

// 爆量（多頭）：窗口均量 1000 股/分、近 5 分鐘 3000 股/分 →
// ratio=3.0、收>開 → BULLISH_BREAKOUT。
func TestSurgeBullishBreakout(t *testing.T) {
	snaps := surgeSnaps("2330", 25, 1000)
	snaps = append(snaps, surgeTail("2330", 25, 5, 3000, 1)...)
	out, err := DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.SurgeType != SurgeTypeBullish {
		t.Errorf("應為 BULLISH_BREAKOUT，實際 %s", out.SurgeType)
	}
	if math.Abs(out.VolumeRatio-3.0) > 1e-9 {
		t.Errorf("volume_ratio 應 3.0，實際 %.4f", out.VolumeRatio)
	}
	if out.RecentVolume != 15000 || math.Abs(out.WindowAvgVolume-1000) > 1e-9 {
		t.Errorf("近 5 分鐘量應 15000、窗口均量 1000，實際 %d/%.1f", out.RecentVolume, out.WindowAvgVolume)
	}
	if out.Minutes != 5 || out.Close <= out.Open {
		t.Errorf("Minutes 應 5、方向向上，實際 %+v", out)
	}
}

// 爆量（空頭）：同量能但收<開 → BEARISH_BREAKDOWN。
func TestSurgeBearishBreakdown(t *testing.T) {
	snaps := surgeSnaps("2330", 25, 1000)
	snaps = append(snaps, surgeTail("2330", 25, 5, 3000, -1)...)
	out, err := DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.SurgeType != SurgeTypeBearish {
		t.Errorf("應為 BEARISH_BREAKDOWN，實際 %s", out.SurgeType)
	}
}

// 閾值邊界：ratio=2.0（精確）→ 判定爆量；ratio=1.0 → NONE。
func TestSurgeThresholdCases(t *testing.T) {
	snaps := surgeSnaps("2330", 25, 1000)
	snaps = append(snaps, surgeTail("2330", 25, 5, 2000, 1)...)
	out, err := DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.VolumeRatio < 2.0-1e-9 || out.SurgeType != SurgeTypeBullish {
		t.Errorf("ratio=2.0 應達閾值並判定爆量，實際 ratio=%.4f type=%s", out.VolumeRatio, out.SurgeType)
	}

	snaps = surgeSnaps("2330", 25, 1000)
	out, err = DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.SurgeType != SurgeTypeNone || math.Abs(out.VolumeRatio-1.0) > 1e-9 {
		t.Errorf("平穩量能應 NONE 且 ratio=1.0，實際 %s/%.4f", out.SurgeType, out.VolumeRatio)
	}
}

// 跨日邊界：前一日大量資料不參與窗口（§8.5 分鐘跨日）。
func TestSurgeCrossDayWindow(t *testing.T) {
	var snaps []model.Snapshot
	for i := 0; i < 20; i++ {
		snaps = append(snaps, model.Snapshot{
			Code:      "2330",
			Time:      model.NewTaipeiTime(time.Date(2026, 7, 30, 9, i, 0, 0, model.Taipei())),
			Last:      100,
			MinuteVol: 50000,
		})
	}
	snaps = append(snaps, surgeSnaps("2330", 25, 1000)...)
	snaps = append(snaps, surgeTail("2330", 25, 5, 3000, 1)...)
	out, err := DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.Date != "2026-07-31" {
		t.Errorf("資料日期應為當日，實際 %s", out.Date)
	}
	if math.Abs(out.VolumeRatio-3.0) > 1e-9 {
		t.Errorf("前一日資料不得污染窗口，ratio 應 3.0，實際 %.4f", out.VolumeRatio)
	}
}

// 資料不足：窗口為空 → NONE；minutes 非法/無快照 → 錯誤。
func TestSurgeInsufficientData(t *testing.T) {
	snaps := surgeSnaps("2330", 5, 1000)
	out, err := DetectSurge(snaps, 10)
	if err != nil {
		t.Fatal(err)
	}
	if out.SurgeType != SurgeTypeNone || out.VolumeRatio != 0 {
		t.Errorf("窗口不足應 NONE 且 ratio=0，實際 %s/%.4f", out.SurgeType, out.VolumeRatio)
	}

	if _, err := DetectSurge(snaps, 0); err == nil {
		t.Error("minutes=0 應回傳錯誤")
	}
	if _, err := DetectSurge(nil, 5); err == nil {
		t.Error("無快照應回傳錯誤")
	}
}

// 部分窗口：recent=5、既有 15 根 → 窗口取前 10 根均量。
func TestSurgePartialWindow(t *testing.T) {
	snaps := surgeSnaps("2330", 15, 1000)
	out, err := DetectSurge(snaps, 5)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(out.WindowAvgVolume-1000) > 1e-9 {
		t.Errorf("部分窗口均量應 1000，實際 %.1f", out.WindowAvgVolume)
	}
}

// Aggregator.Surge：RingStore 讀取路徑（純記憶體、零 HTTP）。
func TestAggregatorSurge(t *testing.T) {
	rings := NewRingStore()
	for _, s := range surgeSnaps("2330", 25, 1000) {
		rings.Append(s)
	}
	out, err := NewAggregator(rings).Surge("2330", 5)
	if err != nil {
		t.Fatal(err)
	}
	if out.Symbol != "2330" || math.Abs(out.VolumeRatio-1.0) > 1e-9 {
		t.Errorf("應回傳 2330、ratio=1.0，實際 %+v", out)
	}

	if _, err := NewAggregator(rings).Surge("9999", 5); err == nil {
		t.Error("未知代碼應回傳錯誤")
	}
}
