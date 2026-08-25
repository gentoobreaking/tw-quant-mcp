package mcp

// tools_esg.go：ESG 揭露雙來源工具（T037）。
//
// 資料來源（同一份官方揭露，兩個管線）：
//   - TWSE OpenAPI：openapi.twse.com.tw/v1/opendata/t187ap46_L_{topic}（JSON，
//     provider.normalizeESG 泛用 Fields map 解析）
//   - MOPS Open Data：mopsfin.twse.com.tw/opendata/t187ap46_L_{topic}.csv
//     （CSV+BOM，provider.parseESGCSV 泛用解析，產出同型別 []provider.ESGRow）
//
// 速度選源：首次呼叫並發探測兩來源（topic=1 各一 request），成功者中取
// 耗時短者為主來源；主來源失敗自動 fallback 另一來源，成功則反轉偏好。
// 探測結果同時暖快取（DatasetESG 24h TTL），之後僅於失敗切換時更新偏好。

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"tw-quant-mcp/pkg/cache"
	"tw-quant-mcp/pkg/model"
	"tw-quant-mcp/pkg/provider"
)

// esgTopicCount 為 t187ap46 家族揭露主題數（L_1..L_8）。
const esgTopicCount = 8

// esgTopicNames 對應 topic → 揭露主題中文名。
var esgTopicNames = map[int]string{
	1: "溫室氣體排放",
	2: "再生能源使用率",
	3: "用水量管理",
	4: "廢棄物管理",
	5: "員工薪資福利",
	6: "董事會組成",
	7: "法說會資訊",
	8: "TCFD 氣候風險揭露",
}

// mopsESGDatasets 對應 topic → MOPS ESG dataset ID。
var mopsESGDatasets = map[int]provider.MOPSDataset{
	1: provider.MOPSESGGhg,
	2: provider.MOPSESGRenewable,
	3: provider.MOPSESGWater,
	4: provider.MOPSESGWaste,
	5: provider.MOPSESgEmployee,
	6: provider.MOPSESGBoard,
	7: provider.MOPSESGConf,
	8: provider.MOPSESGTcfd,
}

// fetchESGTWSETopic 自 TWSE OpenAPI 抓取單一 topic（泛用 normalizeESG）。
func (a *App) fetchESGTWSETopic(ctx context.Context, dataDate string, topic int) ([]provider.ESGRow, bool, bool, error) {
	topicStr := strconv.Itoa(topic)
	key := cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIESG), dataDate, "", map[string]string{"topic": topicStr})
	return fetchNormalize[[]provider.ESGRow](a, ctx, string(provider.TWSEAPIESG), dataDate, key, func() ([]byte, error) {
		return a.fetchAPIRaw(ctx, provider.TWSEAPIESG, url.Values{"topic": {topicStr}})
	})
}

// fetchESGMOPSTopic 自 MOPS Open Data CSV 抓取單一 topic（泛用 parseESGCSV）。
func (a *App) fetchESGMOPSTopic(ctx context.Context, dataDate string, topic int) ([]provider.ESGRow, bool, bool, error) {
	ds, ok := mopsESGDatasets[topic]
	if !ok {
		return nil, false, false, fmt.Errorf("mcp: 未支援之 ESG topic %d", topic)
	}
	if a.mops == nil {
		return nil, false, false, fmt.Errorf("MOPS 資料源尚未接線")
	}
	key := cache.KeyString(model.SourceMOPS, string(ds), dataDate, "", nil)
	return fetchNormalize[[]provider.ESGRow](a, ctx, string(ds), dataDate, key, func() ([]byte, error) {
		req := provider.RawRequest{URL: a.mops.URL(ds, nil)}
		resp, err := a.mops.Fetch(ctx, req)
		if err != nil {
			return nil, err
		}
		if err := a.mops.Validate(resp); err != nil {
			return nil, err
		}
		return a.mops.Normalize(resp)
	})
}

