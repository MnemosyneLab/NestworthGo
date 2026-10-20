package domain

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestPortfolioDecimalsRejectNonCanonicalAndHonorBounds(t *testing.T) {
	for _, input := range []string{"01"} {
		if _, err := ParseQuantity(input); err == nil {
			t.Fatalf("ParseQuantity(%q) succeeded", input)
		}
	}
	if value, err := ParseQuantity("1.23000000"); err != nil || value.Canonical() != "1.23" {
		t.Fatalf("quantity canonical value = %q, err = %v", value.Canonical(), err)
	}
	if _, err := ParseQuantity("1000000000000000000"); err == nil {
		t.Fatal("quantity over the integer-digit maximum succeeded")
	}
	if _, err := ParseUnitPrice("1.000000001"); err == nil {
		t.Fatal("unit price over the scale maximum succeeded")
	}
	if _, err := ParseFxRate("0"); err == nil {
		t.Fatal("zero FX rate succeeded")
	}
	if rate, err := ParseFXRate("1.230000000000"); err != nil || rate.Canonical() != "1.23" {
		t.Fatalf("FX rate canonical value = %q, err = %v", rate.Canonical(), err)
	}
}

func TestPortfolioDecimalArithmeticIsCheckedAndUnrounded(t *testing.T) {
	quantity, err := ParseQuantity("3")
	if err != nil {
		t.Fatal(err)
	}
	price, err := ParseUnitPrice("700.12345678")
	if err != nil {
		t.Fatal(err)
	}
	value, err := quantity.Multiply(price)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "2100.37037034" {
		t.Fatalf("unrounded holding value = %s", value)
	}
	rate, err := ParseFxRate("6.9")
	if err != nil {
		t.Fatal(err)
	}
	converted, err := DivideByFxRate(decimal.RequireFromString("14490"), rate)
	if err != nil {
		t.Fatal(err)
	}
	if !converted.Equal(decimal.RequireFromString("2100")) {
		t.Fatalf("division result = %s", converted)
	}
	if _, err := quantity.Multiply(UnitPrice{value: decimal.RequireFromString("999999999999.99999999")}); err == nil {
		t.Fatal("overflowing multiplication succeeded")
	}
}

func TestNativeAmountAllowsFullQuantityAndUnitPricePrecision(t *testing.T) {
	value, err := ParseNativeAmount("11568.99735")
	if err != nil || value != "11568.99735" {
		t.Fatalf("native amount = %q, err = %v", value, err)
	}
	for _, input := range []string{"01"} {
		if _, err := ParseNativeAmount(input); err == nil {
			t.Fatalf("ParseNativeAmount(%q) succeeded", input)
		}
	}
}

