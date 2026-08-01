package provider

// 契約測試框架（§13 測試策略 / T019）：以 golden fixtures（testdata/ 下官方
// raw response）離線驗證所有 Adapter 之 Normalize 輸出符合 §5 Schema 歸一化：
//
//   - 欄位命名：snake_case、全小寫（§5.1）
//   - 時間：純日期 YYYY-MM-DD；盤中 K 線 HH:MM:00；時間戳 RFC3339（Asia/Taipei）
//   - 價格：一律「元」（float64，2 位小數；缺值以 0/null）
//   - 成交量：一律「股」（int64，≥ 0）
//   - 成交值：一律「元」（int64，≥ 0）
//   - 百分比：一律 %（finite；可為負——如負成長）
//   - 缺值：null（禁止空字串代表缺值）
//
// 不連網；官方格式改版時更新 fixtures（cmd/fixtures）後重跑即可（§13 備註）。

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// snakeRE 為 §5.1 欄位命名規則（snake_case、全小寫、允許 is_/can_ 前綴）。
var snakeRE = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// dateREs 為 §5.1 時間格式（純日期 / 盤中 K 線 / RFC3339）。
var (
	dateOnlyRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	clockRE    = regexp.MustCompile(`^\d{2}:\d{2}:00$`)
	rfc3339RE  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)
	// yearMonthRE 為 YYYYMM（月營收 data_year_month）。
	yearMonthRE = regexp.MustCompile(`^\d{6}$`)
)

// priceKeys 為 §5.1 價格語意欄位（元）。
var priceKeys = map[string]bool{
	"open": true, "high": true, "low": true, "close": true,
	"last": true, "prev_close": true, "previous_close": true,
	"price": true, "open_price": true, "high_price": true, "low_price": true,
	"close_price": true, "avg_price": true, "avg": true, "vwap": true,
	"bid_price": true, "ask_price": true, "y": true, "z": true,
	"subscription_price": true, "par_value": true,
}

// volumeKeys 為 §5.1 成交量語意欄位（股，int64）。
var volumeKeys = map[string]bool{
	"volume": true, "vol": true, "minute_vol": true, "cumulative_vol": true,
	"trade_volume": true, "shares": true, "issue_shares": true,
	"bid_volume": true, "ask_volume": true, "outstanding_shares": true,
	"matched_volume": true, "market_volume": true,
}

// amountKeys 為 §5.1 成交值語意欄位（元，int64）。
var amountKeys = map[string]bool{
	"amount": true, "trade_value": true, "market_value": true,
	"cash_dividend": true, "revenue": true, "capital": true,
	"total_assets": true, "current_assets": true, "non_current_assets": true,
	"total_liabilities": true, "current_liabilities": true,
	"non_current_liabilities": true, "total_equity": true,
	"operating_cash_flow": true, "investing_cash_flow": true,
	"financing_cash_flow": true, "ending_cash_balance": true,
	"net_income": true, "operating_profit": true, "pretax_income": true,
	"cash_total": true, "net_income_after_tax": true,
}

// percentKeys 為 §5.1 百分比語意欄位（%）。
var percentKeys = map[string]bool{
	"change_pct": true, "mom_change_pct": true, "yoy_change_pct": true,
	"cum_change_pct": true, "gross_margin_pct": true, "operating_margin_pct": true,
	"pretax_margin_pct": true, "net_margin_pct": true, "roe_pct": true,
	"roa_pct": true, "dividend_yield": true, "yield_ratio": true,
	"foreign_percent": true, "pe": true, "pb": true, "pc_ratio": true,
}

// dateKeys 為 §5.1 時間語意欄位。
var dateKeys = map[string]bool{
	"timestamp": true, "date": true, "data_date": true, "table_date": true,
	"report_date": true, "trade_date": true, "record_date": true,
	"year_month": true, "fetched_at": true, "dividend_date": true,
	"ex_date": true, "last_date": true, "pay_date": true, "announce_date": true,
}

// contractCase 描述單一 Normalize 輸出之契約驗證。
type contractCase struct {
	name    string // 測試名稱
	dataset string // 資料集 ID（紀錄用）
	fixture string // testdata/<dir>/<file>
	rawURL  string // 來源 URL（資料集分派與紀錄）
	// normalize 執行 Normalize（nil 時用 source.Normalize）。
	normalize func(raw *RawResponse) ([]byte, error)
	// extra 為資料集特定之深入斷言（回傳已解析之輸出）。
	extra func(t *testing.T, data any)
	// allowNegPrice 為 true 時放行「價格語意欄位為負」之官方合法值。
	// 僅用於 TAIFEX 價差契約（如 CHF 202608/202609 開盤價 -0.04）：
	// 價差為兩契約之價差，官方本就可能為負，§5.1 負價格攔截在此不適用。
	allowNegPrice bool
}

