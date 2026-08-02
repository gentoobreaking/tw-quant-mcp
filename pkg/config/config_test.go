package config

import (
	"os"
	"path/filepath"
	"testing"
)

var envKeys = []string{
	"MCP_TRANSPORT", "MCP_HTTP_ADDR", "DATA_DIR", "LOG_LEVEL", "MCP_SCORING_CONFIG",
	"CACHE_L1_MAX_ENTRIES", "CACHE_L1_MAX_MEMORY_MB", "CACHE_L2_SQLITE_PATH", "CACHE_HIT_RATE_TARGET",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range envKeys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗: %v", err)
	}
	if cfg.Transport != TransportStdio {
		t.Errorf("Transport 預設應為 stdio，實際 %q", cfg.Transport)
	}
	if cfg.HTTPAddr != DefaultHTTPAddr {
		t.Errorf("HTTPAddr 預設應為 %q，實際 %q", DefaultHTTPAddr, cfg.HTTPAddr)
	}
	if cfg.LogLevel != DefaultLogLevel {
		t.Errorf("LogLevel 預設應為 %q，實際 %q", DefaultLogLevel, cfg.LogLevel)
	}
	if cfg.DataDir == "" {
		t.Error("DataDir 預設不得為空")
	}
	info, err := os.Stat(cfg.DataDir)
	if err != nil || !info.IsDir() {
		t.Errorf("DATA_DIR %q 應已被建立: %v", cfg.DataDir, err)
	}
	// v2.1 §5.2 快取參數預設值。
	if cfg.L1MaxEntries != DefaultL1MaxEntries {
		t.Errorf("L1MaxEntries 預設應為 %d，實際 %d", DefaultL1MaxEntries, cfg.L1MaxEntries)
	}
	if cfg.L1MaxMemoryMB != DefaultL1MaxMemoryMB {
		t.Errorf("L1MaxMemoryMB 預設應為 %d，實際 %d", DefaultL1MaxMemoryMB, cfg.L1MaxMemoryMB)
	}
	if cfg.CacheHitRateTarget != DefaultCacheHitRateTarget {
		t.Errorf("CacheHitRateTarget 預設應為 %v，實際 %v", DefaultCacheHitRateTarget, cfg.CacheHitRateTarget)
	}
	wantL2, err := filepath.Abs(DefaultL2SQLitePath)
	if err != nil || cfg.L2SQLitePath != wantL2 {
		t.Errorf("L2SQLitePath 預設應為 %q，實際 %q", wantL2, cfg.L2SQLitePath)
	}
	if _, err := os.Stat(filepath.Dir(cfg.L2SQLitePath)); err != nil {
		t.Errorf("L2 SQLite 目錄 %q 應已被建立: %v", filepath.Dir(cfg.L2SQLitePath), err)
	}
}

func TestLoadEnvOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MCP_TRANSPORT", "streamable-http")
	t.Setenv("MCP_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("DATA_DIR", "$HOME/custom-data")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("CACHE_L1_MAX_ENTRIES", "50000")
	t.Setenv("CACHE_L1_MAX_MEMORY_MB", "512")
	t.Setenv("CACHE_L2_SQLITE_PATH", "$HOME/cache-test.db")
	t.Setenv("CACHE_HIT_RATE_TARGET", "0.9")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗: %v", err)
	}
	if cfg.Transport != TransportStreamableHTTP {
		t.Errorf("Transport 應為 streamable-http，實際 %q", cfg.Transport)
	}
	if cfg.HTTPAddr != "127.0.0.1:9000" {
		t.Errorf("HTTPAddr 應為 127.0.0.1:9000，實際 %q", cfg.HTTPAddr)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("LogLevel 應為 debug，實際 %q", cfg.LogLevel)
	}
	want := filepath.Join(home, "custom-data")
	if cfg.DataDir != want {
		t.Errorf("DataDir 應為 %q，實際 %q", want, cfg.DataDir)
	}
	if _, err := os.Stat(cfg.DataDir); err != nil {
		t.Errorf("DATA_DIR 應已被建立: %v", err)
	}
	// v2.1 §5.2 快取參數覆寫。
	if cfg.L1MaxEntries != 50000 {
		t.Errorf("L1MaxEntries 應為 50000，實際 %d", cfg.L1MaxEntries)
	}
	if cfg.L1MaxMemoryMB != 512 {
		t.Errorf("L1MaxMemoryMB 應為 512，實際 %d", cfg.L1MaxMemoryMB)
	}
	if cfg.L2SQLitePath != filepath.Join(home, "cache-test.db") {
		t.Errorf("L2SQLitePath 應為 %q，實際 %q", filepath.Join(home, "cache-test.db"), cfg.L2SQLitePath)
	}
	if cfg.CacheHitRateTarget != 0.9 {
		t.Errorf("CacheHitRateTarget 應為 0.9，實際 %v", cfg.CacheHitRateTarget)
	}
	if _, err := os.Stat(filepath.Dir(cfg.L2SQLitePath)); err != nil {
		t.Errorf("L2 SQLite 目錄應已被建立: %v", err)
	}
}

