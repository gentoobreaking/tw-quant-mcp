package provider

// TWSE Adapter（T008）：TWSE-API（openapi.twse.com.tw）與 TWSE-WEB
// （www.twse.com.tw）之盤後資料來源，實作 SourceContract（§2.2）。
//
// 資料集對應 §2 登錄表：
//
//	TWSE-API：個股日收盤、外資持股、權證、指數、ESG、公司治理
//	TWSE-WEB：個股日 K、月均價、融資融券、三大法人買賣超（上市）、
//	          全市場收盤行情、加權指數歷史、鉅額交易、異常成交量
//
// 單位換算（§5.1）：TWSE 原生「仟元」「張」一律於本檔 Normalize 換算
// （model.ThousandToYuan / model.LotsToShares），對外欄位統一 元/股/%。
//
// 端點實測（2026-07-31）：STOCK_DAY 之 adjust 參數已被官方移除（2025/2026
// 均驗證無效），URL 仍保留 adjust 傳遞通路，官方恢復時自動生效。

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
)

// TWSEWebDataset 為 TWSE-WEB（www.twse.com.tw）資料集 ID。
type TWSEWebDataset string

// TWSE-WEB 資料集（§2 登錄表 TWSE-WEB 內容範圍）。
const (
	TWSEWDDailyK        TWSEWebDataset = "daily_k"         // 個股日 K（日/週/月，adjust 參數）
	TWSEWDMonthlyAvg    TWSEWebDataset = "monthly_avg"     // 月均價（日收盤價及月平均收盤價）
	TWSEWDMargin        TWSEWebDataset = "margin"          // 融資融券
	TWSEWDInstitutional TWSEWebDataset = "institutional"   // 三大法人買賣超（上市，金額+股數）
	TWSEWDMarketClose   TWSEWebDataset = "market_close"    // 全市場收盤行情
	TWSEWDIndexHistory  TWSEWebDataset = "index_history"   // 加權指數歷史
	TWSEWDBlockTrades   TWSEWebDataset = "block_trades"    // 鉅額交易
	TWSEWDAbnormal      TWSEWebDataset = "abnormal_volume" // 異常成交量（當日公布注意股票）
	TWSEWDForeignQFIIS  TWSEWebDataset = "qfiis"           // 外資及陸資投資持股統計（個股每日，T011）
	TWSEWDAfterHours    TWSEWebDataset = "after_hours"     // 盤後定價交易（T040）
	TWSEWDBlockMonthly  TWSEWebDataset = "block_monthly"   // 鉅額交易月統計（T044）
	TWSEWDBlockYearly   TWSEWebDataset = "block_yearly"    // 鉅額交易年統計（T045）
	// ── parity 批次（T115/T116/T119/T122/T139/T142/T149/T163/T172/T175-T177/T179/T184）──
	TWSEWDCrossMarket     TWSEWebDataset = "cross_market"      // 跨市場成交資訊
	TWSEWDDayTradeTargets TWSEWebDataset = "day_trade_targets" // 當日沖銷標的
	TWSEWDSBLVolume       TWSEWebDataset = "sbl_volume"        // 借券賣出每日量
	TWSEWDFirstForeign    TWSEWebDataset = "first_foreign"     // 第一上市外國股票日成交量值
	TWSEWDMarginRestrict  TWSEWebDataset = "margin_restrict"   // 停資停券預告表
	TWSEWDGainLoss        TWSEWebDataset = "gain_loss"         // 市場漲跌家數統計（opendata）
	TWSEWDOddLot          TWSEWebDataset = "odd_lot"           // 盤後零股行情
	TWSEWDTradingChanges  TWSEWebDataset = "trading_changes"   // 證券變更交易
	TWSEWDPriceChangeLim  TWSEWebDataset = "price_change_lim"  // 股價升降幅（漲跌停參考）
	TWSEWDNewList5D       TWSEWebDataset = "new_list_5d"       // 首五日無漲跌幅
	TWSEWDSuspDayTradeAnn TWSEWebDataset = "suspend_daytrade_ann" // 暫停當沖預告
	TWSEWDSuspDayTradeHis TWSEWebDataset = "suspend_daytrade_his" // 暫停當沖歷史
	TWSEWDSuspended       TWSEWebDataset = "suspended"         // 暫停交易證券
	TWSEWDTopVolume       TWSEWebDataset = "top_volume"        // 成交量 Top20
	TWSEWDInstiAmounts    TWSEWebDataset = "insti_amounts"     // 三大法人買賣超金額歷史（T146）
	TWSEWDTurnoverHistory TWSEWebDataset = "turnover_history"  // 市場成交資訊/週轉率歷史（T147）
	TWSEWDSLSBalanceHis   TWSEWebDataset = "sbl_balance_his"   // 融券借券餘額歷史（T164）
	TWSEWDSBLTradesHis    TWSEWebDataset = "sbl_trades_his"    // 借券賣出成交歷史（T165）
	TWSEWDBondRedemption  TWSEWebDataset = "bond_redemption"   // 中央登錄公債補息資料表（T055）
)

// TWSEAPIDataset 為 TWSE-API（openapi.twse.com.tw）資料集 ID。
type TWSEAPIDataset string

// TWSE-API 資料集（§2 登錄表 TWSE-API 內容範圍）。
const (
	TWSEAPIDailyClose      TWSEAPIDataset = "daily_close"        // 個股日收盤（全市場，含 ETF）
	TWSEAPIForeignHoldings TWSEAPIDataset = "foreign_holdings"   // 外資持股（類股持股比率）
	TWSEAPIWarrants        TWSEAPIDataset = "warrants"           // 權證每日成交
	TWSEAPIIndices         TWSEAPIDataset = "indices"            // 每日各指數行情
	TWSEAPIESG             TWSEAPIDataset = "esg"                // ESG 資訊揭露（topic 1..21）
	TWSEAPIGovernance      TWSEAPIDataset = "company_governance" // 公司治理
	TWSEAPIPunish          TWSEAPIDataset = "punish"             // 集中市場公布處置股票（T011）
	TWSEAPIValuation       TWSEAPIDataset = "valuation"          // 本益比/殖利率/股價淨值比（T014）
	TWSEAPIExDiv           TWSEAPIDataset = "ex_div"             // 除權除息預告表（T014）
	TWSEAPIDividend        TWSEAPIDataset = "dividend"           // 股利分派情形（T014）
	TWSEAPICumVoting       TWSEAPIDataset = "cum_voting"         // 累積投票制選任董監事彙總（t187ap34_L，T056）
	TWSEAPIOwnScopeHalt    TWSEAPIDataset = "own_scope_halt"     // 經營權異動且營業範圍重大變更停止買賣（t187ap26_L，T057）
	TWSEAPIOwnScopeTrade   TWSEAPIDataset = "own_scope_trade"    // 經營權異動且營業範圍重大變更列變更交易（t187ap27_L，T058）
	TWSEAPIScopeChanges    TWSEAPIDataset = "scope_changes"      // 營業範圍重大變更公司（t187ap25_L，T060）
	TWSEAPIIndepDirectors  TWSEAPIDataset = "indep_directors"    // 獨立董監事兼任情形彙總（t187ap30_L，T063）
	TWSEAPIOwnershipChange TWSEAPIDataset = "ownership_change"   // 經營權異動公司（t187ap24_L，T064）
	// 資產負債表 t187ap07_L{suffix}（T067；一般/金融/證券期貨/金控/保險/異業）
	TWSEAPIBalCI   TWSEAPIDataset = "balance_sheet_ci"   // 一般業 _ci
	TWSEAPIBalBASI TWSEAPIDataset = "balance_sheet_basi" // 金融業 _basi
	TWSEAPIBalBD   TWSEAPIDataset = "balance_sheet_bd"   // 證券期貨業 _bd
	TWSEAPIBalFH   TWSEAPIDataset = "balance_sheet_fh"   // 金控業 _fh
	TWSEAPIBalINS  TWSEAPIDataset = "balance_sheet_ins"  // 保險業 _ins
	TWSEAPIBalMIM  TWSEAPIDataset = "balance_sheet_mim"  // 異業 _mim
	// 公司治理與內部人報表（T069-T073）
	TWSEAPIBoardInsuff    TWSEAPIDataset = "board_insuff"     // 董監持股不足法定成數（t187ap08_L，T069）
	TWSEAPIBoardInsuffCon TWSEAPIDataset = "board_insuff_con" // 董監持股不足連續月份（t187ap10_L，T070）
	TWSEAPIBoardPledged   TWSEAPIDataset = "board_pledged"    // 董監質權設定彙總（t187ap09_L，T071）
	TWSEAPIBoardHoldings  TWSEAPIDataset = "board_holdings"   // 董監持股餘額明細（t187ap11_L，T072）
	TWSEAPICEODualRole    TWSEAPIDataset = "ceo_dual_role"    // 董事長兼任總經理（t187ap33_L，T073）
	// 內部人／酬金／大股東報表（T076-T080，T094，T097）
	TWSEAPIDirCompCon     TWSEAPIDataset = "dir_comp_con"     // 合併報表董事酬金（t187ap29_C_L，T076）
	TWSEAPISupCompCon     TWSEAPIDataset = "sup_comp_con"      // 合併報表監察人酬金（t187ap29_D_L，T077）
	TWSEAPISupervisorComp TWSEAPIDataset = "supervisor_comp"   // 監察人酬金（t187ap29_B_L，T111）
	TWSEAPIMeetingAnn     TWSEAPIDataset = "meeting_ann"       // 股東會公告彙總（t187ap38_L，T107/T108 共用）
	TWSEAPIMeetingDates   TWSEAPIDataset = "meeting_dates"     // 股東會日期地點（t187ap41_L，T109）
	TWSEAPIProposalExer   TWSEAPIDataset = "proposal_exercise" // 提案權行使彙總（t187ap35_L，T110）
	// 公發公司／基金（T124，T159，T160）
	TWSEAPIFundBasic      TWSEAPIDataset = "fund_basic"       // 基金基本資料（t187ap47_L，T124）
	TWSEAPIPubBoardHold   TWSEAPIDataset = "pub_board_hold"   // 公發董監持股餘額（t187ap11_P，T159）
	TWSEAPIPubIncCI       TWSEAPIDataset = "pub_income_ci"    // 公發損益表-一般業（t187ap06_X_ci，T160）
	TWSEAPIPubIncBASI     TWSEAPIDataset = "pub_income_basi"  // 金融業
	TWSEAPIPubIncBD       TWSEAPIDataset = "pub_income_bd"    // 證券期貨業
	TWSEAPIPubIncFH       TWSEAPIDataset = "pub_income_fh"    // 金控業
	TWSEAPIPubIncINS      TWSEAPIDataset = "pub_income_ins"   // 保險業
	TWSEAPIPubIncMIM      TWSEAPIDataset = "pub_income_mim"   // 異業
	// 行情歷史與指數補齊（T140，T143-T145，T161，T180-T183）
	TWSEWDMarginInfo    TWSEWebDataset = "margin_info"    // 信用交易統計（MI_MARGN，T140）
	TWSEWDHoliday       TWSEWebDataset = "holiday"        // 市場開休市日期（holidaySchedule，T144）
	TWSEWDRealTimeStats TWSEWebDataset = "realtime_stats" // 每5秒委託成交統計（MI_5MINS，T161）
	TWSEWDTaiwan50      TWSEWebDataset = "taiwan50"       // 臺灣50指數歷史（TAI50I，T181）
	TWSEWDIslandIndex   TWSEWebDataset = "island_index"   // 寶島指數歷史（FRMSA，T182）
	TWSEWDTotalReturn   TWSEWebDataset = "total_return"   // 加權報酬指數歷史（MFI94U，T183）
	// 個股交易統計與權證（T166-T190）
	TWSEWDMonthlyAvgAll  TWSEWebDataset = "monthly_avg_all"  // 月平均價全市場（STOCK_DAY_AVG_ALL，T168）
	TWSEWDStockMonTrade  TWSEWebDataset = "stock_mon_trade"  // 個股月成交資訊（FMSRFK，T171）
	TWSEWDStockYearHis   TWSEWebDataset = "stock_year_his"   // 個股歷年成交（FMNPTK，T173）
	TWSEWDStockYearTrade TWSEWebDataset = "stock_year_trade" // 年度成交資訊全市場（FMNPTK_ALL，T174）
	TWSEAPITopForeign    TWSEAPIDataset = "top_foreign"      // 外資持股前20（MI_QFIIS_sort_20，T185）
	TWSEAPITwseNews      TWSEAPIDataset = "twse_news"        // 證交所新聞（news/newsList，T186）
	TWSEAPIWarrantBasic  TWSEAPIDataset = "warrant_basic"    // 權證基本資料（t187ap37_L，T187）
	TWSEAPIWarrantTrader TWSEAPIDataset = "warrant_trader"   // 權證流動量提供者（t187ap43_L，T189）
	TWSEAPIWarrantIssue  TWSEAPIDataset = "warrant_issue"    // 權證年度發行統計（t187ap36_L，T190）
	TWSEAPIInsiderPreann  TWSEAPIDataset = "insider_preann"   // 內部人持股轉讓事前申報（t187ap12_L，T078）
	TWSEAPIInsiderUntrans TWSEAPIDataset = "insider_untrans"  // 內部人持股未轉讓（t187ap13_L，T079）
	TWSEAPIDirComp        TWSEAPIDataset = "dir_comp"         // 董事酬金（t187ap29_A_L，T080）
	TWSEAPIMajorSharehold TWSEAPIDataset = "major_shareholders" // 持股逾10%大股東（t187ap02_L，T097）
	// 財務與監理報表（T081，T083，T092，T094）
	TWSEAPIEPSStats       TWSEAPIDataset = "eps_stats"        // EPS 統計（t187ap14_L，T083）
	TWSEAPIIncCI          TWSEAPIDataset = "income_ci"        // 綜合損益表-一般業（t187ap06_L_ci，T092）
	TWSEAPIIncBASI        TWSEAPIDataset = "income_basi"      // 金融業
	TWSEAPIIncBD          TWSEAPIDataset = "income_bd"        // 證券期貨業
	TWSEAPIIncFH          TWSEAPIDataset = "income_fh"        // 金控業
	TWSEAPIIncINS         TWSEAPIDataset = "income_ins"       // 保險業
	TWSEAPIIncMIM         TWSEAPIDataset = "income_mim"       // 異業
	TWSEAPIDisclosureVio  TWSEAPIDataset = "disclosure_vio"   // 資訊揭露違法（t187ap23_L，T094）
	// 券商資料（T046-T054）
	TWSEAPIBrokerBasic    TWSEAPIDataset = "broker_basic"     // 證券商基本資料（t187ap18，T046）
	TWSEAPIBrokerBranch   TWSEAPIDataset = "broker_branch"    // 分公司基本資料（OpenData_BRK02，T047）
	TWSEAPIBrokerElec     TWSEAPIDataset = "broker_elec"      // 電子式交易統計（t187ap19，T048）
	TWSEAPIBrokerGender   TWSEAPIDataset = "broker_gender"    // 營業員男女人數（OpenData_BRK01，T049）
	TWSEAPIBrokerHQ       TWSEAPIDataset = "broker_hq"        // 本公司基本資料（brokerList，T050）
	TWSEAPIBrokerIncome   TWSEAPIDataset = "broker_income"    // 券商損益彙總（t187ap21，T051）
	TWSEAPIBrokerMonthly  TWSEAPIDataset = "broker_monthly"   // 券商月報表（t187ap20，T052）
	TWSEAPIBrokerPersonnel TWSEAPIDataset = "broker_personnel" // 從業人員統計（t187ap01，T053）
	TWSEAPIBrokerRegInv   TWSEAPIDataset = "broker_reg_inv"   // 定期定額名單（secRegData，T054）
	TWSEAPISupervisorAck  TWSEAPIDataset = "supervisor_ack"   // 財報監察人承認（t187ap31_L，T084）
	TWSEAPIProfitability  TWSEAPIDataset = "profitability"    // 營益分析（t187ap17_L，T101/T102）
	TWSEAPIAuditVariance  TWSEAPIDataset = "audit_variance"   // 財測查核差異（t187ap16_L，T103）
	TWSEAPIForecastAchv   TWSEAPIDataset = "forecast_achv"    // 財測達成率（t187ap15_L，T104）
)