func TestMoneyBoundaryUsesBankersRounding(t *testing.T) {
	even, err := NewMoney(decimal.RequireFromString("1.23445"), CurrencyCode("CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if even.CanonicalAmount() != "1.2344" {
		t.Fatalf("even midpoint rounded to %s", even.CanonicalAmount())
	}
	odd, err := NewMoney(decimal.RequireFromString("1.23455"), CurrencyCode("CNY"))
	if err != nil {
		t.Fatal(err)
	}
	if odd.CanonicalAmount() != "1.2346" {
		t.Fatalf("odd midpoint rounded to %s", odd.CanonicalAmount())
	}
}

func TestUnitPriceFromExactRoundsOnlyWhenScaleExceedsEight(t *testing.T) {
	exact, err := UnitPriceFromExact(decimal.RequireFromString("1.23456789"))
	if err != nil || exact.Canonical() != "1.23456789" {
		t.Fatalf("exact eight-decimal price = %q err=%v", exact.Canonical(), err)
	}
	// 1400/15 = 93.3 repeating; ninth fractional digit is 3, so banker's
	// rounding at eight places is 93.33333333.
	repeating, err := UnitPriceFromExact(decimal.RequireFromString("1400").Div(decimal.RequireFromString("15")))
	if err != nil || repeating.Canonical() != "93.33333333" {
		t.Fatalf("repeating average = %q err=%v", repeating.Canonical(), err)
	}
	// DivisionPrecision pads exact quotients with trailing zeros; those extra
	// scale digits must still round to the supported unit-price scale.
	exactQuotient, err := UnitPriceFromExact(decimal.RequireFromString("840").Div(decimal.RequireFromString("10")))
	if err != nil || exactQuotient.Canonical() != "84" {
		t.Fatalf("exact quotient = %q err=%v", exactQuotient.Canonical(), err)
	}
}

func TestConvertFXAmountUsesWholeAmountAndQuoteOrientation(t *testing.T) {
	quoteRate, err := ParseFxRate("8.123456789012")
	if err != nil {
		t.Fatal(err)
	}
	quote := FXQuote{BaseCurrency: "CNY", QuoteCurrency: "USD", Rate: quoteRate}
	value, err := ConvertFXAmount(decimal.RequireFromString("18236001.8236"), "USD", "CNY", quote)
	if err != nil {
		t.Fatal(err)
	}
	rounded, err := NewSignedMoney(value, "CNY")
	if err != nil || rounded.CanonicalAmount() != "2244857.3677" {
		t.Fatalf("whole amount division = %s, rounded=%s err=%v", value, rounded.CanonicalAmount(), err)
	}

	maxAmount := decimal.RequireFromString("999999999999.9999")
	seven, _ := ParseFxRate("7")
	quote.Rate = seven
	value, err = ConvertFXAmount(maxAmount, "USD", "CNY", quote)
	if err != nil {
		t.Fatal(err)
	}
	rounded, err = NewSignedMoney(value, "CNY")
	if err != nil || rounded.CanonicalAmount() != "142857142857.1428" {
		t.Fatalf("maximum amount division = %s, rounded=%s err=%v", value, rounded.CanonicalAmount(), err)
	}

	// The opposite quote orientation multiplies directly.
	quote.BaseCurrency, quote.QuoteCurrency = "USD", "CNY"
	quote.Rate, _ = ParseFxRate("7")
	value, err = ConvertFXAmount(decimal.RequireFromString("2.5"), "USD", "CNY", quote)
	if err != nil || !value.Equal(decimal.RequireFromString("17.5")) {
		t.Fatalf("direct quote conversion = %s err=%v", value, err)
	}

	quote.Rate, _ = ParseFxRate("1")
	for _, testCase := range []struct{ input, want string }{
		{input: "0.01025", want: "0.0102"},
		{input: "0.01035", want: "0.0104"},
		{input: "-0.01025", want: "-0.0102"},
		{input: "-0.01035", want: "-0.0104"},
	} {
		value, err := ConvertFXAmount(decimal.RequireFromString(testCase.input), "USD", "CNY", quote)
		if err != nil {
			t.Fatal(err)
		}
		rounded, err := NewSignedMoney(value, "CNY")
		if err != nil || rounded.CanonicalAmount() != testCase.want {
			t.Errorf("conversion of %s rounded to %s, want %s (err=%v)", testCase.input, rounded.CanonicalAmount(), testCase.want, err)
		}
	}
}

func TestConvertFXAmountRejectsInvalidPairsRatesAndOverflow(t *testing.T) {
	rate, _ := ParseFxRate("2")
	quote := FXQuote{BaseCurrency: "EUR", QuoteCurrency: "GBP", Rate: rate}
	if _, err := ConvertFXAmount(decimal.NewFromInt(1), "USD", "CNY", quote); err == nil {
		t.Fatal("mismatched quote pair converted an amount")
	}
	if _, err := ConvertFXAmount(decimal.NewFromInt(1), "USD", "CNY", FXQuote{BaseCurrency: "CNY", QuoteCurrency: "USD"}); err == nil {
		t.Fatal("zero FX rate converted an amount")
	}
	quote = FXQuote{BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate}
	if _, err := ConvertFXAmount(decimal.RequireFromString("999999999999.9999"), "USD", "CNY", quote); err == nil {
		t.Fatal("out-of-range conversion succeeded")
	}
}

func TestConvertFXAmountRejectsUnsupportedReciprocalPrecision(t *testing.T) {
	rate, _ := ParseFxRate("2")
	quote := FXQuote{BaseCurrency: "CNY", QuoteCurrency: "USD", Rate: rate}
	value := decimal.RequireFromString("0.00000000000000000000000000000000000000001")
	if _, err := ConvertFXAmount(value, "USD", "CNY", quote); err == nil {
		t.Fatal("reciprocal conversion accepted an amount scale beyond the supported bridge precision")
	}
}

func TestConvertFXAmountViaQuotesRoundsOnlyAfterBothLegs(t *testing.T) {
	inverseRate, _ := ParseFxRate("3")
	directRate, _ := ParseFxRate("0.03045")
	first := FXQuote{BaseCurrency: "USD", QuoteCurrency: "EUR", Rate: inverseRate}
	second := FXQuote{BaseCurrency: "USD", QuoteCurrency: "GBP", Rate: directRate}
	for _, testCase := range []struct{ input, want string }{
		{input: "1", want: "0.0102"},
		{input: "-1", want: "-0.0102"},
	} {
		converted, err := ConvertFXAmountViaQuotes(decimal.RequireFromString(testCase.input), "EUR", "USD", "GBP", first, second)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := NewSignedMoney(converted, "GBP")
		if err != nil || projected.CanonicalAmount() != testCase.want {
			t.Fatalf("two-leg conversion of %s = %s, want %s (err=%v)", testCase.input, projected.CanonicalAmount(), testCase.want, err)
		}
	}
}

func TestConvertFXAmountPrecisionAccountsForPositiveRateExponent(t *testing.T) {
	canonicalRate, err := ParseFxRate("10000000")
	if err != nil {
		t.Fatal(err)
	}
	exponentRate, err := NewFxRate(decimal.New(1, 7))
	if err != nil {
		t.Fatal(err)
	}
	for _, rate := range []FxRate{canonicalRate, exponentRate} {
		quote := FXQuote{BaseCurrency: "CNY", QuoteCurrency: "USD", Rate: rate}
		converted, convertErr := ConvertFXAmount(decimal.RequireFromString("500.1"), "USD", "CNY", quote)
		if convertErr != nil {
			t.Fatal(convertErr)
		}
		projected, projectErr := NewSignedMoney(converted, "CNY")
		if projectErr != nil || projected.CanonicalAmount() != "0.0001" {
			t.Fatalf("rate %s conversion = %s, want 0.0001 (err=%v)", rate, projected.CanonicalAmount(), projectErr)
		}
	}
}

func TestConvertFXAmountRetainsIntermediatePrecisionForAggregation(t *testing.T) {
	rate, _ := ParseFxRate("3")
	quote := FXQuote{BaseCurrency: "CNY", QuoteCurrency: "USD", Rate: rate}
	converted, err := ConvertFXAmount(decimal.NewFromInt(1), "USD", "CNY", quote)
	if err != nil {
		t.Fatal(err)
	}
	if converted.String() != "0.3333333333333333" {
		t.Fatalf("reciprocal intermediate precision = %s, want at least 16 fractional digits", converted)
	}
}
