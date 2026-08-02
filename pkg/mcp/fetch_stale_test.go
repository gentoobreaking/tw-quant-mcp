package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// TestFetchNormalizeStaleFallback v2.1 §5.2 stale-if-error 端到端：
// L2 存在已過期值且上游失敗 → fetchNormalize 回退過期值，
// 回傳 stale=true / cached=true / err=nil（Handler 因此標記
// _lineage.freshness=STALE_FALLBACK，§3.2）。
func TestFetchNormalizeStaleFallback(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "cache.db")
	c, err := cache.New(cache.WithSQLitePath(dbPath))
	if err != nil {
		t.Fatalf("cache.New 失敗: %v", err)
	}
	t.Cleanup(func() { c.Close() })

	app, err := NewApp(nil,
		WithAppClock(testClock),
		WithAppSymbols(seedSymbols()),
		WithAppCache(c),
	)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}

	// 直接種入已過期 L2 列（SQL 直寫；短 TTL 不過 l2WriteMinTTL 門檻，
	// 且 mcp 測試無法存取 l2 內部 API）。
	raw, _ := json.Marshal([]string{"v1"})
	sdb, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer sdb.Close()
	if _, err := sdb.Exec(`INSERT INTO cache_entries
		(key, dataset, data_date, value, created_at, expires_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"stale:mops", cache.DatasetMonthlyRevenue, "2026-07-31", raw,
		time.Now().UnixMilli()-3600_000, // 1 小時前寫入
		time.Now().UnixMilli()-600_000,  // 10 分鐘前已過期
		time.Now().UnixMilli()-3600_000); err != nil {
		t.Fatal(err)
	}

	type rows = []string
	v, cached, stale, err := fetchNormalize[rows](app, ctx, string(provider.MOPSMonthlyRevenue),
		"2026-07-31", "stale:mops",
		func() ([]byte, error) { return nil, errors.New("上游掛點") })
	if err != nil || !stale || !cached {
		t.Fatalf("上游失敗應 stale 回退（stale=true cached=true err=nil），實際 stale=%v cached=%v err=%v", stale, cached, err)
	}
	if len(v) != 1 || v[0] != "v1" {
		t.Fatalf("應回退過期值 [v1]，實際 %v", v)
	}

	// Handler 標記契約：stale=true 時 freshness 為 STALE_FALLBACK。
	lg := postLineage("MOPS", "2026-07-31", cached, stale, 30*24*time.Hour)
	if lg.Freshness != model.FreshnessStaleFallback || !lg.IsCached {
		t.Errorf("stale 回退之 lineage 應標 STALE_FALLBACK 且 IsCached，實際 %+v", lg)
	}
	if lg.CacheTTL != int((30 * 24 * time.Hour).Seconds()) {
		t.Errorf("CacheTTL 應帶政策 TTL，實際 %d", lg.CacheTTL)
	}
}
