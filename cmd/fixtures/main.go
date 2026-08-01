// Command fixtures 錄製官方 raw response 為 golden fixtures（§13 錄製回放）。
//
// 用法：
//
//	go run ./cmd/fixtures -host twse       # 錄製 TWSE（Web + API）
//	go run ./cmd/fixtures -host tpex       # 錄製 TPEx-API
//	go run ./cmd/fixtures -host mops       # 錄製 MOPS（OpenData CSV）
//	go run ./cmd/fixtures -host taifex     # 錄製 TAIFEX（API + DL CSV）
//	go run ./cmd/fixtures -host mis        # 錄製 MIS（index.jsp + 多 tick 序列）
//	go run ./cmd/fixtures -host all        # 全部（預設）
//
// 旗標：
//
//	-out <dir>       輸出目錄（預設 pkg/provider/testdata，相對專案根）
//	-date YYYYMMDD   資料日期（預設今天，Asia/Taipei）
//	-mis-ticks N     MIS 多 tick 序列筆數（預設 5；每筆間隔 ≥ 主機 rate limit 8s）
//	-quiet           僅輸出錄製摘要
//
// 錄製透過 pkg/provider 之 SourceContract（同一 URL/端點建構），以
// BaseClient 預設 rate limit 請求，避免 CI/開發誤觸官方 Rate Limit
// （§13 備註：fixtures 一律於 CI 離線跑）。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

func main() {
	host := flag.String("host", "all", "twse|tpex|mops|taifex|mis|all")
	out := flag.String("out", "pkg/provider/testdata", "輸出目錄")
	date := flag.String("date", "", "資料日期 YYYYMMDD（預設今天）")
	ticks := flag.Int("mis-ticks", 5, "MIS 多 tick 序列筆數")
	quiet := flag.Bool("quiet", false, "僅輸出摘要")
	flag.Parse()

	if *date == "" {
		*date = time.Now().In(model.Taipei()).Format("20060102")
	}
	rec := &recorder{
		ctx:   context.Background(),
		out:   *out,
		date:  *date,
		quiet: *quiet,
		log:   log.New(os.Stderr, "fixtures: ", 0),
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		rec.fatal(err)
	}

	var err error
	switch *host {
	case "twse":
		err = rec.recordTWSE()
	case "tpex":
		err = rec.recordTPEx()
	case "mops":
		err = rec.recordMOPS()
	case "taifex":
		err = rec.recordTAIFEX()
	case "mis":
		err = rec.recordMIS(*ticks)
	case "all":
		err = rec.recordAll(*ticks)
	default:
		rec.fatal(fmt.Errorf("未知 -host %q", *host))
	}
	if err != nil {
		rec.fatal(err)
	}
	fmt.Printf("fixtures 錄製完成：%s（日期 %s）\n", *host, *date)
	fmt.Printf("請更新 %s 之 FIXTURES.md 註記\n", filepath.Join(*out, "FIXTURES.md"))
}

// recorder 為錄製器：依各主機之 URL 建構與 fetch 儲存 raw response。
type recorder struct {
	ctx   context.Context
	out   string
	date  string
	quiet bool
	log   *log.Logger
}

func (r *recorder) fatal(err error) {
	r.log.Fatal(err)
}

func (r *recorder) say(format string, a ...any) {
	if !r.quiet {
		r.log.Printf(format, a...)
	}
}

// save 寫入 raw body（絕對路徑回傳供 FIXTURES.md 註記）。
func (r *recorder) save(host, name string, body []byte) (string, error) {
	dir := filepath.Join(r.out, host)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, body, 0o644); err != nil {
		return "", err
	}
	return p, nil
}

