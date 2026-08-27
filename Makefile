BINARY := bin/tw-quant-mcp
VERSION := $(shell cat VERSION 2>/dev/null || echo 2.1.0)
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-race test-live loadtest fixtures lint vet fmt check run clean snapshots snapshots-call snapshots-render snapshots-report release build-release release-check

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/mcp-server

test:
	go test ./...

# T019：含 race detector 之全套件測試（驗收標準）
test-race:
	go test -race ./...

# T019：Live smoke（僅 CI 開盤時段 09:00–13:30 執行；非開盤自動 Skip）
test-live:
	TW_QUANT_LIVE=1 go test -tags=live ./pkg/mcp/ -run LiveSmoke -v

# T019：壓力測試（20 併發同一熱門股，輸出快取命中率與延遲分位數）
loadtest:
	go run ./cmd/loadtest

# T019：fixtures 錄製工具（-host all -date YYYYMMDD 錄製全部）
fixtures:
	go run ./cmd/fixtures -host all -date "$$(date +%Y%m%d)"

vet:
	go vet ./...

lint:
	go vet ./...
	./scripts/check_fmt.sh

fmt:
	gofmt -s -w .

check: lint test

run:
	go run ./cmd/mcp-server

clean:
	rm -rf bin

# === T020 發布 ===

# 單一可執行檔（CGO-free，帶版本號）
build-release:
	CGO_ENABLED=0 go build $(LDFLAGS) -o bin/tw-quant-mcp-v$(VERSION) ./cmd/mcp-server

# 端到端驗證：MCP client 依序呼叫 A→G 代表工具（離線 fake）
e2e:
	go test -tags=e2e ./pkg/mcp/ -run 'TestE2E' -v

# 4.5h 連續運行測試（實際交易日 09:00 前啟動；非開盤自動 Skip）
soak:
	TW_QUANT_SOAK=1 go test -tags=soak ./pkg/mcp/ -run TestSoakContinuousRun -v

# 發布檢查：CGO-free 建置 + initialize 握手 + tools/list 工具數守門（現 252）
release-check:
	./scripts/release_check.sh $(VERSION)

# 發布：先通過 release-check，再打 v<VERSION> tag 並推送（觸發 GitHub Actions 自動發布）
# 前置：工作區須乾淨（已 commit）、VERSION 檔非空白、tag 尚不存在。
release: release-check
	@bash -c 'V="$(VERSION)"; if [ -z "$$V" ]; then echo "✗ VERSION 檔為空或不存在"; exit 1; fi; if ! git diff --quiet || ! git diff --cached --quiet; then echo "✗ 工作區有未提交變更，請先 git commit 再發布"; exit 1; fi; if git rev-parse "v$$V" >/dev/null 2>&1; then echo "✗ tag v$$V 已存在，請先升版 VERSION 檔"; exit 1; fi; git tag "v$$V" && git push origin "v$$V" && echo "✓ 已推送 tag v$$V，GitHub Actions 將自動建置並發布"'

# T020：全部工具（252+）真實呼叫 + 截圖（一鍵：重建→呼叫→渲染）
# 子流程與參數說明見 scripts/README-snapshots.md
snapshots:
	./scripts/run_all.sh

# 只呼叫全部工具（真資料源，更新 snapshots/raw/*.json）
snapshots-call:
	./scripts/run_all.sh --call-only

# 只渲染 PNG（用現有 raw JSON，秒級）
snapshots-render:
	./scripts/run_all.sh --render-only

# 匯出全部工具呼叫結果為 Markdown 報告（snapshots/REPORT.md）
snapshots-report:
	python3 ./scripts/export_snapshots_md.py

# 重新彙出 docs/TOOL_CATALOG.md（真實 tools/list；工具數變動後執行）
catalog:
	./scripts/update_catalog.sh

# 建立三官方目錄 baseline（首次或確認變更後執行）
catalog-snapshot:
	python3 ./scripts/catalog_snapshot.py update

# 檢查三官方目錄是否新增/刪減端點（有變更 exit 1）
catalog-check:
	python3 ./scripts/catalog_snapshot.py check
