package domain

import (
	"testing"
	"time"
)

func TestLegalAccountCombinationsAreClosed(t *testing.T) {
	if got := len(LegalAccountCombinations()); got != 24 {
		t.Fatalf("combination count = %d, want 24", got)
	}
	valid := []struct {
		accountType AccountType
		role        BalanceSheetRole
		tracking    TrackingMode
	}{
		{TypeCashOnHand, RoleAsset, TrackingBalance},
		{TypeBankAccount, RoleAsset, TrackingBalance},
		{TypeBankAccount, RoleAsset, TrackingHoldings},
		{TypeBrokerage, RoleAsset, TrackingHoldings},
		{TypeBrokerage, RoleAsset, TrackingManualValue},
		{TypeCryptoExchange, RoleAsset, TrackingHoldings},
		{TypeInsurancePolicy, RoleAsset, TrackingManualValue},
		{TypeCreditCard, RoleLiability, TrackingBalance},
		{TypeLoan, RoleLiability, TrackingBalance},
		{TypeOther, RoleAsset, TrackingHoldings},
		{TypeOther, RoleLiability, TrackingBalance},
	}
	for _, row := range valid {
		if !IsValidAccountCombination(row.accountType, row.role, row.tracking) {
			t.Fatalf("rejected legal combination %+v", row)
		}
	}
}

func TestIllegalAccountCombinationsAreRejected(t *testing.T) {
	illegal := []struct {
		accountType AccountType
		role        BalanceSheetRole
		tracking    TrackingMode
	}{
		{TypeCreditCard, RoleLiability, TrackingHoldings},
		{TypeLoan, RoleLiability, TrackingManualValue},
		{TypeBankAccount, RoleLiability, TrackingBalance},
		{TypeCryptoExchange, RoleAsset, TrackingBalance},
		{TypeProperty, RoleAsset, TrackingHoldings},
		{TypeCashOnHand, RoleAsset, TrackingHoldings},
		{TypeOther, RoleLiability, TrackingHoldings},
		{TypeOther, RoleLiability, TrackingManualValue},
		{TypeBrokerage, RoleLiability, TrackingHoldings},
	}
	for _, row := range illegal {
		if IsValidAccountCombination(row.accountType, row.role, row.tracking) {
			t.Fatalf("accepted illegal combination %+v", row)
		}
		_, _, _, err := NewAccount(AccountInput{
			HouseholdID:      HouseholdID(newID()),
			Name:             "Invalid",
			AccountType:      row.accountType,
			BalanceSheetRole: row.role,
			TrackingMode:     row.tracking,
			DefaultCurrency:  CurrencyCode("CNY"),
			Ownership:        []OwnershipShare{{MemberID: MemberID(newID()), ShareBPS: TotalOwnershipBPS}},
			InitialAmount:    "1",
		}, time.Now())
		if err == nil {
			t.Fatalf("NewAccount accepted illegal combination %+v", row)
		}
	}
}

