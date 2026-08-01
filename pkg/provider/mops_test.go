package provider

// MOPS Adapter 契約測試（T012）：以 2026-07-31 實地錄製之 MOPS Open Data CSV
// fixtures（testdata/mops/，欄位/數值為官方原文）驗證 Fetch→Validate→Normalize：
// 欄位型別、單位換算（千元→元，§5.1）、日期格式（民國/西元年→RFC3339 日期）。

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"tw-quant-mcp/pkg/model"
)

func loadMOPSFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "mops", name))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return b
}

func mopsFixtureRaw(t *testing.T, sourceURL string, body []byte) *RawResponse {
	t.Helper()
	return &RawResponse{
		StatusCode: http.StatusOK,
		Body:       body,
		SourceURL:  sourceURL,
		FetchedAt:  model.Now(),
	}
}

func newTestMOPSSource() *MOPSSource {
	return NewMOPSSource(WithRateInterval(0))
}

// TestMOPSURL 驗證 MOPS Open Data URL 建構。
func TestMOPSURL(t *testing.T) {
	s := newTestMOPSSource()

	tests := []struct {
		ds  MOPSDataset
		url string
	}{
		{MOPSCompanyProfile, "https://mopsfin.twse.com.tw/opendata/t187ap03_L.csv"},
		{MOPSAnnouncements, "https://mopsfin.twse.com.tw/opendata/t187ap04_L.csv"},
		{MOPSMonthlyRevenue, "https://mopsfin.twse.com.tw/opendata/t187ap05_L.csv"},
		{MOPSIncomeSummary, "https://mopsfin.twse.com.tw/opendata/t187ap14_L.csv"},
		{MOPSProfitRatios, "https://mopsfin.twse.com.tw/opendata/t187ap17_L.csv"},
	}
	for _, tt := range tests {
		got := s.URL(tt.ds, nil)
		if got != tt.url {
			t.Errorf("%s URL = %q, want %q", tt.ds, got, tt.url)
		}
	}
}

// TestMOPSCompanyProfile 驗證公司基本資料欄位完整度與日期格式。
func TestMOPSCompanyProfile(t *testing.T) {
	body := loadMOPSFixture(t, "company_profile.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSCompanyProfile], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.CompanyProfile
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("回傳空陣列")
	}

	// 找到 2330 台積電
	var found bool
	for _, r := range rows {
		if r.Code == "2330" {
			found = true
			if r.Name == "" {
				t.Error("公司名稱為空")
			}
			if r.Capital <= 0 {
				t.Error("實收資本額為零（可能未換算仟元→元）")
			}
			// 日期格式驗證
			if r.Established != "" && len(r.Established) != 10 {
				t.Errorf("成立日期格式錯誤: %q", r.Established)
			}
			if r.Listed != "" && len(r.Listed) != 10 {
				t.Errorf("上市日期格式錯誤: %q", r.Listed)
			}
			t.Logf("台積電: Capital=%d, SharesOut=%d, Est=%q, Listed=%q",
				r.Capital, r.SharesOut, r.Established, r.Listed)
			break
		}
	}
	if !found {
		t.Error("找不到台積電（2330）")
	}
}

// TestMOPSAnnouncements 驗證重大訊息欄位完整度、日期格式與排序。
func TestMOPSAnnouncements(t *testing.T) {
	body := loadMOPSFixture(t, "announcements.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSAnnouncements], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.MajorAnnouncement
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("回傳空陣列")
	}

	// 驗證排序（日期遞減）
	for i := 1; i < len(rows); i++ {
		if rows[i-1].AnnounceDate < rows[i].AnnounceDate {
			t.Errorf("重大訊息未依日期排序: rows[%d].AnnounceDate=%q < rows[%d].AnnounceDate=%q",
				i-1, rows[i-1].AnnounceDate, i, rows[i].AnnounceDate)
			break
		}
	}

	// 驗證欄位完整度
	for _, r := range rows {
		if r.Code == "" {
			t.Error("公司代號為空")
		}
		if r.Subject == "" {
			t.Error("主旨為空")
		}
		if r.AnnounceDate == "" {
			t.Error("發言日期為空")
		}
	}

	t.Logf("重大訊息共 %d 筆，最新一筆: %s %s %s", len(rows),
		rows[0].AnnounceDate, rows[0].Code, rows[0].Subject)
}