// fetchESGTopic 依來源分派單一 topic 抓取。
func (a *App) fetchESGTopic(ctx context.Context, src, dataDate string, topic int) ([]provider.ESGRow, bool, bool, error) {
	switch src {
	case model.SourceTWSEAPI:
		return a.fetchESGTWSETopic(ctx, dataDate, topic)
	case model.SourceMOPS:
		return a.fetchESGMOPSTopic(ctx, dataDate, topic)
	}
	return nil, false, false, fmt.Errorf("mcp: 未知 ESG 來源 %q", src)
}

// esgProbeResult 為速度探測結果。
type esgProbeResult struct {
	src      string
	duration time.Duration
	err      error
}

// probeESGSource 量測指定來源 topic=1 抓取耗時（探測同時暖快取）。
func (a *App) probeESGSource(ctx context.Context, src, dataDate string) esgProbeResult {
	start := a.now()
	rows, _, _, err := a.fetchESGTopic(ctx, src, dataDate, 1)
	d := a.now().Sub(start)
	if len(rows) == 0 && err == nil {
		err = fmt.Errorf("esg 探測無資料列")
	}
	return esgProbeResult{src: src, duration: d, err: err}
}

// esgProbeTimeout 為單一來源速度探測之上限（探測同時暖快取；逾時視為失敗）。
const esgProbeTimeout = 45 * time.Second

// resolveESGPrimary 回傳目前主來源；未定時「循序」探測兩來源（TWSE 先、
// MOPS 後，各量測 topic=1 抓取耗時），以成功者中較快者為主來源（決策依據
// 記 log）。循序而非並發：並發僅省數秒，卻使探測路徑難以觀察與測試。
// 回傳值：主來源與是否本次執行了探測。
func (a *App) resolveESGPrimary(ctx context.Context, dataDate string) (string, bool) {
	a.esgMu.Lock()
	defer a.esgMu.Unlock()
	if a.esgPrimary != "" {
		return a.esgPrimary, false
	}

	pctx, cancel := context.WithTimeout(ctx, esgProbeTimeout)
	defer cancel()

	resTWSE := a.probeESGSource(pctx, model.SourceTWSEAPI, dataDate)
	resMOPS := a.probeESGSource(pctx, model.SourceMOPS, dataDate)

	switch {
	case resTWSE.err == nil && resMOPS.err == nil:
		if resTWSE.duration <= resMOPS.duration {
			a.esgPrimary = model.SourceTWSEAPI
		} else {
			a.esgPrimary = model.SourceMOPS
		}
		a.logger.Info("ESG 速度選源完成",
			"twse_api_ms", resTWSE.duration.Milliseconds(),
			"mops_ms", resMOPS.duration.Milliseconds(),
			"primary", a.esgPrimary)
	case resTWSE.err == nil:
		a.esgPrimary = model.SourceTWSEAPI
		a.logger.Warn("ESG 速度選源：MOPS 探測失敗，採 TWSE_API", "err", resMOPS.err)
	case resMOPS.err == nil:
		a.esgPrimary = model.SourceMOPS
		a.logger.Warn("ESG 速度選源：TWSE_API 探測失敗，採 MOPS", "err", resTWSE.err)
	default:
		// 兩者皆失敗：不設偏好（每次呼叫重試探測），由呼叫端回傳錯誤。
		a.logger.Warn("ESG 速度選源：兩來源皆失敗", "twse_err", resTWSE.err, "mops_err", resMOPS.err)
		return "", true
	}
	return a.esgPrimary, true
}

// swapESGPrimary 反轉主來源偏好（fallback 成功後呼叫）。
func (a *App) swapESGPrimary(from string) {
	a.esgMu.Lock()
	defer a.esgMu.Unlock()
	if a.esgPrimary == from {
		switch from {
		case model.SourceTWSEAPI:
			a.esgPrimary = model.SourceMOPS
		case model.SourceMOPS:
			a.esgPrimary = model.SourceTWSEAPI
		}
		a.logger.Info("ESG 主來源切換", "from", from, "to", a.esgPrimary)
	}
}

