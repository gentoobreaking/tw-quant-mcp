package screener

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// newIndexStore 於一時目錄建立空索引。
func newIndexStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatalf("NewStore 失敗: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestIndexBuildAndQuery 驗證 §10.3：Replace→Query 依殖利率遞減、
// 條件過濾與 LIMIT，freshness=建立時間。
func TestIndexBuildAndQuery(t *testing.T) {
	ctx := context.Background()
	s := newIndexStore(t)

	date := "2026-08-03"
	built := time.Date(2026, 8, 3, 15, 1, 0, 0, time.FixedZone("CST", 8*3600))
	rows := []IndexRow{
		{Date: date, Symbol: "2414", Name: "中華電", Market: "TSE", DividendYieldPct: 4.2, CashDividend: 4.2, ConsecutiveYears: 20, PE: 26},
		{Date: date, Symbol: "2317", Name: "鴻海", Market: "TSE", DividendYieldPct: 3.5, CashDividend: 7.2, ConsecutiveYears: 2, PE: 12, PB: 1.5},
		{Date: date, Symbol: "6147", Name: "頎邦", Market: "OTC", DividendYieldPct: 4.0, CashDividend: 4.0, ConsecutiveYears: 1, PE: 15},
		{Date: date, Symbol: "1101", Name: "台泥", Market: "TSE", DividendYieldPct: 3.29, CashDividend: 0.8, ConsecutiveYears: 1, PE: 0, PEAvailable: false},
	}
	if err := s.Replace(ctx, date, rows, built); err != nil {
		t.Fatalf("Replace 失敗: %v", err)
	}

	if got, ok, err := s.BuiltAt(ctx, date); err != nil || !ok {
		t.Fatalf("BuiltAt 應存在: got=%v ok=%v err=%v", got, ok, err)
	} else if !got.Equal(built) {
		t.Errorf("freshness 建立時間應為 %v，實際 %v", built, got)
	}

	// 基本查詢：全量依殖利率遞減
	hit, err := s.Query(ctx, IndexQuery{Date: date})
	if err != nil {
		t.Fatalf("Query 失敗: %v", err)
	}
	if !hit.BuiltAt.Equal(built) {
		t.Errorf("查詢應攜帶建立時間 %v，實際 %v", built, hit.BuiltAt)
	}
	wantOrder := []string{"2414", "6147", "2317", "1101"}
	if len(hit.Rows) != len(wantOrder) {
		t.Fatalf("應回傳 %d 列，實際 %d", len(wantOrder), len(hit.Rows))
	}
	for i, code := range wantOrder {
		if hit.Rows[i].Symbol != code {
			t.Errorf("第 %d 應為 %s，實際 %s", i, code, hit.Rows[i].Symbol)
		}
	}

	// min_yield>=4 + LIMIT 1 → 殖利率最高之 2414
	hit, _ = s.Query(ctx, IndexQuery{Date: date, MinYield: 4.0, Rows: 1})
	if len(hit.Rows) != 1 || hit.Rows[0].Symbol != "2414" {
		t.Errorf("min_yield>=4 + LIMIT 1 應回傳 2414，實際 %+v", hit.Rows)
	}

	// min_dividend>=5 → 僅 2317 (7.2)
	hit, _ = s.Query(ctx, IndexQuery{Date: date, MinDividend: 5.0})
	if len(hit.Rows) != 1 || hit.Rows[0].Symbol != "2317" {
		t.Fatalf("min_dividend>=5 應僅 2317，實際 %+v", hit.Rows)
	}

	// market=OTC → 僅 6147
	hit, _ = s.Query(ctx, IndexQuery{Date: date, Market: "OTC"})
	if len(hit.Rows) != 1 || hit.Rows[0].Symbol != "6147" {
		t.Errorf("market=OTC 應僅 6147，實際 %+v", hit.Rows)
	}

	// MaxPE=10（僅 PEAvailable 判定）→ 無命中（1101 無 PE；其餘皆 >10）
	hit, _ = s.Query(ctx, IndexQuery{Date: date, MaxPE: 10})
	if len(hit.Rows) != 0 {
		t.Errorf("MaxPE=10 應無命中，實際 %+v", hit.Rows)
	}

	// MinConsecutive>=2 → 2414 (20) + 2317 (2)
	hit, _ = s.Query(ctx, IndexQuery{Date: date, MinConsecutive: 2})
	if len(hit.Rows) != 2 {
		t.Fatalf("min_consecutive>=2 應回傳 2 列，實際 %+v", hit.Rows)
	}
}

// TestIndexReplaceOverwrites 重建同一日期索引會覆蓋舊值（primary key 語義）。
func TestIndexReplaceOverwrites(t *testing.T) {
	ctx := context.Background()
	s := newIndexStore(t)
	date := "2026-08-03"
	built1 := time.Date(2026, 8, 3, 15, 0, 0, 0, time.Local)
	built2 := built1.Add(1 * time.Hour)
	if err := s.Replace(ctx, date, []IndexRow{
		{Symbol: "2330", Name: "台積電", Market: "TSE", DividendYieldPct: 2.1, ConsecutiveYears: 10},
		{Symbol: "2317", Name: "鴻海", Market: "TSE", DividendYieldPct: 3.5},
	}, built1); err != nil {
		t.Fatal(err)
	}
	if err := s.Replace(ctx, date, []IndexRow{
		{Symbol: "2414", Name: "中華電", Market: "TSE", DividendYieldPct: 4.2},
	}, built2); err != nil {
		t.Fatal(err)
	}
	hit, err := s.Query(ctx, IndexQuery{Date: date})
	if err != nil {
		t.Fatal(err)
	}
	if len(hit.Rows) != 1 || hit.Rows[0].Symbol != "2414" {
		t.Fatalf("重建應取代舊索引（僅剩 2414），實際 %+v", hit.Rows)
	}
	if !hit.BuiltAt.Equal(built2) {
		t.Errorf("freshness 應為新建立時間 %v，實際 %v", built2, hit.BuiltAt)
	}
}

// TestIndexBuiltAtMissing 未建立之日期回傳無。
func TestIndexBuiltAtMissing(t *testing.T) {
	_, ok, err := newIndexStore(t).BuiltAt(context.Background(), "2026-01-01")
	if err != nil || ok {
		t.Fatalf("未建立之日期應回傳 (false, nil)，實際 ok=%v err=%v", ok, err)
	}
}
