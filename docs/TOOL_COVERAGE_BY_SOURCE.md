# 工具覆蓋比較（依資料來源）：TWSEMCPServer 遠端 vs tw-quant-mcp 本機

> 產生時間: 2026-08-25｜實測基準:
> - 遠端 `twstockmcpserver`：`tools/list` 實抓 **180** 個工具，按官方來源分類：
>   TWSE-API 107｜TWSE-WEB 40｜TAIFEX-DL 10｜TPEx 10｜TAIFEX-API 12｜TWSE-MIS 1
> - 本機 `~/Projects/tw-quant-mcp`（Go v2.1）：registry 掃描 **40** 個工具
>
> 圖例：✅ 兩邊都有｜🔵 僅僅遠端｜🟢 僅本機｜🟡 兩邊都有但深度不同

---

## 一、TWSE OpenAPI（openapi.twse.com.tw）——遠端 107 vs 本機 ~8

遠端把 OpenAPI 幾乎整包映射（每個 endpoint 一個工具）。本機只挑選併入少數工具。

### 公司治理／董事會（僅遠端 🔵）
| 遠端工具 | 主題 |
|---|---|
| get_company_board_info / board_shareholdings / board_insufficient_shares（含 consecutive） | 董事會結構、持股、持股不足 |
| get_company_director_compensation / supervisor_compensation / consolidated_* | 董／監事薪酬（含合併報表） |
| get_company_ceo_dual_role | CEO 兼任 |
| get_companies_with_independent_directors | 獨立董事 |
| get_company_ownership_and_control | 股權結構與控制權 |

### 內部人／股務（僅遠端 🔵）
| 遠端工具 | 主題 |
|---|---|
| get_company_daily_insider_trades_preannounced / untransferred | 內部人每日交易（預審/未轉讓） |
| get_companies_with_ownership_changes 系列三支 | 經營權異動 |
| get_company_shareholder_meeting_* 三支、shareholder_proposal_exercise、companies_cumulative_voting | 股東會 |
| get_company_board_pledged_shares | 董監質押 |

### 永續／ESG（僅遠端 🔵，本機以 `get_esg_report` 八主題聚合 🟡）
get_company_climate_management、human_development、waste_management、water_management、energy_management、greenhouse_gas_emissions、anticompetitive_litigation、inclusive_finance、community_relations、investor_communications、product_quality_safety、food_safety、product_lifecycle、fuel_management、supply_chain_management、info_security、risk_management、csr_reports_103……

### 監理／風險（僅遠端 🔵）
get_company_sec_regulatory_penalties、market_disposal_stocks、disposition_securities_period、information_disclosure_violations、financial_program_abnormal_recommendations、quarterly_audit_variance、earnings_forecast_achievement

### 上市程序（僅遠端 🔵）
get_recently_listed_companies、foreign/local_companies_applying_for_listing、suspended_listed_companies

### 兩邊都有 ✅（名稱不同）
| 功能 | 遠端 | 本機 |
|---|---|---|
| 公司基本資料 | get_company_profile / public_company_profile | get_company_profile |
| 月營收（MOPS） | get_company_monthly_revenue / get_public_company_monthly_revenue | get_monthly_revenue |
| 大額持股異動 | get_public_company_board_shareholdings | （board 相關未納入） |
| ESG 揭露 | csr_reports_103 等（分散） | get_esg_report（八主題聚合＋fallback） |

**缺口結論**：公司治理/內部人/ESG 的「細粒度」查詢是本機最大空白區（約 60+ 工具）；但本機 `get_esg_report` 已涵蓋 ESG 八主題的主要需求。

---

## 二、TWSE-WEB（www.twse.com.tw）——遠端 40 vs 本機 8

### 行情與 K 線
| 功能 | 遠端 | 本機 |
|---|---|---|
| 全市場日收盤行情 | 🔵 get_all_stocks_daily_close | 🟢 get_stock_daily_quote（單檔+指標）/ get_market_summary |
| 個股日/月/年 K 與均價 | 🔵 get_stock_history、monthly_avg_history、monthly_history、yearly_history | 🟡 get_stock_daily_kline（日/週/月，官方 period 參數） |
| 個股日成交明細 | 🔵 get_stock_daily_trading | ❌ |
| 加權指數歷史 | 🔵 get_taiex_index_history | 🟢 get_twse_index（加權+寶島+臺灣50） |
| 市場估值（by date） | 🔵 get_market_valuation_by_date | 🟡 get_valuation_ratios（個股） |

