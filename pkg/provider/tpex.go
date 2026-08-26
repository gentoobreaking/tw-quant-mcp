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
	TPExOtcDaily      TPExDataset = "otc_daily"             // 上櫃每日收盤行情（T155）
	TPExOtcMonthlyRev TPExDataset = "otc_monthly_revenue"   // 上櫃月營收彙總（T195）
	TPExBrokerVolume  TPExDataset = "otc_broker_volume"     // 上櫃熱門股券商進出排行（T196）
	TPExOtcForeignTrd TPExDataset = "otc_foreign_trading"   // 上櫃外資及陸資買賣超彙總（T197）
	TPExOtcExRightDay TPExDataset = "otc_exright_daily"     // 上櫃除權息計算結果（T200）
	TPExOtcDTTargets  TPExDataset = "otc_daytrade_targets"  // 上櫃當沖標的（T201）
	TPExOtcDTStats    TPExDataset = "otc_daytrade_stats"    // 上櫃當沖統計（T201）
	TPExOtcESG        TPExDataset = "otc_esg"               // 上櫃 ESG 揭露（T216，topic 1~21）
	TPExHDIndex       TPExDataset = "hd_index"              // 高殖利率指數歷史（T218）
	TPExHDLatest      TPExDataset = "hd_latest"             // 高殖利率指數當日（T218）
	TPExHDConstituent TPExDataset = "hd_constituents"       // 高殖利率指數成分股（T218）
	TPExOtcMopsfin    TPExDataset = "otc_mopsfin"           // 上櫃治理/監理/股務（T237，kind 模板）
	TPExOtcQfiiRank   TPExDataset = "otc_qfii_rank"         // 上櫃外資持股排行（T198）
	TPExOtcQfiiInd    TPExDataset = "otc_qfii_industry"     // 上櫃外資類股持股（T198）
	TPExOtcInstiTrd   TPExDataset = "otc_insti_trading"     // 上櫃投信買賣超彙總（T199）
	TPExOtcDealerTrd  TPExDataset = "otc_dealer_trading"    // 上櫃自營商買賣超彙總（T199）
	TPExOtcAfterHours TPExDataset = "otc_after_hours"       // 上櫃盤後定價行情（T202）
	TPExOtcWarnNote   TPExDataset = "otc_warn_note"         // 上櫃注意累計次數異常（T203）
	TPExEmgQuotes     TPExDataset = "emerging_quotes"       // 興櫃當日行情表（T212）
	TPExEmgHighlight  TPExDataset = "emerging_highlight"    // 興櫃市場現況（T212）
	TPExEmgEpsRank    TPExDataset = "emerging_eps_rank"     // 興櫃 EPS 排名（T213）
	TPExEmgCapRank    TPExDataset = "emerging_cap_rank"     // 興櫃資本額排名（T213）
	TPExT50Latest     TPExDataset = "t50_latest"            // 富櫃50當日收盤（T217）
	TPExT50History    TPExDataset = "t50_history"           // 富櫃50歷史收盤（T217）
	TPExT50Const      TPExDataset = "t50_constituents"      // 富櫃50成分股（T217）
	TPExT200Latest    TPExDataset = "t200_latest"           // 富櫃200當日收盤（T217）
	TPExT200Const     TPExDataset = "t200_constituents"     // 富櫃200成分股（T217）
	TPExGILatest      TPExDataset = "gi_latest"             // 公司治理指數當日收盤（T219）
	TPExGIConst       TPExDataset = "gi_constituents"       // 公司治理指數成分股（T219）
	TPExSILatest      TPExDataset = "si_latest"             // 薪酬指數當日收盤（T219）
	TPExSIConst       TPExDataset = "si_constituents"       // 薪酬指數成分股（T219）
	TPExEmp88History  TPExDataset = "emp88_history"         // 勞工就業88指數歷史收盤（T220）
	TPExEmp88Latest   TPExDataset = "emp88_latest"          // 勞工就業88指數當日收盤（T220）
	TPExEmp88Const    TPExDataset = "emp88_constituents"    // 勞工就業88指數成分股（T220）
	TPExWarrantDaily  TPExDataset = "otc_warrant_daily"     // 上櫃權證收盤行情（T221）
	TPExWarrantBasic  TPExDataset = "otc_warrant_basic"     // 上櫃權證基本資料彙總（T221）
	TPExWarrantIssue  TPExDataset = "otc_warrant_issue"     // 上櫃權證發行基本資料（T221）
	TPExWCBIssue      TPExDataset = "wcb_issue"             // 牛熊證發行基本資料（T222）
	TPExWCBDaily      TPExDataset = "wcb_daily"             // 牛熊證收盤行情（T222）
	TPExWXYIssue      TPExDataset = "wxy_issue"             // 展延牛熊證發行資料（T222）
	TPExWXYDaily      TPExDataset = "wxy_daily"             // 展延牛熊證收盤行情（T222）
	TPExRankPE        TPExDataset = "rank_pe"               // 上櫃本益比排行（T223）
	TPExRankVolume    TPExDataset = "rank_volume"           // 上櫃成交量排行（T223）
	TPExRankAmount    TPExDataset = "rank_amount"           // 上櫃成交值排行（T223）
	TPExRankTurnover  TPExDataset = "rank_turnover"         // 上櫃週轉率排行（T223）
	TPExRankMktVal    TPExDataset = "rank_market_value"     // 上櫃市值排行（T223）
	TPExRankAmtAvg    TPExDataset = "rank_amount_avg"       // 上櫃日均值排行（T223）
	TPExRankVolAvg    TPExDataset = "rank_volume_avg"       // 上櫃日均量排行（T223）
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
		TPExOtcDaily:      "/tpex_mainboard_daily_close_quotes", // T155
		TPExOtcMonthlyRev: "/mopsfin_t187ap05_O",                // 上櫃月營收彙總（T195）
		TPExBrokerVolume:  "/tpex_active_broker_volume",         // 熱門股券商進出排行（T196）
		TPExOtcForeignTrd: "/tpex_3insti_qfii_trading",          // 外資及陸資買賣超彙總（T197）
		TPExOtcExRightDay: "/tpex_exright_daily",                // 除權息計算結果（T200）
		TPExOtcDTTargets:  "/tpex_securities",                   // 當沖標的（T201）
		TPExOtcDTStats:    "/tpex_intraday_trading_statistics",  // 當沖統計（T201）
		TPExOtcESG:        "/t187ap46_O_%s",                     // 上櫃 ESG 揭露 topic 模板（T216）
		TPExHDIndex:       "/tphd_index",                        // 高殖利率指數歷史（T218）
		TPExHDLatest:      "/tphd_change",                       // 高殖利率指數當日（T218）
		TPExHDConstituent: "/tphd_constituents",                 // 高殖利率指數成分股（T218）
		TPExOtcMopsfin:    "/mopsfin_%s",                        // 上櫃治理系列端點模板（T237）
		TPExOtcQfiiRank:   "/tpex_3insti_qfii",                  // 外資持股排行（T198）
		TPExOtcQfiiInd:    "/tpex_3insti_qfii_industry",         // 類股外資持股（T198）
		TPExOtcInstiTrd:   "/tpex_3insti_trading",               // 投信買賣超彙總（T199）
		TPExOtcDealerTrd:  "/tpex_3insti_dealer_trading",        // 自營商買賣超彙總（T199）
		TPExOtcAfterHours: "/tpex_off_market",                   // 盤後定價行情（T202）
		TPExOtcWarnNote:   "/tpex_trading_warning_note",         // 注意累計次數異常（T203）
		TPExEmgQuotes:     "/tpex_esb_latest_statistics",        // 興櫃當日行情表（T212）
		TPExEmgHighlight:  "/tpex_esb_highlight",                // 興櫃市場現況（T212）
		TPExEmgEpsRank:    "/tpex_esb_eps_rank",                 // 興櫃 EPS 排名（T213）
		TPExEmgCapRank:    "/tpex_esb_capitals_rank",            // 興櫃資本額排名（T213）
		TPExT50Latest:     "/tpex50_change",                     // 富櫃50當日收盤（T217）
		TPExT50History:    "/tpex50_index",                      // 富櫃50歷史收盤（T217）
		TPExT50Const:      "/tpex50_constituents",               // 富櫃50成分股（T217）
		TPExT200Latest:    "/tpex200_change",                    // 富櫃200當日收盤（T217）
		TPExT200Const:     "/tpex200_constituents",              // 富櫃200成分股（T217）
		TPExGILatest:      "/tpcgi_change",                      // 治理指數當日收盤（T219）
		TPExGIConst:       "/tpcgi_constituents",                // 治理指數成分股（T219）
		TPExSILatest:      "/tpci_change",                       // 薪酬指數當日收盤（T219）
		TPExSIConst:       "/tpci_constituents",                 // 薪酬指數成分股（T219）
		TPExEmp88History:  "/tpex_emp88_reward_index",           // 勞工就業88歷史收盤（T220）
		TPExEmp88Latest:   "/tpex_emp88_change",                 // 勞工就業88當日收盤（T220）
		TPExEmp88Const:    "/tpex_emp88_constituents",           // 勞工就業88成分股（T220）
		TPExWarrantDaily:  "/tpex_warrant_daily_quts",           // 上櫃權證收盤行情（T221）
		TPExWarrantBasic:  "/mopsfin_t187ap37_O",                // 上櫃權證基本資料彙總（T221）
		TPExWarrantIssue:  "/tpex_warrant_issue",                // 上櫃權證發行基本資料（T221）
		TPExWCBIssue:      "/tpex_warrant_wcb_issue",            // 牛熊證發行資料（T222）
		TPExWCBDaily:      "/tpex_warrant_wcb_daily_quts",       // 牛熊證收盤行情（T222）
		TPExWXYIssue:      "/tpex_warrant_wxy_issue",            // 展延牛熊證發行資料（T222）
		TPExWXYDaily:      "/tpex_warrant_wxy_daily_quts",       // 展延牛熊證收盤行情（T222）
		TPExRankPE:        "/tpex_pe_ratio_top10",               // 本益比排行（T223）
		TPExRankVolume:    "/tpex_volume_rank",                  // 成交量排行（T223）
		TPExRankAmount:    "/tpex_amount_rank",                  // 成交值排行（T223）
		TPExRankTurnover:  "/tpex_daily_turnover",               // 週轉率排行（T223）
		TPExRankMktVal:    "/tpex_daily_market_value",           // 市值排行（T223）
		TPExRankAmtAvg:    "/tpex_trading_amount_avg",           // 日均值排行（T223）
		TPExRankVolAvg:    "/tpex_trading_volumes_avg",          // 日均量排行（T223）
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
	path := tpexPaths[ds]
	if ds == TPExOtcESG {
		// 上櫃 ESG 揭露以 topic 參數展開路徑模板（對稱 TWSEAPIESG 慣例）
		topic := params.Get("topic")
		if topic == "" {
			topic = "1"
		}
		path = fmt.Sprintf(path, topic)
		params = url.Values{}
	}
	if ds == TPExOtcMopsfin {
		// 上櫃治理系列以 kind 參數展開路徑模板（T237；如 t187ap08_O）
		kind := params.Get("kind")
		if kind == "" {
			return tpexBase + "/mopsfin_unknown"
		}
		path = fmt.Sprintf(path, kind)
		params = url.Values{}
	}
	u := tpexBase + path
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

