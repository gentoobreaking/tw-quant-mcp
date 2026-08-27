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
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tw-quant-mcp/pkg/config"
	mcpapp "tw-quant-mcp/pkg/mcp"
)

// version 於 build 時以 -ldflags "-X main.version=..." 覆寫（值取自 repo 根 VERSION 檔）。
var version = "2.2.0" // 與根目錄 VERSION 檔同步（make build 可用 VERSION= 覆寫）

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Symbol Registry 同步載入（上市+上櫃清單，24h TTL 入 L2）：
	// 避免工具首次呼叫與預熱排程之 race（registry 未就緒時 symbolOf
	// 會誤判「非法代號」）。載入失敗僅記錄、不阻斷啟動（L2 既有
	// 快取仍可於下次預熱/呼叫時補上）。
	if loader := app.RegistryLoader(); loader != nil {
		regCtx, regCancel := context.WithTimeout(ctx, 30*time.Second)
		reg, err := loader.Load(regCtx)
		regCancel()
		if err != nil {
			logger.Warn("Symbol Registry 同步載入失敗（沿用既有快取）", "err", err)
		} else if err := app.Symbols().Set(reg.List("")); err != nil {
			logger.Warn("Symbol Registry 寫入失敗", "err", err)
		} else {
			logger.Info("Symbol Registry 已載入", "symbols", app.Symbols().Len())
		}
	}

	// T018：§12.9 預熱排程（08:00 行事曆/代碼表、開盤前 MIS Session、
	// 16:45 當日盤後）。長駐 goroutine 隨 ctx 取消（SIGINT/SIGTERM）
	// 結束；預熱失敗僅記錄、不影響服務啟動。
	go func() {
		if err := mcpapp.NewPrewarmScheduler(app).Run(ctx); err != nil {
			logger.Error("預熱排程結束", "err", err)
		}
	}()

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
		handler := newHTTPHandler(srv)
		httpSrv := &http.Server{
			Addr:              cfg.HTTPAddr,
			Handler:           handler,
			ReadHeaderTimeout: 10 * time.Second,
		}
		logger.Info("啟動 tw-quant-mcp", "transport", "streamable-http", "addr", cfg.HTTPAddr, "version", version)
		errCh := make(chan error, 1)
		go func() { errCh <- httpSrv.ListenAndServe() }()
		select {
		case <-ctx.Done():
			logger.Info("收到中斷訊號，關閉 http server")
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := httpSrv.Shutdown(shutdownCtx); err != nil {
				logger.Error("http server 關閉失敗", "err", err)
			}
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("http server 異常結束", "err", err)
				os.Exit(1)
			}
		}
	}
}

// newHTTPHandler 組裝 streamable-http 之 HTTP 路由：
//   - GET /health：健康檢查（回 {"status":"healthy"}，200），供容器
//     healthcheck 與負載平衡探測；不經 MCP 協定層。
//   - 其餘路徑：交由 MCP Streamable HTTP Handler（JSON-RPC 2.0）。
func newHTTPHandler(srv *mcp.Server) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true},
	)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"healthy"}`))
	})
	mux.Handle("/", mcpHandler)
	return mux
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
