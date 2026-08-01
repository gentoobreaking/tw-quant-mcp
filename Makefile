BINARY := bin/tw-quant-mcp
VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test test-race test-live loadtest fixtures lint vet fmt check run clean

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
