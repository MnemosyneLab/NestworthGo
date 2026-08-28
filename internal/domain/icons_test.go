package domain

import (
	"reflect"
	"testing"
)

func TestSupportedIconKeysAreSortedAndValidate(t *testing.T) {
	keys := SupportedIconKeys()
	if len(keys) == 0 {
		t.Fatal("supported icon catalog is empty")
	}
	if !reflect.DeepEqual(keys, append([]string(nil), sortedStrings(keys)...)) {
		t.Fatalf("icon keys are not sorted: %v", keys)
	}
	for _, key := range keys {
		if err := ValidateIconKey(key); err != nil {
			t.Fatalf("ValidateIconKey(%q): %v", key, err)
		}
	}
	if err := ValidateIconKey("not-in-catalog"); err == nil {
		t.Fatal("unknown icon key was accepted")
	}
}

func TestAccountTypeDefaultIcons(t *testing.T) {
	want := map[AccountType]string{
		TypeCashOnHand: "cash", TypeBankAccount: "bank", TypeBrokerage: "brokerage",
		TypeInvestmentAccount: "investment", TypeCryptoExchange: "bitcoin", TypeDigitalWallet: "wallet-cards",
		TypePension: "pension", TypeInsurancePolicy: "shield-plus", TypeProperty: "home",
		TypeVehicle: "car", TypeCollectible: "gem", TypeReceivable: "receivable",
		TypeCreditCard: "credit-card", TypeLoan: "banknote-down", TypeOther: "account",
	}
	for accountType, icon := range want {
		if got := DefaultAccountIcon(accountType); got != icon {
			t.Errorf("DefaultAccountIcon(%q) = %q, want %q", accountType, got, icon)
		}
	}
}

func TestInstrumentTypeDefaultIcons(t *testing.T) {
	want := map[InstrumentType]string{
		InstrumentStock: "stock", InstrumentETF: "pie-chart", InstrumentMutualFund: "chart",
		InstrumentCrypto: "bitcoin", InstrumentBond: "document", InstrumentPreciousMetal: "gem",
		InstrumentBankInvestmentProduct: "bank", InstrumentOther: "investment",
	}
	for instrumentType, icon := range want {
		if got := DefaultInstrumentIcon(instrumentType); got != icon {
			t.Errorf("DefaultInstrumentIcon(%q) = %q, want %q", instrumentType, got, icon)
		}
	}
}

func TestInstitutionTypeDefaultIcons(t *testing.T) {
	want := map[InstitutionType]string{
		InstitutionBank: "bank", InstitutionBrokerage: "brokerage", InstitutionInsurer: "shield-plus",
		InstitutionExchange: "market", InstitutionEmployer: "building", InstitutionGovernment: "landmark",
		InstitutionOther: "building",
	}
	for institutionType, icon := range want {
		if got := DefaultIconForInstitutionType(institutionType); got != icon {
			t.Errorf("DefaultIconForInstitutionType(%q) = %q, want %q", institutionType, got, icon)
		}
	}
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}
