package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// fetch.go 提供盤後工具（T011）共用之資料取得輔助：
// 快取讀穿（§12.2 GetOrFetch + §4.2 TTL 政策）與交易日歷導航。

// cacheDataset 將 provider 資料集 ID 對映至 §4.2 政策表資料類別
// （新增資料集於此登錄；未登錄資料類別視為不納管快取）。
var cacheDataset = map[string]string{
	string(provider.TWSEWDDailyK):           cache.DatasetDailyKLine,
	string(provider.TWSEWDMonthlyAvg):       cache.DatasetDailyKLine, // 月平均價（T169 補登錄）
	string(provider.TWSEWDMarketClose):      cache.DatasetDailyKLine,
	string(provider.TWSEWDInstitutional):    cache.DatasetInstitutional, // 與 TPEx institutional 同名
	string(provider.TWSEWDMargin):           cache.DatasetMargin,        // 與 TPEx margin 同名
	string(provider.TWSEWDAbnormal):         cache.DatasetAlertStock,
	string(provider.TWSEWDAfterHours):       cache.DatasetDailyKLine,    // 盤後定價交易：日頻快照（T040）
	string(provider.TWSEWDBlockTrades):      cache.DatasetDailyKLine,    // 鉅額交易日統計（T042）
	string(provider.TWSEWDBlockMonthly):     cache.DatasetDailyKLine,    // 鉅額交易月統計（T044）
	string(provider.TWSEWDBlockYearly):      cache.DatasetDailyKLine,    // 鉅額交易年統計（T045）
	string(provider.TWSEWDCrossMarket):      cache.DatasetDailyKLine,    // 跨市場成交（T115）
	string(provider.TWSEWDDayTradeTargets):  cache.DatasetAlertStock,    // 當沖標的（T116）
	string(provider.TWSEWDFinProgAbn):       cache.DatasetAlertStock,    // 異常推介個股（T121）
	string(provider.TWSEWDSBLVolume):        cache.DatasetDailyKLine,    // 借券賣出量（T119）
	string(provider.TWSEWDFirstForeign):     cache.DatasetDailyKLine,    // 第一上市外股（T122）
	string(provider.TWSEWDMarginRestrict):   cache.DatasetCalendar,      // 停資停券預告（T139）
	string(provider.TWSEWDGainLoss):         cache.DatasetDailyKLine,    // 漲跌家數統計（T142）
	string(provider.TWSEWDOddLot):           cache.DatasetDailyKLine,    // 盤後零股（T149）
	string(provider.TWSEWDTradingChanges):   cache.DatasetCalendar,      // 變更交易（T163）
	string(provider.TWSEWDPriceChangeLim):   cache.DatasetValuation,     // 漲跌停參考價（T172）
	string(provider.TWSEWDNewList5D):        cache.DatasetCalendar,      // 首五日無漲跌幅（T175）
	string(provider.TWSEWDSuspDayTradeAnn):  cache.DatasetCalendar,      // 暫停當沖預告（T176）
	string(provider.TWSEWDSuspDayTradeHis):  cache.DatasetTAIFEXHistory, // 暫停當沖歷史（T177）
	string(provider.TWSEWDSuspended):        cache.DatasetCalendar,      // 暫停交易證券（T179）
	string(provider.TWSEWDTopVolume):        cache.DatasetDailyKLine,    // 成交量Top20（T184）
	string(provider.TWSEWDInstiAmounts):     cache.DatasetInstitutional, // 法人買賣超金額歷史（T146）
	string(provider.TWSEWDTurnoverHistory):  cache.DatasetDailyKLine,    // 市場成交資訊歷史（T147）
	string(provider.TWSEWDSLSBalanceHis):    cache.DatasetMargin,        // 融券借券餘額歷史（T164）
	string(provider.TWSEWDSBLTradesHis):     cache.DatasetDailyKLine,    // 借券賣出成交歷史（T165）
	string(provider.TWSEWDBondRedemption):   cache.DatasetDailyKLine,    // 中央登錄公債補息（T055）
	string(provider.TWSEWDEtfRegInv):        cache.DatasetCalendar,      // 定期定額戶數統計排行（T120）
	string(provider.TWSEAPICumVoting):       cache.DatasetCalendar,      // 累積投票制董監事彙總（T056）
	string(provider.TWSEAPIOwnScopeHalt):    cache.DatasetCalendar,      // 經營權異動停止買賣（T057）
	string(provider.TWSEAPIOwnScopeTrade):   cache.DatasetCalendar,      // 經營權異動列變更交易（T058）
	string(provider.TWSEAPIScopeChanges):    cache.DatasetCalendar,      // 營業範圍重大變更公司（T060）
	string(provider.TWSEAPIIndepDirectors):  cache.DatasetCalendar,      // 獨立董監事兼任彙總（T063）
	string(provider.TWSEAPIOwnershipChange): cache.DatasetCalendar,      // 經營權異動公司（T064）
	string(provider.TWSEAPIBalCI):           cache.DatasetFinancials,    // 資產負債表（季頻，§5.2 財報 90d，T067）
	string(provider.TWSEAPIBalBASI):         cache.DatasetFinancials,    // 金融業
	string(provider.TWSEAPIBalBD):           cache.DatasetFinancials,    // 證券期貨業
	string(provider.TWSEAPIBalFH):           cache.DatasetFinancials,    // 金控業
	string(provider.TWSEAPIBalINS):          cache.DatasetFinancials,    // 保險業
	string(provider.TWSEAPIBalMIM):          cache.DatasetFinancials,    // 異業
	string(provider.TWSEAPIBoardInsuff):     cache.DatasetCalendar,      // 董監持股不足（T069）
	string(provider.TWSEAPIBoardInsuffCon):  cache.DatasetCalendar,      // 持股不足連續月份（T070）
	string(provider.TWSEAPIBoardPledged):    cache.DatasetCalendar,      // 董監質權設定（T071）
	string(provider.TWSEAPIBoardHoldings):   cache.DatasetCalendar,      // 董監持股餘額明細（T072）
	string(provider.TWSEAPICEODualRole):     cache.DatasetCalendar,      // 董事長兼任總經理（T073）
	string(provider.TWSEAPIDirCompCon):      cache.DatasetFinancials,    // 合併董事酬金（T076）
	string(provider.TWSEAPISupCompCon):      cache.DatasetFinancials,    // 合併監察人酬金（T077）
	string(provider.TWSEAPIInsiderPreann):   cache.DatasetCalendar,      // 內部人轉讓申報（T078）
	string(provider.TWSEAPIInsiderUntrans):  cache.DatasetCalendar,      // 內部人未轉讓（T079）
	string(provider.TWSEAPIDirComp):         cache.DatasetFinancials,    // 董事酬金（T080）
	string(provider.TWSEAPIMajorSharehold):  cache.DatasetCalendar,      // 大股東名單（T097）
	string(provider.TWSEAPIEPSStats):        cache.DatasetFinancials,    // EPS 統計（T083）
	string(provider.TWSEAPIIncCI):           cache.DatasetFinancials,    // 損益表-一般業（T092）
	string(provider.TWSEAPIIncBASI):         cache.DatasetFinancials,
	string(provider.TWSEAPIIncBD):           cache.DatasetFinancials,
	string(provider.TWSEAPIIncFH):           cache.DatasetFinancials,
	string(provider.TWSEAPIIncINS):          cache.DatasetFinancials,
	string(provider.TWSEAPIIncMIM):          cache.DatasetFinancials,
	string(provider.TWSEAPIDisclosureVio):   cache.DatasetCalendar,   // 資訊揭露違法（T094）
	string(provider.TWSEAPIFundBasic):       cache.DatasetCalendar,   // 基金基本資料（T124）
	string(provider.TWSEAPIPubBoardHold):    cache.DatasetCalendar,   // 公發董監持股（T159）
	string(provider.TWSEAPIPubIncCI):        cache.DatasetFinancials, // 公發損益表-一般業（T160）
	string(provider.TWSEAPIPubIncBASI):      cache.DatasetFinancials,
	string(provider.TWSEAPIPubIncBD):        cache.DatasetFinancials,
	string(provider.TWSEAPIPubIncFH):        cache.DatasetFinancials,
	string(provider.TWSEAPIPubIncINS):       cache.DatasetFinancials,
	string(provider.TWSEAPIPubIncMIM):       cache.DatasetFinancials,
	string(provider.TWSEAPIPubBalCI):        cache.DatasetFinancials, // 公發資產負債表-一般業（T158）
	string(provider.TWSEAPIPubBalBASI):      cache.DatasetFinancials,
	string(provider.TWSEAPIPubBalBD):        cache.DatasetFinancials,
	string(provider.TWSEAPIPubBalFH):        cache.DatasetFinancials,
	string(provider.TWSEAPIPubBalINS):       cache.DatasetFinancials,
	string(provider.TWSEAPIPubBalMIM):       cache.DatasetFinancials,
	string(provider.TWSEAPISupervisorComp):  cache.DatasetFinancials,   // 監察人酬金（T111）
	string(provider.TWSEWDMonthlyAvgAll):    cache.DatasetDailyKLine,   // 月平均價（T168）
	string(provider.TWSEWDStockMonTrade):    cache.DatasetDailyKLine,   // 個股月成交（T171）
	string(provider.TWSEWDStockYearHis):     cache.DatasetDailyKLine,   // 個股歷年成交（T173）
	string(provider.TWSEWDStockYearTrade):   cache.DatasetDailyKLine,   // 年度成交全市場（T174）
	string(provider.TWSEAPITopForeign):      cache.DatasetForeignHold,  // 外資持股Top20（T185）
	string(provider.TWSEAPITwseNews):        cache.DatasetMaterialNews, // 證交所新聞（T186）
	string(provider.TWSEAPITwseEvents):      cache.DatasetMaterialNews, // 證交所活動訊息（T191）
	string(provider.TWSEAPINoteTrans):       cache.DatasetAlertStock,   // 注意累計次數異常（T193）
	string(provider.TWSEAPIWarrantBasic):    cache.DatasetCalendar,     // 權證基本資料（T187）
	string(provider.TWSEAPIWarrantTrader):   cache.DatasetCalendar,     // 權證流動量提供者（T189）
	string(provider.TWSEAPIWarrantIssue):    cache.DatasetCalendar,     // 權證發行統計（T190）
	string(provider.TWSEAPISecPenalty):      cache.DatasetCalendar,     // 證期局裁罰（T106）
	string(provider.TWSEAPIForeignApply):    cache.DatasetCalendar,     // 外國公司申請第一上市（T123）
	string(provider.TWSEAPINewListing):      cache.DatasetCalendar,     // 最近上市公司（T162）
	string(provider.TWSEAPILocalApply):      cache.DatasetCalendar,     // 本國公司申請上市（T138）
	string(provider.TWSEAPISuspListing):     cache.DatasetCalendar,     // 終止上市公司（T178）
	string(provider.TWSEWDMarginInfo):       cache.DatasetMargin,       // 信用交易統計（T140）
	string(provider.TWSEWDHoliday):          cache.DatasetCalendar,     // 開休市日期（T144）
	string(provider.TWSEWDRealTimeStats):    cache.DatasetMISSnapshot,  // 5秒成交統計（盤中即時，T161）
	string(provider.TWSEWDTaiwan50):         cache.DatasetDailyKLine,   // 臺灣50指數歷史（T181）
	string(provider.TWSEWDIslandIndex):      cache.DatasetDailyKLine,   // 寶島指數歷史（T182）
	string(provider.TWSEWDTotalReturn):      cache.DatasetDailyKLine,   // 報酬指數歷史（T183）
	string(provider.TWSEAPIMeetingAnn):      cache.DatasetCalendar,     // 股東會公告（T107/T108）
	string(provider.TWSEAPIMeetingDates):    cache.DatasetCalendar,     // 股東會日期地點（T109）
	string(provider.TWSEAPIProposalExer):    cache.DatasetCalendar,     // 提案權行使（T110）
	string(provider.TWSEAPIBrokerBasic):     cache.DatasetCalendar,     // 券商基本資料（T046）
	string(provider.TWSEAPIBrokerBranch):    cache.DatasetCalendar,     // 券商分公司（T047）
	string(provider.TWSEAPIBrokerElec):      cache.DatasetCalendar,     // 電子交易統計（T048）
	string(provider.TWSEAPIBrokerGender):    cache.DatasetCalendar,     // 營業員性別統計（T049）
	string(provider.TWSEAPIBrokerHQ):        cache.DatasetCalendar,     // 券商本公司（T050）
	string(provider.TWSEAPIBrokerIncome):    cache.DatasetCalendar,     // 券商損益（T051）
	string(provider.TWSEAPIBrokerMonthly):   cache.DatasetCalendar,     // 券商月報表（T052）
	string(provider.TWSEAPIBrokerPersonnel): cache.DatasetCalendar,     // 從業人員統計（T053）
	string(provider.TWSEAPIBrokerRegInv):    cache.DatasetCalendar,     // 定期定額名單（T054）
	string(provider.TWSEAPISupervisorAck):   cache.DatasetCalendar,     // 財報監察人承認（T084）
	string(provider.TWSEAPIProfitability):   cache.DatasetFinancials,   // 營益分析（季頻，T101/T102）
	string(provider.TWSEAPIAuditVariance):   cache.DatasetFinancials,   // 財測查核差異（T103）
	string(provider.TWSEAPIForecastAchv):    cache.DatasetFinancials,   // 財測達成率（T104）
	string(provider.TWSEWDForeignQFIIS):     cache.DatasetForeignHold,
	string(provider.TWSEWDIndexHistory):     cache.DatasetDailyKLine, // 指數歷史同 daily_kline 政策
	string(provider.TWSEAPIForeignHoldings): cache.DatasetForeignHold,
	string(provider.TWSEAPIWarrants):        cache.DatasetWarrants,
	string(provider.TWSEAPIPunish):          cache.DatasetAlertStock,
	string(provider.TWSEAPIESG):             cache.DatasetESG,
	string(provider.TWSEAPIGovernance):      cache.DatasetESG,
	string(provider.TWSEAPIIndices):         cache.DatasetDailyKLine, // 指數收盤同 daily_kline 政策
	string(provider.TPExDailyClose):         cache.DatasetDailyKLine,
	string(provider.TPExOtcDaily):           cache.DatasetDailyKLine, // 上櫃收盤行情（T155）
	string(provider.TPExOtcMonthlyRev):      cache.DatasetMonthlyRevenue, // 上櫃月營收（T195）
	string(provider.TPExBrokerVolume):       cache.DatasetInstitutional,  // 券商進出排行（T196）
	string(provider.TPExOtcForeignTrd):      cache.DatasetInstitutional,  // 外資買賣超彙總（T197）
	string(provider.TPExOtcExRightDay):      cache.DatasetDailyKLine,     // 除權息計算結果（T200）
	string(provider.TPExOtcDTTargets):       cache.DatasetAlertStock,     // 上櫃當沖標的（T201，僅 L1）
	string(provider.TPExOtcDTStats):         cache.DatasetDailyKLine,     // 上櫃當沖統計（T201）
	string(model.TAOptionsDelta):            cache.DatasetCalendar,   // 選擇權 Delta（T151）
	string(model.TAOIChange):                cache.DatasetCalendar,   // 台指選擇權 OI 增減（T154）
	string(model.TAStockMargin):             cache.DatasetCalendar,   // 股票期貨保證金（T167）
	string(provider.TPExAttention):          cache.DatasetAlertStock,
	string(provider.TPExDisposition):        cache.DatasetAlertStock,
	// MOPS Open Data（T012）
	string(provider.MOPSMonthlyRevenue):  cache.DatasetMonthlyRevenue,
	string(provider.MOPSIncomeSummary):   cache.DatasetFinancials,
	string(provider.MOPSProfitRatios):    cache.DatasetFinancials,
	string(provider.MOPSBalanceSheet):    cache.DatasetFinancials,
	string(provider.MOPSIncomeStatement): cache.DatasetFinancials,
	string(provider.MOPSCashFlow):        cache.DatasetFinancials,
	string(provider.MOPSAnnouncements):   cache.DatasetMaterialNews,
	string(provider.MOPSCompanyProfile):  cache.DatasetCalendar, // 24h TTL，類似行事曆
	// T014：TWSE-API 估值/除權息/股利分派資料
	string(provider.TWSEAPIValuation): cache.DatasetValuation,
	string(provider.TWSEAPIExDiv):     cache.DatasetExDivCalendar,
	string(provider.TWSEAPIDividend):  cache.DatasetDividend,
	string(provider.TPExPEValuation):  cache.DatasetValuation,
	string(provider.TPExExRights):     cache.DatasetExDivCalendar,
	// T037：MOPS ESG 揭露八主題（與 TWSE-API ESG 同 DatasetESG 政策，24h）
	string(provider.MOPSESGGhg):       cache.DatasetESG,
	string(provider.MOPSESGRenewable): cache.DatasetESG,
	string(provider.MOPSESGWater):     cache.DatasetESG,
	string(provider.MOPSESGWaste):     cache.DatasetESG,
	string(provider.MOPSESgEmployee):  cache.DatasetESG,
	string(provider.MOPSESGBoard):     cache.DatasetESG,
	string(provider.MOPSESGConf):      cache.DatasetESG,
	string(provider.MOPSESGTcfd):      cache.DatasetESG,
}

