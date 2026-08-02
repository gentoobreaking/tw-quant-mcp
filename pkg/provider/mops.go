package provider

// MOPS Adapter（T012）：公開資訊觀測站（mopsfin.twse.com.tw）之月營收、財報摘要、
// 重大訊息、公司基本資料 Open Data CSV 端點。
// 實作 SourceContract（§2.2）。
//
// 端點清單（2026-07-31 實測）：
//   t187ap03_L = 公司基本資料（上市/上櫃/興櫃，全量）
//   t187ap04_L = 重大訊息（全量，近 ~30 日）
//   t187ap05_L = 月營收（全量，近 ~12 個月）
//   t187ap14_L = 損益表摘要（EPS/營收/營業利益/稅後淨利，全量近四季）
//   t187ap17_L = 獲利能力指標（毛利率/營益率/純益率，全量近四季）
//
// 已知限制與備註（T012）：
//   - MOPS Open Data 不提供完整資產負債表/現金流量表；僅提供損益表摘要
//     （t187ap14_L）與獲利能力指標（t187ap17_L）。完整 IFRS 財報需透過
//     MOPS 頁面 AJAX 端點（如 t164sb01）直接抓取 HTML table 後解析。
//     本 Adapter 目前以 Open Data CSV 為主要來源，完整財報 API 列為
//     後續事項（T012-followup：IFRS balance-sheet/cash-flow AJAX）。
//   - MOPS 頁面結構較多變，fixtures 需保留歷史版本。
//   - Rate Limit 1/2s（§4.4）；CSV 為全量 payload，注意 L1/Single-flight。
//   - CSV 編碼：多數為 UTF-8（含 BOM），部分舊版為 Big5。本實作以
//     UTF-8 為主；如遇 Big5 會回傳格式錯誤。

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"tw-quant-mcp/pkg/model"
)

// MOPSDataset 為 MOPS Open Data CSV 資料集 ID。
type MOPSDataset string

const (
	MOPSCompanyProfile  MOPSDataset = "company_profile"  // t187ap03_L：公司基本資料
	MOPSAnnouncements   MOPSDataset = "announcements"    // t187ap04_L：重大訊息
	MOPSMonthlyRevenue  MOPSDataset = "monthly_revenue"  // t187ap05_L：月營收
	MOPSIncomeSummary   MOPSDataset = "income_summary"   // t187ap14_L：損益表摘要
	MOPSProfitRatios    MOPSDataset = "profit_ratios"    // t187ap17_L：獲利能力指標
	MOPSBalanceSheet    MOPSDataset = "balance_sheet"    // ajax_t164sb03：合併資產負債表
	MOPSCashFlow        MOPSDataset = "cash_flow"        // ajax_t164sb05：合併現金流量表
	MOPSIncomeStatement MOPSDataset = "income_statement" // ajax_t164sb04：合併綜合損益表
)

// MOPS Open Data CSV 端點（2026-07 實測）。
const mopsOpenDataBase = "https://mopsfin.twse.com.tw/opendata"

// MOPS 舊版 AJAX 端點（mopsov.twse.com.tw，不需 CSRF cookie）。
const mopsAJAXBase = "https://mopsov.twse.com.tw/mops/web"

var mopsPaths = map[MOPSDataset]string{
	MOPSCompanyProfile:  "/t187ap03_L.csv",
	MOPSAnnouncements:   "/t187ap04_L.csv",
	MOPSMonthlyRevenue:  "/t187ap05_L.csv",
	MOPSIncomeSummary:   "/t187ap14_L.csv",
	MOPSProfitRatios:    "/t187ap17_L.csv",
	MOPSBalanceSheet:    "/ajax_t164sb03",
	MOPSCashFlow:        "/ajax_t164sb05",
	MOPSIncomeStatement: "/ajax_t164sb04",
}

