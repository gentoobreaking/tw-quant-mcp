# TWSE MCP 比較：TWSEMCPServer（線上遠端）vs tw-quant-mcp（本機 Go）

> 產生時間: 2026-08-25｜比較基準:
> - 遠端: https://github.com/twjackysu/TWSEMCPServer（Python / FastMCP，MIT，151★）
>   託管端點: `https://TW-Stock-MCP-Server.fastmcp.app/mcp`（實測 180 個工具）
> - 本機: `~/Projects/tw-quant-mcp`（Go 1.26，v2.1，README 宣告 40 個工具）

## 一、總覽

| 面向 | TWSEMCPServer（遠端） | tw-quant-mcp（本機） |
|---|---|---|
| 定位 | 社群版「廣度優先」：把官方 OpenAPI 盡量整包暴露 | 「治理優先」：精選工具＋盤中引擎＋資料工程 |
| 語言/執行檔 | Python（FastMCP） | Go 單一靜態執行檔（CGO_ENABLED=0） |
| 取得方式 | 免費託管零維運；或 Docker／本地 uv | 自建：`make build`；stdio 或 streamable-http :8787 |
| 工具數 | **180**（實測 tools/list） | **42**（registry 掃描；README 分類為 40） |
| 名稱重疊 | 幾乎為 0——兩邊對同一功能用不同命名（見 §四對照） | 同左 |
| 使用限制 | 託管版有合理使用量上限；商業/大量呼叫建議自架 | 無外部限制，受各來源官方 Rate Limit 約束 |

## 二、資料來源（兩者皆 100% 官方免費源，不接第三方行情商）