func TestValidateRejects(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{"不支援的 transport", map[string]string{"MCP_TRANSPORT": "tcp"}},
		{"不支援的 log level", map[string]string{"LOG_LEVEL": "trace"}},
		{"streamable-http 缺位址", map[string]string{
			"MCP_TRANSPORT": "streamable-http", "MCP_HTTP_ADDR": " ",
		}},
		{"DATA_DIR 環境變數未定義", map[string]string{"DATA_DIR": "$NOT_DEFINED_XYZ/data"}},
		{"DATA_DIR 無法建立", map[string]string{"DATA_DIR": "/dev/null/child"}},
		{"CACHE_L1_MAX_ENTRIES 非整數", map[string]string{"CACHE_L1_MAX_ENTRIES": "abc"}},
		{"CACHE_L1_MAX_ENTRIES 非正數", map[string]string{"CACHE_L1_MAX_ENTRIES": "0"}},
		{"CACHE_L1_MAX_MEMORY_MB 非整數", map[string]string{"CACHE_L1_MAX_MEMORY_MB": "2.5"}},
		{"CACHE_HIT_RATE_TARGET 非數字", map[string]string{"CACHE_HIT_RATE_TARGET": "high"}},
		{"CACHE_HIT_RATE_TARGET 超出範圍", map[string]string{"CACHE_HIT_RATE_TARGET": "1.5"}},
		{"CACHE_HIT_RATE_TARGET 為零", map[string]string{"CACHE_HIT_RATE_TARGET": "0"}},
		{"CACHE_L2_SQLITE_PATH 環境變數未定義", map[string]string{"CACHE_L2_SQLITE_PATH": "$NOT_DEFINED_XYZ/cache.db"}},
		{"CACHE_L2_SQLITE_PATH 無法建立", map[string]string{"CACHE_L2_SQLITE_PATH": "/dev/null/child/cache.db"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearEnv(t)
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			if _, err := Load(); err == nil {
				t.Errorf("Load() 應失敗，但成功回傳")
			}
		})
	}
}

// TestScoringDefault：未設定 MCP_SCORING_CONFIG 時回傳 v1 內建規則（T017）。
func TestScoringDefault(t *testing.T) {
	clearEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗: %v", err)
	}
	s, err := cfg.Scoring()
	if err != nil {
		t.Fatalf("Scoring() 不應失敗: %v", err)
	}
	if s.Version != "v1" {
		t.Errorf("預設 scoring_version 應為 v1，實際 %s", s.Version)
	}
	w := s.Weights
	if w.Profit+w.Growth+w.Structure+w.Dividend+w.Governance != 1.0 {
		t.Errorf("權重總和應為 1.0，實際 %v", w)
	}
	if s.GrossMarginMax == 0 || s.DebtRatioMax == 0 || s.ConsecutiveMax == 0 {
		t.Errorf("預設規則門檻不得為 0: %+v", s)
	}
}

// TestScoringFromFile：MCP_SCORING_CONFIG JSON 可覆寫規則（版本/權重/門檻）。
func TestScoringFromFile(t *testing.T) {
	clearEnv(t)
	file := filepath.Join(t.TempDir(), "scoring.json")
	content := `{
		"version": "v2-custom",
		"weights": {"profit": 0.4, "growth": 0.2, "structure": 0.2, "dividend": 0.1, "governance": 0.1},
		"gross_margin_max_pct": 60,
		"consecutive_max": 10
	}`
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatalf("寫入設定檔失敗: %v", err)
	}
	t.Setenv("MCP_SCORING_CONFIG", file)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗: %v", err)
	}
	s, err := cfg.Scoring()
	if err != nil {
		t.Fatalf("Scoring() 不應失敗: %v", err)
	}
	if s.Version != "v2-custom" {
		t.Errorf("version 應為 v2-custom，實際 %s", s.Version)
	}
	if s.Weights.Profit != 0.4 || s.Weights.Governance != 0.1 {
		t.Errorf("權重覆寫失敗: %+v", s.Weights)
	}
	if s.GrossMarginMax != 60 || s.ConsecutiveMax != 10 {
		t.Errorf("門檻覆寫失敗: %+v", s)
	}
	// 未覆寫欄位保留預設
	if s.DebtRatioMax == 0 || s.NetMarginMax == 0 {
		t.Errorf("未覆寫欄位應保留預設: %+v", s)
	}
}

// TestScoringFileError：設定檔不存在/格式錯誤 → 明確錯誤。
func TestScoringFileError(t *testing.T) {
	clearEnv(t)
	t.Setenv("MCP_SCORING_CONFIG", "/nonexistent/scoring.json")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗（檔案延遲讀取）: %v", err)
	}
	if _, err := cfg.Scoring(); err == nil {
		t.Error("設定檔不存在時 Scoring() 應失敗")
	}

	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("寫入失敗: %v", err)
	}
	t.Setenv("MCP_SCORING_CONFIG", bad)
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load() 不應失敗: %v", err)
	}
	if _, err := cfg.Scoring(); err == nil {
		t.Error("JSON 格式錯誤時 Scoring() 應失敗")
	}
}
