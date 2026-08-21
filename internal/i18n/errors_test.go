package i18n

import (
	"errors"
	"fmt"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestErrorCatalogsAreCompleteInEveryLanguage(t *testing.T) {
	languages := []settings.Language{settings.LanguageEnglish, settings.LanguageZhCN, settings.LanguageZhTW}
	for message, key := range errorMessageKeys {
		english := New(settings.LanguageEnglish).T(key)
		if english == "" {
			t.Errorf("error %q maps to %q without an English translation", message, key)
		}
		for _, language := range languages {
			got := New(language).T(key)
			if got == "" {
				t.Errorf("error %q key %q is empty for %q", message, key, language)
			}
			if language != settings.LanguageEnglish && got == english {
				t.Errorf("error %q key %q falls back to English for %q", message, key, language)
			}
		}
	}
}

func TestDomainEnumsHaveLocalizedCatalogEntries(t *testing.T) {
	values := []string{
		string(domain.CategoryCashEquivalent), string(domain.CategoryInvestment), string(domain.CategoryProperty), string(domain.CategoryReceivable), string(domain.CategoryLiability),
		string(domain.SecondaryCash), string(domain.SecondaryBankAccount), string(domain.SecondaryDigitalWallet), string(domain.SecondaryBrokerCash), string(domain.SecondaryOtherCashEquivalent),
		string(domain.SecondaryBrokerageAccount), string(domain.SecondaryInvestmentFundAccount), string(domain.SecondaryBankInvestmentProduct), string(domain.SecondaryInsurance), string(domain.SecondaryManualInvestment), string(domain.SecondaryOtherInvestment),
		string(domain.SecondaryRealEstate), string(domain.SecondaryVehicle), string(domain.SecondaryCollectible), string(domain.SecondaryOtherProperty),
		string(domain.SecondaryLoanReceivable), string(domain.SecondaryOtherReceivable), string(domain.SecondaryCreditCard), string(domain.SecondaryMortgage), string(domain.SecondaryAutoLoan), string(domain.SecondaryConsumerLoan), string(domain.SecondaryPersonalDebt), string(domain.SecondaryOtherLiability),
		string(domain.TrackingBalance), string(domain.TrackingManualValue), string(domain.TrackingHoldings),
	}
	for _, value := range values {
		key := "enum." + value
		english := New(settings.LanguageEnglish).T(key)
		if english == "" || english == key {
			t.Errorf("missing English enum translation for %q", value)
		}
		for _, language := range []settings.Language{settings.LanguageZhCN, settings.LanguageZhTW} {
			translated := New(language).T(key)
			if translated == "" || translated == english {
				t.Errorf("missing %q enum translation for %q: got %q", language, value, translated)
			}
		}
	}
}

func TestTranslateErrorLocalizesDomainFieldsAndMessages(t *testing.T) {
	_, err := domain.ParseOwnership(nil)
	requireError(t, err)
	if got, want := New(settings.LanguageZhCN).TranslateError(err), "所有人: 至少需要一位所有人"; got != want {
		t.Fatalf("TranslateError(ParseOwnership(nil)) = %q, want %q", got, want)
	}

	_, err = domain.ParseCurrency("US")
	requireError(t, err)
	if got, want := New(settings.LanguageZhTW).TranslateError(err), "貨幣: 必須恰好包含三個大寫字母"; got != want {
		t.Fatalf("TranslateError(ParseCurrency(%q)) = %q, want %q", "US", got, want)
	}
}

func TestTranslateErrorHandlesWrappedAndUnknownErrors(t *testing.T) {
	_, base := domain.ParseOwnership(nil)
	requireError(t, base)
	wrapped := fmt.Errorf("create account: %w", base)
	if got, want := New(settings.LanguageZhCN).TranslateError(wrapped), "所有人: 至少需要一位所有人"; got != want {
		t.Fatalf("TranslateError(wrapped domain error) = %q, want %q", got, want)
	}

	unknown := errors.New("a new error not in the catalog")
	if got := New(settings.LanguageZhCN).TranslateError(unknown); got != unknown.Error() {
		t.Fatalf("TranslateError(unknown) = %q, want English fallback %q", got, unknown)
	}
}

func requireError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
}
