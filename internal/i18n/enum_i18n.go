package i18n

import "github.com/waltwang/nestworth-go/internal/settings"

// The domain enum list is intentionally registered in one place so every
// value that can reach a Select control has an explicit translation in every
// supported language.
func init() {
	translations := map[string]struct {
		english     string
		simplified  string
		traditional string
	}{
		"enum.cash":                    {"Cash", "现金", "現金"},
		"enum.digital_wallet":          {"Digital wallet", "数字钱包", "數位錢包"},
		"enum.broker_cash":             {"Broker cash", "券商现金", "券商現金"},
		"enum.other_cash_equivalent":   {"Other cash equivalent", "其他现金等价物", "其他現金等價物"},
		"enum.investment_fund_account": {"Investment fund account", "投资基金账户", "投資基金帳戶"},
		"enum.bank_investment_product": {"Bank investment product", "银行理财产品", "銀行投資產品"},
		"enum.insurance":               {"Insurance", "保险", "保險"},
		"enum.manual_investment":       {"Manual investment", "手工投资", "手工投資"},
		"enum.other_investment":        {"Other investment", "其他投资", "其他投資"},
		"enum.collectible":             {"Collectible", "收藏品", "收藏品"},
		"enum.other_property":          {"Other property", "其他财产", "其他財產"},
		"enum.other_receivable":        {"Other receivable", "其他应收款", "其他應收款"},
		"enum.other_liability":         {"Other liability", "其他负债", "其他負債"},
		"enum.holdings":                {"Holdings", "持仓", "持倉"},
	}
	for key, translation := range translations {
		catalogs[settings.LanguageEnglish][key] = translation.english
		catalogs[settings.LanguageZhCN][key] = translation.simplified
		catalogs[settings.LanguageZhTW][key] = translation.traditional
	}
}
