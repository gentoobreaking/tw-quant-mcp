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
// 回傳：
//   - labelMap：label → 本季欄值（欄位存在即記錄，值可能為空字串）
//   - rows：所有資料列（values 不含項目欄）
//   - valueCol：本季資料欄於 header 展開後之 index；-1 表示無法判斷（退回首非空）
func parseMOPSHTMLTable(body []byte) (map[string]string, []mopsTableRow, int, error) {
	s := string(body)
	// 找 <table class='hasBorder'>
	start := strings.Index(s, "<table class='hasBorder'")
	if start == -1 {
		start = strings.Index(s, `<table class="hasBorder"`)
		if start == -1 {
			// fallback: 找任意 <table
			start = strings.Index(s, "<table")
			if start == -1 {
				return nil, nil, -1, fmt.Errorf("mops: 找不到 <table>")
			}
		}
	}
	end := strings.Index(s[start:], "</table>")
	if end == -1 {
		return nil, nil, -1, fmt.Errorf("mops: 找不到 </table>")
	}
	tableHTML := s[start : start+end+8]

	// 收集所有 <tr>...</tr>
	var trs []string
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
		trs = append(trs, tableHTML[trStart:trStart+trEnd+5])
		trStart += trEnd + 5
	}
	if len(trs) == 0 {
		return nil, nil, -1, fmt.Errorf("mops: table 無資料列")
	}

	// 資料列 td 數（第一個含 <td 的 tr）
	numCols := -1
	for _, tr := range trs {
		if strings.Contains(tr, "<td") {
			numCols = len(extractTD(tr))
			break
		}
	}

	// 本季欄：掃描 th 標題行（展開 colspan 後長度與資料列一致者）
	valueCol := mopsValueCol(trs, numCols)

	var rows []mopsTableRow
	labelMap := make(map[string]string)
	for _, tr := range trs {
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

		// 摘要行：優先取本季欄；無法判斷時取首個非空值
		val := ""
		if valueCol >= 1 && valueCol-1 < len(vals) {
			val = vals[valueCol-1]
		} else {
			val = firstNonEmpty(vals)
		}
		labelMap[label] = val
	}

	return labelMap, rows, valueCol, nil
}

// mopsValueCol 於 th 標題行中定位「本季」資料欄（展開 colspan 後之 index）。
// 優先找「民國N年第M季」欄（損益表之單季金額）；否則找「民國N年」期間/日期欄
// （資產負債表之期末日、現金流量表之本季期間）。回傳 -1 表示無法判斷。
func mopsValueCol(trs []string, numCols int) int {
	if numCols <= 0 {
		return -1
	}
	seasonIdx, yearIdx := -1, -1
	for _, tr := range trs {
		if !strings.Contains(tr, "<th") || strings.Contains(tr, "<td") {
			continue // 非標題行
		}
		cells := expandTH(tr)
		if len(cells) != numCols {
			continue // 排除 colspan 整行標題（如「民國115年第1季」）
		}
		// 排除展開後全為同一 cell 之整行標題（防 colspan 恰等於欄數）
		uniq := 0
		prev := ""
		for _, c := range cells {
			if c != prev {
				uniq++
				prev = c
			}
		}
		if uniq <= 1 {
			continue
		}
		for i, c := range cells {
			if !strings.Contains(c, "年") {
				continue
			}
			if seasonIdx == -1 && strings.Contains(c, "季") {
				seasonIdx = i
			} else if yearIdx == -1 {
				yearIdx = i
			}
		}
	}
	if seasonIdx >= 0 {
		return seasonIdx
	}
	return yearIdx
}

// expandTH 展開 <th> 行（含 colspan）為 cell 文字陣列。
// 標題列未閉合之 <th>（無 </th>）視為空，回傳空陣列。
func expandTH(tr string) []string {
	var cells []string
	pos := 0
	for {
		start := strings.Index(tr[pos:], "<th")
		if start == -1 {
			break
		}
		pos += start
		gt := strings.Index(tr[pos:], ">")
		if gt == -1 {
			break
		}
		open := tr[pos : pos+gt]
		pos += gt + 1
		end := strings.Index(tr[pos:], "</th>")
		if end == -1 {
			break
		}
		content := stripTags(tr[pos : pos+end])
		pos += end + 5
		n := thColspan(open)
		for i := 0; i < n; i++ {
			cells = append(cells, content)
		}
	}
	return cells
}

