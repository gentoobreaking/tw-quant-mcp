package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"tw-quant-mcp/pkg/engine"
	"tw-quant-mcp/pkg/model"
)

// MIS 端點（§8.3）。以變數而非 const 提供，供測試以 httptest 注入。
var (
	misIndexURL = "https://mis.twse.com.tw/stock/index.jsp"
	misQuoteURL = "https://mis.twse.com.tw/stock/api/getStockInfo.jsp"
)

// 失敗與重試門檻（§8.3：連續 5 tick 失敗 → DEGRADED，30s 重試）。
const (
	maxConsecutiveFailures = 5
	degradedRetryInterval  = 30 * time.Second
	idleCheckInterval      = 30 * time.Second
)

// MISWorker 是 MIS 盤中採樣 Poller（§8.3）：
//   - 每 tick（8s ±1s Jitter，Jitter 由 BaseClient HostLimiter 置於請求前）
//     單一 GET 請求 ex_ch=tse_2330.tw|otc_6547.tw|...（ex_ch 由 Watchlist
//     依 Symbol Registry 組裝）
//   - Session 預熱：啟動與每日開盤前 GET index.jsp 取 Cookie（cookiejar 自動
//     儲存/回送）；index.jsp 改版 404 時僅記錄、不阻斷
//   - 403/429 指數退避由 BaseClient（§4.4）處理
//   - 連續 5 tick 失敗 → Watchlist 狀態轉 DEGRADED，改為 30s 重試並記錄 Log
//   - 選用：盤中衍生計算（§8.5 VWAP/爆量）於寫入 RingBuffer 後以 best-effort
//     增量更新 IntradayStore（純記憶體、零 HTTP，不影響 Poller 寫入）
type MISWorker struct {
	client    *BaseClient
	watchlist *engine.Watchlist
	rings     *engine.RingStore
	intraday  *engine.IntradayStore
	indexURL  string
	quoteURL  string
	now       func() time.Time
	tick      time.Duration
	idleCheck time.Duration
	degraded  time.Duration
	sleep     sleepFunc
	logger    *slog.Logger

	mu       sync.Mutex
	failures int
}

// MISOption 為 MISWorker 建置選項。
type MISOption func(*MISWorker)

// WithMISClock 注入時鐘（測試用）。
func WithMISClock(now func() time.Time) MISOption {
	return func(w *MISWorker) { w.now = now }
}

// WithMISTick 覆寫採樣間隔（測試用；預設 engine.SamplingInterval）。
func WithMISTick(d time.Duration) MISOption {
	return func(w *MISWorker) { w.tick = d }
}

// WithMISIdleCheck 覆寫 IDLE 檢查間隔（測試用）。
func WithMISIdleCheck(d time.Duration) MISOption {
	return func(w *MISWorker) { w.idleCheck = d }
}

// WithMISDegradedRetry 覆寫 DEGRADED 重試間隔（測試用；預設 30s）。
func WithMISDegradedRetry(d time.Duration) MISOption {
	return func(w *MISWorker) { w.degraded = d }
}

// WithMISURLs 覆寫 MIS 端點（測試用 httptest 注入）。
func WithMISURLs(index, quote string) MISOption {
	return func(w *MISWorker) {
		w.indexURL = index
		w.quoteURL = quote
	}
}

// WithMISSleep 注入等待實作（測試用）。
func WithMISSleep(fn sleepFunc) MISOption {
	return func(w *MISWorker) { w.sleep = fn }
}

// WithMISLogger 注入 slog logger（預設 discard）。
func WithMISLogger(l *slog.Logger) MISOption {
	return func(w *MISWorker) { w.logger = l }
}

// WithMISIntraday 注入盤中衍生計算登錄（§8.5）；未注入時跳過衍生計算。
func WithMISIntraday(s *engine.IntradayStore) MISOption {
	return func(w *MISWorker) { w.intraday = s }
}

