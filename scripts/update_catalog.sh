#!/usr/bin/env bash
# 重新彙出 docs/TOOL_CATALOG.md：以 stdio 啟動服務抓真實 tools/list，
# 依名稱排序產生 Markdown 目錄。
#
# 用法：scripts/update_catalog.sh [version]
#   version 預設取自 Makefile 的 VERSION 變數（僅供標題標註）。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
VERSION="${1:-$(grep -m1 '^VERSION' "$ROOT/Makefile" | awk '{print $NF}')}"
BIN="$ROOT/bin/tw-quant-mcp"

if [ ! -x "$BIN" ]; then
  echo "==> 執行檔不存在，先建置"
  make -C "$ROOT" build
fi

echo "==> 抓取 tools/list（stdio）"
OUT="$( (
  printf '%s\n' \
    '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"catalog","version":"1"}}}' \
    '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}'
  sleep 3
) |
  "$BIN" 2>/dev/null | python3 -c '
import sys, json
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
    except Exception:
        continue
    if msg.get("id") == 2:
        print(json.dumps(msg["result"]["tools"], ensure_ascii=False))
')"

if [ -z "$OUT" ]; then
  echo "錯誤：未取得 tools/list 輸出" >&2
  exit 1
fi

echo "==> 彙出 docs/TOOL_CATALOG.md"
cd "$ROOT"
python3 scripts/gen_catalog.py "$VERSION" "$OUT" >docs/TOOL_CATALOG.md
echo "完成：docs/TOOL_CATALOG.md"
