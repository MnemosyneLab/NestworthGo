package domain

import (
	"sort"
	"strings"
)

const (
	DefaultAccountIcon     = "wallet"
	DefaultInstitutionIcon = "bank"
	DefaultGroupIcon       = "folder"
)

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
	"grid":           {},
	"history":        {},
	"home":           {},
	"info":           {},
	"insurance":      {},
	"investment":     {},
	"liability":      {},
	"list":           {},
	"mail":           {},
	"market":         {},
	"media":          {},
	"money":          {},
	"percent":        {},
	"pension":        {},
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
