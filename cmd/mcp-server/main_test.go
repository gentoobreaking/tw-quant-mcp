package main

import (
	"context"
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
	if len(res.Tools) != 36 {
		t.Fatalf("tools/list 應回傳 36 個工具（§10.A 6 + B/C 11 + D/E 10 + F/G 9），實際 %d 個", len(res.Tools))
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
