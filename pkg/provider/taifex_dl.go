package provider

// TAIFEX-DL Adapter（T013）：期交所官方網站下載頁（www.taifex.com.tw，cold tier）
// 歷史回溯來源，實作 SourceContract（§2.2、§9）。
//
// 端點（2026-07-31 實測，二步式 GET view 頁 → POST 下載 CSV，均為 Big5/MS950）：
//
//	期貨每日交易行情      GET futDailyMarketView → POST futDataDown
//	選擇權每日交易行情    GET optDailyMarketView → POST dlOptDataDown
//	三大法人期貨交易量    GET futContractsDateView → POST futContractsDateDown
//	大額交易人期貨部位    GET largeTraderFutView → POST largeTraderFutDown
//	大額交易人選擇權部位  GET largeTraderOptView → POST largeTraderOptDown
//	選擇權Put/Call比      GET dlPcRatio → POST dlPcRatioDown
//
// POST 參數（form-urlencoded）：
//
//	futDataDown / dlOptDataDown:  down_type=1&commodity_id=<contract>&commodity_id2=
//	                             &queryStartDate=YYYY/MM/DD&queryEndDate=YYYY/MM/DD
//	futContractsDateDown / largeTraderFutDown / largeTraderOptDown:
//	                             queryStartDate&queryEndDate（無 down_type）
//	dlPcRatioDown:               down_type=1&queryStartDate&queryEndDate
//
// 端點特性（2026-07-31 實測）：
//   - 實測單次 POST 即成功（不需 view GET 之 session cookie），但維持二步式
//     （§9.3 之 view→post）以符合官方網頁互動流程；view GET 失敗不阻斷下載。
//   - CSV 為 Big5/MS950 編碼（content-type text/html;charset=MS950），需轉 UTF-8。
//   - 週六/無交易日回傳僅含表頭之 CSV（無資料列）。
//   - 欄位含千分位逗號、"-" 表無值；大額交易人欄位含空白 padding。
//
// 單位（§5.1）：法人金額欄位為「千元」→ ×1000 元；其餘即「口」。
// DL 無保證金資料集（TAMargin 僅 API）。

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
)

const taifexDLBase = "https://www.taifex.com.tw/cht/3/"

// taifexDLSpec 描述單一 DL 資料集之下載端點與表單欄位。
type taifexDLSpec struct {
	view   string            // view 頁 path（Referer/二步式用）
	down   string            // 下載 POST path
	fields []string          // POST 欄位順序（表單定義）
	def    map[string]string // 固定預設值（down_type 等）
}

// taifexDLSpecs 為 DL 資料集 → 端點規格（2026-07-31 實測）。
var taifexDLSpecs = map[model.TAIFEXDataset]taifexDLSpec{
	model.TAFuturesDaily: {
		view: "futDailyMarketView", down: "futDataDown",
		fields: []string{"down_type", "commodity_id", "commodity_id2", "queryStartDate", "queryEndDate"},
		def:    map[string]string{"down_type": "1"},
	},
	model.TAOptionsDaily: {
		view: "optDailyMarketView", down: "dlOptDataDown",
		fields: []string{"down_type", "commodity_id", "commodity_id2", "queryStartDate", "queryEndDate"},
		def:    map[string]string{"down_type": "1"},
	},
	model.TAInstiFutures: {
		view: "futContractsDateView", down: "futContractsDateDown",
		fields: []string{"queryStartDate", "queryEndDate"},
	},
	model.TALargeTraderFut: {
		view: "largeTraderFutView", down: "largeTraderFutDown",
		fields: []string{"queryStartDate", "queryEndDate"},
	},
	model.TALargeTraderOpt: {
		view: "largeTraderOptView", down: "largeTraderOptDown",
		fields: []string{"queryStartDate", "queryEndDate"},
	},
	model.TAPutCallRatio: {
		view: "dlPcRatio", down: "dlPcRatioDown",
		fields: []string{"down_type", "queryStartDate", "queryEndDate"},
		def:    map[string]string{"down_type": "1"},
	},
}

