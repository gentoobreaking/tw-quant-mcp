package provider

// TWSE Adapter 契約測試（T008）：以 2026-07-31 實地錄製之官方 raw fixtures
// （testdata/twse/，欄位/數值為官方原文）驗證 Fetch→Validate→Normalize：
// 欄位型別、單位換算（仟元/張 → 元/股，§5.1）、日期格式（民國年 → RFC3339 日期）。

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tw-quant-mcp/pkg/model"
)

// loadFixture 讀取 testdata/twse/<name> 之官方錄製回應。
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "twse", name))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", name, err)
	}
	return b
}

// fixtureRaw 建立 RawResponse（SourceURL 供資料集分派）。
func fixtureRaw(t *testing.T, sourceURL string, body []byte) *RawResponse {
	t.Helper()
	return &RawResponse{
		StatusCode: http.StatusOK,
		Body:       body,
		SourceURL:  sourceURL,
		FetchedAt:  model.Now(),
	}
}

// sourceOf 依資料集建立對應之 TWSE 來源（不建構 real client）。
func sourceOf(ds string) SourceContract {
	switch ds {
	case "daily_k", "monthly_avg", "margin", "institutional", "market_close",
		"index_history", "block_trades", "abnormal_volume":
		return &TWSEWebSource{client: NewBaseClient("www.twse.com.tw", WithRateInterval(0))}
	default:
		return &TWSEAPISource{client: NewBaseClient("openapi.twse.com.tw", WithRateInterval(0))}
	}
}

// urlOf 建立測試用之官方風格 URL（僅 path 影響分派，host 不拘）。
func urlOf(path string) string { return "https://www.twse.com.tw" + path }

func TestTWSEWebURL(t *testing.T) {
	s := NewTWSEWebSource(WithRateInterval(0))
	u := s.URL(TWSEWDDailyK, url.Values{"date": {"20260731"}, "stockNo": {"2330"}, "adjust": {"true"}})
	want := "https://www.twse.com.tw/rwd/afterTrading/STOCK_DAY?response=json&adjust=true&date=20260731&stockNo=2330"
	if u != want {
		t.Errorf("URL = %q\nwant %q", u, want)
	}
	if got := s.URL(TWSEWDMargin, url.Values{"date": {"20260730"}}); got !=
		"https://www.twse.com.tw/rwd/afterTrading/MI_MARGN?response=json&date=20260730" {
		t.Errorf("margin URL = %q", got)
	}
}

func TestTWSEAPIURL(t *testing.T) {
	s := NewTWSEAPISource(WithRateInterval(0))
	if got := s.URL(TWSEAPIWarrants, nil); got != "https://openapi.twse.com.tw/v1/opendata/t187ap42_L" {
		t.Errorf("warrants URL = %q", got)
	}
	if got := s.URL(TWSEAPIESG, nil); got != "https://openapi.twse.com.tw/v1/opendata/t187ap46_L_1" {
		t.Errorf("esg 預設 topic URL = %q", got)
	}
	if got := s.URL(TWSEAPIESG, url.Values{"topic": {"6"}}); got != "https://openapi.twse.com.tw/v1/opendata/t187ap46_L_6" {
		t.Errorf("esg topic=6 URL = %q", got)
	}
	if got := s.URL(TWSEAPIDailyClose, url.Values{"date": {"20260731"}}); got !=
		"https://openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL?date=20260731" {
		t.Errorf("daily_close URL = %q", got)
	}
}

