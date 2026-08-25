# tw-quant-mcp

台灣量化市場資料 TWSE MCP Server —— 盤中即時 1 分 K 線引擎 + 盤後行情/籌碼/基本面/期權資料，資料 100% 鎖定 TWSE / TPEx / MOPS / TAIFEX **官方免費來源**。

> ⚠️ **免責聲明**：本專案所有回傳資料僅供研究參考，**不構成投資建議**。使用官方公開免費資料以合理使用為原則，嚴禁以本專案進行高頻抓取。

## 功能特色

- **盤中即時引擎**：MIS 8 秒採樣、15 檔 Watchlist、純記憶體 1 分/5 分 K 線組裝（**零 HTTP**）、VWAP、爆量偵測、當沖資格掃描
- **盤後行情與籌碼**：日 K（含 MA/RSI/MACD 技術指標）、三大法人、融資融券、注意/處置股、權證、外資持股
- **ETF 與指數**：ETF 歷史 NAV + 折溢價（e添富平台，§30.1 L1）、加權指數/寶島/臺灣50 盤後行情與歷史日 K
- **基本面與股利**：財報三表、月營收、五面向健康評分、ESG 揭露八主題（雙來源速度選源＋fallback，T037）、公司資料、除權息行事曆、高殖利率篩選
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

### 健康檢查（streamable-http）

streamable-http 模式下提供 `GET /health` 端點（不經 MCP 協定層），供容器 healthcheck、負載平衡與監控探測：

```bash
curl http://localhost:8787/health
# HTTP 200 {"status":"healthy"}
```

> MCP JSON-RPC 請求走 `http://<addr>/mcp` 或根路徑；客戶端依 MCP 規範需帶
> `Accept: application/json, text/event-stream`。docker-compose 部署之
> healthcheck 即使用本端點。

## 設定（環境變數）

| 變數 | 預設 | 說明 |
|---|---|---|
| `MCP_TRANSPORT` | `stdio` | `stdio` 或 `streamable-http` |
| `MCP_HTTP_ADDR` | `127.0.0.1:8787` | streamable-http 監聽位址 |
| `DATA_DIR` | `~/.tw-quant-mcp/data` | L2 SQLite 快取資料目錄（含 Materialized Index） |
| `LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `MCP_SCORING_CONFIG` | 內建 v1 規則 | 五面向評分規則 JSON 檔路徑 |
| `RATE_LIMIT_<HOST>_EVERY` | §4.4 預設表 | 覆寫特定主機請求級間隔（秒） |
| `RATE_LIMIT_ENABLED` | `true` | 啟用 Per-Source 限流（§5.3） |
| `RATE_LIMIT_BULK_CONCURRENCY` | `8` | 篩選類操作最大併發數（§10） |
| `CACHE_L1_MAX_ENTRIES` | `10000` | Ristretto L1 最大條目數（§5.3） |
| `CACHE_L1_MAX_MEMORY_MB` | `256` | Ristretto L1 最大記憶體（MB，§5.3） |
| `CACHE_L2_SQLITE_PATH` | `<DATA_DIR>/cache.db` | L2 SQLite 檔案路徑（§5.3） |
| `CACHE_HIT_RATE_TARGET` | `0.8` | 監控目標命中率（§10） |
| `MIS_JITTER_MIN_MS` | `7000` | 盤中引擎抖動區間下限（§5.3、§8） |
| `MIS_JITTER_MAX_MS` | `9000` | 盤中引擎抖動區間上限（§5.3、§8） |

> 測試專用：`TW_QUANT_LIVE=1`（live smoke）、`TW_QUANT_SOAK=1`（4.5h 連續運行）、
> `TW_QUANT_SOAK_DURATION=10m`（soak 縮短）。

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

## v2.1 系統架構（§2）

三層架構之上新增 **Domain Analysis Layer**（承載十大投資分析情境）與
**Normalization Layer**（欄位歸一化，§6），使「盤中即時」與「盤後／基本面／
籌碼面」兩種資料節奏在同一套架構下並存：

```text
┌───────────────────────────────────────────────────────────────────┐
│                    MCP Clients / External Program                  │
│        (Claude Desktop, OpenClaw, Hermes, 排程程式, 回測系統)       │
└──────────────────────────────────┬────────────────────────────────┘
                                   │ JSON-RPC（Stdio / Streamable HTTP）
┌──────────────────────────────────▼────────────────────────────────┐
│                     MCP Engine Layer（39 Tool Router）             │
│              Handler Routers + Schema Validation（§9/§10）         │
└──────────────────────────────────┬────────────────────────────────┘
                                   │ Normalized Query
┌──────────────────────────────────▼────────────────────────────────┐
│            Domain Analysis Layer（pkg/domain/，§7 六大模組）        │
│     趨勢綜合 │ 外資解讀 │ 熱點捕捉 │ 股利規劃 │ 標的篩選 │ 期貨選擇權 │
│     ETF NAV/折溢價（§30.1 L1，get_etf_nav）                      │
│     ETF 分配收益（get_etf_dividend，T038）                       │
└──────────────────────────────────┬────────────────────────────────┘
                                   │ Normalized Read
┌──────────────────────────────────▼────────────────────────────────┐
│     Core Infra Services（Rate Limit / Cache / Lineage / 盤中引擎） │
│  • Per-Source Token Bucket ×7 + Jitter（§5.3，MIS 8s±1s）          │
│  • Ristretto L1 + SQLite L2 + Single-flight（TTL 矩陣 §5.2）       │
│  • Intraday 1分K 引擎（8s 採樣 RingBuffer，≤15 檔，§8）            │
│  • Prewarm Scheduler（08:00 / 08:45 / 15:00 Index / 16:45，§12.9） │
└──────────────────────────────────┬────────────────────────────────┘
                                   │ Fetch（Resilient HTTP，整批 §12.4）
┌──────────────────────────────────▼────────────────────────────────┐
│          Normalization Layer（pkg/model/normalize，§6）            │
│       7 種上游格式 → §6 正規化 Schema，並附加 Lineage（§4）        │
└──────────────────────────────────┬────────────────────────────────┘
                                   │ Source-Specific Parsing