// mopsOpenDataDatasets 為透過 Open Data CSV（mopsfin）取得之資料集。
// 其餘為 AJAX HTML table（mopsov）。
var mopsOpenDataDatasets = map[MOPSDataset]bool{
	MOPSCompanyProfile: true,
	MOPSAnnouncements:  true,
	MOPSMonthlyRevenue: true,
	MOPSIncomeSummary:  true,
	MOPSProfitRatios:   true,
}

// mopsNormalizeFilter 為 Normalize 階段之請求參數（從查詢工具傳入）。
// 因為 CSV 為全量資料，需於客戶端過濾 (symbol, date, keyword 等)。
type MOPSSource struct {
	client   *BaseClient
	filterFn func(*RawResponse) ([]byte, error) // 測試可注入之過濾函式
}

var _ SourceContract = (*MOPSSource)(nil)

// NewMOPSSource 建立 MOPS Open Data 來源（Rate Limit 1 req/2s，§4.4）。
func NewMOPSSource(opts ...Option) *MOPSSource {
	return &MOPSSource{client: NewBaseClient("mops.twse.com.tw", opts...)}
}

func (s *MOPSSource) ID() string { return model.SourceMOPS }

// URL 建立 MOPS 請求 URL。
// params 支援：
//   - dataset: MOPSDataset 值（必要，供路由用）
//   - co_id: 公司代號（AJAX 財報端點用）
//   - year: 年度（AJAX 財報端點用）
//   - season: 季別 1-4（AJAX 財報端點用）
//   - 其餘參數僅供標記用
func (s *MOPSSource) URL(ds MOPSDataset, params url.Values) string {
	path, ok := mopsPaths[ds]
	if !ok {
		path = "/" + string(ds)
	}
	if mopsOpenDataDatasets[ds] {
		return mopsOpenDataBase + path
	}
	return mopsAJAXBase + path
}

// Fetch 執行 MOPS HTTP 請求（Open Data 為 GET；AJAX 財報為 POST）。
func (s *MOPSSource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	if req.Method == "" {
		req.Method = http.MethodGet
		// AJAX 財報端點強制 POST
		if !mopsOpenDataDatasets[mopsDatasetOf(req.URL)] {
			req.Method = http.MethodPost
		}
	}
	return s.client.Do(ctx, req)
}

// Validate 執行 MOPS 回應結構檢查。
// AJAX 財報回應為含 HTML table 之完整 HTML page。
func (s *MOPSSource) Validate(raw *RawResponse) error {
	if raw.StatusCode != 200 {
		return fmt.Errorf("mops: 非預期 HTTP 狀態 %d", raw.StatusCode)
	}
	if len(raw.Body) == 0 {
		return fmt.Errorf("mops: 回應本體為空")
	}
	// AJAX 財報回應檢查：需含 <table> 標籤
	ds := mopsDatasetOf(raw.SourceURL)
	if ds != "" && !mopsOpenDataDatasets[ds] {
		body := string(raw.Body)
		if !strings.Contains(body, "<table") {
			return fmt.Errorf("mops: AJAX 回應不含 <table>")
		}
	}
	return nil
}

// Normalize 將 MOPS CSV raw payload 轉為歸一化 JSON。
// raw.SourceURL 需包含端點路徑以供分派。
// 支援 filterFn 注入（測試/MCP 過濾用）：非 nil 時直接委派。
// Deprecated: v2.1 §6 起轉換集中於 pkg/model/normalize（FromMOPS）；
// 本方法為 v1.3 相容層，遷移時逐步移除（T022）。
// filterFn 應自行呼叫 normalizeMOPSRaw 獲取原始結果後過濾，
// 避免遞迴呼叫 s.Normalize。
func (s *MOPSSource) Normalize(raw *RawResponse) ([]byte, error) {
	if s.filterFn != nil {
		return s.filterFn(raw)
	}
	return normalizeMOPSRaw(raw)
}

// RawNormalize 直接執行 normalize 邏輯，繞過 filterFn（供 filterFn 內部使用）。
func (s *MOPSSource) RawNormalize(raw *RawResponse) ([]byte, error) {
	return normalizeMOPSRaw(raw)
}

