package mcp

import (
	"fmt"
	"sort"
	"time"

	"tw-quant-mcp/pkg/model"
)

// 盤中時段（§8.2 與 engine 內部時段常數一致）：09:00–13:30。
const (
	marketOpenSecs  = 9 * 3600
	marketCloseSecs = 13*3600 + 30*60
)

// sessionSeconds 將 Taipei 時間轉為當日秒數。
func sessionSeconds(t time.Time) int {
	return t.Hour()*3600 + t.Minute()*60 + t.Second()
}

// intradayGate 判定 now 是否為盤中時段（交易日 09:00–13:30）。
func (a *App) intradayGate(now time.Time) error {
	if !a.calendar.IsTradingDay(now) {
		return fmt.Errorf("非交易時段（%s 為非交易日）無法提供盤中資料", model.FormatDate(now))
	}
	if sec := sessionSeconds(now); sec < marketOpenSecs || sec >= marketCloseSecs {
		return fmt.Errorf("非交易時段（%s）無法提供盤中資料", now.Format("15:04:05"))
	}
	return nil
}

// requireWatchlist 確認代碼已於觀察清單（盤中資料皆為記憶體資料）。
func (a *App) requireWatchlist(code string) error {
	if _, ok := a.watchlist.Lookup(code); !ok {
		return fmt.Errorf("代碼 %s 未在觀察清單（請先使用 set_active_watchlist）", code)
	}
	return nil
}

// handlerSetActiveWatchlist：設定盤中監控清單（§8.2，1..15 檔）。
func handlerSetActiveWatchlist(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	raw, _ := args["symbols"].([]any)
	symbols := make([]model.Symbol, 0, len(raw))
	for _, v := range raw {
		code, ok := v.(string)
		if !ok {
			return HandlerResult{}, fmt.Errorf("參數 symbols 每個元素必須為字串")
		}
		sym, ok := a.symbols.Lookup(code)
		if !ok {
			return HandlerResult{}, fmt.Errorf("非法代號 %q（未註冊於 Symbol Registry）", code)
		}
		symbols = append(symbols, sym)
	}
	if err := a.watchlist.Set(symbols); err != nil {
		return HandlerResult{}, err
	}
	codes := make([]string, 0, len(symbols))
	for _, s := range symbols {
		codes = append(codes, s.Code)
	}
	sort.Strings(codes)
	return HandlerResult{Data: map[string]any{
		"status":       "ok",
		"symbols":      codes,
		"count":        len(codes),
		"sampling_sec": 8,
	}}, nil
}

// handlerGetIntradayKline：純記憶體 1m/5m K 線重採樣（§8.4）。
func handlerGetIntradayKline(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	code, _ := args["symbol"].(string)
	if err := a.requireWatchlist(code); err != nil {
		return HandlerResult{}, err
	}
	tf, _ := args["timeframe"].(string)
	if tf == "" {
		tf = "1m"
	}
	limit := DefaultChartOption().Limit
	if v, ok := args["limit"]; ok {
		if n, err := asInt(v); err == nil {
			limit = n
		}
	}
	candles, err := a.agg.Klines(code, tf, limit)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: candles}, nil
}

// handlerGetIntradayQuote：最新快照報價 + 五檔（記憶體讀取）。
func handlerGetIntradayQuote(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	code, _ := args["symbol"].(string)
	if err := a.requireWatchlist(code); err != nil {
		return HandlerResult{}, err
	}
	snaps := a.rings.Snapshots(code)
	if len(snaps) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 目前無盤中快照（請先加入 watchlist 並等待採樣）", code)
	}
	s := snaps[len(snaps)-1]
	q := model.IntradayQuote{
		Symbol:    s.Code,
		Date:      model.FormatDate(s.Time.Time),
		Time:      s.Time.Time.Format("15:04:05"),
		TradeTime: s.TradeTime,
		Last:      s.Last,
		Change:    s.Change,
		Open:      s.Open,
		High:      s.High,
		Low:       s.Low,
		PrevClose: s.PrevClose,
		Volume:    s.CumulativeVol,
		MinuteVol: s.MinuteVol,
	}
	if s.Last != 0 {
		q.ChangePct = s.Change / s.PrevClose * 100
	}
	if s.Book != nil {
		q.Bids = s.Book.Bids
		q.Asks = s.Book.Asks
	}
	return HandlerResult{Data: q}, nil
}

// handlerGetIntradayVWAP：當日 VWAP / 高低點 / 支撐壓力（§8.5）。
func handlerGetIntradayVWAP(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	code, _ := args["symbol"].(string)
	if err := a.requireWatchlist(code); err != nil {
		return HandlerResult{}, err
	}
	vwap, err := a.intraday.VWAP(code)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: vwap}, nil
}

// handlerDetectVolumeSurge：近 N 分鐘爆量偵測（§8.5）。
func handlerDetectVolumeSurge(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	code, _ := args["symbol"].(string)
	if err := a.requireWatchlist(code); err != nil {
		return HandlerResult{}, err
	}
	minutes := 5
	if v, ok := args["minutes"]; ok {
		if n, err := asInt(v); err == nil {
			minutes = n
		}
	}
	surge, err := a.agg.Surge(code, minutes)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: surge}, nil
}

// handlerScanDaytradeEligibility：買前風險掃描（注意/處置/停資停券）。
func handlerScanDaytradeEligibility(a *App, args map[string]any) (HandlerResult, error) {
	if err := a.intradayGate(a.now()); err != nil {
		return HandlerResult{}, err
	}
	code, _ := args["symbol"].(string)
	scan, err := a.risk.Scan(code)
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{Data: scan}, nil
}