// dlSupported 判定資料集是否由 DL 提供（§9.2；保證金僅 API）。
func dlSupported(ds model.TAIFEXDataset) bool {
	_, ok := taifexDLSpecs[ds]
	return ok
}

// NewTAIFEXDLSource 建立 TAIFEX-DL 來源（Rate Limit 1 req/5s，§4.4）。
// DL 大 CSV 下載建議 timeout 60s（§T003 備註）。
func NewTAIFEXDLSource(opts ...Option) *TAIFEXDLSource {
	opts = append([]Option{WithTimeout(defaultDLTimeout)}, opts...)
	return &TAIFEXDLSource{client: NewBaseClient("www.taifex.com.tw", opts...)}
}

// defaultDLTimeout 為 DL 下載 timeout（大 CSV）。
const defaultDLTimeout = 60 * time.Second

// TAIFEXDLSource 實作 SourceContract（§2.2），ID = TAIFEX_DL。
type TAIFEXDLSource struct {
	client  *BaseClient
	baseURL string // 測試覆寫用（httptest）；空 = 官方基底
}

var _ SourceContract = (*TAIFEXDLSource)(nil)

func (s *TAIFEXDLSource) ID() string { return model.SourceTAIFEXDL }

// base 回傳基底 URL（測試覆寫優先）。
func (s *TAIFEXDLSource) base() string {
	if s.baseURL != "" {
		return s.baseURL
	}
	return taifexDLBase
}

// URL 建立資料集之 view 頁 URL（二步式第一步）。
// params 支援 queryStartDate/queryEndDate（YYYY/MM/DD）與 commodity_id。
func (s *TAIFEXDLSource) URL(ds model.TAIFEXDataset, params url.Values) string {
	spec, ok := taifexDLSpecs[ds]
	if !ok {
		return ""
	}
	u := s.base() + spec.view
	if params.Encode() != "" {
		u += "?" + params.Encode()
	}
	return u
}

// Fetch 執行二步式：GET view 頁 → POST 下載 CSV。回傳之 RawResponse 為
// 下載 CSV（Big5/MS950）。req.URL 為 view URL（含 query 參數）。
// 下載 POST 目標由 view URL 推導（同目錄之 down 端點，相容測試覆寫）。
func (s *TAIFEXDLSource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	ds, start, end, contract, err := parseDLParams(req.URL)
	if err != nil {
		return nil, err
	}
	spec, ok := taifexDLSpecs[ds]
	if !ok {
		return nil, fmt.Errorf("provider: DL 不支援資料集 %q", ds)
	}

	// 第一步：GET view 頁（session cookie 維持；失敗不阻斷）
	viewURL := req.URL
	if _, err := s.client.Do(ctx, RawRequest{URL: viewURL}); err != nil {
		s.client.logger.Debug("TAIFEX-DL view GET 失敗，仍嘗試下載", "err", err)
	}

	// 第二步：POST 下載（目標為 view 同目錄之 down 端點）
	form := s.buildForm(spec, start, end, contract)
	body := form.Encode()
	headers := http.Header{}
	headers.Set("Content-Type", "application/x-www-form-urlencoded")
	headers.Set("Referer", viewURL)
	downURL := strings.TrimSuffix(viewURL, spec.view) + spec.down
	return s.client.Do(ctx, RawRequest{
		Method:  http.MethodPost,
		URL:     downURL,
		Headers: headers,
		Body:    []byte(body),
	})
}

// buildForm 依表單欄位順序建立 POST 參數（欄位順序與官方表單一致）。
func (s *TAIFEXDLSource) buildForm(spec taifexDLSpec, start, end, contract string) url.Values {
	v := url.Values{}
	for k, def := range spec.def {
		v.Set(k, def)
	}
	if start != "" {
		v.Set("queryStartDate", dlDateParam(start))
	}
	if end != "" {
		v.Set("queryEndDate", dlDateParam(end))
	}
	if contract != "" && containsStr(spec.fields, "commodity_id") {
		v.Set("commodity_id", contract)
	}
	return v
}

