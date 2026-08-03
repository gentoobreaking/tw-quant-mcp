# tw-quant-mcp v2.1.0 發布說明

發布日期：2026-08-03
對照規格：`tw-quant-mcp-spec-v2_1.md`（§0 版本異動摘要）

## 版本摘要

v2.1 在 v1.3（首個完整發布）基礎上完成規格書 §0 之全部 10 項異動落地，並以
T021–T031 十一個任務分階段實作，全數驗收完成。核心變更：Data Lineage 全面化
（source_role / grade / cache_age_sec）、Per-Source Token Bucket 限流與 MIS
jitter 修正、§6 六大領域歸一化 Schema、`pkg/domain` 領域分層、Materialized
Screener Index（§10.3）、ChartMeta 五型別、`get_stock_trend_composite`
（§9.1）新工具，以及 §14 需求對照表。

## v2.1 變更重點（§0 對照）

| # | 規格異動 | 落地狀態 |
|---|---|---|
| 1 | 資料來源盤點與 Source Role 分級 | ✅ `model.SourceRole`（CANONICAL / SEMI_OFFICIAL_REALTIME / FALLBACK）；`TestAppendixAOfficialSourcesOnly` 驗證 |
| 2 | Data Lineage 全面化（新增 `source_role` / `grade` / `cache_age_sec`） | ✅ `pkg/model/lineage.go`；全 37 工具 Envelope 一致性測試 |
| 3 | 快取與 Rate Limit 防護具體化（TTL 矩陣、雙層快取、token bucket） | ✅ `pkg/cache` TTL 矩陣 + `CACHE_*` 參數化 + stale-if-error；`pkg/provider/ratelimit.go` per-source limiter ×7 |
| 4 | 欄位歸一化 Schema（§6 六大領域） | ✅ `pkg/model/domain/`（趨勢綜合／籌碼流向／股利／財報體檢／風險旗標／期貨選擇權） |
| 5 | 模組化：領域分層（§7 `pkg/domain/`） | ✅ `pkg/domain/` 六大分析模組，`TestPkgDomain` 邊界檢查 |
| 6 | MCP Tool 目錄擴充（§9，25 Tool） | ✅ `get_stock_trend_composite`（§9.1，PREVIEW）新增；其餘以 v1.3 名稱對齊（見 README 對照表） |
| 7 | 效能最佳化（§10） | ✅ 批次端點、`errgroup.SetLimit`、Materialized Screener Index（15:00 每日重建） |
| 8 | 圖表親和資料設計（§11，五型別） | ✅ `pkg/chart` line / bar / candlestick / heatmap / table |
| 9 | Roadmap 重整（§13，六 Phase） | ✅ 六 Phase 全數交付，Data Grade 分級對應 |
| 10 | 需求對照表（§14） | ✅ `docs/TRACEABILITY-v2.1.md`：7 需求 + 10 情境 × 25 Tool 逐條核對 |

## 任務里程碑（T021–T031）

| 階段 | 任務 | 內容 |
|---|---|---|
| Schema 收斂 | T021 | Lineage 全面化；`derived_from`/`cache_ttl`/`source_url` 改為僅 debug/log 輸出（見「已知變更」） |
| Schema 收斂 | T022 | §6 六大領域正規化 Schema（`pkg/model/domain`） |
| Schema 收斂 | T023 | Source Role 三值分級（TWSE OpenAPI / Web / MIS / TPEx / MOPS / TAIFEX ×2） |
| 防護強化 | T024 | §5.2 TTL 矩陣 + `CACHE_*` 環境參數 + stale-if-error 回退（STALE_FALLBACK） |
| 防護強化 | T025 | Per-Source Token Bucket（§5.3）+ MIS jitter 修正（延遲置於請求前） |
| 領域分層 | T026 | `pkg/domain/` 六大模組與模組邊界（§7） |
| 效能 | T027 | Materialized Screener Index（§10.3）：15:00 排程重建、快取直查、lineage.freshness 標註 |
| 圖表親和 | T028 | ChartMeta 五型別（line/bar/candlestick/heatmap/table，§11） |
| Tool 目錄 | T029 | `get_stock_trend_composite`（§9.1）新增；全 37 工具 Data Grade 標註 |
| 契約測試 | T030 | Cache 一致性修正（`get_market_summary` cached 旗標、TAIFEX lineage TTL）；§14 需求對照表；壓測 200 查詢命中率 100% |
| 發布 | T031 | 15:00 Index 與 16:45 EOD 併存驗證、README v2.1（架構圖/環境變數表/契約確認）、CGO-free 單一執行檔、v2.1.0 tag |