### 籌碼
| 功能 | 遠端 | 本機 |
|---|---|---|
| 三大法人（彙總/個股） | ✅ get_twse_institutional_investors_summary / _by_stock | ✅ get_institutional_investors（個股+彙總） |
| 法人金額歷史 | 🔵 get_market_institutional_amounts_history | ❌ |
| 外資持股歷史 | ✅ get_foreign_holdings_history | ✅ get_foreign_shareholding_history |
| 外資產業配置 | ✅ get_foreign_investment_by_industry | ✅ get_foreign_industry_holdings |
| 融資融券餘額 | ✅ get_margin_balance | ✅ get_margin_trading |
| 融券借券餘額/成交 | 🔵 get_short_sale_lending_balance_history / trades_history | ❌ |

### 交易輔助（僅遠端 🔵）
get_market_turnover_history、get_top_20_volume_stocks、get_after_hours_trading、get_odd_lot_trading_quotes、get_block_trades_detail/daily/monthly/yearly、get_daily_securities_lending_volume、get_stock_price_changes、get_market_gain_loss_statistics、get_cross_market_trading_info、get_first_listed_foreign_stocks_daily、get_suspended_trading_stocks、get_margin_loan_restrictions_announcement、get_abnormal_accumulated_notice_stocks、get_today_notice_stocks、get_daily_day_trading_targets、get_suspended_day_trading_announcement、get_financial_program_abnormal_recommendations、get_stocks_no_price_change_first_five_days、get_valuation_ratios_by_date、get_stock_monthly_trading、get_stock_yearly_trading

**缺口結論**：本機聚焦「單一標的深挖」，遠端多了大量「全市場掃描型」清單（鉅額交易、零股、盤後、漲跌統計……）。

---

## 三、TWSE-MIS（盤中即時）——遠端 1 vs 本機引擎 6

| 能力 | 遠端 | 本機 |
|---|---|---|
| 即時報價快照 | ✅ get_realtime_quote（每次呼叫打一次 HTTP） | ✅✅ get_intraday_quote（8 秒輪詢常駐記憶體，零 HTTP） |
| 盤中 1 分/5 分 K | ❌ | 🟢 get_intraday_kline |
| 累計 VWAP + Fib 支撐壓力 | ❌ | 🟢 get_intraday_vwap |
| 爆量/急拉偵測 | ❌ | 🟢 detect_volume_surge |
| Watchlist 管理（≤15 檔） | ❌ | 🟢 set_active_watchlist |
| 當沖資格買前掃描 | ❌ | 🟢 scan_daytrade_eligibility |

**缺口結論**：這是本機的核心護城河；遠端僅能「問一次答一次」。

---

## 四、TPEx（櫃買中心）——遠端 10 vs 本機 併入各工具

| 功能 | 遠端 | 本機 |
|---|---|---|
| 上櫃日收盤/指數 | ✅ get_otc_daily / otc_index | ✅（get_stock_daily_quote / market_summary 併 TPEx 來源） |
| 上櫃三大法人 | ✅ get_otc_institutional / otc_institutional_summary | ✅（同上） |
| 上櫃融資融券 | ✅ get_otc_margin_balance | ✅（get_margin_trading 併 TPEx） |
| 上櫃注意/處置股 | ✅ get_otc_warning_stocks / otc_disposal_stocks | ✅（get_attention_disposition_stocks） |
| 上櫃估值 | ✅ get_otc_valuation | ✅（get_valuation_ratios 併 TPEx） |
| 上櫃除權息預告 | ✅ get_otc_exright | ✅（get_exdividend_calendar 併 TPEx） |
| 上櫃零股 | 🔵 get_otc_odd_lot | ❌ |

**缺口結論**：功能面幾乎對等（本機採「來源併入工具」策略），僅缺上櫃零股。

