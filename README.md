# tw-quant-mcp

台灣量化市場資料 TWSE MCP Server —— 盤中即時 1 分 K 線引擎 + 盤後行情/籌碼/基本面/期權資料，資料 100% 鎖定 TWSE / TPEx / MOPS / TAIFEX **官方免費來源**。

> ⚠️ **免責聲明**：本專案所有回傳資料僅供研究參考，**不構成投資建議**。使用官方公開免費資料以合理使用為原則，嚴禁以本專案進行高頻抓取。

## 功能特色

- **盤中即時引擎**：MIS 8 秒採樣、15 檔 Watchlist、純記憶體 1 分/5 分 K 線組裝（**零 HTTP**）、VWAP、爆量偵測、當沖資格掃描
- **即時報價雙模式**：watchlist 引擎（零 HTTP，高頻監控）＋ `get_realtime_quote` MIS 單發直查（任意多檔 1~20 檔、免 watchlist、上市/上櫃自動判別）
- **全市場快照與風險累計**：`get_all_stocks_daily_close` 單日全市場逐檔收盤行情（OHLC/PE）、`get_abnormal_accumulated_notice_stocks` 注意累計次數異常、`get_twse_events` 證交所活動訊息
- **盤後行情與籌碼**：日 K（含 MA/RSI/MACD 技術指標）、三大法人、融資融券、注意/處置股、權證、外資持股
- **ETF 與指數**：ETF 歷史 NAV + 折溢價（e添富平台，§30.1 L1）、加權指數/寶島/臺灣50 盤後行情與歷史日 K
- **基本面與股利**：財報三表、月營收、五面向健康評分、ESG 揭露八主題（雙來源速度選源＋fallback，T037）、公司資料、除權息行事曆、高殖利率篩選
- **期貨與選擇權**：台指期等 11 契約每日 OHLC、歷史回溯（TAIFEX-DL）、Put/Call Ratio、大額交易人、三大法人期權部位
- **資料治理**：Data Lineage（`_lineage`）全程標註、三層快取（L1 Ristretto / L2 SQLite / Single-flight）、請求級 Rate Limit + Jitter + 退避 + 熔斷、圖表親和（`_chart_meta`）

## 安裝

### 方式一：Homebrew（推薦，macOS / Linux）

```bash
brew tap gentoobreaking/tap https://github.com/gentoobreaking/homebrew-tap.git
brew install tw-quant-mcp
```

安裝後執行檔位於 `$(brew --prefix)/bin/tw-quant-mcp`。

```bash
brew upgrade tw-quant-mcp          # 升級（新版 tag 發佈後）
brew uninstall tw-quant-mcp        # 移除（不會動到其他套件）
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}' \
  | tw-quant-mcp                   # 驗證：應回傳含 serverInfo 的 JSON
```

> ⚠️ **疑難排解**：若出現 `Tap remote mismatch`，代表本地 tap 的 remote URL 與上游不一致：
>
> ```bash
> brew untap gentoobreaking/tap && brew tap gentoobreaking/tap https://github.com/gentoobreaking/homebrew-tap.git
> ```
>
> 注意：**untap 會一併移除該 tap 安裝的套件**，執行後請重新 `brew install`。

### 方式二：從原始碼建置

#### 需求

- Go 1.26+
- macOS / Linux / Windows（`CGO_ENABLED=0` 純靜態編譯，無 cgo 依賴）

#### 建置

