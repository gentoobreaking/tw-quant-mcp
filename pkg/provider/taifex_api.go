package provider

// TAIFEX-API Adapter（T013）：期交所 OpenAPI（openapi.taifex.com.tw，hot tier），
// 僅提供最新一個交易日，實作 SourceContract（§2.2）。
//
// 端點清單（2026-07-31 實測，swagger 於 /v1/swagger.json）：
//
//	DailyMarketReportFut    期貨每日行情
//	DailyMarketReportOpt    選擇權每日行情
//	MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate 三大法人期貨
//	MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheDate 三大法人選擇權
//	OpenInterestOfLargeTradersFutures  大額交易人期貨未沖銷部位（回應為 CSV 純文字！）
//	OpenInterestOfLargeTradersOptions  大額交易人選擇權未沖銷部位
//	PutCallRatio            買賣權比（回傳多日）
//	IndexFuturesAndOptionsMargining 保證金
//
// 端點特性（2026-07-31 實測）：
//   - 一律 GET，queryDate 參數無效（仍回全量最新交易日資料），故 URL 不帶日期；
//     date 僅作為 Normalize 過濾參數（配合 §9.3 查詢流程）。
//   - 回應全為 string：Date 為 YYYYMMDD 西元 8 碼、"-" 表無值、百分比帶 "%" 後綴。
//   - OpenInterestOfLargeTradersFutures 之 content-type 為 application/octet-stream，
//     body 為 UTF-8 含 BOM 之 CSV 純文字（欄位序同 DL largeTraderFutDown）。
//
// 單位換算（§5.1）：法人 TradingValue/ContractValueofOpenInterest 欄位為「千元」，
// ×1000 → 元；其餘成交量/未沖銷契約數即「口」不需換算。

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/transform"

	"tw-quant-mcp/pkg/model"
)

// TAIFEX-API 基底（swagger servers 定義；無 /v1 前綴會 302 回 SPA）。
const taifexAPIBase = "https://openapi.taifex.com.tw/v1"

// taifexAPIPaths 為資料集 → API 端點路徑對應。
var taifexAPIPaths = map[model.TAIFEXDataset]string{
	model.TAFuturesDaily:   "/DailyMarketReportFut",
	model.TAOptionsDaily:   "/DailyMarketReportOpt",
	model.TAInstiFutures:   "/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate",
	model.TAInstiOptions:   "/MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheDate",
	model.TALargeTraderFut: "/OpenInterestOfLargeTradersFutures",
	model.TALargeTraderOpt: "/OpenInterestOfLargeTradersOptions",
	model.TAPutCallRatio:   "/PutCallRatio",
	model.TAMargin:         "/IndexFuturesAndOptionsMargining",
}

// NewTAIFEXAPISource 建立 TAIFEX-API 來源（Rate Limit 1 req/s，§4.4）。
func NewTAIFEXAPISource(opts ...Option) *TAIFEXAPISource {
	return &TAIFEXAPISource{client: NewBaseClient("openapi.taifex.com.tw", opts...)}
}

// NewTAIFEXAPISourceWithBase 建立 TAIFEX-API 來源並指定基底 URL
// （測試/多環境覆寫用；base 應含 /v1 前綴）。
func NewTAIFEXAPISourceWithBase(base string) *TAIFEXAPISource {
	return &TAIFEXAPISource{
		client:  NewBaseClient("openapi.taifex.com.tw", WithRateInterval(time.Microsecond)),
		baseURL: base,
	}
}

// TAIFEXAPISource 實作 SourceContract（§2.2），ID = TAIFEX_API。
type TAIFEXAPISource struct {
	client  *BaseClient
	baseURL string // 測試覆寫用（httptest）；空 = 官方基底
}

var _ SourceContract = (*TAIFEXAPISource)(nil)

func (s *TAIFEXAPISource) ID() string { return model.SourceTAIFEXAPI }

// base 回傳基底 URL（測試覆寫優先）。
func (s *TAIFEXAPISource) base() string {
	if s.baseURL != "" {
		return s.baseURL
	}
	return taifexAPIBase
}

// URL 建立資料集之官方請求 URL。官方端點不接受 query 參數（恆回最新交易日
// 全量），date/contract 僅由 Normalize 使用做過濾。
func (s *TAIFEXAPISource) URL(ds model.TAIFEXDataset, params url.Values) string {
	u := s.base() + taifexAPIPaths[ds]
	if params.Encode() != "" {
		u += "?" + params.Encode()
	}
	return u
}

func (s *TAIFEXAPISource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	return s.client.Do(ctx, req)
}

func (s *TAIFEXAPISource) Validate(raw *RawResponse) error {
	return validateTAIFEXAPI(raw)
}