// policyDataset 回傳資料集對應之政策類別；未登錄時回傳原字串（cacheable=false）。
func policyDataset(dataset string) string {
	if c, ok := cacheDataset[dataset]; ok {
		return c
	}
	return dataset
}

// fetchNormalize 執行「快取讀穿 → Fetch → Validate → Normalize →
// Unmarshal」之標準路徑，回傳資料、是否命中快取與是否為 stale-if-error
// 回退（§3.2 is_cached；v2.1 §5.2）。stale=true 表示上游失敗回退過期 L2
// 值，Handler 應將 _lineage.freshness 標記為 STALE_FALLBACK。
//   - srcID：資料源 ID（model.Source*，供快取鍵與 lineage）
//   - dataset：資料類別（§4.2，供 TTL 政策與 L2 資格）
//   - dataDate：資料歸屬日期（§4.2 索引，YYYY-MM-DD）
//   - key：快取鍵（含日期/代碼等識別，見 cache.KeyString）
//   - fetch：上游呼叫（URL 建構 + Fetch）；nil 時僅做快取讀穿
func fetchNormalize[T any](a *App, ctx context.Context, dataset, dataDate, key string,
	fetch func() ([]byte, error)) (T, bool, bool, error) {
	var zero T
	if a.cache == nil {
		return zero, false, false, fmt.Errorf("mcp: 快取層未初始化")
	}
	dataset = policyDataset(dataset)
	ttl, cacheable := cache.TTLFor(dataset, a.now())
	fn := func(ctx context.Context) (T, error) {
		a.httpCalls.Add(1) // §12.9 instrumentation：快取 miss 即為一次上游 HTTP 請求
		raw, err := fetch()
		if err != nil {
			return zero, err
		}
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return zero, fmt.Errorf("mcp: Normalize 結果解析失敗: %w", err)
		}
		return v, nil
	}
	if !cacheable {
		v, err := fn(ctx)
		return v, false, false, err
	}
	v, cached, err := cache.GetOrFetch(ctx, a.cache, key, ttl, fn,
		cache.WithDataset(dataset, dataDate), cache.WithStaleFallback())
	if errors.Is(err, cache.ErrServedStale) {
		return v, true, true, nil
	}
	return v, cached, false, err
}