## 驗證結果（發布前）

- ✅ `make test` 全綠（12 套件）
- ✅ `go test -race ./...` 全綠
- ✅ `scripts/release_check.sh`：CGO-free 建置 + tools/list 37 工具全註冊（36 沿用 + 1 新增），36 readOnly
- ✅ 壓力測試：20 併發 × 200 查詢，快取命中率 100%、上游 3 次（Single-flight）、P50=228µs / P95=1.277ms / P99=4.924ms
- ✅ 15:00 Materialized Index 排程：交易日觸發一次、寫入 L2 索引快照、與 16:45 盤後預熱併存不衝突（`TestPrewarmIndexAndEODCoexist`）
- ✅ `screen_high_yield` 走 materialized index（http_calls=0，`TestScreenHighYieldServesFromIndex`）
- ✅ MIS token bucket + jitter 前置驗證（`TestWaitSequentialTiming`、`TestMISJitterWindowRange`、`TestMISJitterEnvOverride`）
- ✅ 盤中 K 線延遲：記憶體組裝 P95 < 200ms 目標由 soak 測試檢查；loadtest 實測 P95 ≈ 1.5ms
- ⏳ 4.5h soak 連續運行：排定實際交易日 09:00 前執行（`make soak`；非開盤時段自動 Skip）

## daybrain 相依契約確認

與 `tw-quant-daybrain`（v1.1 規格，Client 端）§2.2 契約子集比對（詳見 README「daybrain 相依契約確認」）：

- ✅ 工具名稱：15 個依賴中 12 個存在且未變更；`get_pre_market_quote`/`get_taifex_night`/`get_us_market` 自 v1.3 起即不存在（非本版造成），需 daybrain 側對齊
- ✅ Envelope 結構：`data`/`_lineage`/`_chart_meta` 不變；v2.1 僅新增欄位（向後相容）
- ⚠️ 欄位變更：`cache_ttl` 正式 JSON 不再輸出（T021 決策）——daybrain §3.1 守門規則需改用 `cache_age_sec` + `sampling_sec` 判斷（詳見「已知變更」）

## 已知變更（Breaking）

- `_lineage` 之 `derived_from` / `cache_ttl` / `source_url` 三欄自正式 JSON 移除
  （保留於內部 struct，debug/log 模式經 `DebugJSON` 輸出）。此決策已於 T021
  （2026-08-01）與使用者確認，目的為對外介面收斂；對 daybrain 之影響見上文。

## 已知限制

- MIS 盤中資料僅於台灣交易日 09:00–13:30 採樣；非交易時段 A 組工具回傳「非交易時段」錯誤
- 上櫃歷史 K 線資料源尚未接線（`get_stock_daily_kline` OTC 回傳錯誤）
- `get_stock_trend_composite` 為 PREVIEW（欄位/準確度仍可能調整）

## 安裝

```bash
git clone <repo> && cd tw-quant-mcp
make build-release VERSION=2.1.0
./bin/tw-quant-mcp-v2.1.0
```

詳見 [README.md](../README.md)。

## 版本資訊

- git tag：`v2.1.0`（基於 T030 commit `0346149` + T031 發布修正）
- 對照規格：`tw-quant-mcp-spec-v2_1.md` §0
