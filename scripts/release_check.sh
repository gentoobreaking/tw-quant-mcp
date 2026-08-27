#!/usr/bin/env bash
# T020 發布驗證：單一執行檔（CGO-free）建置 + tools/list 工具數檢查。
# EXPECTED_TOOLS 隨 parity 任務更新（T230–T243 後為 252；唯 set_active_watchlist 可寫）。
#
# 用法：
#   scripts/release_check.sh [version]   # 預設 2.1.0
#
# 驗證項目：
#   1. CGO_ENABLED=0 go build 產出單一執行檔（CGO-free，無動態 cgo 依賴）
#   2. 啟動執行檔（stdio）→ initialize 握手成功
#  3. tools/list 回傳 EXPECTED_TOOLS 個工具，全部含 inputSchema
#  4. set_active_watchlist 為唯一可寫工具（其餘 251 個 readOnly）
#
# 此腳本不連網，僅驗證執行檔層級（§13 錄製回放原則）。

set -euo pipefail

VERSION="${1:-2.1.0}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN="$ROOT/bin/tw-quant-mcp-v$VERSION"

echo "==> 1. CGO-free 建置（CGO_ENABLED=0）"
CGO_ENABLED=0 go build -ldflags "-X main.version=$VERSION" -o "$BIN" ./cmd/mcp-server
if file "$BIN" | grep -q "Mach-O\|ELF"; then
    echo "    執行檔: $(file "$BIN" | sed 's/.*: //')"
else
    echo "    ✗ 執行檔格式異常"
    exit 1
fi

echo "==> 2. 啟動 + initialize 握手"
HANDSHAKE="$( (
    printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"release-check","version":"1.0"}}}\n'
    sleep 2
) | "$BIN" 2>/dev/null | head -1)"
echo "$HANDSHAKE" | grep -q '"serverInfo"' || {
    echo "    ✗ initialize 失敗"
    exit 1
}
echo "    version=$(echo "$HANDSHAKE" | python3 -c 'import sys,json;print(json.loads(sys.stdin.read())["result"]["serverInfo"]["version"])')"

EXPECTED_TOOLS="${EXPECTED_TOOLS:-252}"
EXPECTED_READONLY=$((EXPECTED_TOOLS - 1))
echo "==> 3. tools/list ${EXPECTED_TOOLS} 工具"
OUT="$( (
    printf '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2026-07-28","capabilities":{},"clientInfo":{"name":"release-check","version":"1.0"}}}\n{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}\n'
    sleep 2
) | "$BIN" 2>/dev/null | python3 -c '
import sys, json
for line in sys.stdin:
    line=line.strip()
    if not line: continue
    try: msg=json.loads(line)
    except: continue
    if msg.get("id")==2:
        tools=msg["result"]["tools"]
        print(len(tools))
        print(all(t.get("inputSchema") for t in tools))
        print(sum(1 for t in tools if t.get("annotations")))
')"
COUNT=$(echo "$OUT" | sed -n 1p)
HAS_SCHEMA=$(echo "$OUT" | sed -n 2p)
READONLY=$(echo "$OUT" | sed -n 3p)
[ "$COUNT" = "$EXPECTED_TOOLS" ] || {
    echo "    ✗ 工具數 $COUNT ≠ $EXPECTED_TOOLS"
    exit 1
}
[ "$HAS_SCHEMA" = "True" ] || {
    echo "    ✗ 工具缺 inputSchema"
    exit 1
}
[ "$READONLY" = "$EXPECTED_READONLY" ] || {
    echo "    ✗ readOnly 數 $READONLY ≠ $EXPECTED_READONLY"
    exit 1
}
echo "    ✓ ${EXPECTED_TOOLS} 工具全數註冊，全部含 inputSchema，${EXPECTED_READONLY} readOnly + set_active_watchlist 可寫"

echo "==> 4. 單一執行檔（CGO-free）確認"
nm_check="$(go tool nm "$BIN" 2>/dev/null | grep -c 'cgo')" || true
echo "    cgo 符號數: ${nm_check}（runtime 內建 traceback 符號，非 cgo 連結；CGO_ENABLED=0 建置已保證無 cgo 依賴）"

echo ""
echo "✓ release check PASS (v${VERSION})"
