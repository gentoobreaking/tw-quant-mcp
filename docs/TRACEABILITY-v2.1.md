# v2.1 §14 需求對照表 (Requirements Traceability)

> T030 驗收 #4：依 v2.1 §14 逐條核對 **7 項優化需求**、**10 項投資情境（§9，25 Tool）**
> 與章節對應關係。本表列示「規格需求 → 本專案實作位置 → 驗收測試 → 狀態」。
> 核對基準：`T021–T029` 已交付、`T030` 全量回歸通過（`go test ./...`）。

## 一、7 項優化需求

| # | v2.1 §14 原始優化需求 | 對應章節 | 本專案實作位置 | 驗收測試 | 狀態 |
|---|---|---|---|---|---|
| 1 | 資料來源鎖定免費可信任官方（TWSE/TPEx/MOPS/TAIFEX） | §3、§1 原則 1 | `pkg/provider/`（TWSE-API/Web、TPEx、MOPS、TAIFEX-API/DL 六 Adapter）、`pkg/mcp/fetch.go:20`（資料集登錄） | `pkg/provider/source_test.go`（TestSourceContract*）、`contract_test.go`（golden fixtures 七來源） | ✅ 完成 |
| 2 | 貫徹 Data Lineage | §4、§8 尾註 | `pkg/model/lineage.go`（source_role/freshness/cache_age_sec/grade）、`pkg/mcp/core.go:145`（lineageFor）、37 工具全數標註 Grade（T029） | `pkg/mcp/app_envelope_test.go`（TestAllToolsEnvelopeConsistent）、`cache_consistency_test.go`（Lineage/Cache 一致性） | ✅ 完成 |
| 3 | 適度快取防範 Rate Limit | §5（雙層快取+Rate Limiter+TTL 矩陣） | `pkg/cache/`（L1 Ristretto + L2 SQLite + stale-if-error）、`pkg/cache/policy.go:61`（TTL 矩陣 §5.2）、TAIFEX L2 永久 TTL（`pkg/provider/taifex_query.go:117`） | `pkg/cache/cache_test.go`（單飛併流/過期/stale）、`fetch_stale_test.go`、`prewarm_test.go`、`stress_test.go`（命中率 ≥ 80%） | ✅ 完成 |
| 4 | 欄位歸一化（Schema Normalization） | §6（六大正規化 Schema）、§2 Normalization Layer | `pkg/model/domain/`（六 Schema）、`pkg/model/normalize/`（FromMIS/FromTWSEWeb 等） | `pkg/model/domain/domain_test.go`（round-trip）、`pkg/model/normalize/normalize_test.go`（golden fixtures）、`pkg/provider/contract_test.go`（Adapters 輸出型別/單位/日期） | ✅ 完成 |
| 5 | 模組化（領域分層） | §7（`pkg/domain/`） | `pkg/domain/*`（screener/hotspot/institutional 等 12 子模組）、模組化邊界規則（§7 尾註：domain 間不互相 import） | `go build ./...`、`pkg/domain/**` 套件測試 | ✅ 完成 |
| 6 | 效能最佳化 | §10（批次端點優先、Bounded Worker Pool、Materialized Index） | 批次端點優先（§12.3 全市場收盤行情單一請求）、`pkg/engine/composite`、`pkg/domain/screener`（Materialized Index `pkg/mcp/index_build_test.go`） | `pkg/mcp/index_build_test.go`、`cmd/loadtest/main.go`（20 併發熱門股，P99=2.7ms）、`stress_test.go` | ✅ 完成 |
| 7 | 資料設計日後簡易圖表化 | §11（通用 `_chart_meta` 五種型別） | `pkg/chart/`（candlestick/line/bar/histogram/heatmap 五型別）、37 工具全數注入 ChartMeta | `pkg/chart/*_test.go`、`app_envelope_test.go`（chart 型別驗證） | ✅ 完成 |
| — | 盤中 1 分 K 即時線型引擎（15 檔 Watchlist、RingBuffer、MIS 防封鎖） | §8（完整保留 v2.0 設計） | `pkg/engine/`（RingStore/Aggregator/IntradayStore）、`pkg/mcp/tools_a.go`（A 組 6 工具） | `pkg/engine/*_test.go`、`app_bc_test.go` 等 A 組測試 | ✅ 完成 |

