package mcp

import (
	"encoding/json"
	"io"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// wire.go 為 SDK 介面層：將 Core.Call 之 Envelope 轉為 MCP 協定回覆。
// 錯誤一律以 IsError=true 之 TextContent 回傳（SDK 慣例：錯誤放 Content
// 供 LLM 自省）；成功回覆以 StructuredContent 承載結構化 Envelope，
// Content 同時放同一 JSON 文字以相容僅讀文字之 client。

// discard 為預設 logger 輸出（等於 io.Discard）。
var discard = io.Discard

// jsonUnmarshal 為 Args 反序列化（與 encoding/json 同義，測試可覆寫）。
var jsonUnmarshal = json.Unmarshal

// successResult 包裝成功回覆。
func successResult(env interface{}) *mcp.CallToolResult {
	raw, err := json.Marshal(env)
	if err != nil {
		return errorResult(err)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(raw)},
		},
		StructuredContent: env,
	}
}

// errorResult 包裝失敗回覆（IsError=true）。
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: err.Error()},
		},
	}
}
