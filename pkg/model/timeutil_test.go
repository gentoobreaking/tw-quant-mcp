package model

import (
	"testing"
	"time"
)

func TestFormatDate(t *testing.T) {
	// UTC 23:00 在台北已是隔日
	utc := time.Date(2026, 7, 31, 23, 0, 0, 0, time.UTC)
	if got := FormatDate(utc); got != "2026-08-01" {
		t.Errorf("FormatDate(UTC 2026-07-31 23:00) = %q, want 2026-08-01", got)
	}
	tai := time.Date(2026, 7, 31, 18, 30, 0, 0, taipei)
	if got := FormatDate(tai); got != "2026-07-31" {
		t.Errorf("FormatDate(台北 2026-07-31 18:30) = %q, want 2026-07-31", got)
	}
}

func TestParseDate(t *testing.T) {
	got, err := ParseDate("2026-08-01")
	if err != nil {
		t.Fatalf("ParseDate 失敗: %v", err)
	}
	if want := "2026-08-01T00:00:00+08:00"; got.Format(time.RFC3339) != want {
		t.Errorf("ParseDate 結果 = %q, want %q", got.Format(time.RFC3339), want)
	}
	if _, err := ParseDate("2026-8-1"); err == nil {
		t.Error("非 YYYY-MM-DD 格式應回報錯誤")
	}
	if _, err := ParseDate("2026-13-01"); err == nil {
		t.Error("非法月份應回報錯誤")
	}
}

func TestFormatHM(t *testing.T) {
	tests := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 7, 31, 9, 5, 30, 0, taipei), "09:05:00"},
		{time.Date(2026, 7, 31, 13, 30, 0, 0, taipei), "13:30:00"},
		{time.Date(2026, 7, 31, 0, 0, 59, 0, taipei), "00:00:00"},
	}
	for _, tt := range tests {
		if got := FormatHM(tt.in); got != tt.want {
			t.Errorf("FormatHM(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestParseHM(t *testing.T) {
	got, err := ParseHM("09:05:00")
	if err != nil {
		t.Fatalf("ParseHM 失敗: %v", err)
	}
	if want := "09:05:00"; got.Format("15:04:05") != want {
		t.Errorf("ParseHM 結果 = %q, want %q", got.Format("15:04:05"), want)
	}
	if _, off := got.Zone(); off != 8*3600 {
		t.Errorf("ParseHM 結果應為 +08:00，實際 %d", off)
	}
	if _, err := ParseHM("9:05:00"); err == nil {
		t.Error("非 HH:MM:00 格式應回報錯誤")
	}
	if _, err := ParseHM("09:05:30"); err == nil {
		t.Error("秒數非 00 應回報錯誤")
	}
}

func TestFormatRFC3339(t *testing.T) {
	got := FormatRFC3339(time.Date(2026, 7, 31, 17, 0, 0, 0, time.UTC))
	if want := "2026-08-01T01:00:00+08:00"; got != want {
		t.Errorf("FormatRFC3339 = %q, want %q", got, want)
	}
}

func TestTaipeiNow(t *testing.T) {
	now := TaipeiNow()
	if _, off := now.Zone(); off != 8*3600 {
		t.Errorf("TaipeiNow 時區偏移應為 +08:00，實際 %d", off)
	}
	if d := time.Since(now); d < -time.Minute || d > time.Minute {
		t.Errorf("TaipeiNow 應接近現在，實際差 %v", d)
	}
}

func TestNewTaipeiTimeConvertsZone(t *testing.T) {
	utc := time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC)
	tt := NewTaipeiTime(utc)
	if _, off := tt.Zone(); off != 8*3600 {
		t.Errorf("NewTaipeiTime 應轉換為 +08:00，實際 %d", off)
	}
	if tt.Hour() != 18 {
		t.Errorf("UTC 10:00 應為台北 18:00，實際 %d", tt.Hour())
	}
}

func TestTaipeiTimeJSONRoundTrip(t *testing.T) {
	in := TaipeiTime{Time: time.Date(2026, 7, 31, 18, 0, 0, 0, taipei)}
	b, err := in.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 失敗: %v", err)
	}
	if want := `"2026-07-31T18:00:00+08:00"`; string(b) != want {
		t.Errorf("MarshalJSON = %s, want %s", b, want)
	}
	var out TaipeiTime
	if err := out.UnmarshalJSON(b); err != nil {
		t.Fatalf("UnmarshalJSON 失敗: %v", err)
	}
	if !out.Equal(in.Time) {
		t.Errorf("round trip 不符: got %v want %v", out.Time, in.Time)
	}
	if _, off := out.Zone(); off != 8*3600 {
		t.Errorf("round trip 後時區偏移應為 +08:00，實際 %d", off)
	}
}
