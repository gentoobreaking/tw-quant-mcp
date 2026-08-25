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