// fetchRaw 為 fetchNormalize 之原始版：fetch 端即做 Fetch+Validate+
// Normalize（回傳 normalizer 產出之 []byte），僅快取鍵與 TTL 在此層管理。
func (a *App) fetchRaw(ctx context.Context, dataset, dataDate, key string,
	fetch func() ([]byte, error)) (cached, stale bool, raw []byte, err error) {
	if a.cache == nil {
		return false, false, nil, fmt.Errorf("mcp: 快取層未初始化")
	}
	dataset = policyDataset(dataset)
	ttl, cacheable := cache.TTLFor(dataset, a.now())
	fn := func(ctx context.Context) ([]byte, error) {
		a.httpCalls.Add(1) // §12.9 instrumentation
		return fetch()
	}
	if !cacheable {
		b, err := fn(ctx)
		return false, false, b, err
	}
	b, fromCache, err := cache.GetOrFetch(ctx, a.cache, key, ttl, fn,
		cache.WithDataset(dataset, dataDate), cache.WithStaleFallback())
	if errors.Is(err, cache.ErrServedStale) {
		return true, true, b, nil
	}
	return fromCache, false, b, err
}

// prevTradingDay 回傳 d 之前第 n 個交易日（n≥1）；搜尋上限 60 日。
func (a *App) prevTradingDay(d time.Time, n int) (time.Time, error) {
	if n < 1 {
		n = 1
	}
	day := d.AddDate(0, 0, -1)
	for i := 0; i < 60 && n > 0; i++ {
		if a.calendar.IsTradingDay(day) {
			n--
			if n == 0 {
				return day, nil
			}
		}
		day = day.AddDate(0, 0, -1)
	}
	return d, fmt.Errorf("mcp: 60 日內找不到第 %d 個交易日", n)
}

// resolveDate 解析工具之 date 參數（YYYY-MM-DD，可空）：
//   - 給定：驗證格式並回傳；
//   - 空：回傳最近交易日（盤後時段 ≥15:00 為今日，否則前一日）。
func (a *App) resolveDate(date string) (string, error) {
	if date != "" {
		t, err := model.ParseDate(date)
		if err != nil {
			return "", fmt.Errorf("mcp: 參數 date 格式必須為 YYYY-MM-DD: %w", err)
		}
		return model.FormatDate(t), nil
	}
	now := a.now()
	day := now
	if now.Hour()*3600+now.Minute()*60 < 15*3600 || !a.calendar.IsTradingDay(now) {
		prev, err := a.prevTradingDay(now, 1)
		if err != nil {
			return "", err
		}
		day = prev
	}
	return model.FormatDate(day), nil
}

// symbolOf 解析 symbol 參數並查 Symbol Registry。
func (a *App) symbolOf(code string) (model.Symbol, error) {
	sym, ok := a.symbols.Lookup(code)
	if !ok {
		return model.Symbol{}, fmt.Errorf("非法代號 %q（未註冊於 Symbol Registry）", code)
	}
	return sym, nil
}
