package main

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestServerListToolsEmpty 驗證 T001 驗收：tools/list 回傳空清單不報錯。
func TestServerListToolsEmpty(t *testing.T) {
	ctx := context.Background()
	srv := newServer("test")

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
	if len(res.Tools) != 0 {
		t.Fatalf("tools/list 應為空清單，實際回傳 %d 個: %v", len(res.Tools), res.Tools)
	}
}

// TestServerPing 驗證 server 骨架可正常回應協定層請求。
func TestServerPing(t *testing.T) {
	ctx := context.Background()
	srv := newServer("test")

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
