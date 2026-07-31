// tw-quant-mcp MCP Server 入口。
//
// 遵循 tw-quant-mcp-spec-v1.3 §6 架構：此處僅初始化 MCP Engine Layer，
// 並依 MCP_TRANSPORT 選擇 Stdio 或 Streamable HTTP 傳輸。
// 所有 log 一律寫入 stderr，避免污染 stdio 協定。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tw-quant-mcp/pkg/config"
	mcpapp "tw-quant-mcp/pkg/mcp"
)

// version 於 build 時以 -ldflags "-X main.version=..." 覆寫。
var version = "0.1.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("config 載入失敗", "err", err)
		os.Exit(1)
	}

	logger := newLogger(cfg.LogLevel)

	srv := newServer(version)

	// T010：MCP Engine Layer 組裝（Tool Registry / Envelope 注入）並註冊
	// §10.A 盤中工具至 Server。
	app, err := mcpapp.NewApp(cfg, mcpapp.WithAppLogger(logger))
	if err != nil {
		slog.Error("MCP App 初始化失敗", "err", err)
		os.Exit(1)
	}
	app.Wire(srv)

	ctx := context.Background()
	switch cfg.Transport {
	case config.TransportStdio:
		logger.Info("啟動 tw-quant-mcp", "transport", "stdio", "version", version)
		if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
			// 用戶端關閉 stdin 屬正常結束；SDK 以 unexported 錯誤
			// "server is closing: EOF" 表達（見 go-sdk internal/jsonrpc2）。
			if isExpectedStdioExit(err) {
				logger.Info("MCP 用戶端已斷線，正常結束")
				return
			}
			logger.Error("stdio server 異常結束", "err", err)
			os.Exit(1)
		}
	case config.TransportStreamableHTTP:
		handler := mcp.NewStreamableHTTPHandler(
			func(*http.Request) *mcp.Server { return srv },
			&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
		)
		httpSrv := &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		logger.Info("啟動 tw-quant-mcp", "transport", "streamable-http", "addr", cfg.HTTPAddr, "version", version)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server 異常結束", "err", err)
			os.Exit(1)
		}
	}
}

// newServer 建立 MCP Server 骨架；工具目錄依規格書 §10 由
// pkg/mcp（T010）註冊（見 main 內 app.Wire(srv)）。
func newServer(ver string) *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:        "tw-quant-mcp",
		Version:     ver,
		Description: "台灣量化市場資料 MCP Server：TWSE / TPEx / MOPS / TAIFEX 官方免費資料（僅供研究參考，不構成投資建議）",
	}, nil)
}

// isExpectedStdioExit 判斷 Run 的回傳是否為用戶端斷線之正常結束訊號。
func isExpectedStdioExit(err error) bool {
	return err != nil && strings.Contains(err.Error(), "server is closing: EOF")
}

// newLogger 依 LOG_LEVEL 建立輸出至 stderr 的 slog logger。
func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
