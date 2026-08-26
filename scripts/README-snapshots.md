# 全部工具真實呼叫 + 截圖測試 — 操作說明

> 目標：對 tw-quant-mcp 的**全部已註冊 MCP 工具（252+，隨 tools/list 動態增長）**
> 做真實呼叫（真資料源、非 mock），儲存 JSON 結果並渲染為 PNG 截圖。
>
> 工具清單與 inputSchema 於執行時自 `tools/list` 動態取得；`call_tw_quant_tools.py`
> 內僅維護「人工調校參數」覆寫表，未列出的工具依 schema 自動產生最小合法參數。

## 一鍵執行

```bash
cd ~/Projects/tw-quant-mcp
./scripts/run_all.sh
```

流程（3 步驟，約 15~25 分鐘）：

| 步驟 | 內容 | 產出 |
| --- | --- | --- |
| 1 | CGO-free 重建 `bin/tw-quant-mcp` | bin/tw-quant-mcp |
| 2 | stdio 握手 + `tools/list` 取得全量清單 → 逐一呼叫全部工具（真實資料源） | `snapshots/raw/<tool>.json` |
| 3 | Chrome headless 渲染 HTML→PNG | `snapshots/<tool>.png` |

### 變體

```bash
./scripts/run_all.sh --no-build     # 用現有執行檔，跳過重建
./scripts/run_all.sh --call-only    # 只呼叫工具（不渲染）
./scripts/run_all.sh --render-only  # 只渲染（用現有 raw JSON，重跑最快）
```

## 參數產生規則

| 優先序 | 來源 | 說明 |
| --- | --- | --- |
| 1 | `ARGS` 覆寫表 | 人工調校（如盤中引擎 watchlist、財報代號、黃金代號等） |
| 2 | schema required 欄位自動填值 | enum→首值、symbol/code→2330、date→最近平日、int→min、bool→false |

選填過濾器一律省略（取得完整資料）；盤中引擎工具會先自動前置呼叫
`set_active_watchlist`。結束時印出成功/失敗統計與失敗清單。

## 底層腳本

| 腳本 | 位置 | 用途 |
| --- | --- | --- |
| `call_tw_quant_tools.py` | `~/Projects/tw-quant-mcp/scripts/` | stdio 子程序 + initialize 握手 + `tools/list` 全量列舉 + 逐一 `tools/call`，每工具間隔 0.5s |
| `one_tool.py` | 同上 | 單工具呼叫（握手+呼叫+存檔），結果寫入 snapshots/raw/，可被渲染 |
| `render_tool_snapshots.py` | 同上 | 讀 raw JSON → HTML（標題/狀態/參數/data 摘要/_lineage/原始 JSON）→ Chrome headless 截圖 |
| `run_all.sh` | `~/Projects/tw-quant-mcp/scripts/` | 串接上面兩者 + 重建 |

## 前置需求

- Go toolchain（`go build`）
- Python 3.11+（stdlib only）
- Chrome/Chromium（僅渲染步驟需要）

## 注意事項

- TPEx OpenAPI 有間歇性 HTTP 520（限流），失敗屬上游暫時性問題，重跑即可
- 交易日 09:00-13:30 執行可得到最高成功率（盤中工具即時資料完整）
- 部分工具回空陣列屬正常（如 upcoming 配息尚不存在、注意股當日為空）
