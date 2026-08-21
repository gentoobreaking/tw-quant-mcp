package mcp

// tools_esg_test.go：T037 ESG 雙來源工具測試。
//
// 涵蓋：
//   - filterTopics 參數正規化（空/去重排序/型別/範圍）
//   - handlerGetESGReport 快樂路徑（TWSE 勝出平手、8 主題＋治理規程、lineage）
//   - topics 過濾、同公司多年度取最新
//   - 速度選源：TWSE 探測失敗 → MOPS 為主來源
//   - fallback：主來源全數失敗 → 降級另一來源並反轉偏好
//   - 探測雙失敗：回錯且偏好維持未定
//   - 快取命中：二次呼叫 is_cached=true、上游呼叫數不變
//   - 治理規程抓取失敗不阻擋 ESG 主體

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// urlValuesTopic 組裝 TWSE API topic 參數（fakeKey 用）。
func urlValuesTopic(topic int) url.Values {
	return url.Values{"topic": {strconv.Itoa(topic)}}
}

// totalESGCalls 彙總 ESG 相關（TWSE esg + MOPS esg_* + 治理）之上游呼叫數。
func totalESGCalls(f *fakeFetch) int {
	total := 0
	for topic := 1; topic <= esgTopicCount; topic++ {
		total += f.called("esg", urlValuesTopic(topic))
	}
	for _, ds := range mopsESGDatasets {
		total += f.called(string(ds), nil)
	}
	total += f.called("company_governance", nil)
	return total
}

// esgRowStub 產生單一公司單一 topic 之 normalized []provider.ESGRow JSON。
func esgRowStub(code, name, year string, fields string) string {
	return `[{"report_date":"2026-07-31","year":"` + year + `","code":"` + code + `","name":"` + name + `","fields":` + fields + `}]`
}

// stubESGBothSources 對 TWSE（topic=1..8）與 MOPS（esg_*）兩來源之全部
// topic stub 相同內容，另附公司治理規程。回傳治理規程 body 以便個案覆寫。
func stubESGBothSources(f *fakeFetch) {
	for topic := 1; topic <= esgTopicCount; topic++ {
		body := esgRowStub("2330", "台積電", "2025", `{"指標":"`+esgTopicNames[topic]+`"}`)
		f.stub("esg", urlValuesTopic(topic), body)
	}
	stubAllMOPSESG(f, "2330")
	f.stub("company_governance", nil,
		`[{"report_date":"2026-07-31","code":"2330","name":"台積電","rules":"訂有公司治理實務守則"}]`)
}

// stubAllMOPSESG 對 MOPS 八個 ESG dataset stub 同一公司列。
func stubAllMOPSESG(f *fakeFetch, code string) {
	name := "台積電"
	if code == "2317" {
		name = "鴻海"
	}
	for _, ds := range mopsESGDatasets {
		f.stub(string(ds), nil, esgRowStub(code, name, "2025", `{"指標":"MOPS"}`))
	}
}

func TestFilterTopics(t *testing.T) {
	// 空 → 全部 [1..8]
	all, err := filterTopics(nil)
	if err != nil {
		t.Fatalf("nil topics 應合法: %v", err)
	}
	if len(all) != esgTopicCount || all[0] != 1 || all[7] != 8 {
		t.Errorf("空參數應回傳 [1..%d]，實際 %v", esgTopicCount, all)
	}

	// 去重＋排序
	got, err := filterTopics([]any{float64(6), float64(1), float64(6), float64(3)})
	if err != nil {
		t.Fatalf("topics 應合法: %v", err)
	}
	if len(got) != 3 || got[0] != 1 || got[1] != 3 || got[2] != 6 {
		t.Errorf("應去重排序為 [1 3 6]，實際 %v", got)
	}

	// 型別錯誤
	if _, err := filterTopics([]any{"1"}); err == nil {
		t.Error("字串元素應回錯")
	}
	// 範圍錯誤
	if _, err := filterTopics([]any{float64(0)}); err == nil {
		t.Error("0 應回錯")
	}
	if _, err := filterTopics([]any{float64(9)}); err == nil {
		t.Error("9 應回錯")
	}
}

