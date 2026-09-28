package domain

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func mustTestMoney(t *testing.T, amount, currency string) Money {
	t.Helper()
	money, err := ParseMoney(amount, CurrencyCode(currency))
	if err != nil {
		t.Fatalf("ParseMoney(%s, %s): %v", amount, currency, err)
	}
	return money
}

func moneyPtr(t *testing.T, amount, currency string) *Money {
	t.Helper()
	money := mustTestMoney(t, amount, currency)
	return &money
}

func intPtr(value int) *int { return &value }

func dayBasisPtr(value DayBasis) *DayBasis { return &value }

func earlyModePtr(value EarlyAmountMode) *EarlyAmountMode { return &value }

func datePtr(value string) *string { return &value }

func testIDs(t *testing.T) (HouseholdID, AccountID, HoldingID, InstrumentID) {
	t.Helper()
	return NewHouseholdID(), NewAccountID(), NewHoldingID(), NewInstrumentID()
}

func explicitPolicy(source LiquiditySourceRef, household HouseholdID, access AccessKind, unlock *string, settlement int, fee *Money, early EarlyKind) LiquidityPolicy {
	basis := DayBasisCalendar
	policy := LiquidityPolicy{
		ID: NewLiquidityPolicyID(), HouseholdID: household, Source: source,
		AccessKind: access, UnlockOn: unlock, SettlementDays: intPtr(settlement), DayBasis: &basis,
		NormalExitFee: fee, EarlyKind: early, Revision: 1,
		CreatedAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
	}
	if fee == nil {
		zero, _ := ParseMoney("0", *sourceCurrencyOrUSD(source))
		policy.NormalExitFee = &zero
	}
	return policy
}

func sourceCurrencyOrUSD(source LiquiditySourceRef) *CurrencyCode {
	if source.Currency != nil {
		return source.Currency
	}
	usd := CurrencyCode("USD")
	return &usd
}