// Deprecated: v2.1 §6 起轉換集中於 pkg/model/normalize（FromTPEx）；
// 本方法為 v1.3 相容層，遷移時逐步移除（T022）。
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
	if strings.Contains(p, "/t187ap46_O_") {
		return string(TPExOtcESG), nil // topic 模板路徑（T216）
	}
	if strings.Contains(p, "/mopsfin_") {
		return string(TPExOtcMopsfin), nil // 上櫃治理系列模板路徑（T237）
	}
	for ds, path := range tpexPaths {
		if !strings.Contains(path, "%") && strings.HasSuffix(p, path) {
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
	case string(TPExOtcDaily):
		// 欄位序同 tpex_mainboard_quotes（T155 實測），共用收盤行情正規化。
		out = normalizeTPExDailyClose(ms)
	case string(TPExOtcMonthlyRev):
		// 官方中文欄位 passthrough（T195 實測：出表日期/資料年月/公司代號/
		// 營業收入-當月營收/上月比較增減(%) 等）。
		out = ms
	case string(TPExBrokerVolume):
		// passthrough（T196 實測：Date/StockRanking/SecuritiesCompanyCodeAnd
		// CompanyName/SecuritiesFirmsRanking/SecuritiesFirmsCode/TotalPurchase
		// Shares/TotalSellShares）。
		out = ms
	case string(TPExOtcForeignTrd):
		// passthrough（T197 實測；官方欄位 key 含不規則空白，原樣保留）。
		out = ms
	case string(TPExOtcInstiTrd), string(TPExOtcDealerTrd):
		// passthrough（T199 實測；欄位 Date/Rank/SecuritiesCompanyCode/
		// CompanyName/Buy/Sell/NetBuy，dealer 另有 NetBuySell，原樣保留）。
		out = ms
	case string(TPExOtcAfterHours):
		// passthrough（T202 實測；欄位 Date/SecuritiesCompanyCode/CompanyName/
		// Close/Transactions/TradeVolume/TradeAmount 等，原樣保留）。
		out = ms
	case string(TPExOtcWarnNote):
		// passthrough（T203 實測；欄位 Date/SecuritiesCompanyCode/CompanyName/
		// AccumulationSituation，原樣保留）。
		out = ms
	case string(TPExEmgQuotes), string(TPExEmgHighlight):
		// passthrough（T212 實測；興櫃行情 Date/SecuritiesCompanyCode/
		// CompanyName/LatestPrice/TransactionVolume 等、市場現況
		// RegisteredStocksNumber/TotalMarketValue 等，原樣保留）。
		out = ms
	case string(TPExEmgEpsRank), string(TPExEmgCapRank):
		// passthrough（T213 實測；欄位 Date/Rank/SecuritiesCompanyCode/
		// CompanyName/EPS 或 Capital，原樣保留）。
		out = ms
	case string(TPExT50Latest), string(TPExT50History), string(TPExT50Const),
		string(TPExT200Latest), string(TPExT200Const):
		// passthrough（T217 實測；富櫃50/200 指數與成分股，官方中英欄位
		// 混用且兩家族格式不同，原樣保留）。
		out = ms
	case string(TPExGILatest), string(TPExGIConst), string(TPExSILatest), string(TPExSIConst):
		// passthrough（T219 實測；治理/薪酬指數 Date/Name/Index/Change 或
		// SecuritiesCompanyCode/CompanyName，原樣保留）。
		out = ms
	case string(TPExEmp88History), string(TPExEmp88Latest), string(TPExEmp88Const):
		// passthrough（T220 實測；勞工就業88指數 GretaiLaborEmployment88*/
		// Name/Index/Change 或 SecuritiesCompanyCode，原樣保留）。
		out = ms
	case string(TPExWarrantDaily), string(TPExWarrantBasic), string(TPExWarrantIssue):
		// passthrough（T221 實測；上櫃權證行情/基本/發行資料，官方中英欄位
		// 混用，原樣保留）。
		out = ms
	case string(TPExWCBIssue), string(TPExWCBDaily), string(TPExWXYIssue), string(TPExWXYDaily):
		// passthrough（T222 實測；牛熊證/展延牛熊證發行與行情，官方中英欄位
		// 混用，原樣保留）。
		out = ms
	case string(TPExRankPE), string(TPExRankVolume), string(TPExRankAmount),
		string(TPExRankTurnover), string(TPExRankMktVal), string(TPExRankAmtAvg),
		string(TPExRankVolAvg):
		// passthrough（T223 實測；歷史排行七類 Date/Rank/Code/Name+各排行
		// 數值欄位，官方欄名不一，原樣保留）。
		out = ms
	case string(TPExOtcExRightDay):
		// passthrough（T200 實測：Date/SecuritiesCompanyCode/CompanyName/
		// ExRightsDiviend/CashDividend/LimitUp/LimitDown/OpeningReferencePrice 等）。
		out = ms
	case string(TPExOtcDTTargets):
		// passthrough（T201 實測：資料日期/證券代號/證券名稱/暫停現股賣出後
		// 現款買進當沖註記）。
		out = ms
	case string(TPExOtcDTStats):
		// passthrough（T201 實測：Date/DayTradingVolume/佔市場比/買賣值系列）。
		out = ms
	case string(TPExOtcESG):
		// 上櫃 ESG 揭露：passthrough（T216；出表日期/報告年度/公司代號/
		// 公司名稱 + 主題指標欄位）。
		out = ms
	case string(TPExHDIndex):
		// 高殖利率指數歷史：passthrough（T218；Date/TPExHighDividendYieldIndex/
		// TPExHighDividendYieldTotalReturnIndex）。
		out = ms
	case string(TPExHDLatest):
		// 高殖利率指數當日收盤：passthrough（T218；上游端點名為 tphd_change
		// 但實際回傳含治理指數等各指數名稱，如實保留）。
		out = ms
	case string(TPExHDConstituent):
		// 高殖利率指數成分股：passthrough（T218；Date/SecuritiesCompanyCode/
		// CompanyName）。
		out = ms
	case string(TPExOtcMopsfin):
		// 上櫃治理/監理/股務系列：passthrough（T237；官方中文欄位原樣保留）。
		out = ms
	case string(TPExOtcQfiiRank), string(TPExOtcQfiiInd):
		// 上櫃外資持股排行/類股彙總：passthrough（T198；官方英文欄位原樣保留）。
		out = ms
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
	string(TPExOtcDaily):      []TPExDailyCloseRow{},
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
