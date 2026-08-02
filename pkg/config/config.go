// Package config 提供 tw-quant-mcp 的環境變數設定檔。
//
// 環境變數：
//
//	MCP_TRANSPORT  "stdio"（預設）| "streamable-http"
//	MCP_HTTP_ADDR  streamable-http 的監聽位址（預設 127.0.0.1:8787）
//	DATA_DIR       L2 SQLite 資料目錄（預設 ~/.tw-quant-mcp/data，相容舊版）
//	LOG_LEVEL      debug | info | warn | error（預設 info）
//	MCP_SCORING_CONFIG  五面向評分規則 JSON 檔路徑（選填，預設 v1 內建規則）
//	CACHE_L1_MAX_ENTRIES   L1 最大條目數（v2.1 §5.2，預設 10000）
//	CACHE_L1_MAX_MEMORY_MB L1 最大記憶體 MB（v2.1 §5.2，預設 256）
//	CACHE_L2_SQLITE_PATH   L2 SQLite 資料庫檔路徑（v2.1 §5.2，預設 ./data/cache.db）
//	CACHE_HIT_RATE_TARGET  快取命中率目標（v2.1 §5.2，預設 0.8）
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tw-quant-mcp/pkg/engine/composite"
)

// Transport 定義 MCP Server 的傳輸層。
type Transport string

const (
	// TransportStdio 透過 stdin/stdout 以 newline-delimited JSON 通訊。
	TransportStdio Transport = "stdio"
	// TransportStreamableHTTP 以 Streamable HTTP 傳輸層通訊。
	TransportStreamableHTTP Transport = "streamable-http"
)

const (
	// DefaultHTTPAddr 是 streamable-http 的預設監聽位址。
	DefaultHTTPAddr = "127.0.0.1:8787"
	// DefaultLogLevel 是預設 log level。
	DefaultLogLevel = "info"

	// DefaultL1MaxEntries 是 CACHE_L1_MAX_ENTRIES 預設值（v2.1 §5.2）。
	DefaultL1MaxEntries = 10000
	// DefaultL1MaxMemoryMB 是 CACHE_L1_MAX_MEMORY_MB 預設值（v2.1 §5.2）。
	DefaultL1MaxMemoryMB = 256
	// DefaultL2SQLitePath 是 CACHE_L2_SQLITE_PATH 預設值（v2.1 §5.2）。
	DefaultL2SQLitePath = "./data/cache.db"
	// DefaultCacheHitRateTarget 是 CACHE_HIT_RATE_TARGET 預設值（v2.1 §5.2）。
	DefaultCacheHitRateTarget = 0.8
)

// Config 是伺服器執行所需的全部設定。
type Config struct {
	Transport   Transport
	HTTPAddr    string
	DataDir     string
	LogLevel    string
	ScoringFile string // MCP_SCORING_CONFIG：五面向評分規則 JSON 檔（選填）

	// v2.1 §5.2 快取參數化。
	L1MaxEntries       int     // CACHE_L1_MAX_ENTRIES
	L1MaxMemoryMB      int     // CACHE_L1_MAX_MEMORY_MB
	L2SQLitePath       string  // CACHE_L2_SQLITE_PATH
	CacheHitRateTarget float64 // CACHE_HIT_RATE_TARGET
}

