package cache

import (
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

func tp(y, m, d, hh, mm int) time.Time {
	return time.Date(y, time.Month(m), d, hh, mm, 0, 0, model.Taipei())
}

// TTLFor 盤中（16:30 前）測試：對應 §4.2 政策表「盤中」欄。
func TestTTLForIntraday(t *testing.T) {
	now := tp(2026, 7, 31, 10, 0)
	cases := []struct {
		dataset string
		want    time.Duration
	}{
		{DatasetMISSnapshot, 4 * time.Second},
		{DatasetDailyKLine, 60 * time.Second},
		{DatasetInstitutional, 60 * time.Second},
		{DatasetMargin, 60 * time.Second},
		{DatasetAlertStock, 30 * time.Second},
		{DatasetMonthlyRevenue, 12 * time.Hour},
		{DatasetFinancials, 12 * time.Hour},
		{DatasetMaterialNews, 5 * time.Minute},
		{DatasetCalendar, 24 * time.Hour},
		{DatasetTAIFEXHistory, ForeverTTL},
	}
	for _, c := range cases {
		ttl, ok := TTLFor(c.dataset, now)
		if !ok {
			t.Errorf("TTLFor(%s) 應為可快取", c.dataset)
			continue
		}
		if ttl != c.want {
			t.Errorf("TTLFor(%s) 盤中 = %v，預期 %v", c.dataset, ttl, c.want)
		}
	}
}

// TTLFor 盤後（16:30 後）測試：對應 §4.2 政策表「盤後」欄。
func TestTTLForPostMarket(t *testing.T) {
	// 17:00 盤後：日線/法人/融資融券/注意處置股 → 至隔日 08:00。
	now := tp(2026, 7, 31, 17, 0)
	next8AM := tp(2026, 8, 1, 8, 0)
	wantUntil8AM := next8AM.Sub(now)
	for _, ds := range []string{DatasetDailyKLine, DatasetInstitutional, DatasetMargin, DatasetAlertStock} {
		ttl, ok := TTLFor(ds, now)
		if !ok {
			t.Errorf("TTLFor(%s) 盤後應為可快取", ds)
			continue
		}
		if ttl != wantUntil8AM {
			t.Errorf("TTLFor(%s) 盤後 = %v，預期至隔日 08:00（%v）", ds, ttl, wantUntil8AM)
		}
	}

	// 盤後固定 TTL：月營收/財報 12h、重大訊息 5min、行事曆 24h。
	for ds, want := range map[string]time.Duration{
		DatasetMonthlyRevenue: 12 * time.Hour,
		DatasetFinancials:     12 * time.Hour,
		DatasetMaterialNews:   5 * time.Minute,
		DatasetCalendar:       24 * time.Hour,
	} {
		ttl, ok := TTLFor(ds, now)
		if !ok || ttl != want {
			t.Errorf("TTLFor(%s) 盤後 = %v/%v，預期 %v", ds, ttl, ok, want)
		}
	}

	// MIS Snapshot 盤後不查（§4.2「—」欄）。
	if ttl, ok := TTLFor(DatasetMISSnapshot, now); ok || ttl != 0 {
		t.Errorf("TTLFor(mis_snapshot) 盤後應為不可快取，實際 %v/%v", ttl, ok)
	}

	// TAIFEX 歷史永久。
	if ttl, ok := TTLFor(DatasetTAIFEXHistory, now); !ok || ttl != ForeverTTL {
		t.Errorf("TTLFor(taifex_history) 盤後 = %v/%v，預期永久", ttl, ok)
	}
}

// 盤後分界：16:30（含）起為盤後。
func TestPostMarketBoundary(t *testing.T) {
	if ttl, _ := TTLFor(DatasetDailyKLine, tp(2026, 7, 31, 16, 29)); ttl != 60*time.Second {
		t.Errorf("16:29 應為盤中（60s），實際 %v", ttl)
	}
	if ttl, _ := TTLFor(DatasetDailyKLine, tp(2026, 7, 31, 16, 30)); ttl == 60*time.Second {
		t.Errorf("16:30 應為盤後（至隔日 08:00），實際 %v", ttl)
	}
	if ttl, _ := TTLFor(DatasetDailyKLine, tp(2026, 7, 31, 16, 31)); ttl == 60*time.Second {
		t.Errorf("16:31 應為盤後，實際 %v", ttl)
	}
}

// 週末盤後：隔日（下週一）08:00。
func TestPostMarketWeekend(t *testing.T) {
	now := tp(2026, 8, 1, 20, 0) // 週六 20:00
	ttl, ok := TTLFor(DatasetDailyKLine, now)
	if !ok {
		t.Fatal("盤後日線應為可快取")
	}
	want := tp(2026, 8, 2, 8, 0).Sub(now)
	if ttl != want {
		t.Errorf("週末盤後 TTL = %v，預期 %v", ttl, want)
	}
}

// 未登錄資料類別：回傳不可快取。
func TestTTLForUnknown(t *testing.T) {
	if ttl, ok := TTLFor("no_such_dataset", tp(2026, 7, 31, 10, 0)); ok || ttl != 0 {
		t.Errorf("未登錄資料類別應為不可快取，實際 %v/%v", ttl, ok)
	}
}

// AllowL2：§4.1 L2 用途（TAIFEX 歷史/盤後快照/行事曆/代碼表等），MIS 不可入 L2。
func TestAllowL2(t *testing.T) {
	for _, ds := range []string{DatasetDailyKLine, DatasetInstitutional, DatasetMargin,
		DatasetAlertStock, DatasetMonthlyRevenue, DatasetFinancials, DatasetMaterialNews,
		DatasetCalendar, DatasetTAIFEXHistory} {
		if !AllowL2(ds) {
			t.Errorf("AllowL2(%s) 應為 true", ds)
		}
	}
	if AllowL2(DatasetMISSnapshot) {
		t.Error("AllowL2(mis_snapshot) 應為 false（盤中即時路徑不可入 L2，§4.2 備註）")
	}
	if AllowL2("no_such_dataset") {
		t.Error("AllowL2 對未登錄類別應為 false")
	}
}
