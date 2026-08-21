package provider

// mops_esg_test.go：T037 MOPS ESG 揭露 CSV 泛用解析器測試。
//
// 涵蓋：
//   - 真實黃金 fixture（esg_ghg.csv，2026-08 實實測下載）欄位完整度
//   - UTF-8 BOM 剝除
//   - 引號內多行欄位（範疇三資料邊界實務常見）
//   - 無效列跳過（缺代碼/名稱）
//   - 全無效列回錯
//   - Normalize 分派路徑（mopsDatasetOf → parseESGCSV）

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMOPSESGGhgFixture 以真實下載之黃金 fixture 驗證八主題共用解析器。
func TestMOPSESGGhgFixture(t *testing.T) {
	body := loadMOPSFixture(t, "esg_ghg.csv")
	raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[MOPSESGGhg], body)

	s := newTestMOPSSource()
	if err := s.Validate(raw); err != nil {
		t.Fatalf("Validate 失敗: %v", err)
	}
	b, err := s.Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize 失敗: %v", err)
	}

	var rows []ESGRow
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatalf("JSON 解析失敗: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("fixture 應解析 3 列（1101/2330/2317），實際 %d", len(rows))
	}

	byCode := map[string]ESGRow{}
	for _, r := range rows {
		byCode[r.Code] = r
	}
	r, ok := byCode["2330"]
	if !ok {
		t.Fatal("缺少台積電（2330）")
	}
	if r.Name != "台積電" {
		t.Errorf("名稱錯誤: %q", r.Name)
	}
	if r.Year != "114" {
		t.Errorf("報告年度應保留民國格式 114，實際 %q", r.Year)
	}
	if r.ReportDate == "" || !strings.HasPrefix(r.ReportDate, "2026-") {
		t.Errorf("出表日期應轉 ISO，實際 %q", r.ReportDate)
	}
	// 核心欄位不得混入 Fields；揭露指標必在 Fields
	for _, k := range []string{"出表日期", "報告年度", "公司代號", "公司名稱"} {
		if _, dup := r.Fields[k]; dup {
			t.Errorf("核心欄位 %q 不應重複出現於 Fields", k)
		}
	}
	if v, ok := r.Fields["範疇一排放量(公噸CO2e)"]; !ok || v == "" {
		t.Errorf("範疇一排放量應存在且非空，實際 %q", v)
	}
	if v, ok := r.Fields["溫室氣體排放密集度(公噸CO₂e/百萬元營業額)"]; !ok {
		t.Errorf("密集度欄位遺失")
	} else if v == "" {
		t.Error("密集度值為空")
	}

	// 台泥含範疇三驗證狀態「否」（fixture 實值）
	if tc, ok := byCode["1101"]; ok {
		if tc.Fields["範疇三取得驗證"] != "否" && tc.Fields["範疇三取得驗證"] != "是" {
			t.Errorf("台泥範疇三驗證值異常: %q", tc.Fields["範疇三取得驗證"])
		}
	}
}

// TestParseESGCSVInline 覆蓋 BOM／多行欄位／無效列跳過。
func TestParseESGCSVInline(t *testing.T) {
	const bom = "\xEF\xBB\xBF"
	csvBody := bom + `"出表日期","報告年度","公司代號","公司名稱","再生能源使用率(%)","資料邊界"
"1150821","114","2330","台積電","5.5","母公司
及子公司"
"1150821","","","","",""
"",   ,"1102","亞泥","3.0","母公司"
`

	rc := newMOPSReader([]byte(csvBody))
	header, err := rc.Read()
	if err != nil {
		t.Fatalf("header 讀取失敗: %v", err)
	}
	rows, err := parseESGCSV(rc, header)
	if err != nil {
		t.Fatalf("parseESGCSV 失敗: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("應跳過無效列僅存 2 列，實際 %d", len(rows))
	}
	if rows[0].Code != "2330" || rows[0].Year != "114" {
		t.Errorf("第 1 列錯誤: %+v", rows[0])
	}
	// 多行欄位：換行保留於原值
	if got, want := rows[0].Fields["資料邊界"], "母公司\n及子公司"; got != want {
		t.Errorf("多行欄位應保留換行，實際 %q", got)
	}
	if got, want := rows[0].Fields["再生能源使用率(%)"], "5.5"; got != want {
		t.Errorf("指標值錯誤: %q", got)
	}
	if rows[1].Code != "1102" {
		t.Errorf("第 2 列錯誤: %+v", rows[1])
	}
}

// 全列無效：回錯（官方格式變更須立即發現）。
func TestParseESGCSVAllInvalid(t *testing.T) {
	csvBody := `"出表日期","報告年度","公司代號","公司名稱","指標"
"1150821","","","",""
`
	rc := newMOPSReader([]byte(csvBody))
	header, err := rc.Read()
	if err != nil {
		t.Fatalf("header 讀取失敗: %v", err)
	}
	if _, err := parseESGCSV(rc, header); err == nil {
		t.Error("全無效列應回錯")
	}
}

// 八主題 dataset 皆應分派至 parseESGCSV（Normalize 路徑冒煙）。
func TestMOPSESGDatasetsDispatchToESGParser(t *testing.T) {
	const csvBody = `"出表日期","報告年度","公司代號","公司名稱","指標值"
"1150821","114","2330","台積電","X"`
	for ds := range mopsPaths {
		switch ds {
		case MOPSESGGhg, MOPSESGRenewable, MOPSESGWater, MOPSESGWaste,
			MOPSESgEmployee, MOPSESGBoard, MOPSESGConf, MOPSESGTcfd:
			raw := mopsFixtureRaw(t, mopsOpenDataBase+mopsPaths[ds], []byte(csvBody))
			b, err := newTestMOPSSource().Normalize(raw)
			if err != nil {
				t.Errorf("%s Normalize 失敗: %v", ds, err)
				continue
			}
			var rows []ESGRow
			if err := json.Unmarshal(b, &rows); err != nil {
				t.Errorf("%s 解析失敗: %v", ds, err)
			}
		}
	}
}
