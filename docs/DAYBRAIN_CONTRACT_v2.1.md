# daybrain 相依契約確認（v2.1 發布）

> 自 README 獨立（2026-08-26）：記錄 v2.1 發布時與 `tw-quant-daybrain`
> （v1.1 規格，Client 端）§2.2 契約子集的逐項核對結果，供下游整合參考。

## daybrain 相依契約確認（v2.1 發布）

`tw-quant-daybrain`（v1.1 規格，Client 端）§2.2 契約子集與本 v2.1 發布比對：

- **工具名稱（15 依賴）**：12 個存在且未變更（`set_active_watchlist`、`get_intraday_vwap`、`detect_volume_surge`、`get_intraday_quote`、`get_intraday_kline`、`get_market_summary`、`get_futures_daily_ohlc`、`get_put_call_ratio`、`get_institutional_investors`、`get_major_announcements`、`get_abnormal_trading`、`get_stock_daily_kline`、`scan_daytrade_eligibility`、`get_trading_calendar`、`get_symbol_list`）。`get_pre_market_quote` / `get_taifex_night` / `get_us_market` 不在本服務工具目錄（v1.3 起即不存在，非 v2.1 造成）——需 daybrain 側對齊，夜盤可用 `get_futures_daily_ohlc` + `get_futures_history` 替代。
- **Envelope 結構**：`data` / `_lineage` / `_chart_meta` 不變；v2.1 新增 `source_role`、`grade`、`cache_age_sec` 欄位（向後相容）。
- **Lineage 欄位變更（T021 決策，2026-08-01 確認）**：`derived_from` / `cache_ttl` / `source_url` 正式 JSON 不再輸出（內部保留，debug/log 模式可輸出）。daybrain §3.1 守門規則之 `cache_ttl ≤ 4s` 檢查需改以 `cache_age_sec`（資料已存活秒）＋`sampling_sec` 判斷，或於 daybrain 端開啟 debug 模式；其餘守門欄位（`freshness` / `fetched_at` / `is_cached` / `sampling_sec` / `source`）皆仍輸出。

**結論**：工具名稱與 Envelope 契約未破壞；唯一變更為 `cache_ttl` 輸出（T021 已確認之決策，於本發布說明中列為已知變更）。
