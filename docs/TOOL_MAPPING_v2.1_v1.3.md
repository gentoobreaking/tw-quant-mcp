# v2.1 §9 ↔ v1.3 工具對照

> 自 README 獨立（2026-08-26）：記錄 v2.1 規格目錄（25 工具）與 v1.3 既有
> 工具（36 個）的比對結論，作為工具命名演進的歷史脈絡。現行完整工具清單
> （194 個）見 [TOOL_CATALOG.md](TOOL_CATALOG.md)。

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
