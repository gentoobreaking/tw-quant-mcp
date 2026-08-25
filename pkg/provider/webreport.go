package provider

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseWebReport 解析 TWSE-WEB 報表 JSON（stat/fields/data/date，T042 parity 系列）。
// 回傳欄位名、資料列（與 fields 對應）及頂層資料歸屬日（YYYYMMDD 原文，可能為空）。
func ParseWebReport(raw *RawResponse) ([]string, [][]string, string, error) {
	var envelope struct {
		Stat    string          `json:"stat"`
		Date    string          `json:"date"`
		Fields  []string        `json:"fields"`
		DataRaw json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw.Body, &envelope); err != nil {
		return nil, nil, "", fmt.Errorf("provider: envelope JSON 解析失敗: %w", err)
	}
	if envelope.Stat != "" && !strings.EqualFold(envelope.Stat, "OK") {
		return nil, nil, "", fmt.Errorf("provider: 官方回應異常 stat=%q", envelope.Stat)
	}
	var rawRows [][]any
	if len(envelope.DataRaw) > 0 {
		if err := json.Unmarshal(envelope.DataRaw, &rawRows); err != nil {
			return nil, nil, "", fmt.Errorf("provider: data 列解析失敗: %w", err)
		}
	}
	// 官方部分端點（如 MI_INDEX20）之儲存格可能為數字，一律轉字串（稽核 T167/T116 等）。
	rows := make([][]string, 0, len(rawRows))
	for _, rr := range rawRows {
		row := make([]string, 0, len(rr))
		for _, cell := range rr {
			switch x := cell.(type) {
			case string:
				row = append(row, x)
			case nil:
				row = append(row, "")
			default:
				row = append(row, strings.TrimSpace(fmt.Sprint(x)))
			}
		}
		rows = append(rows, row)
	}
	return envelope.Fields, rows, envelope.Date, nil
}

// normalizeWebTablesPick 自 tables[] 結構挑選含 mustField 欄位之首個表格，
// 以官方中文欄名輸出列陣列（TWTB4U 等多表格端點；稽核補強 T116）。
func normalizeWebTablesPick(raw *RawResponse, mustField string) ([]map[string]any, error) {
	var envelope struct {
		Stat   string `json:"stat"`
		Date   string `json:"date"`
		Tables []struct {
			Title  string     `json:"title"`
			Fields []string   `json:"fields"`
			Data   [][]any    `json:"data"`
		} `json:"tables"`
	}
	if err := json.Unmarshal(raw.Body, &envelope); err != nil {
		return nil, fmt.Errorf("provider: tables JSON 解析失敗: %w", err)
	}
	if envelope.Stat != "" && !strings.EqualFold(envelope.Stat, "OK") {
		return nil, fmt.Errorf("provider: 官方回應異常 stat=%q", envelope.Stat)
	}
	var date string
	if ts, err := time.Parse("20060102", envelope.Date); err == nil {
		date = ts.Format("2006-01-02")
	}
	for _, t := range envelope.Tables {
		has := false
		for _, f := range t.Fields {
			if f == mustField {
				has = true
				break
			}
		}
		if !has {
			continue
		}
		out := make([]map[string]any, 0, len(t.Data))
		for _, row := range t.Data {
			rec := make(map[string]any, len(t.Fields)+1)
			for i, f := range t.Fields {
				v := ""
				if i < len(row) && row[i] != nil {
					if s, ok := row[i].(string); ok {
						v = strings.TrimSpace(s)
					} else {
						v = strings.TrimSpace(fmt.Sprint(row[i]))
					}
				}
				rec[f] = v
			}
			if date != "" {
				rec["_date"] = date
			}
			out = append(out, rec)
		}
		return out, nil
	}
	return nil, fmt.Errorf("provider: 找不到含欄位 %q 之表格", mustField)
}

// ZipRow 將 fields 與 row 合成 map（缺欄位為空字串）。
func ZipRow(fields []string, row []string) map[string]string {
	m := make(map[string]string, len(fields))
	for i, f := range fields {
		if i < len(row) {
			m[f] = row[i]
		}
	}
	return m
}

