package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// taipei 是 Asia/Taipei 時區；若系統無 tzdata，退而求其次以固定 +08:00 取代
// （台灣自 1979 年起無日光節約，+08:00 恆為正確）。
var taipei = loadTaipei()

func loadTaipei() *time.Location {
	loc, err := time.LoadLocation("Asia/Taipei")
	if err != nil {
		return time.FixedZone("Asia/Taipei", 8*3600)
	}
	return loc
}

// Taipei 回傳 Asia/Taipei 時區（永不為 nil）。
func Taipei() *time.Location { return taipei }

// TaipeiNow 回傳 Asia/Taipei 時區之現在時間。
func TaipeiNow() time.Time { return time.Now().In(taipei) }

// TaipeiTime 是 Asia/Taipei 時區之時間包裝（§3.2），
// JSON 序列化固定輸出 RFC3339 且含 +08:00 偏移。
type TaipeiTime struct {
	time.Time
}

// NewTaipeiTime 將任意時間轉換至 Asia/Taipei 時區。
func NewTaipeiTime(t time.Time) TaipeiTime { return TaipeiTime{Time: t.In(taipei)} }

// Now 回傳目前之 TaipeiTime。
func Now() TaipeiTime { return NewTaipeiTime(time.Now()) }

// MarshalJSON 輸出 RFC3339（Asia/Taipei），如 "2026-07-31T18:00:00+08:00"。
func (t TaipeiTime) MarshalJSON() ([]byte, error) {
	return []byte(`"` + t.In(taipei).Format(time.RFC3339) + `"`), nil
}

// UnmarshalJSON 解析 RFC3339 並轉換至 Asia/Taipei 時區。
func (t *TaipeiTime) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("model: TaipeiTime 解析失敗: %w", err)
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("model: TaipeiTime %q 非 RFC3339: %w", s, err)
	}
	t.Time = parsed.In(taipei)
	return nil
}

// FormatDate 輸出純日期 YYYY-MM-DD（Asia/Taipei，§5.1）。
func FormatDate(t time.Time) string { return t.In(taipei).Format("2006-01-02") }

// ParseDate 解析 YYYY-MM-DD 並回傳 Asia/Taipei 時區之當日零時。
func ParseDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation("2006-01-02", s, taipei)
	if err != nil {
		return time.Time{}, fmt.Errorf("model: 日期 %q 非 YYYY-MM-DD: %w", s, err)
	}
	return t, nil
}

// FormatHM 輸出盤中 K 線時間 HH:MM:00（Asia/Taipei，§5.1），秒數恆為 00。
func FormatHM(t time.Time) string { return t.In(taipei).Format("15:04:00") }

// ParseHM 解析 HH:MM:00（秒數須為 00）並回傳 Asia/Taipei 時區之時間。
// 回傳值正規化至現代日期（2000-01-01），避免 tzdata 歷史規則
// （如 1945 年前台灣 +08:06）影響時區偏移。
func ParseHM(s string) (time.Time, error) {
	t, err := time.ParseInLocation("15:04:05", s, taipei)
	if err != nil {
		return time.Time{}, fmt.Errorf("model: 時間 %q 非 HH:MM:00: %w", s, err)
	}
	// Go 的解析器對時欄位前導零寬容（"9:05:00" 可過），以 round-trip 嚴格校驗
	if t.Format("15:04:05") != s {
		return time.Time{}, fmt.Errorf("model: 時間 %q 非 HH:MM:00 格式", s)
	}
	if t.Second() != 0 {
		return time.Time{}, fmt.Errorf("model: 時間 %q 秒數必須為 00", s)
	}
	return time.Date(2000, 1, 1, t.Hour(), t.Minute(), 0, 0, taipei), nil
}

// FormatRFC3339 輸出 RFC3339（Asia/Taipei，含 +08:00 偏移）。
func FormatRFC3339(t time.Time) string { return t.In(taipei).Format(time.RFC3339) }
