#!/usr/bin/env bash
# gofmt 檢查：列出未格式化檔案，存在則失敗（供 make lint 呼叫）。
set -euo pipefail
cd "$(dirname "$0")/.."

out=$(gofmt -l .)
if [ -n "$out" ]; then
  echo "gofmt 需要格式化:"
  echo "$out"
  exit 1
fi
