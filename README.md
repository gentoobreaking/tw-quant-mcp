# tw-quant-mcp

台灣量化市場資料 MCP Server —— 盤中即時 1 分 K 線引擎 + 盤後行情/籌碼/基本面/期權資料，資料 100% 鎖定 TWSE / TPEx / MOPS / TAIFEX **官方免費來源**。

> ⚠️ **免責聲明**：本專案所有回傳資料僅供研究參考，**不構成投資建議**。使用官方公開免費資料以合理使用為原則，嚴禁以本專案進行高頻抓取。

## 功能特色

- **盤中即時引擎**：MIS 8 秒採樣、15 檔 Watchlist、純記憶體 1 分/5 分 K 線組裝（**零 HTTP**）、VWAP、爆量偵測、當沖資格掃描
- **盤後行情與籌碼**：日 K（含 MA/RSI/MACD 技術指標）、三大法人、融資融券、注意/處置股、權證、外資持股
- **基本面與股利**：財報三表、月營收、五面向健康評分、ESG、公司資料、除權息行事曆、高殖利率篩選
- **期貨與選擇權**：台指期等 11 契約每日 OHLC、歷史回溯（TAIFEX-DL）、Put/Call Ratio、大額交易人、三大法人期權部位
- **資料治理**：Data Lineage（`_lineage`）全程標註、三層快取（L1 Ristretto / L2 SQLite / Single-flight）、請求級 Rate Limit + Jitter + 退避 + 熔斷、圖表親和（`_chart_meta`）

## 安裝

### 需求

- Go 1.26+
- macOS / Linux / Windows（`CGO_ENABLED=0` 純靜態編譯，無 cgo 依賴）

### 建置

```bash
make build          # 產出 bin/tw-quant-mcp（CGO-free 單一執行檔）
make build-release  # 產出 bin/tw-quant-mcp-v1.3.0（帶版本號）
```

### 執行

```bash
./bin/tw-quant-mcp                 # stdio 傳輸（MCP 預設，供 Claude Desktop 等客戶端）
MCP_TRANSPORT=streamable-http ./bin/tw-quant-mcp   # Streamable HTTP 傳輸
```

## 設定（環境變數）

| 變數 | 預設 | 說明 |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` 或 `streamable-http` |
| `MCP_HTTP_ADDR` | `127.0.0.1:8787` | streamable-http 監聽位址 |
| `DATA_DIR` | `~/.tw-quant-mcp/data` | L2 SQLite 快取資料目錄 |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `MCP_SCORING_CONFIG` | 內建 v1 規則 | 五面向評分規則 JSON 檔路徑 |
| `RATE_LIMIT_<HOST>_EVERY` | §4.4 預設表 | 覆寫特定主機請求級間隔（秒） |

## MCP 客戶端設定

### Claude Desktop

`claude_desktop_config.json`（macOS：`~/Library/Application Support/Claude/claude_desktop_config.json`）：

```json
{
  "mcpServers": {
    "tw-quant-mcp": {
      "command": "/absolute/path/to/bin/tw-quant-mcp",
      "args": []
    }
  }
}
```

### OpenClaw

`openclaw.json` 的 `mcp.servers` 區塊（或執行 `openclaw mcp set tw-quant-mcp -- command /absolute/path/to/bin/tw-quant-mcp`）：

```json
{
  "mcp": {
    "servers": {
      "tw-quant-mcp": {
        "command": "/absolute/path/to/bin/tw-quant-mcp",
        "args": []
      }
    }
  }
}
```

### Hermes agent

`~/.hermes/config.yaml` 新增 `mcp_servers` 區塊：

```yaml
mcp_servers:
  tw-quant-mcp:
    command: /absolute/path/to/bin/tw-quant-mcp
    args: []
```

### Pi agent

於專案目錄執行（官方 `pi mcp add` 方式）：

```bash
pi mcp add tw-quant-mcp -- /absolute/path/to/bin/tw-quant-mcp
```

或於 `~/.pi/agent/settings.json` / 專案 `.pi/` 設定檔新增：

```json
{
  "mcpServers": {
    "tw-quant-mcp": {
      "command": "/absolute/path/to/bin/tw-quant-mcp",
      "args": []
    }
  }
}
```

### OpenCode

`~/.config/opencode/opencode.json` 的 `mcp` 欄位（`type: "local"` + `command` 陣列）：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "tw-quant-mcp": {
      "type": "local",
      "command": ["/absolute/path/to/bin/tw-quant-mcp"],
      "enabled": true
    }
  }
}
```

