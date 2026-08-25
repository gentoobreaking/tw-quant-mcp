package mcp

// registry_broker.go 登錄券商資料工具（T046-T054）。
// 資料源：TWSE-API openapi（t187apXX / OpenData_BRKXX / brokerService/*），
// 全為報表清單型，走 apiListSpec 泛用框架。

import "tw-quant-mcp/pkg/provider"

func registerBrokerTools(r *Registry) {
	brokerListSchema := func(withName bool) map[string]any {
		props := map[string]any{
			"limit":  map[string]any{"type": "integer", "default": 50, "minimum": 1, "description": "回傳筆數上限"},
			"offset": map[string]any{"type": "integer", "default": 0, "minimum": 0, "description": "跳過前 N 筆"},
		}
		if withName {
			props["name"] = map[string]any{"type": "string", "description": "券商名稱關鍵字（選填）"}
		}
		return map[string]any{"type": "object", "properties": props}
	}
	reg := func(symbol, desc string, withName bool, ds provider.TWSEAPIDataset) {
		r.Register(ToolDef{
			Symbol:      symbol,
			Name:        symbol,
			Description: desc,
			Schema:      brokerListSchema(withName),
			ReadOnly:    true,
			Handler:     apiListSpec{ds: ds}.handler(),
		})
	}

	reg("get_broker_basic_info",
		"查詢證券商基本資料（TWSE-API t187ap18，T046）。可選 name 過濾券商簡稱。", true, provider.TWSEAPIBrokerBasic)       // T046
	reg("get_broker_branch_info",
		"查詢證券商分公司基本資料（TWSE-API OpenData_BRK02，T047）。可選 name 過濾券商名稱。", true, provider.TWSEAPIBrokerBranch)   // T047
	reg("get_broker_electronic_trading_statistics",
		"查詢電子式交易統計資訊（TWSE-API t187ap19，T048）。", false, provider.TWSEAPIBrokerElec)                              // T048
	reg("get_broker_gender_statistics",
		"查詢證券商營業員男女人數統計資料（TWSE-API OpenData_BRK01，T049）。", false, provider.TWSEAPIBrokerGender)              // T049
	reg("get_broker_headquarters_info",
		"查詢證券商本公司（總公司）基本資料（TWSE-API brokerService/brokerList，T050）。可選 name 過濾。", true, provider.TWSEAPIBrokerHQ) // T050
	reg("get_broker_income_expenditure",
		"查詢證券商損益彙總資料（TWSE-API t187ap21，T051）。可選 name 過濾券商名稱。", true, provider.TWSEAPIBrokerIncome)             // T051
	reg("get_broker_monthly_statements",
		"查詢證券商月報表資料（TWSE-API t187ap20，T052）。可選 name 過濾券商名稱。", true, provider.TWSEAPIBrokerMonthly)               // T052
	reg("get_broker_service_personnel",
		"查詢證券商從業人員統計資料（TWSE-API t187ap01，T053）。", false, provider.TWSEAPIBrokerPersonnel)                       // T053
	reg("get_brokers_offering_regular_investment",
		"查詢開辦定期定額業務證券商名單（TWSE-API brokerService/secRegData，T054）。可選 name 過濾。", true, provider.TWSEAPIBrokerRegInv) // T054
}