## 二、十大投資情境 ↔ 25 Tool（v2.1 §9）→ 本專案工具對照

> 對照規則：v2.1 Tool 名稱與本專案既有工具**同名者沿用**；功能相同但名稱不同者
> **不更名不 alias**（v1.3 名稱沿用，見 README「v2.1 §9 ↔ v1.3 工具對照」）。
> Grade 依本專案實作狀態標註（v2.1 §4）。

### 情境 1：個股趨勢研判（§9.1）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_stock_trend_composite` | 短中長期技術/基本面/籌碼綜合 | `symbol`,`horizon` | PREVIEW | `get_stock_trend_composite`（`pkg/mcp/tools_trend.go`，T029 新增） | PREVIEW | ✅ 完成 |

### 情境 2：外資投資解讀（§9.2）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_foreign_holdings` | 個股外資持股 | `symbol` | AVAILABLE | `get_foreign_shareholding_history`（功能涵蓋，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_foreign_industry_flow` | 產業別外資流向 | `industry` | PREVIEW | `get_foreign_industry_holdings`（產業外資持股，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_foreign_flow_history` | 個股外資進出追蹤 | `symbol`,`date_range` | AVAILABLE | `get_foreign_shareholding_history`（含歷史區間） | AVAILABLE | ✅ 完成 |

### 情境 3：市場熱點捕捉（§9.3）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_material_announcements` | 重大訊息公告 | `date` | PREVIEW | `get_major_announcements`（MOPS 重大訊息，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_abnormal_volume_stocks` | 異常成交量偵測 | `date` | PREVIEW | `get_abnormal_trading`（TWSE-API+TPEx，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_warrant_activity` | 權證活躍度監控 | `underlying_symbol` | NOT_YET_AVAILABLE | `get_warrant_activity`（TWSE-API 權證 Top N，超前實作） | AVAILABLE | ✅ 完成 |

### 情境 4：股利投資規劃（§9.4）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `screen_high_dividend_yield` | 高殖利率篩選 | `min_yield_pct`,`top_n` | PREVIEW | `screen_high_yield`（TWSE+TPEx 估值，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_ex_dividend_calendar` | 除權息行事曆 | `date_range` | AVAILABLE | `get_exdividend_calendar`（TWSE+TPEx） | AVAILABLE | ✅ 完成 |
| `get_dividend_stability` | 配息穩定性分析（近 5 年） | `symbol` | PREVIEW | `get_dividend_history`（含近 5 年分派紀錄） | AVAILABLE | ✅ 完成 |

### 情境 5：投資標的篩選（§9.5）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `screen_value_growth_stocks` | 價值股/成長股篩選 | `style`,`criteria` | NOT_YET_AVAILABLE | `screen_stocks`（EPS/ROE 等條件篩選，v1.3 名稱） | AVAILABLE | ✅ 完成 |
| `get_valuation_ratios` | PE/PB/ROE 估值比率 | `symbol` | AVAILABLE | `get_valuation_ratios`（同名） | AVAILABLE | ✅ 完成 |
| `get_esg_risk_assessment` | ESG 風險評估 | `symbol` | PREVIEW | `get_esg_report`（公司治理/ESG 專區，v1.3 名稱） | AVAILABLE | ✅ 完成 |

### 情境 6：期貨籌碼與選擇權分析（§9.6）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_put_call_ratio` | Put/Call Ratio | `date_range` | AVAILABLE | `get_put_call_ratio`（同名，TAIFEX API+DL） | AVAILABLE | ✅ 完成 |
| `get_large_trader_positions` | 大額交易人未沖銷部位 | `product`,`date` | AVAILABLE | `get_large_trader_positions`（同名） | AVAILABLE | ✅ 完成 |
| `get_institutional_derivatives_positions` | 三大法人期貨/選擇權部位 | `product`,`date_range` | AVAILABLE | `get_institutional_futures_positions` / `get_institutional_options_positions`（拆分，v1.3 名稱） | AVAILABLE | ✅ 完成 |

