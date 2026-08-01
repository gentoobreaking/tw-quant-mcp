BINARY := bin/tw-quant-mcp
VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-race test-live loadtest fixtures lint vet fmt check run clean snapshots snapshots-call snapshots-render

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
	go run ./cmd/fixtures -host all -date $$(date +%Y%m%d)

vet:
	go vet ./...

lint:
	go vet ./...
	@out="$$(gofmt -l .)"; if [ -n "$$out" ]; then echo "gofmt 需要格式化:"; echo "$$out"; exit 1; fi

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

# 發布檢查：CGO-free 建置 + tools/list 36 工具
release-check:
	./scripts/release_check.sh $(VERSION)

# T020：36 工具真實呼叫 + 截圖（一鍵：重建→呼叫→渲染）
# 子流程與參數說明見 scripts/README-snapshots.md
snapshots:
	./scripts/run_all.sh

# 只呼叫 36 工具（真資料源，更新 snapshots/raw/*.json）
snapshots-call:
	./scripts/run_all.sh --call-only

# 只渲染 PNG（用現有 raw JSON，秒級）
snapshots-render:
	./scripts/run_all.sh --render-only
