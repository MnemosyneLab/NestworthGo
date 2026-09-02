package domain

import (
	"github.com/shopspring/decimal"
	"strings"
	"testing"
	"time"
)

func TestParseMoneyRejectsBinaryStyleAndNonCanonicalInput(t *testing.T) {
	currency := CurrencyCode("CNY")
	for _, input := range []string{"", "01", "1.", "1.23456", "-1", "1e2", "1,2", "1000000000000"} {
		if _, err := ParseMoney(input, currency); err == nil {
			t.Fatalf("ParseMoney(%q) succeeded", input)
		}
	}
	value, err := ParseMoney("001.00", currency)
	if err == nil || value.Amount().String() != "0" {
		// The assignment is intentionally unreachable for a valid result; keep the
		// assertion above explicit so a future parser cannot silently normalize input.
		if err == nil {
			t.Fatalf("ParseMoney accepted a leading zero")
		}
	}
}

func TestOwnershipRequiresExactBasisPoints(t *testing.T) {
	alice := MemberID(newID())
	bob := MemberID(newID())
	if _, err := ParseOwnership([]OwnershipShare{{MemberID: alice, ShareBPS: 6000}, {MemberID: bob, ShareBPS: 3999}}); err == nil {
		t.Fatal("ownership with a missing basis point succeeded")
	}
	ownership, err := EqualOwnership([]MemberID{alice, bob, MemberID(newID())})
	if err != nil {
		t.Fatalf("EqualOwnership returned error: %v", err)
	}
	total := 0
	for _, share := range ownership.Shares() {
		total += share.ShareBPS
	}
	if total != TotalOwnershipBPS {
		t.Fatalf("ownership total = %d", total)
	}
}

func TestAccountEligibleForNetWorthRejectsArchivedAndExcluded(t *testing.T) {
	now := time.Now()
	included := Account{IncludeInNetWorth: true}
	if !AccountEligibleForNetWorth(included) {
		t.Fatal("included active account was rejected")
	}
	excluded := Account{IncludeInNetWorth: false}
	if AccountEligibleForNetWorth(excluded) {
		t.Fatal("excluded account was accepted")
	}
	archived := Account{IncludeInNetWorth: true, ArchivedAt: &now}
	if AccountEligibleForNetWorth(archived) {
		t.Fatal("archived account was accepted")
	}
}

func TestAccountAndValueUseCategorySignSemantics(t *testing.T) {
	householdID := HouseholdID(newID())
	memberID := MemberID(newID())
	account, _, initial, err := NewAccount(AccountInput{
		HouseholdID: householdID,
		Name:        "Credit Card",
		AccountType: TypeCreditCard, BalanceSheetRole: RoleLiability,
		TrackingMode:      TrackingBalance,
		DefaultCurrency:   CurrencyCode("CNY"),
		IncludeInNetWorth: true,
		Ownership:         []OwnershipShare{{MemberID: memberID, ShareBPS: TotalOwnershipBPS}},
		InitialAmount:     "100.00",
	}, time.Now())
	if err != nil {
		t.Fatalf("NewAccount returned error: %v", err)
	}
	if initial == nil {
		t.Fatal("initial value is nil")
	}
	signed, err := account.SignedAmount(*initial)
	if err != nil {
		t.Fatalf("SignedAmount returned error: %v", err)
	}
	if !signed.Equal(decimal.RequireFromString("-100")) {
		t.Fatalf("signed liability = %s", signed)
	}
	if _, err := NewAccountValue(account, *initial, time.Now(), time.Now()); err != nil {
		t.Fatalf("NewAccountValue returned error: %v", err)
	}
}

func TestDomainCanonicalizesCurrenciesAndRejectsInvalidDates(t *testing.T) {
	money, err := ParseMoney("1", CurrencyCode(" usd "))
	if err != nil || money.Currency() != CurrencyCode("USD") {
		t.Fatalf("money currency = %q, err = %v", money.Currency(), err)
	}
	household, err := NewHousehold("Test", CurrencyCode(" cny "), time.Now())
	if err != nil || household.BaseCurrency != CurrencyCode("CNY") {
		t.Fatalf("household currency = %q, err = %v", household.BaseCurrency, err)
	}
	memberID := MemberID(newID())
	_, _, _, err = NewAccount(AccountInput{HouseholdID: HouseholdID(newID()), Name: "Asset", AccountType: TypeProperty, BalanceSheetRole: RoleAsset, TrackingMode: TrackingManualValue, DefaultCurrency: CurrencyCode("CNY"), Ownership: []OwnershipShare{{MemberID: memberID, ShareBPS: TotalOwnershipBPS}}, OpenedOn: stringPtr("2026-02-30"), InitialAmount: "1"}, time.Now())
	if err == nil {
		t.Fatal("invalid lifecycle date was accepted")
	}
}

func TestOwnershipCanonicalizesCaseBeforeDuplicateCheck(t *testing.T) {
	memberID := newID()
	if _, err := ParseOwnership([]OwnershipShare{{MemberID: MemberID(memberID), ShareBPS: 5000}, {MemberID: MemberID(strings.ToUpper(memberID)), ShareBPS: 5000}}); err == nil {
		t.Fatal("case variants of one owner were accepted")
	}
}

func stringPtr(value string) *string { return &value }
