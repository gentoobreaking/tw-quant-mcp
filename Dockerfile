# === Stage 1: Build tw-quant-mcp ===
FROM golang:1.26.6-alpine3.24 AS mcp-builder

WORKDIR /app/tw-quant-mcp
COPY tw-quant-mcp/go.mod tw-quant-mcp/go.sum ./
RUN go mod download
COPY tw-quant-mcp/ ./
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=docker" -o /app/bin/tw-quant-mcp ./cmd/mcp-server

# === Stage 2: alpine + Serve Static ===
FROM alpine:3.20

RUN apk add --no-cache ca-certificates

ENV MCP_TRANSPORT=streamable-http
ENV MCP_HTTP_ADDR=0.0.0.0:8000
ENV LOG_LEVEL=INFO
ENV TZ=Asia/Taipei

WORKDIR /app
COPY --from=mcp-builder /app/bin/tw-quant-mcp /app/bin/tw-quant-mcp

EXPOSE 8000
CMD ["/app/bin/tw-quant-mcp"]
