package provider

import (
	"encoding/json"
	"fmt"
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
	if envelope.Stat != "OK" && envelope.Stat != "" {
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
