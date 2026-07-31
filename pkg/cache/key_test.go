package cache

import "testing"

// §4.3 快取鍵：sha256(source_id | dataset | data_date | symbol | params_hash)[0:16]。
func TestKeyStringGolden(t *testing.T) {
	// sha256("TWSE_API|daily_kline|2026-07-31|2330|") 前 16 字元（預先以 shasum 計算）。
	want := "e4124938b78a441b"
	if got := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330", nil); got != want {
		t.Errorf("KeyString = %s，預期 %s", got, want)
	}
}

// params_hash：以鍵名排序之 k=v 連綴之 sha256 前 16 字元。
func TestKeyStringParamsHash(t *testing.T) {
	// sha256("p=x")=8d6786e0639a79e2；再 sha256("TWSE_API|daily_kline|2026-07-31|2330|8d6786e0639a79e2") 前 16。
	want := "0b5c8df3c5f1ea9a"
	if got := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330",
		map[string]string{"p": "x"}); got != want {
		t.Errorf("KeyString(params) = %s，預期 %s", got, want)
	}
}

func TestKeyStringDeterministic(t *testing.T) {
	params := map[string]string{"period": "10", "chart": "true"}
	a := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330", params)
	b := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330", params)
	if a != b {
		t.Errorf("相同輸入應產生相同鍵：%s vs %s", a, b)
	}
	// params 鍵序不影響結果。
	if c := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330",
		map[string]string{"chart": "true", "period": "10"}); a != c {
		t.Errorf("params 鍵序應不影響鍵值：%s vs %s", a, c)
	}
}

func TestKeyStringDistinct(t *testing.T) {
	cases := []struct {
		name     string
		sourceID string
		dataset  string
		dataDate string
		symbol   string
		params   map[string]string
	}{
		{"source", "TPEX_API", "daily_kline", "2026-07-31", "2330", nil},
		{"dataset", "TWSE_API", "margin", "2026-07-31", "2330", nil},
		{"date", "TWSE_API", "daily_kline", "2026-08-01", "2330", nil},
		{"symbol", "TWSE_API", "daily_kline", "2026-07-31", "2331", nil},
		{"params", "TWSE_API", "daily_kline", "2026-07-31", "2330", map[string]string{"p": "y"}},
		{"params2", "TWSE_API", "daily_kline", "2026-07-31", "2330", map[string]string{"q": "x"}},
	}
	base := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330", nil)
	for _, c := range cases {
		got := KeyString(c.sourceID, c.dataset, c.dataDate, c.symbol, c.params)
		if got == base {
			t.Errorf("輸入 %s 變更後應產生不同鍵", c.name)
		}
	}
}

func TestKeyStringFormat(t *testing.T) {
	got := KeyString("TWSE_API", "daily_kline", "2026-07-31", "2330", nil)
	if len(got) != 16 {
		t.Errorf("快取鍵應為 16 字元（§4.3 [0:16]），實際 %d：%s", len(got), got)
	}
	for _, r := range got {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Errorf("快取鍵應為十六進位字元，實際 %q", got)
			break
		}
	}
}