// fetchESGTopics 以速度選源＋fallback 抓取所選 topics，回傳各 topic 之列
// （以 topic 為鍵）與實際使用之來源。per-topic 容錯：主來源上單一 topic
// 失敗僅記 log、繼續其餘；「全部 topic 失敗」才降級 fallback（成功則反轉
// 偏好）；雙來源皆全失敗才回傳錯誤。
func (a *App) fetchESGTopics(ctx context.Context, dataDate string, topics []int) (map[int][]provider.ESGRow, string, bool, bool, error) {
	primary, _ := a.resolveESGPrimary(ctx, dataDate)

	// fetchAll 盡力抓取所有 topic；回傳成功者與是否「全數失敗」。
	fetchAll := func(src string) (map[int][]provider.ESGRow, bool, bool, bool) {
		all := make(map[int][]provider.ESGRow, len(topics))
		cachedAny, staleAny := false, false
		failCount := 0
		for _, t := range topics {
			rows, cached, stale, err := a.fetchESGTopic(ctx, src, dataDate, t)
			if err != nil {
				a.logger.Warn("ESG topic 抓取失敗", "source", src, "topic", t, "err", err)
				failCount++
				continue
			}
			all[t] = rows
			cachedAny = cachedAny || cached
			staleAny = staleAny || stale
		}
		return all, cachedAny, staleAny, failCount == len(topics)
	}

	if primary == "" {
		// 探測階段兩來源皆失敗：直接回錯。
		return nil, "", false, false, fmt.Errorf("ESG 兩來源皆不可用（速度選源探測失敗）")
	}

	rows, cachedAny, staleAny, allFailed := fetchAll(primary)
	if !allFailed {
		return rows, primary, cachedAny, staleAny, nil
	}

	// 主來源全數失敗 → fallback 另一來源，成功則反轉偏好。
	other := model.SourceTWSEAPI
	if primary == model.SourceTWSEAPI {
		other = model.SourceMOPS
	}
	a.logger.Warn("ESG 主來源全數失敗，降級 fallback", "primary", primary)
	rows2, cachedAny2, staleAny2, allFailed2 := fetchAll(other)
	if allFailed2 {
		return nil, "", false, false, fmt.Errorf("ESG 雙來源皆失敗（%s 與 %s topic %v 全數抓取失敗）", primary, other, topics)
	}
	a.swapESGPrimary(primary)
	return rows2, other, cachedAny2, staleAny2, nil
}