---

## 五、MOPS（公開資訊觀測站）

| 功能 | 遠端 | 本機 |
|---|---|---|
| 財報三表 | 🔵 income_statement / balance_sheet（public 版另有） | ✅ get_financial_statements（三表一次） |
| 月營收 | ✅ get_company_monthly_revenue | ✅ get_monthly_revenue（YoY/MoM/累計，years ≤10） |
| 重大訊息 | 🔵 get_company_major_news / twse_news / twse_events | ✅ get_major_announcements（日期/代號/關鍵字過濾） |
| 五面向健康評分 | ❌ | 🟢 get_financial_health_check |
| 公司資料 | ✅ get_company_profile | ✅ 同 |

---

## 六、TAIFEX API（最新交易日）——遠端 12 vs 本機 5

| 功能 | 遠端 | 本機 |
|---|---|---|
| 期貨/選擇權每日報告 | ✅ get_daily_futures/options_market_report | ✅ get_futures_daily_ohlc（11 契約白名單） |
| 三大法人期貨/選擇權部位 | ✅ institutional_traders_by_futures / by_options / calls_puts | ✅ get_institutional_futures_positions / options_positions |
| 大額交易人 OI | ✅ large_traders_futures_oi / options_oi | ✅ get_large_trader_positions（期+選合併） |
| 期貨/股票期貨保證金 | 🔵 index/stock_futures_margin | ❌ |
| 選擇權 Delta | 🔵 get_options_delta | ❌ |
| 法人 general 彙總 | 🔵 get_institutional_general | ❌ |

## 七、TAIFEX-DL（歷史回溯 CSV）——遠端 10 vs 本機 2

| 功能 | 遠端 | 本機 |
|---|---|---|
| 期貨每日 OHLC 歷史 | 🔵 專用工具 | ✅ get_futures_history |
| 法人期貨部位歷史 | 🔵 同上類 | ✅ get_institutional_futures_history |
| PCR 歷史 | 🔵 put_call_ratio_history | 🟡 get_put_call_ratio（支援範圍回溯） |
| 法人選擇權分計/總表歷史、大額交易人歷史、選擇權 OHLC 歷史 | 🔵 多支 | ❌（未個別開工具） |

**缺口結論**：遠端把 DL 頁的每種報表都開成獨立工具；本機只挑期貨 OHLC 與法人部位兩條高頻需求（但 L2 永久快取）。

---

## 八、本機獨有（遠端完全沒有）🟢

| 工具 | 價值 |
|---|---|
| set_active_watchlist + get_intraday_kline/quote/vwap + detect_volume_surge | MIS 8 秒輪詢的盤中引擎（記憶體、零 HTTP） |
| scan_daytrade_eligibility | 當沖資格＋注意/處置買前風險掃描 |
| screen_stocks / screen_high_yield | 全市場價值/成長/殖利率篩選器 |
| get_financial_health_check | 五面向健康評分（規則版本化） |
| get_stock_trend_composite | 技術+基本面+籌碼跨來源綜合研判（PREVIEW） |
| get_etf_nav | ETF NAV 折溢價序列（e添富） |
| get_etf_dividend | ETF 配息歷史 |
| get_symbol_list / get_trading_calendar | 代碼表與交易日曆（遠端散落或無） |

---

## 九、缺口總帳（若要以本機補齊遠端）

| 優先級 | 缺口 | 建議 |
|---|---|---|
| 高 | 外資持股 by industry 已有；缺「借券餘額/成交歷史」「法人金額歷史」 | 補 TWSE-WEB 兩個 endpoint 即可 |
| 中 | 全市場掃描清單（鉅額交易、盤後、零股、漲跌統計、top20 量） | 依需求逐個加；或直接 fallback 用遠端 |
| 中 | TAIFEX-DL 其餘 7~8 種歷史報表（PCR 歷史已可範圍回溯） | 有台指期籌碼回溯需求再加 |
| 低 | 公司治理/內部人/ESG 細粒度 60+ 工具 | 直接用遠端；本機 `get_esg_report` 已夠日常 |
| 低 | 上櫃零股、保證金、選擇權 Delta | 用到再加 |