// TestMOPSMonthlyRevenue 驗證月營收欄位完整度與單位換算。
func TestMOPSMonthlyRevenue(t *testing.T) {
	body := loadMOPSFixture(t, "monthly_revenue.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSMonthlyRevenue], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.MonthlyRevenueRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("回傳空陣列")
	}

	// 找到 2330 台積電
	var found bool
	for _, r := range rows {
		if r.Code == "2330" {
			found = true
			// 營收單位驗證：官方為千元 → 我們換算為元（×1000）
			// 台積電 2025年6月營收約 4426.8 億 = 442,679,969 千元 → 442,679,969,000 元
			if r.Revenue <= 0 {
				t.Error("營收為零（可能未換算單位）")
			}
			if r.DataYearMonth == "" {
				t.Error("資料年月為空")
			}
			t.Logf("台積電 %s 營收: %d 元 (YoY: %.2f%%, MoM: %.2f%%)",
				r.DataYearMonth, r.Revenue, r.YoYChange, r.MoMChange)
			break
		}
	}
	if !found {
		t.Error("找不到台積電（2330）月營收")
	}

	// 驗證排序（年月遞減）
	for i := 1; i < len(rows); i++ {
		// 同公司檢查排序
		if rows[i-1].Code == rows[i].Code {
			if rows[i-1].DataYearMonth < rows[i].DataYearMonth {
				t.Errorf("月營收未依年月排序: %q < %q for code=%s",
					rows[i-1].DataYearMonth, rows[i].DataYearMonth, rows[i].Code)
				break
			}
		}
	}
}

// TestMOPSIncomeSummary 驗證損益表摘要欄位完整度與單位換算。
func TestMOPSIncomeSummary(t *testing.T) {
	body := loadMOPSFixture(t, "income_summary.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSIncomeSummary], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.IncomeStatementRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("回傳空陣列")
	}

	// 找到 2330 台積電
	var found bool
	for _, r := range rows {
		if r.Code == "2330" {
			found = true
			if r.Revenue <= 0 {
				t.Error("營收為零（可能未換算仟元→元）")
			}
			if r.EPS <= 0 {
				t.Error("EPS 為零")
			}
			if r.Year <= 0 || r.Quarter <= 0 {
				t.Error("年度/季別為零")
			}
			t.Logf("台積電 %dQ%d: Revenue=%d, OpProfit=%d, NetIncome=%d, EPS=%.2f",
				r.Year, r.Quarter, r.Revenue, r.OperatingProfit, r.NetIncome, r.EPS)
			break
		}
	}
	if !found {
		t.Error("找不到台積電（2330）損益表")
	}

	// 驗證無 nil 導致的 pointer 問題（所有 rows 需可序列化）
	b2, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("重新序列化失敗: %v", err)
	}
	if len(b2) == 0 {
		t.Error("重新序列化為空")
	}
}

// TestMOPSProfitabilityRatios 驗證獲利能力指標。
func TestMOPSProfitabilityRatios(t *testing.T) {
	body := loadMOPSFixture(t, "profit_ratios.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSProfitRatios], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.ProfitabilityRatio
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("回傳空陣列")
	}

	// 找到 2330 台積電
	var found bool
	for _, r := range rows {
		if r.Code == "2330" {
			found = true
			if r.GrossMargin <= 0 {
				t.Error("毛利率為零")
			}
			if r.NetMargin <= 0 {
				t.Error("純益率為零")
			}
			t.Logf("台積電 %dQ%d: GrossMargin=%.2f%%, OpMargin=%.2f%%, NetMargin=%.2f%%",
				r.Year, r.Quarter, r.GrossMargin, r.OperatingMargin, r.NetMargin)
			break
		}
	}
	if !found {
		t.Error("找不到台積電（2330）獲利能力")
	}
}

// TestMOPSValidate 驗證 Validate 異常情境。
func TestMOPSValidate(t *testing.T) {
	s := newTestMOPSSource()

	// 非 200 狀態
	if err := s.Validate(&RawResponse{StatusCode: 404}); err == nil {
		t.Error("404 應回傳錯誤")
	}

	// 空 body
	if err := s.Validate(&RawResponse{StatusCode: 200, Body: nil}); err == nil {
		t.Error("空 body 應回傳錯誤")
	}

	// 正常情境
	body := loadMOPSFixture(t, "company_profile.csv")
	if err := s.Validate(&RawResponse{StatusCode: 200, Body: body}); err != nil {
		t.Errorf("正常 Validate 應成功: %v", err)
	}
}

