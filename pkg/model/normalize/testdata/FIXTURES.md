# Normalize 層測試 Fixtures

本目錄為 `pkg/model/normalize` 單元測試之官方 raw fixture **副本**，
與 `pkg/provider/testdata/` 同步（T022 以官方原文驅動 From<Source>() 轉換測試）。

| fixture | 來源 | 錄製日期 | 用途 |
|---|---|---|---|
| `mis/tick_01.json` | MIS getStockInfo.jsp | 2026-08-01（資料日 2026-07-31） | FromMIS → KlineBar（tick bar，tv 張→股） |
| `twse/institutional.json` | TWSE Web T86 三大法人日報 | 2026-07-31 | FromTWSEWeb → InstitutionalFlow（千分位逗號/市場/lineage） |

**禁止修改 fixture 之欄位數值**；官方格式改版時以 `go run ./cmd/fixtures`
重新錄製後同步更新兩處副本與 FIXTURES.md（T019 契約測試守則）。
