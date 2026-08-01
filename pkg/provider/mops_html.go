package provider

import (
	"fmt"
	"strconv"
	"strings"

	"tw-quant-mcp/pkg/model"
)

// ---------------------------------------------------------------------------
// AJAX HTML table 解析（財報三表，mopsov.twse.com.tw）
// ---------------------------------------------------------------------------

// mopsTableRow 表示 HTML table 中的一行資料。
type mopsTableRow struct {
	label  string
	values []string
}

// parseMOPSHTMLTable 從 HTML body 擷取 <table class='hasBorder'>，提取 rows。
// 回傳 label → values 的對映（用於摘要行快速取值），以及所有 rows。
func parseMOPSHTMLTable(body []byte) (map[string]string, []mopsTableRow, error) {
	s := string(body)
	// 找 <table class='hasBorder'>
	start := strings.Index(s, "<table class='hasBorder'")
	if start == -1 {
		start = strings.Index(s, `<table class="hasBorder"`)
		if start == -1 {
			// fallback: 找任意 <table
			start = strings.Index(s, "<table")
			if start == -1 {
				return nil, nil, fmt.Errorf("mops: 找不到 <table>")
			}
		}
	}
	end := strings.Index(s[start:], "</table>")
	if end == -1 {
		return nil, nil, fmt.Errorf("mops: 找不到 </table>")
	}
	tableHTML := s[start : start+end+8]

	// 解析每行 <tr>...</tr>
	var rows []mopsTableRow
	labelMap := make(map[string]string)

	trStart := 0
	for {
		idx := strings.Index(tableHTML[trStart:], "<tr>")
		if idx == -1 {
			break
		}
		trStart += idx
		trEnd := strings.Index(tableHTML[trStart:], "</tr>")
		if trEnd == -1 {
			break
		}
		tr := tableHTML[trStart : trStart+trEnd+5]
		trStart += trEnd + 5

		// 提取所有 <td> 內容
		tds := extractTD(tr)
		if len(tds) == 0 {
			continue
		}

		label := cleanHTML(tds[0])
		var vals []string
		for _, td := range tds[1:] {
			v := strings.TrimSpace(cleanHTML(td))
			vals = append(vals, v)
		}
		rows = append(rows, mopsTableRow{label: label, values: vals})

		// 摘要行存入 map（取第一個非空值）
		for _, v := range vals {
			if v != "" {
				labelMap[label] = v
				break
			}
		}
	}

	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("mops: table 無資料列")
	}
	return labelMap, rows, nil
}

// extractTD 擷取 HTML 中所有 <td...>...</td> 的純文字內容。
func extractTD(html string) []string {
	var result []string
	pos := 0
	for {
		start := strings.Index(html[pos:], "<td")
		if start == -1 {
			break
		}
		pos += start
		// 找到 > 結束標籤開始
		gt := strings.Index(html[pos:], ">")
		if gt == -1 {
			break
		}
		pos += gt + 1
		// 找到 </td>
		end := strings.Index(html[pos:], "</td>")
		if end == -1 {
			break
		}
		content := html[pos : pos+end]
		// 去除巢狀標籤（如 <font>）
		content = stripTags(content)
		result = append(result, content)
		pos += end + 5
	}
	return result
}

// stripTags 去除 HTML 標籤。
func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// cleanHTML 去除 HTML 實體、全形空白與前後空白。
func cleanHTML(s string) string {
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = strings.ReplaceAll(s, "　", "")
	s = strings.ReplaceAll(s, "\u00A0", " ")
	s = strings.TrimSpace(s)
	return s
}

// parseMOPSTableAmount 將 HTML table 金額字串（含逗號）轉為 int64（千元）。
func parseMOPSTableAmount(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// parseMOPSTableFloat 將 HTML table 數值字串轉為 float64。
func parseMOPSTableFloat(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ",", "")
	if s == "" || s == "-" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

// mopsYearQuarter 從 MOPS table 標題解析年度與季別。
// 民國 N 年第 M 季 → year, quarter。
func mopsYearQuarter(html string) (int, int) {
	// 找 "民國N年第M季" 或已在 table header 中
	idx := strings.Index(html, "民國")
	if idx == -1 {
		return 0, 0
	}
	rest := html[idx+len("民國"):]
	yearEnd := strings.Index(rest, "年")
	if yearEnd == -1 {
		return 0, 0
	}
	yearStr := strings.TrimSpace(rest[:yearEnd])
	year, _ := strconv.Atoi(yearStr)
	if year < 1000 {
		year += 1911
	}

	qStart := yearEnd + len("年")
	qEnd := strings.Index(rest[qStart:], "季")
	if qEnd == -1 {
		return year, 0
	}
	// 標題格式「民國115年第1季」→ 季度文字含「第」前綴（"第1"）。
	qText := strings.TrimSpace(rest[qStart : qStart+qEnd])
	qText = strings.TrimPrefix(qText, "第")
	quarter, _ := strconv.Atoi(qText)
	return year, quarter
}

// parseBalanceSheetHTML 解析合併資產負債表 HTML。
func parseBalanceSheetHTML(body []byte) (*model.BalanceSheet, error) {
	m, rows, err := parseMOPSHTMLTable(body)
	if err != nil {
		return nil, fmt.Errorf("mops: 資產負債表解析失敗: %w", err)
	}

	year, quarter := mopsYearQuarter(string(body))
	bs := &model.BalanceSheet{
		Year:    year,
		Quarter: quarter,
	}

	// 預設日期：取民國年份的第一天對應日
	if year > 0 {
		bs.TableDate = fmt.Sprintf("%04d-%02d-01", year, (quarter-1)*3+1)
	}

	// 摘要行取值（仟元 → 元 ×1000）
	bs.TotalAssets = parseMOPSTableAmount(m["資產總額"]) * 1000
	bs.CurrentAssets = parseMOPSTableAmount(m["流動資產合計"]) * 1000
	bs.NonCurrentAssets = parseMOPSTableAmount(m["非流動資產合計"]) * 1000
	bs.TotalLiabilities = parseMOPSTableAmount(m["負債總額"]) * 1000
	bs.CurrentLiabilities = parseMOPSTableAmount(m["流動負債合計"]) * 1000
	bs.NonCurrentLiabilities = parseMOPSTableAmount(m["非流動負債合計"]) * 1000
	bs.TotalEquity = parseMOPSTableAmount(m["權益總額"]) * 1000

	// 若摘要行不存在（欄位名稱可能不同），從 rows 比對
	if bs.TotalAssets == 0 {
		for _, r := range rows {
			switch {
			case strings.Contains(r.label, "資產總"):
				bs.TotalAssets = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "流動資產合計"):
				bs.CurrentAssets = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "非流動資產合計"):
				bs.NonCurrentAssets = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "負債總"):
				bs.TotalLiabilities = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "流動負債合計"):
				bs.CurrentLiabilities = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "非流動負債合計"):
				bs.NonCurrentLiabilities = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(r.label, "權益總"):
				bs.TotalEquity = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			}
		}
	}

	_ = rows
	return bs, nil
}

