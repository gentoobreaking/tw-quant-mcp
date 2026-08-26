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
	"strconv"
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
	model.TAFuturesDaily:     "/DailyMarketReportFut",
	model.TAOptionsDaily:     "/DailyMarketReportOpt",
	model.TAInstiFutures:     "/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheDate",
	model.TAInstiOptions:     "/MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheDate",
	model.TALargeTraderFut:   "/OpenInterestOfLargeTradersFutures",
	model.TALargeTraderOpt:   "/OpenInterestOfLargeTradersOptions",
	model.TAPutCallRatio:     "/PutCallRatio",
	model.TAMargin:           "/IndexFuturesAndOptionsMargining",
	model.TAFAnnualVolume:    "/AnnualTradingVolume",
	model.TAFMonthlyStats:    "/MonthlyTradingStatisticsFutures",                                          // T148
	model.TAInstiDivided:     "/MarketDataOfMajorInstitutionalTradersDividedByFuturesAndOptionsBytheDate", // T126
	model.TAInstiGeneral:     "/MarketDataOfMajorInstitutionalTradersGeneralBytheDate",                    // T129
	model.TAInstiCallsPuts:   "/MarketDataOfMajorInstitutionalTradersDetailsOfCallsAndPutsBytheDate",      // T134
	model.TAOptionsDelta:     "/DailyOptionsDelta",                                                        // 選擇權每日 Delta（T151）
	model.TAOIChange:         "/va01",                                                                     // 台指選擇權未平倉量增減（T154）
	model.TAStockMargin:      "/SingleStockFuturesMargining",                                              // 股票期貨保證金（T167）
	model.TATickFutures:      "/TimeAndSalesData",                                                         // 期貨逐筆成交（T207）
	model.TATickOptions:      "/OptionsTimeAndSalesData",                                                  // 選擇權逐筆成交（T207）
	model.TAInstiGenWeek:     "/MarketDataOfMajorInstitutionalTradersGeneralBytheWeek",                    // 總表-依週別（T204）
	model.TAInstiDivWeek:     "/MarketDataOfMajorInstitutionalTradersDividedByFuturesAndOptionsBytheWeek", // 區分期貨與選擇權-依週別（T204）
	model.TAInstiFutContWeek: "/MarketDataOfMajorInstitutionalTradersDetailsOfFuturesContractsBytheWeek",  // 各期貨契約-依週別（T204）
	model.TAInstiOptContWeek: "/MarketDataOfMajorInstitutionalTradersDetailsOfOptionsContractsBytheWeek",  // 各選擇權契約-依週別（T204）
	model.TAInstiCPWeek:      "/MarketDataOfMajorInstitutionalTradersDetailsOfCallsAndPutsBytheWeek",      // 買賣權分計-依週別（T204）
	model.TAFSPAll:           "/FinalSettlementPrice",                                                     // 最後結算價-全部（T205）
	model.TAFSPFutures:       "/FinalSettlementPriceFutures",                                              // 最後結算價-期貨（T205）
	model.TAFSPIdxFut:        "/FinalSettlementPriceIndexFutures",                                         // 最後結算價-指數期貨（T205）
	model.TAFSPSSf:           "/FinalSettlementPriceSSF",                                                  // 最後結算價-股票期貨（T205）
	model.TAFSPIdxOpt:        "/FinalSettlementPriceIndexOptions",                                         // 最後結算價-指數選擇權（T205）
	model.TAFSPFx:            "/FinalSettlementPriceFx",                                                   // 最後結算價-匯率類（T205）
	model.TAFSPGold:          "/FinalSettlementPriceGold",                                                 // 最後結算價-商品類（T205）
	model.TAFSPIR:            "/FinalSettlementPriceIR",                                                   // 最後結算價-利率類（T205）
	model.TASPOptions:        "/FinalSettlementPriceOptions",                                              // 最後結算價-選擇權（T205）
	model.TAFSPSSO:           "/FinalSettlementPriceSSO",                                                  // 最後結算價-股票選擇權（T205）
	model.TASPAll:            "/SettledPositionsOfContractsOnExpirationDate",                              // 到期履約交割-全部（T206）
	model.TASFutures:         "/SettledPositionsFutures",                                                  // 到期履約交割-期貨商品（T206）
	model.TASPIdxOpt:         "/SettledPositionsIndexOptions",                                             // 到期履約交割-指數選擇權（T206）
	model.TASPFx:             "/SettledPositionsFX",                                                       // 到期履約交割-匯率選擇權（T206）
	model.TASPFxFut:          "/SettledPositionsFXFutures",                                                // 到期履約交割-匯率期貨（T206）
	model.TASPGold:           "/SettledPositionsGold",                                                     // 到期履約交割-商品類（T206）
	model.TASPIR:             "/SettledPositionsIR",                                                       // 到期履約交割-利率類（T206）
	model.TASPIdxFut:         "/SettledPositionsIndexFutures",                                             // 到期履約交割-指數期貨（T206）
	model.TASPOpt:            "/SettledPositionsOptions",                                                  // 到期履約交割-選擇權商品（T206）
	model.TASPSSF:            "/SettledPositionsSSF",                                                      // 到期履約交割-股票期貨（T206）
	model.TASPSSO:            "/SettledPositionsSSO",                                                      // 到期履約交割-股票選擇權（T206）
	model.TABlockTrade:       "/BlockTrade",                                                               // 鉅額交易各商品成交資訊（T208）
	model.TABTFutInfo:        "/BTDailyTradeInformationFutures",                                           // 鉅額交易成交資訊-期貨（T208）
	model.TABTOptInfo:        "/BTDailyTradeInformationOptions",                                           // 鉅額交易成交資訊-選擇權（T208）
	model.TABTFutSummary:     "/DailySummaryOfBlockTradeFutures",                                          // 鉅額交易成交量統計-期貨（T208）
	model.TABTOptSummary:     "/DailySummaryOfBlockTradeOptions",                                          // 鉅額交易成交量統計-選擇權（T208）
	model.TAMarginFx:         "/FXFuturesAndOptionsMargining",                                             // 保證金一覽表-匯率類（T209）
	model.TAMarginIR:         "/InterestRateFuturesMargining",                                             // 保證金一覽表-利率類（T209）
	model.TAMarginGold:       "/GoldFuturesAndOptionsMargining",                                           // 保證金一覽表-商品類（T209）
	model.TAMarginETF:        "/SingleStockFuturesETFMargining",                                           // 保證金一覽表-股票類 ETF（T209）
	model.TAFCMLists:         "/FCMLists",                                                                 // 期貨商總公司名冊（T230）
	model.TAFCMBranchLists:   "/FCMBranchLists",                                                           // 期貨商分公司名冊（T230）
	model.TAFCMNetValue:      "/NetValuePerShareStatement",                                                // 期貨商每股淨值明細表（T230）
	model.TAFCMIncome:        "/IncomeStatementF",                                                         // 專營期貨商稅前累計損益彙總表（T230）
	model.TAFCMAccIncome:     "/AccumulatedIncomeStateF",                                                  // 專營期貨商累計損益明細表（T230）
	model.TAPosLimitEquity:   "/PositionLimitEquity",                                                      // 部位限制-個股類（T231）
	model.TAPosLimitNonEq:    "/PositionLimitNonEquity",                                                   // 部位限制-非個股類（T231）
	model.TAContractAdj:      "/ContractAdj",                                                              // 契約調整一覽事項（T231）
	model.TASSFAdjustedInfo:  "/SSFAdjustedInfo",                                                          // 調整型契約資訊（T231）
	model.TAFeeSchedule:      "/FuturesAndOptionsFeeSchedule",                                             // 期貨及選擇權收費標準表（T231）
	model.TACollStock:        "/AcceptableCollateralStock",                                                // 可抵繳標的-股票含ETF（T232）
	model.TACollGovBond:      "/AcceptableCollateralGovernmentBonds",                                      // 可抵繳標的-公債（T232）
	model.TACollIntlBond:     "/AcceptableCollateralInternationalBonds",                                   // 可抵繳標的-國際債（T232）
	model.TACollLogStock:     "/AcceptableCollateralLogStock",                                             // 可抵繳標的增刪紀錄（T232）
	model.TAFxRates:          "/DailyForeignExchangeRates",                                                // 每日外幣參考匯率（T233）
	model.TAETradeQty:        "/eTradeQty",                                                                // 每月電子式交易下單統計（T233）
	model.TAStockOptOID:      "/va02",                                                                     // 每日個股選擇權未平倉量增減（T210）
	model.TAStockFutStatsD:   "/va12",                                                                     // 每日個股期貨交易量統計（T210）
	model.TAStockFutStatsM:   "/va13",                                                                     // 每月個股期貨交易量統計（T210）
	model.TAStockFutStatsY:   "/va14",                                                                     // 每年個股期貨交易量統計（T210）
	model.TASSFLists:         "/SSFLists",                                                                 // 股票期貨交易標的（T211）
	model.TASTFTop10:         "/STFTop10",                                                                 // 每日股票期貨量前十大（T211）
	model.TASSOLists:         "/SSOLists",                                                                 // 股票選擇權交易標的（T211）
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