| 來源 ID | 端點 | 內容 | 遠端 | 本機 |
|---|---|---|---|---|
| TWSE-API | openapi.twse.com.tw | 公司治理、ESG、日收盤、外資持股、權證、指數 | ✅（143 工具直接映射 OpenAPI schema） | ✅（挑選過） |
| TWSE-WEB | www.twse.com.tw/exchangeReport/* 等 | 日 K、融資融券、三大法人、注意股、收盤行情 | ✅（16 工具） | ✅ |
| TWSE-ETF | e添富平台 ajaxEtfInfoChart / etfDiv | ETF NAV 折溢價、配息 | ❌ | ✅ |
| TWSE-MIS | mis.twse.com.tw | 盤中即時快照 | ✅（單發查詢） | ✅✅（8 秒輪詢引擎） |
| TPEx-API/WEB | www.tpex.org.tw | 上櫃收盤、法人、融券、注意股、除權息、零股 | ✅（10 工具） | ✅ |
| MOPS | mops.twse.com.tw | 月營收、財報三表、重大訊息、ESG 八主題 | ✅ | ✅（雙來源速度選源+fallback） |
| TAIFEX-API | openapi.taifex.com.tw | 期權最新交易日行情、PCR、大額交易人 | ✅（16 工具） | ✅ |
| TAIFEX-DL | taifex 下載頁 CSV | 期權歷史回溯（≤366 日） | ✅（9 工具） | ✅（L2 永久快取） |
| e添富 ETF 平台 | 同 TWSE-ETF | — | ❌ | ✅ |

## 三、能力深度差異

### 遠端獨有／較強
- **廣度**：180 工具幾乎覆蓋 TWSE OpenAPI 全部 schema（公司治理全系列、內部人持股異動、董事薪酬、ESG 全主題、鉅額交易、借券餘額、興櫃申請……）
- 零安裝零維運，適合隨手查證與低頻呼叫
- 官方託管（Prefect Horizon），可用性由供應商保證

### 本機獨有／較強
- **盤中即時引擎**（6 工具）：`set_active_watchlist` 觸發 MIS 8 秒輪詢 →
  `get_intraday_kline`（1 分/5 分 K）、`get_intraday_quote`（五檔）、
  `get_intraday_vwap`（VWAP+Fib 支撐壓力）、`detect_volume_surge`（爆量偵測）、
  `scan_daytrade_eligibility`（當沖資格+注意處置掃描）——純記憶體計算，零 HTTP
- **資料治理**：三層快取（L1 Ristretto / L2 SQLite / single-flight）、
  Per-source Rate Limit + Jitter + 退避 + 熔斷、Data Lineage（`_lineage`）
  全程標註、`_chart_meta` 圖表親和
- 五面向財務健康評分（規則版本化，`MCP_SCORING_CONFIG` 可覆寫）、
  價值/成長篩選器 `screen_stocks`、高殖利率 `screen_high_yield`
- ETF 歷史 NAV + 折溢價序列（e添富 L1）；期貨歷史回溯 L2 永久快取
- `/health` healthcheck 端點（容器化友善）

## 四、功能層級對照（名稱不同、功能對應）

| 功能 | 遠端（Python） | 本機（Go） |
|---|---|---|
| 三大法人個股買賣超 | `get_twse_institutional_investors_by_stock` | `get_institutional_investors` |
| 上櫃三大法人 | `get_otc_institutional` | `get_institutional_investors`（TPEx 併入） |
| 月營收 | `get_company_monthly_revenue` | `get_monthly_revenue` |
| 日 K／技術指標 | `get_stock_history` / `get_stock_monthly_avg_history` | `get_stock_daily_quote`（含 MA/RSI/MACD）/ `get_stock_daily_kline` |
| 融資融券 | `get_margin_trading_info` 系列 | `get_margin_trading` |
| 注意/處置股 | `get_otc_warning_stocks` / `get_today_notice_stocks` | `get_attention_disposition_stocks` |
| 除權息行事曆 | `get_dividend_rights_schedule` | `get_exdividend_calendar` |
| 估值（PE/PB/殖利率） | `get_valuation_ratios_by_date` / `get_stock_valuation_ratios` | `get_valuation_ratios` |
| 外資持股歷史 | `get_foreign_holdings_history` | `get_foreign_shareholding_history` |
| 重大訊息 | `get_twse_news` / `get_major_news` 系列 | `get_major_announcements` |
| ESG | `get_companies_with_csr_reports_103` 等分散式 | `get_esg_report`（八主題一次到位） |
| 期貨 OHLC | `get_daily_futures_market_report` + DL 系列 | `get_futures_daily_ohlc` / `get_futures_history` |
| PCR | `get_put_call_ratio_history` | `get_put_call_ratio` |
| 大額交易人 | `get_large_traders_*` | `get_large_trader_positions` |
| 法人期貨部位 | `get_institutional_traders_by_futures_history` | `get_institutional_futures_positions/history` |
| 趨勢綜合研判 | （散落各工具，需自行聚合） | `get_stock_trend_composite`（跨來源聚合，PREVIEW） |
| 盤中即時 | `get_realtime_quote`（單發） | 引擎式（watchlist→記憶體常駐） |
| 高殖利率/價值篩選 | 無內建（需自行組合查詢） | `screen_high_yield` / `screen_stocks` |

## 五、穩定性與風險

| 風險 | 遠端 | 本機 |
|---|---|---|
| 配額 | 託管版有上限（未公開數字），大量批次會被限 | 無（但須自律遵守官方來源合理使用） |
| 可用性依賴 | Prefect Horizon 託管服務 | 自己的機器/容器 |
| 上游改版衝擊 | 上游更新即受益，但也可能行為突變 | 版本鎖定在自己手裡 |
| 資料正確性爭議 | 難以溯源（無 lineage） | `_lineage` 標註可追溯每筆資料來源與時間 |

## 六、搭配 tw-quant 找買點管線的建議用法

| 場景 | 用哪個 |
|---|---|
| 對帳/驗證報告數據（法人買賣超、月營收、日 K） | 遠端（已註冊 `twstockmcpserver`，零成本） |
| 盤中監控 Top5 進場區貼近度、VWAP、爆量 | 本機（唯一有盤中引擎） |
| 批次回溯驗證（逐日法人部位、期貨歷史） | 本機（限流熔斷保護＋永久快取） |
| 冷門公司治理/ESG 查詢 | 遠端（143 個 TWSE OpenAPI 映射工具） |
| 財務體質快速評分 | 本機 `get_financial_health_check` |

## 七、註冊/啟動方式速查

```bash
# 遠端（已完成）
mcporter config add twstockmcpserver --url https://TW-Stock-MCP-Server.fastmcp.app/mcp

# 本機（streamable-http 模式）
cd ~/Projects/tw-quant-mcp && make build
MCP_TRANSPORT=streamable-http ./bin/tw-quant-mcp &        # http://127.0.0.1:8787/mcp
curl http://localhost:8787/health                          # {"status":"healthy"}
mcporter config add tw-quant-mcp --url http://127.0.0.1:8787/mcp

# 本機（stdio，供 Claude Desktop 等）
./bin/tw-quant-mcp
```

> 附註：兩專案共用 v2.1 規格語彙（§9.1 `get_stock_trend_composite`、§10 工具目錄、
> daybrain 相依契約），屬同一設計脈絡的兩種實作；工具命名策略不同
>（遠端趨近官方 API 原名，本機統一 `get_*` 語意命名），故 MCP 工具名稱幾無重疊，
> 但功能涵蓋高度互補。
