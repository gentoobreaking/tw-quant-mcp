# tw-quant-mcp 36 工具真實呼叫報告

- 產生時間: 2026-08-01 19:01 
- 工具總數: 36
- ✅ 成功: 30
- ❌ 錯誤: 6

> 資料來源為官方公開免費資料（TWSE/TPEx/MOPS/TAIFEX），僅供研究參考，不構成投資建議。

---

## detect_volume_surge.json ❌

- **參數**: `{"symbol": "2330", "minutes": 5}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料

## get_abnormal_trading.json ✅

- **參數**: `{"market": "otc", "top_n": 5}`
- **http_calls**: 1

### data

```json
[
  {
    "code": "2061",
    "name": "風青",
    "info": "最近六個營業日(自當日之前一個營業日起)之當日沖銷成交量占總成交量比率達64.51%，當日之前一個營業日當日沖銷成交量占總成交量達69.14%（第十三款)"
  },
  {
    "code": "3211",
    "name": "順達",
    "info": "最近六個營業日(含當日)累積之最後成交價跌幅達28.03%且最近六個營業日(含當日)起迄兩個營業日之最後成交價價差達新臺幣90元(第一款)當日週轉率達5.79%(第四款)"
  },
  {
    "code": "3236",
    "name": "千如",
    "info": "最近六個營業日(自當日之前一個營業日起)之當日沖銷成交量占總成交量比率達65.66%，當日之前一個營業日當日沖銷成交量占總成交量達66.25%（第十三款)"
  },
  {
    "code": "3260",
    "name": "威剛",
    "info": "最近六個營業日(自當日之前一個營業日起)之當日沖銷成交量占總成交量比率達62.1%，當日之前一個營業日當日沖銷成交量占總成交量達68.07%（第十三款)"
  },
  {
    "code": "3362",
    "name": "先進光",
    "info": "當日本益比為N/A、股價淨值比為6.72且為其所屬產業類別股價淨值比之2.93倍、當日週轉率達6.49%(第六款)"
  }
]
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TPEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:31+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48748 |
| latency_ms | 97 |

> 僅供研究參考，不構成投資建議

## get_attention_disposition_stocks.json ✅

- **參數**: `{"market": "otc"}`
- **http_calls**: 1

### data

| 欄位 | 值 |
|---|---|
| market | otc |
| date | 2026-07-31 |
| attention | `[21 筆] 例: code=2061, name=風青, info=最近六個營業日(自當日之前一個營業日起)之當日沖銷成交量占總成交量比率達64.51%，當日之前一個營業日當日沖銷成交量占總成交量達69.14%（第十三款)…` |
| disposition | `[39 筆] 例: code=3624, name=光頡, period=1150803~1150814, reason=因連續3個營業日達本中心作業要點第四條第一項第一款…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TPEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:34+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 48745 |
| latency_ms | 152 |

> 僅供研究參考，不構成投資建議

## get_company_profile.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 1

### data

| 欄位 | 值 |
|---|---|
| table_date | 2026-08-01 |
| code | 2330 |
| name | 台灣積體電路製造股份有限公司 |
| short_name | 台積電 |
| foreign_reg | － |
| industry | 24 |
| address | 新竹科學園區力行六路8號 |
| tax_id | 22099131 |
| chairman | 魏哲家 |
| president | 總裁: 魏哲家 |
| spokesman | 黃仁昭 |
| spokesman_title | 資深副總經理暨財務長 |
| … | 共 33 鍵（其餘省略） |

### _lineage

| 欄位 | 值 |
|---|---|
| source | MOPS |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:49+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 86400 |
| latency_ms | 387 |

> 僅供研究參考，不構成投資建議

## get_dividend_history.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 0
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| symbol | 2330 |
| name | 台積電 |
| market | tse |
| years | `[2 筆] 例: dividend_year=115, progress=董事會決議, cash_dividend=7, stock_dividend=0, cash_total=181526590469, net_income=572479752038, …` |
| total_years | 2 |
| consecutive_years | 2 |
| avg_cash_dividend | 6.5 |
| last_yield_pct | 0.91 |
| note | 股利年度以官方（民國）為準；連續配息年數僅以官方現行提供之年度計算 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:51+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 43200 |
| latency_ms | 0 |

> 僅供研究參考，不構成投資建議

## get_esg_report.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 0

### data

| 欄位 | 值 |
|---|---|
| symbol | 2330 |
| name | 台積電 |
| market | tse |
| topics | `[9 筆] 例: `{"topic": "溫室氣體排放", "year": "114", "report_date": "2026-08-01", "fields": {"溫室氣體排放密集度(噸CO2e/單位)": "3.4000", "範疇一取得驗證": …` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:48+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 86400 |
| latency_ms | 0 |

> 僅供研究參考，不構成投資建議

## get_exdividend_calendar.json ✅

- **參數**: `{}`
- **http_calls**: 2

### data

