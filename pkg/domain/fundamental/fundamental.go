// Package fundamental 實作「財務基本面檢核」情境（v2.1 §9.8、§10.D）。
// 提供 get_financial_health_check 五面向評分（獲利能力/營運效率/
// 財務結構/償債能力/成長動能）之業務入口，為 §7 模組化邊界中
// pkg/domain/fundamental 之薄層：評分規則實作下沈至
// pkg/engine/composite（T017 下層引擎），本包不重複邏輯、僅對齊
// 介面並以類型別名維持與下層之型別相等。
package fundamental

import (
	"tw-quant-mcp/pkg/engine/composite"
)

// 以下型別別名對齊 composite 下層引擎（`=` 表示同一型別，可直接互轉）。
type (
	// HealthInput 為五面向評分之輸入。
	HealthInput = composite.HealthInput
	// DividendYear 為配息/發放紀錄（穩定配息指標，§10.D）。
	DividendYear = composite.DividendYear
	// HealthScore 為五面向評分之輸出。
	HealthScore = composite.HealthScore
	// ScoringConfig 為評分門檻設定。
	ScoringConfig = composite.ScoringConfig
)

// DefaultScoringConfig 回傳 v1 預設評分規則（薄層委託）。
func DefaultScoringConfig() ScoringConfig {
	return composite.DefaultScoringConfig()
}

// ScoreHealth 對單一標的進行五面向財務健康評分
// （§9.8 get_financial_health_check / §10.D）。
// 薄層入口，直接委託下層 composite.ScoreHealth。
func ScoreHealth(input HealthInput, cfg ScoringConfig) HealthScore {
	return composite.ScoreHealth(input, cfg)
}
