package model

import (
	"sync"
	"testing"
)

func sampleSymbols() []Symbol {
	return []Symbol{
		{Code: "2330", Market: MarketTSE, Name: "台積電", Category: "半導體業"},
		{Code: "1101", Market: MarketTSE, Name: "台泥", Category: "水泥工業"},
		{Code: "6547", Market: MarketOTC, Name: "高端疫苗", Category: ""},
	}
}

func TestRegistryLookup(t *testing.T) {
	r := NewRegistry()
	if err := r.Set(sampleSymbols()); err != nil {
		t.Fatalf("Set 失敗: %v", err)
	}

	s, ok := r.Lookup("2330")
	if !ok {
		t.Fatal("2330 應註冊")
	}
	if s.Name != "台積電" || s.Market != MarketTSE {
		t.Errorf("Symbol = %+v", s)
	}
	if s.Exch() != "tse_2330.tw" {
		t.Errorf("tse ex_ch 組裝錯誤：%s", s.Exch())
	}

	s, ok = r.Lookup("6547")
	if !ok || s.Market != MarketOTC {
		t.Fatalf("6547 應為上櫃：%+v ok=%v", s, ok)
	}
	if s.Exch() != "otc_6547.tw" {
		t.Errorf("otc ex_ch 組裝錯誤：%s", s.Exch())
	}

	if _, ok := r.Lookup("9999"); ok {
		t.Error("未知代碼應回傳 miss（供 handler 回覆明確錯誤）")
	}
}

func TestRegistryMarket(t *testing.T) {
	r := NewRegistry()
	r.Set(sampleSymbols())

	if m, ok := r.Market("2330"); !ok || m != MarketTSE {
		t.Errorf("2330 市場別 = %s/%v", m, ok)
	}
	if m, ok := r.Market("6547"); !ok || m != MarketOTC {
		t.Errorf("6547 市場別 = %s/%v", m, ok)
	}
	if _, ok := r.Market("0001"); ok {
		t.Error("未知代碼 Market 應回傳 false")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	r.Set(sampleSymbols())

	tse := r.List(MarketTSE)
	if len(tse) != 2 || tse[0].Code != "1101" || tse[1].Code != "2330" {
		t.Errorf("tse 清單應依代碼排序：%+v", tse)
	}
	otc := r.List(MarketOTC)
	if len(otc) != 1 || otc[0].Code != "6547" {
		t.Errorf("otc 清單：%+v", otc)
	}
	all := r.List("")
	if len(all) != 3 {
		t.Errorf("全部清單應 3 筆，實際 %d", len(all))
	}
	if r.Len() != 3 {
		t.Errorf("Len = %d", r.Len())
	}
}

func TestRegistrySetValidation(t *testing.T) {
	r := NewRegistry()
	// 任一記錄不合法即整批拒絕（官方格式漂移需即時發現）。
	bad := append(sampleSymbols(), Symbol{Code: "abc", Market: MarketTSE, Name: "X"})
	if err := r.Set(bad); err == nil {
		t.Error("非法代碼應導致 Set 失敗")
	}
	if r.Len() != 0 {
		t.Error("Set 失敗時不得留下部分資料")
	}
	if err := r.Set(sampleSymbols()); err != nil {
		t.Errorf("合法清單 Set 應成功：%v", err)
	}
}

// 覆寫載入（每日預熱）：舊資料被完整取代，Lookup 立即反映新資料。
func TestRegistryReplace(t *testing.T) {
	r := NewRegistry()
	r.Set(sampleSymbols())
	if err := r.Set([]Symbol{{Code: "1101", Market: MarketTSE, Name: "台泥"}}); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Lookup("2330"); ok {
		t.Error("覆寫後 2330 應移除")
	}
	if _, ok := r.Lookup("1101"); !ok {
		t.Error("覆寫後 1101 應仍在")
	}
	if r.Len() != 1 {
		t.Errorf("Len = %d", r.Len())
	}
}

// 併發查詢與覆寫：資料結構必須為 thread-safe。
func TestRegistryConcurrent(t *testing.T) {
	r := NewRegistry()
	r.Set(sampleSymbols())

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				r.Lookup("2330")
				r.Market("6547")
				r.List(MarketTSE)
				r.Len()
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 50; j++ {
			r.Set(sampleSymbols())
		}
	}()
	wg.Wait()

	if _, ok := r.Lookup("2330"); !ok {
		t.Error("併發後 2330 應仍在")
	}
}
