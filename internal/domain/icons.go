package domain

import (
	"sort"
	"strings"
)

const (
	DefaultMemberIcon      = "user"
	DefaultInstitutionIcon = "bank"
	DefaultGroupIcon       = "folder"
)

var accountTypeDefaultIcons = map[AccountType]string{
	TypeCashOnHand: "cash", TypeBankAccount: "bank", TypeBrokerage: "brokerage",
	TypeInvestmentAccount: "investment", TypeCryptoExchange: "bitcoin", TypeDigitalWallet: "wallet-cards",
	TypePension: "pension", TypeInsurancePolicy: "shield-plus", TypeProperty: "home",
	TypeVehicle: "car", TypeCollectible: "gem", TypeReceivable: "receivable",
	TypeCreditCard: "credit-card", TypeLoan: "banknote-down", TypeOther: "account",
}

var instrumentTypeDefaultIcons = map[InstrumentType]string{
	InstrumentStock: "stock", InstrumentETF: "pie-chart", InstrumentMutualFund: "chart",
	InstrumentCrypto: "bitcoin", InstrumentBond: "document", InstrumentPreciousMetal: "gem",
	InstrumentBankInvestmentProduct: "bank", InstrumentOther: "investment",
}

var institutionTypeDefaultIcons = map[InstitutionType]string{
	InstitutionBank: "bank", InstitutionBrokerage: "brokerage", InstitutionInsurer: "shield-plus",
	InstitutionExchange: "market", InstitutionEmployer: "building", InstitutionGovernment: "landmark",
	InstitutionOther: "building",
}

func DefaultAccountIcon(accountType AccountType) string { return accountTypeDefaultIcons[accountType] }
func DefaultInstrumentIcon(instrumentType InstrumentType) string {
	return instrumentTypeDefaultIcons[instrumentType]
}
func DefaultIconForInstitutionType(institutionType InstitutionType) string {
	return institutionTypeDefaultIcons[institutionType]
}

var supportedIconKeys = map[string]struct{}{
	"account":        {},
	"armchair":       {},
	"badge-dollar":   {},
	"badge-euro":     {},
	"badge-franc":    {},
	"badge-rupee":    {},
	"badge-pound":    {},
	"badge-percent":  {},
	"badge-ruble":    {},
	"badge-yen":      {},
	"bank":           {},
	"bank-branch":    {},
	"banknote-down":  {},
	"banknote-up":    {},
	"bitcoin":        {},
	"brokerage":      {},
	"brokerage-cash": {},
	"building":       {},
	"calendar":       {},
	"calendar-clock": {},
	"car":            {},
	"camera":         {},
	"card":           {},
	"cash":           {},
	"chart":          {},
	"circle-check":   {},
	"circle-dollar":  {},
	"circle-percent": {},
	"coins":          {},
	"computer":       {},
	"credit-card":    {},
	"currency":       {},
	"document":       {},
	"dollar":         {},
	"download":       {},
	"euro":           {},
	"file":           {},
	"folder":         {},
	"goal":           {},
	"gem":            {},
	"grid":           {},
	"history":        {},
	"home":           {},
	"info":           {},
	"insurance":      {},
	"investment":     {},
	"landmark":       {},
	"liability":      {},
	"list":           {},
	"mail":           {},
	"market":         {},
	"money":          {},
	"percent":        {},
	"pension":        {},
	"pie-chart":      {},
	"property":       {},
	"receivable":     {},
	"receipt":        {},
	"retirement":     {},
	"savings":        {},
	"search":         {},
	"settings":       {},
	"shield":         {},
	"shield-plus":    {},
	"stock":          {},
	"storage":        {},
	"trending-up":    {},
	"upload":         {},
	"vault":          {},
	"visibility":     {},
	"wallet":         {},
	"wallet-cards":   {},
	"warning":        {},
	"user":           {},
	"yen":            {},
}

func SupportedIconKeys() []string {
	result := make([]string, 0, len(supportedIconKeys))
	for key := range supportedIconKeys {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

func ValidateIconKey(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if _, ok := supportedIconKeys[value]; !ok {
		return validation("iconKey", "unsupported icon")
	}
	return nil
}

func normalizedIconKey(value *string, fallback string) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return &fallback, nil
	}
	normalized := strings.TrimSpace(*value)
	if err := ValidateIconKey(normalized); err != nil {
		return nil, err
	}
	return &normalized, nil
}
