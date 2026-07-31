// Package mcp 是 MCP Engine Layer（規格書 §6）：
// Tool Registry / Schema 驗證 / Response Envelope（§3.3）統一注入。
// 所有 Tool Handler 一律經 Registry 登錄，不得繞過此層直接操作 *mcp.Server。
package mcp

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"tw-quant-mcp/pkg/model"
)

// ToolDef 為單一 Tool 之登錄定義。Handler 收到已通過 schema 驗證的 args，
// 回傳 HandlerResult（業務資料 + 選用之 lineage 覆寫）；lineage/_chart_meta
// 由 response shaping 階段統一注入，Handler 不得自行偽造。
type ToolDef struct {
	// Symbol 為 §10 工具目錄之唯一識別（如 "set_active_watchlist"）。
	Symbol string
	// Name 為 MCP Tool 名稱（與 Symbol 相同）。
	Name string
	// Description 提供 LLM 之用途說明。
	Description string
	// Schema 為輸入 JSON Schema（2020-12 draft，map 形式）。
	Schema map[string]any
	// ReadOnly 為 true 時輸出 annotations.readOnlyHint=true（唯讀查詢）。
	ReadOnly bool
	// Response 為本工具之預設 lineage（§3.2）。nil 時套用盤中預設
	//（TWSE_MIS / REALTIME_INTRADAY / 8s 採樣）；HandlerResult.Lineage
	// 可欄位級覆寫。
	Response *model.Lineage
	// Handler 執行業務邏輯；args 已通過 schema 驗證。
	Handler func(*App, map[string]any) (HandlerResult, error)
}

// Registry 是 Tool 登錄表（§6 MCP Engine Layer）。
type Registry struct {
	mu     sync.RWMutex
	defs   map[string]*ToolDef
	byName map[string]*ToolDef
}

// NewRegistry 建立空登錄表。
func NewRegistry() *Registry {
	return &Registry{
		defs:   make(map[string]*ToolDef),
		byName: make(map[string]*ToolDef),
	}
}

// Register 登錄一個 Tool；名稱重複或 schema 無法解析時 panic
// （註冊框架屬啟動期組裝，失敗應立即暴露）。
func (r *Registry) Register(def ToolDef) {
	if def.Name == "" {
		panic("mcp: Register 要求非空 Name")
	}
	if def.Handler == nil {
		panic(fmt.Sprintf("mcp: Tool %q 未提供 Handler", def.Name))
	}
	if _, err := compileSchema(def.Schema); err != nil {
		panic(fmt.Sprintf("mcp: Tool %q schema 無法解析: %v", def.Name, err))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, dup := r.byName[def.Name]; dup {
		panic(fmt.Sprintf("mcp: Tool %q 重複登錄", def.Name))
	}
	r.byName[def.Name] = &def
}

// Get 依名稱查詢 Tool；未登錄回傳 (nil, false)。
func (r *Registry) Get(name string) (*ToolDef, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	def, ok := r.byName[name]
	return def, ok
}

// Names 回傳已登錄 Tool 名稱（排序）。
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.byName))
	for n := range r.byName {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Tools 轉為 MCP SDK Tool 清單（供 Server.AddTool / tools/list）。
func (r *Registry) Tools() []*mcp.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*mcp.Tool, 0, len(r.byName))
	for _, name := range r.Names() {
		def := r.byName[name]
		t := &mcp.Tool{
			Name:        def.Name,
			Description: def.Description,
			InputSchema: def.Schema,
		}
		if def.ReadOnly {
			t.Annotations = &mcp.ToolAnnotations{ReadOnlyHint: true}
		}
		out = append(out, t)
	}
	return out
}

// compileSchema 解析 JSON Schema map，失敗回傳錯誤（驗證用）。
func compileSchema(s map[string]any) (*jsonschema.Resolved, error) {
	if s == nil {
		return nil, fmt.Errorf("mcp: schema 為 nil")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var sch jsonschema.Schema
	if err := json.Unmarshal(raw, &sch); err != nil {
		return nil, err
	}
	resolved, err := sch.Resolve(nil)
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// BuildTOML 產出 Claude Code / Roo Code 等 client 可用之 tools.toml
// （工具描述 + 快速跳 JSON Schema，供人工 review 工具行為）。
func (r *Registry) BuildTOML() string {
	var b strings.Builder
	b.WriteString("# tw-quant-mcp tool directory（§10）\n")
	for _, name := range r.Names() {
		def, _ := r.Get(name)
		fmt.Fprintf(&b, "\n[\"%s\"]\n", def.Name)
		fmt.Fprintf(&b, "description = %q\n", def.Description)
		if def.Schema != nil {
			raw, err := json.MarshalIndent(def.Schema, "", "  ")
			if err == nil {
				fmt.Fprintf(&b, "input_schema = '''\n%s\n'''\n", raw)
			}
		}
	}
	return b.String()
}
