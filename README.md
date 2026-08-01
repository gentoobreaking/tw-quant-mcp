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

## 工具清單（36 個，§10）

### A. 盤中即時引擎（6）
`set_active_watchlist` · `get_intraday_kline` · `get_intraday_quote` · `get_intraday_vwap` · `detect_volume_surge` · `scan_daytrade_eligibility`

### B. 盤後行情與籌碼（9）
`get_stock_daily_quote` · `get_stock_daily_kline` · `get_market_summary` · `get_institutional_investors` · `get_foreign_industry_holdings` · `get_foreign_shareholding_history` · `get_margin_trading` · `get_abnormal_trading` · `get_warrant_activity`

### C. 重大訊息與風險（2）
`get_major_announcements` · `get_attention_disposition_stocks`

### D. 基本面與篩選（7）
`get_financial_statements` · `get_monthly_revenue` · `get_financial_health_check` · `get_valuation_ratios` · `get_esg_report` · `get_company_profile` · `screen_stocks`

### E. 股利（3）
`get_dividend_history` · `get_exdividend_calendar` · `screen_high_yield`

### F. 期貨與選擇權（7）
`get_futures_daily_ohlc` · `get_futures_history` · `get_put_call_ratio` · `get_large_trader_positions` · `get_institutional_futures_positions` · `get_institutional_options_positions` · `get_institutional_futures_history`

### G. 基礎設施（2）
`get_symbol_list` · `get_trading_calendar`

## 回傳結構（Envelope）

所有工具回傳統一 Envelope（§3.3）：

```json
{
  "data": { "...": "業務資料（Normalized Model）" },
  "_lineage": {
    "source": "TWSE_WEB",
    "source_role": "canonical",
    "freshness": "POST_MARKET_TODAY",
    "fetched_at": "2026-08-01T16:30:00+08:00",
    "data_date": "2026-08-01",
    "sampling_sec": 0,
    "is_cached": false,
    "cache_ttl": 86400,
    "latency_ms": 123
  },
  "_chart_meta": { "recommended_type": "line" },
  "http_calls": 1,
  "disclaimer": "僅供研究參考，不構成投資建議"
}
```

- `_lineage`：來源機構 / 來源角色（canonical/helper/fallback）/ 新鮮度分級 / 採樣間隔 / 快取狀態 / 延遲
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