> 提示：以上皆為 stdio 傳輸範例，請將 `/absolute/path/to/bin/tw-quant-mcp` 換成實際執行檔路徑。若需 HTTP 傳輸，可先以 `MCP_TRANSPORT=streamable-http MCP_HTTP_ADDR=127.0.0.1:8787 ./bin/tw-quant-mcp` 啟動服務，再將客戶端指向 `http://127.0.0.1:8787/mcp`（客戶端需支援 streamable-http）。

## 工具清單（37 個，§10）

> 其中 `get_stock_trend_composite`（§9.1）為 v2.1 新增工具，Data Grade 為 `PREVIEW`；
> 其餘 36 個工具自 v1.3 沿用，Data Grade 皆為 `AVAILABLE`。
> 對照表見「v2.1 §9 ↔ v1.3 工具對照」一節。

### A. 盤中即時引擎（6）

| 工具 | 說明 |
| --- | --- |
| `set_active_watchlist` | 設定盤中監控觀察清單（最多 15 檔）；觸發 8 秒快照輪詢，為其餘盤中工具提供記憶體資料 |
| `get_intraday_kline` | 當日盤中 1 分 / 5 分 K 線（純記憶體重採樣，零 HTTP） |
| `get_intraday_quote` | 最新即時報價 + 五檔買賣價量（純記憶體讀取，零 HTTP） |
| `get_intraday_vwap` | 當日累計 VWAP、當日高低點與 Fibonacci 支撐/壓力位（記憶體計算，零 HTTP） |
| `detect_volume_surge` | 偵測近 N 分鐘爆量/急拉訊號（前 20 分鐘均量滑動窗口比對，零 HTTP） |
| `scan_daytrade_eligibility` | 買前風險掃描：當沖資格、注意/處置股風險摘要（名單來自 TWSE-WEB / TPEx 盤後名單） |

### B. 盤後行情與籌碼（9）

| 工具 | 說明 |
| --- | --- |
| `get_stock_daily_quote` | 盤後日收盤報價 + MA20/MA60、RSI14、MACD helper 指標（上市以 TWSE-WEB 日 K 計算；上櫃以 TPEx 收盤行情） |
| `get_stock_daily_kline` | 盤後日/週/月 K 線（TWSE-WEB STOCK_DAY，period/adjust 官方參數；上櫃未接線） |
| `get_market_summary` | 全市場漲跌家數/成交量/漲跌停統計（上市 TWSE-WEB + 上櫃 TPEx 收盤行情） |
| `get_institutional_investors` | 三大法人買賣超（個股 + 市場彙總；15:00 前資料可能未齊全） |
| `get_foreign_industry_holdings` | 外資產業配置（TWSE-API 類股外資持股比率，chart pie） |
| `get_foreign_shareholding_history` | 個股外資及陸資持股歷史（TWSE-WEB MI_QFIIS 逐日快照，T-1 翌日釋出，僅上市） |
| `get_margin_trading` | 盤後融資融券（上市 TWSE-WEB MI_MARGN / 上櫃 TPEx） |
| `get_abnormal_trading` | 異常成交量（注意股）排名（上市 TWSE-WEB notice / 上櫃 TPEx 注意股；top_n 預設 20，最大 100） |
| `get_warrant_activity` | 權證活躍度：成交金額/張數 Top N（TWSE-API；top_n 預設 10，最大 50） |

### C. 重大訊息與風險（2）

| 工具 | 說明 |
| --- | --- |
| `get_major_announcements` | 上市/上櫃重大訊息（MOPS 公開資訊觀測站），支援依日期、股票代號、關鍵字過濾 |
| `get_attention_disposition_stocks` | 注意股/處置股清單（買前風險掃描；結果同步注入 `scan_daytrade_eligibility` 名單） |

### D. 基本面與篩選（7）

| 工具 | 說明 |
| --- | --- |
| `get_financial_statements` | 財報三表（MOPS）：損益表/資產負債表/現金流量表；period 支援 "2026Q1" 或年度 |
| `get_monthly_revenue` | 月營收與成長率（MOPS t187ap05_L，含 YoY/MoM/累計；years 預設 2，上限 10） |
| `get_financial_health_check` | 財務健康五面向評分（獲利/成長/結構/配息/治理，各 0-100，評分規則版本化） |
| `get_valuation_ratios` | 估值指標（PE/PB/殖利率/ROE/每股股利；上市 TWSE-API BWIBBU_ALL + MOPS，上櫃 TPEx） |
| `get_esg_report` | ESG 揭露與公司治理（TWSE OpenAPI 溫室氣體排放 + 公司治理） |
| `get_company_profile` | 公司基本資料（MOPS t187ap03_L：董事長、資本額、上市日期、產業別、發言人等） |
| `screen_stocks` | 價值/成長篩選全市場股票（條件：max_pe / max_pb / min_yield / min_growth；整批快取記憶體計算） |