// 平手時 TWSE 勝出；預設回傳 8 主題＋治理規程共 9 題材；lineage source=TWSE_API。
func TestESGReportTWSEPrimaryDefaultAllTopics(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)

	env := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	esg, ok := env.Data.(model.ESGReport)
	if !ok {
		t.Fatalf("Data 應為 ESGReport，實際 %T", env.Data)
	}
	if len(esg.Topics) != esgTopicCount+1 {
		t.Fatalf("應有 %d 題材（8 主題＋治理規程），實際 %d", esgTopicCount+1, len(esg.Topics))
	}
	if esg.Topics[0].Topic != "溫室氣體排放" || esg.Topics[0].Year != "2025" {
		t.Errorf("第 1 題材錯誤: %+v", esg.Topics[0])
	}
	last := esg.Topics[len(esg.Topics)-1]
	if last.Topic != "公司治理規程" || last.Fields["公司治理之相關規程規則"] == "" {
		t.Errorf("治理規程題材錯誤: %+v", last)
	}
	if env.Lineage.Source != model.SourceTWSEAPI {
		t.Errorf("lineage source 應為 TWSE_API（平手勝出），實際 %q", env.Lineage.Source)
	}
	// 探測暖快取：topic=1 僅 1 次上游（fetchAll 命中 L1）；其餘 topic 各 1 次
	if n := f.called("esg", urlValuesTopic(1)); n != 1 {
		t.Errorf("TWSE topic=1 上游應 1 次（探測暖快取，fetchAll 命中），實際 %d", n)
	}
	if n := f.called("esg", urlValuesTopic(8)); n != 1 {
		t.Errorf("TWSE topic=8 上游應 1 次，實際 %d", n)
	}
}

// topics=[6] 只回董事會組成＋治理規程。
func TestESGReportTopicsFilter(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)

	env := callEnv(t, app, "get_esg_report",
		map[string]any{"symbol": "2330", "topics": []any{float64(6)}})
	esg := env.Data.(model.ESGReport)
	if len(esg.Topics) != 2 {
		t.Fatalf("過濾後應 2 題材，實際 %d", len(esg.Topics))
	}
	if esg.Topics[0].Topic != "董事會組成" {
		t.Errorf("應僅含董事會組成，實際 %+v", esg.Topics[0])
	}
	// 未請求之 topic 不應觸發上游
	if n := f.called("esg", urlValuesTopic(1)); n > 1 { // 僅探測 1 次（fetchAll 跳過）
		t.Errorf("topic=1 僅應探測 1 次，實際 %d", n)
	}
	if n := f.called("esg", urlValuesTopic(3)); n != 0 {
		t.Errorf("未請求之 topic=3 不應有上游呼叫，實際 %d", n)
	}
}

// 同公司多年度：取報告年度最新。
func TestESGReportLatestYearWins(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)
	f.stub("esg", urlValuesTopic(2), `[
		{"report_date":"2025-07-31","year":"2024","code":"2330","name":"台積電","fields":{"指標":"舊年度"}},
		{"report_date":"2026-07-31","year":"2025","code":"2330","name":"台積電","fields":{"指標":"新年度"}}]`)

	env := callEnv(t, app, "get_esg_report",
		map[string]any{"symbol": "2330", "topics": []any{float64(2)}})
	esg := env.Data.(model.ESGReport)
	if len(esg.Topics) != 2 { // topic 2 + 治理規程
		t.Fatalf("應 2 題材，實際 %d", len(esg.Topics))
	}
	if esg.Topics[0].Year != "2025" || esg.Topics[0].Fields["指標"] != "新年度" {
		t.Errorf("應取 2025 最新年度，實際 %+v", esg.Topics[0])
	}
}