// containsStr 檢查字串是否在切片中。
func containsStr(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// dlDateParam 將 YYYY-MM-DD 轉為官方表單格式 YYYY/MM/DD。
func dlDateParam(date string) string {
	return strings.ReplaceAll(date, "-", "/")
}

// parseDLParams 解析 view URL 之 query 參數。
func parseDLParams(sourceURL string) (ds model.TAIFEXDataset, start, end, contract string, err error) {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return "", "", "", "", fmt.Errorf("provider: 無法解析 DL URL %q: %w", sourceURL, err)
	}
	// 依 view path 判斷資料集
	for d, spec := range taifexDLSpecs {
		if strings.HasSuffix(u.Path, spec.view) {
			ds = d
			break
		}
	}
	if ds == "" {
		return "", "", "", "", fmt.Errorf("provider: 未知 DL view 路徑 %q", u.Path)
	}
	q := u.Query()
	start = strings.TrimSpace(q.Get("queryStartDate"))
	end = strings.TrimSpace(q.Get("queryEndDate"))
	contract = strings.TrimSpace(q.Get("commodity_id"))
	return ds, start, end, contract, nil
}

func (s *TAIFEXDLSource) Validate(raw *RawResponse) error {
	return validateTAIFEXDL(raw)
}

func (s *TAIFEXDLSource) Normalize(raw *RawResponse) ([]byte, error) {
	return normalizeTAIFEXDL(raw)
}

// ---------------------------------------------------------------------------
// Validate：CSV 表頭與欄位數檢查

// dlHeaders 各資料集之表頭欄位（以 fixtures 逐一驗證，§9.2 備註）。
var dlHeaders = map[model.TAIFEXDataset][]string{
	model.TAFuturesDaily: {
		"交易日期", "契約", "到期月份(週別)", "開盤價", "最高價", "最低價", "收盤價",
		"漲跌價", "漲跌%", "成交量", "結算價", "未沖銷契約數", "最後最佳買價",
		"最後最佳賣價", "歷史最高價", "歷史最低價", "是否因訊息面暫停交易", "交易時段",
		"價差對單式委託成交量",
	},
	model.TAOptionsDaily: {
		"交易日期", "契約", "到期月份(週別)", "履約價", "買賣權", "開盤價", "最高價",
		"最低價", "收盤價", "成交量", "結算價", "未沖銷契約數", "最後最佳買價",
		"最後最佳賣價", "歷史最高價", "歷史最低價", "是否因訊息面暫停交易", "交易時段",
		"漲跌價", "漲跌%", "契約到期日",
	},
	model.TAInstiFutures: {
		"日期", "商品名稱", "身份別", "多方交易口數", "多方交易契約金額(千元)",
		"空方交易口數", "空方交易契約金額(千元)", "多空交易口數淨額",
		"多空交易契約金額淨額(千元)", "多方未平倉口數", "多方未平倉契約金額(千元)",
		"空方未平倉口數", "空方未平倉契約金額(千元)", "多空未平倉口數淨額",
		"多空未平倉契約金額淨額(千元)",
	},
	model.TALargeTraderFut: {
		"日期", "商品(契約)", "商品名稱(契約名稱)", "到期月份(週別)", "交易人類別",
		"前五大交易人買方", "前五大交易人賣方", "前十大交易人買方", "前十大交易人賣方",
		"全市場未沖銷部位數",
	},
	model.TALargeTraderOpt: {
		"日期", "商品(契約)", "商品名稱(契約名稱)", "買賣權", "到期月份(週別)",
		"交易人類別", "前五大交易人買方", "前五大交易人賣方", "前十大交易人買方",
		"前十大交易人賣方", "全市場未沖銷部位數",
	},
	model.TAPutCallRatio: {
		"日期", "賣權成交量", "買權成交量", "買賣權成交量比率%", "賣權未平倉量",
		"買權未平倉量", "買賣權未平倉量比率%",
	},
}

