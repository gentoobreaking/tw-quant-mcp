// Package domain 僅含模組化邊界測試（§7 邊界規則）：
// pkg/domain/* 子模組之間不得互相 import；共用邏輯應下沉至
// pkg/model / pkg/provider / pkg/cache 或下層引擎。本檔不入產物。
package domain

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestDomainBoundaryNoCrossImport 驗證 §7 邊界規則：任何 pkg/domain 子模組
// 不得 import 其他 pkg/domain 子模組（僅允許「domain 根測試包」除外，其僅含
// 測試檔不進入產物）。
func TestDomainBoundaryNoCrossImport(t *testing.T) {
	out, err := exec.Command("go", "list", "-json", "tw-quant-mcp/pkg/domain/...").CombinedOutput()
	if err != nil {
		t.Fatalf("go list 失敗: %v\n%s", err, out)
	}

	var pkgs []struct {
		ImportPath string
		Imports    []string
	}
	dec := json.NewDecoder(strings.NewReader(string(out)))
	for dec.More() {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&p); err != nil {
			t.Fatalf("go list JSON 解析失敗: %v", err)
		}
		pkgs = append(pkgs, p)
	}

	// 收集所有 domain 子模組 import path（排除含 .test 之測試包）
	isDomain := make(map[string]bool)
	for _, p := range pkgs {
		if p.ImportPath != "tw-quant-mcp/pkg/domain" &&
			!strings.Contains(p.ImportPath, ".test") &&
			strings.HasPrefix(p.ImportPath, "tw-quant-mcp/pkg/domain/") {
			isDomain[p.ImportPath] = true
		}
	}
	if len(pkgs) < 10 {
		t.Fatalf("pkg/domain 應含 9 子模組 + 根測試包，實際 %d 個包: %v", len(pkgs), pkgs)
	}

	for _, p := range pkgs {
		for _, imp := range p.Imports {
			if isDomain[imp] {
				t.Errorf("邊界違反：%s import 了同層子模組 %s（§7 禁止）", p.ImportPath, imp)
			}
		}
	}
}
