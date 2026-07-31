// Package config 提供 tw-quant-mcp 的環境變數設定檔。
//
// 環境變數：
//
//	MCP_TRANSPORT  "stdio"（預設）| "streamable-http"
//	MCP_HTTP_ADDR  streamable-http 的監聽位址（預設 127.0.0.1:8787）
//	DATA_DIR       L2 SQLite 資料目錄（預設 ~/.tw-quant-mcp/data）
//	LOG_LEVEL      debug | info | warn | error（預設 info）
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
)

// Config 是伺服器執行所需的全部設定。
type Config struct {
	Transport Transport
	HTTPAddr  string
	DataDir   string
	LogLevel  string
}

// Load 從環境變數讀取設定並填入預設值。
func Load() (*Config, error) {
	cfg := &Config{
		Transport: TransportStdio,
		HTTPAddr:  DefaultHTTPAddr,
		DataDir:   defaultDataDir(),
		LogLevel:  DefaultLogLevel,
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
	return nil
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
