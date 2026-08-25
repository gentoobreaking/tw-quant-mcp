# 官方 OpenAPI 目錄異動監控（Catalog Watch）

> 目的：TWSE / TPEx / TAIFEX 三個官方 OpenAPI 目錄會不定期新增或下架端點。
> 本機制以「baseline 快照 + 週期比對」確保任何改動都會被偵測並通知，
> 作為工具擴充（新任務書）與來源失效標註的觸發來源。

## 監控對象

| ID | Swagger URL | baseline 條數（2026-08-26 建立） |
| --- | --- | --- |
| twse | `https://openapi.twse.com.tw/v1/swagger.json` | 143 |
| tpex | `https://www.tpex.org.tw/openapi/swagger.json` | 225 |
| taifex | `https://openapi.taifex.com.tw/swagger.json` | 135 |

## 運作架構

```text
官方 swagger ──每週一自動抓取──▶ 與 baseline 比對
                                    │
                     無變更 → 結束   │  有變更（+新增 / -刪減）
                                    ▼
                        自動開 GitHub Issue（附差異清單）
                                    │
              ┌─────────────────────┴─────────────────────┐
              ▼                                           ▼
      新增端點：評估立擴充任務書                刪減端點：於對應工具
      （或併入既有任務書端點表）                description/文件標註來源失效
```

## 組成元件

| 元件 | 說明 |
| --- | --- |
| `scripts/catalog_snapshot.py` | 核心腳本：抓取三個 swagger、與 baseline 比對或更新快照 |
| `snapshots/catalogs/<source>_endpoints.txt` | baseline 快照（每行一條路徑，純文字入版控，diff 友善） |
| `.github/workflows/catalog-watch.yml` | 每週一台北時間 11:00 排程執行；有變更時自動開 issue |
| `make catalog-snapshot` / `make catalog-check` | 手動操作入口 |

## 使用方式

### 手動檢查

```bash
make catalog-check       # 有變更 exit 1，列出 +/- 差異；無變更 exit 0
```

輸出範例：

```text
==> 抓取 tpex 目錄…
    ⚠ 偵測到變更（+1 / -1）
      + tpex_new_endpoint_x
      - tpex_removed_endpoint_y

結果：官方目錄有變更或異常——請評估新增工具/移除標註，
確認後執行 make catalog-snapshot 更新 baseline。
```

### 更新 baseline

評估完變更、完成對應處置後：

```bash
make catalog-snapshot    # 抓取現況寫入 snapshots/catalogs/，git commit 一併入庫
```

### 自動排程

`.github/workflows/catalog-watch.yml` 已內建，push 後即生效，無需額外設定：

- **排程**：cron `0 3 * * 1`（每週一 UTC 03:00 = 台北 11:00）
- **通知**：偵測到變更時以 `gh issue create` 開立 issue，body 含完整差異清單
- 也可在 Actions 頁面手動 **Run workflow** 觸發

## 變更後的處置流程

1. **新增端點（`+`）**
   - 判斷價值：是否屬找買點管線／既有上市工具之上櫃對稱？
   - 高價值：建立擴充任務書（參考 `~/tasks/tw-quant-mcp/tasks/T195+` 模式，
     附上游端點與實測取樣指引）
   - 低價值：記錄於本文件的「已知未接端點」即可
2. **刪減端點（`-`）**
   - 以 `grep <path>` 找出引用該端點的工具（`pkg/provider/*.go` 的 paths map）
   - 於該工具 description 加註「官方來源已下架」並回明確 unavailable 錯誤
     （先例：T061 CSR103）
3. **兩者皆完成後**：`make catalog-snapshot` 更新 baseline，隨程式碼 commit

## 已知未接端點（低價值長尾）

截至 2026-08-26 差集調查，339 條未接端點已全數納入 T195–T242 任務書
（見 `~/tasks/tw-quant-mcp/tasks/`）。日後由監控機制發現的新增端點，
於此處追加記錄。

| 發現日期 | 端點 | 處置 |
|---|---|---|
| — | — | — |

## 相關文件

- `docs/TOOL_CATALOG.md`：194 個現行工具完整目錄（`make catalog` 彙出）
- `docs/COMPARISON_TWSEMCPServer.md`：遠端 MCP 對照與覆蓋率調查方法
- `docs/TOOL_COVERAGE_BY_SOURCE.md`:逐源覆蓋分析
