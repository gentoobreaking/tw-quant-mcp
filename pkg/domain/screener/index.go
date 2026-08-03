package screener

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // CGO-free SQLite driver（§5 L2）
)

// index.go 實作 v2.1 §10.3 Materialized Screener Index：
// 每日收盤後（15:00）預先計算全市場個股之股利 / 估值 / 財報體檢
// OverallScore 快照，寫入 SQLite；篩選 Tool 直接 SELECT 索引，
// 零即時 Adapter 請求，_lineage.freshness 反應索引建立時間。
//
// 表欄位對應 §6 正規化 Schema 之快照（DividendRecord /
// ValuationRatios / FinancialHealthReport.OverallScore 等）。

// IndexRow 為 Materialized Screener Index 之單檔快照。
type IndexRow struct {
	Date                 string  // 資料歸屬日期 YYYY-MM-DD（查詢鍵）
	Symbol               string  // 代碼
	Name                 string  // 名稱
	Market               string  // TSE | OTC
	DividendYieldPct     float64 // 現金殖利率（%）
	CashDividend         float64 // 每股現金股利（元/股）
	PayoutStability      float64 // 配息穩定度（0-100，連年配息導出）
	ConsecutiveYears     int     // 連年配息年數
	PE                   float64 // 本益比（虧損為 0）
	PEAvailable          bool    // 本益比是否適用
	PB                   float64 // 股價淨值比
	RevenueGrowthPct     float64 // 月營收 YoY（%）
	ProfitGrowthPct      float64 // 淨利 YoY（%）
	FinancialHealthScore float64 // 五面向財報體檢 OverallScore（0=缺資料）
}

// IndexQuery 為索引查詢條件（§10.3）。
type IndexQuery struct {
	Date           string  // 指定日期；空則視為最新
	Market         string  // 過濾市場："" 兩市場；TSE 或 OTC
	MinYield       float64 // 最低現金殖利率（%）
	MinDividend    float64 // 最低每股現金股利
	MaxPE          float64 // 本益比上限（0=不限制；僅於 PEAvailable 時判定）
	MinConsecutive int     // 最低連年配息年數
	Rows           int     // 回傳上限（0=不限）
}

// IndexHit 為一次索引查詢結果（含建立時間）。
type IndexHit struct {
	Rows    []IndexRow
	BuiltAt time.Time // 索引建立時間（freshness 標註）
}

// Store 是 §10.3 之 SQLite 索引儲存。
// 獨立於 pkg/cache 之 KV 表：以型別化欄位支持直接 SELECT ... ORDER BY
// dividend_yield_pct DESC LIMIT ? 之查詢路徑。
type Store struct {
	db *sql.DB
}

// DefaultIndexPath 回傳索引資料庫檔路徑（資料目錄下 index.db）。
func DefaultIndexPath(dataDir string) string {
	return filepath.Join(dataDir, "index.db")
}

// NewStore 於 dbPath 開啟（無則建立）screener_index 資料庫。
func NewStore(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("screener: index 資料庫路徑不得為空")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("screener: 建立索引資料目錄失敗: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("screener: 開啟索引資料庫失敗: %w", err)
	}
	// WAL + 單連線（與 §4.1 L2 同規則避免 SQLITE_BUSY）。
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("screener: PRAGMA %s 失敗: %w", pragma, err)
		}
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS screener_index (
			date                TEXT NOT NULL,
			symbol              TEXT NOT NULL,
			market              TEXT NOT NULL,
			name                TEXT NOT NULL,
			dividend_yield_pct  REAL NOT NULL,
			cash_dividend       REAL NOT NULL,
			payout_stability    REAL NOT NULL,
			consecutive_years   INTEGER NOT NULL,
			pe                  REAL NOT NULL,
			pe_available        INTEGER NOT NULL,
			pb                  REAL NOT NULL,
			revenue_growth_pct  REAL NOT NULL,
			profit_growth_pct   REAL NOT NULL,
			overall_score       REAL NOT NULL,
			created_at          INTEGER NOT NULL,
			PRIMARY KEY (date, symbol)
		);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("screener: 建立 screener_index 表失敗: %w", err)
	}
	return &Store{db: db}, nil
}

// Close 關閉索引資料庫。
func (s *Store) Close() error { return s.db.Close() }