// parseCashFlowHTML 解析合併現金流量表 HTML。
func parseCashFlowHTML(body []byte) (*model.CashFlowStatement, error) {
	m, rows, err := parseMOPSHTMLTable(body)
	if err != nil {
		return nil, fmt.Errorf("mops: 現金流量表解析失敗: %w", err)
	}

	year, quarter := mopsYearQuarter(string(body))
	cf := &model.CashFlowStatement{
		Year:    year,
		Quarter: quarter,
	}
	if year > 0 {
		cf.TableDate = fmt.Sprintf("%04d-%02d-01", year, (quarter-1)*3+1)
	}

	// 摘要行取值（仟元 → 元 ×1000）
	cf.OperatingCashFlow = parseMOPSTableAmount(m["營業活動之淨現金流入（流出）"]) * 1000
	cf.InvestingCashFlow = parseMOPSTableAmount(m["投資活動之淨現金流入（流出）"]) * 1000
	cf.FinancingCashFlow = parseMOPSTableAmount(m["籌資活動之淨現金流入（流出）"]) * 1000
	cf.EndingCashBalance = parseMOPSTableAmount(m["期末現金及約當現金餘額"]) * 1000

	// fallback: 從 rows 比對
	if cf.OperatingCashFlow == 0 {
		for _, r := range rows {
			label := r.label
			switch {
			case strings.Contains(label, "營業活動之淨現金"):
				cf.OperatingCashFlow = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(label, "投資活動之淨現金"):
				cf.InvestingCashFlow = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(label, "籌資活動之淨現金"):
				cf.FinancingCashFlow = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.Contains(label, "期末現金及約當"):
				cf.EndingCashBalance = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			}
		}
	}

	_ = rows
	return cf, nil
}

// parseIncomeStatementHTML 解析合併綜合損益表 HTML。
func parseIncomeStatementHTML(body []byte) (*model.IncomeStatementRow, error) {
	m, rows, err := parseMOPSHTMLTable(body)
	if err != nil {
		return nil, fmt.Errorf("mops: 損益表解析失敗: %w", err)
	}

	year, quarter := mopsYearQuarter(string(body))
	is := &model.IncomeStatementRow{
		Year:    year,
		Quarter: quarter,
	}
	if year > 0 {
		is.TableDate = fmt.Sprintf("%04d-%02d-01", year, (quarter-1)*3+1)
	}

	// 摘要行取值（仟元 → 元 ×1000）
	is.Revenue = parseMOPSTableAmount(m["營業收入合計"]) * 1000
	is.OperatingProfit = parseMOPSTableAmount(m["營業利益（損失）"]) * 1000
	is.NonOperatingItems = parseMOPSTableAmount(m["營業外收入及支出"]) * 1000
	is.NetIncome = parseMOPSTableAmount(m["本期淨利（淨損）"]) * 1000

	// 若欄位名稱不完全匹配，從 rows 比對
	if is.Revenue == 0 {
		for _, r := range rows {
			label := r.label
			switch {
			case label == "營業收入合計":
				is.Revenue = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.HasPrefix(label, "營業利益"):
				is.OperatingProfit = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case label == "營業外收入及支出":
				is.NonOperatingItems = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			case strings.HasPrefix(label, "本期淨利"):
				is.NetIncome = parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			}
		}
	}
	// 若本期淨利未抓到（欄位名為 "本期淨利（淨損）"），用淨利總額 fallback
	if is.NetIncome == 0 {
		if v, ok := m["本期淨利（淨損）"]; ok && v != "" {
			is.NetIncome = parseMOPSTableAmount(v) * 1000
		}
	}

	_ = rows
	return is, nil
}

// firstNonEmpty 回傳第一個非空字串。
func firstNonEmpty(vals []string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
