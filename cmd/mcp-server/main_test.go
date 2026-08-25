package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	mcpapp "tw-quant-mcp/pkg/mcp"
)

// TestServerListTools 驗證 T010 驗收：tools/list 回傳 §10.A 之 6 個
// 盤中工具，且每個工具皆含 name/description/inputSchema。
func TestServerListTools(t *testing.T) {
	ctx := context.Background()
	srv := newServer("test")
	app, err := mcpapp.NewApp(nil)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	app.Wire(srv)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server Connect 失敗: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client Connect 失敗: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools 不應失敗: %v", err)
	}
	if len(res.Tools) < 40 {
		t.Fatalf("tools/list 應回傳至少 40 個工具（§10.A 6 + B/C 11 + D/E 10 + F/G 9 + §9.1 trend_composite + get_twse_index + get_etf_nav + get_etf_dividend），實際 %d 個", len(res.Tools))
	}
	seen := map[string]bool{}
	for _, tool := range res.Tools {
		if tool.Name == "" || tool.Description == "" {
			t.Errorf("工具 %+v 缺 name/description", tool.Name)
		}
		if tool.InputSchema == nil {
			t.Errorf("工具 %s 缺 inputSchema", tool.Name)
		}
		seen[tool.Name] = true
	}
	for _, want := range []string{
		"set_active_watchlist",
		"get_intraday_kline",
		"get_intraday_quote",
		"get_intraday_vwap",
		"detect_volume_surge",
		"scan_daytrade_eligibility",
	} {
		if !seen[want] {
			t.Errorf("tools/list 缺工具 %s", want)
		}
	}
}

// TestServerPing 驗證 server 骨架可正常回應協定層請求。
func TestServerPing(t *testing.T) {
	ctx := context.Background()
	srv := newServer("test")
	app, err := mcpapp.NewApp(nil)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	app.Wire(srv)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server Connect 失敗: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client Connect 失敗: %v", err)
	}
	defer session.Close()

	if err := session.Ping(ctx, &mcp.PingParams{}); err != nil {
		t.Fatalf("Ping 不應失敗: %v", err)
	}
}

// TestHealthEndpoint 驗證 /health 健康檢查端點：
// 回 {"status":"healthy"}、Content-Type application/json、狀態碼 200。
func TestHealthEndpoint(t *testing.T) {
	srv := newServer("test")
	app, err := mcpapp.NewApp(nil)
	if err != nil {
		t.Fatalf("NewApp 失敗: %v", err)
	}
	t.Cleanup(func() { app.Close() })
	app.Wire(srv)

	handler := newHTTPHandler(srv)
	ts := httptest.NewServer(handler)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("GET /health 失敗: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health 狀態碼應 200，實際 %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("/health Content-Type 應為 application/json，實際 %q", ct)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("讀取回應本體失敗: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("/health 回應非合法 JSON: %v（body=%q）", err, body)
	}
	if got["status"] != "healthy" {
		t.Errorf("/health 應回 {\"status\":\"healthy\"}，實際 %s", body)
	}

	// MCP 路徑不受影響：POST / 仍由 Streamable Handler 處理
	// （SDK 依 MCP 規範要求 Accept 同時含 application/json 與 text/event-stream）
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/",
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`))
	if err != nil {
		t.Fatalf("建構 MCP 請求失敗: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	mcpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST / (MCP) 失敗: %v", err)
	}
	defer mcpResp.Body.Close()
	if mcpResp.StatusCode != http.StatusOK {
		t.Errorf("MCP initialize 應 200，實際 %d", mcpResp.StatusCode)
	}
}