// normalizeMOPSRaw 依 raw.SourceURL 分派至各資料集之 Normalize 函式。
func normalizeMOPSRaw(raw *RawResponse) ([]byte, error) {
	ds := mopsDatasetOf(raw.SourceURL)

	// AJAX HTML table 資料集（財報三表）
	switch ds {
	case MOPSBalanceSheet:
		v, err := parseBalanceSheetHTML(raw.Body)
		if err != nil {
			return nil, err
		}
		return json.Marshal(v)
	case MOPSCashFlow:
		v, err := parseCashFlowHTML(raw.Body)
		if err != nil {
			return nil, err
		}
		return json.Marshal(v)
	case MOPSIncomeStatement:
		v, err := parseIncomeStatementHTML(raw.Body)
		if err != nil {
			return nil, err
		}
		return json.Marshal(v)
	}

	// Open Data CSV 資料集
	body := trimBOM(raw.Body)
	rc := newMOPSReader(body)
	defer rc.Close()

	header, err := rc.Read()
	if err != nil {
		return nil, fmt.Errorf("mops: CSV header 解析失敗: %w", err)
	}

	var v any
	switch ds {
	case MOPSCompanyProfile:
		v, err = parseCompanyProfiles(rc, header)
	case MOPSAnnouncements:
		v, err = parseAnnouncements(rc, header)
	case MOPSMonthlyRevenue:
		v, err = parseMonthlyRevenues(rc, header)
	case MOPSIncomeSummary:
		v, err = parseIncomeSummaries(rc, header)
	case MOPSProfitRatios:
		v, err = parseProfitabilityRatios(rc, header)
	default:
		return nil, fmt.Errorf("mops: 未實作之資料集 %q", ds)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// mopsDatasetOf 依 SourceURL 中之端點路徑判斷資料集。
func mopsDatasetOf(sourceURL string) MOPSDataset {
	for ds, path := range mopsPaths {
		if strings.Contains(sourceURL, path) {
			return ds
		}
	}
	return MOPSDataset(sourceURL)
}

// ExtractMeta 由 normalize 後的 []byte 解析為常用過濾結果。
// 此為 MCP 層級輔助：CSV 全量載入後於記憶體過濾（之後會由快取加速）。

// ---------------------------------------------------------------------------
// CSV reader（UTF-8，支援引號欄位）
// ---------------------------------------------------------------------------

type mopsCSVReader struct {
	r   *csv.Reader
	buf *bytes.Buffer
}

func newMOPSReader(data []byte) *mopsCSVReader {
	buf := bytes.NewBuffer(data)
	r := csv.NewReader(buf)
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1 // 可變欄位數
	return &mopsCSVReader{r: r, buf: buf}
}

func (r *mopsCSVReader) Read() ([]string, error) { return r.r.Read() }
func (r *mopsCSVReader) Close() error            { return nil }

// trimBOM 移除 BOM。
func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}

// ---------------------------------------------------------------------------
// 日期/年份/金額換算工具
// ---------------------------------------------------------------------------

// parseMOPSDate 將日期字串轉為 "YYYY-MM-DD"。
// 支援：7碼民國年（如 "1150731"）、8碼西元年（"19870221"）、
// 8碼民國年（"1150101"）、ISO 格式（"2026-07-31"）。
func parseMOPSDate(s string) (string, error) {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" {
		return "", fmt.Errorf("mops: 日期為空")
	}
	// 已為 ISO
	if len(s) == 10 && s[4] == '-' {
		return s, nil
	}
	// 7 碼（民國年，前導零或 1，如 "0890101"）
	if len(s) == 7 && (s[0] == '0' || s[0] == '1') {
		year, err := strconv.Atoi(s[:3])
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%04d-%s-%s", year+1911, s[3:5], s[5:7]), nil
	}
	// 8 碼：需判斷西元或民國年
	if len(s) == 8 {
		year, err := strconv.Atoi(s[:4])
		if err != nil {
			return "", err
		}
		// 西元 1900-2100 → 直接使用
		if year >= 1900 && year <= 2100 {
			return fmt.Sprintf("%s-%s-%s", s[:4], s[4:6], s[6:8]), nil
		}
		// 民國年 100-150（西元 2011-2061）
		if year >= 100 && year <= 150 {
			return fmt.Sprintf("%04d-%s-%s", year+1911, s[4:6], s[6:8]), nil
		}
		return fmt.Sprintf("%s-%s-%s", s[:4], s[4:6], s[6:8]), nil
	}
	return "", fmt.Errorf("mops: 無法識別日期格式 %q", s)
}