// 端點路徑（2026-07 實測可用）。www.twse.com.tw 新版主機將 API 掛在 /rwd/ 下；
// 加權指數歷史（indicesReport）與 openapi 仍用舊路徑前綴。
var (
	twseWebBase  = "https://www.twse.com.tw"
	twseAPIBase  = "https://openapi.twse.com.tw/v1"
	twseWebPaths = map[TWSEWebDataset]string{
		TWSEWDDailyK:        "/rwd/afterTrading/STOCK_DAY",
		TWSEWDMonthlyAvg:    "/rwd/afterTrading/STOCK_DAY_AVG",
		TWSEWDMargin:        "/rwd/marginTrading/MI_MARGN",
		TWSEWDInstitutional: "/rwd/fund/T86",
		TWSEWDMarketClose:   "/rwd/afterTrading/MI_INDEX",
		TWSEWDIndexHistory:  "/indicesReport/MI_5MINS_HIST",
		TWSEWDBlockTrades:   "/rwd/block/BFIAUU_d",
		TWSEWDBlockMonthly:  "/rwd/block/BFIAUU_m",
		TWSEWDBlockYearly:   "/rwd/block/BFIAUU_y",
		TWSEWDCrossMarket:     "/exchangeReport/MI_INDEX4",
		TWSEWDDayTradeTargets: "/exchangeReport/TWTB4U",
		TWSEWDSBLVolume:       "/SBL/TWT96U",
		TWSEWDFirstForeign:    "/exchangeReport/STOCK_FIRST",
		TWSEWDMarginRestrict:  "/exchangeReport/BFI84U",
		TWSEWDGainLoss:        "/opendata/twtazu_od",
		TWSEWDOddLot:          "/exchangeReport/TWT53U",
		TWSEWDTradingChanges:  "/exchangeReport/TWT85U",
		TWSEWDPriceChangeLim:  "/exchangeReport/TWT84U",
		TWSEWDNewList5D:       "/exchangeReport/TWT88U",
		TWSEWDSuspDayTradeAnn: "/exchangeReport/TWTBAU1",
		TWSEWDSuspDayTradeHis: "/exchangeReport/TWTBAU2",
		TWSEWDSuspended:       "/exchangeReport/TWTAWU",
		TWSEWDTopVolume:       "/exchangeReport/MI_INDEX20",
		TWSEWDInstiAmounts:    "/rwd/zh/fund/BFI82U",
		TWSEWDTurnoverHistory: "/rwd/zh/afterTrading/FMTQIK",
		TWSEWDSLSBalanceHis:   "/rwd/zh/marginTrading/TWT93U",
		TWSEWDSBLTradesHis:    "/rwd/zh/afterTrading/TWTASU",
		TWSEWDBondRedemption:  "/exchangeReport/BFI61U",
		TWSEWDAbnormal:      "/rwd/announcement/notice",
		TWSEWDForeignQFIIS:  "/rwd/fund/MI_QFIIS",
		TWSEWDAfterHours:    "/exchangeReport/BFT41U",
		// ── 行情歷史與指數補齊（T140-T183）──
		TWSEWDMarginInfo:    "/exchangeReport/MI_MARGN",
		TWSEWDHoliday:       "/holidaySchedule/holidaySchedule",
		TWSEWDRealTimeStats: "/exchangeReport/MI_5MINS",
		TWSEWDTaiwan50:      "/indicesReport/TAI50I",
		TWSEWDIslandIndex:   "/indicesReport/FRMSA",
		TWSEWDTotalReturn:   "/indicesReport/MFI94U",
		TWSEWDMonthlyAvgAll: "/exchangeReport/STOCK_DAY_AVG_ALL",
		TWSEWDStockMonTrade: "/rwd/zh/afterTrading/FMSRFK",
		TWSEWDStockYearHis:  "/rwd/zh/afterTrading/FMNPTK",
		TWSEWDStockYearTrade:"/exchangeReport/FMNPTK_ALL",
	}
	twseAPIPaths = map[TWSEAPIDataset]string{
		TWSEAPIDailyClose:      "/exchangeReport/STOCK_DAY_ALL",
		TWSEAPIForeignHoldings: "/fund/MI_QFIIS_cat",
		TWSEAPIWarrants:        "/opendata/t187ap42_L",
		TWSEAPIIndices:         "/exchangeReport/MI_INDEX",
		TWSEAPIESG:             "/opendata/t187ap46_L_%s", // topic 1..21
		TWSEAPIGovernance:      "/opendata/t187ap32_L",
		TWSEAPIPunish:          "/announcement/punish",
		TWSEAPIValuation:       "/exchangeReport/BWIBBU_ALL", // 上市個股日本益比、殖利率及股價淨值比
		TWSEAPIExDiv:           "/exchangeReport/TWT48U_ALL", // 除權除息預告表
		TWSEAPIDividend:        "/opendata/t187ap45_L",       // 上市公司股利分派情形
		TWSEAPITopForeign:      "/fund/MI_QFIIS_sort_20",  // 外資持股Top20（T185）
		TWSEAPITwseNews:        "/news/newsList",          // 證交所新聞（T186）
		TWSEAPIWarrantBasic:    "/opendata/t187ap37_L",    // 權證基本資料（T187）
		TWSEAPIWarrantTrader:   "/opendata/t187ap43_L",    // 權證流動量提供者（T189）
		TWSEAPIWarrantIssue:    "/opendata/t187ap36_L",    // 權證年度發行統計（T190）
		TWSEAPICumVoting:       "/opendata/t187ap34_L", // 累積投票制選任董監事彙總（T056）
		TWSEAPIOwnScopeHalt:    "/opendata/t187ap26_L", // 經營權異動且營業範圍重大變更停止買賣（T057）
		TWSEAPIOwnScopeTrade:   "/opendata/t187ap27_L", // 經營權異動且營業範圍重大變更列變更交易（T058）
		TWSEAPIScopeChanges:    "/opendata/t187ap25_L", // 營業範圍重大變更公司（T060）
		TWSEAPIIndepDirectors:  "/opendata/t187ap30_L", // 獨立董監事兼任情形彙總（T063）
		TWSEAPIOwnershipChange: "/opendata/t187ap24_L", // 經營權異動公司（T064）
		TWSEAPIBalCI:           "/opendata/t187ap07_L_ci",
		TWSEAPIBalBASI:         "/opendata/t187ap07_L_basi",
		TWSEAPIBalBD:           "/opendata/t187ap07_L_bd",
		TWSEAPIBalFH:           "/opendata/t187ap07_L_fh",
		TWSEAPIBalINS:          "/opendata/t187ap07_L_ins",
		TWSEAPIBalMIM:          "/opendata/t187ap07_L_mim",
		TWSEAPIBoardInsuff:     "/opendata/t187ap08_L",
		TWSEAPIBoardInsuffCon:  "/opendata/t187ap10_L",
		TWSEAPIBoardPledged:    "/opendata/t187ap09_L",
		TWSEAPIBoardHoldings:   "/opendata/t187ap11_L",
		TWSEAPICEODualRole:     "/opendata/t187ap33_L",
		TWSEAPIDirCompCon:      "/opendata/t187ap29_C_L",
		TWSEAPISupCompCon:      "/opendata/t187ap29_D_L",
		TWSEAPISupervisorComp:  "/opendata/t187ap29_B_L",
		TWSEAPIMeetingAnn:      "/opendata/t187ap38_L",
		TWSEAPIMeetingDates:    "/opendata/t187ap41_L",
		TWSEAPIProposalExer:    "/opendata/t187ap35_L",
		TWSEAPIFundBasic:       "/opendata/t187ap47_L",
		TWSEAPIPubBoardHold:    "/opendata/t187ap11_P",
		TWSEAPIPubIncCI:        "/opendata/t187ap06_X_ci",
		TWSEAPIPubIncBASI:      "/opendata/t187ap06_X_basi",
		TWSEAPIPubIncBD:        "/opendata/t187ap06_X_bd",
		TWSEAPIPubIncFH:        "/opendata/t187ap06_X_fh",
		TWSEAPIPubIncINS:       "/opendata/t187ap06_X_ins",
		TWSEAPIPubIncMIM:       "/opendata/t187ap06_X_mim",
		TWSEAPIInsiderPreann:   "/opendata/t187ap12_L",
		TWSEAPIInsiderUntrans:  "/opendata/t187ap13_L",
		TWSEAPIDirComp:         "/opendata/t187ap29_A_L",
		TWSEAPIMajorSharehold:  "/opendata/t187ap02_L",
		TWSEAPIEPSStats:        "/opendata/t187ap14_L",
		TWSEAPIIncCI:           "/opendata/t187ap06_L_ci",
		TWSEAPIIncBASI:         "/opendata/t187ap06_L_basi",
		TWSEAPIIncBD:           "/opendata/t187ap06_L_bd",
		TWSEAPIIncFH:           "/opendata/t187ap06_L_fh",
		TWSEAPIIncINS:          "/opendata/t187ap06_L_ins",
		TWSEAPIIncMIM:          "/opendata/t187ap06_L_mim",
		TWSEAPIDisclosureVio:   "/opendata/t187ap23_L",
		TWSEAPIBrokerBasic:     "/opendata/t187ap18",
		TWSEAPIBrokerBranch:    "/opendata/OpenData_BRK02",
		TWSEAPIBrokerElec:      "/opendata/t187ap19",
		TWSEAPIBrokerGender:    "/opendata/OpenData_BRK01",
		TWSEAPIBrokerHQ:        "/brokerService/brokerList",
		TWSEAPIBrokerIncome:    "/opendata/t187ap21",
		TWSEAPIBrokerMonthly:   "/opendata/t187ap20",
		TWSEAPIBrokerPersonnel: "/opendata/t187ap01",
		TWSEAPIBrokerRegInv:    "/brokerService/secRegData",
		TWSEAPISupervisorAck:   "/opendata/t187ap31_L",
		TWSEAPIProfitability:   "/opendata/t187ap17_L",
		TWSEAPIAuditVariance:   "/opendata/t187ap16_L",
		TWSEAPIForecastAchv:    "/opendata/t187ap15_L",
	}
)