// TestTWSEWebDailyK 個股日 K：金額元/量股/民國年轉換（2026-07-31 官方錄製）。
func TestTWSEWebDailyK(t *testing.T) {
	src := sourceOf("daily_k")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY?response=json&date=20260731&stockNo=2330"),
		loadFixture(t, "day.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var cs []model.Candle
	if err := json.Unmarshal(out, &cs); err != nil {
		t.Fatalf("Normalize 輸出非 Candle[]: %v", err)
	}
	if len(cs) != 22 {
		t.Fatalf("應有 22 個交易日，實際 %d", len(cs))
	}
	first := cs[0]
	if first.Timestamp != "2026-07-01" {
		t.Errorf("民國 115/07/01 應轉為 2026-07-01，實際 %s", first.Timestamp)
	}
	if first.Close != 2505 || first.Open != 2495 {
		t.Errorf("開/收盤價錯誤: %v/%v", first.Open, first.Close)
	}
	// 成交股數 37,544,470 股（官方即為股，不需 ×1000）
	if first.Volume != 37544470 {
		t.Errorf("Volume 應為 37,544,470 股，實際 %d", first.Volume)
	}
	// 成交金額 93,600,076,825 元（官方即為元）
	if first.Amount != 93600076825 {
		t.Errorf("Amount 應為 93,600,076,825 元，實際 %d", first.Amount)
	}
}

// TestTWSEWebDailyKWeekMonth 週/月 K 聚合（§5.3）。
func TestTWSEWebDailyKWeekMonth(t *testing.T) {
	src := sourceOf("daily_k")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY?response=json&date=20260731&stockNo=2330&period=week"),
		loadFixture(t, "day.json"))
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("週 K Normalize 失敗: %v", err)
	}
	var ws []model.Candle
	if err := json.Unmarshal(out, &ws); err != nil {
		t.Fatal(err)
	}
	if len(ws) >= 22 || len(ws) < 4 {
		t.Errorf("7 月 22 個交易日應聚合為 4~5 根週 K，實際 %d", len(ws))
	}
	// 首週起始日須為週一
	if ws[0].Timestamp != "2026-06-29" {
		t.Errorf("7 月首週應自 2026-06-29（週一）起，實際 %s", ws[0].Timestamp)
	}
	if ws[0].Close != 2445 {
		t.Errorf("首週收盤應為該週最後交易日（07/03）收盤 2445，實際 %v", ws[0].Close)
	}

	raw = fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY?response=json&date=20260731&stockNo=2330&period=month"),
		loadFixture(t, "day.json"))
	out, err = src.Normalize(raw)
	if err != nil {
		t.Fatalf("月 K Normalize 失敗: %v", err)
	}
	var ms []model.Candle
	if err := json.Unmarshal(out, &ms); err != nil {
		t.Fatal(err)
	}
	if len(ms) != 1 || ms[0].Timestamp != "2026-07-01" {
		t.Errorf("月 K 應為單根 2026-07-01，實際 %v", ms)
	}
	if ms[0].High < 2505 || ms[0].Low > 2505 {
		t.Errorf("月 K 高低價應涵蓋單日價格: %v/%v", ms[0].High, ms[0].Low)
	}
}

// TestTWSEWebMonthlyAvg 月均價：收盤價與計算之月平均。
func TestTWSEWebMonthlyAvg(t *testing.T) {
	src := sourceOf("monthly_avg")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY_AVG?response=json&date=20260731&stockNo=2330"),
		loadFixture(t, "day_avg.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []MonthlyAvgRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 22 || rows[0].Date != "2026-07-01" {
		t.Fatalf("列數/日期錯誤: %d/%s", len(rows), rows[0].Date)
	}
	if rows[0].Close != 2505 {
		t.Errorf("07/01 收盤價應為 2505，實際 %v", rows[0].Close)
	}
	if rows[0].MonthAvg <= 0 || rows[0].MonthAvg != rows[1].MonthAvg {
		t.Errorf("月平均收盤價應一致且 >0: %v", rows[0].MonthAvg)
	}
}

