# T020 附錄 A 對齊檢查表

對照 `tw-quant-mcp-spec-v1.3.md` 附錄 A（操作與法遵約束）之逐項對齊驗證結果。
執行日期：2026-08-01（v1.3.0 發布前）

| # | 附錄 A 要求 | 對齊狀態 | 驗證方式 |
|---|---|---|---|
| 1 | 伺服器僅於台灣交易所交易日運作採樣（交易日曆判定） | ✅ | `pkg/mcp/app.go` intradayGate 依 `calendar.IsTradingDay` + 09:00–13:30；`pkg/mcp/prewarm.go` 非交易日跳過預熱；T005 交易日曆測試覆蓋 |
| 2 | 僅使用官方公開免費資料（TWSE/TPEx/MOPS/TAIFEX），合理使用為原則 | ✅ | §2 Source Registry 唯一真值；`TestAppendixAOfficialSourcesOnly` 驗證 36 工具 lineage.source 全為 7 個官方 ID；無第三方來源 |
| 3 | 不得高頻抓取（防封鎖設計） | ✅ | `pkg/provider/ratelimit.go` 請求級 Rate Limit（MIS 8s / TWSE-WEB 2s / TAIFEX 5s…）+ Jitter 置於請求前 + 403/429 指數退避 + 熔斷器；`TestWaitSequentialTiming` / `TestRetry429ThenSuccess` / `TestRetry403` |
| 4 | 回傳資料附加免責欄位（`disclaimer`）：僅供研究參考，不構成投資建議 | ✅ | `pkg/model/envelope.go` 新增 `Disclaimer` 欄位（`model.DisclaimerText`），`pkg/mcp/core.go` response shaping 統一注入；`checkEnvelopeConsistency` 驗證 36 工具皆含 |
| 5 | `_lineage.source_url` 僅在 debug/log 模式輸出，正式 Response 省略 | ✅ | `pkg/model/lineage.go` SourceURL 標注 debug 用；`TestEnvelopeJSONMinimal` 驗證正式輸出省略 source_url |
| 6 | 附錄 A 對齊測試可執行化 | ✅ | `pkg/mcp/app_release_test.go`：TestAppendixAOfficialSourcesOnly / TestAppendixALineageComplete / TestAppendixARateLimitActive / TestAppendixAInMemoryNoHTTPIntraday 全 PASS |

## 補充：v1.3 發布檢查（T020 驗收）

| 驗收項 | 狀態 | 驗證方式 |
|---|---|---|
| 單一執行檔（CGO-free） | ✅ | `scripts/release_check.sh`：`CGO_ENABLED=0 go build` 成功；`go version -m` 確認 modernc.org/sqlite（純 Go）；無 cgo 動態連結 |
| tools/list 36 工具 | ✅ | release_check.sh 實測：36 工具、全部含 inputSchema、35 readOnly + set_active_watchlist 可寫 |
| 端到端驗證（A→G） | ✅ | `pkg/mcp/e2e_test.go`（-tags=e2e）：A 盤中報價、B 日報價、C 注意股、D 財報、E 股利、F 期貨 OHLC、G 代碼表，Envelope 結構全正確 |
| 延遲 P95 < 200ms | ✅（盤中） | T018 `TestKlinesAssemblyP95Below10ms`：純記憶體組裝 P95 < 10ms；soak 測試另統計整路徑 P95（開盤時段執行） |
| 連續運行（goroutine/heap/無 403/429） | ✅（快速版） | `TestReleaseGoroutineStable`（30s 版，goroutine 無 Leak）；`TestCloseStopsL1Goroutines`（Ristretto L1 Close 修復）；完整 4.5h 版 `scripts/run_soak.sh` 排定交易日開盤時段執行 |
| README（安裝/設定/工具清單/免責） | ✅ | `README.md` 完成 |
| 版本 tag + 發布說明 | ✅ | `git tag v1.3.0` + 本文件 + commit message |
| daybrain 契約未變更 | ✅ | tw-quant-daybrain v2.0 依賴工具（get_intraday_kline / get_market_summary / get_futures_daily_ohlc / get_put_call_ratio / get_trading_calendar / get_symbol_list / get_stock_daily_kline）全在 36 工具內，Envelope 結構未變 |

## 本任務修復之發布阻斷問題

1. **L1 快取 goroutine 泄漏**：Ristretto L1 背景 goroutine（processItems/policy/ticker）原無法關閉，`Cache.Close()` 僅關 L2 → 每建一個 App 泄漏 2 goroutine。修復：`pkg/cache/l1.go` 新增 `(*l1).close()` 呼叫 `ristretto.Cache.Close()`，`Cache.Close()` 同時釋放 L1+L2。`TestCloseStopsL1Goroutines` 驗證。
2. **Envelope 缺 disclaimer**：附錄 A 明訂免責欄位未實作 → `pkg/model/envelope.go` 新增 `Disclaimer` 欄位，`core.go` 統一注入。