// Deprecated: v2.1 §6 起轉換集中於 pkg/model/normalize（FromTAIFEXOpenAPI）；
// 本方法為 v1.3 相容層，遷移時逐步移除（T022）。
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
	if ds == model.TAInstiGeneral {
		// 三大法人整體交易總表：不帶參數回 CSV（UTF-8 含 BOM）、帶 query 參數
		// 回 JSON 陣列（T129 實測），兩者皆接受。
		b := string(raw.Body)
		if !strings.HasPrefix(b, "\ufeff日期") && !strings.HasPrefix(b, "日期") && !isJSONArray(raw.Body) {
			return fmt.Errorf("provider: %s 回應非官方 CSV/JSON（格式變更？）", ds)
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
	case model.TAFAnnualVolume:
		out, err = normalizeTAIAnnualVolume(raw.Body, contract)
	case model.TAFMonthlyStats:
		out, err = normalizeTAIMonthlyTraderStats(raw.Body)
	case model.TAInstiDivided:
		// 期貨與選擇權合計每日交易資訊：欄位繁多（多空量/金額/OI/契約價值），
		// 直通保留官方英文鍵名（T126）。
		out = json.RawMessage(raw.Body)
	case model.TAInstiGeneral:
		out, err = normalizeTAIInstiGeneral(raw.Body, date)
	case model.TAInstiCallsPuts:
		// 買賣權分計明細：直通保留官方欄位（含 CallPut，T134）。
		out = json.RawMessage(raw.Body)
	case model.TAOptionsDelta, model.TAOIChange, model.TAStockMargin:
		// Delta / OI 增減 / 股票期貨保證金：直通保留官方欄位（T151/T154/T167）。
		out = json.RawMessage(raw.Body)
	case model.TATickFutures, model.TATickOptions:
		// 期貨/選擇權逐筆成交：直通保留官方欄位（T207；Date/ProductCode/
		// TimeOfTrades/TradePrice/Volume 等）。
		out = json.RawMessage(raw.Body)
	case model.TAInstiGenWeek, model.TAInstiDivWeek, model.TAInstiFutContWeek,
		model.TAInstiOptContWeek, model.TAInstiCPWeek:
		// 三大法人依週別系列：直通保留官方欄位（T204；FromDate/ToDate/
		// Item/ContractCode/CallPut/TradingVolume(Long) 等）。週別資料官方
		// 不接受日期過濾（恆回近期各週），date 僅供工具層參考。
		out = json.RawMessage(raw.Body)
	case model.TAFSPAll, model.TAFSPFutures, model.TAFSPIdxFut, model.TAFSPSSf,
		model.TAFSPIdxOpt, model.TAFSPFx, model.TAFSPGold, model.TAFSPIR,
		model.TASPOptions, model.TAFSPSSO:
		// 最後結算價系列：直通保留官方欄位（T205；TheFinalSettlementDay/
		// Contract/ContractName/ContractDeliveryMonth/TheFinalSettlementPrice）。
		out = json.RawMessage(raw.Body)
	case model.TASPAll, model.TASFutures, model.TASPIdxOpt, model.TASPFx,
		model.TASPFxFut, model.TASPGold, model.TASPIR, model.TASPIdxFut,
		model.TASPOpt, model.TASPSSF, model.TASPSSO:
		// 到期契約履約交割系列：直通保留官方欄位（T206；TheFinalSettlementDay/
		// Contract/ContractName/Long/Short/CallPut 等）。
		out = json.RawMessage(raw.Body)
	case model.TABlockTrade, model.TABTFutInfo, model.TABTOptInfo,
		model.TABTFutSummary, model.TABTOptSummary:
		// 鉅額交易系列：直通保留官方欄位（T208；Date/Contract/
		// ContractMonth(Week)/StrikePrice/CallPut/Volume/MarketShare% 等）。
		out = json.RawMessage(raw.Body)
	case model.TAMarginFx, model.TAMarginIR, model.TAMarginGold, model.TAMarginETF:
		// 保證金一覽表四類別：直通保留官方欄位（T209；Contracts 或 Contract/
		// ClearingMargin/MaintenanceMargin/InitialMargin 等）。
		out = json.RawMessage(raw.Body)
	case model.TAFCMLists, model.TAFCMBranchLists, model.TAFCMNetValue,
		model.TAFCMIncome, model.TAFCMAccIncome:
		// 期貨商名冊/淨值/損益：直通保留官方欄位（T230；FCMCode/FCMName/
		// NetValuePerShare/NetIncomeBeforeTaxThisMonth 等）。
		out = json.RawMessage(raw.Body)
	case model.TAPosLimitEquity, model.TAPosLimitNonEq, model.TAContractAdj,
		model.TASSFAdjustedInfo, model.TAFeeSchedule:
		// 部位限制/契約調整/收費標準：直通保留官方欄位（T231；Contract/
		// Tier/StockCode/CashDividend(NTD/share)/TransactionFee 等）。
		out = json.RawMessage(raw.Body)
	case model.TACollStock, model.TACollGovBond, model.TACollIntlBond,
		model.TACollLogStock:
		// 可抵繳標的：直通保留官方欄位（T232；Date/StockId/Code/
		// InternationalBondCode/New/Delete 等）。
		out = json.RawMessage(raw.Body)
	case model.TAFxRates, model.TAETradeQty:
		// 匯率/電子交易統計：直通保留官方欄位（T233；Date/USD-NTD 或
		// YYYYMM/Volume 等，原樣保留）。
		out = json.RawMessage(raw.Body)
	case model.TAStockOptOID, model.TAStockFutStatsD, model.TAStockFutStatsM,
		model.TAStockFutStatsY:
		// 個股期貨/選擇權交易統計 va 系列：直通保留官方欄位（T210；
		// Date/YYYYMM/YYYY/ContractType/OpenInterest/Volume/Change 等）。
		out = json.RawMessage(raw.Body)
	case model.TASSFLists, model.TASTFTop10, model.TASSOLists:
		// 股票期貨/選擇權標的與前十大：直通保留官方欄位（T211；Contract/
		// UnderlyingStock/StockCode/StockName/Type/Volume 等）。
		out = json.RawMessage(raw.Body)
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

// normalizeTAIInstiGeneral：三大法人整體交易總表 → InstiGeneralRow（T129）。
// 端點帶 query 參數時回 JSON 陣列（英文鍵）、不帶參數回 CSV（中文欄），兩者皆處理。
func normalizeTAIInstiGeneral(body []byte, date string) ([]model.InstiGeneralRow, error) {
	b := string(body)
	if strings.HasPrefix(b, "[") || strings.HasPrefix(b, "\ufeff[") {
		return normalizeTAIInstiGeneralJSON(body, date)
	}
	return normalizeTAIInstiGeneralCSV(body, date)
}

func normalizeTAIInstiGeneralJSON(body []byte, date string) ([]model.InstiGeneralRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: insti_general JSON 解析失敗: %w", err)
	}
	wantDate := strings.ReplaceAll(date, "-", "")
	out := make([]model.InstiGeneralRow, 0, len(rows))
	for _, raw := range rows {
		m := taifexAPIMap(raw)
		d, ok := taifexAPIDate(m["Date"])
		if !ok {
			continue
		}
		if wantDate != "" && strings.ReplaceAll(d, "-", "") != wantDate {
			continue
		}
		out = append(out, model.InstiGeneralRow{
			Date: d, Investor: m["Item"],
			LongVolume: taifexAPIInt(m["TradingVolume(Long)"]), LongValue: taifexAPIFloat(m["TradingValue(Long)(Millions)"]),
			ShortVolume: taifexAPIInt(m["TradingVolume(Short)"]), ShortValue: taifexAPIFloat(m["TradingValue(Short)(Millions)"]),
			NetVolume: taifexAPIInt(m["TradingVolume(Net)"]), NetValue: taifexAPIFloat(m["TradingValue(Net)(Millions)"]),
			OILong: taifexAPIInt(m["OpenInterest(Long)"]), OILongValue: taifexAPIFloat(m["ContractValueOfOpenInterest(Long)(Millions)"]),
			OIShort: taifexAPIInt(m["OpenInterest(Short)"]), OIShortValue: taifexAPIFloat(m["ContractValueOfOpenInterest(Short)(Millions)"]),
			OINet: taifexAPIInt(m["OpenInterest(Net)"]), OINetValue: taifexAPIFloat(m["ContractValueOfOpenInterest(Net)(Millions)"]),
		})
	}
	return out, nil
}

func normalizeTAIInstiGeneralCSV(body []byte, date string) ([]model.InstiGeneralRow, error) {
	text := strings.TrimPrefix(string(body), "\ufeff")
	r := csv.NewReader(strings.NewReader(text))
	r.FieldsPerRecord = -1
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("provider: insti_general CSV 解析失敗: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("provider: insti_general CSV 無資料列")
	}
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
	wantDate := strings.ReplaceAll(date, "-", "") // API date 參數為 YYYY-MM-DD；CSV 為 YYYYMMDD
	out := make([]model.InstiGeneralRow, 0, len(records)-1)
	for _, rec := range records[1:] {
		d, ok := taifexAPIDate(col(rec, "日期"))
		if !ok {
			continue
		}
		if wantDate != "" && strings.ReplaceAll(d, "-", "") != wantDate {
			continue
		}
		out = append(out, model.InstiGeneralRow{
			Date:         d,
			Investor:     col(rec, "身份別"),
			LongVolume:   taifexAPIInt(col(rec, "多方交易口數")),
			LongValue:    taifexAPIFloat(col(rec, "多方交易契約金額(百萬元)")),
			ShortVolume:  taifexAPIInt(col(rec, "空方交易口數")),
			ShortValue:   taifexAPIFloat(col(rec, "空方交易契約金額(百萬元)")),
			NetVolume:    taifexAPIInt(col(rec, "多空交易口數淨額")),
			NetValue:     taifexAPIFloat(col(rec, "多空交易契約金額淨額(百萬元)")),
			OILong:       taifexAPIInt(col(rec, "多方未平倉口數")),
			OILongValue:  taifexAPIFloat(col(rec, "多方未平倉契約金額(百萬元)")),
			OIShort:      taifexAPIInt(col(rec, "空方未平倉口數")),
			OIShortValue: taifexAPIFloat(col(rec, "空方未平倉契約金額(百萬元)")),
			OINet:        taifexAPIInt(col(rec, "多空未平倉口數淨額")),
			OINetValue:   taifexAPIFloat(col(rec, "多空未平倉契約金額淨額(百萬元)")),
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

// AnnualVolumeRow 為單一期貨商品之年成交量統計（T041）。
type AnnualVolumeRow struct {
	Year           string `json:"year"`
	Contract       string `json:"contract"`
	Name           string `json:"name"`
	Volume         int64  `json:"volume"`
	TradingDays    int64  `json:"trading_days"`
	AvgDailyVolume int64  `json:"avg_daily_volume"`
}

// normalizeTAIAnnualVolume：年成交量統計（AnnualTradingVolume，T041）。
func normalizeTAIAnnualVolume(body []byte, contract string) ([]AnnualVolumeRow, error) {
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: annual_volume JSON 解析失敗: %w", err)
	}
	out := make([]AnnualVolumeRow, 0, len(rows))
	for _, m := range rows {
		r := AnnualVolumeRow{
			Year:           strAny(m["YYYY"]),
			Contract:       strings.ToUpper(strAny(m["Contract"])),
			Name:           strAny(m["ContractName"]),
			Volume:         intAny(m["Volume"]),
			TradingDays:    intAny(m["NumberOfTradingDays"]),
			AvgDailyVolume: intAny(m["AvgDailyTradingVolume"]),
		}
		if r.Contract == "" {
			continue
		}
		if contract != "" && r.Contract != strings.ToUpper(contract) {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: annual_volume 無有效資料列")
	}
	return out, nil
}

// MonthlyTraderStatsRow 為期貨市場月統計之單一商品類別列（T148）。
// 官方欄位（2026-08-25 實測）：YYYYMM、ContactName、TotalVolume、
// Brokers-Individual(Buy/Sell)、Brokers-InstutionalInvestors-{SecuritiesDealers/
// SecuritiesInvestmentTrust/Foreign&MainlandAreaInstitutionalInvestors/
// ManagedFuturesEnterprisesAndFuturesTrustFunds/OtherInstitutionalInvesters}(Buy/Sell)、
// ProprietaryTraders(Buy/Sell)、MonthEndOpenInterest；數量單位：口。
type MonthlyTraderStatsRow struct {
	Month          string `json:"month"`                    // YYYYMM
	Category       string `json:"category"`                 // 商品類別（股價指數期貨、股票期貨…）
	TotalVolume    int64  `json:"total_volume"`             // 總成交量
	IndivBuy       int64  `json:"individual_buy"`           // 自然人買方
	IndivSell      int64  `json:"individual_sell"`          // 自然人賣方
	DealerBuy      int64  `json:"dealer_buy"`               // 自營商經紀業務買方
	DealerSell     int64  `json:"dealer_sell"`              // 自營商經紀業務賣方
	TrustBuy       int64  `json:"trust_buy"`                // 投信買方
	TrustSell      int64  `json:"trust_sell"`               // 投信賣方
	ForeignBuy     int64  `json:"foreign_buy"`              // 外資及陸資買方
	ForeignSell    int64  `json:"foreign_sell"`             // 外資及陸資賣方
	ManagedBuy     int64  `json:"managed_futures_buy"`      // 期貨信託/期經買方
	ManagedSell    int64  `json:"managed_futures_sell"`     // 期貨信託/期經賣方
	OtherInstiBuy  int64  `json:"other_institutional_buy"`  // 其他機構買方
	OtherInstiSell int64  `json:"other_institutional_sell"` // 其他機構賣方
	PropBuy        int64  `json:"proprietary_buy"`          // 自營商買方
	PropSell       int64  `json:"proprietary_sell"`         // 自營商賣方
	MonthEndOI     int64  `json:"month_end_open_interest"`  // 月底未平倉
}

// normalizeTAIMonthlyTraderStats：期貨各類交易人各商品交易量統計表
// （MonthlyTradingStatisticsFutures，T148）。
func normalizeTAIMonthlyTraderStats(body []byte) ([]MonthlyTraderStatsRow, error) {
	var rows []map[string]any
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, fmt.Errorf("provider: monthly_stats_futures JSON 解析失敗: %w", err)
	}
	out := make([]MonthlyTraderStatsRow, 0, len(rows))
	for _, m := range rows {
		r := MonthlyTraderStatsRow{
			Month:          strAny(m["YYYYMM"]),
			Category:       strAny(m["ContactName"]),
			TotalVolume:    intAny(m["TotalVolume"]),
			IndivBuy:       intAny(m["Brokers-Individual(Buy)"]),
			IndivSell:      intAny(m["Brokers-Individual(Sell)"]),
			DealerBuy:      intAny(m["Brokers-InstutionalInvestors-SecuritiesDealers(Buy)"]),
			DealerSell:     intAny(m["Brokers-InstutionalInvestors-SecuritiesDealers(Sell)"]),
			TrustBuy:       intAny(m["Brokers-InstutionalInvestors-SecuritiesInvestmentTrust(Buy)"]),
			TrustSell:      intAny(m["Brokers-InstutionalInvestors-SecuritiesInvestmentTrust(Sell)"]),
			ForeignBuy:     intAny(m["Brokers-InstutionalInvestors-Foreign&MainlandAreaInstitutionalInvestors(Buy)"]),
			ForeignSell:    intAny(m["Brokers-InstutionalInvestors-Foreign&MainlandAreaInstitutionalInvestors(Sell)"]),
			ManagedBuy:     intAny(m["Brokers-InstutionalInvestors-ManagedFuturesEnterprisesAndFuturesTrustFunds(Buy)"]),
			ManagedSell:    intAny(m["Brokers-InstutionalInvestors-ManagedFuturesEnterprisesAndFuturesTrustFunds(Sell)"]),
			OtherInstiBuy:  intAny(m["Brokers-InstutionalInvestors-OtherInstitutionalInvesters(Buy)"]),
			OtherInstiSell: intAny(m["Brokers-InstutionalInvestors-OtherInstitutionalInvesters(Sell)"]),
			PropBuy:        intAny(m["ProprietaryTraders(Buy)"]),
			PropSell:       intAny(m["ProprietaryTraders(Sell)"]),
			MonthEndOI:     intAny(m["MonthEndOpenInterest"]),
		}
		if r.Month == "" || r.Category == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: monthly_stats_futures 無有效資料列")
	}
	return out, nil
}

// strAny / intAny：寬鬆轉換 TAIFEX API 欄位。
func strAny(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}

func intAny(v any) int64 {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case string:
		n, _ := strconv.ParseInt(strings.ReplaceAll(x, ",", ""), 10, 64)
		return n
	default:
		return 0
	}
}
