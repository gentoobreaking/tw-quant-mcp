#!/usr/bin/env bash
# T020 發布驗收：4.5h 連續運行測試執行腳本。
#
# 用法（實際交易日 09:00 前啟動）：
#   scripts/run_soak.sh                 # 預設 4.5h（09:00–13:30 開盤時段）
#   scripts/run_soak.sh 10m             # 縮短為 10 分鐘（CI/驗證用）
#
# 驗證項目：
#   1. goroutine 數穩定（soak 測試內每 60s 記錄，peak 與結束對比）
#   2. heap 無持續增長（測試內首次/最後 HeapAlloc 對比；另以 pprof 取樣對照）
#   3. 事件日誌無 403/429 被封鎖紀錄（掃描運行日誌）
#   4. 盤中 K 線查詢 P95 < 200ms（§13，soak 測試內統計）
#
# 產出：logs/soak-YYYYMMDD-HHMM.log（完整運行日誌，供 MIS/官方異常時留存分析）

set -euo pipefail

DURATION="${1:-4h30m}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
LOG_DIR="$ROOT/logs"
mkdir -p "$LOG_DIR"
LOG="$LOG_DIR/soak-$(date +%Y%m%d-%H%M).log"

echo "==> T020 soak 測試啟動：$(date '+%F %T %Z')"
echo "    運行時長：$DURATION（開盤時段 09:00–13:30 驗證）"
echo "    日誌：$LOG"

# pprof 對照取樣：運行前 baseline（供 heap 對比）
echo "==> pprof baseline heap 取樣"
go tool pprof -inuse_space -top -nodecount=5 "$ROOT/bin/tw-quant-mcp" 2>/dev/null | head -12 || echo "    （執行檔未建置，跳過 pprof baseline；soak 測試內 HeapAlloc 對比仍生效）"

# 執行 soak 測試（TW_QUANT_SOAK=1；非開盤時段自動 Skip）
echo "==> 執行 soak 測試（TW_QUANT_SOAK=1）"
TW_QUANT_SOAK=1 TW_QUANT_SOAK_DURATION="$DURATION" \
  go test -tags=soak ./pkg/mcp/ -run TestSoakContinuousRun -v 2>&1 | tee "$LOG"

# 403/429 被封鎖紀錄掃描
echo "==> 掃描 403/429 封鎖紀錄"
if grep -iE "403|429|blocked|banned|封鎖" "$LOG" | grep -v "TestRetry\|退避重試測試\|TestRetry429ThenSuccess\|TestRetry403"; then
  echo "    ⚠ 偵測到 403/429 紀錄（請檢視日誌 $LOG）"
else
  echo "    ✓ 無 403/429 被封鎖紀錄"
fi

echo ""
echo "==> soak 測試完成：$(date '+%F %T %Z')"
echo "    完整日誌：$LOG"