// parseMOPSDateSimple 將日期字串轉為 "YYYY-MM-DD"，失敗時回傳原字串。
// 適用於出表日期等非關鍵欄位。
func parseMOPSDateSimple(s string) (string, error) {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" {
		return "", nil
	}
	if len(s) == 7 {
		return parseMOPSDate(s)
	}
	if len(s) == 8 {
		return parseMOPSDate(s)
	}
	if len(s) >= 10 && s[4] == '-' {
		return s[:10], nil
	}
	return s, nil
}

// parseROCYear 將民國年（如 "115"）轉為西元年。
func parseROCYear(s string) (int, error) {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" {
		return 0, fmt.Errorf("mops: 年度為空")
	}
	y, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}
	if y < 1000 {
		return y + 1911, nil
	}
	return y, nil
}

// parseMOPSCents 將千元金額字串轉為元。
func parseMOPSCents(s string) int64 {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" || s == "-" || s == "0" || s == "0.00" || s == ".00" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 1000))
}

// parseMOPSFloat 解析 float64。
func parseMOPSFloat(s string) float64 {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" || s == "-" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// parseMOPSInt 解析 int64。
func parseMOPSInt(s string) int64 {
	s = strings.TrimSpace(strings.Trim(s, `"`))
	if s == "" || s == "-" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// parseMOPSQuoted 去除前後引號與空白。
func parseMOPSQuoted(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// 各資料集 CSV 解析
// ---------------------------------------------------------------------------

// 公司基本資料（t187ap03_L）
func parseCompanyProfiles(r *mopsCSVReader, header []string) ([]model.CompanyProfile, error) {
	c := resolveMOPSCols(header,
		"出表日期", "公司代號", "公司名稱", "公司簡稱", "外國企業註冊地國", "產業別",
		"住址", "營利事業統一編號", "董事長", "總經理", "發言人", "發言人職稱", "代理發言人",
		"總機電話", "成立日期", "上市日期", "普通股每股面額", "實收資本額", "私募股數", "特別股",
		"編制財務報表類型", "股票過戶機構", "過戶電話", "過戶地址", "簽證會計師事務所",
		"簽證會計師1", "簽證會計師2", "英文簡稱", "英文通訊地址", "傳真機號碼", "電子郵件信箱",
		"網址", "已發行普通股數或TDR原股發行股數")
	var rows []model.CompanyProfile
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mops: CSV 列解析失敗: %w", err)
		}
		rows = append(rows, model.CompanyProfile{
			TableDate:     mustDate(parseMOPSDateSimple(cell(rec, c, "出表日期"))),
			Code:          parseMOPSQuoted(cell(rec, c, "公司代號")),
			Name:          parseMOPSQuoted(cell(rec, c, "公司名稱")),
			ShortName:     parseMOPSQuoted(cell(rec, c, "公司簡稱")),
			ForeignReg:    parseMOPSQuoted(cell(rec, c, "外國企業註冊地國")),
			Industry:      parseMOPSQuoted(cell(rec, c, "產業別")),
			Address:       parseMOPSQuoted(cell(rec, c, "住址")),
			TaxID:         parseMOPSQuoted(cell(rec, c, "營利事業統一編號")),
			Chairman:      parseMOPSQuoted(cell(rec, c, "董事長")),
			President:     parseMOPSQuoted(cell(rec, c, "總經理")),
			Spokesman:     parseMOPSQuoted(cell(rec, c, "發言人")),
			SpokesTitle:   parseMOPSQuoted(cell(rec, c, "發言人職稱")),
			DepSpokes:     parseMOPSQuoted(cell(rec, c, "代理發言人")),
			Phone:         parseMOPSQuoted(cell(rec, c, "總機電話")),
			Established:   mustDate(parseMOPSDate(cell(rec, c, "成立日期"))),
			Listed:        mustDate(parseMOPSDate(cell(rec, c, "上市日期"))),
			ParValue:      parseMOPSQuoted(cell(rec, c, "普通股每股面額")),
			Capital:       parseMOPSCents(cell(rec, c, "實收資本額")),
			PrivateStock:  parseMOPSInt(cell(rec, c, "私募股數")),
			Preferred:     parseMOPSInt(cell(rec, c, "特別股")),
			FinType:       parseMOPSQuoted(cell(rec, c, "編制財務報表類型")),
			Transfer:      parseMOPSQuoted(cell(rec, c, "股票過戶機構")),
			TransferPhone: parseMOPSQuoted(cell(rec, c, "過戶電話")),
			TransferAddr:  parseMOPSQuoted(cell(rec, c, "過戶地址")),
			AuditorFirm:   parseMOPSQuoted(cell(rec, c, "簽證會計師事務所")),
			Auditor1:      parseMOPSQuoted(cell(rec, c, "簽證會計師1")),
			Auditor2:      parseMOPSQuoted(cell(rec, c, "簽證會計師2")),
			EngName:       parseMOPSQuoted(cell(rec, c, "英文簡稱")),
			EngAddr:       parseMOPSQuoted(cell(rec, c, "英文通訊地址")),
			Fax:           parseMOPSQuoted(cell(rec, c, "傳真機號碼")),
			Email:         parseMOPSQuoted(cell(rec, c, "電子郵件信箱")),
			Website:       parseMOPSQuoted(cell(rec, c, "網址")),
			SharesOut:     parseMOPSInt(cell(rec, c, "已發行普通股數或TDR原股發行股數")),
		})
	}
	return rows, nil
}

// 重大訊息（t187ap04_L）
func parseAnnouncements(r *mopsCSVReader, header []string) ([]model.MajorAnnouncement, error) {
	c := resolveMOPSCols(header,
		"出表日期", "發言日期", "發言時間", "公司代號", "公司名稱", "主旨",
		"符合條款", "事實發生日", "說明")
	var rows []model.MajorAnnouncement
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mops: CSV 列解析失敗: %w", err)
		}
		announceTime := parseMOPSQuoted(cell(rec, c, "發言時間"))
		// 正規化時間為 HH:MM:SS
		if len(announceTime) >= 4 && len(announceTime) <= 6 {
			t := announceTime
			if len(t) == 4 {
				t = "0" + t
			}
			if len(t) == 5 {
				t = t + ":00"
			}
			announceTime = fmt.Sprintf("%s:%s:%s", t[0:2], t[2:4], t[4:6])
		}
		rows = append(rows, model.MajorAnnouncement{
			TableDate:    mustDate(parseMOPSDateSimple(cell(rec, c, "出表日期"))),
			AnnounceDate: mustDate(parseMOPSDateSimple(cell(rec, c, "發言日期"))),
			AnnounceTime: announceTime,
			Code:         parseMOPSQuoted(cell(rec, c, "公司代號")),
			Name:         parseMOPSQuoted(cell(rec, c, "公司名稱")),
			Subject:      parseMOPSQuoted(cell(rec, c, "主旨")),
			Clause:       parseMOPSQuoted(cell(rec, c, "符合條款")),
			FactDate:     mustDate(parseMOPSDateSimple(cell(rec, c, "事實發生日"))),
			Description:  parseMOPSQuoted(cell(rec, c, "說明")),
		})
	}
	// 依公告日期（新→舊）排序
	sort.Slice(rows, func(i, j int) bool { return rows[i].AnnounceDate > rows[j].AnnounceDate })
	return rows, nil
}