// TestTWSEWebMargin 融資融券：張 → 股（×1000，§5.1）。
func TestTWSEWebMargin(t *testing.T) {
	src := sourceOf("margin")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/MI_MARGN?response=json&date=20260730&selectType=ALL"),
		loadFixture(t, "margin.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []MarginRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	var r MarginRow
	for _, x := range rows {
		if x.Code == "2330" {
			r = x
		}
	}
	if r.Code != "2330" {
		t.Fatalf("找不到 2330 列，實際 %d 列", len(rows))
	}
	// 官方 2330：融資買進 1,013（張）、今日餘額 30,664（張）→ ×1000 股
	if r.MarginBuy != 1013000 {
		t.Errorf("融資買進 1,013 張 → 1,013,000 股，實際 %d", r.MarginBuy)
	}
	if r.MarginBalance != 30664000 {
		t.Errorf("融資今日餘額 30,664 張 → 30,664,000 股，實際 %d", r.MarginBalance)
	}
	// 融券賣出 23（張）→ 23,000 股
	if r.ShortSell != 23000 {
		t.Errorf("融券賣出 23 張 → 23,000 股，實際 %d", r.ShortSell)
	}
	// 資券互抵 17（張）→ 17,000 股
	if r.Offset != 17000 {
		t.Errorf("資券互抵 17 張 → 17,000 股，實際 %d", r.Offset)
	}
}

// TestTWSEWebInstitutional 三大法人買賣超：官方單位即為股，不換算。
func TestTWSEWebInstitutional(t *testing.T) {
	src := sourceOf("institutional")
	raw := fixtureRaw(t, urlOf("/rwd/fund/T86?response=json&date=20260731&selectType=ALL"),
		loadFixture(t, "institutional.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []InstitutionalRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	var r InstitutionalRow
	for _, x := range rows {
		if x.Code == "0050" {
			r = x
		}
	}
	if r.Code != "0050" {
		t.Fatalf("找不到 0050 列")
	}
	// 官方 0050：外陸資買進 167,204,439（股）→ 原值
	if r.ForeignBuy != 167204439 {
		t.Errorf("外陸資買進應為 167,204,439 股（不 ×1000），實際 %d", r.ForeignBuy)
	}
	if r.ForeignNet != 84169249 {
		t.Errorf("外陸資買賣超應為 84,169,249 股，實際 %d", r.ForeignNet)
	}
	if r.TotalNet != 91840386 {
		t.Errorf("三大法人買賣超應為 91,840,386 股，實際 %d", r.TotalNet)
	}
}

// TestTWSEWebMarketClose 全市場收盤行情：大型 payload 欄位修剪（§12）。
func TestTWSEWebMarketClose(t *testing.T) {
	src := sourceOf("market_close")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/MI_INDEX?response=json&date=20260731&type=ALL"),
		loadFixture(t, "market_close.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []MarketCloseRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列收盤行情，實際 %d", len(rows))
	}
	r := rows[0]
	if r.Code != "00400A" || r.Name != "主動國泰動能高息" {
		t.Errorf("代號/名稱錯誤: %s/%s", r.Code, r.Name)
	}
	// 官方：成交股數 102,910,202 股、成交金額 1,322,798,650 元（原值）
	if r.Volume != 102910202 {
		t.Errorf("Volume 應為 102,910,202 股，實際 %d", r.Volume)
	}
	if r.Amount != 1322798650 {
		t.Errorf("Amount 應為 1,322,798,650 元，實際 %d", r.Amount)
	}
	if r.Close != 12.94 {
		t.Errorf("Close 應為 12.94，實際 %v", r.Close)
	}
}

// TestTWSEWebIndexHistory 加權指數歷史：整月每日 OHLC。
func TestTWSEWebIndexHistory(t *testing.T) {
	src := sourceOf("index_history")
	raw := fixtureRaw(t, urlOf("/indicesReport/MI_5MINS_HIST?response=json&date=20260731"),
		loadFixture(t, "index_history.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []IndexRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 22 {
		t.Fatalf("7 月應有 22 個交易日，實際 %d", len(rows))
	}
	if rows[0].Date != "2026-07-01" || rows[0].Open != 46234.70 {
		t.Errorf("首日錯誤: %s/%v", rows[0].Date, rows[0].Open)
	}
	if rows[len(rows)-1].Date != "2026-07-31" || rows[len(rows)-1].Close != 43119.75 {
		t.Errorf("末日錯誤: %s/%v", rows[len(rows)-1].Date, rows[len(rows)-1].Close)
	}
}

// TestTWSEWebBlockTrades 鉅額交易：整月資料、股/元。
func TestTWSEWebBlockTrades(t *testing.T) {
	src := sourceOf("block_trades")
	raw := fixtureRaw(t, urlOf("/rwd/block/BFIAUU_d?response=json&date=20260731"),
		loadFixture(t, "block_trades.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []BlockTradeRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	if rows[0].Date != "2026-07-01" || rows[0].TradeType != "逐筆交易" || rows[0].Class != "單一證券" {
		t.Errorf("首列錯誤: %+v", rows[0])
	}
}

// TestTWSEWebAbnormalVolume 異常成交量（當日公布注意股票）。
func TestTWSEWebAbnormalVolume(t *testing.T) {
	src := sourceOf("abnormal_volume")
	raw := fixtureRaw(t, urlOf("/rwd/announcement/notice?response=json&date=20260731"),
		loadFixture(t, "abnormal_volume.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []AbnormalVolumeRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) == 0 {
		t.Fatal("應有注意股票列")
	}
	r := rows[0]
	if r.Code != "039457" || r.Name != "宏達電統一6A購02" {
		t.Errorf("代號/名稱錯誤: %s/%s", r.Code, r.Name)
	}
	if r.NoticeCount != 1 {
		t.Errorf("累計次數應為 1，實際 %d", r.NoticeCount)
	}
	// 官方日期 "115.07.31" → 2026-07-31
	if r.Date != "2026-07-31" {
		t.Errorf("日期應為 2026-07-31，實際 %s", r.Date)
	}
	if r.Close != 0.72 {
		t.Errorf("收盤價應為 0.72，實際 %v", r.Close)
	}
}

// TestTWSEAPIDailyClose 個股日收盤（openapi）：官方 T-1、股/元。
func TestTWSEAPIDailyClose(t *testing.T) {
	src := sourceOf("daily_close")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/exchangeReport/STOCK_DAY_ALL?date=20260731",
		loadFixture(t, "daily_close.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []DailyCloseRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	r := rows[0]
	// 官方 Date "1150730" → 2026-07-30（T-1）
	if r.Date != "2026-07-30" {
		t.Errorf("日期應為 2026-07-30，實際 %s", r.Date)
	}
	if r.Code != "00400A" || r.Volume != 65074279 || r.Close != 11.83 {
		t.Errorf("日收盤錯誤: %+v", r)
	}
}

// TestTWSEAPIForeignHoldings 外資持股：股/%。
func TestTWSEAPIForeignHoldings(t *testing.T) {
	src := sourceOf("foreign_holdings")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/fund/MI_QFIIS_cat?date=20260725",
		loadFixture(t, "foreign_holdings.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []ForeignHoldingRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	r := rows[1]
	if r.Industry != "水泥工業" || r.CompanyCount != 8 || r.Percentage != 12.21 {
		t.Errorf("類股錯誤: %+v", r)
	}
	if r.ShareNumber != 14107198706 || r.ForeignShare != 1722588340 {
		t.Errorf("股數錯誤: %d/%d", r.ShareNumber, r.ForeignShare)
	}
}

// TestTWSEAPIWarrants 權證每日成交：成交金額仟元→元、成交張數張→股。
func TestTWSEAPIWarrants(t *testing.T) {
	src := sourceOf("warrants")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/opendata/t187ap42_L",
		loadFixture(t, "warrants.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []WarrantRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	r := rows[0]
	// 官方 043396：成交金額 400.00（仟元）→ 400,000 元；成交張數 20,000（張）→ 20,000,000 股
	if r.Code != "043396" || r.TradeDate != "2026-07-30" {
		t.Errorf("權證錯誤: %+v", r)
	}
	if r.Amount != 400000 {
		t.Errorf("成交金額 400 仟元 → 400,000 元，實際 %d", r.Amount)
	}
	if r.Volume != 20000000 {
		t.Errorf("成交張數 20,000 張 → 20,000,000 股，實際 %d", r.Volume)
	}
}

// TestTWSEAPIIndices 指數（openapi MI_INDEX）。
func TestTWSEAPIIndices(t *testing.T) {
	src := sourceOf("indices")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/exchangeReport/MI_INDEX?date=20260731&type=ALL",
		loadFixture(t, "indices.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []IndexQuoteRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	r := rows[0]
	if r.IndexName != "寶島股價指數" || r.Close != 44114.49 {
		t.Errorf("指數錯誤: %+v", r)
	}
	if r.Change != 179.03 || r.ChangePercent != -0.40 {
		t.Errorf("漲跌錯誤: %v/%v", r.Change, r.ChangePercent)
	}
}

// TestTWSEAPIESG ESG（溫室氣體排放 topic=1）。
func TestTWSEAPIESG(t *testing.T) {
	src := sourceOf("esg")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/opendata/t187ap46_L_1",
		loadFixture(t, "esg.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []ESGRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	r := rows[0]
	if r.Code == "" || r.Name == "" || r.Year == "" {
		t.Errorf("ESG 基本欄位缺失: %+v", r)
	}
	if r.ReportDate != "2026-07-31" {
		t.Errorf("出表日期 1150731 應為 2026-07-31，實際 %s", r.ReportDate)
	}
	if len(r.Fields) == 0 {
		t.Error("topic 欄位不應為空")
	}
	if _, ok := r.Fields["範疇一排放量(噸CO2e)"]; !ok {
		t.Errorf("應含溫室氣體 topic 欄位，實際 keys=%v", keysOf(r.Fields))
	}
}

func keysOf(m map[string]string) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// TestTWSEAPIGovernance 公司治理。
func TestTWSEAPIGovernance(t *testing.T) {
	src := sourceOf("governance")
	raw := fixtureRaw(t, "https://openapi.twse.com.tw/v1/opendata/t187ap32_L",
		loadFixture(t, "governance.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var rows []GovernanceRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("應輸出 3 列，實際 %d", len(rows))
	}
	if rows[0].Code == "" || rows[0].Rules == "" {
		t.Errorf("公司治理列缺失: %+v", rows[0])
	}
}

// TestTWSEValidateErrors Validate 錯誤路徑。
func TestTWSEValidateErrors(t *testing.T) {
	src := sourceOf("daily_k")
	cases := []struct {
		name string
		body string
	}{
		{"非法 JSON", "{bad"},
		{"官方異常 stat", `{"stat":"系統錯誤","total":0}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY?response=json&date=20260731"), []byte(c.body))
			if err := src.Validate(raw); err == nil {
				t.Error("應回傳錯誤")
			}
		})
	}
	// 日期不一致
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/STOCK_DAY?response=json&date=20260101"),
		loadFixture(t, "day.json"))
	if err := src.Validate(raw); err == nil {
		t.Error("請求日期與回應日期不符應回傳錯誤")
	}
	// 未知路徑
	raw = fixtureRaw(t, urlOf("/rwd/unknown/ENDPOINT"), []byte(`{"stat":"OK"}`))
	if err := src.Validate(raw); err == nil {
		t.Error("未知資料集路徑應回傳錯誤")
	}
}

// TestTWSEEmptyData 官方「查無資料」回應視為合法空資料。
func TestTWSEEmptyData(t *testing.T) {
	src := sourceOf("margin")
	raw := fixtureRaw(t, urlOf("/rwd/afterTrading/MI_MARGN?response=json&date=20260731&selectType=ALL"),
		loadFixture(t, "margin_empty.json"))
	if err := src.Validate(raw); err != nil {
		t.Fatalf("空資料 Validate 應通過: %v", err)
	}
	out, err := src.Normalize(raw)
	if err != nil {
		t.Fatalf("空資料 Normalize 失敗: %v", err)
	}
	var rows []MarginRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("空資料應輸出空陣列，實際 %d 列", len(rows))
	}
}

// TestTWSEFetchContract 完整契約：httptest 伺服器（錄製 fixture）→
// BaseClient.Fetch → Validate → Normalize。
func TestTWSEFetchContract(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var name string
		switch {
		case strings.Contains(r.URL.Path, "STOCK_DAY"):
			name = "day.json"
		case strings.Contains(r.URL.Path, "MI_QFIIS_cat"):
			name = "foreign_holdings.json"
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(loadFixture(t, name))
	}))
	defer srv.Close()

	web := &TWSEWebSource{client: NewBaseClient("www.twse.com.tw", WithRateInterval(0))}
	u := srv.URL + twseWebPaths[TWSEWDDailyK] + "?response=json&date=20260731&stockNo=2330"
	raw, err := web.Fetch(context.Background(), RawRequest{URL: u})
	if err != nil {
		t.Fatalf("Fetch 失敗: %v", err)
	}
	if err := web.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	out, err := web.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}
	var cs []model.Candle
	if err := json.Unmarshal(out, &cs); err != nil {
		t.Fatal(err)
	}
	if len(cs) != 22 || cs[0].Close != 2505 {
		t.Errorf("契約 Fetch→Normalize 結果錯誤: n=%d close=%v", len(cs), cs[0].Close)
	}

	api := &TWSEAPISource{client: NewBaseClient("openapi.twse.com.tw", WithRateInterval(0))}
	u2 := srv.URL + twseAPIPaths[TWSEAPIForeignHoldings] + "?date=20260725"
	raw, err = api.Fetch(context.Background(), RawRequest{URL: u2})
	if err != nil {
		t.Fatalf("API Fetch 失敗: %v", err)
	}
	if err := api.Validate(raw); err != nil {
		t.Fatalf("API Validate 失敗: %v", err)
	}
	out, err = api.Normalize(raw)
	if err != nil {
		t.Fatalf("API Normalize 失敗: %v", err)
	}
	var rows []ForeignHoldingRow
	if err := json.Unmarshal(out, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Errorf("API 契約結果錯誤: n=%d", len(rows))
	}
}
