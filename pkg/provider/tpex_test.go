package provider

import (
	"fmt"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tpexFixture 讀取 TPEx 錄製 raw response（testdata/tpex/<name>.json）。
func tpexFixture(t *testing.T, name string) *RawResponse {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "tpex", name+".json"))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return &RawResponse{Body: body, SourceURL: "https://www.tpex.org.tw/openapi/v1/" + name}
}

// tpexRaw 以指定 URL 包裝 fixture（供 URL 依賴之測試）。
func tpexRaw(t *testing.T, name, sourceURL string) *RawResponse {
	t.Helper()
	r := tpexFixture(t, name)
	r.SourceURL = sourceURL
	return r
}

// tpexFixtureDataset 依資料集對應之 fixture 名稱。
func tpexFixtureDataset(ds TPExDataset) string {
	switch ds {
	case TPExDailyClose:
		return "tpex_mainboard_quotes"
	case TPExPEValuation:
		return "tpex_mainboard_peratio_analysis"
	case TPExIndices:
		return "tpex_index"
	case TPExInstitutional:
		return "tpex_3insti_daily_trading"
	case TPExInstiSummary:
		return "tpex_3insti_summary"
	case TPExMargin:
		return "tpex_mainboard_margin_balance"
	case TPExAttention:
		return "tpex_trading_warning_information"
	case TPExDisposition:
		return "tpex_disposal_information"
	case TPExExRights:
		return "tpex_exright_prepost"
	case TPExOddLot:
		return "tpex_odd_stock"
	}
	return ""
}

// TestTPExURL 驗證各資料集 URL 建置與 stockNo 參數。
func TestTPExURL(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	for ds, path := range tpexPaths {
		u := s.URL(ds, url.Values{})
		want := tpexBase + path
		switch ds {
		case TPExOtcESG:
			want = tpexBase + fmt.Sprintf(path, "1") // 預設 topic=1（T216）
		case TPExOtcMopsfin:
			want = tpexBase + "/mopsfin_unknown" // 空 kind 之防禦路徑（T237）
		}
		if u != want {
			t.Errorf("%s URL = %q，期望 %q", ds, u, want)
		}
	}
	// 上櫃 ESG topic 模板展開（T216）
	if u := s.URL(TPExOtcESG, url.Values{"topic": {"6"}}); u != tpexBase+"/t187ap46_O_6" {
		t.Errorf("otc_esg topic URL = %q", u)
	}
	// 上櫃治理系列 kind 模板（T237）：空 kind 回防禦路徑，正常 kind 展開
	if u := s.URL(TPExOtcMopsfin, url.Values{}); u != tpexBase+"/mopsfin_unknown" {
		t.Errorf("otc_mopsfin 空 kind URL = %q", u)
	}
	if u := s.URL(TPExOtcMopsfin, url.Values{"kind": {"t187ap08_O"}}); u != tpexBase+"/mopsfin_t187ap08_O" {
		t.Errorf("otc_mopsfin kind URL = %q", u)
	}
	u := s.URL(TPExDailyClose, url.Values{"stockNo": {"6147"}})
	if want := tpexBase + "/tpex_mainboard_quotes?stockNo=6147"; u != want {
		t.Errorf("stockNo URL = %q，期望 %q", u, want)
	}
}

// TestTPExValidate 驗證各資料集 Validate 通過與錯誤路徑。
func TestTPExValidate(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	datasets := []TPExDataset{
		TPExDailyClose, TPExPEValuation, TPExIndices, TPExInstitutional,
		TPExInstiSummary, TPExMargin, TPExAttention, TPExDisposition,
		TPExExRights, TPExOddLot,
	}
	for _, ds := range datasets {
		if err := s.Validate(tpexFixture(t, tpexFixtureDataset(ds))); err != nil {
			t.Errorf("%s Validate 失敗: %v", ds, err)
		}
	}
	// 空陣列為合法（無資料交易日）
	if err := s.Validate(tpexRaw(t, "empty",
		"https://www.tpex.org.tw/openapi/v1/tpex_odd_stock")); err != nil {
		t.Errorf("empty Validate 失敗: %v", err)
	}
	// 錯誤路徑
	bad := []struct {
		name string
		body string
	}{
		{"未知路徑", `[]`},
		{"非陣列", `{"stat":"OK"}`},
		{"非 JSON", `<<<`},
	}
	for _, b := range bad {
		r := &RawResponse{Body: []byte(b.body), SourceURL: "https://www.tpex.org.tw/openapi/v1/unknown"}
		if err := s.Validate(r); err == nil {
			t.Errorf("%s 應驗證失敗", b.name)
		}
	}
}

// TestTPExFetchContract 以 httptest 驗證 Fetch→Validate→Normalize 全流程。
func TestTPExFetchContract(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "tpex", "tpex_odd_stock.json"))
	if err != nil {
		t.Fatalf("讀取 fixture 失敗: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/v1/tpex_odd_stock" {
			t.Errorf("請求路徑 = %q", r.URL.Path)
		}
		w.Write(body)
	}))
	defer srv.Close()

	s := NewTPExSource(WithRateInterval(0))
	req := RawRequest{URL: srv.URL + "/openapi/v1/tpex_odd_stock"}
	raw, err := s.Fetch(context.Background(), req)
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExOddLotRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("Normalize 輸出解析失敗: %v", err)
	}
	if len(rows) == 0 || rows[0].Code == "" {
		t.Errorf("契約輸出異常: %d 列", len(rows))
	}
}