// Replace 於單一 transaction 內重建某日期之索引（刪舊＋插入新）。
// builtAt 為索引建立時間（lineage.freshness 標註用）。
func (s *Store) Replace(ctx context.Context, date string, rows []IndexRow, builtAt time.Time) error {
	if date == "" {
		return fmt.Errorf("screener: index date 不得為空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM screener_index WHERE date = ?`, date); err != nil {
		return err
	}
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO screener_index
			(date, symbol, market, name, dividend_yield_pct, cash_dividend,
			 payout_stability, consecutive_years, pe, pe_available, pb,
			 revenue_growth_pct, profit_growth_pct, overall_score, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	created := builtAt.UnixMilli()
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, date, r.Symbol, r.Market, r.Name,
			r.DividendYieldPct, r.CashDividend, r.PayoutStability, r.ConsecutiveYears,
			r.PE, boolInt(r.PEAvailable), r.PB,
			r.RevenueGrowthPct, r.ProfitGrowthPct, r.FinancialHealthScore, created); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// BuiltAt 回傳指定日期索引之建立時間；未重建回傳 (zero, false)。
func (s *Store) BuiltAt(ctx context.Context, date string) (time.Time, bool, error) {
	var ms int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(created_at), 0) FROM screener_index WHERE date = ?`, date).Scan(&ms)
	if err != nil {
		if err == sql.ErrNoRows {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, err
	}
	if ms == 0 {
		return time.Time{}, false, nil
	}
	return time.UnixMilli(ms), true, nil
}

// Query 直接對索引執行過濾/排序（ORDER BY dividend_yield_pct DESC LIMIT），
// 零即時 Adapter 請求。回傳 IndexHit.Rows 與 BuiltAt（freshness 標註）。
func (s *Store) Query(ctx context.Context, q IndexQuery) (IndexHit, error) {
	built, _, _ := s.BuiltAt(ctx, q.Date)
	sqlq := `SELECT symbol, name, market, dividend_yield_pct, cash_dividend,
		payout_stability, consecutive_years, pe, pe_available, pb,
		revenue_growth_pct, profit_growth_pct, overall_score
		FROM screener_index WHERE date = ?`
	args := []any{q.Date}
	if q.Market == "TSE" || q.Market == "OTC" {
		sqlq += ` AND market = ?`
		args = append(args, q.Market)
	}
	if q.MinYield > 0 {
		sqlq += ` AND dividend_yield_pct >= ?`
		args = append(args, q.MinYield)
	}
	if q.MinDividend > 0 {
		sqlq += ` AND cash_dividend >= ?`
		args = append(args, q.MinDividend)
	}
	if q.MaxPE > 0 {
		sqlq += ` AND pe_available = 1 AND pe > 0 AND pe <= ?`
		args = append(args, q.MaxPE)
	}
	if q.MinConsecutive > 0 {
		sqlq += ` AND consecutive_years >= ?`
		args = append(args, q.MinConsecutive)
	}
	sqlq += ` ORDER BY dividend_yield_pct DESC`
	if q.Rows > 0 {
		sqlq += ` LIMIT ?`
		args = append(args, q.Rows)
	}

	raw, err := s.queryRows(ctx, sqlq, args...)
	if err != nil {
		return IndexHit{}, err
	}
	rows := make([]IndexRow, 0, len(raw))
	for _, r := range raw {
		rows = append(rows, IndexRow{
			Date:                 q.Date,
			Symbol:               r.symbol,
			Name:                 r.name,
			Market:               r.market,
			DividendYieldPct:     r.yieldPct,
			CashDividend:         r.cash,
			PayoutStability:      r.stability,
			ConsecutiveYears:     r.consecutive,
			PE:                   r.pe,
			PEAvailable:          r.peAvail,
			PB:                   r.pb,
			RevenueGrowthPct:     r.revGrowth,
			ProfitGrowthPct:      r.profitGrowth,
			FinancialHealthScore: r.overall,
		})
	}
	return IndexHit{Rows: rows, BuiltAt: built}, nil
}

// indexRowScan 為單列查詢之中間表示。
type indexRowScan struct {
	symbol       string
	name         string
	market       string
	yieldPct     float64
	cash         float64
	stability    float64
	consecutive  int
	pe           float64
	peAvail      bool
	pb           float64
	revGrowth    float64
	profitGrowth float64
	overall      float64
}

// queryRows 執行參數化 SELECT 並解碼列。
func (s *Store) queryRows(ctx context.Context, sqlq string, args ...any) ([]indexRowScan, error) {
	rows, err := s.db.QueryContext(ctx, sqlq, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []indexRowScan
	for rows.Next() {
		var (
			r       indexRowScan
			peAvail int
		)
		if err := rows.Scan(&r.symbol, &r.name, &r.market, &r.yieldPct, &r.cash,
			&r.stability, &r.consecutive, &r.pe, &peAvail, &r.pb,
			&r.revGrowth, &r.profitGrowth, &r.overall); err != nil {
			return nil, err
		}
		r.peAvail = peAvail == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// IndexMarket 將市場參數歸一化至「tse/otc/空」值（與 model.MarketV 常數同值），
// 供 §10.3 索引查詢之 market 過濾（""＝全部市場）。
func IndexMarket(m string) string {
	switch m {
	case "TSE", "tse":
		return "tse"
	case "OTC", "otc":
		return "otc"
	}
	return ""
}

// Count 回傳指定日期（可選市場）索引之總列數（全市場候選數）。
func (s *Store) Count(ctx context.Context, date, market string) (int, error) {
	sqlq := `SELECT COUNT(*) FROM screener_index WHERE date = ?`
	args := []any{date}
	if market != "" {
		sqlq += ` AND market = ?`
		args = append(args, market)
	}
	var n int
	if err := s.db.QueryRowContext(ctx, sqlq, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