// thColspan 解析 <th ... colspan='N'> 之 N；未指定回傳 1。
func thColspan(open string) int {
	cs := strings.Index(open, "colspan")
	if cs == -1 {
		return 1
	}
	rest := strings.TrimLeft(open[cs+len("colspan"):], " ='\"")
	n := 0
	for _, r := range rest {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 1
	}
	return n
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
// 優先自標題列（tblHead）擷取，避免誤中頁面其他「民國」字樣。
func mopsYearQuarter(html string) (int, int) {
	s := html
	if i := strings.Index(html, "tblHead"); i != -1 {
		if j := strings.Index(html[i:], "民國"); j != -1 {
			s = html[i+j:]
		}
	}
	idx := strings.Index(s, "民國")
	if idx == -1 {
		return 0, 0
	}
	rest := s[idx+len("民國"):]
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

// mopsAmount 依 label 取值（labelMap 優先，欄位存在性以 key 判斷，
// 非以值是否為 0）；欄位缺失時以 rows 模糊比對 fallback（仟元 → 元 ×1000）。
func mopsAmount(m map[string]string, rows []mopsTableRow, label, fallback string) int64 {
	if v, ok := m[label]; ok {
		return parseMOPSTableAmount(v) * 1000
	}
	if fallback != "" {
		for _, r := range rows {
			if strings.Contains(r.label, fallback) {
				return parseMOPSTableAmount(firstNonEmpty(r.values)) * 1000
			}
		}
	}
	return 0
}

// parseBalanceSheetHTML 解析合併資產負債表 HTML。
func parseBalanceSheetHTML(body []byte) (*model.BalanceSheet, error) {
	m, rows, _, err := parseMOPSHTMLTable(body)
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
	bs.TotalAssets = mopsAmount(m, rows, "資產總額", "資產總")
	bs.CurrentAssets = mopsAmount(m, rows, "流動資產合計", "流動資產合計")
	bs.NonCurrentAssets = mopsAmount(m, rows, "非流動資產合計", "非流動資產合計")
	bs.TotalLiabilities = mopsAmount(m, rows, "負債總額", "負債總")
	bs.CurrentLiabilities = mopsAmount(m, rows, "流動負債合計", "流動負債合計")
	bs.NonCurrentLiabilities = mopsAmount(m, rows, "非流動負債合計", "非流動負債合計")
	bs.TotalEquity = mopsAmount(m, rows, "權益總額", "權益總")

	return bs, nil
}

// parseCashFlowHTML 解析合併現金流量表 HTML。
func parseCashFlowHTML(body []byte) (*model.CashFlowStatement, error) {
	m, rows, _, err := parseMOPSHTMLTable(body)
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
	cf.OperatingCashFlow = mopsAmount(m, rows, "營業活動之淨現金流入（流出）", "營業活動之淨現金")
	cf.InvestingCashFlow = mopsAmount(m, rows, "投資活動之淨現金流入（流出）", "投資活動之淨現金")
	cf.FinancingCashFlow = mopsAmount(m, rows, "籌資活動之淨現金流入（流出）", "籌資活動之淨現金")
	cf.EndingCashBalance = mopsAmount(m, rows, "期末現金及約當現金餘額", "期末現金及約當")

	return cf, nil
}

// parseIncomeStatementHTML 解析合併綜合損益表 HTML。
func parseIncomeStatementHTML(body []byte) (*model.IncomeStatementRow, error) {
	m, rows, _, err := parseMOPSHTMLTable(body)
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
	is.Revenue = mopsAmount(m, rows, "營業收入合計", "營業收入合計")
	is.OperatingProfit = mopsAmount(m, rows, "營業利益（損失）", "營業利益")
	is.NonOperatingItems = mopsAmount(m, rows, "營業外收入及支出", "營業外收入及支出")
	is.NetIncome = mopsAmount(m, rows, "本期淨利（淨損）", "本期淨利")

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