func (s *TAIFEXAPISource) Normalize(raw *RawResponse) ([]byte, error) {
	return normalizeTAIFEXAPI(raw)
}

// taifexAPIDatasetOf 依 SourceURL 之 path 判斷資料集。
func taifexAPIDatasetOf(raw *RawResponse) (model.TAIFEXDataset, error) {
	u, err := url.Parse(raw.SourceURL)
	if err != nil {
		return "", fmt.Errorf("provider: 無法解析來源 URL %q: %w", raw.SourceURL, err)
	}
	for ds, path := range taifexAPIPaths {
		if strings.HasSuffix(u.Path, path) {
			return ds, nil
		}
	}
	return "", fmt.Errorf("provider: 未知 TAIFEX-API 路徑 %q", u.Path)
}

// ---------------------------------------------------------------------------
// Validate：schema 檢查（欄位存在性、數值範圍、日期一致性，§2.2）

func validateTAIFEXAPI(raw *RawResponse) error {
	ds, err := taifexAPIDatasetOf(raw)
	if err != nil {
		return err
	}
	if len(raw.Body) == 0 {
		return fmt.Errorf("provider: %s 空 body", ds)
	}
	if ds == model.TALargeTraderFut {
		// 大額交易人期貨為 CSV 純文字（UTF-8 含 BOM）
		if !strings.HasPrefix(string(raw.Body), "\ufeff日期") {
			return fmt.Errorf("provider: %s 回應非官方 CSV（表頭變更？）", ds)
		}
		return nil
	}
	if !isJSONArray(raw.Body) {
		return fmt.Errorf("provider: %s 回應非 JSON 陣列（官方格式可能變更）", ds)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return fmt.Errorf("provider: %s 回應 JSON 解析失敗: %w", ds, err)
	}
	if len(rows) == 0 {
		return fmt.Errorf("provider: %s 回應為空陣列", ds)
	}
	return validateOpenAPIList(raw, rows)
}

// ---------------------------------------------------------------------------
// Normalize：依資料集將 raw 轉為 Normalized Model（JSON），單位 口/元/%

