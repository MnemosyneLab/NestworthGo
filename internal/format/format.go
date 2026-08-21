// Package format contains display-only formatting helpers. It deliberately
// works on decimal strings and time.Time values; it never performs a
// financial calculation or uses binary floating point.
package format

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/settings"
)

var currencySymbols = map[string]string{
	"AUD": "A$",
	"CNY": "¥",
	"EUR": "€",
	"GBP": "£",
	"HKD": "HK$",
	"JPY": "¥",
	"SGD": "S$",
	"TWD": "NT$",
	"USD": "$",
}

func CurrencySymbol(currency string) string {
	if symbol := currencySymbols[currency]; symbol != "" {
		return symbol
	}
	return currency
}

// Money formats a canonical decimal string for display. Invalid or
// unavailable values are returned unchanged so the UI never silently turns a
// missing value into zero.
func Money(value, currency string, preference settings.Settings) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "—" {
		return "—"
	}

	negative := false
	if strings.HasPrefix(value, "-") {
		negative = true
		value = value[1:]
	} else if strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	if !decimalString(value) {
		return value
	}

	integer, fraction := splitDecimal(value)
	integer, fraction = roundDecimal(integer, fraction, preference.DecimalPlaces)
	integer = groupInteger(integer, preference.GroupingSeparator)

	result := CurrencySymbol(currency) + " " + integer
	if preference.DecimalPlaces > 0 {
		result += preference.DecimalSeparator + fraction
	}
	if negative && result != "—" {
		return "-" + result
	}
	return result
}

func decimalString(value string) bool {
	seenDecimal := false
	seenDigit := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			seenDigit = true
		case r == '.' && !seenDecimal:
			seenDecimal = true
		default:
			return false
		}
	}
	return seenDigit
}

func splitDecimal(value string) (string, string) {
	parts := strings.SplitN(value, ".", 2)
	integer := parts[0]
	if integer == "" {
		integer = "0"
	}
	integer = strings.TrimLeft(integer, "0")
	if integer == "" {
		integer = "0"
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	return integer, fraction
}

func roundDecimal(integer, fraction string, places int) (string, string) {
	if places < 0 {
		places = 0
	}
	if len(fraction) <= places {
		return integer, fraction + strings.Repeat("0", places-len(fraction))
	}

	kept := fraction[:places]
	next := fraction[places]
	restNonZero := strings.Trim(fraction[places+1:], "0") != ""
	odd := lastDecimalDigitIsOdd(integer, kept)
	shouldRound := next > '5' || (next == '5' && (restNonZero || odd))
	if shouldRound {
		integer, kept = incrementDecimal(integer, kept)
	}
	return integer, kept + strings.Repeat("0", places-len(kept))
}

func lastDecimalDigitIsOdd(integer, fraction string) bool {
	if fraction != "" {
		return (fraction[len(fraction)-1]-'0')%2 == 1
	}
	return (integer[len(integer)-1]-'0')%2 == 1
}

func incrementDecimal(integer, fraction string) (string, string) {
	fractionLength := len(fraction)
	digits := []byte(integer + fraction)
	for index := len(digits) - 1; index >= 0; index-- {
		if digits[index] < '9' {
			digits[index]++
			break
		}
		digits[index] = '0'
		if index == 0 {
			digits = append([]byte{'1'}, digits...)
			break
		}
	}
	integerLength := len(digits) - fractionLength
	if integerLength < 1 {
		integerLength = 1
	}
	return string(digits[:integerLength]), string(digits[integerLength:])
}

func groupInteger(integer, grouping string) string {
	separator := grouping
	if grouping == settings.GroupingSpace {
		separator = " "
	}
	if grouping == settings.GroupingNone || separator == "" || len(integer) <= 3 {
		return integer
	}
	first := len(integer) % 3
	if first == 0 {
		first = 3
	}
	var builder strings.Builder
	builder.WriteString(integer[:first])
	for index := first; index < len(integer); index += 3 {
		builder.WriteString(separator)
		builder.WriteString(integer[index : index+3])
	}
	return builder.String()
}

func Location(name string) (*time.Location, error) {
	if name == "" || name == settings.TimezoneSystem {
		return time.Local, nil
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("load timezone %q: %w", name, err)
	}
	return location, nil
}

func DateTime(value time.Time, preference settings.Settings, language settings.Language) (string, error) {
	location, err := Location(preference.Timezone)
	if err != nil {
		return "", err
	}
	value = value.In(location)
	dateLayout := dateLayout(preference.DateFormat, language)
	timeLayout := "15:04"
	if preference.TimeFormat == settings.TimeFormat12 {
		timeLayout = "3:04 PM"
	}
	return value.Format(dateLayout + " " + timeLayout), nil
}

func dateLayout(value string, language settings.Language) string {
	switch value {
	case settings.DateFormatDayFirst:
		return "02/01/2006"
	case settings.DateFormatMonthFirst:
		return "01/02/2006"
	case settings.DateFormatLocalized:
		if language == settings.LanguageZhCN || language == settings.LanguageZhTW {
			return "2006年1月2日"
		}
		return "Jan 2, 2006"
	default:
		return "2006-01-02"
	}
}

func ValidateTimezone(value string) error {
	if strings.TrimSpace(value) == "" {
		return errors.New("timezone cannot be empty")
	}
	_, err := Location(value)
	return err
}
