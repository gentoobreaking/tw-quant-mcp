# 36 工具真實呼叫 + 截圖測試 — 操作說明

> 目標：對 tw-quant-mcp 的全部 36 個 MCP 工具做真實呼叫（真資料源、非 mock），
> 儲存 JSON 結果並渲染為 PNG 截圖，作為 v1.3.0 功能驗證紀錄。

## 一鍵執行

```bash
cd ~/Projects/tw-quant-mcp
./scripts/run_all.sh
```

流程（3 步驟，約 2~3 分鐘）：

| 步驟 | 內容 | 產出 |
|---|---|---|
| 1 | CGO-free 重建 `bin/tw-quant-mcp` | bin/tw-quant-mcp |
| 2 | stdio 握手 + 呼叫 36 工具（真實資料源） | `snapshots/raw/<tool>.json` |
| 3 | Chrome headless 渲染 HTML→PNG | `snapshots/<tool>.png` |

### 變體

```bash
./scripts/run_all.sh --no-build     # 用現有執行檔，跳過重建
./scripts/run_all.sh --call-only    # 只呼叫工具（不渲染）
./scripts/run_all.sh --render-only  # 只渲染（用現有 raw JSON，重跑最快）
```

## 底層腳本

| 腳本 | 位置 | 用途 |
|---|---|---|
| `call_tw_quant_tools.py` | `~/.qclaw/workspace/scripts/` | stdio 子程序 + initialize 握手 + 逐一 `tools/call` 36 工具，每工具間隔 1s |
| `one_tool.py` | 同上 | 單工具呼叫（握手+呼叫+存檔），結果寫入 snapshots/raw/，可被渲染 |
| `render_tool_snapshots.py` | 同上 | 讀 raw JSON → HTML（標題/狀態/參數/data 摘要/_lineage/原始 JSON）→ Chrome headless 截圖 |
| `run_all.sh` | `~/Projects/tw-quant-mcp/scripts/` | 串接上面兩者 + 重建 |

## 前置需求

- Go toolchain（`go build`）
- Python 3
- Google Chrome（`/Applications/Google Chrome.app`，渲染腳本使用）
- 網路連線（呼叫真實官方資料源：TWSE / TPEx / MOPS / TAIFEX）

## 執行時機的差異

| 時機 | 預期結果 | 說明 |
|---|---|---|
| **交易日 09:00–13:30** | 36/36 成功 | A 群盤中工具可取得即時資料 |
| **交易日盤後** | 36/36 成功 | 盤中工具回「非交易時段」，其餘 31 工具正常 |
| **週末/假日** | 31 成功 + 5 A 群「非交易時段」錯誤 | 此為**正確行為**，非 bug（如 2026-08-01 週六實測） |

> A 群：set_active_watchlist、get_intraday_kline、get_intraday_quote、
> get_intraday_vwap、detect_volume_surge、scan_daytrade_eligibility

## 已知官方資料特性（非 bug，勿誤判為失敗）

- **MI_QFIIS 外資持股**：官方改版後只回「有異動」清單，2330 等穩定持股不在清單 → 查無資料正常。測試用 1101（台泥）避免空結果。
- **TWSE 注意股 notice 端點**：已停用/改版，連續多日回 0；注意股測試用 `market=otc`（TPEx 正常）。
- **MOPS 損益表摘要 `t187ap14_L`**：只含最新一季已公布公司（非全體）。財報測試用 1232（大統益，Q2 已公布）。
- **MOPS 財報 AJAX 偶發 502**：mopsov 暫時性不穩定，重試即成功。
- **TAIFEX 價差契約**：CHF/QA1 等契約 Open/Close 可為負值（合法價差），契約測試已設 allowNegPrice 豁免。

## 參數調整

各工具呼叫參數集中在 `call_tw_quant_tools.py` 的 `ARGS` dict（頂部）。
若有工具改用其他標的，直接改該 dict 後重跑即可（例：`get_financial_statements` 的 `symbol`）。

## 常見問題

**Q: 渲染出現「無法解析 result」？**
A: raw JSON 格式異常。渲染腳本已相容兩種格式（批次 `response.result` / one_tool `result`），
若仍失敗代表檔案結構不同，需檢查 `snapshots/raw/<tool>.json` 內容。

**Q: 呼叫階段一堆「非法代號」錯誤？**
A: Symbol Registry 未載入完成（啟動 race）。run_all.sh 會先重建執行檔（含同步載入修復），
若仍發生，檢查網路後重跑。

**Q: 想只重跑單一工具？**
A: 用 `one_tool.py`（`~/.qclaw/workspace/scripts/one_tool.py`）：
```bash
python3 one_tool.py get_margin_trading '{"symbol":"2330"}'
python3 one_tool.py get_financial_statements '{"symbol":"1232"}'   # 不傳參數則用 ARGS 表預設
```
結果寫入 snapshots/raw/ 同名檔，之後 `make snapshots-render` 可重新渲染該工具 PNG。

## 歷史紀錄

- 2026-08-01（週六）：首次執行，31 成功 + 5 A 群正確拒絕；commit `ad4b0f9`
