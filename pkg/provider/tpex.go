package provider

// TPEx Adapter（T009）：TPEx-API（www.tpex.org.tw/openapi）上櫃盤後資料來源，
// 實作 SourceContract（§2.2）。
//
// 資料集對應 §2 登錄表 TPEx-API（2026-07-31 swagger + 全端點實測）：
//
//	tpex_mainboard_quotes           上櫃股票收盤行情
//	tpex_mainboard_peratio_analysis 上櫃個股本益比、殖利率、股價淨值比
//	tpex_index                      櫃買指數歷史（當月 OHLC，西元年日期）
//	tpex_3insti_daily_trading       上櫃三大法人買賣明細（個股）
//	tpex_3insti_summary             上櫃三大法人買賣金額彙總表
//	tpex_mainboard_margin_balance   上櫃融資融券餘額
//	tpex_trading_warning_information 上櫃公布注意股票
//	tpex_disposal_information       上櫃處置有價證券
//	tpex_exright_prepost            上櫃除權除息預告表
//	tpex_odd_stock                  上櫃零股交易資訊
//
// 端點特性（2026-07-31 實測）：TPEx OpenAPI 不接受日期等 query 參數，一律回
// 傳最新交易日之全市場資料；資料列皆含 Date 欄（民國 YYYMMDD，tpex_index 為
// 西元 YYYYMMDD）。URL 之 stockNo 參數僅作為 Normalize 過濾用。
//
// 單位換算（§5.1）：融資融券餘額官方以「張」計（以 6147 融資利用率
// 15.59% = 29,023/186,148 張核對），一律 ×1000 → 股；其餘成交量值已為
// 股/元不需換算。
//
// 上市/上櫃邊界（§2.1）：stockNo 過濾後查無資料（如上市 2330 查上櫃）回傳
// 空陣列而非錯誤，供上層工具做 cross-market fallback。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
)

// TPExDataset 為 TPEx-API（www.tpex.org.tw/openapi）資料集 ID。
type TPExDataset string

// TPEx-API 資料集（§2 登錄表 TPEx-API 內容範圍）。
const (
	TPExDailyClose    TPExDataset = "daily_close"           // 上櫃收盤行情
	TPExPEValuation   TPExDataset = "pe_valuation"          // 本益比/殖利率/股價淨值比
	TPExIndices       TPExDataset = "indices"               // 櫃買指數歷史
	TPExInstitutional TPExDataset = "institutional"         // 三大法人買賣明細（個股）
	TPExInstiSummary  TPExDataset = "institutional_summary" // 三大法人買賣金額彙總
	TPExMargin        TPExDataset = "margin"                // 融資融券餘額
	TPExAttention     TPExDataset = "attention"             // 注意股票
	TPExDisposition   TPExDataset = "disposition"           // 處置股票
	TPExExRights      TPExDataset = "ex_rights"             // 除權息預告表
	TPExOddLot        TPExDataset = "odd_lot"               // 零股交易
)

// 端點路徑（2026-07 實測可用）。
var (
	tpexBase  = "https://www.tpex.org.tw/openapi/v1"
	tpexPaths = map[TPExDataset]string{
		TPExDailyClose:    "/tpex_mainboard_quotes",
		TPExPEValuation:   "/tpex_mainboard_peratio_analysis",
		TPExIndices:       "/tpex_index",
		TPExInstitutional: "/tpex_3insti_daily_trading",
		TPExInstiSummary:  "/tpex_3insti_summary",
		TPExMargin:        "/tpex_mainboard_margin_balance",
		TPExAttention:     "/tpex_trading_warning_information",
		TPExDisposition:   "/tpex_disposal_information",
		TPExExRights:      "/tpex_exright_prepost",
		TPExOddLot:        "/tpex_odd_stock",
	}
)

// NewTPExSource 建立 TPEx-API 來源（Rate Limit 1 req/s，§4.4）。
func NewTPExSource(opts ...Option) *TPExSource {
	return &TPExSource{client: NewBaseClient("www.tpex.org.tw", opts...)}
}

// TPExSource 實作 SourceContract（§2.2），ID = TPEX_API。
type TPExSource struct{ client *BaseClient }

