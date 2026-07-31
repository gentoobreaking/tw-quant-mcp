package engine

import (
	"fmt"

	"tw-quant-mcp/pkg/model"
)

// 爆量偵測參數（§8.5：前 20 分鐘均量滑動窗口）。
const (
	SurgeWindowMinutes  = 20  // 滑動窗口分鐘數
	SurgeRatioThreshold = 2.0 // volume_ratio ≥ 2.0 判定為爆量
	SurgeTypeBullish    = "BULLISH_BREAKOUT"
	SurgeTypeBearish    = "BEARISH_BREAKDOWN"
	SurgeTypeNone       = "NONE"
)

// DetectSurge 對單一代碼之當日快照偵測爆量（§8.5，純記憶體計算）：
//   - 以當日 1m K 線（resample1m）為輸入，前一日資料不參與窗口（跨日邊界）
//   - recent = 最後 minutes 根 1m K 線（不足時以既有數量為準）
//   - window = recent 之前最多 20 根 1m K 線；windowAvg = 窗口均量（股/分）
//   - volume_ratio = 近 N 分鐘均量（股/分） / windowAvg
//   - volume_ratio ≥ 2.0 且方向向上 → BULLISH_BREAKOUT；向下 → BEARISH_BREAKDOWN；
//     其餘（含窗口資料不足）→ NONE
func DetectSurge(snaps []model.Snapshot, minutes int) (model.VolumeSurge, error) {
	if minutes < 1 {
		return model.VolumeSurge{}, fmt.Errorf("engine: minutes 需 ≥ 1，實際 %d", minutes)
	}
	if len(snaps) == 0 {
		return model.VolumeSurge{}, fmt.Errorf("engine: 無盤中資料（請先加入 watchlist 並等待採樣）")
	}
	last := snaps[len(snaps)-1]
	day := model.FormatDate(last.Time.Time)
	var daySnaps []model.Snapshot
	for _, s := range snaps {
		if model.FormatDate(s.Time.Time) == day {
			daySnaps = append(daySnaps, s)
		}
	}
	bars := resample1m(daySnaps)
	if len(bars) == 0 {
		return model.VolumeSurge{}, fmt.Errorf("engine: 代碼 %s 無當日 1m K 線", last.Code)
	}

	n := minutes
	if n > len(bars) {
		n = len(bars)
	}
	recent := bars[len(bars)-n:]
	var window []model.Candle
	if n < len(bars) {
		window = bars[:len(bars)-n]
		if len(window) > SurgeWindowMinutes {
			window = window[len(window)-SurgeWindowMinutes:]
		}
	}

	out := model.VolumeSurge{
		Symbol:  last.Code,
		Date:    day,
		Time:    bars[len(bars)-1].Timestamp,
		Minutes: minutes,
		Open:    recent[0].Open,
		Close:   bars[len(bars)-1].Close,
	}
	var recentVol int64
	for _, b := range recent {
		recentVol += b.Volume
	}
	out.RecentVolume = recentVol
	if len(window) > 0 {
		var winVol int64
		for _, b := range window {
			winVol += b.Volume
		}
		windowAvg := float64(winVol) / float64(len(window))
		recentPerMin := float64(recentVol) / float64(len(recent))
		out.WindowAvgVolume = windowAvg
		if windowAvg > 0 {
			out.VolumeRatio = recentPerMin / windowAvg
		}
	}

	if out.VolumeRatio >= SurgeRatioThreshold {
		if out.Close > out.Open {
			out.SurgeType = SurgeTypeBullish
		} else if out.Close < out.Open {
			out.SurgeType = SurgeTypeBearish
		} else {
			out.SurgeType = SurgeTypeNone
		}
	} else {
		out.SurgeType = SurgeTypeNone
	}
	return out, nil
}

// Surge 是 §10.A detect_volume_surge 之讀取路徑：由 RingStore 讀取
// 當日快照進行爆量偵測（純記憶體、零 HTTP）。
func (a *Aggregator) Surge(code string, minutes int) (model.VolumeSurge, error) {
	snaps := a.rings.Snapshots(code)
	if len(snaps) == 0 {
		return model.VolumeSurge{}, fmt.Errorf("engine: 代碼 %s 目前無盤中資料（請先加入 watchlist 並等待採樣）", code)
	}
	return DetectSurge(snaps, minutes)
}