| 欄位 | 值 |
|---|---|
| range_start | 2026-08-01 |
| range_end | 2027-02-01 |
| events | `[134 筆] 例: date=2026-08-03, code=1465, name=偉全, market=tse, kind=息, cash_dividend=0.3, stock_dividend=0…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:52+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 86400 |
| latency_ms | 114 |

> 僅供研究參考，不構成投資建議

## get_financial_health_check.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 4
- **建議圖表**: `radar`

### data

| 欄位 | 值 |
|---|---|
| code | 2330 |
| name | 台積電 |
| market | tse |
| scoring_version | v1 |
| date | 2026-08-01 |
| profit | score=0, available=False, note=無獲利能力指標（MOPS profit_ratios） |
| growth | score=0, available=False, note=無損益表摘要（MOPS income_summary） |
| structure | score=0, available=False, note=無資產負債表（MOPS balance_sheet） |
| dividend | score=58, available=True, note=連年配息 2 年（滿分基準 5 年）、有配息 2/2 年度、殖利率 0.91% |
| governance | score=100, available=True, note=具 ESG、公司治理規程 揭露 |
| total | 23.7 |
| note | 評分輸入來自 T014 已快取之官方資料（MOPS 財報/TWSE 估值・股利・ESG/TPEx 估值） |

### _lineage

| 欄位 | 值 |
|---|---|
| source | MOPS |
| source_role | helper |
| derived_from | `[6 筆] 例: MOPS:income_summary…` |
| fetched_at | 2026-08-01T18:27:43+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 43200 |
| latency_ms | 3176 |

> 僅供研究參考，不構成投資建議

## get_financial_statements.json ✅

- **參數**: `{}`
- **http_calls**: 4

### data

| 欄位 | 值 |
|---|---|
| symbol | 1232 |
| name | 大統益 |
| year | 2026 |
| quarter | 2 |
| income | `{"table_date": "2026-08-01", "year": 2026, "quarter": 2, "code": "1232", "name": "大統益股份有限公司", "industry": "食品工業", "eps": 4.71, "par_value": "新台幣                 10.0000元", "revenue…` |
| balance_sheet | `{"table_date": "2026-04-01", "year": 2026, "quarter": 2, "total_assets": 10715479000, "current_assets": 7829900000, "non_current_assets": 2885579000, "total_liabilities": 497426500…` |
| cash_flow | table_date=2026-04-01, year=2026, quarter=2, operating_cash_flow=696778000, investing_cash_flow=-189324000, financing_cash_flow=-24943000, ending_cash_balance=3172462000 |
| profit_ratios | `{"table_date": "2026-08-01", "year": 2026, "quarter": 2, "code": "1232", "name": "大統益", "revenue_million": 11482, "gross_margin_pct": 13.85, "operating_margin_pct": 8.36, "pretax_m…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | MOPS |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:35+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 43200 |
| latency_ms | 6142 |

> 僅供研究參考，不構成投資建議

## get_foreign_industry_holdings.json ✅

- **參數**: `{}`
- **http_calls**: 1
- **建議圖表**: `pie`

### data

```json
[
  {
    "industry": "ETF",
    "company_count": 236,
    "share_number": 245637445760,
    "foreign_share": 6173897057,
    "percentage": 2.51
  },
  {
    "industry": "水泥工業",
    "company_count": 8,
    "share_number": 14107198706,
    "foreign_share": 1722768021,
    "percentage": 12.21
  },
  {
    "industry": "食品工業",
    "company_count": 25,
    "share_number": 14867130138,
    "foreign_share": 2420084212,
    "percentage": 16.28
  },
  {
    "industry": "塑膠工業",
    "company_count": 22,
    "share_number": 32236388213,
    "foreign_share": 8059533606,
    "percentage": 25
  },
  {
    "industry": "紡織纖維",
    "company_count": 42,
    "share_number": 18966727617,
    "foreign_share": 1774262933,
    "percentage": 9.35
  },
  {
    "industry": "電機機械",
    "company_count": 50,
    "share_number": 12186726917,
    "foreign_share": 1435748119,
    "percentage": 11.78
  },
  {
    "industry": "電器電纜",
    "company_count": 16,
    "share_number": 8469626504,
    "foreign_share": 1005166472,
    "percentage": 11.87
  },
  {
    "industry": "化學生技醫療",
    "company_count": 89,
    "share_number": 21186671828,
    "foreign_share": 3193112556,
    "percentage": 15.07
  },
  {
    "industry": "化學工業",
    "company_count": 28,
    "share_number": 10749596100,
    "foreign_share": 1396575174,
    "percentage": 12.99
  },
  {
    "industry": "生技醫療業",
    "company_count": 61,
    "share_number": 10437075728,
    "foreign_share": 1796537382,
    "percentage": 17.21
  },
  {
    "industry": "玻璃陶瓷",
    "company_count": 5,
    "share_number": 3840410326,
    "foreign_share": 197744395,
    "percentage": 5.15
  },
  {
    "industry": "造紙工業",
    "company_count": 7,
    "share_number": 5877335202,
    "foreign_share": 510304657,
    "percentage": 8.68
  },
  {
    "industry": "鋼鐵工業",
    "company_count": 32,
    "share_number": 30071963006,
    "foreign_share": 3846431481,
    "percentage": 12.79
  },
  {
    "industry": "橡膠工業",
    "company_count": 11,
    "share_number": 8473538291,
    "foreign_share": 815293098,
    "percentage": 9.62
  },
  {
    "industry": "汽車工業",
    "company_count": 44,
    "share_number": 9423658423,
    "foreign_share": 1036585110,
    "percentage": 11
  },
  {
    "industry": "電子工業",
    "company_count": 457,
    "share_number": 238393884554,
    "foreign_share": 66790123601,
    "percentage": 28.02
  },
  {
    "industry": "半導體業",
    "company_count": 96,
    "share_number": 79024428481,
    "foreign_share": 33843920302,
    "percentage": 42.83
  },
  {
    "industry": "電腦及週邊設備業",
    "company_count": 64,
    "share_number": 37609156096,
    "foreign_share": 8218700525,
    "percentage": 21.85
  },
  {
    "industry": "光電業",
    "company_count": 68,
    "share_number": 35372900467,
    "foreign_share": 4889961051,
    "percentage": 13.82
  },
  {
    "industry": "通信網路業",
    "company_count": 46,
    "share_number": 24014128830,
    "foreign_share": 3786746262,
    "percentage": 15.77
  },
  {
    "industry": "電子零組件業",
    "company_count": 104,
    "share_number": 29250925540,
    "foreign_share": 7078558252,
    "percentage": 24.2
  },
  {
    "industry": "電子通路業",
    "company_count": 22,
    "share_number": 8833953485,
    "foreign_share": 1957111453,
    "percentage": 22.15
  },
  {
    "industry": "資訊服務業",
    "company_count": 11,
    "share_number": 1444906567,
    "foreign_share": 92685007,
    "percentage": 6.41
  },
  {
    "industry": "其他電子業",
    "company_count": 46,
    "share_number": 22843485088,
    "foreign_share": 6922440749,
    "percentage": 30.3
  },
  {
    "industry": "建材營造",
    "company_count": 55,
    "share_number": 29100096429,
    "foreign_share": 3572194957,
    "percentage": 12.28
  },
  {
    "industry": "航運業",
    "company_count": 28,
    "share_number": 38039579607,
    "foreign_share": 6697707428,
    "percentage": 17.61
  },
  {
    "industry": "觀光餐旅",
    "company_count": 19,
    "share_number": 3069101690,
    "foreign_share": 270547313,
    "percentage": 8.82
  },
  {
    "industry
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:22+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48757 |
| latency_ms | 230 |

> 僅供研究參考，不構成投資建議

## get_foreign_shareholding_history.json ✅

- **參數**: `{"symbol": "1101", "range": 3}`
- **http_calls**: 3
- **建議圖表**: `line`

### data

| 欄位 | 值 |
|---|---|
| symbol | 1101 |
| name | 台泥 |
| range | 3 |
| series | `[3 筆] 例: date=2026-07-31, foreign_shares=1087366385, foreign_percent=14.45…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:23+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48752 |
| latency_ms | 4128 |

> 僅供研究參考，不構成投資建議

## get_futures_daily_ohlc.json ✅

- **參數**: `{"contract": "TX"}`
- **http_calls**: 2
- **建議圖表**: `candlestick`

### data

```json
[
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202608",
    "session": "一般",
    "open": 42331,
    "high": 43951,
    "low": 42305,
    "close": 43678,
    "change": 3392,
    "change_pct": 8.42,
    "volume": 91473,
    "settlement": 43727,
    "open_interest": 111302,
    "best_bid": 43676,
    "best_ask": 43688
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202608",
    "session": "盤後",
    "open": 40035,
    "high": 41874,
    "low": 39866,
    "close": 41859,
    "change": 1573,
    "change_pct": 3.9,
    "volume": 49738,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 41835,
    "best_ask": 41860
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202609",
    "session": "一般",
    "open": 42478,
    "high": 44091,
    "low": 42478,
    "close": 43836,
    "change": 3387,
    "change_pct": 8.37,
    "volume": 541,
    "settlement": 43888,
    "open_interest": 5821,
    "best_bid": 43828,
    "best_ask": 43845
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202609",
    "session": "盤後",
    "open": 40192,
    "high": 42024,
    "low": 40036,
    "close": 42000,
    "change": 1551,
    "change_pct": 3.83,
    "volume": 380,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 41979,
    "best_ask": 42011
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202610",
    "session": "一般",
    "open": 42768,
    "high": 44154,
    "low": 42722,
    "close": 44150,
    "change": 3558,
    "change_pct": 8.77,
    "volume": 25,
    "settlement": 44010,
    "open_interest": 141,
    "best_bid": 44002,
    "best_ask": 44019
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202610",
    "session": "盤後",
    "open": 40928,
    "high": 42168,
    "low": 40928,
    "close": 42168,
    "change": 1576,
    "change_pct": 3.88,
    "volume": 45,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 42147,
    "best_ask": 42185
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202612",
    "session": "一般",
    "open": 42888,
    "high": 44514,
    "low": 42888,
    "close": 44514,
    "change": 3595,
    "change_pct": 8.79,
    "volume": 22,
    "settlement": 44306,
    "open_interest": 377,
    "best_bid": 44296,
    "best_ask": 44317
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202612",
    "session": "盤後",
    "open": 41454,
    "high": 42370,
    "low": 41454,
    "close": 42350,
    "change": 1431,
    "change_pct": 3.5,
    "volume": 13,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 42455,
    "best_ask": 42520
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202703",
    "session": "一般",
    "open": 44300,
    "high": 44430,
    "low": 44300,
    "close": 44306,
    "change": 2766,
    "change_pct": 6.66,
    "volume": 5,
    "settlement": 44906,
    "open_interest": 115,
    "best_bid": 44895,
    "best_ask": 44918
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202703",
    "session": "盤後",
    "open": 42910,
    "high": 42962,
    "low": 42898,
    "close": 42962,
    "change": 1422,
    "change_pct": 3.42,
    "volume": 6,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 43075,
    "best_ask": 43135
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202706",
    "session": "一般",
    "open": 44528,
    "high": 45286,
    "low": 44528,
    "close": 45209,
    "change": 2998,
    "change_pct": 7.1,
    "volume": 10,
    "settlement": 45543,
    "open_interest": 62,
    "best_bid": 45526,
    "best_ask": 45561
  },
  {
    "date": "2026-07-31",
    "contract": "TX",
    "contract_month": "202706",
    "session": "盤後",
    "open": 42168,
    "high": 43680,
    "low": 42168,
    "close": 43680,
    "change": 1469,
    "change_pct": 3.48,
    "volume":
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:54+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 1147 |

> 僅供研究參考，不構成投資建議

## get_futures_history.json ✅

- **參數**: `{"contract": "TX", "start": "2026-07-28", "end": "2026-07-31"}`
- **http_calls**: 1
- **建議圖表**: `candlestick`

### data

```json
[
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608",
    "session": "一般",
    "open": 42569,
    "high": 42627,
    "low": 41516,
    "close": 41613,
    "change": -2267,
    "change_pct": -5.17,
    "volume": 81786,
    "settlement": 41573,
    "open_interest": 104993,
    "best_bid": 41609,
    "best_ask": 41614
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608",
    "session": "盤後",
    "open": 43956,
    "high": 44241,
    "low": 42787,
    "close": 43172,
    "change": -708,
    "change_pct": -1.61,
    "volume": 53743,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 43170,
    "best_ask": 43182
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202609",
    "session": "一般",
    "open": 197,
    "high": 197,
    "low": 160,
    "close": 163,
    "change": 0,
    "change_pct": 0,
    "volume": 733,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 160,
    "best_ask": 168
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202609",
    "session": "盤後",
    "open": 190,
    "high": 203,
    "low": 190,
    "close": 203,
    "change": 0,
    "change_pct": 0,
    "volume": 12,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 191,
    "best_ask": 198
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202610",
    "session": "一般",
    "open": 0,
    "high": 0,
    "low": 0,
    "close": 0,
    "change": 0,
    "change_pct": 0,
    "volume": 0,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 401,
    "best_ask": 408
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202610",
    "session": "盤後",
    "open": 409,
    "high": 420,
    "low": 409,
    "close": 420,
    "change": 0,
    "change_pct": 0,
    "volume": 11,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 409,
    "best_ask": 425
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202612",
    "session": "盤後",
    "open": 0,
    "high": 0,
    "low": 0,
    "close": 0,
    "change": 0,
    "change_pct": 0,
    "volume": 0,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 830,
    "best_ask": 876
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202612",
    "session": "一般",
    "open": 865,
    "high": 865,
    "low": 818,
    "close": 818,
    "change": 0,
    "change_pct": 0,
    "volume": 34,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 797,
    "best_ask": 809
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202703",
    "session": "一般",
    "open": 1630,
    "high": 1630,
    "low": 1630,
    "close": 1630,
    "change": 0,
    "change_pct": 0,
    "volume": 1,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 1571,
    "best_ask": 1596
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202703",
    "session": "盤後",
    "open": 0,
    "high": 0,
    "low": 0,
    "close": 0,
    "change": 0,
    "change_pct": 0,
    "volume": 0,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 1570,
    "best_ask": 1639
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202706",
    "session": "一般",
    "open": 2351,
    "high": 2351,
    "low": 2349,
    "close": 2349,
    "change": 0,
    "change_pct": 0,
    "volume": 14,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 2340,
    "best_ask": 2349
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
    "contract_month": "202608/202706",
    "session": "盤後",
    "open": 0,
    "high": 0,
    "low": 0,
    "close": 0,
    "change": 0,
    "change_pct": 0,
    "volume": 0,
    "settlement": 0,
    "open_interest": 0,
    "best_bid": 2400,
    "best_ask": 2460
  },
  {
    "date": "2026-07-28",
    "contract": "TX",
  
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_DL |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:56+08:00 |
| data_date | 2026-07-31 |
| freshness | HISTORICAL |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 0 |
| latency_ms | 5431 |

> 僅供研究參考，不構成投資建議

## get_institutional_futures_history.json ✅

- **參數**: `{"start": "2026-07-28", "end": "2026-07-31"}`
- **http_calls**: 1
- **建議圖表**: `line`

### data

```json
[
  {
    "date": "2026-07-28",
    "contract": "ETF期貨",
    "investor": "外資及陸資",
    "long_volume": 8757,
    "long_value": 5948149000,
    "short_volume": 9137,
    "short_value": 5897802000,
    "net_volume": -380,
    "net_value": 50347000,
    "oi_long": 15932,
    "oi_long_value": 12652051000,
    "oi_short": 3080,
    "oi_short_value": 1930014000,
    "oi_net": 12852,
    "oi_net_value": 10722037000
  },
  {
    "date": "2026-07-28",
    "contract": "半導體30期貨",
    "investor": "外資及陸資",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-28",
    "contract": "富櫃200期貨",
    "investor": "外資及陸資",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 2,
    "short_value": 1618000,
    "net_volume": -2,
    "net_value": -1618000,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 2,
    "oi_short_value": 1614000,
    "oi_net": -2,
    "oi_net_value": -1614000
  },
  {
    "date": "2026-07-28",
    "contract": "小型臺指期貨",
    "investor": "外資及陸資",
    "long_volume": 166168,
    "long_value": 355276368000,
    "short_volume": 172043,
    "short_value": 367977141000,
    "net_volume": -5875,
    "net_value": -12700773000,
    "oi_long": 653,
    "oi_long_value": 1357673000,
    "oi_short": 3753,
    "oi_short_value": 7809380000,
    "oi_net": -3100,
    "oi_net_value": -6451707000
  },
  {
    "date": "2026-07-28",
    "contract": "小型金融期貨",
    "investor": "外資及陸資",
    "long_volume": 231,
    "long_value": 178245000,
    "short_volume": 214,
    "short_value": 165164000,
    "net_volume": 17,
    "net_value": 13081000,
    "oi_long": 40,
    "oi_long_value": 30680000,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 40,
    "oi_net_value": 30680000
  },
  {
    "date": "2026-07-28",
    "contract": "小型電子期貨",
    "investor": "外資及陸資",
    "long_volume": 1001,
    "long_value": 1347221000,
    "short_volume": 1008,
    "short_value": 1358104000,
    "net_volume": -7,
    "net_value": -10884000,
    "oi_long": 40,
    "oi_long_value": 52560000,
    "oi_short": 53,
    "oi_short_value": 69642000,
    "oi_net": -13,
    "oi_net_value": -17082000
  },
  {
    "date": "2026-07-28",
    "contract": "微型臺指期貨",
    "investor": "外資及陸資",
    "long_volume": 241774,
    "long_value": 103531243000,
    "short_volume": 247005,
    "short_value": 105770045000,
    "net_volume": -5231,
    "net_value": -2238803000,
    "oi_long": 1093,
    "oi_long_value": 454393000,
    "oi_short": 5174,
    "oi_short_value": 2151285000,
    "oi_net": -4081,
    "oi_net_value": -1696892000
  },
  {
    "date": "2026-07-28",
    "contract": "東證期貨",
    "investor": "外資及陸資",
    "long_volume": 44,
    "long_value": 35501000,
    "short_volume": 39,
    "short_value": 31552000,
    "net_volume": 5,
    "net_value": 3949000,
    "oi_long": 70,
    "oi_long_value": 55808000,
    "oi_short": 103,
    "oi_short_value": 82096000,
    "oi_net": -33,
    "oi_net_value": -26288000
  },
  {
    "date": "2026-07-28",
    "contract": "櫃買指數期貨",
    "investor": "外資及陸資",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-28",
    "contract": "美國標普500期貨",
    "investor": "外資及陸資",
    "long_volume": 19,
    "long_value": 28303000,
    "short_volume": 12,
    "short_value": 17847000,
    "net_volume": 7,
    "net_value": 10457000,
    "oi_long": 58,
    "oi_long_value": 86223000,
    "oi_short": 63,
    "oi_short_value": 93737000,
    "oi_net": -5,
    "oi_net_value": -7514000
  },
  {
    "date": "2026-07-28",
    "contract": "美國費城半導體期貨",
    "investor": "外資及陸資",
    "long_volume": 69,
    "long_value": 636
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_DL |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:15+08:00 |
| data_date | 2026-07-31 |
| freshness | HISTORICAL |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 0 |
| latency_ms | 6355 |

> 僅供研究參考，不構成投資建議

## get_institutional_futures_positions.json ✅

- **參數**: `{}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

```json
[
  {
    "date": "2026-07-31",
    "contract": "臺股期貨",
    "investor": "自營商",
    "long_volume": 7583,
    "long_value": 65268505000,
    "short_volume": 11958,
    "short_value": 103260466000,
    "net_volume": -4375,
    "net_value": -37991961000,
    "oi_long": 4767,
    "oi_long_value": 41802482000,
    "oi_short": 7144,
    "oi_short_value": 62534469000,
    "oi_net": -2377,
    "oi_net_value": -20731987000
  },
  {
    "date": "2026-07-31",
    "contract": "臺股期貨",
    "investor": "投信",
    "long_volume": 9649,
    "long_value": 84376678000,
    "short_volume": 181,
    "short_value": 1576061000,
    "net_volume": 9468,
    "net_value": 82800618000,
    "oi_long": 88383,
    "oi_long_value": 772944688000,
    "oi_short": 3058,
    "oi_short_value": 26743433000,
    "oi_net": 85325,
    "oi_net_value": 746201255000
  },
  {
    "date": "2026-07-31",
    "contract": "臺股期貨",
    "investor": "外資及陸資",
    "long_volume": 79661,
    "long_value": 678072898000,
    "short_volume": 83953,
    "short_value": 715789471000,
    "net_volume": -4292,
    "net_value": -37716573000,
    "oi_long": 12704,
    "oi_long_value": 111170593000,
    "oi_short": 95219,
    "oi_short_value": 832927612000,
    "oi_net": -82515,
    "oi_net_value": -721757019000
  },
  {
    "date": "2026-07-31",
    "contract": "電子期貨",
    "investor": "自營商",
    "long_volume": 58,
    "long_value": 605108000,
    "short_volume": 104,
    "short_value": 1084670000,
    "net_volume": -46,
    "net_value": -479563000,
    "oi_long": 11,
    "oi_long_value": 122552000,
    "oi_short": 219,
    "oi_short_value": 2438897000,
    "oi_net": -208,
    "oi_net_value": -2316345000
  },
  {
    "date": "2026-07-31",
    "contract": "電子期貨",
    "investor": "投信",
    "long_volume": 13,
    "long_value": 144784000,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 13,
    "net_value": 144784000,
    "oi_long": 211,
    "oi_long_value": 2349780000,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 211,
    "oi_net_value": 2349780000
  },
  {
    "date": "2026-07-31",
    "contract": "電子期貨",
    "investor": "外資及陸資",
    "long_volume": 111,
    "long_value": 1178318000,
    "short_volume": 51,
    "short_value": 544043000,
    "net_volume": 60,
    "net_value": 634275000,
    "oi_long": 25,
    "oi_long_value": 278410000,
    "oi_short": 219,
    "oi_short_value": 2438872000,
    "oi_net": -194,
    "oi_net_value": -2160462000
  },
  {
    "date": "2026-07-31",
    "contract": "金融期貨",
    "investor": "自營商",
    "long_volume": 24,
    "long_value": 77045000,
    "short_volume": 22,
    "short_value": 71106000,
    "net_volume": 2,
    "net_value": 5938000,
    "oi_long": 21,
    "oi_long_value": 67927000,
    "oi_short": 114,
    "oi_short_value": 368744000,
    "oi_net": -93,
    "oi_net_value": -300817000
  },
  {
    "date": "2026-07-31",
    "contract": "金融期貨",
    "investor": "投信",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 38,
    "oi_long_value": 122915000,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 38,
    "oi_net_value": 122915000
  },
  {
    "date": "2026-07-31",
    "contract": "金融期貨",
    "investor": "外資及陸資",
    "long_volume": 205,
    "long_value": 662498000,
    "short_volume": 223,
    "short_value": 721527000,
    "net_volume": -18,
    "net_value": -59030000,
    "oi_long": 131,
    "oi_long_value": 423733000,
    "oi_short": 179,
    "oi_short_value": 578993000,
    "oi_net": -48,
    "oi_net_value": -155260000
  },
  {
    "date": "2026-07-31",
    "contract": "小型臺指期貨",
    "investor": "自營商",
    "long_volume": 13898,
    "long_value": 29761102000,
    "short_volume": 13989,
    "short_value": 29909324000,
    "net_volume": -91,
    "net_value": -148222000,
    "oi_long": 3664,
    "oi_long_value": 8031841000,
    "oi_short": 9904,
    "oi_short_value": 21716350000,
    "oi_net": -6240,
    "oi_net_value": -13684509000
  },
  
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:13+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 126 |

> 僅供研究參考，不構成投資建議

## get_institutional_investors.json ✅

- **參數**: `{"market": "tse"}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| market | tse |
| date | 2026-07-31 |
| rows | `[8 筆] 例: `{"code": "1102", "name": "亞泥", "foreign_buy": 27748409, "foreign_sell": 24462994, "foreign_net": 3285415, "foreign_deal…` |
| total_net | 5883681 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:20+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48759 |
| latency_ms | 581 |

> 僅供研究參考，不構成投資建議

## get_institutional_options_positions.json ✅

- **參數**: `{}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

```json
[
  {
    "date": "2026-07-31",
    "contract": "臺指選擇權",
    "investor": "自營商",
    "long_volume": 161101,
    "long_value": 1711376000,
    "short_volume": 151905,
    "short_value": 1424949000,
    "net_volume": 9196,
    "net_value": 286428000,
    "oi_long": 37051,
    "oi_long_value": 1809671000,
    "oi_short": 37339,
    "oi_short_value": 1646754000,
    "oi_net": -288,
    "oi_net_value": 162917000
  },
  {
    "date": "2026-07-31",
    "contract": "臺指選擇權",
    "investor": "投信",
    "long_volume": 2,
    "long_value": 32000,
    "short_volume": 1367,
    "short_value": 158132000,
    "net_volume": -1365,
    "net_value": -158100000,
    "oi_long": 9,
    "oi_long_value": 595000,
    "oi_short": 2628,
    "oi_short_value": 308134000,
    "oi_net": -2619,
    "oi_net_value": -307539000
  },
  {
    "date": "2026-07-31",
    "contract": "臺指選擇權",
    "investor": "外資及陸資",
    "long_volume": 221395,
    "long_value": 1549217000,
    "short_volume": 214805,
    "short_value": 1457924000,
    "net_volume": 6590,
    "net_value": 91293000,
    "oi_long": 19555,
    "oi_long_value": 901677000,
    "oi_short": 21695,
    "oi_short_value": 989936000,
    "oi_net": -2140,
    "oi_net_value": -88259000
  },
  {
    "date": "2026-07-31",
    "contract": "電子選擇權",
    "investor": "自營商",
    "long_volume": 2,
    "long_value": 44000,
    "short_volume": 4,
    "short_value": 93000,
    "net_volume": -2,
    "net_value": -49000,
    "oi_long": 156,
    "oi_long_value": 4832000,
    "oi_short": 98,
    "oi_short_value": 2438000,
    "oi_net": 58,
    "oi_net_value": 2394000
  },
  {
    "date": "2026-07-31",
    "contract": "電子選擇權",
    "investor": "投信",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-31",
    "contract": "電子選擇權",
    "investor": "外資及陸資",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-31",
    "contract": "金融選擇權",
    "investor": "自營商",
    "long_volume": 238,
    "long_value": 4927000,
    "short_volume": 146,
    "short_value": 2767000,
    "net_volume": 92,
    "net_value": 2160000,
    "oi_long": 217,
    "oi_long_value": 4087000,
    "oi_short": 132,
    "oi_short_value": 2394000,
    "oi_net": 85,
    "oi_net_value": 1693000
  },
  {
    "date": "2026-07-31",
    "contract": "金融選擇權",
    "investor": "投信",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-31",
    "contract": "金融選擇權",
    "investor": "外資及陸資",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_value": 0,
    "oi_net": 0,
    "oi_net_value": 0
  },
  {
    "date": "2026-07-31",
    "contract": "股票選擇權",
    "investor": "自營商",
    "long_volume": 40,
    "long_value": 2107000,
    "short_volume": 51,
    "short_value": 7026000,
    "net_volume": -11,
    "net_value": -4919000,
    "oi_long": 1419,
    "oi_long_value": 54134000,
    "oi_short": 1480,
    "oi_short_value": 91205000,
    "oi_net": -61,
    "oi_net_value": -37071000
  },
  {
    "date": "2026-07-31",
    "contract": "股票選擇權",
    "investor": "投信",
    "long_volume": 0,
    "long_value": 0,
    "short_volume": 0,
    "short_value": 0,
    "net_volume": 0,
    "net_value": 0,
    "oi_long": 0,
    "oi_long_value": 0,
    "oi_short": 0,
    "oi_short_va
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:14+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 50 |

> 僅供研究參考，不構成投資建議

## get_intraday_kline.json ❌

- **參數**: `{"symbol": "2330", "timeframe": "1m", "limit": 10}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料

## get_intraday_quote.json ❌

- **參數**: `{"symbol": "2330"}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料

## get_intraday_vwap.json ❌

- **參數**: `{"symbol": "2330"}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料

## get_large_trader_positions.json ✅

- **參數**: `{}`
- **http_calls**: 3
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| date | 2026-07-31 |
| futures | `[1366 筆] 例: `{"date": "2026-07-31", "contract": "BRF", "contract_name": "布蘭特原油期貨", "contract_month": "202609", "trader_type": "0", "…` |
| options | `[328 筆] 例: `{"date": "2026-07-31", "contract": "CA", "contract_name": "南亞", "contract_month": "666666", "call_put": "買權", "trader_t…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_DL |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:04+08:00 |
| data_date | 2026-07-31 |
| freshness | HISTORICAL |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 7855 |

> 僅供研究參考，不構成投資建議

## get_major_announcements.json ✅

- **參數**: `{}`
- **http_calls**: 1

### data

```json
[
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "305",
    "code": "4414",
    "name": "如興",
    "subject": "代子公司依公開發行公司資金貸與及背書保證處理準則\n第二十五條第一項第四款及證交所臺證上一字\n第1111803907號函指示公告",
    "clause": "第22款",
    "fact_date": "2026-07-30",
    "description": "1.事實發生日:115/07/30\n2.被背書保證之:\n(1)公司名稱:常州東奧服裝有限公司\n(2)與提供背書保證公司之關係:\n與提供保證常州穗滿國際貿易有限公司同為本公司直接及間接持有100%之子公司\n(3)背書保證之限額(仟元):4,969,174\n(4)原背書保證之餘額(仟元):3,386,180\n(5)本次新增背書保證之金額(仟元):234,500\n(6)迄事實發生日止背書保證餘額(仟元):3,620,680\n(7)被背書保證公司實際動支金額(仟元):1,050,657\n(8)本次新增背書保證之原因:\n集團資金調度需要。\n3.被背書保證公司提供擔保品之:\n(1)內容:\n無\n(2)價值(仟元):0\n4.被背書保證公司最近期財務報表之:\n(1)資本(仟元):955,500\n(2)累積盈虧金額(仟元):-924,242\n5.解除背書保證責任之:\n(1)條件:\n依合約簽訂之日起為期一年。\n(2)日期:\n依合約簽訂之日起為期一年。\n6.背書保證之總限額(仟元):\n16,215,120\n7.迄事實發生日為止，背書保證餘額(仟元):\n8,572,688\n8.迄事實發生日為止，A提供背書保證餘額占公開發行公司最近期財務報表淨值之\n比率:\n211.47\n9.迄事實發生日為止，背書保證、長期投資及資金貸與餘額合計數達該公開發行公\n司最近期財務報表淨值之比率:\n20.31\n10.其他應敘明事項:\n無"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "70:00:3:",
    "code": "3708",
    "name": "上緯投控",
    "subject": "公告本公司名稱由「上緯國際投資控股股份有限公司」更名為\n「上緯國際控股股份有限公司」，公告期間：115年6月11日至\n115年9月10日。",
    "clause": "第51款",
    "fact_date": "2026-06-11",
    "description": "1.事實發生日：民國115年06月11日\n2.公司名稱：上緯國際控股股份有限公司(原名：上緯國際投資控股股份有限公司)\n3.與公司關係(請輸入本公司或子公司)：本公司\n4.相互持股比例：不適用\n5.發生緣由：\n(1)公司名稱變更核准日期/事實發生日：民國115年06月11日\n(2)公司名稱變更核准文號：經授商字第11530084540號函\n(3)更名案股東會決議通過日期：民國115年05月28日\n(4)變更前公司名稱：上緯國際投資控股股份有限公司\n(5)變更後公司名稱：上緯國際控股股份有限公司\n(6)變更前公司簡稱：上緯投控\n(7)變更後公司簡稱：上緯控股\n6.因應措施：無\n7.其他應敘明事項：(1)本公司於115/6/11收到經濟部變更登記核准函。\n(2)依據臺灣證券交易所股份有限公司營業細則第45條規定，於更\n名後須連續公告三個月。\n(3)本公司股票代號未變動，普通股仍為「3708」"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "70:00:3:",
    "code": "1721",
    "name": "三晃",
    "subject": "公告本公司名稱由「三晃股份有限公司」更名為\n「國慶科技股份有限公司」\n，公告期間：115年07月01日至115年9月30日。",
    "clause": "第51款",
    "fact_date": "2026-06-29",
    "description": "1.事實發生日：民國115年06月29日\n2.公司名稱：國慶科技股份有限公司(原名：三晃股份有限公司)\n3.與公司關係(請輸入本公司或子公司)：本公司\n4.相互持股比例：不適用\n5.發生緣由：\n(1)公司名稱變更核准日期/事實發生日：民國115年06月29日\n(2)公司名稱變更核准文號：11530099260\n(3)更名案股東會決議通過日期：民國115年06月16日\n(4)變更前公司名稱：三晃股份有限公司\n(5)變更後公司名稱：國慶科技股份有限公司\n(6)變更前公司簡稱：三晃\n(7)變更後公司簡稱：國慶科技\n6.因應措施：無\n7.其他應敘明事項：(1)本公司於115/07/01收到經濟部變更登記核准函。\n(2)依據臺灣證券交易所股份有限公司營業細則第45條規定，於更名後須連續公告三個月。\n(3)本公司股票代號未變動，普通股仍為「1721」。\n(4)本公司英文名未更動，仍為「SUNKO INK CO., LTD.」"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "83:43:9:",
    "code": "1220",
    "name": "台榮",
    "subject": "公告本公司技術總廠長退休",
    "clause": "第8款",
    "fact_date": "2026-07-31",
    "description": "1.人員變動別（請輸入發言人、代理發言人、重要營運主管(如:執行長、營運長、\n行銷長及策略長等)、財務主管、會計主管、公司治理主管、資訊安全長、研發主管、\n內部稽核主管或訴訟及非訟代理人）:技術總廠長\n2.發生變動日期:115/07/31\n3.舊任者姓名、級職及簡歷:林永富 / 協理\n4.新任者姓名、級職及簡歷:不適用\n5.異動情形（請輸入「辭職」、「職務調整」、「資遣」、「退休」、「死亡」、「新\n任」或「解任」）:退休\n6.異動原因:退休\n7.生效日期:115/07/31\n8.其他應敘明事項:無"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "85:01:6:",
    "code": "1321",
    "name": "大洋",
    "subject": "公告本公司民國115年第2季合併財務報告提報董事會預計召開日期\n為115年08月07日",
    "clause": "第31款",
    "fact_date": "2026-07-30",
    "description": "1.董事會召集通知日:115/07/30\n2.董事會預計召開日期:115/08/07\n3.預計提報董事會或經董事會決議之財務報告或\n年度自結財務資訊年季:115年第2季合併財務報告\n4.其他應敘明事項:無"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "91:90:9:",
    "code": "2109",
    "name": "華豐",
    "subject": "公告本公司115年第二季財務報告董事會預計召開日期為\n115年8月10日。",
    "clause": "第31款",
    "fact_date": "2026-07-31",
    "description": "1.董事會召集通知日:115/07/31\n2.董事會預計召開日期:115/08/10\n3.預計提報董事會或經董事會決議之財務報告或\n年度自結財務資訊年季:115年第二季\n4.其他應敘明事項:無。"
  },
  {
    "table_date": "2026-08-01",
    "announce_date": "2026-07-31",
    "announce_time": "92:02:6:",
    "code": "6139",
    "name": "亞翔",
    "subject": "代子公司亞翔系統集成科技(蘇州)股份有限公司\n公告董事會決議分配股利",
    "clause": "第14款",
    "fact_date": "2026-07-30",
    "description": "1.董事會決議日期:115/07/30\n2.發放股利種類及金額:\n(1)中期每10股配
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | MOPS |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:33+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 300 |
| latency_ms | 178 |

> 僅供研究參考，不構成投資建議

## get_margin_trading.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| code | 2330 |
| name | 台積電 |
| margin_buy | 1340000 |
| margin_sell | 2674000 |
| margin_cash_redeem | 41000 |
| margin_prev_balance | 30664000 |
| margin_balance | 29289000 |
| margin_limit | 6483092000 |
| short_buy | 7000 |
| short_sell | 25000 |
| short_cash_redeem | 0 |
| short_prev_balance | 130000 |
| … | 共 15 鍵（其餘省略） |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:28+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48749 |
| latency_ms | 1155 |

> 僅供研究參考，不構成投資建議

## get_market_summary.json ✅

- **參數**: `{}`
- **http_calls**: 2
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| date | 2026-07-31 |
| tse | advancers=0, decliners=0, unchanged=31267, limit_up=0, limit_down=0, total_volume=13813496579, total_amount=887732085792 |
| otc | advancers=870, decliners=80, unchanged=48, limit_up=14, limit_down=0, total_volume=948460000, total_amount=0 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:17+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48760 |
| latency_ms | 2713 |

> 僅供研究參考，不構成投資建議

## get_monthly_revenue.json ✅

- **參數**: `{"symbol": "2330", "years": 2}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| symbol | 2330 |
| name | 台積電 |
| market | tse |
| rows | `{"table_date": "2026-07-17", "data_year_month": "202606", "code": "2330", "name": "台積電", "industry": "半導體業", "revenue": 442679969000, "last_month_revenue": 416975163000, "last_year…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | MOPS |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:41+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 43200 |
| latency_ms | 1025 |

> 僅供研究參考，不構成投資建議

## get_put_call_ratio.json ✅

- **參數**: `{}`
- **http_calls**: 2
- **建議圖表**: `line`

### data

```json
[
  {
    "date": "2026-07-31",
    "call_volume": 364487,
    "put_volume": 351655,
    "volume_ratio": 96.48,
    "call_oi": 50049,
    "put_oi": 53689,
    "oi_ratio": 107.27
  }
]
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TAIFEX_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:55:43+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 1027 |

> 僅供研究參考，不構成投資建議

## get_stock_daily_kline.json ✅

- **參數**: `{"symbol": "2330", "period": "day"}`
- **http_calls**: 1
- **建議圖表**: `candlestick`

### data

```json
[
  {
    "timestamp": "2026-07-01",
    "open": 2495,
    "high": 2505,
    "low": 2475,
    "close": 2505,
    "volume": 37544470,
    "amount": 93600076825
  },
  {
    "timestamp": "2026-07-02",
    "open": 2450,
    "high": 2480,
    "low": 2445,
    "close": 2465,
    "volume": 35919290,
    "amount": 88369879773
  },
  {
    "timestamp": "2026-07-03",
    "open": 2415,
    "high": 2465,
    "low": 2415,
    "close": 2445,
    "volume": 32905868,
    "amount": 80082636340
  },
  {
    "timestamp": "2026-07-06",
    "open": 2465,
    "high": 2500,
    "low": 2455,
    "close": 2460,
    "volume": 21041918,
    "amount": 52022654454
  },
  {
    "timestamp": "2026-07-07",
    "open": 2480,
    "high": 2500,
    "low": 2440,
    "close": 2440,
    "volume": 31400854,
    "amount": 77617188273
  },
  {
    "timestamp": "2026-07-08",
    "open": 2445,
    "high": 2465,
    "low": 2420,
    "close": 2465,
    "volume": 25519599,
    "amount": 62400639776
  },
  {
    "timestamp": "2026-07-09",
    "open": 2450,
    "high": 2460,
    "low": 2415,
    "close": 2415,
    "volume": 34681018,
    "amount": 84397735035
  },
  {
    "timestamp": "2026-07-13",
    "open": 2460,
    "high": 2480,
    "low": 2440,
    "close": 2440,
    "volume": 35310380,
    "amount": 86697387632
  },
  {
    "timestamp": "2026-07-14",
    "open": 2410,
    "high": 2430,
    "low": 2390,
    "close": 2420,
    "volume": 42857055,
    "amount": 103370532441
  },
  {
    "timestamp": "2026-07-15",
    "open": 2425,
    "high": 2460,
    "low": 2415,
    "close": 2440,
    "volume": 33665566,
    "amount": 82213791038
  },
  {
    "timestamp": "2026-07-16",
    "open": 2430,
    "high": 2470,
    "low": 2420,
    "close": 2470,
    "volume": 30538604,
    "amount": 74750491934
  },
  {
    "timestamp": "2026-07-17",
    "open": 2375,
    "high": 2395,
    "low": 2290,
    "close": 2290,
    "volume": 97362670,
    "amount": 229051751965
  },
  {
    "timestamp": "2026-07-20",
    "open": 2300,
    "high": 2345,
    "low": 2300,
    "close": 2320,
    "volume": 55790346,
    "amount": 129815956839
  },
  {
    "timestamp": "2026-07-21",
    "open": 2350,
    "high": 2410,
    "low": 2345,
    "close": 2410,
    "volume": 31605663,
    "amount": 75429747048
  },
  {
    "timestamp": "2026-07-22",
    "open": 2440,
    "high": 2445,
    "low": 2385,
    "close": 2400,
    "volume": 31653123,
    "amount": 76206482625
  },
  {
    "timestamp": "2026-07-23",
    "open": 2385,
    "high": 2405,
    "low": 2370,
    "close": 2405,
    "volume": 28001492,
    "amount": 66833713878
  },
  {
    "timestamp": "2026-07-24",
    "open": 2355,
    "high": 2365,
    "low": 2345,
    "close": 2350,
    "volume": 24810509,
    "amount": 58407263735
  },
  {
    "timestamp": "2026-07-27",
    "open": 2330,
    "high": 2365,
    "low": 2330,
    "close": 2350,
    "volume": 28939466,
    "amount": 67885214772
  },
  {
    "timestamp": "2026-07-28",
    "open": 2270,
    "high": 2305,
    "low": 2270,
    "close": 2280,
    "volume": 45333029,
    "amount": 103624205064
  },
  {
    "timestamp": "2026-07-29",
    "open": 2260,
    "high": 2280,
    "low": 2180,
    "close": 2200,
    "volume": 68139691,
    "amount": 151551634905
  },
  {
    "timestamp": "2026-07-30",
    "open": 2205,
    "high": 2260,
    "low": 2190,
    "close": 2205,
    "volume": 51372177,
    "amount": 114098819878
  },
  {
    "timestamp": "2026-07-31",
    "open": 2350,
    "high": 2425,
    "low": 2345,
    "close": 2425,
    "volume": 69478145,
    "amount": 166661984712
  }
]
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:15+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48763 |
| latency_ms | 647 |

> 僅供研究參考，不構成投資建議

## get_stock_daily_quote.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 3
- **建議圖表**: `line`

### data

| 欄位 | 值 |
|---|---|
| symbol | 2330 |
| name | 台積電 |
| market | tse |
| date | 2026-07-31 |
| open | 2350 |
| high | 2425 |
| low | 2345 |
| close | 2425 |
| volume | 69478145 |
| amount | 166661984712 |
| indicators | ma20=2381.5, ma60=2348.0833333333335, rsi14=54.41799689320122, macd=macd=-22.06017839272181, signal=-14.388572823294608, hist=-7.671605569427202 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| derived_from | TWSE_WEB:daily_k |
| fetched_at | 2026-08-01T18:27:10+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48765 |
| latency_ms | 4392 |

> 僅供研究參考，不構成投資建議

## get_symbol_list.json ✅

- **參數**: `{"market": "tse"}`
- **http_calls**: 0

### data

```json
[
  {
    "code": "1101",
    "market": "tse",
    "name": "台泥",
    "category": "水泥工業"
  },
  {
    "code": "1102",
    "market": "tse",
    "name": "亞泥",
    "category": "水泥工業"
  },
  {
    "code": "1103",
    "market": "tse",
    "name": "嘉泥",
    "category": "水泥工業"
  },
  {
    "code": "1104",
    "market": "tse",
    "name": "環泥",
    "category": "水泥工業"
  },
  {
    "code": "1108",
    "market": "tse",
    "name": "幸福",
    "category": "水泥工業"
  },
  {
    "code": "1109",
    "market": "tse",
    "name": "信大",
    "category": "水泥工業"
  },
  {
    "code": "1110",
    "market": "tse",
    "name": "東泥",
    "category": "水泥工業"
  },
  {
    "code": "1201",
    "market": "tse",
    "name": "味全",
    "category": "食品工業"
  },
  {
    "code": "1203",
    "market": "tse",
    "name": "味王",
    "category": "食品工業"
  },
  {
    "code": "1210",
    "market": "tse",
    "name": "大成",
    "category": "食品工業"
  },
  {
    "code": "1213",
    "market": "tse",
    "name": "大飲",
    "category": "食品工業"
  },
  {
    "code": "1215",
    "market": "tse",
    "name": "卜蜂",
    "category": "食品工業"
  },
  {
    "code": "1216",
    "market": "tse",
    "name": "統一",
    "category": "食品工業"
  },
  {
    "code": "1217",
    "market": "tse",
    "name": "愛之味",
    "category": "食品工業"
  },
  {
    "code": "1218",
    "market": "tse",
    "name": "泰山",
    "category": "食品工業"
  },
  {
    "code": "1219",
    "market": "tse",
    "name": "福壽",
    "category": "食品工業"
  },
  {
    "code": "1220",
    "market": "tse",
    "name": "台榮",
    "category": "食品工業"
  },
  {
    "code": "1225",
    "market": "tse",
    "name": "福懋油",
    "category": "食品工業"
  },
  {
    "code": "1227",
    "market": "tse",
    "name": "佳格",
    "category": "食品工業"
  },
  {
    "code": "1229",
    "market": "tse",
    "name": "聯華",
    "category": "食品工業"
  },
  {
    "code": "1231",
    "market": "tse",
    "name": "聯華食",
    "category": "食品工業"
  },
  {
    "code": "1232",
    "market": "tse",
    "name": "大統益",
    "category": "食品工業"
  },
  {
    "code": "1233",
    "market": "tse",
    "name": "天仁",
    "category": "食品工業"
  },
  {
    "code": "1234",
    "market": "tse",
    "name": "黑松",
    "category": "食品工業"
  },
  {
    "code": "1235",
    "market": "tse",
    "name": "興泰",
    "category": "食品工業"
  },
  {
    "code": "1236",
    "market": "tse",
    "name": "宏亞",
    "category": "食品工業"
  },
  {
    "code": "1256",
    "market": "tse",
    "name": "鮮活果汁-KY",
    "category": "食品工業"
  },
  {
    "code": "1301",
    "market": "tse",
    "name": "台塑",
    "category": "塑膠工業"
  },
  {
    "code": "1303",
    "market": "tse",
    "name": "南亞",
    "category": "塑膠工業"
  },
  {
    "code": "1304",
    "market": "tse",
    "name": "台聚",
    "category": "塑膠工業"
  },
  {
    "code": "1305",
    "market": "tse",
    "name": "華夏",
    "category": "塑膠工業"
  },
  {
    "code": "1307",
    "market": "tse",
    "name": "三芳",
    "category": "塑膠工業"
  },
  {
    "code": "1308",
    "market": "tse",
    "name": "亞聚",
    "category": "塑膠工業"
  },
  {
    "code": "1309",
    "market": "tse",
    "name": "台達化",
    "category": "塑膠工業"
  },
  {
    "code": "1310",
    "market": "tse",
    "name": "台苯",
    "category": "塑膠工業"
  },
  {
    "code": "1312",
    "market": "tse",
    "name": "國喬",
    "category": "塑膠工業"
  },
  {
    "code": "1313",
    "market": "tse",
    "name": "聯成",
    "category": "塑膠工業"
  },
  {
    "code": "1314",
    "market": "tse",
    "name": "中石化",
    "category": "塑膠工業"
  },
  {
    "code": "1315",
    "market": "tse",
    "name": "達新",
    "category": "塑膠工業"
  },
  {
    "code": "1316",
    "market": "tse",
    "name": "上曜",
    "category": "建材營造"
  },
  {
    "code": "1319",
    "market": "tse",
    "name": "東陽",
    "category": "汽車工業"
  },
  {
    "code": "1321",
    "market": "tse",
    "name": "大洋",
    "category": "塑膠工業"
  },
  {
    "code": "1323",
    "market": "tse",
    "name": "永裕",
    "category": "塑膠工業"
  },
  {
    "code": "1324",
    "market": "tse",
    "name": "地球",
    "catego
  …（截斷）
```

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:49:15+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 0 |

> 僅供研究參考，不構成投資建議

## get_trading_calendar.json ✅

- **參數**: `{}`
- **http_calls**: 0

### data

| 欄位 | 值 |
|---|---|
| year | 2026 |
| trading_days | `[243 筆] 例: 2026-01-02…` |
| holidays | `[24 筆] 例: date=2026-01-01, name=中華民國開國紀念日…` |
| note | 行事曆版本 TWSE-holidaySchedule-2026-01-01 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_WEB |
| source_role | canonical |
| fetched_at | 2026-08-01T18:28:24+08:00 |
| data_date | 2026-08-01 |
| freshness | HISTORICAL |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 0 |
| latency_ms | 0 |

> 僅供研究參考，不構成投資建議

## get_valuation_ratios.json ✅

- **參數**: `{"symbol": "2330"}`
- **http_calls**: 0

### data

| 欄位 | 值 |
|---|---|
| symbol | 2330 |
| name | 台積電 |
| market | tse |
| date | 2026-07-31 |
| pe | 32.6 |
| pe_available | True |
| pb | 10.67 |
| dividend_yield_pct | 0.91 |
| roe_pct | 0 |
| roe_method |  |
| dividend_per_share | 7 |
| note | ROE 計算失敗：無損益表摘要 |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:47+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 48732 |
| latency_ms | 0 |

> 僅供研究參考，不構成投資建議

## get_warrant_activity.json ✅

- **參數**: `{"top_n": 5}`
- **http_calls**: 1
- **建議圖表**: `bar`

### data

| 欄位 | 值 |
|---|---|
| date | 2026-07-31 |
| amount_top | `[5 筆] 例: trade_date=2026-07-31, code=03732B, name=臺股指元大61熊24, amount=11108350000, volume=14114000000…` |
| volume_top | `[5 筆] 例: trade_date=2026-07-31, code=03732B, name=臺股指元大61熊24, amount=11108350000, volume=14114000000…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| fetched_at | 2026-08-01T18:27:32+08:00 |
| data_date | 2026-07-31 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | False |
| cache_ttl | 48747 |
| latency_ms | 421 |

> 僅供研究參考，不構成投資建議

## scan_daytrade_eligibility.json ❌

- **參數**: `{"symbol": "2330"}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料

## screen_high_yield.json ✅

- **參數**: `{"limit": 5}`
- **http_calls**: 0
- **建議圖表**: `scatter`

### data

| 欄位 | 值 |
|---|---|
| total | 1969 |
| matched | 5 |
| limit | 5 |
| rows | `[5 筆] 例: `{"code": "6596", "name": "寬宏藝術", "market": "otc", "pe": 5.15, "pe_available": true, "pb": 3.24, "dividend_yield_pct": 2…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| derived_from | `[5 筆] 例: TWSE_API:valuation…` |
| fetched_at | 2026-08-01T18:27:53+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 48726 |
| latency_ms | 6 |

> 僅供研究參考，不構成投資建議

## screen_stocks.json ✅

- **參數**: `{"limit": 5}`
- **http_calls**: 1
- **建議圖表**: `scatter`

### data

| 欄位 | 值 |
|---|---|
| total | 1969 |
| matched | 5 |
| limit | 5 |
| rows | `[5 筆] 例: `{"code": "4523", "name": "永彰", "market": "otc", "pe": 1.06, "pe_available": true, "pb": 0.76, "dividend_yield_pct": 10.…` |

### _lineage

| 欄位 | 值 |
|---|---|
| source | TWSE_API |
| source_role | canonical |
| derived_from | `[5 筆] 例: TWSE_API:valuation…` |
| fetched_at | 2026-08-01T18:27:50+08:00 |
| data_date | 2026-08-01 |
| freshness | POST_MARKET_TODAY |
| sampling_sec | 0 |
| is_cached | True |
| cache_ttl | 48729 |
| latency_ms | 68 |

> 僅供研究參考，不構成投資建議

## set_active_watchlist.json ❌

- **參數**: `{"symbols": ["2330", "2317", "2454"]}`
- **錯誤**: 非交易時段（2026-08-01 為非交易日）無法提供盤中資料