func validateTAIFEXDL(raw *RawResponse) error {
	if raw.StatusCode != 200 {
		return fmt.Errorf("provider: TAIFEX-DL 非預期 HTTP 狀態 %d", raw.StatusCode)
	}
	if len(raw.Body) == 0 {
		return fmt.Errorf("provider: TAIFEX-DL 空 body")
	}
	ds, err := taifexAPIDatasetOfDL(raw)
	if err != nil {
		return err
	}
	text, err := decodeUTF8OrBig5(raw.Body)
	if err != nil {
		return err
	}
	r := csv.NewReader(bytes.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true // 大額交易人 CSV 之備註列含未跳脫引號（如「"0"表…」）
	records, err := r.ReadAll()
	if err != nil {
		return fmt.Errorf("provider: %s CSV 解析失敗: %w", ds, err)
	}
	if len(records) == 0 {
		return fmt.Errorf("provider: %s CSV 無表頭", ds)
	}
	want := dlHeaders[ds]
	got := len(records[0])
	if len(want) > got {
		return fmt.Errorf("provider: %s CSV 表頭欄位數不符（期望 %d，實際 %d）：%v",
			ds, len(want), got, records[0])
	}
	for i, h := range want {
		if strings.TrimSpace(records[0][i]) != h {
			return fmt.Errorf("provider: %s CSV 表頭第 %d 欄不符（期望 %q，實際 %q）",
				ds, i+1, h, strings.TrimSpace(records[0][i]))
		}
	}
	return nil
}

// taifexAPIDatasetOfDL 依 SourceURL 判斷 DL 資料集（down path 或 view path 皆可）。
func taifexAPIDatasetOfDL(raw *RawResponse) (model.TAIFEXDataset, error) {
	u, err := url.Parse(raw.SourceURL)
	if err != nil {
		return "", fmt.Errorf("provider: 無法解析來源 URL %q: %w", raw.SourceURL, err)
	}
	for ds, spec := range taifexDLSpecs {
		if strings.HasSuffix(u.Path, spec.down) || strings.HasSuffix(u.Path, spec.view) {
			return ds, nil
		}
	}
	return "", fmt.Errorf("provider: 未知 TAIFEX-DL 路徑 %q", u.Path)
}

// ---------------------------------------------------------------------------
// Normalize：依資料集解析 CSV → Normalized Model（JSON）

func normalizeTAIFEXDL(raw *RawResponse) ([]byte, error) {
	ds, err := taifexAPIDatasetOfDL(raw)
	if err != nil {
		return nil, err
	}
	text, err := decodeUTF8OrBig5(raw.Body)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(text))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true // 大額交易人 CSV 之備註列含未跳脫引號（如「"0"表…」）
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("provider: %s CSV 解析失敗: %w", ds, err)
	}
	if len(records) < 2 {
		// 僅表頭（週六/無交易日）→ 空陣列
		return []byte("[]"), nil
	}
	// 依表頭建立欄位索引（欄位對齊以表頭名稱為準，§9.2 備註）
	idx := map[string]int{}
	for i, h := range records[0] {
		idx[strings.TrimSpace(h)] = i
	}
	col := func(rec []string, name string) string {
		if i, ok := idx[name]; ok && i < len(rec) {
			return strings.TrimSpace(rec[i])
		}
		return ""
	}

	var out interface{}
	switch ds {
	case model.TAFuturesDaily:
		out = normalizeDLFuturesDaily(records[1:], col)
	case model.TAOptionsDaily:
		out = normalizeDLOptionsDaily(records[1:], col)
	case model.TAInstiFutures:
		out = normalizeDLInstitutional(records[1:], col)
	case model.TALargeTraderFut, model.TALargeTraderOpt:
		out = normalizeDLLargeTrader(records[1:], col)
	case model.TAPutCallRatio:
		out = normalizeDLPCRatio(records[1:], col)
	default:
		return nil, fmt.Errorf("provider: 不支援資料集 %q", ds)
	}
	return json.Marshal(out)
}

