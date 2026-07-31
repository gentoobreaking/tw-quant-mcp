package config

import (
	"os"
	"path/filepath"
	"testing"
)

var envKeys = []string{"MCP_TRANSPORT", "MCP_HTTP_ADDR", "DATA_DIR", "LOG_LEVEL"}

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
}

func TestLoadEnvOverrides(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MCP_TRANSPORT", "streamable-http")
	t.Setenv("MCP_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("DATA_DIR", "$HOME/custom-data")
	t.Setenv("LOG_LEVEL", "debug")

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
