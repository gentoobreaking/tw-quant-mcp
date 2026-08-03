package domain

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"tw-quant-mcp/pkg/mcp"
)

// TestExtensibilityAddSituation 驗證 §7 擴充性：新增「第 11 種情境」=
// 新增 1 個 domain 子模組 + 在 pkg/mcp 註冊其 Tool，無需改動既有九域。
// 本測試於 repo 內暫建 probe 子模組並驗證可獨立 build，再註冊 Tool 驗證
// tools/list 組裝；probe 於測試結束即移除，不入產物。
func TestExtensibilityAddSituation(t *testing.T) {
	dir := filepath.Join("zzprobe", "extrapolation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll("zzprobe")

	src := `// Package extrapolation 為§7 擴充性驗證之「第 11 情境」probe 骨架。
package extrapolation

import "errors"

// ErrNotImplemented 表示該情境引擎尚未實作（僅作獨立 build 驗證）。
var ErrNotImplemented = errors.New("extrapolation: 未實作")

// Entrance 為該情境之入口函式（骨架）。
func Entrance() error { return ErrNotImplemented }
`
	if err := os.WriteFile(filepath.Join(dir, "extrapolation.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1) 新 domain 子模組可獨立 build（不需改動既有九子模組）。
	if out, err := exec.Command("go", "build", "./zzprobe/...").CombinedOutput(); err != nil {
		t.Fatalf("新增情境子模組無法獨立 build: %v\n%s", err, out)
	}

	// 2) 僅需在 Tool Registry 登錄該情境之 Tool，tools/list 即組裝完成。
	r := mcp.NewRegistry()
	r.Register(mcp.ToolDef{
		Symbol:      "get_extrapolation_probe",
		Name:        "get_extrapolation_probe",
		Description: "§7 擴充性驗證：第 11 情境之 probe Tool（骨架）",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"symbol": map[string]any{"type": "string"},
			},
		},
		Handler: func(_ *mcp.App, args map[string]any) (mcp.HandlerResult, error) {
			return mcp.HandlerResult{Data: args}, nil
		},
	})
	if _, ok := r.Get("get_extrapolation_probe"); !ok {
		t.Fatal("新情境 Tool 未登錄")
	}
	if !slices.Contains(r.Names(), "get_extrapolation_probe") {
		t.Fatal("tools/list 未包含新情境 Tool（擴充失敗）")
	}
}
