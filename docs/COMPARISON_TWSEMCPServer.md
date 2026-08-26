# TWSE MCP 比較：TWSEMCPServer（線上遠端）vs tw-quant-mcp（本機 Go）

> 更新時間: 2026-08-26（第三版：補入遠端獨有清單實測；修正名稱重疊過時描述）｜比較基準:
>
> - 遠端: <https://github.com/twjackysu/TWSEMCPServer（Python> / FastMCP，MIT，151★）
>   託管端點: `https://TW-Stock-MCP-Server.fastmcp.app/mcp`（實測 180 個工具，serverInfo v1.27.0）
> - 本機: `~/Projects/tw-quant-mcp`（Go 1.26，v2.1 規格脈絡，實測 tools/list **190** 個工具；
>   README 文末附錄由真實 tools/list 自動產生，舊「40 工具清單」章節已標記過時）

## 一、總覽

| 面向 | TWSEMCPServer（遠端） | tw-quant-mcp（本機） |
| --- | --- | --- |
| 定位 | 社群版「廣度優先」：把官方 OpenAPI 盡量整包暴露 | 「治理＋廣度並進」：OpenAPI 映射大幅補齊＋盤中引擎＋資料工程與真實呼叫稽核 |
| 語言/執行檔 | Python（FastMCP） | Go 單一靜態執行檔（CGO_ENABLED=0） |
| 取得方式 | 免費託管零維運；或 Docker／本地 uv | 自建：`make build`；stdio 或 streamable-http :8787 |
| 工具數 | **180**（實測 tools/list） | **190**（實測 tools/list；v2.1 發布時僅 42，稽核期擴增 4.5 倍） |
| 名稱重疊 | **高**：180 個遠端工具中 **152 個與本機同名**（兩專案同源 v2.1 規格語彙，遠端已同步採用相同命名）；僅餘 28 個名稱差異，其中 24 個功能已被本機覆蓋、4 個為遠端獨有模式（見 §三之一） |
| 使用限制 | 託管版有合理使用量上限；商業/大量呼叫建議自架 | 無外部限制，受各來源官方 Rate Limit 約束 |

## 二、資料來源（兩者皆 100% 官方免費源，不接第三方行情商）

| 來源 ID | 端點 | 內容 | 遠端 | 本機 |
| --- | --- | --- | --- | --- |
| TWSE-API | openapi.twse.com.tw | 公司治理、ESG、日收盤、外資持股、權證、指數 | ✅（107 工具直接映射 OpenAPI schema，按來源分類實抓） | ✅（~76 工具映射，含鉅額交易、券商系列、經營權異動、ESG 全市場掃描） |
| TWSE-WEB | <www.twse.com.tw/exchangeReport/>* 等 | 日 K、融資融券、三大法人、注意股、收盤行情 | ✅（40 工具） | ✅（37 工具） |
| TWSE-ETF | e添富平台 ajaxEtfInfoChart / etfDiv | ETF NAV 折溢價、配息 | ❌ | ✅ |
| TWSE-MIS | mis.twse.com.tw | 盤中即時快照 | ✅（單發查詢） | ✅✅（8 秒輪詢引擎） |
| TPEx-API/WEB | <www.tpex.org.tw> | 上櫃收盤行情、櫃買指數、法人、融券、注意股、除權息、零股 | ✅（10 工具） | ✅（20 工具，含 get_otc_daily/_index/_odd_lot） |
| MOPS | mops.twse.com.tw | 月營收、財報三表、重大訊息、ESG 八主題 | ✅ | ✅（雙來源速度選源+fallback） |
| TAIFEX-API | openapi.taifex.com.tw | 期權最新交易日行情、PCR、大額交易人 | ✅（12 工具） | ✅（32 工具，含年成交量統計等） |
| TAIFEX-DL | taifex 下載頁 CSV | 期權歷史回溯（≤366 日） | ✅（10 工具） | ✅（L2 永久快取） |
| e添富 ETF 平台 | 同 TWSE-ETF | — | ❌ | ✅ |

## 三、能力深度差異

### 遠端獨有／較強

