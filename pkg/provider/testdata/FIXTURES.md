# Golden Fixtures 清單（§13 錄製回放）

本目錄存放各主機**官方 raw response**（未經 Normalize），供 unit/integration
測試離線回放（不連網、不觸發 Rate Limit）。**禁止修改 fixtures 之欄位數值**；
官方格式改版時以 `go run ./cmd/fixtures` 重新錄製並更新本清單（T012/T019 備註）。

- 錄製工具：`cmd/fixtures/`（以 pkg/provider 之 SourceContract 建構 URL，依
  §4.4 rate limit 節流）
- 重新錄製：`go run ./cmd/fixtures -host all -date YYYYMMDD`
- 更新契約：錄製後人工比對 Normalize 輸出（`go test ./pkg/provider/`），
  官方改版僅更新 fixtures，不動 Adapter 預設（§2.2 SourceContract 契約）。

## twse/（TWSE_WEB：www.twse.com.tw；TWSE_API：openapi.twse.com.tw）

| fixture | 資料集 | 錄製日期 | 來源 URL |
|---|---|---|---|
| daily_k_2330.json | TWSE_WEB:daily_k | 2026-07-31 | /rwd/afterTrading/STOCK_DAY?response=json&date=20260731&stockNo=2330 |
| market_close.json | TWSE_WEB:market_close | 2026-07-31 | /rwd/afterTrading/MI_INDEX?response=json |
| institutional.json | TWSE_WEB:institutional | 2026-07-31 | /rwd/fund/T86?response=json |
| margin.json | TWSE_WEB:margin | 2026-07-31 | /rwd/afterTrading/MI_MARGN?response=json&date=20260731&selectType=ALL |
| margin_empty.json | TWSE_WEB:margin（空） | 2026-07-31 | 同上（融資融券無資料日） |
| abnormal_volume.json | TWSE_WEB:abnormal_volume | 2026-07-31 | /rwd/announcement/notice?response=json |
| qfiis.json | TWSE_WEB:qfiis | 2026-07-31 | /rwd/fund/MI_QFIIS?response=json&date=20260731 |
| day_avg.json | TWSE_WEB:monthly_avg | 2026-07-31 | /rwd/afterTrading/STOCK_DAY_AVG?response=json&date=20260731&stockNo=2330 |
| day.json | TWSE_WEB:day | 2026-07-31 | 官網收盤行情端點 |
| indices.json | TWSE_WEB:index_history | 2026-07-31 | /indicesReport/MI_5MINS_HIST |
| index_history.json | 指數歷史 | 2026-07-31 | 官網指數端點 |
| block_trades.json / warrants.json / t187ap45.json / twt48u_all.json / esg.json / governance.json / bwibbu_all.json | 各 TWSE 資料集 | 2026-07-31 | openapi.twse.com.tw/v1 對應端點 |
| punish.json | TWSE_API:punish | 2026-07-31 | /v1/announcement/punish |
| foreign_holdings.json | TWSE_API:foreign_holdings | 2026-07-31 | /v1/fund/MI_QFIIS_cat |
| daily_close.json | TWSE_API:daily_close | 2026-07-31 | /v1/exchangeReport/STOCK_DAY_ALL |

## tpex/（TPEX_API：www.tpex.org.tw/openapi/v1）

| fixture | 資料集 | 錄製日期 | 來源 URL |
|---|---|---|---|
| tpex_mainboard_quotes.json | daily_close | 2026-07-31 | /v1/tpex_mainboard_quotes |
| tpex_mainboard_peratio_analysis.json | pe_valuation | 2026-07-31 | /v1/tpex_mainboard_peratio_analysis |
| tpex_3insti_daily_trading.json | institutional | 2026-07-31 | /v1/tpex_3insti_daily_trading |
| tpex_3insti_summary.json | institutional_summary | 2026-07-31 | /v1/tpex_3insti_summary |
| tpex_mainboard_margin_balance.json | margin | 2026-07-31 | /v1/tpex_mainboard_margin_balance |
| tpex_trading_warning_information.json | attention | 2026-07-31 | /v1/tpex_trading_warning_information |
| tpex_disposal_information.json | disposition | 2026-07-31 | /v1/tpex_disposal_information |
| tpex_exright_prepost.json | ex_rights | 2026-07-31 | /v1/tpex_exright_prepost |
| tpex_odd_stock.json | odd_lot | 2026-07-31 | /v1/tpex_odd_stock |
| tpex_index.json | indices | 2026-07-31 | /v1/tpex_index |
| empty.json | 空回應範本 | 2026-07-31 | — |