// NewTWSEWebSource 建立 TWSE-WEB 來源（Rate Limit 1 req/2s，§4.4）。
func NewTWSEWebSource(opts ...Option) *TWSEWebSource {
	return &TWSEWebSource{client: NewBaseClient("www.twse.com.tw", opts...)}
}

// TWSEWebSource 實作 SourceContract（§2.2），ID = TWSE_WEB。
type TWSEWebSource struct{ client *BaseClient }

var _ SourceContract = (*TWSEWebSource)(nil)

func (s *TWSEWebSource) ID() string { return model.SourceTWSEWeb }

// URL 建立資料集之官方請求 URL（params 為官方 query 參數，如 date/stockNo）。
func (s *TWSEWebSource) URL(ds TWSEWebDataset, params url.Values) string {
	return twseWebBase + twseWebPaths[ds] + "?response=json&" + params.Encode()
}

func (s *TWSEWebSource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	return s.client.Do(ctx, req)
}

func (s *TWSEWebSource) Validate(raw *RawResponse) error {
	return validateTWSE(raw, s.ID())
}

// Deprecated: v2.1 §6 起轉換集中於 pkg/model/normalize（FromTWSEWeb）；
// 本方法為 v1.3 相容層，遷移時逐步移除（T022）。
func (s *TWSEWebSource) Normalize(raw *RawResponse) ([]byte, error) {
	return normalizeTWSE(raw, s.ID())
}

// NewTWSEAPISource 建立 TWSE-API 來源（Rate Limit 1 req/s，§4.4）。
func NewTWSEAPISource(opts ...Option) *TWSEAPISource {
	return &TWSEAPISource{client: NewBaseClient("openapi.twse.com.tw", opts...)}
}

// TWSEAPISource 實作 SourceContract（§2.2），ID = TWSE_API。
type TWSEAPISource struct{ client *BaseClient }

var _ SourceContract = (*TWSEAPISource)(nil)

func (s *TWSEAPISource) ID() string { return model.SourceTWSEAPI }

// URL 建立資料集之官方請求 URL。esg 資料集以 params["topic"] 指定
// t187ap46_L_<topic>（1..21，預設 1 = 溫室氣體排放）；topic 為路徑參數，
// 不會出現於 query。
func (s *TWSEAPISource) URL(ds TWSEAPIDataset, params url.Values) string {
	path := twseAPIPaths[ds]
	if ds == TWSEAPIESG {
		topic := params.Get("topic")
		if topic == "" {
			topic = "1"
		}
		path = fmt.Sprintf(path, topic)
		params = url.Values{}
	}
	u := twseAPIBase + path
	if params.Encode() != "" {
		u += "?" + params.Encode()
	}
	return u
}

func (s *TWSEAPISource) Fetch(ctx context.Context, req RawRequest) (*RawResponse, error) {
	return s.client.Do(ctx, req)
}

func (s *TWSEAPISource) Validate(raw *RawResponse) error {
	return validateTWSE(raw, s.ID())
}

// Deprecated: v2.1 §6 起轉換集中於 pkg/model/normalize（FromTWSEOpenAPI）；
// 本方法為 v1.3 相容層，遷移時逐步移除（T022）。
func (s *TWSEAPISource) Normalize(raw *RawResponse) ([]byte, error) {
	return normalizeTWSE(raw, s.ID())
}

// twseDatasetOf 依 SourceURL 之 path 判斷資料集（供 Validate/Normalize 分派）。
func twseDatasetOf(raw *RawResponse) (string, error) {
	u, err := url.Parse(raw.SourceURL)
	if err != nil {
		return "", fmt.Errorf("provider: 無法解析來源 URL %q: %w", raw.SourceURL, err)
	}
	p := u.Path
	for ds, path := range twseWebPaths {
		if strings.HasSuffix(p, path) {
			return string(ds), nil
		}
	}
	for ds, path := range twseAPIPaths {
		if ds == TWSEAPIESG {
			if strings.Contains(p, "/t187ap46_L_") {
				return string(ds), nil
			}
			continue
		}
		if strings.HasSuffix(p, path) {
			return string(ds), nil
		}
	}
	return "", fmt.Errorf("provider: 未知 TWSE 資料集路徑 %q", p)
}

// ---------------------------------------------------------------------------
// Validate：schema 檢查（欄位存在性、數值範圍、日期一致性，§2.2）

// validateTWSE 依資料集執行 schema 檢查。非 2xx 已由 BaseClient 擋下；
// 官方「查無資料」回應（stat 含「沒有符合條件」）視為合法空資料。
func validateTWSE(raw *RawResponse, sourceID string) error {
	ds, err := twseDatasetOf(raw)
	if err != nil {
		return err
	}
	body := raw.Body
	if len(body) == 0 {
		return fmt.Errorf("provider: %s 空 body", ds)
	}
	// openapi 資料集為頂層 JSON 陣列
	if isJSONArray(body) {
		var rows []json.RawMessage
		if err := json.Unmarshal(body, &rows); err != nil {
			return fmt.Errorf("provider: %s 回應 JSON 解析失敗: %w", ds, err)
		}
		return validateOpenAPIList(raw, rows)
	}
	var envelope struct {
		Stat  string `json:"stat"`
		Total int    `json:"total"`
		Date  string `json:"date"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("provider: %s 回應 JSON 解析失敗: %w", ds, err)
	}
	// 官方「查無資料」回應
	if envelope.Stat != "OK" && strings.Contains(envelope.Stat, "沒有符合條件") {
		return nil
	}
	if !strings.EqualFold(envelope.Stat, "OK") {
		return fmt.Errorf("provider: %s 官方回應異常 stat=%q", ds, envelope.Stat)
	}
	if isTablesDataset(ds) {
		if err := validateDateConsistency(ds, raw.SourceURL, envelope.Date); err != nil {
			return err
		}
		return validateTables(ds, body)
	}
	// 日期一致性：請求日期（URL query）與回應日期須一致（部分資料集為同月）
	if err := validateDateConsistency(ds, raw.SourceURL, envelope.Date); err != nil {
		return err
	}
	var fld struct {
		Fields  []string        `json:"fields"`
		DataRaw json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &fld); err != nil {
		return fmt.Errorf("provider: %s fields/data 解析失敗: %w", ds, err)
	}
	if err := validateRequiredFields(ds, fld.Fields); err != nil {
		return err
	}
	rows, err := rawRows(fld.DataRaw)
	if err != nil {
		return fmt.Errorf("provider: %s data 非列式陣列: %w", ds, err)
	}
	for i, row := range rows {
		if len(row) != len(fld.Fields) {
			return fmt.Errorf("provider: %s 第 %d 列欄位數 %d ≠ fields %d",
				ds, i, len(row), len(fld.Fields))
		}
	}
	return nil
}

// isJSONArray 判斷 body 首個非空白字元是否為 '['。
func isJSONArray(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// rawRows 將 data（JSON 陣列之陣列）轉為 [][]string；
// 官方少數欄位為 JSON number（如 notice 之「編號」），一律轉字串。
func rawRows(data json.RawMessage) ([][]string, error) {
	var cells [][]json.RawMessage
	if err := json.Unmarshal(data, &cells); err != nil {
		return nil, err
	}
	out := make([][]string, len(cells))
	for i, row := range cells {
		out[i] = make([]string, len(row))
		for j, c := range row {
			var s string
			if json.Unmarshal(c, &s) != nil {
				s = strings.Trim(string(c), `"`)
			}
			out[i][j] = s
		}
	}
	return out, nil
}

// isTablesDataset 判斷資料集是否為「tables」結構（margin/market_close/block_trades）。
func isTablesDataset(ds string) bool {
	switch ds {
	case "margin", "market_close", "block_trades", "margin_info", "stock_mon_trade", "stock_year_his":
		return true
	}
	return false
}