// dlDate 解析 DL 日期（西元年："2026/07/29" 或 "20260729"）→ YYYY-MM-DD。
// 注意：TAIFEX DL 使用西元 4 碼年（不同於 TWSE 之民國年）。
func dlDate(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if t == "" {
		return "", false
	}
	var y, m, d int
	switch {
	case strings.Contains(t, "/"):
		if _, err := fmt.Sscanf(t, "%d/%d/%d", &y, &m, &d); err != nil {
			return "", false
		}
	case len(t) == 8:
		if _, err := fmt.Sscanf(t, "%4d%2d%2d", &y, &m, &d); err != nil {
			return "", false
		}
	default:
		return "", false
	}
	if y < 1990 || m < 1 || m > 12 || d < 1 || d > 31 {
		return "", false
	}
	return model.FormatDate(time.Date(y, time.Month(m), d, 0, 0, 0, 0, model.Taipei())), true
}

// dlFloat 解析 DL 數值（千分位、"-"、% 後綴）。
func dlFloat(s string) float64 {
	t := strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return commaFloatOrZero(t)
}

// dlInt 解析 DL 整數（千分位、"-"）。
func dlInt(s string) int64 {
	return commaIntOrZero(s)
}

func normalizeDLFuturesDaily(recs [][]string, col func([]string, string) string) []model.FuturesDailyRow {
	out := make([]model.FuturesDailyRow, 0, len(recs))
	for _, rec := range recs {
		d, ok := dlDate(col(rec, "交易日期"))
		if !ok {
			continue
		}
		out = append(out, model.FuturesDailyRow{
			Date:          d,
			Contract:      col(rec, "契約"),
			ContractMonth: col(rec, "到期月份(週別)"),
			Session:       col(rec, "交易時段"),
			Open:          dlFloat(col(rec, "開盤價")),
			High:          dlFloat(col(rec, "最高價")),
			Low:           dlFloat(col(rec, "最低價")),
			Close:         dlFloat(col(rec, "收盤價")),
			Change:        dlFloat(col(rec, "漲跌價")),
			ChangePct:     dlFloat(col(rec, "漲跌%")),
			Volume:        dlInt(col(rec, "成交量")),
			Settlement:    dlFloat(col(rec, "結算價")),
			OpenInterest:  dlInt(col(rec, "未沖銷契約數")),
			BestBid:       dlFloat(col(rec, "最後最佳買價")),
			BestAsk:       dlFloat(col(rec, "最後最佳賣價")),
			TradingHalt:   col(rec, "是否因訊息面暫停交易") != "",
		})
	}
	return out
}

func normalizeDLOptionsDaily(recs [][]string, col func([]string, string) string) []model.OptionsDailyRow {
	out := make([]model.OptionsDailyRow, 0, len(recs))
	for _, rec := range recs {
		d, ok := dlDate(col(rec, "交易日期"))
		if !ok {
			continue
		}
		expiry, _ := dlDate(col(rec, "契約到期日"))
		out = append(out, model.OptionsDailyRow{
			Date:          d,
			Contract:      col(rec, "契約"),
			ContractMonth: col(rec, "到期月份(週別)"),
			Strike:        dlFloat(col(rec, "履約價")),
			CallPut:       col(rec, "買賣權"),
			Session:       col(rec, "交易時段"),
			Open:          dlFloat(col(rec, "開盤價")),
			High:          dlFloat(col(rec, "最高價")),
			Low:           dlFloat(col(rec, "最低價")),
			Close:         dlFloat(col(rec, "收盤價")),
			Volume:        dlInt(col(rec, "成交量")),
			Settlement:    dlFloat(col(rec, "結算價")),
			OpenInterest:  dlInt(col(rec, "未沖銷契約數")),
			BestBid:       dlFloat(col(rec, "最後最佳買價")),
			BestAsk:       dlFloat(col(rec, "最後最佳賣價")),
			Change:        dlFloat(col(rec, "漲跌價")),
			ChangePct:     dlFloat(col(rec, "漲跌%")),
			ExpiryDate:    expiry,
		})
	}
	return out
}

