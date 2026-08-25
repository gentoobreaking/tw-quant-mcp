package mcp

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// app_envelope_test.go：T019 驗收 #3 — Envelope 一致性測試。
//
// 對**所有已註冊 Tool**（A 6 + B/C 11 + D/E 10 + F/G 9 = 36）逐一呼叫，
// 驗證回傳 Envelope 之 `_lineage` 欄位齊全且語意正確（§3.2/§5）：
//   - source/source_role 非空且為登錄值
//   - data_date 為 YYYY-MM-DD
//   - freshness 為 §3.2 允許之三值之一
//   - fetched_at 非零（RFC3339）；latency_ms/cache_ttl ≥ 0
//   - data 非 nil；http_calls ≥ 0
//
// A 組盤中工具以 newTestApp（交易時段 09:30 + 快照種入）執行，
// 其餘以 fgApp（盤後 16:00 + fake 資料替身）執行。全程離線不連網
// （§13 測試策略：錄製回放，CI 不觸發 Rate Limit）。

// dateOnlyRE 為 §5.1 日期格式（YYYY-MM-DD）。
var dateOnlyRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// envelopeProbe 描述單一工具之最小呼叫（args 依各工具 schema 最小可行值）。
type envelopeProbe struct {
	name string
	args map[string]any
}

// intradayTools 為 A 組盤中（交易時段 gate）工具。
var intradayTools = map[string]bool{
	"set_active_watchlist":      true,
	"get_intraday_kline":        true,
	"get_intraday_quote":        true,
	"get_realtime_quote":        true, // T194：MIS 單發直查（非記憶體；測試以替身注入）
	"get_intraday_vwap":         true,
	"detect_volume_surge":       true,
	"scan_daytrade_eligibility": true,
}