// 月營收（t187ap05_L）
func parseMonthlyRevenues(r *mopsCSVReader, header []string) ([]model.MonthlyRevenueRow, error) {
	c := resolveMOPSCols(header,
		"出表日期", "資料年月", "公司代號", "公司名稱", "產業別",
		"營業收入-當月營收", "營業收入-上月營收", "營業收入-去年當月營收",
		"營業收入-上月比較增減(%)", "營業收入-去年同月增減(%)",
		"累計營業收入-當月累計營收", "累計營業收入-去年累計營收",
		"累計營業收入-前期比較增減(%)", "備註")
	var rows []model.MonthlyRevenueRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mops: CSV 列解析失敗: %w", err)
		}
		dataYM := parseMOPSQuoted(cell(rec, c, "資料年月"))
		// 民國年 → 西元年
		if len(dataYM) == 5 {
			y, _ := strconv.Atoi(dataYM[:3])
			dataYM = fmt.Sprintf("%04d%s", y+1911, dataYM[3:])
		}
		rows = append(rows, model.MonthlyRevenueRow{
			TableDate:        mustDate(parseMOPSDateSimple(cell(rec, c, "出表日期"))),
			DataYearMonth:    dataYM,
			Code:             parseMOPSQuoted(cell(rec, c, "公司代號")),
			Name:             parseMOPSQuoted(cell(rec, c, "公司名稱")),
			Industry:         parseMOPSQuoted(cell(rec, c, "產業別")),
			Revenue:          parseMOPSCents(cell(rec, c, "營業收入-當月營收")),
			LastMonthRevenue: parseMOPSCents(cell(rec, c, "營業收入-上月營收")),
			LastYearRevenue:  parseMOPSCents(cell(rec, c, "營業收入-去年當月營收")),
			MoMChange:        parseMOPSFloat(cell(rec, c, "營業收入-上月比較增減(%)")),
			YoYChange:        parseMOPSFloat(cell(rec, c, "營業收入-去年同月增減(%)")),
			CumRevenue:       parseMOPSCents(cell(rec, c, "累計營業收入-當月累計營收")),
			CumLastYear:      parseMOPSCents(cell(rec, c, "累計營業收入-去年累計營收")),
			CumChange:        parseMOPSFloat(cell(rec, c, "累計營業收入-前期比較增減(%)")),
			Note:             parseMOPSQuoted(cell(rec, c, "備註")),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].DataYearMonth > rows[j].DataYearMonth })
	return rows, nil
}

