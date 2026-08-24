package ui

import (
	"sort"

	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// commonCurrencyCodes keeps currency inputs selectable while still allowing a
// saved household to display a code that is not in the short common list.
var commonCurrencyCodes = []domain.CurrencyCode{
	"AUD", "CAD", "CHF", "CNY", "EUR", "GBP", "HKD", "INR", "JPY", "KRW", "NZD", "SGD", "USD",
}

func newCurrencySelect(current ...domain.CurrencyCode) *widget.Select {
	values := make(map[string]struct{}, len(commonCurrencyCodes)+len(current))
	for _, code := range commonCurrencyCodes {
		values[code.String()] = struct{}{}
	}
	for _, code := range current {
		if code != "" {
			values[code.String()] = struct{}{}
		}
	}
	options := make([]string, 0, len(values))
	for code := range values {
		options = append(options, code)
	}
	sort.Strings(options)
	selectWidget := widget.NewSelect(options, nil)
	for _, code := range current {
		if code != "" {
			selectWidget.SetSelected(code.String())
			break
		}
	}
	return selectWidget
}