- **廣度仍略勝**：TWSE OpenAPI 映射 107 vs 本機 ~76——董事薪酬、內部人每日交易預審/未轉讓、董監質押、股東會系列、監理裁罰細項、上市程序等冷門 endpoint 本機尚未全數納入
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
- **稽核期新工具家族**（T042–T190，42→190）：鉅額交易日月年四表＋逐筆明細、
  證券商系列 10 支（基本資料/分公司/損益/月報表/電子交易/定期定額名單…）、
  經營權及營業範圍異動四支、ESG 全市場掃描（反競爭訴訟/普惠金融）、
  公開發行公司資產負債表（t187ap07_X 六產業 fallback）、盤後定價交易、
  上櫃收盤行情/櫃買指數/上櫃零股、中央登錄公債補息、權證年度發行統計等
- **可驗證性**：189/190 工具有真實呼叫快照存證（`snapshots/raw/<tool>.json`），
  已下架來源（如 CSR103）回明確 unavailable 說明而非靜默失敗

## 三之一、遠端獨有清單（2026-08-26 雙邊 tools/list 實抓逐支比對）

遠端 180 工具中 152 個與本機同名；28 個名稱差異者歸類如下：

### A. 真正遠端獨有（本機無對應工具）——已立任務 T191–T193

| 遠端工具 | 內容 | 本機缺口 | 上游取值 API |
| --- | --- | --- | --- |
| `get_twse_events` | 證交所活動訊息 | 無任何對應 | `openapi.twse.com.tw/v1/news/eventList` |
| `get_all_stocks_daily_close` | 指定日期**全市場逐檔**收盤行情（OHLC＋PE） | 本機僅個股日收盤／市場彙總，無「單日查全市場」工具 | TWSE-WEB `MI_INDEX?date=&type=ALLBUT0999`（tables 取「每日收盤行情」表） |
| `get_abnormal_accumulated_notice_stocks` | 注意股**累計次數**異常資訊 | 本機注意/處置僅當日清單，無累計次數維度 | `openapi.twse.com.tw/v1/announcement/notetrans` |

### B. 模式差異（功能重疊、使用模型不同）——已立任務 T194

| 遠端工具 | 差異 |
|---|---|
| `get_realtime_quote` | **任意多檔單發**即時報價（MIS `getStockInfo.jsp`，tse_前綴優先、otc_ 重試）；本機盤中引擎雖較強，但須先 `set_active_watchlist`（上限 15 檔），watchlist 外標的無法隨手即時查 |

### C. 名稱不同但功能已被本機覆蓋——其餘 24 支

月營收 ×2 → `get_monthly_revenue`｜除權息 ×2（含上櫃 exright）→ `get_exdividend_calendar`｜
外資持股 ×2 → `get_foreign_shareholding_history` / `get_foreign_industry_holdings`｜
融資融券 ×2 → `get_margin_trading` / `get_margin_trading_info`｜估值 ×3 → `get_valuation_ratios`｜
注意/處置 ×3 → `get_attention_disposition_stocks`｜三大法人 ×5 → `get_institutional_investors` 等｜
日 K → `get_stock_daily_kline/_quote`｜大盤成交/漲跌家數 ×2 → `get_market_summary`｜
PCR 歷史 → `get_put_call_ratio`（同支援任意區間回溯）｜公開發行公司基本資料 → `get_company_profile`。

**結論：實質遠端獨有僅約 3 支冷門查詢＋1 種即時報價模式，均已列入 T191–T194 對齊。**

## 四、功能層級對照（名稱不同、功能對應）