┌──────────────────────────────────▼────────────────────────────────┐
│              Official Provider Adapters（官方來源唯一）             │
│ TWSE OpenAPI │ TWSE Web │ MIS Worker │ TPEx │ MOPS │ TAIFEX ×2     │
└───────────────────────────────────────────────────────────────────┘
```

## 工具清單（40 個，§10；⚠️ 已過時——完整目錄見文末附錄）

> 其中 `get_stock_trend_composite`（§9.1）為 v2.1 新增工具，Data Grade 為 `PREVIEW`；
> `get_twse_index`（T032）、`get_etf_nav`（T032-fix，§30.1 L1）、`get_etf_dividend`（T038）為 ETF/指數支援新增工具。
> `get_esg_report`（T037）升級為雙來源（TWSE OpenAPI / MOPS CSV）速度選源＋fallback，涵蓋 t187ap46 八主題。
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

### B. 盤後行情與籌碼（10）

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
| `get_twse_index` | TWSE 指數盤後行情與歷史日 K（加權指數、寶島、臺灣50；TWSE-API MI_INDEX + TWSE-WEB MI_5MINS_HIST） |

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
| `get_esg_report` | ESG 揭露完整報告（T037 雙來源：TWSE OpenAPI / MOPS CSV t187ap46_L_1~8 八主題——溫室氣體排放/再生能源/用水/廢棄物/員工薪資福利/董事會組成/法說會/TCFD，另附公司治理規程；`topics` 參數可過濾；首次呼叫速度選源，快者為主來源、失敗自動 fallback） |
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

### I. ETF（§30.1 L1 / T038 新增，2）

| 工具 | 說明 |
| --- | --- |
| `get_etf_nav` | 上市 ETF 歷史淨值（NAV）與折溢價率序列（TWSE e添富平台 `POST /zh/ETFortune/ajaxEtfInfoChart`，type=fundPric；參數 `symbol`、`start`/`end` 日期範圍；上櫃 ETF 暫無資料源，回傳明確錯誤） |
| `get_etf_dividend` | 上市 ETF 歷史分配收益（配息/收益分配）（TWSE 官方 `GET /rwd/zh/ETF/etfDiv`，參數 `symbol`、`start`/`end` 日期範圍；回傳除息日、基準日、發放日、配息金額、分配標準、公告年度；上櫃 ETF 需確認資料源） |

## v2.1 §9 ↔ v1.3 工具對照

v2.1 工具目錄（25 工具）與 v1.3 既有工具（36 工具）比對結論：

- **A 組（同名同功能，12 個）**：`get_intraday_kline`、`get_intraday_quote`、`get_stock_daily_kline`、`get_institutional_investors`、`get_foreign_shareholding_history`、`get_margin_trading`、`get_financial_statements`、`get_monthly_revenue`、`get_valuation_ratios`、`get_exdividend_calendar`、`get_futures_history`、`get_put_call_ratio`。零修改。
- **B 組（功能相同、名稱不同，12 個）**：以 v1.3 名稱沿用、不更名不 alias。包括 `get_stock_daily_quote`（v2.1 `get_stock_quote`）、`get_market_summary`（v2.1 `get_market_overview`）、`get_foreign_industry_holdings`（v2.1 `get_foreign_industry_holdings`）、`get_abnormal_trading`（v2.1 `get_volume_anomalies`）、`get_attention_disposition_stocks`（v2.1 `get_risk_flags` 之一）、`screen_stocks`/`screen_high_yield`（v2.1 `screen_value_stocks`/`screen_high_dividend_yield`）、`get_financial_health_check`（v2.1 `get_financial_health_score`）、`get_dividend_history`（v2.1 `get_dividend_history`）、`get_company_profile`（v2.1 `get_company_profile`）、`get_esg_report`（v2.1 `get_esg_report`）、`get_futures_daily_ohlc`/`get_large_trader_positions`/`get_institutional_*`（v2.1 同名）。
- **C 組（v2.1 新增，1 個）**：`get_stock_trend_composite`（§9.1）——本次新增實作。
- **ETF/指數（T032/T032-fix/T038 新增，3 個）**：`get_twse_index`（T032，加權指數/寶島/臺灣50 盤後行情與歷史日 K）、`get_etf_nav`（T032-fix，上市 ETF 歷史 NAV + 折溢價，§30.1 L1；e添富平台）、`get_etf_dividend`（T038，上市 ETF 歷史分配收益/配息；TWSE 官方 etfDiv API）。

**Data Grade 註記**：

- v2.1 §9.9 標 `get_risk_flags` 為 `AVAILABLE`；v1.3 以 `scan_daytrade_eligibility` 對應實作（同為注意/處置股風險摘要），本專案維持 `scan_daytrade_eligibility` 名稱並標 `AVAILABLE`。
- v2.1 標 `get_warrant_activity` 為 `NOT_YET_AVAILABLE`（Roadmap）；v1.3 已實作（TWSE-API 權證每日成交 Top N），本專案標 `AVAILABLE`（超前實作）。
- `get_stock_trend_composite` 為 v2.1 §9.1 首發新工具，標 `PREVIEW`（欄位/準確度仍可能調整）。

## daybrain 相依契約確認（v2.1 發布）

`tw-quant-daybrain`（v1.1 規格，Client 端）§2.2 契約子集與本 v2.1 發布比對：

- **工具名稱（15 依賴）**：12 個存在且未變更（`set_active_watchlist`、`get_intraday_vwap`、`detect_volume_surge`、`get_intraday_quote`、`get_intraday_kline`、`get_market_summary`、`get_futures_daily_ohlc`、`get_put_call_ratio`、`get_institutional_investors`、`get_major_announcements`、`get_abnormal_trading`、`get_stock_daily_kline`、`scan_daytrade_eligibility`、`get_trading_calendar`、`get_symbol_list`）。`get_pre_market_quote` / `get_taifex_night` / `get_us_market` 不在本服務工具目錄（v1.3 起即不存在，非 v2.1 造成）——需 daybrain 側對齊，夜盤可用 `get_futures_daily_ohlc` + `get_futures_history` 替代。
- **Envelope 結構**：`data` / `_lineage` / `_chart_meta` 不變；v2.1 新增 `source_role`、`grade`、`cache_age_sec` 欄位（向後相容）。
- **Lineage 欄位變更（T021 決策，2026-08-01 確認）**：`derived_from` / `cache_ttl` / `source_url` 正式 JSON 不再輸出（內部保留，debug/log 模式可輸出）。daybrain §3.1 守門規則之 `cache_ttl ≤ 4s` 檢查需改以 `cache_age_sec`（資料已存活秒）＋`sampling_sec` 判斷，或於 daybrain 端開啟 debug 模式；其餘守門欄位（`freshness` / `fetched_at` / `is_cached` / `sampling_sec` / `source`）皆仍輸出。

**結論**：工具名稱與 Envelope 契約未破壞；唯一變更為 `cache_ttl` 輸出（T021 已確認之決策，於本發布說明中列為已知變更）。

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
| TWSE-API | openapi.twse.com.tw | 公司治理、ESG 揭露（t187ap46_L_1~21，T037 雙來源之一）、日收盤、外資持股、權證、指數 |
| TWSE-WEB | www.twse.com.tw/exchangeReport/* | 日 K、融資融券、三大法人、收盤行情、注意股 |
| TWSE-ETF | www.twse.com.tw/zh/ETFortune/* | ETF 歷史 NAV/折溢價（e添富平台，POST ajaxEtfInfoChart） |
| TWSE-MIS | mis.twse.com.tw | 盤中即時 Snapshot（8 秒採樣） |
| TPEx-API | www.tpex.org.tw/openapi | 上櫃日收盤、法人、融資融券、注意/處置股 |
| MOPS | mops.twse.com.tw | 月營收、財報三表、重大訊息、公司資料、ESG 揭露八主題（t187ap46_L_1~8，T037 雙來源之一） |
| TAIFEX-API | openapi.taifex.com.tw | 期貨/選擇權行情、PCR、大額交易人、保證金（最新交易日） |
| TAIFEX-DL | www.taifex.com.tw/cht/3/*DateDown* | 歷史回溯 CSV（T-1 起） |

**免責聲明（官方來源政策）**：本服務 100% 僅使用上述官方免費來源（§2 Source
Registry），不連線任何第三方行情供應商；所有輸出僅供研究與程式化測試參考，
**不構成投資建議**。使用者應自行核對官方原始資料。

## 開發

```bash
make test         # 全套件測試（含契約測試、Envelope 一致性、附錄 A 對齊檢查）
make test-race    # race detector 全套件
make test-live    # Live smoke（僅開盤時段 09:00–13:30 執行，非開盤自動 Skip）
make loadtest     # 壓力測試（20 併發，快取命中率 + 延遲分位數）
make fixtures     # 錄製官方 raw response fixtures（-host all -date YYYYMMDD）
make e2e          # 端到端驗證（MCP client 依序呼叫 A→G 代表工具）
make soak         # 4.5h 連續運行測試（需實際交易日開盤時段）
scripts/release_check.sh   # 發布檢查：CGO-free 建置 + tools/list 40 工具
```

---
## License

本專案採用 **Apache License 2.0** 授權。

- 完整授權條款見 [`LICENSE`](LICENSE)（專案根目錄）
- Apache-2.0 官方條款：<https://www.apache.org/licenses/LICENSE-2.0>
- 版權與貢獻者資訊以 LICENSE 檔案為準

> 本專案為研究/模擬用途，授權條款不構成任何投資建議或保證；
> 使用/修改/再散佈前請詳閱 LICENSE 全文。

本專案僅供個人量化研究與教育用途。資料來源（FinMind、TWSE、TPEX）之使用請遵守各平台之服務條款。

## 附錄：完整工具目錄（194 個，由 tools/list 自動產生 2026-08-26；T191–T194 遠端獨有全數對齊）

> 本目錄由真實服務 `tools/list` 輸出自動彙出；上方「工具清單（40 個）」章節為
> v2.1 發布時之手寫清單，已過時。各工具之 Envelope、`_lineage`、快取政策與
> 真實呼叫快照見 `snapshots/raw/<tool>.json`。

- `detect_volume_surge`：偵測指定股票近 N 分鐘爆量/急拉訊號（前 20 分鐘均量滑動窗口比對，§8.5 記憶體計算，零 HTTP）。
- `get_abnormal_trading`：查詢異常成交量（注意股）排名（上市 TWSE-WEB notice / 上櫃 TPEx 注意股）。top_n 預設 20，最大 100。
- `get_after_hours_trading`：查詢集中市場盤後定價交易（TWSE-WEB BFT41U，T040）。code 選填（單檔查詢）；limit 預設 50；offset 分頁。
- `get_all_stocks_daily_close`：查詢指定日期全部上市股票收盤行情（開高低收/成交量/本益比；TWSE-WEB MI_INDEX type=ALLBUT0999，T192）。「單一日期 × 全市場」快照；stock_no/name 本地過濾，limit/offset 分頁。
- `get_annual_trading_volume`：查詢各期貨商品年成交量統計（年度總成交量、交易日數、平均日成交量；TAIFEX-API，T041）。contract 省略則回傳全部商品。
- `get_abnormal_accumulated_notice_stocks`：查詢集中市場公布注意累計次數異常資訊（TWSE-API announcement/notetrans，T193）。與當日注意/處置清單互補之累計紀錄；kind 可過濾 stock/warrant（清單含權證）；name/limit/offset。
- `get_attention_disposition_stocks`：查詢注意股/處置股清單（買前風險掃描）。上市：TWSE-WEB notice + TWSE-API punish；上櫃：TPEx 注意/處置。結果同步注入 scan_daytrade_eligibility 名單。
- `get_block_trades_daily`：查詢集中市場鉅額交易日成交量值統計（TWSE-WEB BFIAUU_d，T042）。
- `get_block_trades_detail`：查詢集中市場鉅額交易逐筆明細（含配對交易、盤後鉅額等交易別；TWSE-WEB BFIAUU_d date 查詢，T043）。stock_no/name 為本地端過濾；limit 預設 50。
- `get_block_trades_monthly`：查詢集中市場鉅額交易月成交量值統計（TWSE-WEB BFIAUU_m，T044）。
- `get_block_trades_yearly`：查詢集中市場鉅額交易年成交量值統計（TWSE-WEB BFIAUU_y，T045）。
- `get_broker_basic_info`：查詢證券商基本資料（TWSE-API t187ap18，T046）。可選 name 過濾券商簡稱。
- `get_broker_branch_info`：查詢證券商分公司基本資料（TWSE-API OpenData_BRK02，T047）。可選 name 過濾券商名稱。
- `get_broker_electronic_trading_statistics`：查詢電子式交易統計資訊（TWSE-API t187ap19，T048）。
- `get_broker_gender_statistics`：查詢證券商營業員男女人數統計資料（TWSE-API OpenData_BRK01，T049）。
- `get_broker_headquarters_info`：查詢證券商本公司（總公司）基本資料（TWSE-API brokerService/brokerList，T050）。可選 name 過濾。
- `get_broker_income_expenditure`：查詢證券商損益彙總資料（TWSE-API t187ap21，T051）。可選 name 過濾券商名稱。
- `get_broker_monthly_statements`：查詢證券商月報表資料（TWSE-API t187ap20，T052）。可選 name 過濾券商名稱。
- `get_broker_service_personnel`：查詢證券商從業人員統計資料（TWSE-API t187ap01，T053）。
- `get_brokers_offering_regular_investment`：查詢開辦定期定額業務證券商名單（TWSE-API brokerService/secRegData，T054）。可選 name 過濾。
- `get_central_depository_bond_redemption`：查詢中央登錄公債補息資料表（TWSE-WEB BFI61U，T055）。
- `get_companies_cumulative_voting`：查詢上市公司採累積投票制、全額連記法、候選人提名制選任董監事及當選資料彙總表（TWSE-API t187ap34_L，T056）。可選 name 過濾。
- `get_companies_ownership_changes_business_scope`：查詢上市公司經營權及營業範圍異(變)動專區-經營權異動且營業範圍重大變更停止買賣公司（TWSE-API t187ap26_L，T057）。
- `get_companies_ownership_changes_business_scope_trading`：查詢上市公司經營權及營業範圍異(變)動專區-經營權異動且營業範圍重大變更列為變更交易公司（TWSE-API t187ap27_L，T058）。
- `get_companies_with_anticompetitive_losses`：查詢所有已申報反競爭行為法律訴訟損失的上市公司，排除零值及 N/A（TWSE-API t187ap46_L_20，T059）。
- `get_companies_with_business_scope_changes`：查詢上市公司經營權及營業範圍異(變)動專區-營業範圍重大變更公司（TWSE-API t187ap25_L，T060）。可選 name 過濾。
- `get_companies_with_csr_reports_103`：查詢民國103年應編製及申報企業社會責任報告書之公司（T061）。注意：官方資料源已下架，目前回明確錯誤訊息。
- `get_companies_with_inclusive_finance_data`：查詢所有已申報普惠金融活動的上市公司，排除零值及 N/A（TWSE-API t187ap46_L_17，T062）。
- `get_companies_with_independent_directors`：查詢上市公司獨立董監事兼任情形彙總表（TWSE-API t187ap30_L，T063）。可選 name 過濾。
- `get_companies_with_ownership_changes`：查詢上市公司經營權及營業範圍異(變)動專區-經營權異動公司（TWSE-API t187ap24_L，T064）。可選 name 過濾。
- `get_companies_with_refineries_in_populated_areas`：查詢所有已申報在人口密集區設有煉油廠的上市公司（排除零值及 N/A；TWSE-API ESG t187ap46_L_15，T065）。
- `get_company_anticompetitive_litigation`：根據股票代號查詢上市公司訴訟、非訟與行政爭訟事項資訊（反競爭爭議）。
- `get_company_balance_sheet`：根據股票代號查詢上市公司資產負債表（TWSE-API t187ap07_L，T067）。自動偵測公司所屬產業並使用對應的財務報表格式（一般業、金融業、證券期貨業、金控業、保險業、異業）。
- `get_company_board_info`：根據股票代號查詢上市公司董事會資訊（ESG 揭露 t187ap46_L_6，T068）。
- `get_company_board_insufficient_shares`：查詢上市公司董事、監察人持股不足法定成數彙總表（TWSE-API t187ap08_L，T069）。可選 name 過濾。
- `get_company_board_insufficient_shares_consecutive`：查詢上市公司董事、監察人持股連續不足月數彙總表（TWSE-API t187ap10_L，T070）。
- `get_company_board_pledged_shares`：查詢上市公司董事、監察人質權設定占董事及監察人實際持有股數彙總表（TWSE-API t187ap09_L，T071）。
- `get_company_board_shareholdings`：根據股票代號查詢上市公司董監事持股餘額明細資料（TWSE-API t187ap11_L，T072）。
- `get_company_ceo_dual_role`：查詢上市公司董事長是否兼任總經理資訊彙總表（TWSE-API t187ap33_L，T073）。可選 name 過濾。
- `get_company_climate_management`：根據股票代號查詢上市公司氣候相關財務揭露（TCFD）管理資訊。
- `get_company_community_relations`：根據股票代號查詢上市公司社區關懷與社會服務資訊。
- `get_company_consolidated_director_compensation`：根據股票代號查詢上市公司合併報表董事酬金相關資訊（TWSE-API t187ap29_C_L，T076）。
- `get_company_consolidated_supervisor_compensation`：根據股票代號查詢上市公司合併報表監察人酬金相關資訊（TWSE-API t187ap29_D_L，T077）。
- `get_company_daily_insider_trades_preannounced`：根據股票代號查詢上市公司每日內部人持股轉讓事前申報表-持股轉讓日報表（TWSE-API t187ap12_L，T078）。
- `get_company_daily_insider_trades_untransferred`：根據股票代號查詢上市公司每日內部人持股轉讓事前申報表-持股未轉讓日報表（TWSE-API t187ap13_L，T079）。
- `get_company_director_compensation`：根據股票代號查詢上市公司董事酬金相關資訊（TWSE-API t187ap29_A_L，T080）。
- `get_company_dividend`：根據股票代號查詢上市公司股利分派情形（TWSE-API t187ap45_L 正規化模型，T081）。
- `get_company_energy_management`：根據股票代號查詢上市公司能源管理資訊。
- `get_company_eps_statistics`：根據股票代號查詢上市公司各產業EPS統計資訊（TWSE-API t187ap14_L，T083）。
- `get_company_financial_reports_supervisor_acknowledgment`：根據股票代號查詢上市公司財務報告經監察人承認情形（TWSE-API t187ap31_L，T084）。
- `get_company_food_safety`：根據股票代號查詢上市公司食品安全資訊。
- `get_company_fuel_management`：根據股票代號查詢上市公司燃料管理資訊。
- `get_company_governance_info`：根據股票代號查詢上市公司公司治理資訊（ESG 揭露 t187ap46_L_9，T087）。
- `get_company_governance_regulations`：根據股票代號查詢上市公司公司治理之相關規程規則（TWSE-API t187ap32_L 正規化模型，T088）。
- `get_company_greenhouse_gas_emissions`：根據股票代號查詢上市公司溫室氣體排放資訊。
- `get_company_human_development`：根據股票代號查詢上市公司人力發展資訊。
- `get_company_inclusive_finance`：根據股票代號查詢上市公司普惠金融資訊。
- `get_company_income_statement`：根據股票代號查詢上市公司綜合損益表（TWSE-API t187ap06_L，T092）。自動偵測公司所屬產業並使用對應的財務報表格式（一般業、金融業、證券期貨業、金控業、保險業、異業）。
- `get_company_info_security`：根據股票代號查詢上市公司資通安全管理制度資訊。
- `get_company_information_disclosure_violations`：根據股票代號查詢上市公司資訊揭露違法情形（金管會證期局裁罰/揭露違法，TWSE-API t187ap23_L，T094）。
- `get_company_investor_communications`：根據股票代號查詢上市公司投資人溝通資訊。
- `get_company_major_news`：查詢上市公司每日重大訊息（MOPS t187ap04_L，T096）。code 選填，指定則僅回傳該公司。
- `get_company_major_shareholders`：根據股票代號查詢上市公司持股逾10%大股東名單（TWSE-API t187ap02_L，T097）。
- `get_company_ownership_and_control`：根據股票代號查詢上市公司所有權及控制權資訊。
- `get_company_product_lifecycle`：根據股票代號查詢上市公司產品生命週期資訊。
- `get_company_product_quality_safety`：根據股票代號查詢上市公司產品品質與安全資訊。
- `get_company_profile`：查詢公司基本資料（MOPS t187ap03_L：董事長、資本額、上市日期、產業別、發言人、過戶機構等）。
- `get_company_profitability_analysis`：根據股票代號查詢上市公司營益分析（毛利率/營業利益率/純益率，TWSE-API t187ap17_L，T101）。
- `get_company_profitability_analysis_summary`：查詢上市公司營益分析彙總表（全體公司，支援排序與分頁；TWSE-API t187ap17_L，T102）。order_by 可用欄位：公司代號、公司名稱、年度、季別、營業收入(百萬元)、毛利率(%)(營業毛利)/(營業收入)等比率欄。
- `get_company_quarterly_audit_variance`：根據股票代號查詢上市公司當季綜合損益經會計師查核(核閱)數與當季預測數差異達百分之十以上者(簡式)（TWSE-API t187ap16_L，T103）。
- `get_company_quarterly_earnings_forecast_achievement`：根據股票代號查詢上市公司截至各季綜合損益財測達成情形(簡式)（TWSE-API t187ap15_L，T104）。
- `get_company_risk_management`：根據股票代號查詢上市公司風險管理資訊。
- `get_company_sec_regulatory_penalties`：根據股票代號查詢上市公司金管會證券期貨局裁罰案件專區（TWSE-API t187ap22_L passthrough，T106）。
- `get_company_shareholder_meeting_announcements`：查詢上市公司股東會公告-召集股東常(臨時)會公告資料彙總表(95年度起適用)（TWSE-API t187ap38_L，T107）。可選 name 過濾。
- `get_company_shareholder_meeting_announcements_by_code`：根據股票代號查詢上市公司股東會公告資料（TWSE-API t187ap38_L，T108）。
- `get_company_shareholder_meeting_dates`：查詢上市公司召開股東常(臨時)會日期、地點及採用電子投票情形等資料彙總表（TWSE-API t187ap41_L，T109）。可選 name 過濾。
- `get_company_shareholder_proposal_exercise`：查詢上市公司股東行使提案權情形彙總表（TWSE-API t187ap35_L，T110）。可選 name 過濾。
- `get_company_supervisor_compensation`：根據股票代號查詢上市公司監察人酬金相關資訊（TWSE-API t187ap29_B_L，T111）。
- `get_company_supply_chain_management`：根據股票代號查詢上市公司供應鏈管理資訊。
- `get_company_waste_management`：根據股票代號查詢上市公司廢棄物管理資訊。
- `get_company_water_management`：根據股票代號查詢上市公司水資源管理資訊。
- `get_cross_market_trading_info`：查詢每日上市上櫃跨市場成交資訊（T115）。
- `get_daily_day_trading_targets`：查詢上市股票每日當日沖銷交易標的及統計（T116）。可選 name 過濾。
- `get_daily_futures_market_report`：查詢期貨每日交易行情，包含開高低收、成交量、未平倉量等資訊（TAIFEX-API DailyMarketReportFut，T117）。常用契約代碼：TX（臺指期貨）、MTX（小型臺指）等白名單契約；contract 留空可列出所有可用契約代碼。
- `get_daily_options_market_report`：查詢選擇權每日交易行情，篩選有成交量的履約價資料，按成交量由大到小排序（TAIFEX-API DailyMarketReportOpt，T118）。常用契約代碼：TXO（臺指選擇權）、TEO（電子選擇權）、TFO（金融選擇權）；contract 留空可列出所有可用契約代碼。
- `get_daily_securities_lending_volume`：查詢集中市場借券賣出每日量（T119）。
- `get_dividend_history`：查詢個股配息歷史與穩定性（上市 t187ap45_L 股利分派；上櫃 TPEx 最新年度）。輸出：各股利年度現金/股票股利、連續配息年數、平均每股現金股利、最新殖利率。官方 Open API 僅提供現行年度分派資料，歷史深度有限（note 說明）。
- `get_esg_report`：查詢個股 ESG 揭露完整報告（T037 雙來源：TWSE OpenAPI / MOPS CSV t187ap46_L_1~8 八主題——溫室氣體排放/再生能源/用水/廢棄物/員工薪資福利/董事會組成/法說會/TCFD，另附 t187ap32_L 公司治理規程）。首次呼叫自動速度選源（快者為主來源），主來源失敗自動 fallback。
- `get_etf_dividend`：查詢 ETF 歷史分配收益（配息/收益分配）。資料源：TWSE ETF 分配收益 API（etfDiv）；回傳期間內每筆除息日、基準日、發放日、配息金額、分配標準。僅上市 ETF。start/end 省略時為近 2 年。
- `get_etf_nav`：查詢 ETF 歷史淨值（NAV）與折溢價（spec §30.1 L1）。資料源：TWSE ETF e添富平台（ajaxEtfInfoChart）；回傳期間內逐日 NAV/市價/折溢價率。僅上市 ETF（上櫃 ETF 暫無資料源）。start/end 省略時為近 3 個月。
- `get_etf_regular_investment_ranking`：查詢定期定額交易戶數統計排行月報表（TWSE-WEB ETFReport/ETFRank，T120）。每列含排名、股票與 ETF 之代碼/名稱/交易戶數。可選 code/name 過濾（比對股票欄）。
- `get_exdividend_calendar`：查詢除權除息行事曆（上市 TWT48U_ALL + 上櫃 TPEx 預告；§10.E）。start/end 省略時為今日起 6 個月；事件依日期排序。資料 L2 持久（§4.1/4.2），L1 24h 內重取以納入新公告。
- `get_financial_health_check`：查詢個股財務健康五面向評分（獲利/成長/結構/配息/治理，各 0-100）。評分輸入來自 T014 已快取之官方資料（MOPS 財報/TWSE 估值・股利・ESG/TPEx 估值）；評分規則版本化（scoring_version，config 可調）；輸出為 helper 資料（_lineage.source_role=helper）。
- `get_financial_program_abnormal_recommendations`：查詢投資理財節目異常推介個股（TWSE-WEB Announcement/BFZFZU_T，T121）。可選 name 過濾；無異常推介時官方回「本日無」佔位列。
- `get_financial_statements`：查詢個股財報三表（MOPS）。period 支援 "2026Q1"（或 "2026" 年度，省略時為最新一季）；statement 為 income（損益表摘要+獲利能力）/balance/cashflow，省略時回傳全部。
- `get_first_listed_foreign_stocks_daily`：查詢每日第一上市外國股票成交量值（T122）。可選 name 過濾。
- `get_foreign_companies_applying_for_listing`：查詢外國公司向證交所申請第一上市之公司（TWSE-API company/applylistingForeign，T123）。可選 name 過濾。
- `get_foreign_industry_holdings`：查詢外資產業配置（TWSE-API 類股外資持股比率，chart pie）。date 省略時為最近交易日。
- `get_foreign_shareholding_history`：查詢個股外資及陸資持股歷史（TWSE-WEB MI_QFIIS 逐日快照，T-1 翌日釋出）。僅上市股票。series 由近至遠。
- `get_fund_basic_info`：查詢基金基本資料彙總表（TWSE-API t187ap47_L，T124）。
- `get_futures_daily_history`：查詢期貨每日OHLC歷史行情（可回溯查詢，非僅最新一日；TAIFEX-DL 下載頁回溯，T125）。contract 省略時預設 TX（臺股期貨）；回傳區間內每個交易日、每個到期月份、一般與盤後時段行情。
- `get_futures_daily_ohlc`：查詢期貨契約每日 OHLC（TAIFEX，openapi 最新交易日 hot tier）。date 省略時為最新交易日；回傳該日該契約全到期月份/時段之行情（價格單位：點）。契約代號限白名單（TX/MTX/GTX/G2F/G1F/G9F/E4F/XIF/GXF/T5F）。
- `get_futures_history`：查詢期貨 OHLC 歷史（TAIFEX-DL 下載頁回溯，§9.3；L2 永久快取）。start/end 跨度 ≤ 366 日；回傳依日期/到期月份排序之行情。
- `get_futures_institutional`：查詢三大法人期貨與選擇權每日交易資訊（期貨+選擇權合計；多空交易量/金額、未平倉與契約價值；TAIFEX-API DividedByFuturesAndOptionsBytheDate，T126）。date 省略為最新交易日。
- `get_index_futures_margin`：查詢股價指數類期貨與選擇權保證金一覽表，包含結算保證金、維持保證金、原始保證金（元；TAIFEX-API IndexFuturesAndOptionsMargining，T127）。contract 為中文商品名子字串（如「臺股期貨」），留空顯示全部。
- `get_institutional_fut_opt_split_history`：查詢三大法人期貨與選擇權分計交易歷史（期貨、選擇權並列顯示，可回溯查詢；TAIFEX-DL futAndOptDateDown，T128）。與 get_institutional_total_history（合計）不同，本工具將期貨與選擇權之多空交易口數、契約金額（千元）、未平倉分開列出。區間不可超過 92 日。
- `get_institutional_futures_history`：查詢三大法人期貨部位歷史（TAIFEX-DL 回溯，§9.3；L2 永久快取）。start/end 跨度 ≤ 366 日；回傳依日期/身份別排序之部位。
- `get_institutional_futures_positions`：查詢三大法人期貨部位（自營/投信/外資之多方、空方、未平倉口數與金額，§10.F；TAIFEX API 最新交易日，date 省略為最新交易日）。
- `get_institutional_general`：查詢三大法人（自營商、投信、外資）當日期貨與選擇權市場整體交易總表，包含交易量、交易金額（百萬元）、未平倉口數及契約價值（TAIFEX-API GeneralBytheDate，T129）。date 省略為最新交易日。
- `get_institutional_investors`：查詢三大法人買賣超（個股 + 市場彙總）。上市 TWSE-WEB T86 / 上櫃 TPEx 三大法人明細。15:00 前資料可能未齊全（lineage 註記）。
- `get_institutional_options_positions`：查詢三大法人選擇權部位（自營/投信/外資之多方、空方、未平倉口數與金額，§10.F；TAIFEX API 最新交易日，date 省略為最新交易日）。
- `get_institutional_total_history`：查詢三大法人期貨與選擇權合計總表歷史（可回溯查詢；TAIFEX-DL totalTableDateDown，T130）。與 get_institutional_traders_by_futures_history（僅期貨）不同，本工具為期貨+選擇權合計數字。區間不可超過 92 日。
- `get_institutional_traders_by_futures`：查詢三大法人依各期貨契約分類的交易資料，可觀察各期貨商品的法人買賣情況（TAIFEX-API DetailsOfFuturesContracts，T131）。contract_code 為中文契約名子字串（如「臺股期貨」），留空顯示全部。
- `get_institutional_traders_by_futures_history`：查詢三大法人期貨部位歷史資料（可回溯查詢；TAIFEX-DL futContractsDateDown，T132）。contract 為期貨契約代碼（TXF/MXF/EXF/FXF/TMF 等，預設 TXF），與日行情之 TX/MTX 為不同代碼系統。區間不可超過 92 日。
- `get_institutional_traders_by_options`：查詢三大法人依各選擇權契約分類的交易資料，可觀察各選擇權商品的法人買賣情況（TAIFEX-API DetailsOfOptionsContracts，T133）。contract_code 為中文契約名子字串（如「臺指選擇權」），留空顯示全部。
- `get_institutional_traders_calls_puts`：查詢三大法人選擇權買賣權分計交易資料，分別顯示 CALL 與 PUT 的法人持倉情況（TAIFEX-API DetailsOfCallsAndPuts，T134）。外資偏多時 CALL 淨多單會大幅增加。contract_code 留空顯示全部。
- `get_intraday_kline`：查詢指定股票當日盤中即時 1 分 K / 5 分 K 線（純記憶體重採樣，零 HTTP）。回傳 Candle[]（timestamp/open/high/low/close/volume）+ _chart_meta（candlestick）。
- `get_intraday_quote`：查詢指定股票最新即時報價 + 五檔買賣價量（純記憶體讀取，零 HTTP）。回傳報價欄位與 bids/asks（price/volume）。
- `get_intraday_vwap`：查詢指定股票當日累計 VWAP、當日高低點與 Fibonacci 支撐/壓力位（§8.5 記憶體計算，零 HTTP）。
- `get_large_trader_positions`：查詢大額交易人未沖銷部位（期貨 + 選擇權合併；§10.F）。單日（date，省略為最新交易日）或範圍（start/end）；回傳前五大/前十大交易人買賣方口數與全市場未沖銷部位。
- `get_large_traders_futures_history`：查詢期貨大額交易人未沖銷部位歷史資料（可回溯查詢；TAIFEX-DL largeTraderFutDown，T135）。contract 為必填契約代碼（如 TX、MTX、TE、TF），由本工具取得資料後本地端篩選。區間不可超過 31 日。
- `get_large_traders_futures_oi`：查詢期貨大額交易人（前五大、前十大）未沖銷部位資料，可觀察大戶持倉方向（TAIFEX-API OpenInterestOfLargeTradersFutures，T136）。contract 精確比對契約代碼，預設 TX；留空列出所有可用契約代碼。
- `get_large_traders_options_oi`：查詢選擇權大額交易人（前五大、前十大）未沖銷部位資料，可觀察大戶選擇權布局（TAIFEX-API OpenInterestOfLargeTradersOptions，T137）。contract 精確比對契約代碼，預設 TXO；留空列出所有可用契約代碼。
- `get_local_companies_applying_for_listing`：查詢申請上市之本國公司（TWSE-API company/applylistingLocal，T138）。可選 name 過濾。
- `get_major_announcements`：查詢上市/上櫃重大訊息（MOPS 公開資訊觀測站 Open Data，T012）。支援依日期、股票代號、關鍵字過濾。資料來源：mopsfin.twse.com.tw
- `get_margin_loan_restrictions_announcement`：查詢集中市場停資停券預告表（T139）。可選 name 過濾。
- `get_margin_trading`：查詢個股盤後融資融券（上市 TWSE-WEB MI_MARGN / 上櫃 TPEx 融資融券）。date 省略時為最近交易日。
- `get_margin_trading_info`：查詢信用交易統計（融資融券餘額；TWSE-WEB MI_MARGN tables 型，T140）。
- `get_market_disposal_stocks`：查詢集中市場公布處置股票（TWSE-API announcement/punish 正規化模型，T141）。可選 name 過濾。
- `get_market_historical_index`：查詢加權指數歷史資料（每 5 分鐘軌跡；TWSE-WEB MI_5MINS_HIST，T143）。
- `get_market_holiday_schedule`：查詢有價證券集中交易市場開（休）市日期（TWSE-WEB holidaySchedule，T144）。
- `get_market_index_info`：查詢每日市場各類指數行情明細（TWSE-API MI_INDEX 正規化模型，T145）。可選 name 過濾。
- `get_market_institutional_amounts_history`：查詢外資及陸資/投信/自營商買賣超金額彙總歷史（TWSE-WEB BFI82U，T146）。
- `get_market_summary`：查詢全市場盤後漲跌家數/成交量/漲跌停（上市 TWSE-WEB 收盤行情 + 上櫃 TPEx 收盤行情）。date 省略時為最近交易日。
- `get_market_turnover_history`：查詢集中市場每日成交資訊（含週轉率）歷史（TWSE-WEB FMTQIK，T147）。
- `get_monthly_revenue`：查詢個股月營收與成長率（MOPS t187ap05_L，含 YoY/MoM/累計）。years 指定回傳年數（預設 2，上限 10），列由近至遠。
- `get_monthly_trading_statistics`：查詢期貨市場月統計資料，依商品類別（股價指數、利率、商品、股票）分類，顯示各類型交易人（自營商、投信、外資、散戶等）的買賣量與月底未平倉量（TAIFEX-API MonthlyTradingStatisticsFutures，T148）。
- `get_odd_lot_trading_quotes`：查詢集中市場盤後零股交易行情單（T149）。可選 name 過濾。
- `get_options_daily_history`：查詢選擇權每日OHLC歷史行情（可回溯查詢；TAIFEX-DL dlOptDataDown，T150）。contract 預設 TXO。資料量龐大，建議指定 contract_month（如 202606、202606W1）；未指定且資料量過大時改為列出可用到期月份。區間跨度上限 366 日。
- `get_options_delta`：查詢選擇權每日 Delta 值，了解各履約價的風險敏感度與隱含方向性（TAIFEX-API DailyOptionsDelta，T151）。contract 預設 TXO；contract_month 留空則列出可用月份；call_put 可篩選買賣權。
- `get_options_institutional_by_contract_history`：查詢三大法人各選擇權契約交易歷史（CALL+PUT合計，可回溯查詢；TAIFEX-DL optContractsDateDown，T152）。contract 為選擇權契約代碼（TXO/TEO/TFO 等，預設 TXO）。區間不可超過 92 日。
- `get_options_institutional_calls_puts_history`：查詢三大法人選擇權買賣權（CALL/PUT）分計交易歷史（可回溯查詢；TAIFEX-DL callsAndPutsDateDown，T153）。適合觀察外資對選擇權 CALL/PUT 布局隨時間的變化趨勢。contract 預設 TXO。區間不可超過 92 日。
- `get_options_oi_change`：查詢台指選擇權每日未平倉量增減，顯示今日與前一交易日的未平倉量及變化量（TAIFEX-API va01，T154）。未平倉大幅增加代表新部位建立，大幅減少代表部位了結或到期。
- `get_otc_daily`：查詢上櫃（OTC）市場當日所有股票收盤行情（TPEx-API tpex_mainboard_daily_close_quotes，T155）。stock_no 選填，指定則只回傳該股票。
- `get_otc_index`：查詢櫃買市場（上櫃）指數歷史行情，包含開高低收、漲跌幅（TPEx-API tpex_index，T156）。
- `get_otc_odd_lot`：查詢上櫃零股（不足一張）交易行情，包含零股成交價、成交量、成交金額（TPEx-API tpex_odd_stock，T157）。stock_no 選填，指定則只回傳該股票。
- `get_public_company_balance_sheet`：根據股票代號查詢公開發行公司資產負債表（TWSE-API t187ap07_X 系列，T158）。自動偵測公司所屬產業並使用對應的財務報表格式。
- `get_public_company_board_shareholdings`：根據股票代號查詢公開發行公司董監事持股餘額明細（TWSE-API t187ap11_P，T159）。
- `get_public_company_income_statement`：根據股票代號查詢公開發行公司綜合損益表（TWSE-API t187ap06_X，T160）。自動偵測公司所屬產業並使用對應的財務報表格式。
- `get_put_call_ratio`：查詢買賣權比（Put/Call Ratio，成交量/未平倉比；§10.F）。單日（date，省略為最新交易日）或範圍（start/end，支援歷史回溯）；多空分界線 1.0 由 _chart_meta 標示。
- `get_real_time_trading_stats`：查詢每 5 秒委託成交統計（盤中即時；TWSE-WEB MI_5MINS，T161）。
- `get_realtime_quote`：查詢任意多檔台股盤中即時報價＋五檔（T194；MIS 單發直查模式，上市/上櫃自動判別，1~20 檔）。與 get_intraday_quote 互補：即查即走、無需 watchlist；盤後回最後成交價或昨收（price_source 標註）。
- `get_recently_listed_companies`：查詢最近上市公司（TWSE-API company/newlisting，T162）。可選 name 過濾。
- `get_securities_trading_changes`：查詢集中市場證券變更交易（T163）。可選 name 過濾。
- `get_short_sale_lending_balance_history`：查詢信用交易融資融券餘額歷史（TWSE-WEB TWT93U，T164）。可選 code/name 過濾。
- `get_short_sale_lending_trades_history`：查詢借券賣出及借券賣出價量歷史（TWSE-WEB TWTASU，T165）。可選 code/name 過濾。
- `get_stock_daily_kline`：查詢個股盤後日/週/月 K 線（TWSE-WEB STOCK_DAY，period/adjust 官方參數）。date 為月份起點，省略時為最近交易日。上櫃資料源未接線（錯誤）。
- `get_stock_daily_quote`：查詢個股盤後日收盤報價（含 MA20/MA60、RSI14、MACD helper 指標）。上市以 TWSE-WEB 日 K（近 3 個月）計算指標；上櫃以 TPEx 收盤行情（指標暫缺）。date 省略時回傳最近交易日。
- `get_stock_daily_trading`：根據股票代號查詢個股日成交資訊（TWSE-API STOCK_DAY_ALL 正規化模型，T166）。
- `get_stock_futures_margin`：查詢股票期貨保證金一覽表，顯示各股票期貨的保證金率及分組級距（TAIFEX-API SingleStockFuturesMargining，T167）。stock_code 可輸入股票代號（如 2330）或期貨契約代碼（如 CAF），留空顯示全部。
- `get_stock_monthly_average`：根據股票代號過濾個股日收盤價及月平均價全市場報表（TWSE-WEB STOCK_DAY_AVG_ALL，T168）。
- `get_stock_monthly_avg_history`：查詢個股月平均價歷史（指定年度逐月；TWSE-WEB STOCK_DAY_AVG，T169）。
- `get_stock_monthly_history`：查詢個股月 K 歷史（指定年度逐月行情；TWSE-WEB STOCK_DAY，T170）。
- `get_stock_monthly_trading`：根據股票代號查詢個股月成交資訊（TWSE-WEB FMSRFK，T171）。
- `get_stock_price_changes`：查詢上市個股股價升降幅（漲跌停參考價；T172）。可選 name 過濾。
- `get_stock_trend_composite`：短中長期「技術面+基本面+籌碼面」綜合研判（v2.1 §9.1，Grade PREVIEW）。horizon 為 short（近 1 月 MA5/MA20 + 法人 5 日）/mid（近 3 月 MA20/MA60 + 法人 20 日）/long（近 6 月 MA20/MA60 + 法人 60 日）。跨來源聚合（TWSE Web API 日K/法人 + TWSE-API/TPEx 估值 + MOPS 損益表），_lineage 為 []Lineage 陣列；上櫃無歷史 K 線，技術面從缺（note 標註）。
- `get_stock_yearly_history`：查詢個股歷年成交資訊彙總（每年一筆長期彙總；TWSE-WEB FMNPTK，T173）。
- `get_stock_yearly_trading`：根據股票代號過濾年度成交資訊全市場報表（TWSE-WEB FMNPTK_ALL，T174）。
- `get_stocks_no_price_change_first_five_days`：查詢上市個股首五日無漲跌幅（T175）。可選 name 過濾。
- `get_suspended_day_trading_announcement`：查詢暫停先賣後買當日沖銷標的預告表（T176）。可選 name 過濾。
- `get_suspended_day_trading_history`：查詢暫停先賣後買當日沖銷交易歷史（T177）。可選 name 過濾。
- `get_suspended_listed_companies`：查詢終止上市公司（TWSE-API company/suspendListingCsvAndHtml，T178）。可選 name 過濾。
- `get_suspended_trading_stocks`：查詢集中市場暫停交易證券（T179）。可選 name 過濾。
- `get_symbol_list`：查詢上市/上櫃代碼表（§10.G；Symbol Registry，§5.2）。來源為 TWSE/TPEx 官方清單（24h 快取每日預熱）；market 省略時回傳全部（依代碼排序）。
- `get_taiex_index_history`：查詢發行量加權股價指數歷史資料（TWSE-WEB MI_5MINS_HIST，T180）。
- `get_taiwan_50_index_history`：查詢臺灣50指數歷史資料（TWSE-WEB TAI50I，T181）。
- `get_taiwan_island_index_history`：查詢寶島股價指數歷史資料（TWSE-WEB FRMSA，T182）。
- `get_taiwan_total_return_index`：查詢發行量加權股價報酬指數歷史資料（TWSE-WEB MFI94U，T183）。
- `get_top_20_volume_stocks`：查詢當日成交量 Top20（TWSE-WEB MI_INDEX20，T184）。可選 name 過濾。
- `get_top_foreign_holdings`：查詢外資持股前 20 名上市公司（TWSE-API MI_QFIIS_sort_20 passthrough，T185）。
- `get_trading_calendar`：查詢交易日曆（§10.G；TWSE 官方開休市表，內嵌 2026 年資料）。year/month 省略時為今年/全年；回傳交易日清單與官方休市日（含名稱）。
- `get_twse_index`：查詢 TWSE 指數盤後行情與歷史日 K（加權指數、寶島、臺灣50 等）。symbol 為指數名稱（省略預設「發行量加權股價指數」）；date 省略時為最近交易日。資料來源：TWSE-API MI_INDEX（單日收盤）+ TWSE-WEB MI_5MINS_HIST（歷史日 K）。
- `get_twse_events`：查詢證交所活動訊息（TWSE-API news/eventList，T191）。top 為回傳筆數上限（預設 10，填 0 回傳全部）；每筆含 No/Title/Details。
- `get_twse_news`：查詢證交所新聞清單（TWSE-API news/newsList passthrough，T186）。
- `get_valuation_ratios`：查詢個股估值指標（PE/PB/殖利率/ROE/每股股利）。上市 TWSE-API BWIBBU_ALL + MOPS；上櫃 TPEx 本益比/殖利率/淨值比。ROE 為 MOPS 損益表摘要年化 ÷ 權益之年化估計（官方無直接端點）。
- `get_warrant_activity`：查詢權證活躍度（TWSE-API 權證每日成交：成交金額/張數 Top N）。top_n 預設 10，最大 50。
- `get_warrant_basic_info`：查詢權證基本資料（TWSE-API t187ap37_L passthrough，T187）。code 選填過濾。
- `get_warrant_daily_trading`：根據股票代號查詢權證每日成交資訊（TWSE-API t187ap42_L 正規化模型，T188）。code 選填過濾。
- `get_warrant_trader_count`：查詢權證流動量提供者報價方式統計（TWSE-API t187ap43_L passthrough，T189）。
- `get_warrant_yearly_issuance_statistics`：查詢權證年度發行統計（TWSE-API t187ap36_L passthrough，T190）。
- `scan_daytrade_eligibility`：買前風險掃描：比對當日注意股/處置股名單與停資停券狀態，回傳當沖資格、風險摘要（名單來源 TWSE-WEB / TPEx 盤後名單）。
- `screen_high_yield`：高殖利率排行（§10.E；T017 composite 引擎批次過濾）。條件：min_yield（預設 3%）、min_dividend（每股現金股利下限）、max_pe、min_consecutive（最低連年配息年數，配息穩定性）。結果依殖利率遞減；整批快取 + 記憶體計算（§12.4）。
- `screen_stocks`：價值/成長篩選全市場股票（§10.D；T017 composite 引擎批次過濾，整批快取 + 記憶體計算，§12.4）。條件：max_pe（低本益比）、max_pb（低股價淨值比）、min_yield（高殖利率）、min_growth（月營收 YoY）、min_profit_growth（淨利 YoY）、require_esg（具 ESG 揭露）。排序 sort（pe 預設|yield|pb|growth）；limit 即 top_n 回傳上限。
- `set_active_watchlist`：設定盤中即時監控的股票觀察清單（最多 15 檔）。呼叫後 background worker 每 8 秒進行快照輪詢，為其餘盤中工具提供記憶體資料。