func normalizeDLInstitutional(recs [][]string, col func([]string, string) string) []model.InstitutionalRow {
	out := make([]model.InstitutionalRow, 0, len(recs))
	for _, rec := range recs {
		d, ok := dlDate(col(rec, "日期"))
		if !ok {
			continue
		}
		out = append(out, model.InstitutionalRow{
			Date:         d,
			Contract:     col(rec, "商品名稱"),
			Investor:     col(rec, "身份別"),
			LongVolume:   dlInt(col(rec, "多方交易口數")),
			LongValue:    model.ThousandToYuan(dlInt(col(rec, "多方交易契約金額(千元)"))),
			ShortVolume:  dlInt(col(rec, "空方交易口數")),
			ShortValue:   model.ThousandToYuan(dlInt(col(rec, "空方交易契約金額(千元)"))),
			NetVolume:    dlInt(col(rec, "多空交易口數淨額")),
			NetValue:     model.ThousandToYuan(dlInt(col(rec, "多空交易契約金額淨額(千元)"))),
			OILong:       dlInt(col(rec, "多方未平倉口數")),
			OILongValue:  model.ThousandToYuan(dlInt(col(rec, "多方未平倉契約金額(千元)"))),
			OIShort:      dlInt(col(rec, "空方未平倉口數")),
			OIShortValue: model.ThousandToYuan(dlInt(col(rec, "空方未平倉契約金額(千元)"))),
			OINet:        dlInt(col(rec, "多空未平倉口數淨額")),
			OINetValue:   model.ThousandToYuan(dlInt(col(rec, "多空未平倉契約金額淨額(千元)"))),
		})
	}
	return out
}

func normalizeDLLargeTrader(recs [][]string, col func([]string, string) string) []model.LargeTraderRow {
	out := make([]model.LargeTraderRow, 0, len(recs))
	for _, rec := range recs {
		d, ok := dlDate(col(rec, "日期"))
		if !ok {
			continue
		}
		out = append(out, model.LargeTraderRow{
			Date:          d,
			Contract:      col(rec, "商品(契約)"),
			ContractName:  col(rec, "商品名稱(契約名稱)"),
			ContractMonth: col(rec, "到期月份(週別)"),
			CallPut:       col(rec, "買賣權"),
			TraderType:    col(rec, "交易人類別"),
			Top5Long:      dlInt(col(rec, "前五大交易人買方")),
			Top5Short:     dlInt(col(rec, "前五大交易人賣方")),
			Top10Long:     dlInt(col(rec, "前十大交易人買方")),
			Top10Short:    dlInt(col(rec, "前十大交易人賣方")),
			MarketOI:      dlInt(col(rec, "全市場未沖銷部位數")),
		})
	}
	return out
}

func normalizeDLPCRatio(recs [][]string, col func([]string, string) string) []model.PCRow {
	out := make([]model.PCRow, 0, len(recs))
	for _, rec := range recs {
		d, ok := dlDate(col(rec, "日期"))
		if !ok {
			continue
		}
		out = append(out, model.PCRow{
			Date:        d,
			CallVolume:  dlInt(col(rec, "買權成交量")),
			PutVolume:   dlInt(col(rec, "賣權成交量")),
			VolumeRatio: dlFloat(col(rec, "買賣權成交量比率%")),
			CallOI:      dlInt(col(rec, "買權未平倉量")),
			PutOI:       dlInt(col(rec, "賣權未平倉量")),
			OIRatio:     dlFloat(col(rec, "買賣權未平倉量比率%")),
		})
	}
	return out
}