func TestClassifySimpleAccountMatchesDesignTable(t *testing.T) {
	cases := []struct {
		accountType AccountType
		role        BalanceSheetRole
		tracking    TrackingMode
		bucket      string
	}{
		{TypeCashOnHand, RoleAsset, TrackingBalance, BucketCash},
		{TypeBankAccount, RoleAsset, TrackingBalance, BucketCash},
		{TypeDigitalWallet, RoleAsset, TrackingBalance, BucketCash},
		{TypeBrokerage, RoleAsset, TrackingManualValue, BucketUnclassifiedInvestment},
		{TypeInvestmentAccount, RoleAsset, TrackingManualValue, BucketUnclassifiedInvestment},
		{TypePension, RoleAsset, TrackingManualValue, BucketPension},
		{TypeInsurancePolicy, RoleAsset, TrackingManualValue, BucketInsurance},
		{TypeProperty, RoleAsset, TrackingManualValue, BucketProperty},
		{TypeVehicle, RoleAsset, TrackingManualValue, BucketVehicle},
		{TypeCollectible, RoleAsset, TrackingManualValue, BucketCollectible},
		{TypeReceivable, RoleAsset, TrackingBalance, BucketReceivable},
		{TypeReceivable, RoleAsset, TrackingManualValue, BucketReceivable},
		{TypeCreditCard, RoleLiability, TrackingBalance, BucketCreditCard},
		{TypeLoan, RoleLiability, TrackingBalance, BucketLoan},
		{TypeOther, RoleAsset, TrackingBalance, BucketOtherAsset},
		{TypeOther, RoleLiability, TrackingBalance, BucketOtherLiability},
	}
	for _, row := range cases {
		class, err := ClassifySimpleAccount(row.accountType, row.role, row.tracking)
		if err != nil {
			t.Fatalf("ClassifySimpleAccount(%s) error: %v", row.accountType, err)
		}
		if class.Role != row.role || class.Bucket != row.bucket {
			t.Fatalf("ClassifySimpleAccount(%s) = %+v, want role %s bucket %s", row.accountType, class, row.role, row.bucket)
		}
		if class.ClassificationBasis != ClassificationCurrentMetadataDerived {
			t.Fatalf("ClassifySimpleAccount(%s) basis = %q, want %s", row.accountType, class.ClassificationBasis, ClassificationCurrentMetadataDerived)
		}
	}
	if _, err := ClassifySimpleAccount(TypeBankAccount, RoleAsset, TrackingHoldings); err == nil {
		t.Fatal("composite simple classification succeeded")
	}
}

func TestSuggestedInclusionMatrix(t *testing.T) {
	broker := SuggestedInclusion(TypeBrokerage, TrackingHoldings)
	if !broker.IncludeInNetWorth || !broker.IncludeInPortfolio || broker.IncludeInLiquidAssets {
		t.Fatalf("brokerage defaults = %+v", broker)
	}
	cash := SuggestedInclusion(TypeBankAccount, TrackingBalance)
	if !cash.IncludeInLiquidAssets || cash.IncludeInPortfolio {
		t.Fatalf("bank balance defaults = %+v", cash)
	}
	mixed := SuggestedInclusion(TypeBankAccount, TrackingHoldings)
	if mixed.IncludeInPortfolio || mixed.IncludeInLiquidAssets {
		t.Fatalf("bank holdings defaults = %+v", mixed)
	}
}

func TestClassifyAccountComponentSplitsCompositeHoldings(t *testing.T) {
	account := Account{AccountType: TypeBrokerage, BalanceSheetRole: RoleAsset, TrackingMode: TrackingHoldings}
	cash, err := ClassifyAccountComponent(account, nil, true)
	if err != nil {
		t.Fatalf("cash component: %v", err)
	}
	if cash.Bucket != BucketCash || cash.Role != RoleAsset || cash.Incomplete || cash.ClassificationBasis != "" {
		t.Fatalf("cash component = %+v", cash)
	}
	stock := Instrument{Type: InstrumentStock}
	holding, err := ClassifyAccountComponent(account, &stock, false)
	if err != nil {
		t.Fatalf("stock holding: %v", err)
	}
	if holding.Bucket != BucketStock || holding.Role != RoleAsset || holding.ClassificationBasis != "" {
		t.Fatalf("stock holding = %+v", holding)
	}
	missing, err := ClassifyAccountComponent(account, nil, false)
	if err != nil {
		t.Fatalf("missing instrument: %v", err)
	}
	if !missing.MissingInstrument || !missing.Incomplete || missing.Bucket != "" {
		t.Fatalf("missing instrument = %+v", missing)
	}
}

func TestClassifySimpleAccountCarriesCurrentMetadataBasis(t *testing.T) {
	class, err := ClassifySimpleAccount(TypeBankAccount, RoleAsset, TrackingBalance)
	if err != nil {
		t.Fatal(err)
	}
	if class.Bucket != BucketCash || class.Role != RoleAsset || class.ClassificationBasis != ClassificationCurrentMetadataDerived {
		t.Fatalf("simple bank = %+v", class)
	}
}
