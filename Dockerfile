# === Stage 1: Build tw-quant-mcp ===
FROM golang:1.26-alpine3.24 AS mcp-builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=docker" -o /app/bin/tw-quant-mcp ./cmd/mcp-server

# === Stage 2: alpine runtime ===
FROM alpine:3.20

RUN apk add --no-cache ca-certificates curl tzdata \
    && adduser -D -u 10001 appuser

ENV MCP_TRANSPORT=streamable-http
ENV MCP_HTTP_ADDR=0.0.0.0:8000
ENV LOG_LEVEL=INFO
ENV TZ=Asia/Taipei
ENV DATA_DIR=/app/data
ENV SYMBOL_REGISTRY_OVERRIDE=/app/data/manual_overrides.json

WORKDIR /app
COPY --from=mcp-builder /app/bin/tw-quant-mcp /app/bin/tw-quant-mcp

USER appuser

EXPOSE 8000
CMD ["/app/bin/tw-quant-mcp"]
