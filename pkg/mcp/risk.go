package mcp

import (
	"fmt"
	"sync"

	"tw-quant-mcp/pkg/model"
)

// AlertList 為一檔名單內股票之公告資訊（注意/處置/停資停券）。
type AlertList struct {
	// Scope 市場別：model.MarketTSE | model.MarketOTC。
	Scope string
	// Kind 名單種類：attention | disposition | margin（停資）| short（停券）。
	Kind string
	// Code 為官方代碼。
	Code string
	// Info 為官方原文資訊（如注意交易資訊、處置原因）。
	Info string
	// Period 為期間（如處置期間 "1150803~1150814"）。
	Period string
}

// DaytradeScanner 是 scan_daytrade_eligibility 之記憶體比對器：
// 以盤後名單（T008 TWSE-WEB / T009 TPEx 名單，AddLists 注入）掃描
// 單檔之當沖資格/處置/注意/停資停券狀態。名單未載入時以「無名單資料」
// 狀態回傳，並於 summary 說明（名單供應器於後續任務接線）。
type DaytradeScanner struct {
	reg *model.Registry

	mu     sync.RWMutex
	date   string
	byKey  map[string]*AlertList // key = scope|kind|code
	loaded bool
}

// NewDaytradeScanner 建立掃描器。
func NewDaytradeScanner(reg *model.Registry) *DaytradeScanner {
	return &DaytradeScanner{
		reg:   reg,
		byKey: make(map[string]*AlertList),
	}
}

// AddLists 以指定日期與市場注入名單（可重複呼叫累加；不同日期時清空舊資料）。
func (s *DaytradeScanner) AddLists(date, scope string, lists []AlertList) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.date != date || !s.loaded {
		s.date = date
		s.byKey = make(map[string]*AlertList)
	}
	for i := range lists {
		l := lists[i]
		if l.Scope == "" {
			l.Scope = scope
		}
		s.byKey[l.Scope+"|"+l.Kind+"|"+l.Code] = &l
	}
	s.loaded = true
}

// Loaded 回傳是否已有任何名單資料。
func (s *DaytradeScanner) Loaded() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loaded
}

// Date 回傳名單日期。
func (s *DaytradeScanner) Date() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.date
}

// Scan 掃描指定代碼（需已註冊於 Symbol Registry），回傳 §10.A
// scan_daytrade_eligibility 之 data（model.DaytradeScan）。
// 代碼未註冊回傳錯誤；名單未載入時回傳無名單之掃描結果。
func (s *DaytradeScanner) Scan(code string) (model.DaytradeScan, error) {
	sym, ok := s.reg.Lookup(code)
	if !ok {
		return model.DaytradeScan{}, fmt.Errorf("mcp: 未知代號 %q（未註冊於 Symbol Registry）", code)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	scan := model.DaytradeScan{
		Symbol:          sym.Code,
		Name:            sym.Name,
		Market:          sym.Market,
		Date:            s.date,
		DaytradeAllowed: true,
	}
	var notes []string
	if att, ok := s.byKey[sym.Market+"|attention|"+code]; ok {
		scan.IsAttention = true
		scan.AttentionInfo = att.Info
	}
	if disp, ok := s.byKey[sym.Market+"|disposition|"+code]; ok {
		scan.IsDisposition = true
		scan.DispositionPeriod = disp.Period
		scan.DispositionInfo = disp.Info
	}
	if m, ok := s.byKey[sym.Market+"|margin|"+code]; ok {
		scan.MarginSuspended = true
		scan.MarginNote = m.Info
	}
	if sh, ok := s.byKey[sym.Market+"|short|"+code]; ok {
		scan.ShortSuspended = true
		if scan.MarginNote == "" {
			scan.MarginNote = sh.Info
		} else {
			scan.MarginNote += "；" + sh.Info
		}
	}
	if !s.loaded {
		scan.DaytradeNote = "名單資料尚未載入（盤後名單供應器未啟動）；以無名單狀態評估"
		notes = append(notes, scan.DaytradeNote)
	}
	if scan.IsAttention {
		notes = append(notes, fmt.Sprintf("為注意股票：%s", scan.AttentionInfo))
	}
	if scan.IsDisposition {
		notes = append(notes, fmt.Sprintf("為處置股票（%s）：%s", scan.DispositionPeriod, scan.DispositionInfo))
	}
	if scan.MarginSuspended {
		notes = append(notes, "融資交易暫停")
	}
	if scan.ShortSuspended {
		notes = append(notes, "融券交易暫停")
	}
	if scan.MarginSuspended && scan.ShortSuspended {
		scan.DaytradeAllowed = false
		notes = append(notes, "現股當沖與資券當沖皆不適用（停資停券）")
	} else if scan.IsDisposition {
		scan.DaytradeNote = appendNote(scan.DaytradeNote, "處置期間可能限制當沖，依證交所規定為準")
	}
	scan.Summary = notes
	return scan, nil
}

func appendNote(n, extra string) string {
	if n == "" {
		return extra
	}
	return n + "；" + extra
}