// contractRaw 由 fixture 建構 RawResponse。
func contractRaw(t *testing.T, fixture string) *RawResponse {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", fixture, err)
	}
	return &RawResponse{Body: b, StatusCode: 200}
}

// runContractCases 執行契約測試框架之所有案例（離線）。
func runContractCases(t *testing.T, cases []contractCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			raw := contractRaw(t, tc.fixture)
			var out []byte
			var err error
			if tc.normalize != nil {
				out, err = tc.normalize(raw)
			} else {
				t.Fatalf("案例 %s 未提供 normalize", tc.name)
			}
			if err != nil {
				t.Fatalf("Normalize 失敗: %v", err)
			}
			var data any
			if err := json.Unmarshal(out, &data); err != nil {
				t.Fatalf("Normalize 輸出非合法 JSON: %v", err)
			}
			if err := checkContract(data, tc.allowNegPrice); err != nil {
				t.Errorf("§5 契約違反: %v", err)
			}
			if tc.extra != nil {
				tc.extra(t, data)
			}
		})
	}
}

// checkContract 遞迴驗證 §5 規則（欄位命名/型別/單位/日期格式）。
func checkContract(v any, allowNegPrice bool) error {
	switch n := v.(type) {
	case map[string]any:
		for k, val := range n {
			if !snakeRE.MatchString(k) {
				return fmt.Errorf("欄位名稱非 snake_case: %q", k)
			}
			if err := checkValue(k, val, allowNegPrice); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range n {
			if err := checkContract(item, allowNegPrice); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkValue 驗證單一欄位之型別/單位/日期格式。
func checkValue(key string, v any, allowNegPrice bool) error {
	switch val := v.(type) {
	case map[string]any:
		return checkContract(val, allowNegPrice)
	case []any:
		for _, item := range val {
			if err := checkValue(key, item, allowNegPrice); err != nil {
				return err
			}
		}
		return nil
	case string:
		if val == "" {
			// §5.1：禁止空字串代表缺值（缺值用 null）
			return fmt.Errorf("欄位 %q 以空字串代表缺值（應為 null）", key)
		}
		if dateKeys[key] {
			switch {
			case dateOnlyRE.MatchString(val):
			case clockRE.MatchString(val):
			case rfc3339RE.MatchString(val):
			case yearMonthRE.MatchString(val):
			default:
				return fmt.Errorf("欄位 %q 日期格式不符 §5.1: %q", key, val)
			}
		}
		return nil
	case float64:
		if priceKeys[key] && val < 0 && !allowNegPrice {
			return fmt.Errorf("欄位 %q 價格為負: %v", key, val)
		}
		if percentKeys[key] && math.IsNaN(val) || percentKeys[key] && math.IsInf(val, 0) {
			return fmt.Errorf("欄位 %q 百分比非 finite: %v", key, val)
		}
		return nil
	case int64:
		if volumeKeys[key] && val < 0 {
			return fmt.Errorf("欄位 %q 成交量為負: %d", key, val)
		}
		if amountKeys[key] && val < 0 {
			return fmt.Errorf("欄位 %q 成交值為負: %d", key, val)
		}
		return nil
	case nil:
		return nil
	default:
		return nil
	}
}

// contractSourceURL 建構各來源之紀錄 URL（資料集分派用）。
func contractSourceURL(base, path, query string) string {
	if query != "" {
		return base + path + "?" + query
	}
	return base + path
}

// ************** 各 Adapter 契約案例 **************

// twseContractCases 涵蓋 TWSE_WEB/TWSE_API 代表資料集（§13 契約測試）。
func twseContractCases() []contractCase {
	web := func(ds TWSEWebDataset, path string, q, fixture string) contractCase {
		return contractCase{
			name:    string(ds) + "_" + fixture,
			dataset: string(ds),
			fixture: "twse/" + fixture,
			rawURL:  contractSourceURL(twseWebBase, path, q),
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = contractSourceURL(twseWebBase, path, q)
				return normalizeTWSE(raw, model.SourceTWSEWeb)
			},
		}
	}
	api := func(ds TWSEAPIDataset, path, fixture string) contractCase {
		return contractCase{
			name:    string(ds) + "_" + fixture,
			dataset: string(ds),
			fixture: "twse/" + fixture,
			rawURL:  contractSourceURL(twseAPIBase, path, ""),
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = contractSourceURL(twseAPIBase, path, "")
				return normalizeTWSE(raw, model.SourceTWSEAPI)
			},
		}
	}
	return []contractCase{
		web(TWSEWDDailyK, "/rwd/afterTrading/STOCK_DAY", "date=20260731&stockNo=2330&response=json", "daily_k_2330.json"),
		web(TWSEWDMarketClose, "/rwd/afterTrading/MI_INDEX", "response=json", "market_close.json"),
		web(TWSEWDInstitutional, "/rwd/fund/T86", "response=json", "institutional.json"),
		web(TWSEWDMargin, "/rwd/marginTrading/MI_MARGN", "date=20260731&selectType=ALL&response=json", "margin.json"),
		web(TWSEWDAbnormal, "/rwd/announcement/notice", "response=json", "abnormal_volume.json"),
		web(TWSEWDForeignQFIIS, "/rwd/fund/MI_QFIIS", "date=20260731&response=json", "qfiis.json"),
		web(TWSEWDMonthlyAvg, "/rwd/afterTrading/STOCK_DAY_AVG", "date=20260731&stockNo=2330&response=json", "day_avg.json"),
		web(TWSEWDBlockTrades, "/rwd/block/BFIAUU_d", "response=json", "block_trades.json"),
		api(TWSEAPIDailyClose, "/exchangeReport/STOCK_DAY_ALL", "daily_close.json"),
		api(TWSEAPIForeignHoldings, "/fund/MI_QFIIS_cat", "foreign_holdings.json"),
		api(TWSEAPIPunish, "/announcement/punish", "punish.json"),
		api(TWSEAPIValuation, "/exchangeReport/BWIBBU_ALL", "bwibbu_all.json"),
		api(TWSEAPIExDiv, "/exchangeReport/TWT48U_ALL", "twt48u_all.json"),
		api(TWSEAPIDividend, "/opendata/t187ap45_L", "t187ap45.json"),
		api(TWSEAPIWarrants, "/opendata/t187ap42_L", "warrants.json"),
	}
}

// tpexContractCases 涵蓋 TPEx-API 代表資料集。
func tpexContractCases() []contractCase {
	byDS := map[TPExDataset]string{
		TPExDailyClose:    "tpex_mainboard_quotes.json",
		TPExPEValuation:   "tpex_mainboard_peratio_analysis.json",
		TPExInstitutional: "tpex_3insti_daily_trading.json",
		TPExInstiSummary:  "tpex_3insti_summary.json",
		TPExMargin:        "tpex_mainboard_margin_balance.json",
		TPExAttention:     "tpex_trading_warning_information.json",
		TPExDisposition:   "tpex_disposal_information.json",
		TPExExRights:      "tpex_exright_prepost.json",
		TPExOddLot:        "tpex_odd_stock.json",
		TPExIndices:       "tpex_index.json",
	}
	var cases []contractCase
	for ds, fixture := range byDS {
		ds := ds
		fixture := fixture
		cases = append(cases, contractCase{
			name:    string(ds),
			dataset: string(ds),
			fixture: "tpex/" + fixture,
			rawURL:  tpexBase + tpexPaths[ds],
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = tpexBase + tpexPaths[ds]
				return normalizeTPEx(raw)
			},
		})
	}
	return cases
}

// mopsContractCases 涵蓋 MOPS OpenData CSV 資料集。
func mopsContractCases() []contractCase {
	byDS := map[MOPSDataset]string{
		MOPSMonthlyRevenue: "monthly_revenue.csv",
		MOPSIncomeSummary:  "income_summary.csv",
		MOPSProfitRatios:   "profit_ratios.csv",
		MOPSCompanyProfile: "company_profile.csv",
		MOPSAnnouncements:  "announcements.csv",
	}
	var cases []contractCase
	for ds, fixture := range byDS {
		ds := ds
		fixture := fixture
		cases = append(cases, contractCase{
			name:    string(ds),
			dataset: string(ds),
			fixture: "mops/" + fixture,
			rawURL:  mopsOpenDataBase + mopsPaths[ds],
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = mopsOpenDataBase + mopsPaths[ds]
				return normalizeMOPSRaw(raw)
			},
		})
	}
	return cases
}