func normalizeTAIFEXAPI(raw *RawResponse) ([]byte, error) {
	ds, err := taifexAPIDatasetOf(raw)
	if err != nil {
		return nil, err
	}
	date := queryOf(raw.SourceURL, "date") // 過濾參數（YYYY-MM-DD，可空）
	contract := queryOf(raw.SourceURL, "contract")

	var out interface{}
	switch ds {
	case model.TAFuturesDaily:
		out, err = normalizeTAIFuturesDaily(raw.Body, date, contract)
	case model.TAOptionsDaily:
		out, err = normalizeTAIOptionsDaily(raw.Body, date, contract)
	case model.TAInstiFutures, model.TAInstiOptions:
		out, err = normalizeTAIInstitutional(raw.Body, date, contract)
	case model.TALargeTraderFut:
		out, err = normalizeTALargeTraderFuturesCSV(raw.Body, date, contract)
	case model.TALargeTraderOpt:
		out, err = normalizeTALargeTraderOptions(raw.Body, date, contract)
	case model.TAPutCallRatio:
		out, err = normalizeTAIPCRatio(raw.Body, date)
	case model.TAMargin:
		out, err = normalizeTAIMargin(raw.Body, date, contract)
	default:
		return nil, fmt.Errorf("provider: 不支援資料集 %q", ds)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// queryOf 取 URL query 之參數。
func queryOf(sourceURL, key string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

// taifexAPIDate 解析 API 之西元 8 碼日期（20260731）→ YYYY-MM-DD。
func taifexAPIDate(s string) (string, bool) {
	t := strings.TrimSpace(s)
	if len(t) != 8 {
		return "", false
	}
	var y, m, d int
	if _, err := fmt.Sscanf(t, "%4d%2d%2d", &y, &m, &d); err != nil {
		return "", false
	}
	if y < 1990 || m < 1 || m > 12 || d < 1 || d > 31 {
		return "", false
	}
	return model.FormatDate(time.Date(y, time.Month(m), d, 0, 0, 0, 0, model.Taipei())), true
}

// taifexAPIFloat 解析 API 數值字串："-"/"" → 0；去除 "%" 後綴與千分位。
func taifexAPIFloat(s string) float64 {
	t := strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return commaFloatOrZero(t)
}

// taifexAPIInt 解析 API 整數字串："-"→0，去除千分位。
func taifexAPIInt(s string) int64 {
	return commaIntOrZero(s)
}

// taifexAPIMap 為 API 回應單列 → 欄位 map（值全為 string）。
func taifexAPIMap(row map[string]json.RawMessage) map[string]string {
	return rowToMap(row)
}

// ---------------------------------------------------------------------------
// 期貨每日行情（DailyMarketReportFut）

func normalizeTAIFuturesDaily(body []byte, date, contract string) ([]model.FuturesDailyRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: futures_daily JSON 解析失敗: %w", err)
	}
	out := make([]model.FuturesDailyRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(m["Contract"]) != contract {
			continue
		}
		out = append(out, model.FuturesDailyRow{
			Date:          d,
			Contract:      strings.TrimSpace(m["Contract"]),
			ContractMonth: strings.TrimSpace(m["ContractMonth(Week)"]),
			Session:       strings.TrimSpace(m["TradingSession"]),
			Open:          taifexAPIFloat(m["Open"]),
			High:          taifexAPIFloat(m["High"]),
			Low:           taifexAPIFloat(m["Low"]),
			Close:         taifexAPIFloat(m["Last"]),
			Change:        taifexAPIFloat(m["Change"]),
			ChangePct:     taifexAPIFloat(m["%"]),
			Volume:        taifexAPIInt(m["Volume"]),
			Settlement:    taifexAPIFloat(m["SettlementPrice"]),
			OpenInterest:  taifexAPIInt(m["OpenInterest"]),
			BestBid:       taifexAPIFloat(m["BestBid"]),
			BestAsk:       taifexAPIFloat(m["BestAsk"]),
			TradingHalt:   strings.TrimSpace(m["TradingHalt"]) != "",
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 選擇權每日行情（DailyMarketReportOpt）

func normalizeTAIOptionsDaily(body []byte, date, contract string) ([]model.OptionsDailyRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: options_daily JSON 解析失敗: %w", err)
	}
	out := make([]model.OptionsDailyRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(m["Contract"]) != contract {
			continue
		}
		out = append(out, model.OptionsDailyRow{
			Date:          d,
			Contract:      strings.TrimSpace(m["Contract"]),
			ContractMonth: strings.TrimSpace(m["ContractMonth(Week)"]),
			Strike:        taifexAPIFloat(m["StrikePrice"]),
			CallPut:       strings.TrimSpace(m["CallPut"]),
			Session:       strings.TrimSpace(m["TradingSession"]),
			Open:          taifexAPIFloat(m["Open"]),
			High:          taifexAPIFloat(m["High"]),
			Low:           taifexAPIFloat(m["Low"]),
			Close:         taifexAPIFloat(m["Close"]),
			Volume:        taifexAPIInt(m["Volume"]),
			Settlement:    taifexAPIFloat(m["SettlementPrice"]),
			OpenInterest:  taifexAPIInt(m["OpenInterest"]),
			BestBid:       taifexAPIFloat(m["BestBid"]),
			BestAsk:       taifexAPIFloat(m["BestAsk"]),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 三大法人（期貨/選擇權）：金額欄位為千元，×1000 → 元

func normalizeTAIInstitutional(body []byte, date, contract string) ([]model.InstitutionalRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: institutional JSON 解析失敗: %w", err)
	}
	out := make([]model.InstitutionalRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(m["ContractCode"]) != contract {
			continue
		}
		out = append(out, model.InstitutionalRow{
			Date:         d,
			Contract:     strings.TrimSpace(m["ContractCode"]),
			Investor:     strings.TrimSpace(m["Item"]),
			LongVolume:   taifexAPIInt(m["TradingVolume(Long)"]),
			LongValue:    model.ThousandToYuan(taifexAPIInt(m["TradingValue(Long)(Thousands)"])),
			ShortVolume:  taifexAPIInt(m["TradingVolume(Short)"]),
			ShortValue:   model.ThousandToYuan(taifexAPIInt(m["TradingValue(Short)(Thousands)"])),
			NetVolume:    taifexAPIInt(m["TradingVolume(Net)"]),
			NetValue:     model.ThousandToYuan(taifexAPIInt(m["TradingValue(Net)(Thousands)"])),
			OILong:       taifexAPIInt(m["OpenInterest(Long)"]),
			OILongValue:  model.ThousandToYuan(taifexAPIInt(m["ContractValueofOpenInterest(Long)(Thousands)"])),
			OIShort:      taifexAPIInt(m["OpenInterest(Short)"]),
			OIShortValue: model.ThousandToYuan(taifexAPIInt(m["ContractValueofOpenInterest(Short)(Thousands)"])),
			OINet:        taifexAPIInt(m["OpenInterest(Net)"]),
			OINetValue:   model.ThousandToYuan(taifexAPIInt(m["ContractValueofOpenInterest(Net)(Thousands)"])),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 大額交易人期貨（CSV 純文字，UTF-8 含 BOM；欄位序同 DL largeTraderFutDown）

func normalizeTALargeTraderFuturesCSV(body []byte, date, contract string) ([]model.LargeTraderRow, error) {
	text, err := decodeUTF8OrBig5(body)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(bytes.NewReader(text))
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("provider: large_trader_fut CSV 解析失敗: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("provider: large_trader_fut CSV 無資料列")
	}
	out := make([]model.LargeTraderRow, 0, len(records)-1)
	for _, rec := range records[1:] {
		if len(rec) < 10 {
			continue
		}
		d, ok := taifexAPIDate(strings.TrimSpace(rec[0]))
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(rec[1]) != contract {
			continue
		}
		out = append(out, model.LargeTraderRow{
			Date:          d,
			Contract:      strings.TrimSpace(rec[1]),
			ContractName:  strings.TrimSpace(rec[2]),
			ContractMonth: strings.TrimSpace(rec[3]),
			TraderType:    strings.TrimSpace(rec[4]),
			Top5Long:      taifexAPIInt(rec[5]),
			Top5Short:     taifexAPIInt(rec[6]),
			Top10Long:     taifexAPIInt(rec[7]),
			Top10Short:    taifexAPIInt(rec[8]),
			MarketOI:      taifexAPIInt(rec[9]),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 大額交易人選擇權（JSON）

func normalizeTALargeTraderOptions(body []byte, date, contract string) ([]model.LargeTraderRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: large_trader_opt JSON 解析失敗: %w", err)
	}
	out := make([]model.LargeTraderRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(m["Contract"]) != contract {
			continue
		}
		out = append(out, model.LargeTraderRow{
			Date:          d,
			Contract:      strings.TrimSpace(m["Contract"]),
			ContractName:  strings.TrimSpace(m["ContractName"]),
			ContractMonth: strings.TrimSpace(m["SettlementMonth"]),
			CallPut:       strings.TrimSpace(m["CallPut"]),
			TraderType:    strings.TrimSpace(m["TypeOfTraders"]),
			Top5Long:      taifexAPIInt(m["Top5Buy"]),
			Top5Short:     taifexAPIInt(m["Top5Sell"]),
			Top10Long:     taifexAPIInt(m["Top10Buy"]),
			Top10Short:    taifexAPIInt(m["Top10Sell"]),
			MarketOI:      taifexAPIInt(m["OIOfMarket"]),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 買賣權比（PutCallRatio，回傳多日）

func normalizeTAIPCRatio(body []byte, date string) ([]model.PCRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: put_call_ratio JSON 解析失敗: %w", err)
	}
	out := make([]model.PCRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		out = append(out, model.PCRow{
			Date:        d,
			CallVolume:  taifexAPIInt(m["CallVolume"]),
			PutVolume:   taifexAPIInt(m["PutVolume"]),
			VolumeRatio: taifexAPIFloat(m["PutCallVolumeRatio%"]),
			CallOI:      taifexAPIInt(m["CallOI"]),
			PutOI:       taifexAPIInt(m["PutOI"]),
			OIRatio:     taifexAPIFloat(m["PutCallOIRatio%"]),
		})
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 保證金（IndexFuturesAndOptionsMargining，單位元）

func normalizeTAIMargin(body []byte, date, contract string) ([]model.MarginRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: margin JSON 解析失敗: %w", err)
	}
	out := make([]model.MarginRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if date != "" && d != date {
			continue
		}
		if contract != "" && strings.TrimSpace(m["Contract"]) != contract {
			continue
		}
		out = append(out, model.MarginRow{
			Date:              d,
			Contract:          strings.TrimSpace(m["Contract"]),
			ClearingMargin:    taifexAPIInt(m["ClearingMargin"]),
			MaintenanceMargin: taifexAPIInt(m["MaintenanceMargin"]),
			InitialMargin:     taifexAPIInt(m["InitialMargin"]),
		})
	}
	return out, nil
}

// utf8Valid 檢查 body 是否為合法 UTF-8（DL CSV 可能為 Big5/MS950）。
func utf8Valid(b []byte) bool {
	return utf8.Valid(b)
}

// decodeUTF8OrBig5 解碼官方文字資料：先試 UTF-8（含 BOM strip），
// 失敗（非 UTF-8）以 Big5 解碼（DL CSV 為 Big5/MS950）。
func decodeUTF8OrBig5(body []byte) ([]byte, error) {
	if len(body) == 0 {
		return nil, fmt.Errorf("provider: 空 body")
	}
	if strings.HasPrefix(string(body), "\ufeff") {
		body = bytes.TrimPrefix(body, []byte("\ufeff"))
	}
	if utf8Valid(body) {
		return body, nil
	}
	dec := traditionalchinese.Big5.NewDecoder()
	text, _, err := transform.Bytes(dec, body)
	if err != nil {
		return nil, fmt.Errorf("provider: Big5 解碼失敗: %w", err)
	}
	return text, nil
}