// TWSE 探測失敗 → MOPS 為主來源；lineage source=MOPS。
func TestESGSpeedSelectMOPSWhenTWSEProbeFails(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	f.stub404("esg", urlValuesTopic(1)) // TWSE 探測失敗
	stubAllMOPSESG(f, "2330")
	f.stub("company_governance", nil,
		`[{"report_date":"2026-07-31","code":"2330","name":"台積電","rules":"R"}]`)

	env := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	if env.Lineage.Source != model.SourceMOPS {
		t.Errorf("TWSE 探測失敗時應採 MOPS，實際 %q", env.Lineage.Source)
	}
	if got := app.esgPrimary; got != model.SourceMOPS {
		t.Errorf("偏好應記為 MOPS，實際 %q", got)
	}
	// 後續呼叫不再探測 TWSE（fetchAll 直接走 MOPS）
	callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	if n := f.called("esg", urlValuesTopic(2)); n != 0 {
		t.Errorf("偏好已定後不應再打 TWSE，實際 topic=2 呼叫 %d 次", n)
	}
}

// fallback：主來源（預設 MOPS）全數失敗 → 降級 TWSE 且偏好反轉。
func TestESGFallbackSwapsPreference(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)
	// 覆寫：MOPS 全部 404
	for _, ds := range mopsESGDatasets {
		f.stub404(string(ds), nil)
	}
	app.esgMu.Lock()
	app.esgPrimary = model.SourceMOPS // 預設主來源為 MOPS（跳過探測）
	app.esgMu.Unlock()

	env := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	if env.Lineage.Source != model.SourceTWSEAPI {
		t.Errorf("MOPS 全敗應降級 TWSE_API，實際 %q", env.Lineage.Source)
	}
	if got := app.esgPrimary; got != model.SourceTWSEAPI {
		t.Errorf("fallback 成功後偏好應反轉為 TWSE_API，實際 %q", got)
	}
	esg := env.Data.(model.ESGReport)
	if len(esg.Topics) != esgTopicCount+1 {
		t.Errorf("降級後仍應有完整題材，實際 %d", len(esg.Topics))
	}
}

// 探測雙失敗：回錯、偏好維持未定。
func TestESGBothSourcesFailAtProbe(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	f.stub404("esg", urlValuesTopic(1))
	f.stub404(string(provider.MOPSESGGhg), nil)

	_, err := app.core.Call(context.Background(), "get_esg_report", map[string]any{"symbol": "2330"})
	if err == nil {
		t.Fatal("探測雙失敗應回錯")
	}
	if !strings.Contains(err.Error(), "不可用") {
		t.Errorf("錯誤應提及來源不可用，實際 %v", err)
	}
	if got := app.esgPrimary; got != "" {
		t.Errorf("雙失敗後偏好應維持未定，實際 %q", got)
	}
}

// 快取命中：第二次呼叫 is_cached=true、上游呼叫數不變。
func TestESGCacheHitSecondCall(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)

	callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	totalAfterFirst := totalESGCalls(f)

	env2 := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	if !env2.Lineage.IsCached {
		t.Errorf("第二次呼叫應命中快取，lineage=%+v", env2.Lineage)
	}
	if total := totalESGCalls(f); total != totalAfterFirst {
		t.Errorf("命中快取不應有上游呼叫，第一次 %d → 第二次後 %d", totalAfterFirst, total)
	}
}

// 治理規程抓取失敗不阻擋 ESG 主體（仍回 8 主題）。
func TestESGGovernanceFailureNotBlocking(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)
	f.stub404("company_governance", nil)

	env := callEnv(t, app, "get_esg_report", map[string]any{"symbol": "2330"})
	esg := env.Data.(model.ESGReport)
	if len(esg.Topics) != esgTopicCount {
		t.Errorf("治理失敗時應仍有 8 主題，實際 %d", len(esg.Topics))
	}
}

// 查詢未揭露公司：回明確錯誤。
func TestESGReportUnknownCompany(t *testing.T) {
	f := newFake(t)
	app := deApp(t, f)
	stubESGBothSources(f)

	_, err := app.core.Call(context.Background(), "get_esg_report", map[string]any{"symbol": "6547"})
	if err == nil {
		t.Fatal("無揭露資料之公司應回錯")
	}
	if !strings.Contains(err.Error(), "6547") {
		t.Errorf("錯誤應含代碼，實際 %v", err)
	}
}