// TestMOPSDatasetOf 驗證 URL 路由分派。
func TestMOPSDatasetOf(t *testing.T) {
	tests := []struct {
		url string
		ds  MOPSDataset
	}{
		{"https://mopsfin.twse.com.tw/opendata/t187ap03_L.csv", MOPSCompanyProfile},
		{"https://mopsfin.twse.com.tw/opendata/t187ap04_L.csv", MOPSAnnouncements},
		{"https://mopsfin.twse.com.tw/opendata/t187ap05_L.csv", MOPSMonthlyRevenue},
		{"https://mopsfin.twse.com.tw/opendata/t187ap14_L.csv", MOPSIncomeSummary},
		{"https://mopsfin.twse.com.tw/opendata/t187ap17_L.csv", MOPSProfitRatios},
	}
	for _, tt := range tests {
		got := mopsDatasetOf(tt.url)
		if got != tt.ds {
			t.Errorf("mopsDatasetOf(%q) = %q, want %q", tt.url, got, tt.ds)
		}
	}
}

// TestMOPSFilterNormalize 驗證 filterFn 注入機制（供 MCP 層過濾用）。
func TestMOPSFilterNormalize(t *testing.T) {
	body := loadMOPSFixture(t, "company_profile.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSCompanyProfile], body)

	s := newTestMOPSSource()
	s.filterFn = func(r *RawResponse) ([]byte, error) {
		// 模擬 MCP 層過濾：先 RawNormalize 再過濾只保留 2330
		all, err := s.RawNormalize(r)
		if err != nil {
			return nil, err
		}
		var rows []model.CompanyProfile
		if err := json.Unmarshal(all, &rows); err != nil {
			return nil, err
		}
		var filtered []model.CompanyProfile
		for _, r := range rows {
			if r.Code == "2330" {
				filtered = append(filtered, r)
			}
		}
		return json.Marshal(filtered)
	}

	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("過濾 Normalize 失敗: %v", err)
	}
	var rows []model.CompanyProfile
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("過濾後應為 1 筆，實際 %d 筆", len(rows))
	}
	if rows[0].Code != "2330" {
		t.Errorf("過濾結果應為 2330，實際 %s", rows[0].Code)
	}
	t.Logf("過濾成功: %s %s", rows[0].Code, rows[0].Name)
}

// TestMOPSID 驗證 Source ID。
func TestMOPSID(t *testing.T) {
	s := newTestMOPSSource()
	if s.ID() != model.SourceMOPS {
		t.Errorf("ID = %q, want %q", s.ID(), model.SourceMOPS)
	}
}

// TestMOPSRateLimit 驗證 Rate Limit 設定。
func TestMOPSRateLimit(t *testing.T) {
	// WithRateInterval(0) 會觸發 NewHostLimiter 改用預設值（mops = 2s，§4.4）
	s := &MOPSSource{client: NewBaseClient("mops.twse.com.tw", WithRateInterval(0))}
	got := s.client.RateInterval()
	t.Logf("WithRateInterval(0) → %v（NewHostLimiter 以 0 觸發預設值）", got)

	// 正式 source 應有正數間隔
	realSource := NewMOPSSource()
	if realGot := realSource.client.RateInterval(); realGot <= 0 {
		t.Errorf("正式 RateInterval = %v, want > 0", realGot)
	}
}