### E. 股利（3）

| 工具 | 說明 |
| --- | --- |
| `get_dividend_history` | 配息歷史與穩定性：現金/股票股利、連續配息年數、平均每股現金股利、最新殖利率 |
| `get_exdividend_calendar` | 除權除息行事曆（上市 TWT48U_ALL + 上櫃 TPEx 預告；預設今日起 6 個月，L2 持久） |
| `screen_high_yield` | 高殖利率排行（條件：min_yield 預設 3%、min_dividend、max_pe、min_consecutive 連年配息） |

### F. 期貨與選擇權（7）

| 工具 | 說明 |
| --- | --- |
| `get_futures_daily_ohlc` | 期貨契約每日 OHLC（TAIFEX API 最新交易日；契約限白名單 TX/MTX/GTX/G2F/G1F/G9F/E4F/XIF/GXF/T5F） |
| `get_futures_history` | 期貨 OHLC 歷史回溯（TAIFEX-DL 下載頁，跨度 ≤ 366 日，L2 永久快取） |
| `get_put_call_ratio` | 買賣權比（Put/Call Ratio：成交量/未平倉比；支援單日或範圍回溯，多空分界 1.0 由 _chart_meta 標示） |
| `get_large_trader_positions` | 大額交易人未沖銷部位（期貨 + 選擇權合併；前五大/前十大交易人買賣方口數） |
| `get_institutional_futures_positions` | 三大法人期貨部位（自營/投信/外資之多方、空方、未平倉口數與金額） |
| `get_institutional_options_positions` | 三大法人選擇權部位（自營/投信/外資之多方、空方、未平倉口數與金額） |
| `get_institutional_futures_history` | 三大法人期貨部位歷史（TAIFEX-DL 回溯，跨度 ≤ 366 日，L2 永久快取） |

### G. 基礎設施（2）

| 工具 | 說明 |
| --- | --- |
| `get_symbol_list` | 上市/上櫃代碼表（Symbol Registry，來源 TWSE/TPEx 官方清單，24h 快取每日預熱） |
| `get_trading_calendar` | 交易日曆（TWSE 官方開休市表，內嵌 2026 年資料；回傳交易日清單與官方休市日） |

### H. 趨勢研判（v2.1 §9.1 新增，1）

| 工具 | 說明 |
| --- | --- |
| `get_stock_trend_composite` | 短中長期「技術面+基本面+籌碼面」綜合研判（參數 `symbol`、`horizon`=short/mid/long，預設 mid；跨來源聚合 TWSE Web API + TWSE-API + TPEx-API + MOPS，_lineage 為多來源陣列；Data Grade `PREVIEW`） |

## v2.1 §9 ↔ v1.3 工具對照

v2.1 工具目錄（25 工具）與 v1.3 既有工具（36 工具）比對結論：

- **A 組（同名同功能，12 個）**：`get_intraday_kline`、`get_intraday_quote`、`get_stock_daily_kline`、`get_institutional_investors`、`get_foreign_shareholding_history`、`get_margin_trading`、`get_financial_statements`、`get_monthly_revenue`、`get_valuation_ratios`、`get_exdividend_calendar`、`get_futures_history`、`get_put_call_ratio`。零修改。
- **B 組（功能相同、名稱不同，12 個）**：以 v1.3 名稱沿用、不更名不 alias。包括 `get_stock_daily_quote`（v2.1 `get_stock_quote`）、`get_market_summary`（v2.1 `get_market_overview`）、`get_foreign_industry_holdings`（v2.1 `get_foreign_industry_holdings`）、`get_abnormal_trading`（v2.1 `get_volume_anomalies`）、`get_attention_disposition_stocks`（v2.1 `get_risk_flags` 之一）、`screen_stocks`/`screen_high_yield`（v2.1 `screen_value_stocks`/`screen_high_dividend_yield`）、`get_financial_health_check`（v2.1 `get_financial_health_score`）、`get_dividend_history`（v2.1 `get_dividend_history`）、`get_company_profile`（v2.1 `get_company_profile`）、`get_esg_report`（v2.1 `get_esg_report`）、`get_futures_daily_ohlc`/`get_large_trader_positions`/`get_institutional_*`（v2.1 同名）。
- **C 組（v2.1 新增，1 個）**：`get_stock_trend_composite`（§9.1）——本次新增實作。