### 情境 7：三大法人籌碼流向（§9.7）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_institutional_investors`（既有） | 上市/上櫃三大法人買賣超 | `symbol_or_market`,`date` | AVAILABLE | `get_institutional_investors`（同名） | AVAILABLE | ✅ 完成 |
| `get_foreign_industry_allocation` | 外資產業配置總覽 | — | PREVIEW | `get_foreign_industry_holdings`（產業外資持股，涵蓋配置總覽） | AVAILABLE | ✅ 完成 |

### 情境 8：個股財報體檢（§9.8）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_financial_health_report` | 獲利/成長/財務結構/配息/治理五面向 | `symbol` | PREVIEW | `get_financial_health_check`（五面向健康度，v1.3 名稱） | AVAILABLE | ✅ 完成 |

### 情境 9：買前風險掃描（§9.9）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_risk_flags` | 處置/注意/當沖限制/停資停券比對 | `symbol` | AVAILABLE | `scan_daytrade_eligibility`（注意/處置風險摘要，v1.3 名稱）＋`get_attention_disposition_stocks` | AVAILABLE | ✅ 完成 |

### 情境 10：期貨/三大法人歷史回溯查詢（§9.10）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `get_futures_ohlc_history` | 期貨每日 OHLC 歷史 | `product`,`date_range` | AVAILABLE | `get_futures_history`（TAIFEX-DL，FALLBACK） | AVAILABLE | ✅ 完成 |
| `get_institutional_derivatives_history` | 三大法人期貨部位歷史 | `product`,`date_range` | AVAILABLE | `get_institutional_futures_history`（TAIFEX-DL，FALLBACK） | AVAILABLE | ✅ 完成 |

### 盤中即時／盤後基礎（§9.11 既有 Tool 維持）

| v2.1 Tool | 說明 | 參數 | Grade(v2.1) | 本專案對應工具 | Grade(本專案) | 狀態 |
|---|---|---|---|---|---|---|
| `set_active_watchlist` | 盤中即時監控清單（≤15 檔） | `symbols` | AVAILABLE | `set_active_watchlist`（同名） | AVAILABLE | ✅ 完成 |
| `get_intraday_kline` | 盤中 1分K/5分K | `symbol`,`timeframe` | AVAILABLE | `get_intraday_kline`（同名） | AVAILABLE | ✅ 完成 |
| `get_stock_daily_quote` | 盤後歷史日K與籌碼 | `symbol`,`date_range` | AVAILABLE | `get_stock_daily_quote`（同名） | AVAILABLE | ✅ 完成 |

> 25 Tool 全數對應完成；本專案共登錄 **37 工具**（25 涵蓋 + 12 個 v1.3 既有工具
> 如 `get_stock_daily_kline`、`get_market_summary`、`get_margin_trading`、
> `get_monthly_revenue`、`get_trading_calendar` 等），無缺漏。

## 三、驗收對應（T030）

| T030 驗收項目 | 對應文件/測試 | 狀態 |
|---|---|---|
| ① 七 Adapter 契約測試（golden fixtures） | `pkg/provider/contract_test.go`＋`testdata/`（TWSE-WEB/API、TPEx、MOPS、TAIFEX-API/DL、MIS） | ✅ `go test ./pkg/provider/` 通過 |
| ② 全量工具 Lineage/Cache/Chart 欄位一致性 | `pkg/mcp/app_envelope_test.go`、`cache_consistency_test.go`、`app_release_test.go` | ✅ 37 工具全覆蓋 |
| ③ 全量工具回歸 | `go test ./...`（含 T021–T029 測試） | ✅ 通過 |
| ④ §14 需求對照表（本文件） | `docs/TRACEABILITY-v2.1.md`＋README 連結 | ✅ 完成 |
| ⑤ 壓測 20 併發熱門股、命中率 ≥ 80% | `cmd/loadtest/main.go`（CI 可執行）、`pkg/mcp/stress_test.go`（`go test` 內建） | ✅ 通過（100% 命中率、Single-flight 併流） |