// stubBCEnvelope 建立 B/C 組探針所需之 fake 資料（與既有 B/C 測試同款）。
func stubBCEnvelope(f *fakeFetch) {
	// get_etf_nav（§30.1：fundPric netPrice+atmps / close 市價）
	f.bodies["etf|0050|fundPric"] = `{"netPrice":[{"date":"2026/07/30","count":101.0},{"date":"2026/07/29","count":100.0}],"atmps":[{"date":"2026/07/30","count":0.15},{"date":"2026/07/29","count":-0.1}]}`
	f.bodies["etf|0050|close"] = `[{"date":"2026/07/30","count":101.15},{"date":"2026/07/29","count":99.9}]`
	// get_block_trades_daily（T042，tables 型）
	f.bodies["block_trades|"] = `[{"date":"2026-08-03","trade_type":"逐筆交易","class":"特定證券","volume":1000000,"volume_share":0.01,"amount":50000000,"amount_share":0.05}]`
	// get_after_hours_trading（T040）
	f.stub("after_hours", nil,
		`[{"code":"2330","name":"台積電","volume":100,"transaction":5,"amount":482500,"price":4825,"bid_volume":10,"ask_volume":20,"date":"2026-07-30"}]`)
	f.bodies["block_monthly|"] = `[{"month":"2026-08","volume":2000000,"amount":100000000}]`
	f.bodies["block_yearly|"] = `[{"month":"2026","volume":30000000,"amount":1500000000}]`
	f.bodies["block_trades|20260730"] = `[{"code":"2330","name":"台積電","trade_type":"配對交易","price":4825,"volume":50000,"amount":241250000}]`
	f.bodies["block_trades|date=20260730"] = `[{"code":"2330","name":"台積電","trade_type":"配對交易","price":4825,"volume":50000,"amount":241250000}]`
	f.bodies["cross_market|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["day_trade_targets|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["sbl_volume|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["first_foreign|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["margin_restrict|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["odd_lot|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["trading_changes|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["price_change_lim|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["new_list_5d|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["suspend_daytrade_ann|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["suspend_daytrade_his|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["suspended|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["insti_amounts|date=20260730"] = `[{"日期":"115/07/30","value":"樣本"}]`
	f.bodies["turnover_history|date=20260730"] = `[{"日期":"115/07/30","value":"樣本"}]`
	f.bodies["sbl_balance_his|date=20260730"] = `[{"日期":"115/07/30","value":"樣本"}]`
	f.bodies["sbl_trades_his|date=20260730"] = `[{"日期":"115/07/30","value":"樣本"}]`
	// get_central_depository_bond_redemption（T055）
	f.bodies["bond_redemption|"] = `[{"債券代號":"A00101","債券簡稱":"100央債甲01","發行日期":"2011/01/05","起息日":"2011/01/05","票面利率":"1.0000","每日補息金額(以面額十萬元計算)":"2.7397","_date":"2026-07-30"}]`
	// get_companies_cumulative_voting（T056，TWSE-API passthrough）
	f.bodies["cum_voting|"] = `[{"出表日期":"1150824","公司代號":"1101","公司名稱":"台泥","股東常(臨時)會日期-常或臨時":"常會","股東常(臨時)會日期-日期":"1150522","是否選任董監事":"否","董監事選任方式":"累積投票制"}]`
	// get_companies_ownership_changes_business_scope（T057）
	f.bodies["own_scope_halt|"] = `[{"出表日期":"1150824","公司代號":"1234","公司名稱":"示例公司","停止買賣開始日":"1150901"}]`
	// get_companies_ownership_changes_business_scope_trading（T058）
	f.bodies["own_scope_trade|"] = `[{"出表日期":"1150824","公司代號":"5678","公司名稱":"示例二公司","變更交易開始日":"1150901"}]`
	// get_companies_with_business_scope_changes（T060）
	f.bodies["scope_changes|"] = `[{"出表日期":"1150824","公司代號":"1101","公司名稱":"台泥","年度":"115","季別":"2","營業範圍重大變更說明":"新增水泥製品買賣業務"}]`
	// get_companies_with_independent_directors（T063）
	f.bodies["indep_directors|"] = `[{"出表日期":"1150824","序號":"1","公司代號":"1101","公司名稱":"台泥","職稱":"法人董事代表人","姓名":"示例","就任日期":"1130521","目前兼任其他公司名稱":""}]`
	// get_companies_with_ownership_changes（T064）
	f.bodies["ownership_change|"] = `[{"出表日期":"1150824","公司代號":"1516","公司名稱":"川飛","經營權異動日期":"1150629","經營權異動說明":"董事席次累積變動過半"}]`
	f.bodies["balance_sheet_ci|"] = `[{"出表日期":"1150825","年度":"115","季別":"2","公司代號":"2330","公司名稱":"台積電","流動資產":"1953224680.00","資產總計":"5960165310.00","負債總計":"2966944650.00","股本":"772318170.00","保留盈餘":"629290890.00"}]`
	f.bodies["board_insuff|"] = `[{"出表日期":"1150819","公司代號":"1225","公司名稱":"福懋油","全體董事本人實際持有股數":"6130138"}]`
	f.bodies["board_insuff_con|"] = `[{"出表日期":"1150819","連續不足達3個月":"2515","連續不足達4個月":"1225"}]`
	f.bodies["board_pledged|"] = `[{"出表日期":"1150819","公司代號":"1101","公司名稱":"台泥","質權設定股數":"100000"}]`
	f.bodies["board_holdings|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","職稱":"董事","姓名":"示例","持有股數":"1000000"}]`
	f.bodies["ceo_dual_role|"] = `[{"出表日期":"1150825","公司代號":"1101","公司名稱":"台泥","董事長是否兼任總經理":"否"}]`
	f.bodies["dir_comp_con|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","酬金總額":"10000000"}]`
	f.bodies["sup_comp_con|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","酬金總額":"5000000"}]`
	f.bodies["insider_preann|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","申報轉讓股數":"10000"}]`
	f.bodies["insider_untrans|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","未轉讓股數":"10000"}]`
	f.bodies["dir_comp|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","酬金總額":"8000000"}]`
	f.bodies["major_shareholders|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"行政院國家發展基金","持有股數":"1000000000"}]`
	f.bodies["broker_basic|"] = `[{"出表日期":"1150825","證券代號":"1020","券商(證券IB)簡稱":"合庫","設立日期":"1001202"}]`
	f.bodies["broker_branch|"] = `[{"出表日期":"1150825","證券商代號":"1021","證券商名稱":"合庫- 台中","地址":"台中市西區民台中市西區民權路91號6樓"}]`
	f.bodies["broker_elec|"] = `[{"出表日期":"1150815","成交月份":"11507","累計開戶數":"20958039","成交筆數":"50000000"}]`
	f.bodies["broker_gender|"] = `[{"出表日期":"1150825","證券商代號":"1020","男性員工人數":"68","女性員工人數":"109","總人數":"177"}]`
	f.bodies["broker_hq|"] = `[{"Code":"1020","Name":"合庫","EstablishmentDate":"1001202","Address":"台北市大安區忠孝東路四段285號1樓(部分)"}]`
	f.bodies["broker_income|"] = `[{"出表日期":"1150825","券商代號":"0200","券商名稱":"遠智證券","會計科目名稱":"經紀手續費收入","本月金額":"9856892","累進金額":"102032114"}]`
	f.bodies["broker_monthly|"] = `[{"出表日期":"1150825","券商代號":"0200","券商名稱":"遠智證券","會計科目名稱":"流動資產","本月借方總額":"73498551","本月貸方總額":"88602523"}]`
	f.bodies["broker_personnel|"] = `[{"出表日期":"1150801","職位":"高級業務員","受託買賣":"2130"}]`
	f.bodies["broker_reg_inv|"] = `[{"SecuritiesFirmCode":"1020","Name":"合庫","BrokerageBusinessStartingDate":"1100701","WealthManagementBusinessStartingDate":""}]`
	f.bodies["supervisor_ack|"] = `[{"出表日期":"1150824","公司代號":"2330","公司名稱":"台積電","是否設置審計委員會":"是"}]`
	f.bodies["profitability|"] = `[{"出表日期":"1150825","年度":"115","季別":"2","公司代號":"2330","公司名稱":"台積電","營業收入(百萬元)":"933786.86","毛利率(%)(營業毛利)/(營業收入)":"58.61"}]`
	f.bodies["audit_variance|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","年度":"115","季別":"2","差異說明":"無"}]`
	f.bodies["forecast_achv|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","年度":"115","季別":"2","達成率(%)":"105.20"}]`
	f.bodies["supervisor_comp|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","姓名":"示例","酬金總額":"3000000"}]`
	f.bodies["punish|"] = `[{"number":"1","date":"1150722","code":"2317","name":"鴻海","notice_count":3,"reasons":"連續三次","disposition_period":"115/07/23～115/08/05","disposition_measure":"第一次處置","detail":"人工管制撮合"}]`
	f.bodies["meeting_ann|"] = `[{"出表日期":"1150824","公司代號":"2330","公司名稱":"台積電","股東常(臨時)會日期-常或臨時":"常會"}]`
	f.bodies["meeting_dates|"] = `[{"出表日期":"1150824","公司代號":"1101","公司名稱":"台泥","公司地址":"台北市"}]`
	f.bodies["proposal_exercise|"] = `[{"出表日期":"1150824","公司代號":"1101","公司名稱":"台泥","召開股東會日期":"1150522"}]`
	f.bodies["fund_basic|"] = `[{"出表日期":"1150824","基金代號":"00400A","基金簡稱":"主動國泰動能高息","基金類型":"主動式ETF"}]`
	f.bodies["pub_board_hold|"] = `[{"出表日期":"1150820","資料年月":"11507","公司代號":"2330","公司名稱":"台積電"}]`
	f.bodies["pub_income_ci|"] = `[{"出表日期":"1150825","年度":"115","季別":"2","公司代號":"2330","公司名稱":"台積電","營業收入":"247728.00"}]`
	f.bodies["pub_bal_ci|"] = `[{"出表日期":"1150825","年度":"115","季別":"2","公司代號":"2330","公司名稱":"台積電","資產總額":"1000000.00"}]`
	f.bodies["margin_info|"] = `[{"項目":"融資(交易單位)","買進":"100","賣出":"90","今日餘額":"5000000","_table":"115年08月24日 信用交易統計","_date":"2026-07-30"}]`
	f.bodies["holiday|"] = `[{"日期":"2026-01-01","名稱":"中華民國開國紀念日","說明":"休市"}]`
	f.bodies["realtime_stats|"] = `[{"時間":"13:25:00","累計委託筆數":"3000000"}]`
	f.bodies["taiwan50|"] = `[{"日期":"20260825","臺灣50指數":"55000.00"}]`
	f.bodies["island_index|"] = `[{"日期":"20260825","寶島股價指數":"25000.00"}]`
	f.bodies["total_return|"] = `[{"日　期":"20260825","發行量加權股價報酬指數":"26000.00"}]`
	f.bodies["index_history|"] = `[{"date":"2026-07-30","open":23000.0,"high":23100.0,"low":22950.0,"close":23050.0},{"date":"2026-07-29","open":22900.0,"high":23050.0,"low":22880.0,"close":23000.0}]`
	f.bodies["monthly_avg_all|"] = `[{"股票代號":"2330","股票名稱":"台積電","收盤價":"1150.00","月平均價":"1130.00"}]`
	f.bodies["stock_mon_trade|date=20260730&stockNo=2330"] = `[{"年度月份":"115/07","最高價":"1180","最低價":"1100"}]`
	f.bodies["stock_year_his|date=20260730&stockNo=2330"] = `[{"年度":"114","最高價":"1180","最低價":"650"}]`
	f.bodies["stock_year_trade|"] = `[{"股票代號":"2330","股票名稱":"台積電","成交股數":"50000000"}]`
	f.bodies["top_foreign|"] = `[{"Rank":"1","Code":"2923","Name":"鼎固-KY"}]`
	f.bodies["twse_news|"] = `[{"SeqNumber":"1","Title":"示例新聞","PublishDate":"2026-08-25"}]`
	f.stub("market_close", url.Values{"date": {"20260825"}, "type": {"ALLBUT0999"}},
		`[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20},{"code":"2317","name":"鴻海","volume":2000,"amount":200000,"open":170,"high":180,"low":169,"close":175,"change_dir":"-","change":-5,"pe":10}]`)
	f.bodies["twse_events|"] = `[{"No":"1","Title":"115年SEMICON Taiwan主題式業績發表會","Details":"https://www.twse.com.tw/zh/about/news/event/content.html?x"},{"No":"2","Title":"法人說明會（8月）","Details":"https://www.twse.com.tw/zh/about/news/event/content.html?y"}]`
	f.bodies["note_trans|"] = `[{"Code":"2615","Name":"萬海","RecentlyMetAttentionSecuritiesCriteria":"115年8月21日至115年8月24日連續二次"},{"Code":"052176","Name":"聯電統一61購01","RecentlyMetAttentionSecuritiesCriteria":"115年8月21日"}]`
	f.bodies["warrant_basic|"] = `[{"權證代號":"030012","權證簡稱":"AES凱基57購02"}]`
	f.bodies["warrant_issue|"] = `[{"出表日期":"1150106","發行人代號":"5380","發行人名稱":"第一金證券股份有限公司","權證代號":"074888"}]`
	f.bodies["warrant_trader|"] = `[{"出表日期":"1150812","日期":"1150808","人數":"12345"}]`
	f.stub("monthly_avg", url.Values{"date": {"20260730"}, "stockNo": {"2330"}}, `[{"date":"2026-07-30","close":"1150.00","monthly_avg":"1130.00"}]`)
	f.stub("daily_k", url.Values{"date": {"20260730"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 0, 10)))
	f.bodies["daily_close|"] = `[{"code":"2330","name":"台積電","close":"1150.00"},{"code":"2317","name":"鴻海","close":"180.00"}]`
	f.bodies["sec_penalty|"] = `[{"出表日期":"1150824","股票代號":"2330","公司名稱":"台積電","違規事由":"示例"}]`
	f.bodies["foreign_apply|"] = `[{"No":"1","Code":"7874","Company":"禾碩康-KY","ApplicationDate":"1150429","Chairman":"康潤生","AmountofCapital ":"270000","CommitteeDate":"1150731","ApprovedDate":"","AgreementDate":"","ListingDate":"","Underwriter":"凱基","UnderwritingPrice":"","Note":""}]`
	f.bodies["new_listing|"] = `[{"Code":"7855","Company":"和運租車","ApplicationDate":"1150414","Chairman":"劉源森","ApprovedListingDate":"1150811","Underwriter":"台新"}]`
	f.bodies["local_apply|"] = `[{"Code":"7883","Company":"饗賓","ApplicationDate":"1150727","Chairman":"陳毅航","AmountofCapital ":"609760","CommitteeDate":"1150827","Underwriter":"元大","Note":""}]`
	f.bodies["suspend_listing|"] = `[{"DelistingDate":"115/06/23","Company":"森崴能源","Code":"6806"}]`
	f.bodies["otc_exright_daily|"] = `[{"Date":"1150730","SecuritiesCompanyCode":"8110","CompanyName":"華東","ExRightsDiviend":"除息","CashDividend":"0.5","OpeningReferencePrice":"49.5"}]`
	f.bodies["otc_foreign_trading|"] = `[{"Date":"1150730","Rank":"1","SecuritiesCompanyCode":"8110","CompanyName":"華東"," ForeignInvestorsIncludeMainlandAreaInvestors-TotalBuy":"1000"}]`
	f.bodies["otc_broker_volume|"] = `[{"Date":"20260730","StockRanking":"1","SecuritiesCompanyCodeAndCompanyName":"華東(8110)","SecuritiesFirmsRanking":"1","SecuritiesFirmsCode":"元大","TotalPurchaseShares":"1000","TotalSellShares":"900"}]`
	f.bodies["otc_monthly_revenue|"] = `[{"出表日期":"1150817","資料年月":"11507","公司代號":"1240","公司名稱":"茂生農經","營業收入-當月營收":"242511","營業收入-上月比較增減(%)":"-10.24"}]`
	f.bodies["otc_daily|"] = `[{"date":"2026-07-30","code":"8110","name":"華東","close":30.5,"change_dir":"+","change":0.35,"open":30.1,"high":30.7,"low":30.0,"volume":120000,"amount":3660000,"transaction":80}]`
	f.bodies["eps_stats|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","年度":"114","季度":"4","EPS":"15.85"}]`
	f.bodies["income_ci|"] = `[{"出表日期":"1150825","年度":"115","季別":"2","公司代號":"2330","公司名稱":"台積電","營業收入":"933786855000.00"}]`
	f.bodies["disclosure_vio|"] = `[{"出表日期":"1150825","公司代號":"2330","公司名稱":"台積電","違法情形":"未依法令期限公告申報"}]`
	f.bodies["top_volume|"] = `[{"證券代號":"2330","證券名稱":"台積電","成交股數":"1000","成交金額":"4825000","成交價":"4825"}]`
	f.bodies["etf_reg_inv|"] = `[{"rank":"1","code":"2330","name":"台積電","stock_accounts":"236,742","etf_code":"0050","etf_name":"元大台灣50","etf_accounts":"1,241,976","_date":"2026-07-30"}]`
	f.bodies["fin_prog_abnormal|"] = `[{"編號":"1","證券代號":"2330","證券名稱":"台積電","日期":"115年08月25日"}]`
	// get_stock_daily_quote（TSE：3 個月日 K，2026-07-30 在最後月份）
	f.stub("daily_k", url.Values{"date": {"20260501"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "05", 0, 20)))
	f.stub("daily_k", url.Values{"date": {"20260601"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "06", 20, 20)))
	f.stub("daily_k", url.Values{"date": {"20260701"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 40, 30)))
	// get_stock_daily_kline（day 預設）
	f.stub("daily_k", url.Values{"date": {"20260730"}, "stockNo": {"2330"}}, string(mkDailyMonth("2026", "07", 0, 10)))
	// get_market_summary
	f.stub("market_close", url.Values{"date": {"20260730"}, "type": {"ALL"}}, `[{"code":"2330","name":"台積電","volume":1000,"amount":100000,"open":100,"high":110,"low":99,"close":110,"change_dir":"+","change":10,"pe":20}]`)
	f.stub("daily_close", url.Values{"date": {"20260730"}}, `[{"date":"2026-07-30","code":"6147","name":"頎邦","close":75.5,"change_dir":"+","change":1.2,"open":74.3,"high":76,"low":74.1,"volume":1200000}]`)
	// get_institutional_investors
	f.stub("institutional", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","foreign_buy":1000,"foreign_sell":400,"foreign_net":600,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":0,"investment_sell":0,"investment_net":0}]`)
	// get_stock_trend_composite 法人回溯（2026-07-29/28/27/24/23：short 5 日，跳過週末）
	for _, d := range []string{"20260729", "20260728", "20260727", "20260724", "20260723"} {
		f.stub("institutional", url.Values{"date": {d}},
			`[{"code":"2330","name":"台積電","foreign_buy":500,"foreign_sell":300,"foreign_net":200,"foreign_dealer_buy":0,"foreign_dealer_sell":0,"foreign_dealer_net":0,"investment_buy":100,"investment_sell":50,"investment_net":50}]`)
	}
	// get_foreign_industry_holdings
	f.stub("foreign_holdings", nil,
		`[{"industry":"半導體業","company_count":10,"share_number":1000,"foreign_share":500,"percentage":50.0}]`)
	// get_foreign_shareholding_history（range=3；selectType=24 半導體業）
	for _, d := range []string{"20260730", "20260729", "20260728"} {
		f.stub("qfiis", url.Values{"dayDate": {d}, "selectType": {"24"}},
			`[{"date":"`+d[:4]+`-`+d[4:6]+`-`+d[6:]+`","code":"2330","name":"台積電","issue_shares":25930389000,"foreign_shares":1000000,"foreign_percent":10.5,"upper_limit_pct":100.0,"change_reason":"","last_changed_date":""}]`)
	}
	// get_margin_trading（TSE）
	f.stub("margin", url.Values{"date": {"20260730"}, "selectType": {"ALL"}},
		`[{"code":"2330","name":"台積電","margin_buy":100000,"margin_sell":50000,"margin_cash_redeem":10000,"margin_prev_balance":1000000,"margin_balance":1040000,"margin_limit":2000000}]`)
	// get_abnormal_trading + get_attention_disposition_stocks（abnormal_volume/punish）
	f.stub("abnormal_volume", url.Values{"date": {"20260730"}},
		`[{"code":"2330","name":"台積電","notice_count":2,"info":"連續三個營業日達注意標準","date":"2026-07-30","close":169,"pe":28}]`)
	f.stub("punish", url.Values{"date": {"20260730"}},
		`[{"number":"1","date":"1150722","code":"2317","name":"鴻海","notice_count":3,"reasons":"連續三次","disposition_period":"115/07/23～115/08/05","disposition_measure":"第一次處置","detail":"人工管制撮合"}]`)
	// get_warrant_activity
	f.stub("warrants", nil, `[{"trade_date":"2026-07-30","code":"052644","name":"台積電國票41購01","amount":5000000,"volume":100000}]`)
	// get_major_announcements
	f.stub("announcements", nil, `[
{"table_date":"2026-07-30","announce_date":"2026-07-30","announce_time":"18:30:00",
"code":"2330","name":"台積電","subject":"本公司董事會決議配發現金股利",
"clause":"第14款","fact_date":"2026-07-30","description":"每股配發新台幣8元"}]`)
	// get_twse_index (stub normalized JSON for fakeFetch)
	f.stub("indices", nil,
		`[{"date":"2026-07-30","index_name":"發行量加權股價指數","close":17000.0,"change":-50.0,"change_percent":-0.29,"change_dir":"-","note":""}]`)
	f.stub("index_history", url.Values{"date": {"20260701"}},
		`[{"date":"2026-07-01","open":17000.0,"high":17100.0,"low":16900.0,"close":17050.0},{"date":"2026-07-02","open":17050.0,"high":17150.0,"low":16950.0,"close":17100.0}]`)
}

// allToolProbes 為全部 40 個註冊工具之呼叫探針。
func allToolProbes() []envelopeProbe {
	return []envelopeProbe{
		// ── A 組（盤中，7；以 newTestApp 交易時段執行）──
		{name: "set_active_watchlist", args: map[string]any{"symbols": []any{"2330"}}},
		{name: "get_intraday_kline", args: map[string]any{"symbol": "2330", "timeframe": "1m", "limit": float64(5)}},
		{name: "get_intraday_quote", args: map[string]any{"symbol": "2330"}},
		{name: "get_realtime_quote", args: map[string]any{"stock_nos": []any{"2330", "6547"}}},
		{name: "get_intraday_vwap", args: map[string]any{"symbol": "2330"}},
		{name: "detect_volume_surge", args: map[string]any{"symbol": "2330", "minutes": float64(5)}},
		{name: "scan_daytrade_eligibility", args: map[string]any{"symbol": "2330"}},
		// ── B 組（盤後行情，6）──
		{name: "get_stock_daily_quote", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_stock_daily_kline", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_market_summary", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_abnormal_trading", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		{name: "get_warrant_activity", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_twse_index", args: map[string]any{"symbol": "發行量加權股價指數", "date": "2026-07-30"}},
		// ── ETF（§30.1，2）──
		{name: "get_etf_nav", args: map[string]any{"symbol": "0050"}},
		{name: "get_etf_dividend", args: map[string]any{"symbol": "0056"}},
		// ── C 組（籌碼/法人，6）──
		{name: "get_institutional_investors", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		{name: "get_foreign_industry_holdings", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_foreign_shareholding_history", args: map[string]any{"symbol": "2330", "range": float64(3), "date": "2026-07-30"}},
		{name: "get_margin_trading", args: map[string]any{"symbol": "2330", "date": "2026-07-30"}},
		{name: "get_major_announcements", args: map[string]any{}},
		{name: "get_attention_disposition_stocks", args: map[string]any{"market": "tse", "date": "2026-07-30"}},
		// ── T029 缺口工具（跨來源聚合，Grade PREVIEW）──
		{name: "get_stock_trend_composite", args: map[string]any{"symbol": "2330", "horizon": "short"}},
		// ── D 組（基本面，6）──
		{name: "get_financial_statements", args: map[string]any{"symbol": "2330", "period": "2026Q1"}},
		{name: "get_monthly_revenue", args: map[string]any{"symbol": "2330"}},
		{name: "get_financial_health_check", args: map[string]any{"symbol": "2330"}},
		{name: "get_valuation_ratios", args: map[string]any{"symbol": "2330"}},
		{name: "get_esg_report", args: map[string]any{"symbol": "2330"}},
		{name: "get_companies_with_anticompetitive_losses", args: map[string]any{}},
		{name: "get_companies_with_csr_reports_103", args: map[string]any{}},
		{name: "get_companies_with_inclusive_finance_data", args: map[string]any{}},
		{name: "get_company_profile", args: map[string]any{"symbol": "2330"}},
		// ── E 組（篩選/股利，4）──
		{name: "screen_stocks", args: map[string]any{"min_eps": float64(1)}},
		{name: "get_dividend_history", args: map[string]any{"symbol": "2330"}},
		{name: "get_exdividend_calendar", args: map[string]any{}},
		{name: "screen_high_yield", args: map[string]any{"min_yield": float64(1)}},
		// ── F 組（期貨選擇權，7）──
		{name: "get_futures_daily_ohlc", args: map[string]any{"contract": "TX"}},
		{name: "get_daily_futures_market_report", args: map[string]any{}},
		{name: "get_daily_options_market_report", args: map[string]any{}},
		{name: "get_futures_history", args: map[string]any{"contract": "TX", "start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_futures_daily_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_put_call_ratio", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_large_trader_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_institutional_futures_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_futures_institutional", args: map[string]any{}},
		{name: "get_index_futures_margin", args: map[string]any{}},
		{name: "get_institutional_fut_opt_split_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_institutional_general", args: map[string]any{}},
		{name: "get_institutional_total_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_institutional_traders_by_futures", args: map[string]any{}},
		{name: "get_institutional_traders_by_options", args: map[string]any{}},
		{name: "get_institutional_traders_calls_puts", args: map[string]any{}},
		{name: "get_institutional_traders_by_futures_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_large_traders_futures_history", args: map[string]any{"contract": "TX", "start": "2026-07-28", "end": "2026-07-29"}},
		{name: "get_large_traders_futures_oi", args: map[string]any{}},
		{name: "get_large_traders_options_oi", args: map[string]any{}},
		{name: "get_options_daily_history", args: map[string]any{"start": "2026-07-28", "end": "2026-07-29", "contract_month": "202608"}},
		{name: "get_options_delta", args: map[string]any{"contract": "TXO", "contract_month": "202608"}},
		{name: "get_options_oi_change", args: map[string]any{}},
		{name: "get_options_institutional_by_contract_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_options_institutional_calls_puts_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		{name: "get_stock_futures_margin", args: map[string]any{}},
		{name: "get_institutional_options_positions", args: map[string]any{"date": "2026-07-29"}},
		{name: "get_institutional_futures_history", args: map[string]any{"start": "2026-07-27", "end": "2026-07-29"}},
		// ── T040 parity ──
		{name: "get_after_hours_trading", args: map[string]any{}},
		{name: "get_annual_trading_volume", args: map[string]any{}},
		{name: "get_monthly_trading_statistics", args: map[string]any{}},
		{name: "get_block_trades_daily", args: map[string]any{}},
		{name: "get_block_trades_detail", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_block_trades_monthly", args: map[string]any{}},
		{name: "get_block_trades_yearly", args: map[string]any{}},
		// ── parity 批次 T115-T184 ──
		{name: "get_cross_market_trading_info", args: map[string]any{}},
		{name: "get_daily_day_trading_targets", args: map[string]any{}},
		{name: "get_financial_program_abnormal_recommendations", args: map[string]any{}},
		{name: "get_foreign_companies_applying_for_listing", args: map[string]any{}},
		{name: "get_local_companies_applying_for_listing", args: map[string]any{}},
		{name: "get_recently_listed_companies", args: map[string]any{}},
		{name: "get_suspended_listed_companies", args: map[string]any{}},
		{name: "get_otc_daily", args: map[string]any{}},
		{name: "get_otc_monthly_revenue", args: map[string]any{"code": "1240"}},
		{name: "get_otc_active_broker_volume", args: map[string]any{"stock_no": "8110"}},
		{name: "get_otc_foreign_trading", args: map[string]any{"code": "8110"}},
		{name: "get_otc_exdividend_result", args: map[string]any{"code": "8110"}},
		{name: "get_otc_index", args: map[string]any{}},
		{name: "get_otc_odd_lot", args: map[string]any{}},
		{name: "get_daily_securities_lending_volume", args: map[string]any{}},
		{name: "get_first_listed_foreign_stocks_daily", args: map[string]any{}},
		{name: "get_margin_loan_restrictions_announcement", args: map[string]any{}},
		{name: "get_odd_lot_trading_quotes", args: map[string]any{}},
		{name: "get_securities_trading_changes", args: map[string]any{}},
		{name: "get_stock_price_changes", args: map[string]any{}},
		{name: "get_stocks_no_price_change_first_five_days", args: map[string]any{}},
		{name: "get_suspended_day_trading_announcement", args: map[string]any{}},
		{name: "get_suspended_day_trading_history", args: map[string]any{}},
		{name: "get_suspended_trading_stocks", args: map[string]any{}},
		{name: "get_top_20_volume_stocks", args: map[string]any{"name": ""}},
		{name: "get_etf_regular_investment_ranking", args: map[string]any{}},
		{name: "get_market_institutional_amounts_history", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_market_turnover_history", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_short_sale_lending_balance_history", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_short_sale_lending_trades_history", args: map[string]any{"date": "2026-07-30"}},
		{name: "get_central_depository_bond_redemption", args: map[string]any{}},
		{name: "get_companies_cumulative_voting", args: map[string]any{}},
		{name: "get_companies_ownership_changes_business_scope", args: map[string]any{}},
		{name: "get_companies_ownership_changes_business_scope_trading", args: map[string]any{}},
		{name: "get_companies_with_business_scope_changes", args: map[string]any{}},
		{name: "get_companies_with_independent_directors", args: map[string]any{}},
		{name: "get_companies_with_ownership_changes", args: map[string]any{}},
		{name: "get_companies_with_refineries_in_populated_areas", args: map[string]any{}},
		{name: "get_company_balance_sheet", args: map[string]any{"code": "2330"}},
		{name: "get_company_board_info", args: map[string]any{"code": "2330"}},
		{name: "get_company_board_insufficient_shares", args: map[string]any{}},
		{name: "get_company_board_insufficient_shares_consecutive", args: map[string]any{}},
		{name: "get_company_board_pledged_shares", args: map[string]any{}},
		{name: "get_company_board_shareholdings", args: map[string]any{"code": "2330"}},
		{name: "get_company_ceo_dual_role", args: map[string]any{}},
		{name: "get_company_consolidated_director_compensation", args: map[string]any{"code": "2330"}},
		{name: "get_company_consolidated_supervisor_compensation", args: map[string]any{"code": "2330"}},
		{name: "get_company_daily_insider_trades_preannounced", args: map[string]any{"code": "2330"}},
		{name: "get_company_daily_insider_trades_untransferred", args: map[string]any{"code": "2330"}},
		{name: "get_company_director_compensation", args: map[string]any{"code": "2330"}},
		{name: "get_company_governance_info", args: map[string]any{"code": "2330"}},
		{name: "get_company_major_shareholders", args: map[string]any{"code": "2330"}},
		{name: "get_company_anticompetitive_litigation", args: map[string]any{"code": "2330"}},
		{name: "get_company_climate_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_community_relations", args: map[string]any{"code": "2330"}},
		{name: "get_company_energy_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_food_safety", args: map[string]any{"code": "2330"}},
		{name: "get_company_fuel_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_greenhouse_gas_emissions", args: map[string]any{"code": "2330"}},
		{name: "get_company_human_development", args: map[string]any{"code": "2330"}},
		{name: "get_company_inclusive_finance", args: map[string]any{"code": "2330"}},
		{name: "get_company_info_security", args: map[string]any{"code": "2330"}},
		{name: "get_company_investor_communications", args: map[string]any{"code": "2330"}},
		{name: "get_company_ownership_and_control", args: map[string]any{"code": "2330"}},
		{name: "get_company_product_lifecycle", args: map[string]any{"code": "2330"}},
		{name: "get_company_product_quality_safety", args: map[string]any{"code": "2330"}},
		{name: "get_company_risk_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_supply_chain_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_waste_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_water_management", args: map[string]any{"code": "2330"}},
		{name: "get_company_dividend", args: map[string]any{"code": "2330"}},
		{name: "get_company_eps_statistics", args: map[string]any{"code": "2330"}},
		{name: "get_company_income_statement", args: map[string]any{"code": "2330"}},
		{name: "get_company_information_disclosure_violations", args: map[string]any{"code": "2330"}},
		{name: "get_broker_basic_info", args: map[string]any{}},
		{name: "get_broker_branch_info", args: map[string]any{}},
		{name: "get_broker_electronic_trading_statistics", args: map[string]any{}},
		{name: "get_broker_gender_statistics", args: map[string]any{}},
		{name: "get_broker_headquarters_info", args: map[string]any{}},
		{name: "get_broker_income_expenditure", args: map[string]any{}},
		{name: "get_broker_monthly_statements", args: map[string]any{}},
		{name: "get_broker_service_personnel", args: map[string]any{}},
		{name: "get_brokers_offering_regular_investment", args: map[string]any{}},
		{name: "get_company_financial_reports_supervisor_acknowledgment", args: map[string]any{"code": "2330"}},
		{name: "get_company_governance_regulations", args: map[string]any{"code": "2330"}},
		{name: "get_company_major_news", args: map[string]any{"code": "2330"}},
		{name: "get_company_profitability_analysis", args: map[string]any{"code": "2330"}},
		{name: "get_company_profitability_analysis_summary", args: map[string]any{}},
		{name: "get_company_quarterly_audit_variance", args: map[string]any{"code": "2330"}},
		{name: "get_company_quarterly_earnings_forecast_achievement", args: map[string]any{"code": "2330"}},
		{name: "get_company_supervisor_compensation", args: map[string]any{"code": "2330"}},
		{name: "get_fund_basic_info", args: map[string]any{}},
		{name: "get_public_company_board_shareholdings", args: map[string]any{"code": "2330"}},
		{name: "get_public_company_income_statement", args: map[string]any{"code": "2330"}},
		{name: "get_public_company_balance_sheet", args: map[string]any{"code": "2330"}},
		{name: "get_margin_trading_info", args: map[string]any{}},
		{name: "get_market_disposal_stocks", args: map[string]any{}},
		{name: "get_market_historical_index", args: map[string]any{}},
		{name: "get_taiex_index_history", args: map[string]any{}},
		{name: "get_market_holiday_schedule", args: map[string]any{}},
		{name: "get_market_index_info", args: map[string]any{}},
		{name: "get_real_time_trading_stats", args: map[string]any{}},
		{name: "get_taiwan_50_index_history", args: map[string]any{}},
		{name: "get_taiwan_island_index_history", args: map[string]any{}},
		{name: "get_taiwan_total_return_index", args: map[string]any{}},
		{name: "get_stock_daily_trading", args: map[string]any{"code": "2330"}},
		{name: "get_stock_monthly_average", args: map[string]any{}},
		{name: "get_stock_monthly_trading", args: map[string]any{"code": "2330"}},
		{name: "get_stock_yearly_trading", args: map[string]any{}},
		{name: "get_top_foreign_holdings", args: map[string]any{}},
		{name: "get_twse_news", args: map[string]any{}},
		{name: "get_twse_events", args: map[string]any{"top": float64(10)}},
		{name: "get_abnormal_accumulated_notice_stocks", args: map[string]any{"limit": float64(10)}},
		{name: "get_all_stocks_daily_close", args: map[string]any{"date": "2026-08-25", "limit": float64(5)}},
		{name: "get_warrant_yearly_issuance_statistics", args: map[string]any{}},
		{name: "get_warrant_trader_count", args: map[string]any{}},
		{name: "get_stock_monthly_avg_history", args: map[string]any{"stock_no": "2330"}},
		{name: "get_stock_monthly_history", args: map[string]any{"stock_no": "2330"}},
		{name: "get_stock_yearly_history", args: map[string]any{"stock_no": "2330"}},
		{name: "get_warrant_basic_info", args: map[string]any{"code": "030012"}},
		{name: "get_company_sec_regulatory_penalties", args: map[string]any{"code": "2330"}},
		{name: "get_warrant_daily_trading", args: map[string]any{"code": "2330"}},
		{name: "get_company_shareholder_meeting_announcements", args: map[string]any{}},
		{name: "get_company_shareholder_meeting_announcements_by_code", args: map[string]any{"code": "2330"}},
		{name: "get_company_shareholder_meeting_dates", args: map[string]any{}},
		{name: "get_company_shareholder_proposal_exercise", args: map[string]any{}},
		// ── G 組（基礎設施，3）──
		{name: "get_symbol_list", args: map[string]any{}},
		{name: "get_trading_calendar", args: map[string]any{"year": float64(2026), "month": float64(2)}},
	}
}

// TestAllToolsEnvelopeConsistent 對全部 37 個註冊工具驗證 Envelope 一致性。
func TestAllToolsEnvelopeConsistent(t *testing.T) {
	// 盤後 App（B–G 工具）：fake 資料替身
	f := newFake(t)
	stubBCEnvelope(f)
	stubDE(f) // D/E 資料
	tq := newFakeTAIFEX(t, "2026-07-29")
	stubFG(tq) // F/G 資料
	app := fgApp(t, f, tq)

	// 盤中 App（A 工具）：交易時段 + 快照 + watchlist
	intraday := newTestApp(t)

	names := intraday.Registry().Names()
	if len(names) < 40 {
		t.Fatalf("前置：應至少登錄 40 工具，實際 %d", len(names))
	}
	covered := map[string]bool{}
	for _, p := range allToolProbes() {
		covered[p.name] = true
	}
	for _, n := range names {
		if !covered[n] {
			t.Errorf("探針清單缺漏工具 %q（驗收要求覆蓋所有已註冊 Tool）", n)
		}
	}
	if len(covered) != len(names) {
		t.Fatalf("探針應覆蓋全部 %d 個工具，實際 %d", len(names), len(covered))
	}

	for _, p := range allToolProbes() {
		p := p
		t.Run(p.name, func(t *testing.T) {
			target := app
			if intradayTools[p.name] {
				target = intraday
			}
			env, err := target.core.Call(context.Background(), p.name, p.args)
			if err != nil {
				t.Fatalf("Call 失敗: %v", err)
			}
			e, ok := env.(*model.Envelope)
			if !ok {
				t.Fatalf("回傳應為 *model.Envelope，實際 %T", env)
			}
			checkEnvelopeConsistency(t, p.name, e)
		})
	}
}

// checkEnvelopeConsistency 驗證單一 Envelope 之 _lineage 欄位齊全（§3.2/§5）。
func checkEnvelopeConsistency(t *testing.T, name string, e *model.Envelope) {
	t.Helper()
	// 附錄 A：所有回傳附加免責欄位（僅供研究參考，不構成投資建議）
	if e.Disclaimer != model.DisclaimerText {
		t.Errorf("%s: 缺免責欄位（附錄 A），實際 %q", name, e.Disclaimer)
	}
	// 多來源聚合工具（v2.1 §4 設計規則 2）：_lineage 為 []Lineage，逐一驗證
	//（lineage 陣列存在時，primary Lineage 僅為 first() 之檢視）。
	if len(e.Lineage.Multi) > 0 {
		for i, sub := range e.Lineage.Multi {
			checkLineageFields(t, fmt.Sprintf("%s[%d]", name, i), sub)
		}
	} else {
		checkLineageFields(t, name, e.Lineage.Lineage)
	}
	if e.Data == nil {
		t.Errorf("%s: data 不得為 nil", name)
	}
	if e.HTTPCalls < 0 {
		t.Errorf("%s: http_calls 應 ≥ 0，實際 %d", name, e.HTTPCalls)
	}
	if e.ChartMeta != nil && e.ChartMeta.RecommendedType == "" {
		t.Errorf("%s: _chart_meta.recommended_type 不得為空", name)
	}
}

// checkLineageFields 驗證單一 lineage 之必填語意欄位（§3.2 附錄 A）。
func checkLineageFields(t *testing.T, name string, lg model.Lineage) {
	t.Helper()
	if lg.Source == "" {
		t.Errorf("%s: source 不得為空", name)
	}
	switch lg.Source {
	case model.SourceTWSEAPI, model.SourceTWSEWeb, model.SourceTWSEMIS,
		model.SourceTPExAPI, model.SourceMOPS, model.SourceTAIFEXAPI, model.SourceTAIFEXDL:
	default:
		t.Errorf("%s: source 非登錄值 %q", name, lg.Source)
	}
	if lg.SourceRole == "" {
		t.Errorf("%s: source_role 不得為空", name)
	}
	switch lg.SourceRole {
	case model.SourceRoleCanonical, model.SourceRoleRealtime, model.SourceRoleFallback:
	default:
		t.Errorf("%s: source_role 非登錄值 %q", name, lg.SourceRole)
	}
	if !model.ValidFreshness(lg.Freshness) {
		t.Errorf("%s: freshness 非法 %q（v2.1 §4 僅允許五值）", name, lg.Freshness)
	}
	if lg.DataDate == "" {
		t.Errorf("%s: data_date 不得為空", name)
	} else if !dateOnlyRE.MatchString(lg.DataDate) {
		t.Errorf("%s: data_date 格式不符 YYYY-MM-DD: %q", name, lg.DataDate)
	}
	if lg.FetchedAt.Time.IsZero() {
		t.Errorf("%s: fetched_at 不得為零值", name)
	}
	if lg.LatencyMS < 0 {
		t.Errorf("%s: latency_ms 應 ≥ 0，實際 %d", name, lg.LatencyMS)
	}
	if lg.CacheTTL < 0 {
		t.Errorf("%s: cache_ttl 應 ≥ 0，實際 %d", name, lg.CacheTTL)
	}
	if lg.CacheAgeSec < 0 {
		t.Errorf("%s: cache_age_sec 應 ≥ 0，實際 %d", name, lg.CacheAgeSec)
	}
}

// TestIntradayToolsZeroHTTP 盤中 A 組工具之 Envelope 必須 http_calls=0
// （純記憶體組裝，§12.9 零 HTTP 驗收）。
func TestIntradayToolsZeroHTTP(t *testing.T) {
	app := newTestApp(t) // 已含快照種入
	for _, p := range []envelopeProbe{
		{name: "get_intraday_kline", args: map[string]any{"symbol": "2330", "timeframe": "1m"}},
		{name: "get_intraday_quote", args: map[string]any{"symbol": "2330"}},
		{name: "get_intraday_vwap", args: map[string]any{"symbol": "2330"}},
		{name: "detect_volume_surge", args: map[string]any{"symbol": "2330", "minutes": float64(5)}},
		{name: "scan_daytrade_eligibility", args: map[string]any{"symbol": "2330"}},
	} {
		p := p
		t.Run(p.name, func(t *testing.T) {
			env := callCore(t, app, p.name, p.args)
			if env.HTTPCalls != 0 {
				t.Errorf("%s: http_calls 應為 0（盤中純記憶體），實際 %d", p.name, env.HTTPCalls)
			}
			if !env.Lineage.FetchedAt.Time.Equal(testClock()) {
				t.Logf("%s: fetched_at=%s（testClock 09:30）", p.name, env.Lineage.FetchedAt.Format(time.RFC3339))
			}
		})
	}
}
