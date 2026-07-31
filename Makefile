BINARY := bin/tw-quant-mcp
VERSION ?= 0.1.0
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

.PHONY: build test lint vet fmt check run clean

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/mcp-server

test:
	go test ./...

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