**Data Grade 註記**：

- v2.1 §9.9 標 `get_risk_flags` 為 `AVAILABLE`；v1.3 以 `scan_daytrade_eligibility` 對應實作（同為注意/處置股風險摘要），本專案維持 `scan_daytrade_eligibility` 名稱並標 `AVAILABLE`。
- v2.1 標 `get_warrant_activity` 為 `NOT_YET_AVAILABLE`（Roadmap）；v1.3 已實作（TWSE-API 權證每日成交 Top N），本專案標 `AVAILABLE`（超前實作）。
- `get_stock_trend_composite` 為 v2.1 §9.1 首發新工具，標 `PREVIEW`（欄位/準確度仍可能調整）。

## v2.1 §14 需求對照表（Traceability）

v2.1 §14 之 **7 項優化需求**、**十大投資情境（§9，25 Tool）** 與本專案實作位置／
驗收測試之逐條核對，見 [docs/TRACEABILITY-v2.1.md](docs/TRACEABILITY-v2.1.md)（T030）。

## 回傳結構（Envelope）

所有工具回傳統一 Envelope（§3.3）：

```json
{
  "data": { "...": "業務資料（Normalized Model）" },
  "_lineage": {
    "source": "TWSE_WEB",
    "source_role": "CANONICAL",
    "freshness": "POST_MARKET",
    "fetched_at": "2026-08-01T16:30:00+08:00",
    "data_date": "2026-08-01",
    "sampling_sec": 0,
    "is_cached": false,
    "cache_age_sec": 86400,
    "latency_ms": 123,
    "grade": "AVAILABLE"
  },
  "_chart_meta": { "recommended_type": "line" },
  "http_calls": 1,
  "disclaimer": "僅供研究參考，不構成投資建議"
}
```

- `_lineage`：來源機構 / 來源角色（CANONICAL/SEMI_OFFICIAL_REALTIME/FALLBACK）/ 新鮮度分級（REALTIME_INTRADAY/POST_MARKET/MONTHLY/QUARTERLY/STALE_FALLBACK）/ 採樣間隔 / 快取狀態 / 資料存活秒數 / 延遲 / 成熟度分級（AVAILABLE/PREVIEW/NOT_YET_AVAILABLE，v2.1 §4）；多來源聚合時 `_lineage` 輸出為 `[]Lineage` 陣列。`derived_from` / `cache_ttl` / `source_url` 僅內部保留（debug/log 模式輸出），正式 JSON 不含
- `_chart_meta`：圖表渲染描述（請求含 `chart=true`，預設 true）
- `http_calls`：本次查詢實際上游 HTTP 請求數（盤中 K 線恆為 0）
- `disclaimer`：附錄 A 法遵免責欄位

## 資料來源（官方唯一，§2）

| ID | 來源 | 內容 |
|---|---|---|
| TWSE-API | openapi.twse.com.tw | 公司治理、ESG、日收盤、外資持股、權證、指數 |
| TWSE-WEB | www.twse.com.tw/exchangeReport/* | 日 K、融資融券、三大法人、收盤行情、注意股 |
| TWSE-MIS | mis.twse.com.tw | 盤中即時 Snapshot（8 秒採樣） |
| TPEx-API | www.tpex.org.tw/openapi | 上櫃日收盤、法人、融資融券、注意/處置股 |
| MOPS | mops.twse.com.tw | 月營收、財報三表、重大訊息、公司資料 |
| TAIFEX-API | openapi.taifex.com.tw | 期貨/選擇權行情、PCR、大額交易人、保證金（最新交易日） |
| TAIFEX-DL | www.taifex.com.tw/cht/3/*DateDown* | 歷史回溯 CSV（T-1 起） |

## 開發

```bash
make test         # 全套件測試（含契約測試、Envelope 一致性、附錄 A 對齊檢查）
make test-race    # race detector 全套件
make test-live    # Live smoke（僅開盤時段 09:00–13:30 執行，非開盤自動 Skip）
make loadtest     # 壓力測試（20 併發，快取命中率 + 延遲分位數）
make fixtures     # 錄製官方 raw response fixtures（-host all -date YYYYMMDD）
make e2e          # 端到端驗證（MCP client 依序呼叫 A→G 代表工具）
make soak         # 4.5h 連續運行測試（需實際交易日開盤時段）
scripts/release_check.sh   # 發布檢查：CGO-free 建置 + tools/list 36 工具
```

## 授權

Apache License 2.0（見 [LICENSE](LICENSE)）。