// fetch 以 source 之 URL 建構並抓取；空 params 表 nil。
func (r *recorder) fetchURL(source provider.SourceContract, rawURL string, name string) error {
	resp, err := source.Fetch(r.ctx, provider.RawRequest{URL: rawURL})
	if err != nil {
		return fmt.Errorf("fetch %s 失敗: %w", rawURL, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("fetch %s 回應異常（status=%d）", rawURL, resp.StatusCode)
	}
	p, err := r.save(r.hostOf(source), name, resp.Body)
	if err != nil {
		return err
	}
	r.say("已錄製 %s（%d bytes）", p, len(resp.Body))
	return nil
}

func (r *recorder) hostOf(s provider.SourceContract) string {
	switch s.ID() {
	case model.SourceTWSEWeb, model.SourceTWSEAPI:
		return "twse"
	case model.SourceTPExAPI:
		return "tpex"
	case model.SourceMOPS:
		return "mops"
	case model.SourceTAIFEXAPI, model.SourceTAIFEXDL:
		return "taifex"
	default:
		return strings.ToLower(s.ID())
	}
}

func (r *recorder) recordAll(ticks int) error {
	for _, fn := range []func() error{
		r.recordTWSE, r.recordTPEx, r.recordMOPS, r.recordTAIFEX,
	} {
		if err := fn(); err != nil {
			return err
		}
	}
	return r.recordMIS(ticks)
}

// ************** TWSE（www.twse.com.tw + openapi.twse.com.tw） **************

func (r *recorder) recordTWSE() error {
	web := provider.NewTWSEWebSource()
	api := provider.NewTWSEAPISource()
	date := r.date

	webItems := []struct {
		ds     provider.TWSEWebDataset
		params url.Values
		name   string
	}{
		{provider.TWSEWDDailyK, url.Values{"date": {date}, "stockNo": {"2330"}}, "daily_k_2330.json"},
		{provider.TWSEWDMarketClose, nil, "market_close.json"},
		{provider.TWSEWDInstitutional, nil, "institutional.json"},
		{provider.TWSEWDMargin, url.Values{"date": {date}, "selectType": {"ALL"}}, "margin.json"},
		{provider.TWSEWDAbnormal, nil, "abnormal_volume.json"},
	}
	for _, it := range webItems {
		if err := r.fetchURL(web, web.URL(it.ds, it.params), it.name); err != nil {
			return err
		}
	}

	apiItems := []struct {
		ds     provider.TWSEAPIDataset
		params url.Values
		name   string
	}{
		{provider.TWSEAPIDailyClose, nil, "daily_close.json"},
		{provider.TWSEAPIForeignHoldings, nil, "foreign_holdings.json"},
		{provider.TWSEAPIPunish, nil, "punish.json"},
		{provider.TWSEAPIValuation, nil, "valuation.json"},
		{provider.TWSEAPIExDiv, nil, "ex_div.json"},
	}
	for _, it := range apiItems {
		if err := r.fetchURL(api, api.URL(it.ds, it.params), it.name); err != nil {
			return err
		}
	}
	return nil
}

// ************** TPEx（www.tpex.org.tw/openapi） **************

func (r *recorder) recordTPEx() error {
	s := provider.NewTPExSource()
	items := []struct {
		ds     provider.TPExDataset
		params url.Values
		name   string
	}{
		{provider.TPExDailyClose, nil, "daily_close.json"},
		{provider.TPExPEValuation, nil, "pe_valuation.json"},
		{provider.TPExInstitutional, nil, "institutional.json"},
		{provider.TPExInstiSummary, nil, "institutional_summary.json"},
		{provider.TPExMargin, nil, "margin.json"},
		{provider.TPExAttention, nil, "attention.json"},
		{provider.TPExDisposition, nil, "disposition.json"},
	}
	for _, it := range items {
		if err := r.fetchURL(s, s.URL(it.ds, it.params), it.name); err != nil {
			return err
		}
	}
	return nil
}

// ************** MOPS（mopsfin.twse.com.tw OpenData CSV） **************

func (r *recorder) recordMOPS() error {
	s := provider.NewMOPSSource()
	items := []struct {
		ds     provider.MOPSDataset
		params url.Values
		name   string
	}{
		{provider.MOPSMonthlyRevenue, nil, "monthly_revenue.csv"},
		{provider.MOPSIncomeSummary, nil, "income_summary.csv"},
		{provider.MOPSProfitRatios, nil, "profit_ratios.csv"},
		{provider.MOPSCompanyProfile, nil, "company_profile.csv"},
		{provider.MOPSAnnouncements, nil, "announcements.csv"},
	}
	for _, it := range items {
		if err := r.fetchURL(s, s.URL(it.ds, it.params), it.name); err != nil {
			return err
		}
	}
	return nil
}

// ************** TAIFEX（openapi + DL CSV） **************

func (r *recorder) recordTAIFEX() error {
	api := provider.NewTAIFEXAPISource()
	dl := provider.NewTAIFEXDLSource()
	date := r.date

	apiItems := []struct {
		ds   model.TAIFEXDataset
		name string
	}{
		{model.TAPutCallRatio, "pc_ratio.json"},
		{model.TAMargin, "margin.json"},
	}
	for _, it := range apiItems {
		if err := r.fetchURL(api, api.URL(it.ds, nil), it.name); err != nil {
			return err
		}
	}

	// DL 大 CSV（每日盤後）：以源之 Download 流程抓取（含 view+POST 二步式）
	dlItems := []struct {
		ds   model.TAIFEXDataset
		name string
	}{
		{model.TAFuturesDaily, "futures_daily.csv"},
		{model.TAOptionsDaily, "options_daily.csv"},
		{model.TAInstiFutures, "insti_futures.csv"},
		{model.TAPutCallRatio, "pc_ratio.csv"},
	}
	for _, it := range dlItems {
		if err := r.fetchDL(dl, it.ds, date, it.name); err != nil {
			return err
		}
	}
	return nil
}

// fetchDL 以 DL 源之下載流程抓取（§9.3 view+POST）；日期以 YYYY-MM-DD 傳入，
// 由源內部轉為官方表單 YYYY/MM/DD。
func (r *recorder) fetchDL(dl *provider.TAIFEXDLSource, ds model.TAIFEXDataset, date, name string) error {
	if len(date) != 8 {
		return fmt.Errorf("DL 錄製需 YYYYMMDD 日期，實際 %q", date)
	}
	iso := date[0:4] + "-" + date[4:6] + "-" + date[6:8]
	rawURL := dl.URL(ds, url.Values{
		"queryStartDate": {iso},
		"queryEndDate":   {iso},
	})
	resp, err := dl.Fetch(r.ctx, provider.RawRequest{URL: rawURL})
	if err != nil {
		return fmt.Errorf("DL fetch %s 失敗: %w", ds, err)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("DL fetch %s 回應異常（status=%d）", ds, resp.StatusCode)
	}
	// DL 端需瀏覽器 session；回應非 CSV 時視為未錄到（保留既有人工下載 fixtures）
	if !strings.Contains(http.DetectContentType(resp.Body), "text/csv") &&
		!strings.HasPrefix(strings.TrimSpace(string(resp.Body)), "交易日期") {
		r.say("DL %s 回應非 CSV（status=%d，需瀏覽器 session；跳過，保留既有 fixture）",
			ds, resp.StatusCode)
		return nil
	}
	p, err := r.save("taifex", name, resp.Body)
	if err != nil {
		return err
	}
	r.say("已錄製 %s（%d bytes）", p, len(resp.Body))
	return nil
}

// ************** MIS（mis.twse.com.tw） **************

func (r *recorder) recordMIS(ticks int) error {
	client := provider.NewBaseClient("mis.twse.com.tw")
	indexURL := "https://mis.twse.com.tw/stock/index.jsp"

	// Session 預熱端點（§8.3）：index.jsp 改版/阻擋（404）時僅記錄不阻斷
	// （與 MISWorker 行為一致）。
	resp, err := client.Do(r.ctx, provider.RawRequest{URL: indexURL})
	if err != nil {
		r.say("MIS index.jsp 抓取失敗（跳過，僅錄 tick 序列）: %v", err)
	} else if resp.StatusCode >= 400 {
		r.say("MIS index.jsp 回應異常（status=%d，跳過）", resp.StatusCode)
	} else {
		p, err := r.save("mis", "index.html", resp.Body)
		if err != nil {
			return err
		}
		r.say("已錄製 %s（%d bytes）", p, len(resp.Body))
	}

	// 多 tick 序列：單一請求 ex_ch 兩檔熱門股；每筆間隔 ≥ 8s（§4.4）
	quoteURL := "https://mis.twse.com.tw/stock/api/getStockInfo.jsp"
	for i := 1; i <= ticks; i++ {
		u := fmt.Sprintf("%s?ex_ch=%s&_=%d", quoteURL,
			url.QueryEscape("tse_2330.tw|otc_6547.tw"), time.Now().UnixMilli())
		resp, err := client.Do(r.ctx, provider.RawRequest{URL: u})
		if err != nil {
			return fmt.Errorf("MIS tick %d 抓取失敗: %w", i, err)
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("MIS tick %d 回應異常（status=%d）", i, resp.StatusCode)
		}
		p, err := r.save("mis", fmt.Sprintf("tick_%02d.json", i), resp.Body)
		if err != nil {
			return err
		}
		r.say("已錄製 %s（%d bytes）", p, len(resp.Body))
		if i < ticks {
			r.say("等待 9s（rate limit 8s）…")
			time.Sleep(9 * time.Second)
		}
	}
	return nil
}