```bash
make build          # 產出 bin/tw-quant-mcp（CGO-free 單一執行檔）
make build-release  # 產出 bin/tw-quant-mcp-v$(VERSION)（帶版本號；VERSION 預設 2.1.0，可 make build-release VERSION=x.y.z 覆寫）
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
| --- | --- | --- |
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

### Homebrew 安裝（macOS / Linux）

```bash
brew tap gentoobreaking/tap https://github.com/gentoobreaking/homebrew-tap.git
brew install tw-quant-mcp
```

安裝後將 `$(brew --prefix)/bin/tw-quant-mcp` 填入下方任一 MCP client 的 command 欄位。

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
│                     MCP Engine Layer（194 Tool Router）             │
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

## 工具清單（v2.1 發布時 40 個，§10；⚠️ 已過時——完整目錄見 docs/TOOL_CATALOG.md）

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

v2.1 規格目錄（25 工具）與 v1.3 既有工具的命名比對結論，已獨立至
[docs/TOOL_MAPPING_v2.1_v1.3.md](docs/TOOL_MAPPING_v2.1_v1.3.md)。
Data Grade 註記：`get_stock_trend_composite` 為 `PREVIEW`；其餘核心工具皆 `AVAILABLE`。

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
| --- | --- | --- |
| TWSE-API | openapi.twse.com.tw | 公司治理、ESG 揭露（t187ap46_L_1~21，T037 雙來源之一）、日收盤、外資持股、權證、指數 |
| TWSE-WEB | <www.twse.com.tw/exchangeReport/>* | 日 K、融資融券、三大法人、收盤行情、注意股 |
| TWSE-ETF | <www.twse.com.tw/zh/ETFortune/>* | ETF 歷史 NAV/折溢價（e添富平台，POST ajaxEtfInfoChart） |
| TWSE-MIS | mis.twse.com.tw | 盤中即時 Snapshot（8 秒採樣） |
| TPEx-API | <www.tpex.org.tw/openapi> | 上櫃日收盤、法人、融資融券、注意/處置股 |
| MOPS | mops.twse.com.tw | 月營收、財報三表、重大訊息、公司資料、ESG 揭露八主題（t187ap46_L_1~8，T037 雙來源之一） |
| TAIFEX-API | openapi.taifex.com.tw | 期貨/選擇權行情、PCR、大額交易人、保證金（最新交易日） |
| TAIFEX-DL | <www.taifex.com.tw/cht/3/*DateDown>* | 歷史回溯 CSV（T-1 起） |

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
scripts/release_check.sh   # 發布檢查：CGO-free 建置 + tools/list 工具數
```

### GitHub Release 流程

推送 `v*` tag 即自動觸發（`.github/workflows/release.yml`）：

1. 測試守門（`go vet` + `go test`）
2. 五平台交叉編譯：linux/amd64+arm64、darwin/amd64+arm64、windows/amd64
   （`CGO_ENABLED=0 -trimpath`，版本號以 ldflags 注入 tag）
3. 產出 tar.gz / zip ＋ SHA256 checksums.txt，自動建立 GitHub Release

```bash
git tag v2.1.0 && git push origin v2.1.0
```

CI（`.github/workflows/ci.yml`）於每次 push / PR 執行 vet＋test＋建置確認。

#### Homebrew tap 自動發佈（可選）

Release 流程尾端會自動把 Formula 推送到 `<owner>/homebrew-tap` repo。啟用步驟：

1. GitHub 上建立**公開空 repo**：`gentoobreaking/homebrew-tap`
2. 建立 fine-grained PAT（僅授權 `homebrew-tap` 的 Contents: Read/Write），
   加到本 repo 的 **Settings → Secrets and variables → Actions → New secret**，
   名稱 `TAP_TOKEN`
3. 之後每次打 tag，Formula 自動帶入新版版本號與 SHA256；未設定 `TAP_TOKEN`
   則此步驟自動跳過、不影響 Release

---

## License

本專案採用 **Apache License 2.0** 授權。

- 完整授權條款見 [`LICENSE`](LICENSE)（專案根目錄）
- Apache-2.0 官方條款：<https://www.apache.org/licenses/LICENSE-2.0>
- 版權與貢獻者資訊以 LICENSE 檔案為準

> 本專案為研究/模擬用途，授權條款不構成任何投資建議或保證；
> 使用/修改/再散佈前請詳閱 LICENSE 全文。

本專案僅供個人量化研究與教育用途。資料來源（TWSE、TPEx、MOPS、TAIFEX 官方平台）之使用請遵守各平台之服務條款。

## 附錄：完整工具目錄

全部 **194 個工具**的清單與說明已獨立至 [docs/TOOL_CATALOG.md](docs/TOOL_CATALOG.md)（由真實 `tools/list` 自動彙出，2026-08-26）。
