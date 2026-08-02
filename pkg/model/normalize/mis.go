package normalize

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"tw-quant-mcp/pkg/model"
)

// misResponse 為 getStockInfo.jsp 回應之頂層結構（MIS 官方原始欄位）。
type misResponse struct {
	Rtcode   string     `json:"rtcode"` // "0000" = OK
	MsgArray []misEntry `json:"msgArray"`
}

// misEntry 宣告本層所需之 MIS 原生欄位（§8.3；上游欄位唯本層可解析）。
type misEntry struct {
	Code  string `json:"c"`  // 代碼
	Ch    string `json:"ch"` // "2330.tw"
	Z     string `json:"z"`  // 成交價（元，4 位小數）
	Tv    string `json:"tv"` // 當分鐘內累積成交量（張，每分鐘重置）
	Tlong string `json:"tlong"`
}

// fromMIS 將 MIS 原始回應轉為 tick bar 序列（KlineBar）：
//   - 每筆有效 msgArray 記錄輸出一根 KlineBar（tick bar：OHLC=成交價 z）；
//   - 單位換算（§5.1）：tv 張 ×1000 → 股；z 保留 2 位小數（元）；
//   - Timestamp 由 tlong（毫秒）轉 Asia/Taipei 之 "HH:MM:SS"；
//   - 個別無效記錄略過（容錯官方格式雜訊）；全滅或 rtcode 異常時回傳錯誤。
func fromMIS(raw []byte) ([]model.KlineBar, error) {
	var r misResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("normalize: MIS 回應 JSON 解析失敗: %w", err)
	}
	if r.Rtcode != "0000" {
		return nil, fmt.Errorf("normalize: MIS 回應異常（rtcode=%q）", r.Rtcode)
	}
	if len(r.MsgArray) == 0 {
		return nil, fmt.Errorf("normalize: MIS 回應無 msgArray（watchlist 可能為空）")
	}
	out := make([]model.KlineBar, 0, len(r.MsgArray))
	for _, e := range r.MsgArray {
		bar, ok := misToBar(e)
		if ok {
			out = append(out, bar)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("normalize: MIS 回應無有效記錄（官方格式可能變更）")
	}
	return out, nil
}

// misToBar 將單筆 MIS 記錄轉為 KlineBar；z/tv/tlong 為必要欄位。
func misToBar(e misEntry) (model.KlineBar, bool) {
	price, ok := parsePrice(e.Z)
	if !ok {
		return model.KlineBar{}, false
	}
	vol, ok := parseVol(e.Tv)
	if !ok {
		return model.KlineBar{}, false
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(e.Tlong), 10, 64)
	if err != nil || ms <= 0 {
		return model.KlineBar{}, false
	}
	return model.KlineBar{
		Timestamp: time.UnixMilli(ms).In(model.Taipei()).Format("15:04:05"),
		Open:      price,
		High:      price,
		Low:       price,
		Close:     price,
		Volume:    vol,
	}, true
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