// taifexContractCases 涵蓋 TAIFEX API JSON 與 DL CSV 代表資料集。
func taifexContractCases() []contractCase {
	var cases []contractCase
	apiCases := []struct {
		ds      model.TAIFEXDataset
		fixture string
	}{
		{model.TAPutCallRatio, "tfx_PutCallRatio.json"},
		// 期貨/選擇權含價差契約（如 CHF 202608/202609 開盤價 -0.04）：價差為
		// 兩契約之價差，官方本就可能為負（§5.1 負價格攔截在此豁免）。
		{model.TAFuturesDaily, "tfx_fut.json"},
		{model.TAOptionsDaily, "tfx_opt.json"},
		{model.TAMargin, "tfx_margin2.json"},
	}
	for _, c := range apiCases {
		c := c
		cases = append(cases, contractCase{
			name:    "api_" + string(c.ds),
			dataset: string(c.ds),
			fixture: "taifex/" + c.fixture,
			rawURL:  taifexAPIBase + taifexAPIPaths[c.ds],
			// 價差契約負價格豁免（TAFuturesDaily/TAOptionsDaily）
			allowNegPrice: c.ds == model.TAFuturesDaily || c.ds == model.TAOptionsDaily,
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = taifexAPIBase + taifexAPIPaths[c.ds]
				return normalizeTAIFEXAPI(raw)
			},
		})
	}
	dlCases := []struct {
		ds      model.TAIFEXDataset
		fixture string
	}{
		{model.TAFuturesDaily, "taifex_fut_daily.csv"},
		{model.TAOptionsDaily, "taifex_opt_daily.csv"},
		{model.TAInstiFutures, "taifex_insti_fut.csv"},
		{model.TAPutCallRatio, "taifex_pc_ratio.csv"},
		{model.TALargeTraderFut, "taifex_large_trader_fut.csv"},
		{model.TALargeTraderOpt, "taifex_large_trader_opt.csv"},
	}
	for _, c := range dlCases {
		c := c
		spec := taifexDLSpecs[c.ds]
		viewURL := contractSourceURL(taifexDLBase, spec.view, "queryStartDate=2026/07/31&queryEndDate=2026/07/31")
		cases = append(cases, contractCase{
			name:    "dl_" + string(c.ds),
			dataset: string(c.ds),
			fixture: "taifex/" + c.fixture,
			rawURL:  viewURL,
			normalize: func(raw *RawResponse) ([]byte, error) {
				raw.SourceURL = viewURL
				return normalizeTAIFEXDL(raw)
			},
		})
	}
	return cases
}

