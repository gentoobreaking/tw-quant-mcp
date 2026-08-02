package normalize

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"tw-quant-mcp/pkg/model"
)

// T022 驗收：FromMIS / FromTWSEWeb 以官方 golden fixtures 驅動
//（複製自 pkg/provider/testdata，2026-07-31/08-01 錄製之官方 raw 原文）。

func readFixture(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", rel))
	if err != nil {
		t.Fatalf("讀取 fixture %s 失敗: %v", rel, err)
	}
	return b
}

// FromMIS → KlineBar：tick bar 轉換與單位換算（tv 張 ×1000 → 股，z 2 位小數）。
func TestFromMISToKlineBar(t *testing.T) {
	bars, err := FromMIS(readFixture(t, "mis/tick_01.json"))
	if err != nil {
		t.Fatalf("FromMIS 失敗: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("應轉出 2 根 tick bar，實際 %d", len(bars))
	}

	a := bars[0]
	if a.Timestamp != "14:30:00" {
		t.Errorf("tlong 應轉為 14:30:00（Asia/Taipei），實際 %q", a.Timestamp)
	}
	// tick bar：OHLC = 成交價 z=2425（2 位小數）
	for name, v := range map[string]float64{"Open": a.Open, "High": a.High, "Low": a.Low, "Close": a.Close} {
		if v != 2425 {
			t.Errorf("%s 應為 2425，實際 %v", name, v)
		}
	}
	// 單位換算：tv=4512 張 → 4,512,000 股
	if a.Volume != 4512000 {
		t.Errorf("Volume(tv×1000) 應為 4,512,000 股，實際 %d", a.Volume)
	}

	b := bars[1]
	if b.Close != 45.8 {
		t.Errorf("6547 收盤應為 45.8，實際 %v", b.Close)
	}
	// tv=93 張 → 93,000 股
	if b.Volume != 93000 {
		t.Errorf("6547 Volume 應為 93,000 股，實際 %d", b.Volume)
	}
}

// FromTWSEWeb → InstitutionalFlow：T86 日報轉換（千分位逗號、市場/日期、lineage）。
func TestFromTWSEWebToInstitutionalFlow(t *testing.T) {
	flows, err := FromTWSEWeb(readFixture(t, "twse/institutional.json"))
	if err != nil {
		t.Fatalf("FromTWSEWeb 失敗: %v", err)
	}
	if len(flows) != 3 {
		t.Fatalf("T86 fixture 應有 3 列，實際 %d", len(flows))
	}

	// StockIdentity 正確性 + 市場標註（T86 僅涵蓋上市）
	one := flows[0]
	if one.Stock.Symbol != "00685L" || one.Stock.Name != "群益臺灣加權正2" {
		t.Errorf("StockIdentity 錯誤: %+v", one.Stock)
	}
	if one.Stock.Market != "TSE" || one.Market != "TSE" {
		t.Errorf("market 應為 TSE，實際 %q/%q", one.Stock.Market, one.Market)
	}

	// 千分位逗號移除 + 負數：外陸資買賣超 -2,484,521
	if one.ForeignNetShares != -2484521 {
		t.Errorf("foreign_net_shares 應為 -2,484,521，實際 %d", one.ForeignNetShares)
	}
	if one.DealerNetShares != 152261703 {
		t.Errorf("dealer_net_shares 應為 152,261,703，實際 %d", one.DealerNetShares)
	}

	// 0050 元大台灣50：投信 +2,262,000
	five := flows[1]
	if five.Stock.Symbol != "0050" || five.Stock.Name != "元大台灣50" {
		t.Errorf("StockIdentity 錯誤: %+v", five.Stock)
	}
	if five.ForeignNetShares != 84169249 || five.TrustNetShares != 2262000 || five.DealerNetShares != 5409137 {
		t.Errorf("0050 三大法人買賣超錯誤: %+v", five)
	}

	// 日期：raw "date":"20260731" → "2026-07-31"
	if five.Date != "2026-07-31" {
		t.Errorf("date 應為 2026-07-31，實際 %q", five.Date)
	}

	// Lineage：TWSE_WEB / CANONICAL / POST_MARKET，data_date 同步
	lg := five.Lineage
	if lg.Source != model.SourceTWSEWeb || lg.SourceRole != model.SourceRoleCanonical ||
		lg.Freshness != model.FreshnessPostMarket {
		t.Errorf("lineage 錯誤: %+v", lg)
	}
	if lg.DataDate != "2026-07-31" {
		t.Errorf("lineage data_date 應為 2026-07-31，實際 %q", lg.DataDate)
	}

	// ForeignHoldingPct（來自 QFIIS）不得輸出
	if five.ForeignHoldingPct != 0 {
		t.Errorf("foreign_holding_pct 應為 0（QFIIS 另行提供），實際 %v", five.ForeignHoldingPct)
	}
}

// 錯誤路徑：rtcode 異常 / 無資料列。
func TestFromSourceErrorPaths(t *testing.T) {
	if _, err := FromMIS([]byte(`{"rtcode":"9000","msgArray":[]}`)); err == nil {
		t.Error("rtcode 異常應回傳錯誤")
	}
	if _, err := FromMIS([]byte(`{"rtcode":"0000","msgArray":[]}`)); err == nil {
		t.Error("無 msgArray 應回傳錯誤")
	}
	if _, err := FromTWSEWeb([]byte(`{"stat":"NG","data":[]}`)); err == nil {
		t.Error("stat 異常應回傳錯誤")
	}
	if _, err := FromTWSEWeb([]byte(`{"stat":"OK","data":[]}`)); err == nil {
		t.Error("無資料列應回傳錯誤")
	}
}

// 未實作路徑：回傳 ErrNotImplemented（T022 骨架，T026/T027 填實）。
func TestFromSourceNotImplemented(t *testing.T) {
	for name, fn := range map[string]func() error{
		"FromTWSEOpenAPI":   func() error { _, err := FromTWSEOpenAPI(nil); return err },
		"FromTPEx":          func() error { _, err := FromTPEx(nil); return err },
		"FromMOPS":          func() error { _, err := FromMOPS(nil); return err },
		"FromTAIFEXOpenAPI": func() error { _, err := FromTAIFEXOpenAPI(nil); return err },
		"FromTAIFEXDownload": func() error {
			_, err := FromTAIFEXDownload(nil)
			return err
		},
	} {
		if err := fn(); !errors.Is(err, ErrNotImplemented) {
			t.Errorf("%s 應回傳 ErrNotImplemented，實際 %v", name, err)
		}
	}
}

// tlong 轉換之時間基準驗證（tick_01：1785479400000 ms = 2026-07-31 14:30:00 +08）。
func TestMISTlongConversion(t *testing.T) {
	bars, err := FromMIS(readFixture(t, "mis/tick_01.json"))
	if err != nil {
		t.Fatalf("FromMIS 失敗: %v", err)
	}
	got, err := time.Parse("15:04:05", bars[0].Timestamp)
	if err != nil {
		t.Fatalf("timestamp 解析失敗 %q: %v", bars[0].Timestamp, err)
	}
	want := time.Date(2026, 7, 31, 14, 30, 0, 0, model.Taipei())
	if got.Hour() != want.Hour() || got.Minute() != want.Minute() || got.Second() != want.Second() {
		t.Errorf("tlong 應為 2026-07-31 14:30:00（Asia/Taipei），實際 %v", bars[0].Timestamp)
	}
}
