#!/usr/bin/env bash
# 一鍵重跑「全部工具（252+）真實呼叫 + 截圖」測試
# 用法:
#   ./scripts/run_all.sh            # 完整流程（重建→呼叫→渲染）
#   ./scripts/run_all.sh --no-build # 跳過重建，用現有 bin/tw-quant-mcp
#   ./scripts/run_all.sh --call-only # 只呼叫工具（不渲染）
#   ./scripts/run_all.sh --render-only # 只渲染截圖（用現有 raw JSON）
set -euo pipefail
cd "$(dirname "$0")/.."   # 專案根目錄

BUILD=1; CALL=1; RENDER=1
for arg in "$@"; do
  case "$arg" in
    --no-build)   BUILD=0 ;;
    --call-only)  RENDER=0 ;;
    --render-only) BUILD=0; CALL=0 ;;
    *) echo "未知參數: $arg"; exit 1 ;;
  esac
done

echo "══════════════════════════════════════════"
echo " tw-quant-mcp 全部工具真實呼叫 + 截圖"
echo " $(date '+%Y-%m-%d %H:%M %Z')"
echo "══════════════════════════════════════════"

if [ "$BUILD" = 1 ]; then
  echo "▶ 1/3 重建執行檔 (CGO-free)..."
  CGO_ENABLED=0 go build -ldflags "-X main.version=test" -o bin/tw-quant-mcp ./cmd/mcp-server
fi

if [ "$CALL" = 1 ]; then
  echo "▶ 2/3 呼叫全部工具（真實資料源，約 10~20 分鐘）..."
  python3 ./scripts/call_tw_quant_tools.py
fi

if [ "$RENDER" = 1 ]; then
  echo "▶ 3/3 渲染 PNG 截圖..."
  python3 ./scripts/render_tool_snapshots.py
fi

echo ""
echo "✅ 完成。結果位置:"
echo "   JSON: snapshots/raw/*.json"
echo "   PNG : snapshots/*.png"
echo ""
echo "注意:"
echo "  - 週末/假日: A 群 5 個盤中工具會回「非交易時段」錯誤，屬正常"
echo "  - 交易日 09:00-13:30 執行可得到最高成功率（盤中工具即時資料完整）"