// NewMISWorker 建立 MIS Poller。client 須以主機 "mis.twse.com.tw" 建立
// （§4.4 1 req/8s ±1s jitter 由該主機之 HostLimiter 提供）。
func NewMISWorker(client *BaseClient, watchlist *engine.Watchlist, rings *engine.RingStore, opts ...MISOption) *MISWorker {
	w := &MISWorker{
		client:    client,
		watchlist: watchlist,
		rings:     rings,
		indexURL:  misIndexURL,
		quoteURL:  misQuoteURL,
		now:       func() time.Time { return model.Now().Time },
		tick:      engine.SamplingInterval,
		idleCheck: idleCheckInterval,
		degraded:  degradedRetryInterval,
		sleep:     sleepCtx,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Run 是採樣主迴圈（§8.2/8.3）：依 Watchlist 狀態機驅動。ctx 取消時回傳 nil。
func (w *MISWorker) Run(ctx context.Context) error {
	warmedUp := false
	lastDay := ""
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		now := w.now()
		st := w.watchlist.Advance(now)
		if w.watchlist.Len() == 0 || st == engine.StateIDLE {
			// IDLE：非交易日或盤外；清零失敗計數與 DEGRADED（次日重新開始）
			w.resetFailures()
			w.watchlist.MarkHealthy()
			warmedUp = false
			if err := w.sleep(ctx, w.idleCheck); err != nil {
				return nil
			}
			continue
		}

		// 重啟日清零：進入新交易日首次採樣前清空 RingBuffer（§8.4）
		if day := model.FormatDate(now); day != lastDay {
			w.rings.Reset()
			lastDay = day
		}

		// Session 預熱：啟動與每日開盤前（WARMUP 窗口內或首次進入盤中）
		if !warmedUp {
			if err := w.warmupSession(ctx); err != nil {
				w.logger.Warn("MIS session 預熱失敗（cookie 未取得，繼續採樣）", "err", err)
			}
			warmedUp = true
		}
		if st == engine.StateWARMUP {
			// 開盤前窗口：僅預熱，不採樣
			if err := w.sleep(ctx, w.tick); err != nil {
				return nil
			}
			continue
		}

		// SAMPLING / FLUSH / DEGRADED：採樣並寫入 RingBuffer
		if _, err := w.pollAndStore(ctx); err != nil {
			w.fail()
			wait := w.tick
			if st == engine.StateDEGRADED {
				wait = w.degraded
			}
			w.logger.Warn("MIS 採樣失敗", "state", st, "failures", w.failures, "err", err)
			if err := w.sleep(ctx, wait); err != nil {
				return nil
			}
			continue
		}
		w.resetFailures()
		w.watchlist.MarkHealthy()
		if err := w.sleep(ctx, w.tick); err != nil {
			return nil
		}
	}
}

// warmupSession 執行 Session 預熱（§8.3：GET index.jsp 取 Cookie）。
// index.jsp 回應異常（如官方改版 404）不阻斷：cookie 由 cookiejar 維護，
// 取不到時 MIS 仍可正常回應。
func (w *MISWorker) warmupSession(ctx context.Context) error {
	resp, err := w.client.Do(ctx, RawRequest{URL: w.indexURL})
	if err != nil {
		return fmt.Errorf("provider: MIS session 預熱失敗: %w", err)
	}
	if resp.StatusCode >= 400 {
		w.logger.Warn("MIS index.jsp 回應異常", "status", resp.StatusCode)
	}
	return nil
}

// pollAndStore 抓取目前 watchlist 之快照批次並寫入 RingStore。
func (w *MISWorker) pollAndStore(ctx context.Context) ([]model.Snapshot, error) {
	u := fmt.Sprintf("%s?ex_ch=%s&_=%d", w.quoteURL,
		url.QueryEscape(w.watchlist.ExCh()), w.now().UnixMilli())
	resp, err := w.client.Do(ctx, RawRequest{URL: u})
	if err != nil {
		return nil, err
	}
	snaps, err := parseMIS(resp.Body)
	if err != nil {
		return nil, err
	}
	for _, s := range snaps {
		w.rings.Append(s)
	}
	// 盤中衍生計算（§8.5）：純記憶體增量更新；計算失敗不影響 Poller 寫入
	if w.intraday != nil {
		w.intraday.UpdateAll(snaps)
	}
	return snaps, nil
}

func (w *MISWorker) fail() {
	w.mu.Lock()
	w.failures++
	if w.failures >= maxConsecutiveFailures {
		w.watchlist.MarkDegraded()
	}
	w.mu.Unlock()
}

func (w *MISWorker) resetFailures() {
	w.mu.Lock()
	w.failures = 0
	w.mu.Unlock()
}

// misResponse 為 getStockInfo.jsp 回應之頂層結構。
type misResponse struct {
	Rtcode   string     `json:"rtcode"` // "0000" = OK
	MsgArray []misEntry `json:"msgArray"`
}

// misEntry 僅宣告 T006/T010 所需原生欄位（§8.3）。
type misEntry struct {
	Code  string `json:"c"`  // 代碼（注意：MIS 之 c 為代號，非漲跌）
	Ch    string `json:"ch"` // "2330.tw"
	Ex    string `json:"ex"` // "tse" | "otc"
	Z     string `json:"z"`  // 成交價（元，4 位小數）
	O     string `json:"o"`  // 開盤價
	H     string `json:"h"`  // 最高價
	L     string `json:"l"`  // 最低價
	Y     string `json:"y"`  // 昨收
	V     string `json:"v"`  // 當日累積成交量（張）
	Tv    string `json:"tv"` // 當分鐘內累積成交量（張，每分鐘重置）
	Tlong string `json:"tlong"`
	T     string `json:"t"` // 最近成交時刻 "HH:MM:SS"

	// T010：五檔買賣價量（MIS 原生為 "_" 分隔字串，缺檔或無報價為 "-"）
	B string `json:"b"` // 五檔買價（由高至低）
	G string `json:"g"` // 五檔買量（張）
	A string `json:"a"` // 五檔賣價（由低至高）
	F string `json:"f"` // 五檔賣量（張）
}

// parseMIS 解析並正規化 MIS 回應為 []model.Snapshot。
// 個別無效記錄略過（容錯官方格式雜訊）；全滅或 rtcode 異常時回傳錯誤。
func parseMIS(body []byte) ([]model.Snapshot, error) {
	var r misResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("provider: MIS 回應 JSON 解析失敗: %w", err)
	}
	if r.Rtcode != "0000" {
		return nil, fmt.Errorf("provider: MIS 回應異常（rtcode=%q）", r.Rtcode)
	}
	if len(r.MsgArray) == 0 {
		return nil, fmt.Errorf("provider: MIS 回應無 msgArray（watchlist 可能為空）")
	}
	out := make([]model.Snapshot, 0, len(r.MsgArray))
	for _, e := range r.MsgArray {
		if s, ok := normalizeMIS(e); ok {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("provider: MIS 回應無有效快照（官方格式可能變更）")
	}
	return out, nil
}

// normalizeMIS 將單筆 MIS 原生欄位轉換為 model.Snapshot（§8.3/§5.1）：
// z/o/h/l/y 元（2 位）、tv/v 張 ×1000 → 股、tlong 毫秒 → Asia/Taipei。
// z/tlong/tv 為必要欄位（重採樣核心）；o/h/l/y 缺漏時以 0 輸出。
func normalizeMIS(e misEntry) (model.Snapshot, bool) {
	code := strings.TrimSpace(e.Code)
	if code == "" || strings.TrimSpace(e.Z) == "" || strings.TrimSpace(e.Tlong) == "" ||
		strings.TrimSpace(e.Tv) == "" {
		return model.Snapshot{}, false
	}
	last, ok := parsePrice(e.Z)
	if !ok {
		return model.Snapshot{}, false
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(e.Tlong), 10, 64)
	if err != nil || ms <= 0 {
		return model.Snapshot{}, false
	}
	minuteVol, ok := parseVol(e.Tv)
	if !ok {
		return model.Snapshot{}, false
	}

	s := model.Snapshot{
		Code:          code,
		Exch:          strings.TrimSpace(e.Ex) + "_" + strings.TrimSpace(e.Ch),
		Time:          model.NewTaipeiTime(time.UnixMilli(ms)),
		TradeTime:     strings.TrimSpace(e.T),
		Last:          last,
		MinuteVol:     minuteVol,
		CumulativeVol: parseVolOrZero(e.V),
	}
	if v, ok := parsePrice(e.O); ok {
		s.Open = v
	}
	if v, ok := parsePrice(e.H); ok {
		s.High = v
	}
	if v, ok := parsePrice(e.L); ok {
		s.Low = v
	}
	if v, ok := parsePrice(e.Y); ok {
		s.PrevClose = v
	}
	s.Change = math.Round((s.Last-s.PrevClose)*100) / 100
	if book := parseBook(e.B, e.G, e.A, e.F); book != nil {
		s.Book = book
	}
	return s, true
}

// parseBook 將 MIS 五檔字串（b/g/a/f，"_" 分隔，單位 張）轉為
// model.LevelBook（股）。任一側全部無效時該側為空；兩側皆空回傳 nil。
func parseBook(b, g, a, f string) *model.LevelBook {
	bidPrices, bidVols := splitLevels(b), splitLevels(g)
	askPrices, askVols := splitLevels(a), splitLevels(f)
	bid := make([]model.PriceLevel, 0, len(bidPrices))
	for i, p := range bidPrices {
		price, ok := parsePrice(p)
		if !ok {
			continue
		}
		bid = append(bid, model.PriceLevel{Price: price, Volume: parseVolOrZero(at(bidVols, i))})
	}
	ask := make([]model.PriceLevel, 0, len(askPrices))
	for i, p := range askPrices {
		price, ok := parsePrice(p)
		if !ok {
			continue
		}
		ask = append(ask, model.PriceLevel{Price: price, Volume: parseVolOrZero(at(askVols, i))})
	}
	if len(bid) == 0 && len(ask) == 0 {
		return nil
	}
	return &model.LevelBook{Bids: bid, Asks: ask}
}

// splitLevels 將 MIS "_" 分隔之五檔字串切為 slice；"-"/空字串回傳 nil。
func splitLevels(s string) []string {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" {
		return nil
	}
	return strings.Split(t, "_")
}

// at 安全取陣列元素；越界回傳空字串。
func at(ss []string, i int) string {
	if i < 0 || i >= len(ss) {
		return ""
	}
	return ss[i]
}

// parsePrice 解析價格字串為「元」（2 位小數）；"-"/空字串/非法回傳 ok=false。
func parsePrice(s string) (float64, bool) {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" {
		return 0, false
	}
	f, err := strconv.ParseFloat(t, 64)
	if err != nil {
		return 0, false
	}
	return math.Round(f*100) / 100, true
}

// parseVol 解析張數並 ×1000 換算為股（§5.1）；"-"/空字串/非法回傳 ok=false。
func parseVol(s string) (int64, bool) {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" {
		return 0, false
	}
	v, err := strconv.ParseInt(t, 10, 64)
	if err != nil {
		return 0, false
	}
	return v * 1000, true
}

func parseVolOrZero(s string) int64 {
	v, ok := parseVol(s)
	if !ok {
		return 0
	}
	return v
}
