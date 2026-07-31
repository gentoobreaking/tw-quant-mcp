package engine

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

func sym(code, market, name string) model.Symbol {
	return model.Symbol{Code: code, Market: market, Name: name}
}

// tradingDay 為 2026-07-31（週五，T005 行事曆之交易日）。
func tradingDay(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 7, 31, 0, 0, 0, 0, model.Taipei())
}

func alwaysTrading(time.Time) bool { return true }

func neverTrading(time.Time) bool { return false }

func at(h, m, s int) time.Time {
	return time.Date(2026, 7, 31, h, m, s, 0, model.Taipei())
}

// 容量硬上限 15 與覆寫式更新（§8.2）。
func TestWatchlistSetLimit(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	symbols := make([]model.Symbol, 15)
	for i := range symbols {
		symbols[i] = sym(fmt.Sprintf("%d", 2330+i), "tse", "台積電")
	}
	if err := w.Set(symbols); err != nil {
		t.Fatalf("15 檔應允許: %v", err)
	}
	if w.Len() != 15 {
		t.Fatalf("Len 應為 15，實際 %d", w.Len())
	}

	if err := w.Set(make([]model.Symbol, 16)); err == nil {
		t.Error("16 檔應回傳錯誤（§8.2 硬限制）")
	}
	if err := w.Set(nil); err == nil {
		t.Error("空清單應回傳錯誤")
	}
}

func TestWatchlistSetInvalid(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	if err := w.Set([]model.Symbol{{Code: "2330"}}); err == nil {
		t.Error("缺 market/name 之 Symbol 應回傳錯誤")
	}
}

func TestWatchlistOverwrite(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	if err := w.Set([]model.Symbol{sym("2330", "tse", "台積電")}); err != nil {
		t.Fatal(err)
	}
	if err := w.Set([]model.Symbol{sym("6547", "otc", "高端疫苗")}); err != nil {
		t.Fatal(err)
	}
	if w.Len() != 1 {
		t.Fatalf("覆寫後應僅 1 檔，實際 %d", w.Len())
	}
	if _, ok := w.Lookup("6547"); !ok {
		t.Error("覆寫後應含 6547")
	}
	if _, ok := w.Lookup("2330"); ok {
		t.Error("覆寫後不應含 2330")
	}
}

// ExCh 組裝（§8.3）：市場別一律來自 Symbol（tse_/otc_ 前綴）。
func TestWatchlistExCh(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	if err := w.Set([]model.Symbol{sym("6547", "otc", "高端疫苗"), sym("2330", "tse", "台積電")}); err != nil {
		t.Fatal(err)
	}
	got := w.ExCh()
	if got != "otc_6547.tw|tse_2330.tw" {
		t.Errorf("ExCh 應為 otc_6547.tw|tse_2330.tw（依代碼排序），實際 %q", got)
	}
	if strings.Contains(got, "tse_6547") || strings.Contains(got, "otc_2330") {
		t.Error("市場別不得猜測或混淆")
	}
}

// 狀態機（§8.2）：IDLE → WARMUP → SAMPLING → FLUSH → IDLE。
func TestWatchlistAdvance(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	cases := []struct {
		now  time.Time
		want State
	}{
		{at(8, 0, 0), StateIDLE},
		{at(8, 59, 29), StateIDLE},
		{at(8, 59, 30), StateWARMUP}, // 09:00 ±30s 開盤前窗口
		{at(8, 59, 59), StateWARMUP},
		{at(9, 0, 0), StateSAMPLING},
		{at(9, 30, 15), StateSAMPLING},
		{at(13, 29, 59), StateSAMPLING},
		{at(13, 30, 0), StateFLUSH},
		{at(13, 34, 59), StateFLUSH},
		{at(13, 35, 0), StateIDLE},
		{at(23, 59, 59), StateIDLE},
	}
	for _, c := range cases {
		if got := w.Advance(c.now); got != c.want {
			t.Errorf("%s 狀態應為 %s，實際 %s", c.now.Format("15:04:05"), c.want, got)
		}
	}
}

// 非交易日：恆為 IDLE，Poller 不啟動（依 T005 行事曆判定）。
func TestWatchlistAdvanceNonTradingDay(t *testing.T) {
	w := NewWatchlist(neverTrading)
	for _, h := range []time.Time{at(9, 0, 0), at(10, 0, 0), at(13, 30, 0)} {
		if got := w.Advance(h); got != StateIDLE {
			t.Errorf("非交易日 %s 應為 IDLE，實際 %s", h.Format("15:04:05"), got)
		}
	}
}

// DEGRADED（§8.3）：連續 5 tick 失敗後盤中回傳 DEGRADED；成功後恢復。
func TestWatchlistDegraded(t *testing.T) {
	w := NewWatchlist(alwaysTrading)
	w.MarkDegraded()
	if got := w.Advance(at(10, 0, 0)); got != StateDEGRADED {
		t.Errorf("盤中 + degraded 應為 DEGRADED，實際 %s", got)
	}
	w.MarkHealthy()
	if got := w.Advance(at(10, 0, 0)); got != StateSAMPLING {
		t.Errorf("恢復後應為 SAMPLING，實際 %s", got)
	}
	// 盤外時段不因 degraded 而誤報
	w.MarkDegraded()
	if got := w.Advance(at(14, 0, 0)); got != StateIDLE {
		t.Errorf("盤外 + degraded 應為 IDLE，實際 %s", got)
	}
}