## mops/（MOPS：mopsfin.twse.com.tw OpenData + mopsov.twse.com.tw AJAX）

| fixture | 資料集 | 錄製日期 | 來源 URL |
|---|---|---|---|
| monthly_revenue.csv | monthly_revenue | 2026-07-31 | /opendata/t187ap05_L.csv |
| income_summary.csv | income_summary | 2026-07-31 | /opendata/t187ap14_L.csv |
| profit_ratios.csv | profit_ratios | 2026-07-31 | /opendata/t187ap17_L.csv |
| company_profile.csv | company_profile | 2026-07-31 | /opendata/t187ap03_L.csv |
| announcements.csv | announcements | 2026-07-31 | /opendata/t187ap04_L.csv |
| income_statement_2330_2026Q1.html | income_statement | 2026-07-31 | /mops/web/ajax_t164sb04 |
| balance_sheet_2330_2026Q1.html | balance_sheet | 2026-07-31 | /mops/web/ajax_t164sb03 |
| cash_flow_2330_2026Q1.html | cash_flow | 2026-07-31 | /mops/web/ajax_t164sb05 |

## taifex/（TAIFEX_API：openapi.taifex.com.tw/v1；TAIFEX_DL：www.taifex.com.tw/cht/3/）

| fixture | 資料集 | 錄製日期 | 來源 URL |
|---|---|---|---|
| tfx_PutCallRatio.json | put_call_ratio（API） | 2026-07-31 | /v1/PutCallRatio |
| tfx_fut.json | futures_daily（API） | 2026-07-31 | /v1/FuturesDaily |
| tfx_opt.json | options_daily（API） | 2026-07-31 | /v1/OptionsDaily |
| tfx_margin2.json | margin（API） | 2026-07-31 | /v1/Margin |
| tfx_MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate.json | insti_futures（API） | 2026-07-31 | /v1/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate |
| tfx_MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheDate.json | insti_options（API） | 2026-07-31 | /v1/…OptionsContractsBytheDate |
| tfx_OpenInterestOfLargeTradersFutures.json | large_trader_fut（API） | 2026-07-31 | /v1/OpenInterestOfLargeTradersFutures |
| tfx_OpenInterestOfLargeTradersOptions.json | large_trader_opt（API） | 2026-07-31 | /v1/OpenInterestOfLargeTradersOptions |
| pc_ratio.json | put_call_ratio（API，重錄） | 2026-07-31 | 同上 PutCallRatio |
| margin.json | margin（API，重錄） | 2026-07-31 | 同上 Margin |
| taifex_fut_daily.csv | futures_daily（DL） | 2026-07-31 | /cht/3/futDataDown（瀏覽器 session 下載轉 UTF-8） |
| taifex_opt_daily.csv | options_daily（DL） | 2026-07-31 | /cht/3/dlOptDataDown |
| taifex_insti_fut.csv | insti_futures（DL） | 2026-07-31 | /cht/3/futContractsDateDown |
| taifex_large_trader_fut.csv | large_trader_fut（DL） | 2026-07-31 | /cht/3/largeTraderFutDown |
| taifex_large_trader_opt.csv | large_trader_opt（DL） | 2026-07-31 | /cht/3/largeTraderOptDown |
| taifex_pc_ratio.csv | put_call_ratio（DL） | 2026-07-31 | /cht/3/dlPcRatioDown |

> 註：DL CSV 需瀏覽器 session（`cmd/fixtures` 錄不到時保留人工下載版，並
> 於重新錄製後以 `head -1` 比對表頭契約——欄位順序變更屬官方改版）。

## mis/（MIS：mis.twse.com.tw）

| fixture | 內容 | 錄製日期 | 來源 URL |
|---|---|---|---|
| tick_01.json … tick_05.json | 盤中多 tick 序列（tse_2330.tw｜otc_6547.tw，間隔 9s） | 2026-08-01（資料日 2026-07-31） | /stock/api/getStockInfo.jsp?ex_ch=tse_2330.tw\|otc_6547.tw |
| index.html | index.jsp Session 預熱回應 | 2026-08-01 | /stock/index.jsp |

> 註：MIS 於非交易時段回傳最近交易日（2026-07-31）之收盤快照；盤中錄製
> （08:45–13:30）時 tlong/tv 隨 tick 變動，即為完整多 tick 序列。
> index.jsp 偶發 404（官方改版/阻擋）時僅跳過（MISWorker 同行為）。
