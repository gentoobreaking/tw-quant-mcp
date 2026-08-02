package cache

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver（§4.1 L2）
)

// l2 是 §4.1 L2 本地磁碟快取：SQLite（WAL mode、prepared statement、(dataset, date) 索引，§12.8）。
type l2 struct {
	db    *sql.DB
	stmts l2Stmts
}

type l2Stmts struct {
	get   *sql.Stmt // 依 key 讀取（含 expires_at）
	set   *sql.Stmt // upsert
	del   *sql.Stmt // 依 key 刪除
	list  *sql.Stmt // 依 (dataset, data_date) 列出 key（§12.8 索引消費端，供預熱/清掃）
	count *sql.Stmt // 依 dataset 計數（RingBuffer 守門測試/觀測用）
}

// l2Entry 為 L2 讀取結果。
type l2Entry struct {
	value     []byte
	expiresAt time.Time // zero 表示永久
	expired   bool      // 已過期（僅 stale-if-error 路徑使用，§5.2）
}

// openL2 於 dbPath 建立 SQLite 資料庫（WAL、prepared statement）；目錄不存在時建立。
func openL2(dbPath string) (*l2, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("cache: 建立 L2 資料目錄失敗: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cache: 開啟 L2 失敗: %w", err)
	}
	// WAL 單寫者 + 併發讀者；單連線避免 SQLITE_BUSY。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA journal_size_limit=67108864", // 64MB
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("cache: 設定 PRAGMA %s 失敗: %w", pragma, err)
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS cache_entries (
			key        TEXT PRIMARY KEY,
			dataset    TEXT NOT NULL DEFAULT '',
			data_date  TEXT NOT NULL DEFAULT '',
			value      BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL, -- unix 毫秒；0 表示永久
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_cache_entries_dataset_date
			ON cache_entries (dataset, data_date);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 建立 L2 表格失敗: %w", err)
	}

	l := &l2{db: db}
	if l.stmts.get, err = db.Prepare(
		`SELECT value, expires_at FROM cache_entries WHERE key = ?`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 準備 L2 get 失敗: %w", err)
	}
	if l.stmts.set, err = db.Prepare(`
		INSERT INTO cache_entries (key, dataset, data_date, value, created_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value, dataset = excluded.dataset,
			data_date = excluded.data_date, expires_at = excluded.expires_at,
			updated_at = excluded.updated_at`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 準備 L2 set 失敗: %w", err)
	}
	if l.stmts.del, err = db.Prepare(
		`DELETE FROM cache_entries WHERE key = ?`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 準備 L2 del 失敗: %w", err)
	}
	if l.stmts.list, err = db.Prepare(
		`SELECT key FROM cache_entries WHERE dataset = ? AND data_date = ? ORDER BY key`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 準備 L2 list 失敗: %w", err)
	}
	if l.stmts.count, err = db.Prepare(
		`SELECT COUNT(*) FROM cache_entries WHERE dataset = ?`); err != nil {
		db.Close()
		return nil, fmt.Errorf("cache: 準備 L2 count 失敗: %w", err)
	}
	return l, nil
}

// get 依 key 讀取原始列；過期項目仍回傳（expired=true）且不刪除，
// 供 v2.1 §5.2 stale-if-error 回退「已過期但仍存在」之 L2 值。
// 過期列由後續同鍵 upsert（set）覆寫，不需惰性清除。
func (l *l2) get(ctx context.Context, key string) (l2Entry, bool, error) {
	var value []byte
	var expiresAt int64
	err := l.stmts.get.QueryRowContext(ctx, key).Scan(&value, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return l2Entry{}, false, nil
	}
	if err != nil {
		return l2Entry{}, false, err
	}
	e := l2Entry{value: value}
	if expiresAt > 0 {
		e.expiresAt = time.UnixMilli(expiresAt)
		e.expired = time.Now().UnixMilli() >= expiresAt
	}
	return e, true, nil
}

// set 寫入（upsert）；ttl <= 0 表示永久（expires_at = 0）。
func (l *l2) set(ctx context.Context, key, dataset, dataDate string, value []byte, ttl time.Duration) error {
	now := time.Now()
	expiresAt := int64(0)
	if ttl > 0 {
		expiresAt = now.Add(ttl).UnixMilli()
	}
	_, err := l.stmts.set.ExecContext(ctx, key, dataset, dataDate, value,
		now.UnixMilli(), expiresAt, now.UnixMilli())
	return err
}

// del 依 key 刪除。
func (l *l2) del(ctx context.Context, key string) error {
	_, err := l.stmts.del.ExecContext(ctx, key)
	return err
}

// list 依 (dataset, data_date) 列出所有 key（§12.8 索引）。
func (l *l2) list(ctx context.Context, dataset, dataDate string) ([]string, error) {
	rows, err := l.stmts.list.QueryContext(ctx, dataset, dataDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// count 依 dataset 計數（含過期列）。
func (l *l2) count(ctx context.Context, dataset string) (int, error) {
	var n int
	if err := l.stmts.count.QueryRowContext(ctx, dataset).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// close 關閉資料庫與 prepared statement。
func (l *l2) close() error {
	var err error
	for _, s := range []*sql.Stmt{l.stmts.get, l.stmts.set, l.stmts.del, l.stmts.list, l.stmts.count} {
		if s != nil {
			if e := s.Close(); e != nil && err == nil {
				err = e
			}
		}
	}
	if e := l.db.Close(); e != nil && err == nil {
		err = e
	}
	return err
}