// 損益表摘要（t187ap14_L）
func parseIncomeSummaries(r *mopsCSVReader, header []string) ([]model.IncomeStatementRow, error) {
	c := resolveMOPSCols(header,
		"出表日期", "年度", "季別", "公司代號", "公司名稱", "產業別",
		"基本每股盈餘(元)", "普通股每股面額", "營業收入", "營業利益",
		"營業外收入及支出", "稅後淨利")
	var rows []model.IncomeStatementRow
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mops: CSV 列解析失敗: %w", err)
		}
		year, _ := parseROCYear(cell(rec, c, "年度"))
		q, _ := strconv.Atoi(strings.TrimSpace(cell(rec, c, "季別")))

		rows = append(rows, model.IncomeStatementRow{
			TableDate:         mustDate(parseMOPSDateSimple(cell(rec, c, "出表日期"))),
			Year:              year,
			Quarter:           q,
			Code:              parseMOPSQuoted(cell(rec, c, "公司代號")),
			Name:              parseMOPSQuoted(cell(rec, c, "公司名稱")),
			Industry:          parseMOPSQuoted(cell(rec, c, "產業別")),
			EPS:               parseMOPSFloat(cell(rec, c, "基本每股盈餘(元)")),
			ParValue:          parseMOPSQuoted(cell(rec, c, "普通股每股面額")),
			Revenue:           parseMOPSCents(cell(rec, c, "營業收入")),
			OperatingProfit:   parseMOPSCents(cell(rec, c, "營業利益")),
			NonOperatingItems: parseMOPSCents(cell(rec, c, "營業外收入及支出")),
			NetIncome:         parseMOPSCents(cell(rec, c, "稅後淨利")),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Year != rows[j].Year {
			return rows[i].Year > rows[j].Year
		}
		return rows[i].Quarter > rows[j].Quarter
	})
	return rows, nil
}

