//go:build e2e

package mcp

// T020 發布驗收：端到端驗證腳本（E2E，-tags=e2e）。
//
// 以 MCP client（in-memory transport）依序呼叫 §10 A→G 每組代表性工具，驗證：
//  1. tools/list 回傳 36 工具且欄位齊全（§10 總數）；
//  2. 每組（A/B/C/D/E/F/G）至少一個代表工具 Call 成功；
//  3. 回傳皆為 §3.3 Envelope 結構（data + _lineage + http_calls）且 JSON 可解析。
//
// 離線執行（fake 資料源），不連網。執行：
//
//	go test -tags=e2e ./pkg/mcp/ -run E2E -v

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// e2eGroupApp 依 §10 分組組裝 App（A 組盤中引擎，其餘 fake 資料源）。
func e2eGroupApp(t *testing.T, group string) *App {
	t.Helper()
	switch group {
	case "A":
		return newTestApp(t) // 測試時鐘 09:30 + 快照，盤中
	default:
		f := newFake(t)
		stubBCEnvelope(f)
		stubDE(f)
		tq := newFakeTAIFEX(t, "2026-07-29")
		stubFG(tq)
		return fgApp(t, f, tq) // 盤後 16:00 + fake 資料源
	}
}

// e2eServer 建立 MCP Server 骨架（同 cmd/mcp-server newServer）。
func e2eServer() *mcp.Server {
	return mcp.NewServer(&mcp.Implementation{
		Name:        "tw-quant-mcp",
		Version:     "e2e",
		Description: "T020 E2E 測試",
	}, nil)
}

// TestE2EAllGroups 依序呼叫 A→G 代表工具，驗證 Envelope 結構正確。
func TestE2EAllGroups(t *testing.T) {
	cases := []struct {
		group string
		name  string
		args  map[string]any
	}{
		{"A", "get_intraday_quote", map[string]any{"symbol": "2330"}},
		{"B", "get_stock_daily_quote", map[string]any{"symbol": "2330"}},
		{"C", "get_attention_disposition_stocks", map[string]any{"market": "tse", "date": "2026-07-30"}},
		{"D", "get_financial_statements", map[string]any{"symbol": "2330", "period": "2026Q1", "statement": "income"}},
		{"E", "get_dividend_history", map[string]any{"symbol": "2330"}},
		{"F", "get_futures_daily_ohlc", map[string]any{"contract": "TX"}},
		{"G", "get_symbol_list", map[string]any{"market": "tse"}},
	}

	for _, tc := range cases {
		t.Run(tc.group+"_"+tc.name, func(t *testing.T) {
			ctx := context.Background()
			srv := e2eServer()
			app := e2eGroupApp(t, tc.group)
			app.Wire(srv)

			clientT, serverT := mcp.NewInMemoryTransports()
			serverSession, err := srv.Connect(ctx, serverT, nil)
			if err != nil {
				t.Fatalf("server Connect 失敗: %v", err)
			}
			defer serverSession.Close()

			client := mcp.NewClient(&mcp.Implementation{Name: "e2e-client", Version: "v1.0.0"}, nil)
			session, err := client.Connect(ctx, clientT, nil)
			if err != nil {
				t.Fatalf("client Connect 失敗: %v", err)
			}
			defer session.Close()

			// tools/list：本工具已註冊
			listRes, err := session.ListTools(ctx, &mcp.ListToolsParams{})
			if err != nil {
				t.Fatalf("ListTools 失敗: %v", err)
			}
			found := false
			for _, tool := range listRes.Tools {
				if tool.Name == tc.name {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("tools/list 缺工具 %s", tc.name)
			}

			// tools/call：代表工具
			res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tc.name, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool %s 失敗: %v", tc.name, err)
			}
			if res.IsError {
				t.Fatalf("CallTool %s 回傳錯誤: %+v", tc.name, res.StructuredContent)
			}
			if res.StructuredContent == nil {
				t.Fatalf("CallTool %s 無回傳內容", tc.name)
			}
			// 驗證 Envelope 結構：data + _lineage + http_calls
			raw, err := json.Marshal(res.StructuredContent)
			if err != nil {
				t.Fatalf("%s StructuredContent 序列化失敗: %v", tc.name, err)
			}
			var env struct {
				Data    json.RawMessage `json:"data"`
				Lineage struct {
					Source     string `json:"source"`
					SourceRole string `json:"source_role"`
					Freshness  string `json:"freshness"`
				} `json:"_lineage"`
				HTTPCalls int `json:"http_calls"`
			}
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("%s 回傳非 JSON Envelope: %v\n%s", tc.name, err, raw)
			}
			if len(env.Data) == 0 {
				t.Errorf("%s Envelope 缺 data", tc.name)
			}
			if env.Lineage.Source == "" || env.Lineage.SourceRole == "" || env.Lineage.Freshness == "" {
				t.Errorf("%s _lineage 缺必填欄位: %+v", tc.name, env.Lineage)
			}
			if !strings.Contains(string(raw), "http_calls") {
				t.Errorf("%s Envelope 缺 http_calls", tc.name)
			}
			t.Logf("%s: lineage source=%s role=%s freshness=%s", tc.name, env.Lineage.Source, env.Lineage.SourceRole, env.Lineage.Freshness)
		})
	}
}

// TestE2EListTools36 端到端確認 tools/list 總數 36（§10）。
func TestE2EListTools36(t *testing.T) {
	ctx := context.Background()
	srv := e2eServer()
	app := e2eGroupApp(t, "B") // 任一 App 皆註冊全部 36 工具
	app.Wire(srv)

	clientT, serverT := mcp.NewInMemoryTransports()
	serverSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server Connect 失敗: %v", err)
	}
	defer serverSession.Close()

	client := mcp.NewClient(&mcp.Implementation{Name: "e2e-client", Version: "v1.0.0"}, nil)
	session, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client Connect 失敗: %v", err)
	}
	defer session.Close()

	res, err := session.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools 失敗: %v", err)
	}
	if len(res.Tools) != 36 {
		t.Fatalf("tools/list 應回傳 36 個工具（§10），實際 %d", len(res.Tools))
	}
	t.Logf("tools/list 36 工具全數註冊正確")
}
