package model

import (
	"fmt"
	"strings"
)

// 市場別（§5.2）。Symbol.Market 僅允許下列兩值。
const (
	MarketTSE = "tse" // 台灣證券交易所（上市）
	MarketOTC = "otc" // 證券櫃檯買賣中心（上櫃）
)

// Symbol 是統一之證券代號模型（§5.2）。
// 所有工具輸入之 symbol 統一為 6 碼數字字串（"2330"），市場別由 Symbol Registry 判定。
type Symbol struct {
	Code     string `json:"code"`     // "2330"
	Market   string `json:"market"`   // "tse" | "otc"
	Name     string `json:"name"`     // 公司名稱
	Category string `json:"category"` // 產業別（來自 TWSE/TPEx 官方分類）
}

// ValidMarket 檢查市場別是否合法。
func ValidMarket(m string) bool {
	return m == MarketTSE || m == MarketOTC
}

// Validate 檢查 Symbol 是否符合 §5.2 契約（代碼為 4~6 位字串，
// 4~5 碼為純數字（股票），6 碼允許英數混合（ETF/ETN 如 00400A））。
func (s Symbol) Validate() error {
	if len(s.Code) < 4 || len(s.Code) > 6 {
		return fmt.Errorf("model: code %q 長度必須為 4~6 碼", s.Code)
	}
	// 4~5 碼必須為純數字；6 碼允許英數混合（ETF/ETN）
	if len(s.Code) <= 5 && !isDigits(s.Code) {
		return fmt.Errorf("model: code %q 必須為數字字串", s.Code)
	}
	if len(s.Code) == 6 && !isAlphanumeric(s.Code) {
		return fmt.Errorf("model: code %q 必須為英數字串", s.Code)
	}
	if !ValidMarket(s.Market) {
		return fmt.Errorf("model: market %q 必須為 %q 或 %q", s.Market, MarketTSE, MarketOTC)
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("model: name 不得為空")
	}
	return nil
}

func isAlphanumeric(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

// Exch 組裝 MIS 用之 ex_ch 參數（§5.2）。
// 一律由 Symbol 組裝（tse_2330.tw / otc_6547.tw），禁止簡易猜測市場別。
func (s Symbol) Exch() string {
	if s.Market == MarketOTC {
		return "otc_" + s.Code + ".tw"
	}
	return "tse_" + s.Code + ".tw"
}

// IsETF 判斷是否為上市 ETF（代碼 6 碼且以 00 開頭，與 pkg/engine/composite/screen.go isETF 一致）。
func (s Symbol) IsETF() bool {
	return len(s.Code) == 6 && s.Code[:2] == "00"
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
