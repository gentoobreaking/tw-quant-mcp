package normalize

import (
	"encoding/json"
	"fmt"
	"strings"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/model/domain"
)

// t86Response 為 TWSE Web 三大法人買賣超（T86 日報）之原始回應結構。
// 官方全部欄位皆為「股數」（2026-07 實測 title 明示「單位：股」），
// 僅需移除千分位逗號，無需單位換算。
type t86Response struct {
	Stat   string     `json:"stat"` // "OK"
	Date   string     `json:"date"` // "20260731"
	Title  string     `json:"title"`
	Hints  string     `json:"hints"`
	Fields []string   `json:"fields"`
	Data   [][]string `json:"data"`
}

// fromTWSEWeb 將 TWSE Web 三大法人買賣超（T86 日報）轉為
// []InstitutionalFlow（§9.7）：
//   - T86 僅涵蓋上市（TSE）——Stock.Market 標註 "TSE"；
//   - 外陸資/投信/自營商買賣超股數（含千分位逗號）直接帶入，
//     ForeignHoldingPct 來自 QFIIS（qfiis 資料集），此處不輸出（omitempty）；
//   - Lineage：Source=TWSE_WEB、CANONICAL、POST_MARKET、data_date 由 raw date 轉出；
//   - 全滅或 stat 異常時回傳錯誤。
func fromTWSEWeb(raw []byte) ([]domain.InstitutionalFlow, error) {
	var r t86Response
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("normalize: TWSE Web 回應 JSON 解析失敗: %w", err)
	}
	if r.Stat != "OK" {
		return nil, fmt.Errorf("normalize: TWSE Web 回應異常（stat=%q）", r.Stat)
	}
	if len(r.Data) == 0 {
		return nil, fmt.Errorf("normalize: TWSE Web 回應無資料列")
	}
	m := fieldMap(r.Fields)
	date := fmtDate(r.Date)
	out := make([]domain.InstitutionalFlow, 0, len(r.Data))
	for _, row := range r.Data {
		get := func(k string) string {
			if i, ok := m[k]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		code, name := get("證券代號"), get("證券名稱")
		if code == "" || name == "" {
			continue
		}
		out = append(out, domain.InstitutionalFlow{
			Stock: domain.StockIdentity{
				Symbol: code,
				Name:   name,
				Market: "TSE",
			},
			Date:             date,
			Market:           "TSE",
			ForeignNetShares: commaInt(get("外陸資買賣超股數(不含外資自營商)")),
			TrustNetShares:   commaInt(get("投信買賣超股數")),
			DealerNetShares:  commaInt(get("自營商買賣超股數")),
			Lineage: model.Lineage{
				Source:     model.SourceTWSEWeb,
				SourceRole: model.SourceRoleCanonical,
				Freshness:  model.FreshnessPostMarket,
				DataDate:   date,
			},
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("normalize: TWSE Web 回應無有效資料列")
	}
	return out, nil
}

// fieldMap 建立 fields 名稱 → 索引之對照表。
func fieldMap(fields []string) map[string]int {
	m := make(map[string]int, len(fields))
	for i, f := range fields {
		m[strings.TrimSpace(f)] = i
	}
	return m
}

// commaInt 移除千分位逗號並轉為 int64（支援負數）；空字串/非法回傳 0。
func commaInt(s string) int64 {
	t := strings.TrimSpace(s)
	if t == "" || t == "-" {
		return 0
	}
	neg := false
	if t[0] == '-' {
		neg = true
		t = t[1:]
	}
	var n int64
	for _, r := range t {
		if r == ',' {
			continue
		}
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int64(r-'0')
	}
	if neg {
		return -n
	}
	return n
}

// fmtDate 將 "20260731" 轉為 "2026-07-31"；格式不符時原樣回傳。
func fmtDate(ymd string) string {
	if len(ymd) == 8 {
		return ymd[:4] + "-" + ymd[4:6] + "-" + ymd[6:]
	}
	return ymd
}