// TestMOPSParseDate 驗證日期解析（民國/西元/ISO）。
func TestMOPSParseDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"1150731", "2026-07-31"},
		{"20260731", "2026-07-31"},
		{"2026-07-31", "2026-07-31"},
		{"1150101", "2026-01-01"},
	}
	for _, tt := range tests {
		got, err := parseMOPSDate(tt.input)
		if err != nil {
			t.Errorf("parseMOPSDate(%q) 錯誤: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMOPSDate(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestMOPSUnits 驗證營收單位換算（仟元→元）。
func TestMOPSUnits(t *testing.T) {
	// 官方原始資料：台積電 11506 月營收 = 442,679,969 千元
	// 換算後應為 442,679,969,000 元
	body := loadMOPSFixture(t, "monthly_revenue.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSMonthlyRevenue], body)

	s := newTestMOPSSource()
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []model.MonthlyRevenueRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}

	for _, r := range rows {
		if r.Code == "2330" {
			if r.Revenue < 1e11 {
				t.Errorf("台積電營收 %d < 1000 億，疑似未換算仟元→元", r.Revenue)
			}
			if r.CumRevenue < 1e12 {
				t.Errorf("台積電累計營收 %d < 1 兆，疑似未換算仟元→元", r.CumRevenue)
			}
			t.Logf("單位驗證通過: Revenue=%d, CumRevenue=%d", r.Revenue, r.CumRevenue)
			break
		}
	}
}

// TestMOPSContext 驗證 Fetch 可被 context 取消。
// TestMOPSHTMLValueCol 驗證「本季」欄定位
// （資產負債表=期末日欄、現金流量=本季期間欄、損益表=第N季單季欄）。
func TestMOPSHTMLValueCol(t *testing.T) {
	cases := []struct {
		name, file string
		want       int
	}{
		{"資產負債表", "balance_sheet_2330_2026Q1.html", 1},
		{"現金流量表", "cash_flow_2330_2026Q1.html", 1},
		{"綜合損益表", "income_statement_2330_2026Q1.html", 3},
	}
	for _, tc := range cases {
		body := loadMOPSHTMLFixture(t, tc.file)
		_, _, valueCol, err := parseMOPSHTMLTable(body)
		if err != nil {
			t.Fatalf("%s 解析失敗: %v", tc.name, err)
		}
		if valueCol != tc.want {
			t.Errorf("%s 本季欄應為 %d，實際 %d", tc.name, tc.want, valueCol)
		}
	}
}

// TestMOPSAmountLabelPriority 驗證 labelMap 優先與 rows fallback 語意：
// 欄位存在（含值為 0）時不得以「值為 0」誤觸 fallback。
func TestMOPSAmountLabelPriority(t *testing.T) {
	rows := []mopsTableRow{{label: "資產總額", values: []string{"999"}}}
	// labelMap 優先
	m := map[string]string{"資產總額": "1,000"}
	if got := mopsAmount(m, rows, "資產總額", "資產總"); got != 1000000 {
		t.Errorf("labelMap 優先應為 1,000×1000=1000000，實際 %d", got)
	}
	// 欄位缺失 → fallback 由 rows 比對
	m = map[string]string{}
	if got := mopsAmount(m, rows, "資產總額", "資產總"); got != 999000 {
		t.Errorf("fallback 應為 999×1000=999000，實際 %d", got)
	}
	// 欄位存在但值為 0 → 採用 labelMap 值，不觸發 fallback
	m = map[string]string{"資產總額": "0"}
	if got := mopsAmount(m, rows, "資產總額", "資產總"); got != 0 {
		t.Errorf("值為 0 時應直接採用 labelMap，實際 %d", got)
	}
}

// TestMOPSYearQuarterPrefersTblHead 驗證年度/季別解析優先自標題列，
// 避免誤中頁面其他「民國」字樣（如出表日期）。
func TestMOPSYearQuarterPrefersTblHead(t *testing.T) {
	body := `<th class='tblHead' colspan='10'>民國115年第1季</tr>出表日期：民國115年5月15日`
	year, quarter := mopsYearQuarter(body)
	if year != 2026 || quarter != 1 {
		t.Errorf("應解析為 2026Q1，實際 %dQ%d", year, quarter)
	}
}

func TestMOPSContext(t *testing.T) {
	s := newTestMOPSSource()
	req := RawRequest{Method: "GET", URL: mopsOpenDataBase + mopsPaths[MOPSCompanyProfile]}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	if _, err := s.Fetch(ctx, req); err == nil {
		t.Error("已取消的 context 應回傳錯誤")
	}
}

// ---------------------------------------------------------------------------
// AJAX HTML table 解析測試（財報三表，T012-followup）
// ---------------------------------------------------------------------------

func loadMOPSHTMLFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", "mops", name))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return body
}

// TestMOPSBalanceSheetHTML 驗證合併資產負債表 HTML 解析。
func TestMOPSBalanceSheetHTML(t *testing.T) {
	body := loadMOPSHTMLFixture(t, "balance_sheet_2330_2026Q1.html")
	raw := &RawResponse{
		StatusCode: 200,
		Body:       body,
		SourceURL:  "https://mopsov.twse.com.tw/mops/web/ajax_t164sb03",
	}
	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var bs model.BalanceSheet
	if err := json.Unmarshal(out, &bs); err != nil {
		t.Fatalf("Unmarshal 失敗: %v", err)
	}
	t.Logf("台積電 2026Q1 資產負債表: 資產=%d 負債=%d 權益=%d", bs.TotalAssets, bs.TotalLiabilities, bs.TotalEquity)

	// 官方 2026Q1：資產總額 8,660,950（百萬元）→ 8,660,949,685 仟元 → 8,660,949,685,000 元
	// 驗證 TotalAssets
	if bs.TotalAssets != 8660949685000 {
		t.Errorf("TotalAssets 應為 8660949685000（元），實際 %d", bs.TotalAssets)
	}
	if bs.TotalLiabilities != 2728560764000 {
		t.Errorf("TotalLiabilities 應為 2728560764000，實際 %d", bs.TotalLiabilities)
	}
	if bs.TotalEquity != 5932388921000 {
		t.Errorf("TotalEquity 應為 5932388921000，實際 %d", bs.TotalEquity)
	}
	if bs.Year != 2026 || bs.Quarter != 1 {
		t.Errorf("Year/Quarter 應為 2026/1，實際 %d/%d", bs.Year, bs.Quarter)
	}
}

// TestMOPSCashFlowHTML 驗證合併現金流量表 HTML 解析。
func TestMOPSCashFlowHTML(t *testing.T) {
	body := loadMOPSHTMLFixture(t, "cash_flow_2330_2026Q1.html")
	raw := &RawResponse{
		StatusCode: 200,
		Body:       body,
		SourceURL:  "https://mopsov.twse.com.tw/mops/web/ajax_t164sb05",
	}
	s := newTestMOPSSource()
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var cf model.CashFlowStatement
	if err := json.Unmarshal(out, &cf); err != nil {
		t.Fatalf("Unmarshal 失敗: %v", err)
	}
	t.Logf("台積電 2026Q1 現金流量: 營業=%d 投資=%d 籌資=%d 期末現金=%d",
		cf.OperatingCashFlow, cf.InvestingCashFlow, cf.FinancingCashFlow, cf.EndingCashBalance)

	if cf.OperatingCashFlow != 698976265000 {
		t.Errorf("OperatingCashFlow 應為 698976265000，實際 %d", cf.OperatingCashFlow)
	}
	if cf.InvestingCashFlow != -356853756000 {
		t.Errorf("InvestingCashFlow 應為 -356853756000，實際 %d", cf.InvestingCashFlow)
	}
	if cf.FinancingCashFlow != -119910612000 {
		t.Errorf("FinancingCashFlow 應為 -119910612000，實際 %d", cf.FinancingCashFlow)
	}
	if cf.EndingCashBalance != 3035637228000 {
		t.Errorf("EndingCashBalance 應為 3035637228000，實際 %d", cf.EndingCashBalance)
	}
}

// TestMOPSIncomeStatementHTML 驗證合併綜合損益表 HTML 解析。
func TestMOPSIncomeStatementHTML(t *testing.T) {
	body := loadMOPSHTMLFixture(t, "income_statement_2330_2026Q1.html")
	raw := &RawResponse{
		StatusCode: 200,
		Body:       body,
		SourceURL:  "https://mopsov.twse.com.tw/mops/web/ajax_t164sb04",
	}
	s := newTestMOPSSource()
	out, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var is model.IncomeStatementRow
	if err := json.Unmarshal(out, &is); err != nil {
		t.Fatalf("Unmarshal 失敗: %v", err)
	}
	t.Logf("台積電 2026Q1 損益: 營收=%d 營業利益=%d 稅後淨利=%d", is.Revenue, is.OperatingProfit, is.NetIncome)

	if is.Revenue != 1134103440000 {
		t.Errorf("Revenue 應為 1134103440000，實際 %d", is.Revenue)
	}
	if is.OperatingProfit != 658966142000 {
		t.Errorf("OperatingProfit 應為 658966142000，實際 %d", is.OperatingProfit)
	}
	if is.NetIncome != 572801304000 {
		t.Errorf("NetIncome 應為 572801304000，實際 %d", is.NetIncome)
	}
}
