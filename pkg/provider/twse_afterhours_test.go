package provider

import (
	"strings"
	"testing"
)

func TestNormalizeAfterHours(t *testing.T) {
	body := `{"stat":"OK","date":"20260824","title":"115年08月24日盤後定價交易",
"fields":["證券代號","證券名稱","成交數量","成交筆數","成交金額","成交價","最後揭示買量","最後揭示賣量"],
"data":[["1101","台泥","203","17","5,014,100","24.70","0","612"],
        ["1101B","台泥乙特","0","0","0","43.25","0","0"]]}`
	raw := &RawResponse{Body: []byte(body)}
	rows, err := normalizeAfterHours(raw)
	if err != nil {
		t.Fatalf("normalizeAfterHours: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("期望 2 列，得到 %d", len(rows))
	}
	r := rows[0]
	if r.Code != "1101" || r.Name != "台泥" {
		t.Errorf("Code/Name = %q/%q", r.Code, r.Name)
	}
	if r.Volume != 203 || r.Transaction != 17 {
		t.Errorf("Volume/Transaction = %d/%d", r.Volume, r.Transaction)
	}
	if r.Amount != 5014100 || r.Price != 24.70 {
		t.Errorf("Amount/Price = %d/%v", r.Amount, r.Price)
	}
	if r.BidVolume != 0 || r.AskVolume != 612 {
		t.Errorf("Bid/Ask = %d/%d", r.BidVolume, r.AskVolume)
	}
	if !strings.Contains(r.Date, "2026-08-24") {
		t.Errorf("Date = %q，期望 2026-08-24", r.Date)
	}
}

// T120：定期定額交易戶數統計排行（ETFRank）正規化
func TestNormalizeEtfRegInv(t *testing.T) {
	body := `{"stat":"OK","title":"115年07月 定期定額交易戶數統計排行月報表",
"fields":[" ","代號","名稱","交易戶數","代號","名稱","交易戶數"],
"date":"20260701",
"data":[["1","2330","台積電","236,742","0050","元大台灣50","1,241,976"]]}`
	raw := &RawResponse{Body: []byte(body)}
	rows, err := normalizeEtfRegInv(raw)
	if err != nil {
		t.Fatalf("normalizeEtfRegInv: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("期望 1 列，得到 %d", len(rows))
	}
	r := rows[0]
	if r["rank"] != "1" || r["code"] != "2330" || r["name"] != "台積電" || r["stock_accounts"] != "236,742" {
		t.Errorf("股票欄錯誤: %+v", r)
	}
	if r["etf_code"] != "0050" || r["etf_name"] != "元大台灣50" || r["etf_accounts"] != "1,241,976" {
		t.Errorf("ETF 欄錯誤: %+v", r)
	}
	if r["_date"] != "2026-07-01" {
		t.Errorf("_date = %q，期望 2026-07-01", r["_date"])
	}
}