// misContractCase 驗證 MIS Normalize（parseMIS）輸出（§8.3/§5.1）。
func misContractCase() contractCase {
	return contractCase{
		name:    "mis_quote_tick",
		dataset: "mis_quote",
		fixture: "mis/tick_01.json",
		rawURL:  "https://mis.twse.com.tw/stock/api/getStockInfo.jsp?ex_ch=tse_2330.tw%7Cotc_6547.tw",
		normalize: func(raw *RawResponse) ([]byte, error) {
			snaps, err := parseMIS(raw.Body)
			if err != nil {
				return nil, err
			}
			return json.Marshal(snaps)
		},
	}
}

// TestContractAllAdapters 為 §13 契約測試總入口：所有 Adapter 之 Normalize
// 輸出驗證 §5 規則（不連網，fixtures 離線回放）。
func TestContractAllAdapters(t *testing.T) {
	var cases []contractCase
	cases = append(cases, twseContractCases()...)
	cases = append(cases, tpexContractCases()...)
	cases = append(cases, mopsContractCases()...)
	cases = append(cases, taifexContractCases()...)
	cases = append(cases, misContractCase())
	runContractCases(t, cases)
}

// 驗證框架本身：欄位命名/日期/單位規則之邊界案例。
func TestContractFrameworkRules(t *testing.T) {
	if !snakeRE.MatchString("is_cached") || !snakeRE.MatchString("can_trade") {
		t.Error("is_/can_ 前綴應符合 snake_case")
	}
	if snakeRE.MatchString("NotSnake") || snakeRE.MatchString("hasSpace ") {
		t.Error("大寫/空格不應符合 snake_case")
	}
	// 日期格式
	for _, bad := range []string{"2026/07/31", "113/07/31", "2026-7-1", "09:30", "31-07-2026"} {
		if dateOnlyRE.MatchString(bad) || clockRE.MatchString(bad) {
			t.Errorf("格式 %q 不應通過日期規則", bad)
		}
	}
	// 空字串缺值攔截
	if err := checkValue("data_date", "", false); err == nil {
		t.Error("空字串代表缺值應被攔截")
	}
	// 價格為負攔截
	if err := checkValue("close", -1.0, false); err == nil {
		t.Error("負價格應被攔截")
	}
	// 成交量為負攔截
	if err := checkValue("volume", int64(-5), false); err == nil {
		t.Error("負成交量應被攔截")
	}
	// 價格/量非負不攔截
	if err := checkValue("close", 0.0, false); err != nil {
		t.Errorf("零價格不應攔截: %v", err)
	}
	if err := checkValue("volume", int64(0), false); err != nil {
		t.Errorf("零成交量不應攔截: %v", err)
	}
	_ = time.Now // keep time import for date reference
}