var _ SourceContract = (*TPExSource)(nil)

func (s *TPExSource) ID() string { return model.SourceTPExAPI }

// URL 建立資料集之官方請求 URL。官方端點不接受 query 參數（恆回最新交易日
// 全市場），stockNo 僅由 Normalize 使用做過濾。
func (s *TPExSource) URL(ds TPExDataset, params url.Values) string {
	u := tpexBase + tpexPaths[ds]
	if params.Encode() != "" {
		u += "?" + params.Encode()
	}
	return u
}

func (s *TPExSource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	return s.client.Do(ctx, req)
}

func (s *TPExSource) Validate(raw *RawResponse) error {
	return validateTPEx(raw)
}

func (s *TPExSource) Normalize(raw *RawResponse) ([]byte, error) {
	return normalizeTPEx(raw)
}

// tpexDatasetOf 依 SourceURL 之 path 判斷資料集。
func tpexDatasetOf(raw *RawResponse) (string, error) {
	u, err := url.Parse(raw.SourceURL)
	if err != nil {
		return "", fmt.Errorf("provider: 無法解析來源 URL %q: %w", raw.SourceURL, err)
	}
	p := u.Path
	for ds, path := range tpexPaths {
		if strings.HasSuffix(p, path) {
			return string(ds), nil
		}
	}
	return "", fmt.Errorf("provider: 未知 TPEx 資料集路徑 %q", p)
}

// ---------------------------------------------------------------------------
// Validate：schema 檢查（欄位存在性、數值範圍、日期一致性，§2.2）

func validateTPEx(raw *RawResponse) error {
	ds, err := tpexDatasetOf(raw)
	if err != nil {
		return err
	}
	body := raw.Body
	if len(body) == 0 {
		return fmt.Errorf("provider: %s 空 body", ds)
	}
	if !isJSONArray(body) {
		return fmt.Errorf("provider: %s 回應非 JSON 陣列（官方格式可能變更）", ds)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return fmt.Errorf("provider: %s 回應 JSON 解析失敗: %w", ds, err)
	}
	return validateOpenAPIList(raw, rows)
}

// ---------------------------------------------------------------------------
// Normalize：依資料集將 raw 轉為 Normalized Model（JSON），單位 元/股/%

