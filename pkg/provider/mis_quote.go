package provider

// mis_quote.go：MIS 單發直查即時報價（T194，get_realtime_quote）。
//
// 與 MISWorker（§8.3 watchlist 8 秒輪詢引擎）完全解耦：
//   - 任意多檔、即查即走，不佔 watchlist 名額、不觸發 RingBuffer 寫入
//   - 前綴策略：Symbol Registry 可判定市場者直接使用；未註冊代號先試
//     tse_，缺漏者再以 otc_ 重試一次（對齊遠端 TWSEMCPServer 行為）
//   - 盤後/未成交（z="-"）以昨收 y fallback，並標註 price_source

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
)

// RealtimeQuote 為單發即時報價。內嵌 model.IntradayQuote（欄位與
// get_intraday_quote 完全一致，含五檔 bids/asks），另標註價格來源。
type RealtimeQuote struct {
	model.IntradayQuote
	// Name 為證券名稱（MIS msgArray 之 "n" 欄；IntradayQuote 無此欄）。
	Name string `json:"name,omitempty"`
	// PriceSource 標示 Last 之來源："trade"=盤中最新成交價；
	// "prev_close_fallback"=無成交（盤後或暫停），以昨收價替代。
	PriceSource string `json:"price_source"`
}

// FetchRealtimeQuotes 以單發 GET 查詢任意多檔即時報價（T194）。
//   - client：須為主機 mis.twse.com.tw 之 BaseClient（HostLimiter 節流）
//   - marketOf：code → "tse" | "otc" | ""（未註冊；先試 tse_ 後補 otc_）
//   - 回傳值第二項為實際發出之上游 HTTP 請求數（1 或 2，供 §12.9 instrumentation）
func FetchRealtimeQuotes(ctx context.Context, client *BaseClient,
	marketOf func(string) string, codes []string) ([]RealtimeQuote, int, error) {
	if len(codes) == 0 {
		return nil, 0, fmt.Errorf("provider: codes 不得為空")
	}
	prefix := func(code string) string {
		if marketOf != nil && marketOf(code) == "otc" {
			return "otc"
		}
		return "tse"
	}
	exCh := make([]string, 0, len(codes))
	for _, c := range codes {
		exCh = append(exCh, prefix(c)+"_"+c+".tw")
	}
	entries, reqs, err := fetchMISBatch(ctx, client, exCh)
	if err != nil {
		return nil, reqs, err
	}
	byCode := make(map[string]misEntry, len(entries))
	for _, e := range entries {
		byCode[strings.TrimSpace(e.Code)] = e
	}

	// 缺漏重試：未取得有效資料之代碼，翻轉前綴再試一次
	var retry []string
	for _, c := range codes {
		e, ok := byCode[c]
		if ok && entryQuotable(e) {
			continue
		}
		p := "tse"
		if prefix(c) == "tse" {
			p = "otc"
		}
		retry = append(retry, p+"_"+c+".tw")
	}
	if len(retry) > 0 {
		retryEntries, r2, err := fetchMISBatch(ctx, client, retry)
		reqs += r2
		if err != nil {
			return nil, reqs, err
		}
		for _, e := range retryEntries {
			if _, dup := byCode[strings.TrimSpace(e.Code)]; !dup || !entryQuotable(byCode[strings.TrimSpace(e.Code)]) {
				byCode[strings.TrimSpace(e.Code)] = e
			}
		}
	}

	out := make([]RealtimeQuote, 0, len(codes))
	for _, c := range codes {
		e, ok := byCode[c]
		if !ok || !entryQuotable(e) {
			continue
		}
		if q, ok := realtimeQuoteFromEntry(e); ok {
			out = append(out, q)
		}
	}
	if len(out) == 0 {
		return nil, reqs, fmt.Errorf("provider: MIS 回應查無 %s 之即時報價", strings.Join(codes, ","))
	}
	return out, reqs, nil
}

// entryQuotable 判定 msgArray 項目是否可用（有成交價或昨收皆可——盤後回最後成交價/昨收）。
func entryQuotable(e misEntry) bool {
	return strings.TrimSpace(e.Z) != "" && e.Z != "-" ||
		strings.TrimSpace(e.Y) != "" && e.Y != "-"
}

// fetchMISBatch 發出單一 getStockInfo.jsp 請求並解析 msgArray。
func fetchMISBatch(ctx context.Context, client *BaseClient, exCh []string) ([]misEntry, int, error) {
	u := fmt.Sprintf("%s?ex_ch=%s&json=1&delay=0&_=%d", misQuoteURL,
		url.QueryEscape(strings.Join(exCh, "|")), time.Now().UnixMilli())
	resp, err := client.Do(ctx, RawRequest{URL: u})
	if err != nil {
		return nil, 1, err
	}
	var r misResponse
	if err := json.Unmarshal(resp.Body, &r); err != nil {
		return nil, 1, fmt.Errorf("provider: MIS 回應 JSON 解析失敗: %w", err)
	}
	if r.Rtcode != "0000" {
		return nil, 1, fmt.Errorf("provider: MIS 回應異常（rtcode=%q）", r.Rtcode)
	}
	return r.MsgArray, 1, nil
}

// realtimeQuoteFromEntry 將 msgArray 項目正規化為 RealtimeQuote。
// 寬鬆版 normalizeMIS：不要求當分鐘成交量（tv）/tlong（盤後可能缺）。
func realtimeQuoteFromEntry(e misEntry) (RealtimeQuote, bool) {
	code := strings.TrimSpace(e.Code)
	if code == "" {
		return RealtimeQuote{}, false
	}
	last, lastOK := parsePrice(e.Z)
	prev, prevOK := parsePrice(e.Y)
	if !lastOK && !prevOK {
		return RealtimeQuote{}, false
	}
	q := RealtimeQuote{PriceSource: "trade"}
	q.Symbol = code
	q.Name = strings.TrimSpace(e.N)
	if lastOK {
		q.Last = last
	} else {
		q.Last = prev // 盤後/未成交 fallback：以昨收替代並標註
		q.PriceSource = "prev_close_fallback"
	}
	if prevOK {
		q.PrevClose = prev
	}
	if v, ok := parsePrice(e.O); ok {
		q.Open = v
	}
	if v, ok := parsePrice(e.H); ok {
		q.High = v
	}
	if v, ok := parsePrice(e.L); ok {
		q.Low = v
	}
	q.Change = math.Round((q.Last-q.PrevClose)*100) / 100
	if q.PrevClose > 0 {
		q.ChangePct = math.Round(q.Change/q.PrevClose*10000) / 100
	}
	q.Volume = parseVolOrZero(e.V)
	q.TradeTime = strings.TrimSpace(e.T)
	if ms, err := strconv.ParseInt(strings.TrimSpace(e.Tlong), 10, 64); err == nil && ms > 0 {
		t := model.NewTaipeiTime(time.UnixMilli(ms))
		q.Date = model.FormatDate(t.Time)
		q.Time = t.Time.Format("15:04:05")
	} else {
		q.Time = q.TradeTime
	}
	if book := parseBook(e.B, e.G, e.A, e.F); book != nil {
		q.Bids = book.Bids
		q.Asks = book.Asks
	}
	return q, true
}