// 獲利能力指標（t187ap17_L）
func parseProfitabilityRatios(r *mopsCSVReader, header []string) ([]model.ProfitabilityRatio, error) {
	c := resolveMOPSCols(header,
		"出表日期", "年度", "季別", "公司代號", "公司名稱",
		"營業收入(百萬元)", "毛利率(%)(營業毛利)/(營業收入)",
		"營業利益率(%)(營業利益)/(營業收入)", "稅前純益率(%)(稅前純益)/(營業收入)",
		"稅後純益率(%)(稅後純益)/(營業收入)")
	var rows []model.ProfitabilityRatio
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("mops: CSV 列解析失敗: %w", err)
		}
		year, _ := parseROCYear(cell(rec, c, "年度"))
		q, _ := strconv.Atoi(strings.TrimSpace(cell(rec, c, "季別")))

		rows = append(rows, model.ProfitabilityRatio{
			TableDate:       mustDate(parseMOPSDateSimple(cell(rec, c, "出表日期"))),
			Year:            year,
			Quarter:         q,
			Code:            parseMOPSQuoted(cell(rec, c, "公司代號")),
			Name:            parseMOPSQuoted(cell(rec, c, "公司名稱")),
			RevenueMillion:  parseMOPSFloat(cell(rec, c, "營業收入(百萬元)")),
			GrossMargin:     parseMOPSFloat(cell(rec, c, "毛利率(%)(營業毛利)/(營業收入)")),
			OperatingMargin: parseMOPSFloat(cell(rec, c, "營業利益率(%)(營業利益)/(營業收入)")),
			PreTaxMargin:    parseMOPSFloat(cell(rec, c, "稅前純益率(%)(稅前純益)/(營業收入)")),
			NetMargin:       parseMOPSFloat(cell(rec, c, "稅後純益率(%)(稅後純益)/(營業收入)")),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Year != rows[j].Year {
			return rows[i].Year > rows[j].Year
		}
		return rows[i].Quarter > rows[j].Quarter
	})
	return rows, nil
}

// ---------------------------------------------------------------------------
// CSV 輔助工具
// ---------------------------------------------------------------------------

// headerMap 建立 header → index 對映。
func headerMap(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		h = strings.TrimSpace(strings.Trim(h, `"`))
		// 去除後綴空格（MOPS header 常見 "欄位名  " 格式）
		h = strings.TrimRight(h, " ")
		m[h] = i
	}
	return m
}

// resolveMOPSCols 預解析 header 欄位 index（每請求一次，取代逐列模糊比對）。
// 缺失欄位以 -1 標記；含既有 get() 之模糊比對邏輯（部分欄位名含多餘空格）。
func resolveMOPSCols(header []string, names ...string) map[string]int {
	m := headerMap(header)
	c := make(map[string]int, len(names))
	for _, n := range names {
		idx, ok := m[n]
		if !ok {
			for k, v := range m {
				if strings.Contains(k, n) || strings.Contains(n, k) {
					idx, ok = v, true
					break
				}
			}
		}
		if !ok {
			idx = -1
		}
		c[n] = idx
	}
	return c
}

// cell 以預解析 index 取值；缺失或越界回傳 ""。
func cell(rec []string, c map[string]int, name string) string {
	idx, ok := c[name]
	if !ok || idx < 0 || idx >= len(rec) {
		return ""
	}
	return rec[idx]
}

// mustDate 包裝 date 回傳（忽略 parse 錯誤時回傳 ""）。
func mustDate(s string, err error) string {
	if err != nil {
		return ""
	}
	return s
}
