package provider

import (
	"encoding/json"
	"fmt"
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
	var rows [][]string
	if len(envelope.DataRaw) > 0 {
		if err := json.Unmarshal(envelope.DataRaw, &rows); err != nil {
			return nil, nil, "", fmt.Errorf("provider: data 列解析失敗: %w", err)
		}
	}
	return envelope.Fields, rows, envelope.Date, nil
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
			Title  string     `json:"title"`
			Fields []string   `json:"fields"`
			Data   [][]string `json:"data"`
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
	out := make([]map[string]any, 0)
	for _, t := range env.Tables {
		for _, row := range t.Data {
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