// TestTPExDailyClose 上櫃收盤行情契約。
func TestTPExDailyClose(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	raw := tpexFixture(t, "tpex_mainboard_quotes")
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExDailyCloseRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	byCode := map[string]TPExDailyCloseRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	r, ok := byCode["6147"]
	if !ok {
		t.Fatalf("缺 6147 列（共 %d 列）", len(rows))
	}
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-31"},
		{"name", r.Name, "頎邦"},
		{"close", r.Close, 130.0},
		{"change_dir", r.ChangeDir, "+"},
		{"change", r.Change, 11.5},
		{"open", r.Open, 130.0},
		{"high", r.High, 130.0},
		{"low", r.Low, 126.0},
		{"volume", r.Volume, int64(20185000)},
		{"amount", r.Amount, int64(2607211500)},
		{"transaction", r.Transaction, int64(6857)},
		{"capital", r.Capital, int64(744593539)},
		{"limit_up", r.LimitUp, 143.0},
		{"limit_down", r.LimitDown, 117.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExPEValuation 本益比/殖利率/股價淨值比契約。
func TestTPExPEValuation(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_mainboard_peratio_analysis"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExPEValuationRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	r := rows[0]
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-31"},
		{"code", r.Code, "1240"},
		{"name", r.Name, "茂生農經"},
		{"pe", r.PE, 11.82},
		{"dividend_per_share", r.DividendPerShare, 3.5},
		{"yield_ratio", r.YieldRatio, 6.34},
		{"price_book_ratio", r.PriceBookRatio, 1.57},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExIndices 櫃買指數歷史契約（西元日期）。
func TestTPExIndices(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_index"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExIndexRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	if len(rows) != 22 {
		t.Fatalf("列數 = %d，期望 22（全月）", len(rows))
	}
	first, last := rows[0], rows[len(rows)-1]
	if first.Date != "2026-07-01" || first.Open != 430.29 || first.Close != 431.23 || first.Change != 4.26 {
		t.Errorf("首列錯誤: %+v", first)
	}
	if last.Date != "2026-07-31" || last.Close != 347.85 || last.Change != 21.62 {
		t.Errorf("末列錯誤: %+v", last)
	}
}

// TestTPExInstitutional 三大法人個股契約（英文欄位名容錯）。
func TestTPExInstitutional(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_3insti_daily_trading"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExInstitutionalRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	byCode := map[string]TPExInstitutionalRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	r, ok := byCode["6147"]
	if !ok {
		t.Fatalf("缺 6147 列（共 %d 列）", len(rows))
	}
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-31"},
		{"name", r.Name, "頎邦"},
		{"foreign_buy", r.ForeignBuy, int64(3478490)},
		{"foreign_sell", r.ForeignSell, int64(7651771)},
		{"foreign_net", r.ForeignNet, int64(-4173281)},
		{"investment_buy", r.InvestmentBuy, int64(2350703)},
		{"investment_sell", r.InvestmentSell, int64(942000)},
		{"investment_net", r.InvestmentNet, int64(1408703)},
		{"dealer_buy", r.DealerBuy, int64(356585)},
		{"dealer_sell", r.DealerSell, int64(674846)},
		{"dealer_net", r.DealerNet, int64(-318261)},
		{"total_net", r.TotalNet, int64(-3082839)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExInstiSummary 三大法人彙總契約（金額=元）。
func TestTPExInstiSummary(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_3insti_summary"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExInstiSummaryRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	if len(rows) != 8 {
		t.Fatalf("列數 = %d，期望 8", len(rows))
	}
	byInv := map[string]TPExInstiSummaryRow{}
	for _, r := range rows {
		byInv[r.Investor] = r
	}
	r, ok := byInv["外資及陸資合計"]
	if !ok {
		t.Fatalf("缺外資列")
	}
	if r.Date != "2026-07-31" || r.PurchaseAmount != int64(42429452107) ||
		r.SaleAmount != int64(49744055673) || r.Net != int64(-7314603566) {
		t.Errorf("外資彙總錯誤: %+v", r)
	}
	if got := byInv["三大法人合計*"].Net; got != int64(-6353190029) {
		t.Errorf("三大法人合計 net = %d", got)
	}
}

// TestTPExMargin 融資融券契約（張→股換算 §5.1）。
func TestTPExMargin(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_mainboard_margin_balance"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExMarginRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	byCode := map[string]TPExMarginRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	r, ok := byCode["6147"]
	if !ok {
		t.Fatalf("缺 6147 列（共 %d 列）", len(rows))
	}
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-30"},
		{"name", r.Name, "頎邦"},
		{"margin_prev_balance", r.MarginPrevBalance, int64(31553000)},
		{"margin_buy", r.MarginBuy, int64(1754000)},
		{"margin_sell", r.MarginSell, int64(4205000)},
		{"margin_cash_redeem", r.MarginCashRedeem, int64(79000)},
		{"margin_balance", r.MarginBalance, int64(29023000)},
		{"margin_rate", r.MarginRate, 15.59},
		{"margin_quota", r.MarginQuota, int64(186148000)},
		{"short_prev_balance", r.ShortPrevBalance, int64(131000)},
		{"short_sell", r.ShortSell, int64(12000)},
		{"short_convering", r.ShortConvering, int64(28000)},
		{"short_balance", r.ShortBalance, int64(115000)},
		{"short_rate", r.ShortRate, 0.06},
		{"offsetting", r.Offsetting, int64(3000)},
		{"note", r.Note, "11   A"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExAttention 注意股票契約（PE 為 N/A 時歸零）。
func TestTPExAttention(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_trading_warning_information"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExAttentionRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	r := rows[0]
	if r.Code != "2061" || r.Name != "風青" || r.Date != "2026-07-31" ||
		r.Close != 55.5 || r.PE != 0 || !strings.Contains(r.Info, "當日沖銷成交量") {
		t.Errorf("注意股契約錯誤: %+v", r)
	}
}

// TestTPExDisposition 處置股票契約。
func TestTPExDisposition(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_disposal_information"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExDispositionRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	r := rows[0]
	if r.Code != "3624" || r.Name != "光頡" || r.Date != "2026-07-31" ||
		r.Period != "1150803~1150814" || !strings.Contains(r.Reasons, "連續3個營業日") ||
		!strings.Contains(r.Condition, "人工管制") {
		t.Errorf("處置股契約錯誤: %+v", r)
	}
}

// TestTPExExRights 除權息預告契約（基準日欄位名不同）。
func TestTPExExRights(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_exright_prepost"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExExRightRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	r := rows[0]
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-22"},
		{"code", r.Code, "3402"},
		{"name", r.Name, "漢科"},
		{"kind", r.Kind, "除息"},
		{"cash_dividend", r.CashDividend, 6.0},
		{"subscription_price", r.SubscriptionPrice, 0.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExOddLot 零股交易契約（股/元）。
func TestTPExOddLot(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	out, err := s.Normalize(tpexFixture(t, "tpex_odd_stock"))
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []TPExOddLotRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	byCode := map[string]TPExOddLotRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	r, ok := byCode["6147"]
	if !ok {
		t.Fatalf("缺 6147 列（共 %d 列）", len(rows))
	}
	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"date", r.Date, "2026-07-31"},
		{"name", r.Name, "頎邦"},
		{"volume", r.Volume, int64(1455)},
		{"transactions", r.Transactions, int64(24)},
		{"amount", r.Amount, int64(189150)},
		{"price", r.Price, 130.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v，期望 %v", c.name, c.got, c.want)
		}
	}
}

// TestTPExCrossMarket 上市/上櫃邊界：上市 code（2330）查上櫃 → 空陣列非錯誤。
func TestTPExCrossMarket(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	raw := tpexRaw(t, "tpex_mainboard_quotes",
		"https://www.tpex.org.tw/openapi/v1/tpex_mainboard_quotes?stockNo=2330")
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("跨市場查詢應回傳空陣列而非錯誤: %v", err)
	}
	var rows []TPExDailyCloseRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("2330 於上櫃應為空，實際 %d 列", len(rows))
	}
	// 同市場過濾正常命中
	raw2 := tpexRaw(t, "tpex_mainboard_quotes",
		"https://www.tpex.org.tw/openapi/v1/tpex_mainboard_quotes?stockNo=6147")
	out2, err := s.Normalize(raw2)
	if err != nil {
		t.Fatalf("6147 過濾失敗: %v", err)
	}
	var rows2 []TPExDailyCloseRow
	if err := json.Unmarshal(out2, &rows2); err != nil {
		t.Fatalf("解析失敗: %v", err)
	}
	if len(rows2) != 1 || rows2[0].Code != "6147" {
		t.Errorf("6147 過濾應回 1 列，實際 %d", len(rows2))
	}
}

// TestTPExEmptyData 官方空陣列：Validate 合法，未過濾時 Normalize 報錯。
func TestTPExEmptyData(t *testing.T) {
	s := NewTPExSource(WithRateInterval(0))
	r := tpexRaw(t, "empty",
		"https://www.tpex.org.tw/openapi/v1/tpex_odd_stock")
	if err := s.Validate(r); err != nil {
		t.Fatalf("空陣列 Validate 應通過: %v", err)
	}
	if _, err := s.Normalize(r); err == nil {
		t.Fatal("空陣列未過濾 Normalize 應報錯")
	}
}

// TestTPExDatasetOf 未知路徑錯誤。
func TestTPExDatasetOf(t *testing.T) {
	r := &RawResponse{Body: []byte("[]"), SourceURL: "https://www.tpex.org.tw/openapi/v1/nope"}
	if _, err := tpexDatasetOf(r); err == nil {
		t.Fatal("未知路徑應報錯")
	}
}