// filterTopics 正規化 topics 參數：空 → 全部 [1..8]；去重、排序、範圍驗證。
func filterTopics(raw []any) ([]int, error) {
	if len(raw) == 0 {
		topics := make([]int, 0, esgTopicCount)
		for i := 1; i <= esgTopicCount; i++ {
			topics = append(topics, i)
		}
		return topics, nil
	}
	seen := make(map[int]bool, len(raw))
	out := make([]int, 0, len(raw))
	for _, v := range raw {
		n, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("參數 topics 須為整數陣列")
		}
		t := int(n)
		if t < 1 || t > esgTopicCount {
			return nil, fmt.Errorf("參數 topics 值須在 1~%d，實際 %d", esgTopicCount, t)
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	sort.Ints(out)
	return out, nil
}

// handlerGetESGReport：ESG 揭露報告（T037 雙來源：TWSE OpenAPI / MOPS CSV，
// 速度選源＋fallback）。topics 可選（預設全部 8 主題）；另附公司治理規程
// （t187ap32_L）維持既有行為。
func handlerGetESGReport(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	code, _ := args["symbol"].(string)
	sym, err := a.symbolOf(code)
	if err != nil {
		return HandlerResult{}, err
	}
	topicsRaw, _ := args["topics"].([]any)
	topics, err := filterTopics(topicsRaw)
	if err != nil {
		return HandlerResult{}, err
	}

	dataDate := a.now().Format("2006-01-02")
	rowsByTopic, usedSource, cachedAny, staleAny, err := a.fetchESGTopics(ctx, dataDate, topics)
	if err != nil {
		return HandlerResult{}, err
	}

	out := model.ESGReport{Symbol: sym.Code, Name: sym.Name, Market: sym.Market, Topics: make([]model.ESGTopic, 0)}
	for _, t := range topics {
		// 同公司同 topic 取報告年度最新（CSV/JSON 皆可能多年度）。
		var latest *provider.ESGRow
		for i := range rowsByTopic[t] {
			r := &rowsByTopic[t][i]
			if r.Code != sym.Code {
				continue
			}
			if latest == nil || r.Year > latest.Year {
				latest = r
			}
		}
		if latest == nil {
			continue
		}
		out.Topics = append(out.Topics, model.ESGTopic{
			Topic: esgTopicNames[t], Year: latest.Year, ReportDate: latest.ReportDate, Fields: latest.Fields,
		})
	}

	// 公司治理規程（t187ap32_L，維持既有附加行為；失敗不阻擋 ESG 主體）。
	govRows, cachedGov, staleGov, gerr := fetchNormalize[[]provider.GovernanceRow](a, ctx,
		string(provider.TWSEAPIGovernance), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIGovernance), dataDate, "", nil),
		func() ([]byte, error) { return a.fetchAPIRaw(ctx, provider.TWSEAPIGovernance, nil) })
	if gerr == nil {
		for _, r := range govRows {
			if r.Code == sym.Code {
				out.Topics = append(out.Topics, model.ESGTopic{
					Topic: "公司治理規程", ReportDate: r.ReportDate,
					Fields: map[string]string{"公司治理之相關規程規則": r.Rules},
				})
				cachedAny, staleAny = cachedAny || cachedGov, staleAny || staleGov
				break
			}
		}
	}

	if len(out.Topics) == 0 {
		return HandlerResult{}, fmt.Errorf("代碼 %s 無 ESG 揭露資料（上櫃公司官方未提供或該公司未揭露）", sym.Code)
	}
	ttl, _ := a.ttlOf(string(cache.DatasetESG))
	lg := postLineage(usedSource, dataDate, cachedAny, staleAny, ttl)
	return HandlerResult{Data: out, Lineage: lg}, nil
}

// esgRefineryField 為 topic 15 之煉油廠數量欄位名（T065）。
const esgRefineryField = "在人口密集地區的煉油廠數量(座)"

// handlerGetRefineriesPopulatedAreas：人口密集區設有煉油廠之上市公司
//（ESG topic 15，排除零值與 N/A；T065）。
func handlerGetRefineriesPopulatedAreas(a *App, args map[string]any) (HandlerResult, error) {
	ctx := context.Background()
	dataDate := a.now().Format("2006-01-02")
	rows, cached, stale, err := fetchNormalize[[]provider.ESGRow](a, ctx,
		string(provider.TWSEAPIESG), dataDate,
		cache.KeyString(model.SourceTWSEAPI, string(provider.TWSEAPIESG), dataDate, "", map[string]string{"topic": "15"}),
		func() ([]byte, error) {
			return a.fetchAPIRaw(ctx, provider.TWSEAPIESG, url.Values{"topic": {"15"}})
		})
	if err != nil {
		return HandlerResult{}, err
	}
	ttl, _ := a.ttlOf(string(provider.TWSEAPIESG))
	lineage := postLineage(model.SourceTWSEAPI, dataDate, cached || stale, stale, ttl)

	out := make([]map[string]any, 0)
	for _, r := range rows {
		v := strings.TrimSpace(r.Fields[esgRefineryField])
		if v == "" || v == "N/A" || commaFloat(v) == 0 {
			continue
		}
		out = append(out, map[string]any{
			"code": r.Code, "name": r.Name, "year": r.Year, "refineries": v,
		})
	}
	return HandlerResult{Data: out, Lineage: lineage}, nil
}