| 功能 | 遠端（Python） | 本機（Go） |
| --- | --- | --- |
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
| 鉅額交易 | `get_block_trade_*` 系列（若有映射） | `get_block_trades_daily/monthly/yearly/detail` 四表＋逐筆 |
| 券商資料 | （散落各工具） | `get_broker_*` 系列 10 支（TWSE-API t187ap18–t187ap22 等） |
| PCR | `get_put_call_ratio_history` | `get_put_call_ratio` |
| 大額交易人 | `get_large_traders_*` | `get_large_trader_positions` |
| 法人期貨部位 | `get_institutional_traders_by_futures_history` | `get_institutional_futures_positions/history` |
| 趨勢綜合研判 | （散落各工具，需自行聚合） | `get_stock_trend_composite`（跨來源聚合，PREVIEW） |
| 盤中即時 | `get_realtime_quote`（單發） | 引擎式（watchlist→記憶體常駐） |
| 高殖利率/價值篩選 | 無內建（需自行組合查詢） | `screen_high_yield` / `screen_stocks` |

## 五、穩定性與風險

| 風險 | 遠端 | 本機 |
| --- | --- | --- |
| 配額 | 託管版有上限（未公開數字），大量批次會被限 | 無（但須自律遵守官方來源合理使用） |
| 可用性依賴 | Prefect Horizon 託管服務 | 自己的機器/容器 |
| 上游改版衝擊 | 上游更新即受益，但也可能行為突變 | 版本鎖定在自己手裡 |
| 資料正確性爭議 | 難以溯源（無 lineage） | `_lineage` 標註可追溯每筆資料來源與時間 |

## 六、搭配 tw-quant 找買點管線的建議用法

| 場景 | 用哪個 |
| --- | --- |
| 對帳/驗證報告數據（法人買賣超、月營收、日 K） | 遠端（免費託管；本機 `mcporter` 目前未註冊，需要時依 §七指令加入） |
| 盤中監控 Top5 進場區貼近度、VWAP、爆量 | 本機（唯一有盤中引擎） |
| 批次回溯驗證（逐日法人部位、期貨歷史） | 本機（限流熔斷保護＋永久快取） |
| 冷門公司治理/ESG 查詢 | 遠端仍最全（107 個 TWSE-API 映射）；本機已涵蓋經營權異動、獨立董事、累積投票、ESG 掃描等常用子集 |
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

> 附註：兩專案共用 v2.1 規格語彙且遠端已同步本機命名（152/180 同名），
> 屬同一設計脈絡的兩種實作。本機稽核擴增後功能重疊已大幅上升——遠端優勢收斂至
> 少數冷門 endpoint 與零維運（§三之一），本機優勢在盤中引擎、資料治理與可驗證性。
> 本機逐源覆蓋明細另見 `docs/TOOL_COVERAGE_BY_SOURCE.md`。

## TWSE OpenAPI 餘量端點等價覆蓋聲明（T242，2026-08-26）

以下 12 條 TWSE OpenAPI 目錄端點已全數對齊：1 條新接線（announcement/notice →
`get_twse_announcement_notice`），11 條由既有工具等價覆蓋：

| OpenAPI 端點 | 等價覆蓋工具 | 備註 |
| --- | --- | --- |
| announcement/notice | get_twse_announcement_notice（T242 新接線） | passthrough |
| block/BFIAUU_d | get_block_trades / get_block_trades_detail（T042/T043） | TWSE-WEB 同源 |
| block/BFIAUU_m | get_block_trades_monthly（T044） | TWSE-WEB 同源 |
| block/BFIAUU_y | get_block_trades_yearly（T045） | TWSE-WEB 同源 |
| exchangeReport/BWIBBU_d | get_valuation_ratios（T014，BWIBBU_ALL 全市場快照） | d 為單日子集 |
| exchangeReport/FMSRFK_ALL | get_stock_month_trade（T171，FMSRFK） | ALL 為全市場版 |
| exchangeReport/FMTQIK | 成交統計工具（TWSE-WEB FMTQIK 同源） | 同一資料源 |
| opendata/t187ap03_L/P | 上市基本資料工具（MOPS 版）；上櫃對稱 get_otc_fundamental_stats kind=profile（T238） | L=上市、P=上市櫃合併 |
| opendata/t187ap04_L | 重大訊息工具；上櫃對稱 get_otc_fundamental_stats kind=major_message（T238） | 同上 |
| opendata/t187ap05_L/P | 財測/查核差異工具；上櫃對稱 get_otc_fundamental_stats kind=forecast_*/audit_diff（T238） | 同上 |