// validateTables 檢查 tables 結構資料集：所有表格之列欄位數一致性 +
// 目標表格（title 過濾）之必備欄位。
func validateTables(ds string, body []byte) error {
	var t struct {
		Tables []struct {
			Title   string          `json:"title"`
			Fields  []string        `json:"fields"`
			DataRaw json.RawMessage `json:"data"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(body, &t); err != nil {
		return fmt.Errorf("provider: %s tables 解析失敗: %w", ds, err)
	}
	if len(t.Tables) == 0 {
		return fmt.Errorf("provider: %s 無 tables", ds)
	}
	required := map[string]struct {
		title  string
		fields []string
	}{
		"margin":       {"融資融券彙總", []string{"代號", "名稱", "買進", "前日餘額", "今日餘額"}},
		"market_close": {"每日收盤行情", []string{"證券代號", "證券名稱", "成交股數", "成交金額", "收盤價"}},
		"block_trades": {"鉅額交易", []string{"日期", "交易別", "類別", "成交股數", "成交金額"}},
	}
	for _, table := range t.Tables {
		// 容忍官方尾表瑕疵：MI_INDEX type=ALL 等回應末尾偶有
		// title 為空、data 為 null 之空表，跳過不驗證。
		if len(table.Title) == 0 || len(table.DataRaw) == 0 || string(table.DataRaw) == "null" {
			continue
		}
		rows, err := rawRows(table.DataRaw)
		if err != nil {
			return fmt.Errorf("provider: %s 表格 %q data 非列式陣列: %w", ds, table.Title, err)
		}
		for i, row := range rows {
			if len(row) != len(table.Fields) {
				return fmt.Errorf("provider: %s 表格 %q 第 %d 列欄位數 %d ≠ fields %d",
					ds, table.Title, i, len(row), len(table.Fields))
			}
		}
	}
	want := required[ds]
	found := false
	for _, table := range t.Tables {
		if !strings.Contains(table.Title, want.title) {
			continue
		}
		found = true
		for _, f := range want.fields {
			if !containsString(table.Fields, f) {
				return fmt.Errorf("provider: %s 目標表格 %q 缺少必備欄位 %q（官方格式可能變更）",
					ds, table.Title, f)
			}
		}
	}
	if !found {
		return fmt.Errorf("provider: %s 找不到目標表格 %q（官方格式可能變更）", ds, want.title)
	}
	return nil
}

// validateOpenAPIList 檢查頂層陣列資料集之列結構（欄位存在性由各
// Normalize 實作以 rowToMap 嚴格把關；此處只檢查列為物件）。
func validateOpenAPIList(raw *RawResponse, rows []json.RawMessage) error {
	for i, r := range rows {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(r, &m); err != nil {
			return fmt.Errorf("provider: 第 %d 列非物件: %w", i, err)
		}
		if len(m) == 0 {
			return fmt.Errorf("provider: 第 %d 列為空物件", i)
		}
	}
	return nil
}

// validateRequiredFields 檢查 www envelope 資料集之必備欄位（2026-07 實測）。
func validateRequiredFields(ds string, fields []string) error {
	required := map[string][]string{
		"daily_k":         {"日期", "成交股數", "成交金額", "開盤價", "最高價", "最低價", "收盤價"},
		"monthly_avg":     {"日期", "收盤價"},
		"margin":          {"代號", "名稱", "買進", "前日餘額", "今日餘額"},
		"institutional":   {"證券代號", "證券名稱", "三大法人買賣超股數"},
		"market_close":    {"證券代號", "證券名稱", "成交股數", "成交金額", "收盤價"},
		"index_history":   {"日期", "開盤指數", "最高指數", "最低指數", "收盤指數"},
		"block_trades":    {"日期", "交易別", "類別", "成交股數", "成交金額"},
		"abnormal_volume": {"編號", "證券代號", "證券名稱", "累計次數"},
		"qfiis":           {"證券代號", "證券名稱", "發行股數", "全體外資及陸資持有股數", "全體外資及陸資持股比率"},
	}
	for _, f := range required[ds] {
		if !containsString(fields, f) {
			return fmt.Errorf("provider: %s 缺少必備欄位 %q（官方格式可能變更）", ds, f)
		}
	}
	return nil
}

// validateDateConsistency 檢查回應日期與請求日期一致性：
// 每日資料集須同日；整月資料集（index_history/block_trades）須同月。
func validateDateConsistency(ds, sourceURL, respDate string) error {
	reqDate := queryDate(sourceURL)
	if reqDate == "" || respDate == "" || len(reqDate) != 8 || len(respDate) != 8 {
		return nil // 無日期可比對（openapi 全量資料集等）
	}
	if ds == "index_history" || ds == "block_trades" {
		if reqDate[:6] != respDate[:6] {
			return fmt.Errorf("provider: %s 回應月份 %s 與請求 %s 不符", ds, respDate, reqDate)
		}
		return nil
	}
	if reqDate != respDate {
		return fmt.Errorf("provider: %s 回應日期 %s 與請求 %s 不符", ds, respDate, reqDate)
	}
	return nil
}

// queryDate 取 URL query 之 date 參數（YYYYMMDD）。
func queryDate(sourceURL string) string {
	u, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("date")
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Normalize：依資料集將 raw 轉為 Normalized Model（JSON），單位 元/股/%

func normalizeTWSE(raw *RawResponse, sourceID string) ([]byte, error) {
	ds, err := twseDatasetOf(raw)
	if err != nil {
		return nil, err
	}
	var out interface{}
	switch ds {
	case "daily_k":
		out, err = normalizeDailyK(raw)
	case "monthly_avg":
		out, err = normalizeMonthlyAvg(raw)
	case "margin":
		out, err = normalizeMargin(raw)
	case "institutional":
		out, err = normalizeInstitutional(raw)
	case "market_close":
		out, err = normalizeMarketClose(raw)
	case "index_history":
		out, err = normalizeIndexHistory(raw)
	case "block_trades":
		out, err = normalizeBlockTrades(raw)
	case "block_monthly":
		out, err = normalizeBlockTrades(raw)
	case "block_yearly":
		out, err = normalizeBlockTrades(raw)
	case "cross_market":
		out, err = normalizeWebTable(raw)
	case "day_trade_targets":
		out, err = normalizeWebTable(raw)
	case "sbl_volume":
		out, err = normalizeWebTable(raw)
	case "first_foreign":
		out, err = normalizeWebTable(raw)
	case "margin_restrict":
		out, err = normalizeWebTable(raw)
	case "odd_lot":
		out, err = normalizeWebTable(raw)
	case "trading_changes":
		out, err = normalizeWebTable(raw)
	case "price_change_lim":
		out, err = normalizeWebTable(raw)
	case "new_list_5d":
		out, err = normalizeWebTable(raw)
	case "suspend_daytrade_ann":
		out, err = normalizeWebTable(raw)
	case "suspend_daytrade_his":
		out, err = normalizeWebTable(raw)
	case "suspended":
		out, err = normalizeWebTable(raw)
	case "top_volume":
		out, err = normalizeWebTable(raw)
	case "gain_loss":
		out, err = normalizePassthroughArray(raw)
	case "insti_amounts":
		out, err = normalizeWebTable(raw)
	case "turnover_history":
		out, err = normalizeWebTable(raw)
	case "sbl_balance_his":
		out, err = normalizeWebTable(raw)
	case "bond_redemption":
		out, err = normalizeWebTable(raw)
	case "cum_voting":
		out, err = normalizePassthroughArray(raw)
	case "own_scope_halt":
		out, err = normalizePassthroughArray(raw)
	case "own_scope_trade":
		out, err = normalizePassthroughArray(raw)
	case "scope_changes":
		out, err = normalizePassthroughArray(raw)
	case "indep_directors":
		out, err = normalizePassthroughArray(raw)
	case "ownership_change":
		out, err = normalizePassthroughArray(raw)
	case "balance_sheet_ci", "balance_sheet_basi", "balance_sheet_bd",
		"balance_sheet_fh", "balance_sheet_ins", "balance_sheet_mim":
		out, err = normalizePassthroughArray(raw)
	case "board_insuff", "board_insuff_con", "board_pledged",
		"board_holdings", "ceo_dual_role":
		out, err = normalizePassthroughArray(raw)
	case "dir_comp_con", "sup_comp_con", "insider_preann",
		"insider_untrans", "dir_comp", "major_shareholders":
		out, err = normalizePassthroughArray(raw)
	case "supervisor_comp", "meeting_ann", "meeting_dates", "proposal_exercise":
		out, err = normalizePassthroughArray(raw)
	case "top_foreign", "twse_news", "warrant_basic",
		"warrant_trader", "warrant_issue":
		out, err = normalizePassthroughArray(raw)
	case "fund_basic", "pub_board_hold", "pub_income_ci", "pub_income_basi",
		"pub_income_bd", "pub_income_fh", "pub_income_ins", "pub_income_mim":
		out, err = normalizePassthroughArray(raw)
	case "margin_info":
		out, err = normalizeWebTablesList(raw)
	case "holiday", "realtime_stats",
		"taiwan50", "island_index", "total_return":
		out, err = normalizeWebTable(raw)
	case "monthly_avg_all", "stock_year_trade":
		out, err = normalizeWebTable(raw)
	case "stock_mon_trade", "stock_year_his":
		out, err = normalizeWebTablesList(raw)
	case "eps_stats", "income_ci", "income_basi", "income_bd",
		"income_fh", "income_ins", "income_mim", "disclosure_vio":
		out, err = normalizePassthroughArray(raw)
	case "broker_basic", "broker_branch", "broker_elec", "broker_gender",
		"broker_hq", "broker_income", "broker_monthly", "broker_personnel",
		"broker_reg_inv":
		out, err = normalizePassthroughArray(raw)
	case "supervisor_ack":
		out, err = normalizePassthroughArray(raw)
	case "profitability":
		out, err = normalizePassthroughArray(raw)
	case "audit_variance", "forecast_achv":
		out, err = normalizePassthroughArray(raw)
	case "sbl_trades_his":
		out, err = normalizeWebTable(raw)
	case "abnormal_volume":
		out, err = normalizeAbnormalVolume(raw)
	case "after_hours":
		out, err = normalizeAfterHours(raw)
	case "qfiis":
		out, err = normalizeQFIIS(raw)
	case "punish":
		out, err = normalizePunish(raw)
	case "daily_close":
		out, err = normalizeDailyClose(raw)
	case "foreign_holdings":
		out, err = normalizeForeignHoldings(raw)
	case "warrants":
		out, err = normalizeWarrants(raw)
	case "indices":
		out, err = normalizeIndices(raw)
	case "esg":
		out, err = normalizeESG(raw)
	case "company_governance":
		out, err = normalizeGovernance(raw)
	case "valuation":
		out, err = normalizeValuation(raw)
	case "ex_div":
		out, err = normalizeExDiv(raw)
	case "dividend":
		out, err = normalizeDividend(raw)
	default:
		return nil, fmt.Errorf("provider: 不支援資料集 %q", ds)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// parseRow 依 fields 將資料列轉為欄位名 → 值 map。
func parseRow(fields, row []string) map[string]string {
	m := make(map[string]string, len(fields))
	for i, f := range fields {
		if i < len(row) {
			m[f] = row[i]
		}
	}
	return m
}

// rowsOf 取 www envelope 之 data（列式）。「查無資料」時回傳 nil。
func rowsOf(raw *RawResponse) (fields []string, rows [][]string, err error) {
	var envelope struct {
		Stat    string          `json:"stat"`
		Total   int             `json:"total"`
		Fields  []string        `json:"fields"`
		DataRaw json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw.Body, &envelope); err != nil {
		return nil, nil, fmt.Errorf("provider: envelope JSON 解析失敗: %w", err)
	}
	if envelope.Stat != "OK" {
		if strings.Contains(envelope.Stat, "沒有符合條件") {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("provider: 官方回應異常 stat=%q", envelope.Stat)
	}
	rows, err = rawRows(envelope.DataRaw)
	if err != nil {
		return nil, nil, fmt.Errorf("provider: envelope data 非列式陣列: %w", err)
	}
	return envelope.Fields, rows, nil
}

// tablesOf 取 www tables 結構資料集（margin/market_close/block_trades）
// 之第一個符合 titleContains 之表格；「查無資料」時回傳 nil。
func tablesOf(raw *RawResponse, titleContains string) (title string, fields []string, rows [][]string, err error) {
	var envelope struct {
		Stat   string `json:"stat"`
		Total  int    `json:"total"`
		Tables []struct {
			Title   string          `json:"title"`
			Fields  []string        `json:"fields"`
			DataRaw json.RawMessage `json:"data"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(raw.Body, &envelope); err != nil {
		return "", nil, nil, fmt.Errorf("provider: tables JSON 解析失敗: %w", err)
	}
	if envelope.Stat != "OK" {
		if strings.Contains(envelope.Stat, "沒有符合條件") {
			return "", nil, nil, nil
		}
		return "", nil, nil, fmt.Errorf("provider: 官方回應異常 stat=%q", envelope.Stat)
	}
	for _, t := range envelope.Tables {
		if titleContains == "" || strings.Contains(t.Title, titleContains) {
			r, err := rawRows(t.DataRaw)
			if err != nil {
				return "", nil, nil, fmt.Errorf("provider: 表格 %q data 非列式陣列: %w", t.Title, err)
			}
			return t.Title, t.Fields, r, nil
		}
	}
	return "", nil, nil, fmt.Errorf("provider: 找不到表格 %q", titleContains)
}

// rowToMap 將 openapi 列式資料轉為欄位名 → 值 map。
func rowToMap(row map[string]json.RawMessage) map[string]string {
	m := make(map[string]string, len(row))
	for k, v := range row {
		var s string
		if json.Unmarshal(v, &s) == nil {
			m[k] = s
		}
	}
	return m
}

// parseROCDate 解析民國年日期：支援 "115/07/01"、"115.07.31"、"1150730"。
func parseROCDate(s string) (time.Time, error) {
	t := strings.TrimSpace(s)
	var y, m, d int
	switch {
	case strings.Contains(t, "/"):
		if _, err := fmt.Sscanf(t, "%d/%d/%d", &y, &m, &d); err != nil {
			return time.Time{}, fmt.Errorf("provider: 民國日期 %q 解析失敗: %w", s, err)
		}
	case strings.Contains(t, "."):
		if _, err := fmt.Sscanf(t, "%d.%d.%d", &y, &m, &d); err != nil {
			return time.Time{}, fmt.Errorf("provider: 民國日期 %q 解析失敗: %w", s, err)
		}
	case len(t) == 7:
		if _, err := fmt.Sscanf(t, "%3d%2d%2d", &y, &m, &d); err != nil {
			return time.Time{}, fmt.Errorf("provider: 民國日期 %q 解析失敗: %w", s, err)
		}
	default:
		return time.Time{}, fmt.Errorf("provider: 民國日期 %q 格式未知", s)
	}
	if y < 100 || m < 1 || m > 12 || d < 1 || d > 31 {
		return time.Time{}, fmt.Errorf("provider: 民國日期 %q 範圍異常", s)
	}
	return time.Date(y+1911, time.Month(m), d, 0, 0, 0, 0, model.Taipei()), nil
}

// parseCommaInt 解析含千分位之整數；空字串/"-" 回傳 ok=false。
func parseCommaInt(s string) (int64, bool) {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" || t == "-----" {
		return 0, false
	}
	v, err := strconv.ParseInt(strings.ReplaceAll(t, ",", ""), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// parseCommaFloat 解析含千分位之浮點數；空字串/"-" 回傳 ok=false。
func parseCommaFloat(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" || t == "-----" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(t, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	return math.Round(f*100) / 100, true
}

func commaIntOrZero(s string) int64 {
	v, ok := parseCommaInt(s)
	if !ok {
		return 0
	}
	return v
}

func commaFloatOrZero(s string) float64 {
	v, ok := parseCommaFloat(s)
	if !ok {
		return 0
	}
	return v
}

// commaDecimalToInt 解析含小數之整數欄位（如權證成交金額 "400.00"）。
func commaDecimalToInt(s string) int64 {
	f, ok := parseCommaFloat(s)
	if !ok {
		return 0
	}
	return int64(math.Round(f))
}

// ---------------------------------------------------------------------------
// 個股日 K（TWSE-WEB）：Normalize 回傳 []model.Candle（元/股/元）。
// period=day|week|month 由 URL query 指定（週/月由日線聚合）。
// adjust=true 時 URL 附 adjust=Y（官方現行端點已忽略，見檔頭說明）。

func normalizeDailyK(raw *RawResponse) ([]model.Candle, error) {
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	q, _ := url.Parse(raw.SourceURL)
	params := q.Query()
	period := params.Get("period")
	if period == "" {
		period = "day"
	}
	candles := make([]model.Candle, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		ts, err := parseROCDate(m["日期"])
		if err != nil {
			continue // 個別列容錯（§12.4 官方格式雜訊）
		}
		c := model.Candle{
			Timestamp: model.FormatDate(ts),
			Open:      commaFloatOrZero(m["開盤價"]),
			High:      commaFloatOrZero(m["最高價"]),
			Low:       commaFloatOrZero(m["最低價"]),
			Close:     commaFloatOrZero(m["收盤價"]),
			Volume:    commaIntOrZero(m["成交股數"]), // 已為「股」（2026-07 實測）
			Amount:    commaIntOrZero(m["成交金額"]), // 已為「元」（2026-07 實測）
		}
		if c.Timestamp == "" || c.Close <= 0 {
			continue
		}
		candles = append(candles, c)
	}
	switch period {
	case "week":
		candles = aggregateCandles(candles, "week")
	case "month":
		candles = aggregateCandles(candles, "month")
	case "day":
	default:
		return nil, fmt.Errorf("provider: 不支援 period=%q（day/week/month）", period)
	}
	if len(candles) == 0 {
		return nil, fmt.Errorf("provider: daily_k 無有效資料列")
	}
	return candles, nil
}

// aggregateCandles 將日線聚合為週/月 K（§5.3：O/H/L/C/量/值）。
func aggregateCandles(daily []model.Candle, kind string) []model.Candle {
	type key string
	keyOf := func(ts string) key {
		if kind == "week" {
			t, err := model.ParseDate(ts)
			if err != nil {
				return key(ts)
			}
			// 週一起始之 ISO 週
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			start := t.AddDate(0, 0, -(wd - 1))
			return key("wk:" + model.FormatDate(start))
		}
		return key(ts[:7])
	}
	var (
		ks    []key
		byKey = map[key]*model.Candle{}
	)
	for _, c := range daily {
		k := keyOf(c.Timestamp)
		if _, ok := byKey[k]; !ok {
			ks = append(ks, k)
			cc := c
			byKey[k] = &cc
			continue
		}
		g := byKey[k]
		if c.High > g.High {
			g.High = c.High
		}
		if c.Low < g.Low {
			g.Low = c.Low
		}
		g.Close = c.Close
		g.Volume += c.Volume
		g.Amount += c.Amount
	}
	out := make([]model.Candle, 0, len(ks))
	for _, k := range ks {
		g := byKey[k]
		if kind == "week" {
			g.Timestamp = strings.TrimPrefix(string(k), "wk:")
		} else {
			g.Timestamp = string(k) + "-01" // 月份首日代表月 K
		}
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out
}

// ---------------------------------------------------------------------------
// 月均價（TWSE-WEB）：Normalize 回傳 []MonthlyAvgRow。
// 官方 STOCK_DAY_AVG 目前僅回傳 日期/收盤價（title 為「日收盤價及月平均
// 收盤價」，2026-07 實測無月平均欄位），月平均收盤價由本檔計算輸出。

// MonthlyAvgRow 為個股日收盤價及月平均收盤價。
type MonthlyAvgRow struct {
	Date     string  `json:"date"`      // YYYY-MM-DD
	Close    float64 `json:"close"`     // 收盤價（元）
	MonthAvg float64 `json:"month_avg"` // 當月平均收盤價（元）
}

func normalizeMonthlyAvg(raw *RawResponse) ([]MonthlyAvgRow, error) {
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]MonthlyAvgRow, 0, len(rows))
	var sum float64
	officialAvg := 0.0
	for _, row := range rows {
		m := parseRow(fields, row)
		// 官方末列為「月平均收盤價」彙總列（實測 2026-07-31）
		if strings.TrimSpace(m["日期"]) == "月平均收盤價" {
			if v, ok := parseCommaFloat(m["收盤價"]); ok {
				officialAvg = v
			}
			continue
		}
		ts, err := parseROCDate(m["日期"])
		if err != nil {
			continue
		}
		close_, ok := parseCommaFloat(m["收盤價"])
		if !ok {
			continue
		}
		sum += close_
		out = append(out, MonthlyAvgRow{
			Date:  model.FormatDate(ts),
			Close: close_,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: monthly_avg 無有效資料列")
	}
	avg := math.Round(sum/float64(len(out))*100) / 100
	if officialAvg > 0 {
		avg = officialAvg // 官方彙總列優先
	}
	for i := range out {
		out[i].MonthAvg = avg
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 融資融券（TWSE-WEB）：MI_MARGN 彙總表。官方欄位以「張」計（實測：
// 「信用交易統計」表之「融資(交易單位)」即為張），一律 ×1000 → 股。

// MarginRow 為單一股票之融資融券餘額（§5.1：股）。
type MarginRow struct {
	Code              string `json:"code"`
	Name              string `json:"name"`
	MarginBuy         int64  `json:"margin_buy"`          // 融資買進（股）
	MarginSell        int64  `json:"margin_sell"`         // 融資賣出（股）
	MarginCashRedeem  int64  `json:"margin_cash_redeem"`  // 融資現金償還（股）
	MarginPrevBalance int64  `json:"margin_prev_balance"` // 融資前日餘額（股）
	MarginBalance     int64  `json:"margin_balance"`      // 融資今日餘額（股）
	MarginLimit       int64  `json:"margin_limit"`        // 融資限額（股）
	ShortBuy          int64  `json:"short_buy"`           // 融券買進（股）
	ShortSell         int64  `json:"short_sell"`          // 融券賣出（股）
	ShortCashRedeem   int64  `json:"short_cash_redeem"`   // 融券現券償還（股）
	ShortPrevBalance  int64  `json:"short_prev_balance"`  // 融券前日餘額（股）
	ShortBalance      int64  `json:"short_balance"`       // 融券今日餘額（股）
	ShortLimit        int64  `json:"short_limit"`         // 融券限額（股）
	Offset            int64  `json:"offset"`              // 資券互抵（股）
	Note              string `json:"note,omitempty"`
}

func normalizeMargin(raw *RawResponse) ([]MarginRow, error) {
	_, fields, rows, err := tablesOf(raw, "融資融券彙總")
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []MarginRow{}, nil // 官方查無資料
	}
	// 官方欄位含重複名稱（融資組 6 欄 + 融券組 6 欄）：以首次/第二次出現位置區分
	first := map[string]int{}
	second := map[string]int{}
	counts := map[string]int{}
	for i, f := range fields {
		counts[f]++
		if counts[f] == 1 {
			first[f] = i
		} else if counts[f] == 2 {
			second[f] = i
		}
	}
	out := make([]MarginRow, 0, len(rows))
	for _, row := range rows {
		get := func(idx map[string]int, f string) string {
			if i, ok := idx[f]; ok && i < len(row) {
				return row[i]
			}
			return ""
		}
		r := MarginRow{
			Code: strings.TrimSpace(get(first, "代號")),
			Name: strings.TrimSpace(get(first, "名稱")),
			Note: strings.TrimSpace(get(first, "註記")),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		r.MarginBuy = model.LotsToShares(commaIntOrZero(get(first, "買進")))
		r.MarginSell = model.LotsToShares(commaIntOrZero(get(first, "賣出")))
		r.MarginCashRedeem = model.LotsToShares(commaIntOrZero(get(first, "現金償還")))
		r.MarginPrevBalance = model.LotsToShares(commaIntOrZero(get(first, "前日餘額")))
		r.MarginBalance = model.LotsToShares(commaIntOrZero(get(first, "今日餘額")))
		r.MarginLimit = model.LotsToShares(commaIntOrZero(get(first, "次一營業日限額")))
		r.ShortBuy = model.LotsToShares(commaIntOrZero(get(second, "買進")))
		r.ShortSell = model.LotsToShares(commaIntOrZero(get(second, "賣出")))
		r.ShortCashRedeem = model.LotsToShares(commaIntOrZero(get(second, "現券償還")))
		r.ShortPrevBalance = model.LotsToShares(commaIntOrZero(get(second, "前日餘額")))
		r.ShortBalance = model.LotsToShares(commaIntOrZero(get(second, "今日餘額")))
		r.ShortLimit = model.LotsToShares(commaIntOrZero(get(second, "次一營業日限額")))
		r.Offset = model.LotsToShares(commaIntOrZero(get(first, "資券互抵")))
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: margin 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 三大法人買賣超（TWSE-WEB）：T86 日報。官方全部欄位皆為「股數」
// （2026-07 實測 title 明示「單位：股」），無需換算。

// InstitutionalRow 為單一股票之三大法人買賣超（股）。
type InstitutionalRow struct {
	Code              string `json:"code"`
	Name              string `json:"name"`
	ForeignBuy        int64  `json:"foreign_buy"`         // 外陸資買進股數（不含外資自營商）
	ForeignSell       int64  `json:"foreign_sell"`        // 外陸資賣出股數（不含外資自營商）
	ForeignNet        int64  `json:"foreign_net"`         // 外陸資買賣超股數
	ForeignDealerBuy  int64  `json:"foreign_dealer_buy"`  // 外資自營商買進股數
	ForeignDealerSell int64  `json:"foreign_dealer_sell"` // 外資自營商賣出股數
	ForeignDealerNet  int64  `json:"foreign_dealer_net"`  // 外資自營商買賣超股數
	InvestmentBuy     int64  `json:"investment_buy"`      // 投信買進股數
	InvestmentSell    int64  `json:"investment_sell"`     // 投信賣出股數
	InvestmentNet     int64  `json:"investment_net"`      // 投信買賣超股數
	DealerNet         int64  `json:"dealer_net"`          // 自營商買賣超股數
	DealerSelfBuy     int64  `json:"dealer_self_buy"`     // 自營商買進股數（自行買賣）
	DealerSelfSell    int64  `json:"dealer_self_sell"`    // 自營商賣出股數（自行買賣）
	DealerSelfNet     int64  `json:"dealer_self_net"`     // 自營商買賣超股數（自行買賣）
	DealerHedgeBuy    int64  `json:"dealer_hedge_buy"`    // 自營商買進股數（避險）
	DealerHedgeSell   int64  `json:"dealer_hedge_sell"`   // 自營商賣出股數（避險）
	DealerHedgeNet    int64  `json:"dealer_hedge_net"`    // 自營商買賣超股數（避險）
	TotalNet          int64  `json:"total_net"`           // 三大法人買賣超股數
}

func normalizeInstitutional(raw *RawResponse) ([]InstitutionalRow, error) {
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]InstitutionalRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		r := InstitutionalRow{
			Code: strings.TrimSpace(m["證券代號"]),
			Name: strings.TrimSpace(m["證券名稱"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		r.ForeignBuy = commaIntOrZero(m["外陸資買進股數(不含外資自營商)"])
		r.ForeignSell = commaIntOrZero(m["外陸資賣出股數(不含外資自營商)"])
		r.ForeignNet = commaIntOrZero(m["外陸資買賣超股數(不含外資自營商)"])
		r.ForeignDealerBuy = commaIntOrZero(m["外資自營商買進股數"])
		r.ForeignDealerSell = commaIntOrZero(m["外資自營商賣出股數"])
		r.ForeignDealerNet = commaIntOrZero(m["外資自營商買賣超股數"])
		r.InvestmentBuy = commaIntOrZero(m["投信買進股數"])
		r.InvestmentSell = commaIntOrZero(m["投信賣出股數"])
		r.InvestmentNet = commaIntOrZero(m["投信買賣超股數"])
		r.DealerNet = commaIntOrZero(m["自營商買賣超股數"])
		r.DealerSelfBuy = commaIntOrZero(m["自營商買進股數(自行買賣)"])
		r.DealerSelfSell = commaIntOrZero(m["自營商賣出股數(自行買賣)"])
		r.DealerSelfNet = commaIntOrZero(m["自營商買賣超股數(自行買賣)"])
		r.DealerHedgeBuy = commaIntOrZero(m["自營商買進股數(避險)"])
		r.DealerHedgeSell = commaIntOrZero(m["自營商賣出股數(避險)"])
		r.DealerHedgeNet = commaIntOrZero(m["自營商買賣超股數(避險)"])
		r.TotalNet = commaIntOrZero(m["三大法人買賣超股數"])
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: institutional 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 全市場收盤行情（TWSE-WEB）：MI_INDEX type=ALL 大型 payload（2026-07 實測
// 4.2MB / 31,267 列）。Normalize 僅取「每日收盤行情」表並輸出精簡欄位
// （§12 JSON 最小化：省略最後揭示買賣價量之冗餘）。

// MarketCloseRow 為單一股票之收盤行情（股/元）。
type MarketCloseRow struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Volume      int64   `json:"volume"`               // 成交股數（股）
	Transaction int64   `json:"transaction"`          // 成交筆數
	Amount      int64   `json:"amount"`               // 成交金額（元）
	Open        float64 `json:"open"`                 // 開盤價（元）
	High        float64 `json:"high"`                 // 最高價（元）
	Low         float64 `json:"low"`                  // 最低價（元）
	Close       float64 `json:"close"`                // 收盤價（元）
	ChangeDir   string  `json:"change_dir,omitempty"` // 漲跌(+/-)
	Change      float64 `json:"change"`               // 漲跌價差（元）
	PE          float64 `json:"pe"`                   // 本益比
}

func normalizeMarketClose(raw *RawResponse) ([]MarketCloseRow, error) {
	_, fields, rows, err := tablesOf(raw, "每日收盤行情")
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []MarketCloseRow{}, nil
	}
	var out []MarketCloseRow
	for _, row := range rows {
		m := parseRow(fields, row)
		r := MarketCloseRow{
			Code:        strings.TrimSpace(m["證券代號"]),
			Name:        strings.TrimSpace(m["證券名稱"]),
			Volume:      commaIntOrZero(m["成交股數"]), // 股
			Transaction: commaIntOrZero(m["成交筆數"]),
			Amount:      commaIntOrZero(m["成交金額"]), // 元
			Open:        commaFloatOrZero(m["開盤價"]),
			High:        commaFloatOrZero(m["最高價"]),
			Low:         commaFloatOrZero(m["最低價"]),
			Close:       commaFloatOrZero(m["收盤價"]),
			ChangeDir:   strings.TrimSpace(m["漲跌(+/-)"]),
			Change:      commaFloatOrZero(m["漲跌價差"]),
			PE:          commaFloatOrZero(m["本益比"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: market_close 無「每日收盤行情」表或無有效列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 加權指數歷史（TWSE-WEB）：MI_5MINS_HIST 回傳請求月份之每日
// 發行量加權股價指數 OHLC（2026-07 實測：date=YYYYMMDD → 該月全部交易日）。

// IndexRow 為加權指數歷史之一日 OHLC。
type IndexRow struct {
	Date  string  `json:"date"`  // YYYY-MM-DD
	Open  float64 `json:"open"`  // 開盤指數
	High  float64 `json:"high"`  // 最高指數
	Low   float64 `json:"low"`   // 最低指數
	Close float64 `json:"close"` // 收盤指數
}

func normalizeIndexHistory(raw *RawResponse) ([]IndexRow, error) {
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]IndexRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		ts, err := parseROCDate(m["日期"])
		if err != nil {
			continue
		}
		r := IndexRow{
			Date:  model.FormatDate(ts),
			Open:  commaFloatOrZero(m["開盤指數"]),
			High:  commaFloatOrZero(m["最高指數"]),
			Low:   commaFloatOrZero(m["最低指數"]),
			Close: commaFloatOrZero(m["收盤指數"]),
		}
		if r.Close <= 0 {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: index_history 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 鉅額交易（TWSE-WEB）：BFIAUU_d 回傳請求月份之鉅額交易日成交量值
// （2026-07 實測：date=YYYYMMDD → 該月全部鉅額交易）。單位為股/元。

// BlockTradeRow 為一筆鉅額交易統計。
type BlockTradeRow struct {
	Date        string  `json:"date"`         // YYYY-MM-DD
	TradeType   string  `json:"trade_type"`   // 交易別（逐筆交易/配對交易/盤後定價）
	Class       string  `json:"class"`        // 類別（單一證券/股票組合）
	Volume      int64   `json:"volume"`       // 成交股數（股）
	VolumeShare float64 `json:"volume_share"` // 成交股數占市場比重（%）
	Amount      int64   `json:"amount"`       // 成交金額（元）
	AmountShare float64 `json:"amount_share"` // 成交金額占市場比重（%）
}

func normalizeBlockTrades(raw *RawResponse) ([]BlockTradeRow, error) {
	_, fields, rows, err := tablesOf(raw, "鉅額交易")
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return []BlockTradeRow{}, nil
	}
	out := make([]BlockTradeRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		ts, err := parseROCDate(m["日期"])
		if err != nil {
			continue
		}
		r := BlockTradeRow{
			Date:        model.FormatDate(ts),
			TradeType:   strings.TrimSpace(m["交易別"]),
			Class:       strings.TrimSpace(m["類別"]),
			Volume:      commaIntOrZero(m["成交股數"]),
			VolumeShare: commaFloatOrZero(m["成交股數占市場比重%"]),
			Amount:      commaIntOrZero(m["成交金額"]),
			AmountShare: commaFloatOrZero(m["成交金額占市場比重%"]),
		}
		if r.TradeType == "" && r.Class == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: block_trades 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 異常成交量（TWSE-WEB）：當日公布注意股票（成交量/價量異常警示）。
// 官方欄位：編號/證券代號/證券名稱/累計次數/注意交易資訊/日期/收盤價/本益比。

// AbnormalVolumeRow 為一檔當日異常成交量（注意）股票。
type AbnormalVolumeRow struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	NoticeCount int     `json:"notice_count"` // 累計次數
	Info        string  `json:"info"`         // 注意交易資訊
	Date        string  `json:"date"`         // YYYY-MM-DD
	Close       float64 `json:"close"`        // 收盤價（元）
	PE          float64 `json:"pe"`           // 本益比
}

// ---------------------------------------------------------------------------
// 盤後定價交易（TWSE-WEB）：/exchangeReport/BFT41U（T040）。
// 官方欄位：證券代號/證券名稱/成交數量/成交筆數/成交金額/成交價/最後揭示買量/最後揭示賣量。

// AfterHoursRow 為一檔盤後定價交易資訊。
type AfterHoursRow struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Volume      int64   `json:"volume"`      // 成交數量（股）
	Transaction int64   `json:"transaction"` // 成交筆數
	Amount      int64   `json:"amount"`      // 成交金額（元）
	Price       float64 `json:"price"`       // 成交價（元）
	BidVolume   int64   `json:"bid_volume"`  // 最後揭示買量（股）
	AskVolume   int64   `json:"ask_volume"`  // 最後揭示賣量（股）
	Date        string  `json:"date"`         // YYYY-MM-DD（資料歸屬日）
}

// normalizeAfterHours：盤後定價交易（BFT41U，T040）。頂層 date 為資料歸屬日。
func normalizeAfterHours(raw *RawResponse) ([]AfterHoursRow, error) {
	var meta struct {
		Date string `json:"date"`
	}
	_ = json.Unmarshal(raw.Body, &meta)
	date := ""
	if ts, err := time.Parse("20060102", strings.TrimSpace(meta.Date)); err == nil {
		date = model.FormatDate(ts)
	}
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]AfterHoursRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		r := AfterHoursRow{
			Code:        strings.TrimSpace(m["證券代號"]),
			Name:        strings.TrimSpace(m["證券名稱"]),
			Volume:      commaIntOrZero(m["成交數量"]),
			Transaction: commaIntOrZero(m["成交筆數"]),
			Amount:      commaDecimalToInt(m["成交金額"]),
			Price:       commaFloatOrZero(m["成交價"]),
			BidVolume:   commaIntOrZero(m["最後揭示買量"]),
			AskVolume:   commaIntOrZero(m["最後揭示賣量"]),
			Date:        date,
		}
		if r.Code == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: after_hours 無有效資料列")
	}
	return out, nil
}

func normalizeAbnormalVolume(raw *RawResponse) ([]AbnormalVolumeRow, error) {
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]AbnormalVolumeRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		r := AbnormalVolumeRow{
			Code:        strings.TrimSpace(m["證券代號"]),
			Name:        strings.TrimSpace(m["證券名稱"]),
			NoticeCount: int(commaIntOrZero(m["累計次數"])),
			Info:        strings.TrimSpace(m["注意交易資訊"]),
			Close:       commaFloatOrZero(m["收盤價"]),
			PE:          commaFloatOrZero(m["本益比"]),
		}
		if ts, err := parseROCDate(m["日期"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		if r.Code == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: abnormal_volume 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 個股日收盤（TWSE-API）：STOCK_DAY_ALL 全市場（含 ETF）日成交。
// 官方回傳前一交易日（2026-07 實測 date 請求無效、恆為 T-1），
// 日期以資料列之 Date 欄為準。單位：股/元。

// DailyCloseRow 為單一股票之日收盤。
type DailyCloseRow struct {
	Date        string  `json:"date"` // YYYY-MM-DD（官方 T-1）
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Volume      int64   `json:"volume"`      // 成交股數（股）
	Amount      int64   `json:"amount"`      // 成交金額（元）
	Open        float64 `json:"open"`        // 開盤價（元）
	High        float64 `json:"high"`        // 最高價（元）
	Low         float64 `json:"low"`         // 最低價（元）
	Close       float64 `json:"close"`       // 收盤價（元）
	Change      float64 `json:"change"`      // 漲跌（元）
	Transaction int64   `json:"transaction"` // 成交筆數
}

func normalizeDailyClose(raw *RawResponse) ([]DailyCloseRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: STOCK_DAY_ALL JSON 解析失敗: %w", err)
	}
	out := make([]DailyCloseRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := DailyCloseRow{
			Code:        strings.TrimSpace(m["Code"]),
			Name:        strings.TrimSpace(m["Name"]),
			Volume:      commaIntOrZero(m["TradeVolume"]),
			Amount:      commaIntOrZero(m["TradeValue"]),
			Open:        commaFloatOrZero(m["OpeningPrice"]),
			High:        commaFloatOrZero(m["HighestPrice"]),
			Low:         commaFloatOrZero(m["LowestPrice"]),
			Close:       commaFloatOrZero(m["ClosingPrice"]),
			Change:      commaFloatOrZero(m["Change"]),
			Transaction: commaIntOrZero(m["Transaction"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["Date"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: daily_close 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 外資持股（TWSE-API）：MI_QFIIS_cat 外資及陸資投資類股持股比率表。
// 單位：股/%。

// ForeignHoldingRow 為一類股之外資持股比率。
type ForeignHoldingRow struct {
	Industry     string  `json:"industry"`      // 類股（如 水泥工業/ETF）
	CompanyCount int64   `json:"company_count"` // 家數
	ShareNumber  int64   `json:"share_number"`  // 發行股數（股）
	ForeignShare int64   `json:"foreign_share"` // 外資及陸資持有股數（股）
	Percentage   float64 `json:"percentage"`    // 持股比率（%）
}

func normalizeForeignHoldings(raw *RawResponse) ([]ForeignHoldingRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: MI_QFIIS_cat JSON 解析失敗: %w", err)
	}
	out := make([]ForeignHoldingRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := ForeignHoldingRow{
			Industry:     strings.TrimSpace(m["IndustryCat"]),
			CompanyCount: commaIntOrZero(m["Numbers"]),
			ShareNumber:  commaIntOrZero(m["ShareNumber"]),
			ForeignShare: commaIntOrZero(m["ForeignMainlandAreaShare"]),
			Percentage:   commaFloatOrZero(m["Percentage"]),
		}
		if r.Industry == "" {
			continue
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: foreign_holdings 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 權證每日成交（TWSE-API）：t187ap42_L。官方單位：成交金額=仟元、
// 成交張數=張（2026-07 實測以 0.02 元/股權證核對）→ 換算為 元/股。

// WarrantRow 為一檔權證之每日成交統計。
type WarrantRow struct {
	TradeDate string `json:"trade_date"` // YYYY-MM-DD
	Code      string `json:"code"`
	Name      string `json:"name"`
	Amount    int64  `json:"amount"` // 成交金額（元）
	Volume    int64  `json:"volume"` // 成交張數（股）
}

func normalizeWarrants(raw *RawResponse) ([]WarrantRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: t187ap42_L JSON 解析失敗: %w", err)
	}
	out := make([]WarrantRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := WarrantRow{
			Code:   strings.TrimSpace(m["權證代號"]),
			Name:   strings.TrimSpace(m["權證名稱"]),
			Amount: model.ThousandToYuan(commaDecimalToInt(m["成交金額"])),
			Volume: model.LotsToShares(commaIntOrZero(m["成交張數"])),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["交易日期"]); err == nil {
			r.TradeDate = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: warrants 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 指數（TWSE-API）：MI_INDEX 每日各指數行情（加權/寶島/跨市場/報酬…）。

// IndexQuoteRow 為單一指數之收盤行情。
type IndexQuoteRow struct {
	Date          string  `json:"date"` // YYYY-MM-DD（官方 T-1）
	IndexName     string  `json:"index_name"`
	Close         float64 `json:"close"`                // 收盤指數
	ChangeDir     string  `json:"change_dir,omitempty"` // 漲跌(+/-)
	Change        float64 `json:"change"`               // 漲跌點數
	ChangePercent float64 `json:"change_percent"`       // 漲跌百分比（%）
	Note          string  `json:"note,omitempty"`
}

func normalizeIndices(raw *RawResponse) ([]IndexQuoteRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: MI_INDEX(API) JSON 解析失敗: %w", err)
	}
	out := make([]IndexQuoteRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := IndexQuoteRow{
			IndexName:     strings.TrimSpace(m["指數"]),
			Close:         commaFloatOrZero(m["收盤指數"]),
			ChangeDir:     strings.TrimSpace(m["漲跌"]),
			Change:        commaFloatOrZero(m["漲跌點數"]),
			ChangePercent: commaFloatOrZero(m["漲跌百分比"]),
			Note:          strings.TrimSpace(m["特殊處理註記"]),
		}
		if r.IndexName == "" || r.Close <= 0 {
			continue
		}
		if ts, err := parseROCDate(m["日期"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: indices 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// ESG（TWSE-API）：t187ap46_L_<topic> 上市公司企業 ESG 資訊揭露彙總資料。
// 欄位依 topic 不同（如 L_1 溫室氣體排放：範疇一/二/三排放量等）；
// Normalize 輸出通用結構，題材名與數值原樣保留（單位依官方欄位）。

// ESG 資料集 topic 對照（2026-07 swagger 實測）。
const (
	TWSEESGEmissions      = "1"  // 溫室氣體排放
	TWSEESGEnergy         = "2"  // 能源管理
	TWSEESGWater          = "3"  // 水資源管理
	TWSEESGWaste          = "4"  // 廢棄物管理
	TWSEESGHumanResource  = "5"  // 人力發展
	TWSEESGBoard          = "6"  // 董事會
	TWSEESGInvestorComm   = "7"  // 投資人溝通
	TWSEESGClimate        = "8"  // 氣候相關議題管理
	TWSEESGCommittee      = "9"  // 功能性委員會
	TWSEESGFuel           = "10" // 燃料管理
	TWSEESGProductCycle   = "11" // 產品生命週期
	TWSEESGFoodSafety     = "12" // 食品安全
	TWSEESGSupplyChain    = "13" // 供應鏈管理
	TWSEESGProductQuality = "14" // 產品品質與安全
	TWSEESGCommunity      = "15" // 社區關係
	TWSEESGInfoSecurity   = "16" // 資訊安全
	TWSEESGInclusiveFin   = "17" // 普惠金融
	TWSEESGControl        = "18" // 持股及控制力
	TWSEESGRiskPolicy     = "19" // 風險管理政策
	TWSEESGAntiCompet     = "20" // 反競爭行為法律訴訟
	TWSEESGOccupSafety    = "21" // 職業安全衛生
)

// ESGRow 為單一公司之 ESG 揭露列（欄位依 topic）。
type ESGRow struct {
	ReportDate string            `json:"report_date"` // 出表日期 YYYY-MM-DD
	Year       string            `json:"year"`        // 報告年度
	Code       string            `json:"code"`
	Name       string            `json:"name"`
	Fields     map[string]string `json:"fields"` // 該 topic 之其餘欄位（原值）
}

func normalizeESG(raw *RawResponse) ([]ESGRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: t187ap46_L JSON 解析失敗: %w", err)
	}
	out := make([]ESGRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := ESGRow{
			Year:   strings.TrimSpace(m["報告年度"]),
			Code:   strings.TrimSpace(m["公司代號"]),
			Name:   strings.TrimSpace(m["公司名稱"]),
			Fields: make(map[string]string, len(m)),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["出表日期"]); err == nil {
			r.ReportDate = model.FormatDate(ts)
		}
		for k, v := range m {
			switch k {
			case "出表日期", "報告年度", "公司代號", "公司名稱":
				continue
			}
			r.Fields[k] = v
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: esg 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 公司治理（TWSE-API）：t187ap32_L 上市公司公司治理之相關規程規則。

// GovernanceRow 為單一公司之公司治理規程規則。
type GovernanceRow struct {
	ReportDate string `json:"report_date"` // 出表日期 YYYY-MM-DD
	Code       string `json:"code"`
	Name       string `json:"name"`
	Rules      string `json:"rules"` // 公司治理之相關規程規則
}

func normalizeGovernance(raw *RawResponse) ([]GovernanceRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: t187ap32_L JSON 解析失敗: %w", err)
	}
	out := make([]GovernanceRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := GovernanceRow{
			Code:  strings.TrimSpace(m["公司代號"]),
			Name:  strings.TrimSpace(m["公司名稱"]),
			Rules: strings.TrimSpace(m["公司治理之相關規程規則"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["出表日期"]); err == nil {
			r.ReportDate = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: company_governance 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 估值指標（TWSE-API）：BWIBBU_ALL 上市個股日本益比、殖利率及股價淨值比
// （依代碼查詢，全市場快照；2026-07 實測 1081 列，含 ETF）。
// 虧損公司 PEratio 為空字串 → pe=0（由 handler 標記 pe_available=false）。

// ValuationRow 為單一上市股票之估值指標。
type ValuationRow struct {
	Date          string  `json:"date"` // YYYY-MM-DD
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	PE            float64 `json:"pe"`             // 本益比（虧損/不適用為 0）
	DividendYield float64 `json:"dividend_yield"` // 現金殖利率 %
	PB            float64 `json:"pb"`             // 股價淨值比
}

func normalizeValuation(raw *RawResponse) ([]ValuationRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: BWIBBU_ALL JSON 解析失敗: %w", err)
	}
	out := make([]ValuationRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := ValuationRow{
			Code: strings.TrimSpace(m["Code"]),
			Name: strings.TrimSpace(m["Name"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["Date"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		r.PE = commaFloatOrZero(m["PEratio"])
		r.DividendYield = commaFloatOrZero(m["DividendYield"])
		r.PB = commaFloatOrZero(m["PBratio"])
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: valuation 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 除權除息預告表（TWSE-API）：TWT48U_ALL 上市股票除權除息預告（2026-07
// 實測 122 列；Exdividend 為官方欄位名，值 息/權/權息；含 ETF）。

// ExDivEventRow 為單一除權息事件。
type ExDivEventRow struct {
	Date         string  `json:"date"` // 除權息日 YYYY-MM-DD
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Kind         string  `json:"kind"`          // 息 / 權 / 權息
	CashDividend float64 `json:"cash_dividend"` // 現金股利（元/股）
	StockRatio   float64 `json:"stock_ratio"`   // 股票股利（元/股）
}

func normalizeExDiv(raw *RawResponse) ([]ExDivEventRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: TWT48U_ALL JSON 解析失敗: %w", err)
	}
	out := make([]ExDivEventRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := ExDivEventRow{
			Code: strings.TrimSpace(m["Code"]),
			Name: strings.TrimSpace(m["Name"]),
			Kind: strings.TrimSpace(m["Exdividend"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["Date"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		r.CashDividend = commaFloatOrZero(m["CashDividend"])
		r.StockRatio = commaFloatOrZero(m["StockDividendRatio"])
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: ex_div 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 股利分派情形（TWSE-API）：t187ap45_L 上市公司股利分派（2026-07 實測 1148
// 列，股利年度 114/115；股利年度為分派基準年度，非除息日）。

// DividendRow 為單一公司單一年度之股利分派決議。
type DividendRow struct {
	TableDate     string  `json:"table_date"` // 出表日期 YYYY-MM-DD
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Progress      string  `json:"progress"`       // 決議（擬議）進度
	DividendYear  string  `json:"dividend_year"`  // 股利年度（民國）
	CashDividend  float64 `json:"cash_dividend"`  // 現金股利合計（盈餘+公積+法定）
	StockDividend float64 `json:"stock_dividend"` // 股票股利合計（盈餘+公積+法定）
	CashTotal     float64 `json:"cash_total"`     // 現金股利總金額（元）
	NetIncome     float64 `json:"net_income"`     // 本期淨利（元）
	Retained      float64 `json:"retained"`       // 可分配盈餘（元）
}

func normalizeDividend(raw *RawResponse) ([]DividendRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: t187ap45_L JSON 解析失敗: %w", err)
	}
	out := make([]DividendRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := DividendRow{
			Code:         strings.TrimSpace(m["公司代號"]),
			Name:         strings.TrimSpace(m["公司名稱"]),
			Progress:     strings.TrimSpace(m["決議（擬議）進度"]),
			DividendYear: strings.TrimSpace(m["股利年度"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["出表日期"]); err == nil {
			r.TableDate = model.FormatDate(ts)
		}
		cash := commaFloatOrZero(m["股東配發-盈餘分配之現金股利(元/股)"]) +
			commaFloatOrZero(m["股東配發-法定盈餘公積發放之現金(元/股)"]) +
			commaFloatOrZero(m["股東配發-資本公積發放之現金(元/股)"])
		stock := commaFloatOrZero(m["股東配發-盈餘轉增資配股(元/股)"]) +
			commaFloatOrZero(m["股東配發-法定盈餘公積轉增資配股(元/股)"]) +
			commaFloatOrZero(m["股東配發-資本公積轉增資配股(元/股)"])
		r.CashDividend = math.Round(cash*100) / 100
		r.StockDividend = math.Round(stock*100) / 100
		r.CashTotal = commaFloatOrZero(m["股東配發-股東配發之現金(股利)總金額(元)"])
		r.NetIncome = commaFloatOrZero(m["本期淨利(淨損)(元)"])
		r.Retained = commaFloatOrZero(m["可分配盈餘(元)"])
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: dividend 無有效資料列")
	}
	return out, nil
}

// Client 回傳底層 BaseClient（供 handler 共用連線池/限流/熔斷）。
func (s *TWSEWebSource) Client() *BaseClient { return s.client }

// Client 回傳底層 BaseClient（供 handler 共用連線池/限流/熔斷）。
func (s *TWSEAPISource) Client() *BaseClient { return s.client }

// ---------------------------------------------------------------------------
// 外資及陸資投資持股統計（TWSE-WEB）：MI_QFIIS 每日全市場個股快照
// （2026-07 實測：dayDate 請求日之資料於翌日釋出，等同 T-1；欄位單位
// 股/% 官方已明示）。get_foreign_shareholding_history 資料源（T011）。

// ForeignHoldingPointRow 為單一股票之一日外資持股快照。
type ForeignHoldingPointRow struct {
	Date            string  `json:"date"` // YYYY-MM-DD（官方 T-1）
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	IssueShares     int64   `json:"issue_shares"`    // 發行股數（股）
	ForeignShares   int64   `json:"foreign_shares"`  // 全體外資及陸資持有股數（股）
	ForeignPercent  float64 `json:"foreign_percent"` // 全體外資及陸資持股比率（%）
	UpperLimitPct   float64 `json:"upper_limit_pct"` // 共用法令投資上限比率（%）
	ChangeReason    string  `json:"change_reason,omitempty"`
	LastChangedDate string  `json:"last_changed_date,omitempty"` // 最近一次持股異動申報日期
}

func normalizeQFIIS(raw *RawResponse) ([]ForeignHoldingPointRow, error) {
	var env struct {
		Date string `json:"date"` // YYYYMMDD
	}
	if err := json.Unmarshal(raw.Body, &env); err != nil {
		return nil, fmt.Errorf("provider: qfiis 回應解析失敗: %w", err)
	}
	fields, rows, err := rowsOf(raw)
	if err != nil {
		return nil, err
	}
	out := make([]ForeignHoldingPointRow, 0, len(rows))
	for _, row := range rows {
		m := parseRow(fields, row)
		r := ForeignHoldingPointRow{
			Code:           strings.TrimSpace(m["證券代號"]),
			Name:           strings.TrimSpace(m["證券名稱"]),
			IssueShares:    commaIntOrZero(m["發行股數"]),
			ForeignShares:  commaIntOrZero(m["全體外資及陸資持有股數"]),
			ForeignPercent: commaFloatOrZero(m["全體外資及陸資持股比率"]),
			UpperLimitPct:  commaFloatOrZero(m["外資及陸資共用法令投資上限比率"]),
			ChangeReason:   strings.TrimSpace(m["與前日異動原因(註)"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if env.Date != "" {
			if ts, err := time.Parse("20060102", env.Date); err == nil {
				r.Date = model.FormatDate(ts)
			}
		}
		if ts, err := parseROCDate(m["最近一次上市公司申報外資及陸資持股異動日期"]); err == nil {
			r.LastChangedDate = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: qfiis 無有效資料列")
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// 集中市場公布處置股票（TWSE-API）：announcement/punish 處置公告清單
// （2026-07 實測：不含 date 參數，回傳最近處置名單）。
// get_attention_disposition_stocks 之上市處置資料源（T011）。

// PunishRow 為一筆處置公告。
type PunishRow struct {
	Number             string `json:"number"` // 編號
	Date               string `json:"date"`   // 公布日期 YYYY-MM-DD
	Code               string `json:"code"`
	Name               string `json:"name"`
	NoticeCount        int64  `json:"notice_count"`        // 累計（公布注意資訊次數）
	Reasons            string `json:"reasons"`             // 處置條件（如 連續三次）
	DispositionPeriod  string `json:"disposition_period"`  // 處置起迄時間（官方原文）
	DispositionMeasure string `json:"disposition_measure"` // 處置措施（如 第一次處置）
	Detail             string `json:"detail"`              // 處置內容（原文）
}

func normalizePunish(raw *RawResponse) ([]PunishRow, error) {
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(raw.Body, &rows); err != nil {
		return nil, fmt.Errorf("provider: announcement/punish JSON 解析失敗: %w", err)
	}
	out := make([]PunishRow, 0, len(rows))
	for _, row := range rows {
		m := rowToMap(row)
		r := PunishRow{
			Number:             strings.TrimSpace(m["Number"]),
			Code:               strings.TrimSpace(m["Code"]),
			Name:               strings.TrimSpace(m["Name"]),
			NoticeCount:        commaIntOrZero(m["NumberOfAnnouncement"]),
			Reasons:            strings.TrimSpace(m["ReasonsOfDisposition"]),
			DispositionPeriod:  strings.TrimSpace(m["DispositionPeriod"]),
			DispositionMeasure: strings.TrimSpace(m["DispositionMeasures"]),
			Detail:             strings.TrimSpace(m["Detail"]),
		}
		if r.Code == "" || r.Name == "" {
			continue
		}
		if ts, err := parseROCDate(m["Date"]); err == nil {
			r.Date = model.FormatDate(ts)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: punish 無有效資料列")
	}
	return out, nil
}