// Load 從環境變數讀取設定並填入預設值。
func Load() (*Config, error) {
	cfg := &Config{
		Transport:          TransportStdio,
		HTTPAddr:           DefaultHTTPAddr,
		DataDir:            defaultDataDir(),
		LogLevel:           DefaultLogLevel,
		L1MaxEntries:       DefaultL1MaxEntries,
		L1MaxMemoryMB:      DefaultL1MaxMemoryMB,
		L2SQLitePath:       DefaultL2SQLitePath,
		CacheHitRateTarget: DefaultCacheHitRateTarget,
	}

	if v := os.Getenv("MCP_TRANSPORT"); v != "" {
		cfg.Transport = Transport(strings.ToLower(strings.TrimSpace(v)))
	}
	if v := os.Getenv("MCP_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = strings.TrimSpace(v)
	}
	if v := os.Getenv("DATA_DIR"); v != "" {
		cfg.DataDir = strings.TrimSpace(v)
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = strings.ToLower(strings.TrimSpace(v))
	}
	if v := os.Getenv("MCP_SCORING_CONFIG"); v != "" {
		cfg.ScoringFile = strings.TrimSpace(v)
	}
	if v := os.Getenv("CACHE_L1_MAX_ENTRIES"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("config: CACHE_L1_MAX_ENTRIES 須為整數，實際 %q", v)
		}
		cfg.L1MaxEntries = n
	}
	if v := os.Getenv("CACHE_L1_MAX_MEMORY_MB"); v != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil, fmt.Errorf("config: CACHE_L1_MAX_MEMORY_MB 須為整數，實際 %q", v)
		}
		cfg.L1MaxMemoryMB = n
	}
	if v := os.Getenv("CACHE_L2_SQLITE_PATH"); v != "" {
		cfg.L2SQLitePath = strings.TrimSpace(v)
	}
	if v := os.Getenv("CACHE_HIT_RATE_TARGET"); v != "" {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return nil, fmt.Errorf("config: CACHE_HIT_RATE_TARGET 須為數字，實際 %q", v)
		}
		cfg.CacheHitRateTarget = f
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate 檢查設定值是否合法，並展開 DataDir 中的環境變數與家目錄。
func (c *Config) Validate() error {
	switch c.Transport {
	case TransportStdio, TransportStreamableHTTP:
	default:
		return fmt.Errorf("config: 不支援的 MCP_TRANSPORT %q（僅支援 %q / %q）",
			c.Transport, TransportStdio, TransportStreamableHTTP)
	}

	if c.Transport == TransportStreamableHTTP && strings.TrimSpace(c.HTTPAddr) == "" {
		return errors.New("config: MCP_TRANSPORT=streamable-http 時必須設定 MCP_HTTP_ADDR")
	}

	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("config: 不支援的 LOG_LEVEL %q（僅支援 debug/info/warn/error）", c.LogLevel)
	}

	dir, err := expandPath(c.DataDir)
	if err != nil {
		return fmt.Errorf("config: DATA_DIR 無法解析: %w", err)
	}
	c.DataDir = dir

	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return fmt.Errorf("config: 建立 DATA_DIR %q 失敗: %w", c.DataDir, err)
	}

	if c.L1MaxEntries <= 0 {
		return fmt.Errorf("config: CACHE_L1_MAX_ENTRIES 須為正整數，實際 %d", c.L1MaxEntries)
	}
	if c.L1MaxMemoryMB <= 0 {
		return fmt.Errorf("config: CACHE_L1_MAX_MEMORY_MB 須為正整數，實際 %d", c.L1MaxMemoryMB)
	}
	if c.CacheHitRateTarget <= 0 || c.CacheHitRateTarget > 1 {
		return fmt.Errorf("config: CACHE_HIT_RATE_TARGET 須在 (0, 1]，實際 %v", c.CacheHitRateTarget)
	}
	l2, err := expandPath(c.L2SQLitePath)
	if err != nil {
		return fmt.Errorf("config: CACHE_L2_SQLITE_PATH 無法解析: %w", err)
	}
	c.L2SQLitePath = l2
	if err := os.MkdirAll(filepath.Dir(l2), 0o755); err != nil {
		return fmt.Errorf("config: 建立 L2 SQLite 目錄 %q 失敗: %w", filepath.Dir(l2), err)
	}
	return nil
}

// Scoring 回傳五面向評分規則（§10.D，T017）：預設 v1 內建規則，
// 或自 MCP_SCORING_CONFIG 指定之 JSON 檔載入（欄位與
// composite.ScoringConfig 對應，可部分覆寫）。
func (c *Config) Scoring() (composite.ScoringConfig, error) {
	cfg := composite.DefaultScoringConfig()
	if c.ScoringFile == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(c.ScoringFile)
	if err != nil {
		return cfg, fmt.Errorf("config: 讀取 MCP_SCORING_CONFIG %q 失敗: %w", c.ScoringFile, err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("config: 解析 MCP_SCORING_CONFIG %q 失敗: %w", c.ScoringFile, err)
	}
	if cfg.Version == "" {
		cfg.Version = composite.DefaultScoringConfig().Version
	}
	return cfg, nil
}

// defaultDataDir 回傳 ~/.tw-quant-mcp/data（無法取得家目錄時回傳 ./data）。
func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "data"
	}
	return filepath.Join(home, ".tw-quant-mcp", "data")
}

// expandPath 展開 $ENV 與 ~ 前綴；未定義的環境變數視為錯誤。
func expandPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("路徑不得為空")
	}
	missing := false
	expanded := os.Expand(p, func(key string) string {
		v, ok := os.LookupEnv(key)
		if !ok {
			missing = true
		}
		return v
	})
	if missing {
		return "", fmt.Errorf("環境變數未定義: %q", p)
	}
	if strings.HasPrefix(expanded, "~/") || expanded == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if expanded == "~" {
			return home, nil
		}
		expanded = filepath.Join(home, expanded[2:])
	}
	abs, err := filepath.Abs(expanded)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