func fixtureA(t *testing.T) (LiquidityQuery, []LiquiditySource, []LiquidityReservation) {
	t.Helper()
	household, bank, depositHolding, depositInstrument := testIDs(t)
	_, _, lockedHolding, lockedInstrument := testIDs(t)
	_, _, stockHolding, _ := testIDs(t)
	_, propertyAccount, _, _ := testIDs(t)
	usd := CurrencyCode("USD")
	query := LiquidityQuery{
		AsOf: time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC), Timezone: "Asia/Singapore", BaseCurrency: usd,
	}
	zeroFee := mustTestMoney(t, "0", "USD")
	cashRef := AccountCashSourceRef(bank, usd)
	cashPolicy := explicitPolicy(cashRef, household, AccessOnRequest, nil, 0, &zeroFee, EarlyNotAllowed)

	depositID := NewProductContractID()
	depositRef := HoldingSourceRef(bank, depositHolding)
	depositInterest := mustTestMoney(t, "200", "USD")
	maturity := "2026-09-25"
	depositContract := ProductContract{
		ID: depositID, HouseholdID: household, AccountID: bank, HoldingID: depositHolding, InstrumentID: depositInstrument,
		Kind: ProductTermDeposit, Name: "D1", Currency: usd, Principal: mustTestMoney(t, "20000", "USD"),
		StartOn: "2026-09-01", MaturityOn: &maturity, InterestMode: InterestManualMaturityAmount, MaturityInterest: &depositInterest,
		State: ProductStateOpen, OpenedOperationID: NewProductOperationID(), Revision: 1, CreatedAt: query.AsOf, UpdatedAt: query.AsOf,
	}
	earlyFee := mustTestMoney(t, "50", "USD")
	earlyGross := mustTestMoney(t, "20000", "USD")
	earlyDays := 0
	earlyBasis := DayBasisCalendar
	earlyMode := EarlyAmountFixedGross
	depositPolicy := LiquidityPolicy{
		ID: NewLiquidityPolicyID(), HouseholdID: household, Source: depositRef,
		AccessKind: AccessOnDate, UnlockOn: &maturity, SettlementDays: intPtr(0), DayBasis: dayBasisPtr(DayBasisCalendar),
		NormalExitFee: &zeroFee, EarlyKind: EarlyAllowed, EarlySettlementDays: &earlyDays, EarlyDayBasis: &earlyBasis,
		EarlyFee: &earlyFee, EarlyAmountMode: &earlyMode, EarlyGrossAmount: &earlyGross, Revision: 1,
		CreatedAt: query.AsOf, UpdatedAt: query.AsOf,
	}

	lockedID := NewProductContractID()
	lockedRef := HoldingSourceRef(bank, lockedHolding)
	lockedMaturity := "2026-10-05"
	lockedContract := ProductContract{
		ID: lockedID, HouseholdID: household, AccountID: bank, HoldingID: lockedHolding, InstrumentID: lockedInstrument,
		Kind: ProductLockedProduct, Name: "L1", Currency: usd, Principal: mustTestMoney(t, "5000", "USD"),
		StartOn: "2026-08-01", MaturityOn: &lockedMaturity, InterestMode: InterestNone,
		State: ProductStateOpen, OpenedOperationID: NewProductOperationID(), Revision: 1, CreatedAt: query.AsOf, UpdatedAt: query.AsOf,
	}
	lockedFee := mustTestMoney(t, "10", "USD")
	lockedPolicy := explicitPolicy(lockedRef, household, AccessOnDate, &lockedMaturity, 0, &lockedFee, EarlyNotAllowed)
	lockedPolicy.Source.Currency = &usd

	stockRef := HoldingSourceRef(bank, stockHolding)
	stockPolicy := explicitPolicy(stockRef, household, AccessOnRequest, nil, 2, &zeroFee, EarlyNotAllowed)

	propertyRef := AccountValueSourceRef(propertyAccount)
	propertyPolicy := explicitPolicy(propertyRef, household, AccessExcluded, nil, 0, &zeroFee, EarlyNotAllowed)

	depositType := InstrumentBankInvestmentProduct
	stockType := InstrumentStock
	sources := []LiquiditySource{
		{
			Ref: cashRef, AccountID: bank, DisplayName: "Bank cash", NativeCurrency: usd,
			CurrentNativeValue: moneyPtr(t, "10000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
			ExplicitPolicy: &cashPolicy,
		},
		{
			Ref: depositRef, AccountID: bank, ProductID: &depositID, DisplayName: "D1", NativeCurrency: usd,
			CurrentNativeValue: moneyPtr(t, "20000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
			InstrumentType: &depositType, Managed: true, Contract: &depositContract, ExplicitPolicy: &depositPolicy,
		},
		{
			Ref: lockedRef, AccountID: bank, ProductID: &lockedID, DisplayName: "L1", NativeCurrency: usd,
			CurrentNativeValue: moneyPtr(t, "5000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
			InstrumentType: &depositType, Managed: true, Contract: &lockedContract, ExplicitPolicy: &lockedPolicy,
		},
		{
			Ref: stockRef, AccountID: bank, DisplayName: "H1", NativeCurrency: usd,
			CurrentNativeValue: moneyPtr(t, "3000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
			InstrumentType: &stockType, ExplicitPolicy: &stockPolicy,
		},
		{
			Ref: propertyRef, AccountID: propertyAccount, DisplayName: "Property", NativeCurrency: usd,
			CurrentNativeValue: moneyPtr(t, "100000", "USD"), AccountType: TypeProperty, TrackingMode: TrackingManualValue,
			ExplicitPolicy: &propertyPolicy,
		},
	}
	reservations := []LiquidityReservation{
		{ID: NewLiquidityReservationID(), HouseholdID: household, Source: cashRef, Label: "Cash reserve", Amount: mustTestMoney(t, "2000", "USD"), Currency: usd, Revision: 1, CreatedAt: query.AsOf, UpdatedAt: query.AsOf},
		{ID: NewLiquidityReservationID(), HouseholdID: household, Source: depositRef, Label: "Deposit reserve", Amount: mustTestMoney(t, "5000", "USD"), Currency: usd, Revision: 1, CreatedAt: query.AsOf, UpdatedAt: query.AsOf},
	}
	return query, sources, reservations
}

func identityFX(base CurrencyCode) ConvertToBase {
	return func(amount Money) (*decimal.Decimal, bool, error) {
		if amount.Currency() != base {
			return nil, false, nil
		}
		value := amount.Amount()
		return &value, true, nil
	}
}

func assertBucket(t *testing.T, bucket LiquidityBucket, available, reserve, unreserved string) {
	t.Helper()
	if bucket.FullAvailable == nil || bucket.FullAvailable.CanonicalAmount() != available {
		t.Fatalf("horizon %s available = %v, want %s", bucket.HorizonOn, bucket.FullAvailable, available)
	}
	if bucket.AppliedReserveSubtotal == nil || bucket.AppliedReserveSubtotal.CanonicalAmount() != reserve {
		t.Fatalf("horizon %s reserve = %v, want %s", bucket.HorizonOn, bucket.AppliedReserveSubtotal, reserve)
	}
	if bucket.FullUnreserved == nil || bucket.FullUnreserved.CanonicalAmount() != unreserved {
		t.Fatalf("horizon %s unreserved = %v, want %s", bucket.HorizonOn, bucket.FullUnreserved, unreserved)
	}
	if bucket.Status != StatusComplete {
		t.Fatalf("horizon %s status = %s, want complete", bucket.HorizonOn, bucket.Status)
	}
}

func TestEvaluateLiquidityFixtureA(t *testing.T) {
	query, sources, reservations := fixtureA(t)
	overview, err := EvaluateLiquidity(query, sources, reservations, identityFX(query.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if overview.LocalDate != "2026-09-20" || overview.Timezone != "Asia/Singapore" {
		t.Fatalf("local date/tz = %s %s", overview.LocalDate, overview.Timezone)
	}
	if len(overview.Buckets) < 3 {
		t.Fatalf("buckets = %d, want at least 3", len(overview.Buckets))
	}
	assertBucket(t, overview.Buckets[0], "10000", "2000", "8000")
	assertBucket(t, overview.Buckets[1], "33200", "7000", "26200")
	assertBucket(t, overview.Buckets[2], "38190", "7000", "31190")
	query.IncludeEarlyWithdrawal = true
	early, err := EvaluateLiquidity(query, sources, reservations, identityFX(query.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	assertBucket(t, early.Buckets[0], "29950", "7000", "22950")
	assertBucket(t, early.Buckets[1], "33200", "7000", "26200")
	assertBucket(t, early.Buckets[2], "38190", "7000", "31190")
	if err := assertCumulativeMonotonic(overview.Buckets); err != nil {
		t.Fatal(err)
	}
	if err := assertCumulativeMonotonic(early.Buckets); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateLiquidityMissingFXKeepsNativeSubtotals(t *testing.T) {
	query, sources, reservations := fixtureA(t)
	household := sources[0].Ref.AccountID
	eur := CurrencyCode("EUR")
	eurAccount := NewAccountID()
	eurRef := AccountCashSourceRef(eurAccount, eur)
	zero := mustTestMoney(t, "0", "EUR")
	policy := explicitPolicy(eurRef, NewHouseholdID(), AccessOnRequest, nil, 0, &zero, EarlyNotAllowed)
	_ = household
	sources = append(sources, LiquiditySource{
		Ref: eurRef, AccountID: eurAccount, DisplayName: "EUR cash", NativeCurrency: eur,
		CurrentNativeValue: moneyPtr(t, "100", "EUR"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
		ExplicitPolicy: &policy,
	})
	overview, err := EvaluateLiquidity(query, sources, reservations, identityFX(query.BaseCurrency))
	if err != nil {
		t.Fatal(err)
	}
	if overview.Buckets[0].FullAvailable != nil {
		t.Fatalf("USD full total should be null when EUR FX is missing, got %v", overview.Buckets[0].FullAvailable)
	}
	if overview.Buckets[0].Status != StatusPartial {
		t.Fatalf("status = %s, want partial", overview.Buckets[0].Status)
	}
	if overview.Buckets[0].KnownAvailableSubtotal == nil || overview.Buckets[0].KnownAvailableSubtotal.CanonicalAmount() != "10000" {
		t.Fatalf("known USD subtotal = %v, want 10000", overview.Buckets[0].KnownAvailableSubtotal)
	}
	foundEUR := false
	for _, group := range overview.Buckets[0].NativeCurrencyGroups {
		if group.Currency == "EUR" && group.KnownAvailableSubtotal != nil && group.KnownAvailableSubtotal.CanonicalAmount() == "100" {
			foundEUR = true
		}
	}
	if !foundEUR {
		t.Fatal("native EUR 100 was not preserved")
	}
}

func TestLiquidityHorizonsInclusiveAndCustom(t *testing.T) {
	horizons, err := LiquidityHorizons("2026-09-20", nil)
	if err != nil {
		t.Fatal(err)
	}
	if horizons[0] != "2026-09-20" || horizons[1] != "2026-09-27" || horizons[2] != "2026-10-20" {
		t.Fatalf("horizons = %v", horizons)
	}
	custom := "2026-09-19"
	if _, err := LiquidityHorizons("2026-09-20", &custom); err == nil {
		t.Fatal("expected custom date before today to fail")
	}
	custom = "2037-09-21"
	if _, err := LiquidityHorizons("2026-09-20", &custom); err == nil {
		t.Fatal("expected custom date more than 10 years ahead to fail")
	}
	custom = "2026-09-22"
	horizons, err = LiquidityHorizons("2026-09-20", &custom)
	if err != nil {
		t.Fatal(err)
	}
	if horizons[3] != "2026-09-22" {
		t.Fatalf("custom horizon missing: %v", horizons)
	}
}

func TestWeekdaySettlementAndDSTCalendar(t *testing.T) {
	got, err := SettlementReceiptOn("2026-09-18", 2, DayBasisWeekdays)
	if err != nil {
		t.Fatal(err)
	}
	if got != "2026-09-22" {
		t.Fatalf("Friday+2 weekdays = %s, want 2026-09-22", got)
	}
	same, err := SettlementReceiptOn("2026-09-18", 0, DayBasisWeekdays)
	if err != nil {
		t.Fatal(err)
	}
	if same != "2026-09-18" {
		t.Fatalf("+0 = %s, want Friday", same)
	}
	asOf := time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	local, zone, err := LocalCivilDate(asOf, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if local != "2026-03-08" || zone != "America/New_York" {
		t.Fatalf("local = %s %s", local, zone)
	}
	plus7, err := AddCalendarDays(local, 7)
	if err != nil {
		t.Fatal(err)
	}
	if plus7 != "2026-03-15" {
		t.Fatalf("+7 calendar days across DST = %s, want 2026-03-15", plus7)
	}
	leap, err := AddCalendarDays("2024-02-28", 1)
	if err != nil {
		t.Fatal(err)
	}
	if leap != "2024-02-29" {
		t.Fatalf("leap day = %s", leap)
	}
	month, err := AddCalendarDays("2026-01-31", 1)
	if err != nil {
		t.Fatal(err)
	}
	if month != "2026-02-01" {
		t.Fatalf("month boundary = %s", month)
	}
}

func TestSimpleInterestFixtureD(t *testing.T) {
	rate, err := ParseAnnualRatePercent("10")
	if err != nil {
		t.Fatal(err)
	}
	act365, err := SimpleInterest(mustTestMoney(t, "36500", "USD"), rate, 10, 365)
	if err != nil {
		t.Fatal(err)
	}
	if act365.CanonicalAmount() != "100" {
		t.Fatalf("ACT/365 interest = %s, want 100", act365.CanonicalAmount())
	}
	act360, err := SimpleInterest(mustTestMoney(t, "36000", "USD"), rate, 10, 360)
	if err != nil {
		t.Fatal(err)
	}
	if act360.CanonicalAmount() != "100" {
		t.Fatalf("ACT/360 interest = %s, want 100", act360.CanonicalAmount())
	}
	contract := ProductContract{
		Kind: ProductTermDeposit, Currency: "USD", Principal: mustTestMoney(t, "36500", "USD"),
		StartOn: "2026-01-01", MaturityOn: datePtr("2026-01-11"), InterestMode: InterestSimpleAct365,
		AnnualRate: &rate, InterestPaidThroughOn: datePtr("2026-01-06"), State: ProductStateOpen,
		HouseholdID: NewHouseholdID(), AccountID: NewAccountID(), HoldingID: NewHoldingID(), InstrumentID: NewInstrumentID(),
		ID: NewProductContractID(), Name: "D", OpenedOperationID: NewProductOperationID(), Revision: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	remaining, err := contract.EstimatedUnpaidMaturityInterest()
	if err != nil {
		t.Fatal(err)
	}
	if remaining.CanonicalAmount() != "50" {
		t.Fatalf("remaining interest = %s, want 50", remaining.CanonicalAmount())
	}
	if _, err := ParseAnnualRateRatio("1.1"); err == nil {
		t.Fatal("expected ratio > 1 to fail")
	}
	if _, err := NewAnnualRate(decimal.RequireFromString("0.123456789")); err == nil {
		t.Fatal("expected more than 8 fractional digits to fail")
	}
	fromPercent, err := ParseAnnualRatePercent("2.5")
	if err != nil {
		t.Fatal(err)
	}
	if fromPercent.Canonical() != "0.025" {
		t.Fatalf("2.5%% = %s, want 0.025", fromPercent.Canonical())
	}
}

func TestKnownZeroVersusMissingAmount(t *testing.T) {
	household, account, holding, _ := testIDs(t)
	usd := CurrencyCode("USD")
	ref := HoldingSourceRef(account, holding)
	unlock := "2026-09-20"
	fee := mustTestMoney(t, "50", "USD")
	policy := explicitPolicy(ref, household, AccessOnDate, &unlock, 0, &fee, EarlyNotAllowed)
	policy.Source.Currency = &usd
	stock := InstrumentStock
	source := LiquiditySource{
		Ref: ref, AccountID: account, DisplayName: "Fee exceeds", NativeCurrency: usd,
		CurrentNativeValue: moneyPtr(t, "40", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
		InstrumentType: &stock, ExplicitPolicy: &policy,
	}
	query := LiquidityQuery{AsOf: time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC), Timezone: "UTC", BaseCurrency: usd}
	overview, err := EvaluateLiquidity(query, []LiquiditySource{source}, nil, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if overview.Buckets[0].FullAvailable == nil || overview.Buckets[0].FullAvailable.CanonicalAmount() != "0" {
		t.Fatalf("fee > gross should be known zero, got %v", overview.Buckets[0].FullAvailable)
	}
	if overview.Sources[0].NormalRoute == nil || len(overview.Sources[0].NormalRoute.Assumptions) == 0 {
		t.Fatal("expected fee-exceeds-gross assumption")
	}

	missingFee := policy
	missingFee.NormalExitFee = nil
	source.ExplicitPolicy = &missingFee
	source.CurrentNativeValue = moneyPtr(t, "40", "USD")
	partial, err := EvaluateLiquidity(query, []LiquiditySource{source}, nil, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if partial.Buckets[0].FullAvailable != nil {
		t.Fatalf("unknown fee must not become a full total, got %v", partial.Buckets[0].FullAvailable)
	}
	if partial.Sources[0].NormalRoute == nil || partial.Sources[0].NormalRoute.NetNative != nil {
		t.Fatal("unknown fee must leave net amount missing")
	}

	unknownPolicy := policy
	unknownPolicy.AccessKind = AccessUnknown
	source.ExplicitPolicy = &unknownPolicy
	unknown, err := EvaluateLiquidity(query, []LiquiditySource{source}, nil, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if unknown.Buckets[0].Status != StatusUnavailable && unknown.Buckets[0].FullAvailable != nil {
		t.Fatalf("unknown access must not yield a known full total: %+v", unknown.Buckets[0])
	}
}

func TestReservationsCapAndUnavailableSource(t *testing.T) {
	household, account, _, _ := testIDs(t)
	usd := CurrencyCode("USD")
	ref := AccountCashSourceRef(account, usd)
	zero := mustTestMoney(t, "0", "USD")
	cap := mustTestMoney(t, "5000", "USD")
	policy := explicitPolicy(ref, household, AccessOnRequest, nil, 0, &zero, EarlyNotAllowed)
	policy.AccessibleAmountCap = &cap
	source := LiquiditySource{
		Ref: ref, AccountID: account, DisplayName: "Capped cash", NativeCurrency: usd,
		CurrentNativeValue: moneyPtr(t, "10000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
		ExplicitPolicy: &policy,
	}
	reservation := LiquidityReservation{
		ID: NewLiquidityReservationID(), HouseholdID: household, Source: ref, Label: "Reserve",
		Amount: mustTestMoney(t, "1000", "USD"), Currency: usd, Revision: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	query := LiquidityQuery{AsOf: time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC), Timezone: "UTC", BaseCurrency: usd}
	overview, err := EvaluateLiquidity(query, []LiquiditySource{source}, []LiquidityReservation{reservation}, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	assertBucket(t, overview.Buckets[0], "5000", "1000", "4000")

	over := reservation
	over.Amount = mustTestMoney(t, "8000", "USD")
	overReserved, err := EvaluateLiquidity(query, []LiquiditySource{source}, []LiquidityReservation{over}, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if overReserved.Sources[0].BucketResults[0].ReserveShortfallNative == nil || overReserved.Sources[0].BucketResults[0].ReserveShortfallNative.CanonicalAmount() != "3000" {
		t.Fatalf("shortfall = %v, want 3000", overReserved.Sources[0].BucketResults[0].ReserveShortfallNative)
	}

	futureUnlock := "2026-10-01"
	lockedPolicy := explicitPolicy(ref, household, AccessOnDate, &futureUnlock, 0, &zero, EarlyNotAllowed)
	locked := source
	locked.ExplicitPolicy = &lockedPolicy
	locked.CurrentNativeValue = moneyPtr(t, "10000", "USD")
	lockedOverview, err := EvaluateLiquidity(query, []LiquiditySource{locked}, []LiquidityReservation{reservation}, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if lockedOverview.Buckets[0].FullAvailable == nil || lockedOverview.Buckets[0].FullAvailable.CanonicalAmount() != "0" {
		t.Fatalf("unavailable-before-horizon source must not reduce unrelated cash; today available = %v", lockedOverview.Buckets[0].FullAvailable)
	}
	if lockedOverview.Buckets[2].FullAvailable == nil || lockedOverview.Buckets[2].FullAvailable.CanonicalAmount() != "10000" {
		t.Fatalf("+30 available = %v, want 10000", lockedOverview.Buckets[2].FullAvailable)
	}
}

func TestAssumedBankCashTreatsMissingFeeAsKnownZero(t *testing.T) {
	_, account, _, _ := testIDs(t)
	usd := CurrencyCode("USD")
	source := LiquiditySource{
		Ref: AccountCashSourceRef(account, usd), AccountID: account, DisplayName: "Bank cash", NativeCurrency: usd,
		CurrentNativeValue: moneyPtr(t, "10000", "USD"), AccountType: TypeBankAccount, TrackingMode: TrackingHoldings,
	}
	query := LiquidityQuery{AsOf: time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC), Timezone: "UTC", BaseCurrency: usd}
	overview, err := EvaluateLiquidity(query, []LiquiditySource{source}, nil, identityFX(usd))
	if err != nil {
		t.Fatal(err)
	}
	if overview.Sources[0].PolicyOrigin != PolicyOriginAssumed {
		t.Fatalf("origin = %s", overview.Sources[0].PolicyOrigin)
	}
	if overview.Buckets[0].FullAvailable == nil || overview.Buckets[0].FullAvailable.CanonicalAmount() != "10000" {
		t.Fatalf("assumed bank cash today = %v, want 10000", overview.Buckets[0].FullAvailable)
	}
}

func TestProductContractRejectsInconsistentDates(t *testing.T) {
	rate, err := ParseAnnualRatePercent("2.5")
	if err != nil {
		t.Fatal(err)
	}
	input := ProductContractInput{
		HouseholdID: NewHouseholdID(), AccountID: NewAccountID(), HoldingID: NewHoldingID(), InstrumentID: NewInstrumentID(),
		Kind: ProductTermDeposit, Name: "Bad", Currency: "USD", Principal: mustTestMoney(t, "1000", "USD"),
		StartOn: "2026-09-20", MaturityOn: datePtr("2026-09-20"), InterestMode: InterestSimpleAct365, AnnualRate: &rate,
		InterestPaidThroughOn: datePtr("2026-09-20"), State: ProductStateOpen, OpenedOperationID: NewProductOperationID(), Revision: 1,
	}
	if _, err := NewProductContract(input, time.Now()); err == nil {
		t.Fatal("expected maturity_on <= start_on to fail")
	}
}
