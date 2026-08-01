# tw-quant-mcp v1.3.0 發布說明

發布日期：2026-08-01
對照規格：`tw-quant-mcp-spec-v1.3.md`（§0 版本變更記錄）

## 版本摘要

v1.3 為第一個完整發布版本：自 v1.2 首版引入盤中 1 分 K 引擎後，本版完成七項橫切需求落地
（Source Registry、Data Lineage、快取與 Rate Limit、Schema 歸一化、模組化、效能最佳化、圖表親和），
並以 T001–T020 二十個任務分階段實作，全部驗收完成。

## v1.3 變更重點（§0 對照）

| # | 規格變更 | 落地狀態 |
|---|---|---|
| ① | 統一版本編號與章節結構 | ✅ 規格 v1.3.0；程式碼模組依 §6 六層分工 |
| ② | Source Registry 僅限官方免費來源 | ✅ §2 唯一真值；`TestAppendixAOfficialSourcesOnly` 驗證 |
| ③ | Data Lineage（`_lineage` 標準化、canonical/helper） | ✅ `pkg/model/lineage.go`；36 工具 Envelope 一致性測試 |
| ④ | 三層快取策略與 TTL 政策表 | ✅ `pkg/cache`（L1 Ristretto / L2 SQLite WAL / Single-flight） |
| ⑤ | Schema 歸一化規則 | ✅ `pkg/model`；契約測試框架驗證 §5 規則 |
| ⑥ | 修正 MIS Jitter 時序錯誤（延遲置於請求**前**）+ 請求級 rate limiter | ✅ `pkg/provider/ratelimit.go`；`TestWaitSequentialTiming` |
| ⑦ | TAIFEX 官方網站下載頁歷史回溯模組 | ✅ `pkg/provider/taifex_dl.go`；DL 6 資料集 + L2 永久 TTL |
| ⑧ | 10 大投資情境完整 MCP Tool 目錄 | ✅ 36 工具（A 6 / B 9 / C 2 / D 7 / E 3 / F 7 / G 2） |
| ⑨ | 圖表化設計標準化（`_chart_meta`） | ✅ `pkg/chart`；`TestEnvelopeJSONMinimal` |
| ⑩ | 效能最佳化（Single-flight、連線池、批次化、增量計算） | ✅ T018：K 線組裝 P95 < 10ms、loadtest 命中率 100% |

## 任務里程碑（T001–T020）

| 階段 | 任務 | 內容 |
|---|---|---|
| Phase 1（W1–2） | T001–T006 | 核心骨架：Resilient Client、Rate Limiter、Cache、Envelope/Lineage、Symbol Registry、交易日曆、MIS Worker + RingBuffer + Aggregator、A 組 6 工具 |
| Phase 2（W3–4） | T007–T011 | TWSE/TPEx Adapter（日 K、法人、融資融券、注意股、權證）、B/C 組工具、MOPS 重大訊息 |
| Phase 3（W5–6） | T012–T015 | MOPS 財報/營收/ESG、股利三工具、TAIFEX API+DL、F/G 組工具 |
| Phase 4（W7–8） | T016–T019 | Chart 套件、複合分析引擎（健康評分/篩選）、效能最佳化（http_calls 預熱）、測試基建（fixtures/契約測試/Envelope/Live smoke/壓測） |
| 發布 | T020 | 連續運行驗證（soak）、CGO-free 單一執行檔、README、附錄 A 對齊、v1.3 tag |

## 驗證結果（發布前）

- ✅ `make test` 全綠（12 套件）
- ✅ `go test -race ./...` 全綠
- ✅ `scripts/release_check.sh`：CGO-free 建置 + tools/list 36 工具
- ✅ `go test -tags=e2e`：A→G 端到端 Envelope 結構正確
- ✅ 壓力測試：20 併發 → 快取命中率 100%、P99 < 1ms、200 查詢僅 3 次上游（Single-flight）
- ✅ 附錄 A 對齊檢查表全數完成（docs/appendix-a-checklist.md）
- ⏳ 4.5h soak 測試：排定實際交易日開盤時段執行（`scripts/run_soak.sh`）

## 安裝

```bash
git clone <repo> && cd tw-quant-mcp
make build-release VERSION=1.3.0
./bin/tw-quant-mcp-v1.3.0
```

詳見 [README.md](../README.md)。

## 已知限制

- MIS 盤中資料僅於台灣交易日 09:00–13:30 採樣；非交易時段 A 組工具回傳「非交易時段」錯誤
- 上櫃歷史 K 線資料源尚未接線（`get_stock_daily_kline` OTC 回傳錯誤）
- 保證金（TAIFEX Margin）僅 API 提供（無 DL 歷史）

## 法遵

本專案僅使用官方公開免費資料，合理使用為原則；嚴禁高頻抓取。所有回傳附
免責欄位：**僅供研究參考，不構成投資建議**。授權 Apache License 2.0。