// normalizeWebTable 泛用 TWSE-WEB 表格直通正規化：以官方中文欄位名為 JSON 鍵
// 輸出列陣列（parity 批次 T115/T116/T119/T122/T139/T149/T163/T172/T175-T177/T179/T184）。
func normalizeWebTable(raw *RawResponse) ([]map[string]any, error) {
	fields, rows, date, err := ParseWebReport(raw)
	if err != nil {
		return nil, err
	}
	var meta struct {
		Date string `json:"date"`
	}
	_ = json.Unmarshal(raw.Body, &meta)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m := ZipRow(fields, row)
		rec := make(map[string]any, len(m)+1)
		for k, v := range m {
			rec[k] = strings.TrimSpace(v)
		}
		if date != "" {
			if ts, err := time.Parse("20060102", date); err == nil {
				rec["_date"] = ts.Format("2006-01-02")
			}
		}
		out = append(out, rec)
	}
	return out, nil
}

// normalizeEtfRegInv 正規化定期定額交易戶數統計排行月報表（ETFRank，T120）：
// 官方 fields 含重複欄名（股票/ETF 兩組「代號/名稱/交易戶數」），改用語意化鍵名。
func normalizeEtfRegInv(raw *RawResponse) ([]map[string]any, error) {
	fields, rows, date, err := ParseWebReport(raw)
	if err != nil {
		return nil, err
	}
	if len(fields) < 7 {
		return nil, fmt.Errorf("provider: ETFRank 欄位數不足: %v", fields)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		get := func(i int) string {
			if i < len(row) {
				return strings.TrimSpace(row[i])
			}
			return ""
		}
		rec := map[string]any{
			"rank":           get(0),
			"code":           get(1),
			"name":           get(2),
			"stock_accounts": get(3),
			"etf_code":       get(4),
			"etf_name":       get(5),
			"etf_accounts":   get(6),
		}
		if ts, perr := time.Parse("20060102", date); perr == nil {
			rec["_date"] = ts.Format("2006-01-02")
		}
		out = append(out, rec)
	}
	return out, nil
}

// normalizePassthroughArray 裸 JSON 陣列直通（opendata/* 端點，T142）。
func normalizePassthroughArray(raw *RawResponse) (json.RawMessage, error) {
	var arr json.RawMessage
	if err := json.Unmarshal(raw.Body, &arr); err != nil {
		return nil, fmt.Errorf("provider: passthrough 陣列解析失敗: %w", err)
	}
	return arr, nil
}

// normalizeWebTablesList：tables 型回應（tables[] 各含 title/fields/data）
// 攤平為列陣列，附 _table 欄標示來源表（T140 起信用交易統計等）。
func normalizeWebTablesList(raw *RawResponse) ([]map[string]any, error) {
	var env struct {
		Date   string `json:"date"`
		Stat   string `json:"stat"`
		Tables []struct {
			Title  string    `json:"title"`
			Fields []string  `json:"fields"`
			Data   [][]any   `json:"data"` // 官方可能回數字或字串
		} `json:"tables"`
	}
	if err := json.Unmarshal(raw.Body, &env); err != nil {
		return nil, fmt.Errorf("provider: tables JSON 解析失敗: %w", err)
	}
	if env.Stat != "" && env.Stat != "OK" && env.Stat != "ok" {
		return nil, fmt.Errorf("provider: 官方回應異常 stat=%q", env.Stat)
	}
	var date string
	if ts, err := time.Parse("20060102", env.Date); err == nil {
		date = ts.Format("2006-01-02")
	}
	cellStr := func(v any) string {
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(x)
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		case nil:
			return ""
		default:
			return strings.TrimSpace(fmt.Sprint(x))
		}
	}
	out := make([]map[string]any, 0)
	for _, t := range env.Tables {
		for _, rowAny := range t.Data {
			row := make([]string, len(rowAny))
			for i, v := range rowAny {
				row[i] = cellStr(v)
			}
			m := ZipRow(t.Fields, row)
			rec := make(map[string]any, len(m)+2)
			for k, v := range m {
				rec[k] = strings.TrimSpace(v)
			}
			rec["_table"] = strings.TrimSpace(t.Title)
			if date != "" {
				rec["_date"] = date
			}
			out = append(out, rec)
		}
	}
	return out, nil
}
