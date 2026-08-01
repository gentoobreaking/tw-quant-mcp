package cache

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func openTestL2(t *testing.T) *l2 {
	t.Helper()
	l, err := openL2(t.TempDir())
	if err != nil {
		t.Fatalf("openL2 失敗: %v", err)
	}
	t.Cleanup(func() { l.close() })
	return l
}

func TestL2SetGetDel(t *testing.T) {
	l := openTestL2(t)
	ctx := context.Background()

	if _, ok, err := l.get(ctx, "missing"); err != nil || ok {
		t.Fatalf("空鍵應 miss，實際 ok=%v err=%v", ok, err)
	}

	if err := l.set(ctx, "k1", "daily_kline", "2026-07-31", []byte("v1"), time.Hour); err != nil {
		t.Fatal(err)
	}
	e, ok, err := l.get(ctx, "k1")
	if err != nil || !ok {
		t.Fatalf("應命中，實際 ok=%v err=%v", ok, err)
	}
	if string(e.value) != "v1" {
		t.Errorf("值 = %q，預期 %q", e.value, "v1")
	}
	if e.expiresAt.Before(time.Now().Add(59 * time.Minute)) {
		t.Errorf("expiresAt 應約為 1 小時後，實際 %v", e.expiresAt)
	}

	if err := l.del(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := l.get(ctx, "k1"); err != nil || ok {
		t.Fatalf("刪除後應 miss，實際 ok=%v err=%v", ok, err)
	}
}

// upsert：同鍵覆寫值與到期。
func TestL2Upsert(t *testing.T) {
	l := openTestL2(t)
	ctx := context.Background()

	if err := l.set(ctx, "k1", "daily_kline", "2026-07-31", []byte("old"), time.Hour); err != nil {
		t.Fatal(err)
	}
	if err := l.set(ctx, "k1", "daily_kline", "2026-07-31", []byte("new"), 2*time.Hour); err != nil {
		t.Fatal(err)
	}
	e, ok, err := l.get(ctx, "k1")
	if err != nil || !ok {
		t.Fatalf("upsert 後應命中，實際 ok=%v err=%v", ok, err)
	}
	if string(e.value) != "new" {
		t.Errorf("upsert 應覆寫值，實際 %q", e.value)
	}
	if !e.expiresAt.After(time.Now().Add(119 * time.Minute)) {
		t.Errorf("upsert 應覆寫到期，實際 %v", e.expiresAt)
	}
}

// TTL 過期：過期項目回傳 miss 且惰性清除。
func TestL2Expiry(t *testing.T) {
	l := openTestL2(t)
	ctx := context.Background()

	if err := l.set(ctx, "k1", "daily_kline", "2026-07-31", []byte("v1"), 60*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := l.get(ctx, "k1"); err != nil || !ok {
		t.Fatalf("未過期應命中，實際 ok=%v err=%v", ok, err)
	}
	time.Sleep(150 * time.Millisecond)
	if _, ok, err := l.get(ctx, "k1"); err != nil || ok {
		t.Fatalf("過期應 miss，實際 ok=%v err=%v", ok, err)
	}
	// 惰性清除後再次確認已自表格刪除。
	if _, ok, err := l.get(ctx, "k1"); err != nil || ok {
		t.Fatalf("惰性清除後仍應 miss，實際 ok=%v err=%v", ok, err)
	}
}

// 永久 TTL：expires_at 為 0，永不到期。
func TestL2Forever(t *testing.T) {
	l := openTestL2(t)
	ctx := context.Background()

	if err := l.set(ctx, "k1", "taifex_history", "2025-01-15", []byte("hist"), ForeverTTL); err != nil {
		t.Fatal(err)
	}
	e, ok, err := l.get(ctx, "k1")
	if err != nil || !ok {
		t.Fatalf("永久項目應命中，實際 ok=%v err=%v", ok, err)
	}
	if !e.expiresAt.IsZero() {
		t.Errorf("永久項目 expiresAt 應為 zero，實際 %v", e.expiresAt)
	}
}

// (dataset, data_date) 索引（§12.8）：list 依兩欄篩選。
func TestL2ListByDatasetDate(t *testing.T) {
	l := openTestL2(t)
	ctx := context.Background()

	for _, tc := range []struct{ key, dataset, date string }{
		{"a1", "daily_kline", "2026-07-31"},
		{"a2", "daily_kline", "2026-07-31"},
		{"b1", "daily_kline", "2026-08-01"},
		{"c1", "taifex_history", "2026-07-31"},
	} {
		if err := l.set(ctx, tc.key, tc.dataset, tc.date, []byte("v"), time.Hour); err != nil {
			t.Fatal(err)
		}
	}

	keys, err := l.list(ctx, "daily_kline", "2026-07-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "a1" || keys[1] != "a2" {
		t.Errorf("list(daily_kline, 07-31) = %v，預期 [a1 a2]", keys)
	}

	keys, err = l.list(ctx, "daily_kline", "2026-08-01")
	if err != nil || len(keys) != 1 || keys[0] != "b1" {
		t.Errorf("list(daily_kline, 08-01) = %v，預期 [b1]（err=%v）", keys, err)
	}
	if keys, err = l.list(ctx, "daily_kline", "2026-08-02"); err != nil || len(keys) != 0 {
		t.Errorf("list 無匹配應為空，實際 %v（err=%v）", keys, err)
	}
}

// §12.8 L2 最佳化驗收：WAL 模式、(dataset, data_date) 索引、prepared statement 生效。
func TestL2Optimizations(t *testing.T) {
	l := openTestL2(t)

	// WAL journal mode
	var mode string
	if err := l.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Errorf("journal_mode 應為 wal，實際 %q", mode)
	}

	// 索引存在
	var idxName string
	err := l.db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_cache_entries_dataset_date'`,
	).Scan(&idxName)
	if err != nil {
		t.Fatalf("應存在 (dataset, data_date) 索引: %v", err)
	}
	if idxName != "idx_cache_entries_dataset_date" {
		t.Errorf("索引名稱錯誤: %s", idxName)
	}

	// prepared statement 已建置
	for name, stmt := range map[string]*sql.Stmt{
		"get": l.stmts.get, "set": l.stmts.set,
		"del": l.stmts.del, "list": l.stmts.list,
	} {
		if stmt == nil {
			t.Errorf("prepared statement %s 未建置", name)
		}
	}

	// 查詢計畫確實使用索引
	ctx := context.Background()
	if err := l.set(ctx, "k1", "daily_kline", "2026-07-31", []byte("v"), time.Hour); err != nil {
		t.Fatal(err)
	}
	rows, err := l.db.Query(
		`EXPLAIN QUERY PLAN SELECT key FROM cache_entries WHERE dataset = ? AND data_date = ?`,
		"daily_kline", "2026-07-31",
	)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	foundIdx := false
	for rows.Next() {
		var id, parent, notused, detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		if detail == "SEARCH cache_entries USING INDEX idx_cache_entries_dataset_date (dataset=? AND data_date=?)" {
			foundIdx = true
		}
	}
	if !foundIdx {
		t.Error("list 查詢計畫應使用 idx_cache_entries_dataset_date")
	}
}