func normalizeTPEx(raw *RawResponse) ([]byte, error) {
	ds, err := tpexDatasetOf(raw)
	if err != nil {
		return nil, err
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: %s JSON 解析失敗: %w", ds, err)
	}
	stockNo := queryStockNo(raw.SourceURL)
	ms := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		if stockNo != "" && strings.TrimSpace(m["SecuritiesCompanyCode"]) != stockNo {
			continue
		}
		ms = append(ms, m)
	}
	// 上市/上櫃邊界：指定 stockNo 但查無資料（如上市 2330 查上櫃）→ 空陣列，
	// 由工具層 fallback（§2.1）；未過濾時官方空資料視為格式異常。
	if len(ms) == 0 {
		if stockNo != "" {
			out, err := json.Marshal(emptyRows[ds])
			if err != nil {
				return nil, err
			}
			return out, nil
		}
		return nil, fmt.Errorf("provider: %s 無有效資料列", ds)
	}
	var out interface{}
	switch ds {
	case string(TPExDailyClose):
		out = normalizeTPExDailyClose(ms)
	case string(TPExPEValuation):
		out = normalizePEValuation(ms)
	case string(TPExIndices):
		out, err = normalizeIndexHistoryTPEx(ms)
	case string(TPExInstitutional):
		out = normalizeInstitutionalTPEx(ms)
	case string(TPExInstiSummary):
		out = normalizeInstiSummary(ms)
	case string(TPExMargin):
		out = normalizeMarginTPEx(ms)
	case string(TPExAttention):
		out = normalizeAttention(ms)
	case string(TPExDisposition):
		out = normalizeDisposition(ms)
	case string(TPExExRights):
		out = normalizeExRights(ms)
	case string(TPExOddLot):
		out = normalizeOddLot(ms)
	default:
		return nil, fmt.Errorf("provider: 不支援資料集 %q", ds)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// emptyRows 各資料集之空陣列（cross-market 邊界案例輸出）。
var emptyRows = map[string]interface{}{
	string(TPExDailyClose):    []TPExDailyCloseRow{},
	string(TPExPEValuation):   []TPExPEValuationRow{},
	string(TPExIndices):       []TPExIndexRow{},
	string(TPExInstitutional): []TPExInstitutionalRow{},
	string(TPExInstiSummary):  []TPExInstiSummaryRow{},
	string(TPExMargin):        []TPExMarginRow{},
	string(TPExAttention):     []TPExAttentionRow{},
	string(TPExDisposition):   []TPExDispositionRow{},
	string(TPExExRights):      []TPExExRightRow{},
	string(TPExOddLot):        []TPExOddLotRow{},
}

// queryStockNo 取 URL query 之 stockNo（TPEx 過濾參數）。
func queryStockNo(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("stockNo")
}

// tpexDate 解析 TPEx 日期：民國 7 碼（1150731）或西元 8 碼（20260701）。
func tpexDate(s string) (string, bool) {
	t := strings.TrimSpace(s)
	switch len(t) {
	case 7:
		ts, err := parseROCDate(t)
		if err != nil {
			return "", false
		}
		return model.FormatDate(ts), true
	case 8:
		var y, m, d int
		if _, err := fmt.Sscanf(t, "%4d%2d%2d", &y, &m, &d); err != nil {
			return "", false
		}
		if y < 1990 || m < 1 || m > 12 || d < 1 || d > 31 {
			return "", false
		}
		return model.FormatDate(time.Date(y, time.Month(m), d, 0, 0, 0, 0, model.Taipei())), true
	}
	return "", false
}

// pick 於 row 中依子字串找值（掃描排序後之 key 以維持決定性）。
// TPEx 英文欄位名含不一致之空格（如 " Total Sell"），以子字串比對容錯。
func pick(row map[string]string, substr string) string {
	keys := make([]string, 0, len(row))
	for k := range row {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if strings.Contains(k, substr) {
			return row[k]
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 上櫃收盤行情（tpex_mainboard_quotes）：全市場最新交易日收盤行情。
// 單位：股/元（2026-07 實測 TradingShares=股、TransactionAmount=元、
// Capitals=股）。

// TPExDailyCloseRow 為單一上櫃證券之收盤行情。
type TPExDailyCloseRow struct {
	Date        string  `json:"date"` // YYYY-MM-DD
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Close       float64 `json:"close"`                // 收盤價（元）
	ChangeDir   string  `json:"change_dir,omitempty"` // 漲跌(+/-)
	Change      float64 `json:"change"`               // 漲跌價差（元）
	Open        float64 `json:"open"`                 // 開盤價（元）
	High        float64 `json:"high"`                 // 最高價（元）
	Low         float64 `json:"low"`                  // 最低價（元）
	Volume      int64   `json:"volume"`               // 成交股數（股）
	Amount      int64   `json:"amount"`               // 成交金額（元）
	Transaction int64   `json:"transaction"`          // 成交筆數
	Capital     int64   `json:"capital"`              // 股本（股）
	LimitUp     float64 `json:"limit_up,omitempty"`   // 次日漲停價（元）
	LimitDown   float64 `json:"limit_down,omitempty"` // 次日跌停價（元）
}

func normalizeTPExDailyClose(ms []map[string]string) []TPExDailyCloseRow {
	out := make([]TPExDailyCloseRow, 0, len(ms))
	for _, m := range ms {
		r := TPExDailyCloseRow{
			Code:        strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:        strings.TrimSpace(m["CompanyName"]),
			Close:       commaFloatOrZero(m["Close"]),
			ChangeDir:   changeDirOf(m["Change"]),
			Change:      commaFloatOrZero(m["Change"]),
			Open:        commaFloatOrZero(m["Open"]),
			High:        commaFloatOrZero(m["High"]),
			Low:         commaFloatOrZero(m["Low"]),
			Volume:      commaIntOrZero(m["TradingShares"]),
			Amount:      commaIntOrZero(m["TransactionAmount"]),
			Transaction: commaIntOrZero(m["TransactionNumber"]),
			Capital:     commaIntOrZero(m["Capitals"]),
			LimitUp:     commaFloatOrZero(m["NextLimitUp"]),
			LimitDown:   commaFloatOrZero(m["NextLimitDown"]),
		}
		if r.Code == "" || r.Name == "" || r.Close <= 0 {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// changeDirOf 由官方漲跌字串（"+11.50"/"-3.10"）取漲跌方向。
func changeDirOf(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "+") {
		return "+"
	}
	if strings.HasPrefix(s, "-") {
		return "-"
	}
	return ""
}

// ---------------------------------------------------------------------------
// 本益比/殖利率/股價淨值比（tpex_mainboard_peratio_analysis）。

// TPExPEValuationRow 為單一上櫃股票之估值指標。
type TPExPEValuationRow struct {
	Date             string  `json:"date"` // YYYY-MM-DD
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	PE               float64 `json:"pe"`                 // 本益比
	DividendPerShare float64 `json:"dividend_per_share"` // 每股股利（元）
	YieldRatio       float64 `json:"yield_ratio"`        // 殖利率（%）
	PriceBookRatio   float64 `json:"price_book_ratio"`   // 股價淨值比
}

func normalizePEValuation(ms []map[string]string) []TPExPEValuationRow {
	out := make([]TPExPEValuationRow, 0, len(ms))
	for _, m := range ms {
		r := TPExPEValuationRow{
			Code:             strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:             strings.TrimSpace(m["CompanyName"]),
			PE:               commaFloatOrZero(m["PriceEarningRatio"]),
			DividendPerShare: commaFloatOrZero(m["DividendPerShare"]),
			YieldRatio:       commaFloatOrZero(m["YieldRatio"]),
			PriceBookRatio:   commaFloatOrZero(m["PriceBookRatio"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 櫃買指數歷史（tpex_index）：當月每日 OHLC（西元日期，2026-07 實測）。

// TPExIndexRow 為櫃買指數之一日 OHLC。
type TPExIndexRow struct {
	Date   string  `json:"date"`   // YYYY-MM-DD
	Open   float64 `json:"open"`   // 開盤指數
	High   float64 `json:"high"`   // 最高指數
	Low    float64 `json:"low"`    // 最低指數
	Close  float64 `json:"close"`  // 收盤指數
	Change float64 `json:"change"` // 漲跌點數
}

func normalizeIndexHistoryTPEx(ms []map[string]string) ([]TPExIndexRow, error) {
	out := make([]TPExIndexRow, 0, len(ms))
	for _, m := range ms {
		d, ok := tpexDate(m["Date"])
		if !ok {
			continue
		}
		r := TPExIndexRow{
			Date:   d,
			Open:   commaFloatOrZero(m["Open"]),
			High:   commaFloatOrZero(m["High"]),
			Low:    commaFloatOrZero(m["Low"]),
			Close:  commaFloatOrZero(m["Close"]),
			Change: commaFloatOrZero(m["Change"]),
		}
		if r.Close <= 0 {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: indices 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 三大法人買賣明細（tpex_3insti_daily_trading）：個股層級，單位：股。
// 官方英文欄位名含不一致空格，以 pick 子字串比對取值。

// TPExInstitutionalRow 為單一上櫃股票之三大法人買賣超（股）。
type TPExInstitutionalRow struct {
	Date             string `json:"date"` // YYYY-MM-DD
	Code             string `json:"code"`
	Name             string `json:"name"`
	ForeignBuy       int64  `json:"foreign_buy"`        // 外陸資買進（不含外資自營商）
	ForeignSell      int64  `json:"foreign_sell"`       // 外陸資賣出（不含外資自營商）
	ForeignNet       int64  `json:"foreign_net"`        // 外陸資買賣超
	ForeignDealerNet int64  `json:"foreign_dealer_net"` // 外資自營商買賣超
	InvestmentBuy    int64  `json:"investment_buy"`     // 投信買進
	InvestmentSell   int64  `json:"investment_sell"`    // 投信賣出
	InvestmentNet    int64  `json:"investment_net"`     // 投信買賣超
	DealerBuy        int64  `json:"dealer_buy"`         // 自營商買進
	DealerSell       int64  `json:"dealer_sell"`        // 自營商賣出
	DealerNet        int64  `json:"dealer_net"`         // 自營商買賣超
	TotalNet         int64  `json:"total_net"`          // 三大法人買賣超
}

func normalizeInstitutionalTPEx(ms []map[string]string) []TPExInstitutionalRow {
	out := make([]TPExInstitutionalRow, 0, len(ms))
	for _, m := range ms {
		r := TPExInstitutionalRow{
			Code:             strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:             strings.TrimSpace(m["CompanyName"]),
			ForeignBuy:       commaIntOrZero(pick(m, "excluded)-Total Buy")),
			ForeignSell:      commaIntOrZero(pick(m, "excluded)-Total Sell")),
			ForeignNet:       commaIntOrZero(pick(m, "excluded)-Difference")),
			ForeignDealerNet: commaIntOrZero(pick(m, "ForeignDealers-Difference")),
			InvestmentBuy:    commaIntOrZero(m["SecuritiesInvestmentTrustCompanies-TotalBuy"]),
			InvestmentSell:   commaIntOrZero(m["SecuritiesInvestmentTrustCompanies-TotalSell"]),
			InvestmentNet:    commaIntOrZero(m["SecuritiesInvestmentTrustCompanies-Difference"]),
			DealerBuy:        commaIntOrZero(m["Dealers-TotalBuy"]),
			DealerSell:       commaIntOrZero(m["Dealers-TotalSell"]),
			DealerNet:        commaIntOrZero(pick(m, "Dealers-Difference")),
			TotalNet:         commaIntOrZero(pick(m, "TotalDifference")),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 三大法人買賣金額彙總（tpex_3insti_summary）：單位：元（實測外資買進
// 42,429,452,107 元）。

// TPExInstiSummaryRow 為單一法人類別之買賣金額彙總。
type TPExInstiSummaryRow struct {
	Date           string `json:"date"`            // YYYY-MM-DD
	Investor       string `json:"investor"`        // 法人類別（外資及陸資合計/投信/自營商合計…）
	PurchaseAmount int64  `json:"purchase_amount"` // 買進金額（元）
	SaleAmount     int64  `json:"sale_amount"`     // 賣出金額（元）
	Net            int64  `json:"net"`             // 買賣超金額（元）
}

func normalizeInstiSummary(ms []map[string]string) []TPExInstiSummaryRow {
	out := make([]TPExInstiSummaryRow, 0, len(ms))
	for _, m := range ms {
		r := TPExInstiSummaryRow{
			Investor:       strings.TrimSpace(m["Investor"]),
			PurchaseAmount: commaIntOrZero(m["PurchaseAmount"]),
			SaleAmount:     commaIntOrZero(m["SaleAmount"]),
			Net:            commaIntOrZero(m["Net"]),
		}
		if r.Investor == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 融資融券餘額（tpex_mainboard_margin_balance）：官方以「張」計
// （以 6147 頎邦利用率 15.59% = 29,023/186,148 張核對），×1000 → 股。

// TPExMarginRow 為單一上櫃股票之融資融券餘額（股）。
type TPExMarginRow struct {
	Date              string  `json:"date"` // YYYY-MM-DD
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	MarginPrevBalance int64   `json:"margin_prev_balance"` // 融資前日餘額（股）
	MarginBuy         int64   `json:"margin_buy"`          // 融資買進（股）
	MarginSell        int64   `json:"margin_sell"`         // 融資賣出（股）
	MarginCashRedeem  int64   `json:"margin_cash_redeem"`  // 融資現金償還（股）
	MarginBalance     int64   `json:"margin_balance"`      // 融資今日餘額（股）
	MarginSFE         int64   `json:"margin_sfe"`          // 融資餘額（屬證金）（股）
	MarginRate        float64 `json:"margin_rate"`         // 融資利用率（%）
	MarginQuota       int64   `json:"margin_quota"`        // 融資限額（股）
	ShortPrevBalance  int64   `json:"short_prev_balance"`  // 融券前日餘額（股）
	ShortSell         int64   `json:"short_sell"`          // 融券賣出（股）
	ShortConvering    int64   `json:"short_convering"`     // 融券買進（股）
	StockRedemption   int64   `json:"stock_redemption"`    // 融券現券償還（股）
	ShortBalance      int64   `json:"short_balance"`       // 融券今日餘額（股）
	ShortSFE          int64   `json:"short_sfe"`           // 融券餘額（屬證金）（股）
	ShortRate         float64 `json:"short_rate"`          // 融券利用率（%）
	ShortQuota        int64   `json:"short_quota"`         // 融券限額（股）
	Offsetting        int64   `json:"offsetting"`          // 資券互抵（股）
	Note              string  `json:"note,omitempty"`
}

func normalizeMarginTPEx(ms []map[string]string) []TPExMarginRow {
	out := make([]TPExMarginRow, 0, len(ms))
	for _, m := range ms {
		r := TPExMarginRow{
			Code:              strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:              strings.TrimSpace(m["CompanyName"]),
			MarginPrevBalance: model.LotsToShares(commaIntOrZero(m["MarginPurchaseBalancePreviousDay"])),
			MarginBuy:         model.LotsToShares(commaIntOrZero(m["MarginPurchase"])),
			MarginSell:        model.LotsToShares(commaIntOrZero(m["MarginSales"])),
			MarginCashRedeem:  model.LotsToShares(commaIntOrZero(m["CashRedemption"])),
			MarginBalance:     model.LotsToShares(commaIntOrZero(m["MarginPurchaseBalance"])),
			MarginSFE:         model.LotsToShares(commaIntOrZero(m["MarginPurchaseBalanceBelongSecuritiesFinanceEnterprise"])),
			MarginRate:        commaFloatOrZero(m["MarginPurchaseUtilizationRate"]),
			MarginQuota:       model.LotsToShares(commaIntOrZero(m["MarginPurchaseQuota"])),
			ShortPrevBalance:  model.LotsToShares(commaIntOrZero(m["ShortSaleBalancePreviousDay"])),
			ShortSell:         model.LotsToShares(commaIntOrZero(m["ShortSale"])),
			ShortConvering:    model.LotsToShares(commaIntOrZero(m["ShortConvering"])),
			StockRedemption:   model.LotsToShares(commaIntOrZero(m["StockRedemption"])),
			ShortBalance:      model.LotsToShares(commaIntOrZero(m["ShortSaleBalance"])),
			ShortSFE:          model.LotsToShares(commaIntOrZero(m["ShortSaleBalanceBelongSecuritiesFinanceEnterprise"])),
			ShortRate:         commaFloatOrZero(m["ShortSaleUtilizationRate"]),
			ShortQuota:        model.LotsToShares(commaIntOrZero(m["ShortSaleQuota"])),
			Offsetting:        model.LotsToShares(commaIntOrZero(m["Offsetting"])),
			Note:              strings.TrimSpace(m["Note"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 注意股票（tpex_trading_warning_information）。

// TPExAttentionRow 為一檔上櫃注意股票。
type TPExAttentionRow struct {
	Date  string  `json:"date"` // YYYY-MM-DD
	Code  string  `json:"code"`
	Name  string  `json:"name"`
	Info  string  `json:"info"`  // 注意交易資訊
	Close float64 `json:"close"` // 收盤價（元）
	PE    float64 `json:"pe"`    // 本益比（官方可能為 "N/A"）
}

func normalizeAttention(ms []map[string]string) []TPExAttentionRow {
	out := make([]TPExAttentionRow, 0, len(ms))
	for _, m := range ms {
		r := TPExAttentionRow{
			Code:  strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:  strings.TrimSpace(m["CompanyName"]),
			Info:  strings.TrimSpace(m["TradingInformation"]),
			Close: commaFloatOrZero(m["ClosePrice"]),
			PE:    commaFloatOrZero(m["PriceEarningRatio"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 處置股票（tpex_disposal_information）。

// TPExDispositionRow 為一檔上櫃處置股票。
type TPExDispositionRow struct {
	Date      string `json:"date"` // YYYY-MM-DD
	Code      string `json:"code"`
	Name      string `json:"name"`
	Period    string `json:"period"`    // 處置期間（如 1150803~1150814）
	Reasons   string `json:"reasons"`   // 處置原因
	Condition string `json:"condition"` // 處置條件
}

func normalizeDisposition(ms []map[string]string) []TPExDispositionRow {
	out := make([]TPExDispositionRow, 0, len(ms))
	for _, m := range ms {
		r := TPExDispositionRow{
			Code:      strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:      strings.TrimSpace(m["CompanyName"]),
			Period:    strings.TrimSpace(m["DispositionPeriod"]),
			Reasons:   strings.TrimSpace(m["DispositionReasons"]),
			Condition: strings.TrimSpace(m["DisposalCondition"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 除權息預告表（tpex_exright_prepost）。

// TPExExRightRow 為單一上櫃公司之除權除息預告。
type TPExExRightRow struct {
	Date               string  `json:"date"` // 除權除息基準日 YYYY-MM-DD
	Code               string  `json:"code"`
	Name               string  `json:"name"`
	Kind               string  `json:"kind"`                 // 除權/除息
	StockDividendRatio float64 `json:"stock_dividend_ratio"` // 股票股利比率
	SubscriptionRatio  float64 `json:"subscription_ratio"`   // 現金增資認購比率
	SubscriptionPrice  float64 `json:"subscription_price"`   // 現金增資認購價（元）
	CashDividend       float64 `json:"cash_dividend"`        // 現金股利（元/股）
}

func normalizeExRights(ms []map[string]string) []TPExExRightRow {
	out := make([]TPExExRightRow, 0, len(ms))
	for _, m := range ms {
		r := TPExExRightRow{
			Code:               strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:               strings.TrimSpace(m["CompanyName"]),
			Kind:               strings.TrimSpace(m["ExRrightsExDividend"]),
			StockDividendRatio: commaFloatOrZero(m["StockDividendRatio"]),
			SubscriptionRatio:  commaFloatOrZero(m["SubscriptionRatioToNewSharesIssued"]),
			SubscriptionPrice:  commaFloatOrZero(m["SubscriptionPricePerShare"]),
			CashDividend:       commaFloatOrZero(m["CashDividend"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["ExRrightsExDividendDate"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}

// ---------------------------------------------------------------------------
// 零股交易（tpex_odd_stock）：單位股/元（6147 驗算 1,455 股 × 130 = 189,150 元 ✓）。

// TPExOddLotRow 為單一上櫃證券之零股交易統計。
type TPExOddLotRow struct {
	Date             string  `json:"date"` // YYYY-MM-DD
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Volume           int64   `json:"volume"`               // 成交股數（股）
	Transactions     int64   `json:"transactions"`         // 成交筆數
	Amount           int64   `json:"amount"`               // 成交金額（元）
	Price            float64 `json:"price"`                // 成交均價（元）
	LastBestBidPrice float64 `json:"last_best_bid_price"`  // 最後揭示最佳買價（元）
	LastBestBidVol   int64   `json:"last_best_bid_volume"` // 最後揭示最佳買量（股）
	LastBestAskPrice float64 `json:"last_best_ask_price"`  // 最後揭示最佳賣價（元）
	LastBestAskVol   int64   `json:"last_best_ask_volume"` // 最後揭示最佳賣量（股）
}

func normalizeOddLot(ms []map[string]string) []TPExOddLotRow {
	out := make([]TPExOddLotRow, 0, len(ms))
	for _, m := range ms {
		r := TPExOddLotRow{
			Code:             strings.TrimSpace(m["SecuritiesCompanyCode"]),
			Name:             strings.TrimSpace(m["CompanyName"]),
			Volume:           commaIntOrZero(m["TradeVolume"]),
			Transactions:     commaIntOrZero(m["NumberOfTransactions"]),
			Amount:           commaIntOrZero(m["TradeAmount"]),
			Price:            commaFloatOrZero(m["Price"]),
			LastBestBidPrice: commaFloatOrZero(m["LastBestBidPrice"]),
			LastBestBidVol:   commaIntOrZero(m["LastBestBidVolume"]),
			LastBestAskPrice: commaFloatOrZero(m["LastBestAskPrice"]),
			LastBestAskVol:   commaIntOrZero(m["LastBestAskVolume"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if d, ok := tpexDate(m["Date"]); ok {
			r.Date = d
		}
		out = append(out, r)
	}
	return out
}
