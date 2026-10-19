package domain

import (
	"testing"
	"time"
)

func TestNewProductContractNormalizesCivilDatesAndPreservesOptionalNulls(t *testing.T) {
	principal, err := ParseMoney("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	rate, err := ParseAnnualRatePercent("2.5")
	if err != nil {
		t.Fatal(err)
	}

	contract, err := NewProductContract(ProductContractInput{
		HouseholdID: NewHouseholdID(), AccountID: NewAccountID(), HoldingID: NewHoldingID(), InstrumentID: NewInstrumentID(),
		Kind: ProductTermDeposit, Name: "Synthetic deposit", Currency: "USD", Principal: principal,
		StartOn: " 2026-09-20 ", MaturityOn: datePtr(" 2026-12-20 "), InterestMode: InterestSimpleAct365,
		AnnualRate: &rate, InterestPaidThroughOn: datePtr(" 2026-09-20 "), OpenedOperationID: NewProductOperationID(),
	}, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if contract.StartOn != "2026-09-20" || contract.MaturityOn == nil || *contract.MaturityOn != "2026-12-20" || contract.InterestPaidThroughOn == nil || *contract.InterestPaidThroughOn != "2026-09-20" {
		t.Fatalf("normalized contract dates = start %q maturity %v paid-through %v", contract.StartOn, contract.MaturityOn, contract.InterestPaidThroughOn)
	}

	locked, err := NewProductContract(ProductContractInput{
		HouseholdID: NewHouseholdID(), AccountID: NewAccountID(), HoldingID: NewHoldingID(), InstrumentID: NewInstrumentID(),
		Kind: ProductLockedProduct, Name: "Synthetic lock", Currency: "USD", Principal: principal,
		StartOn: " 2026-09-20 ", InterestMode: InterestNone, OpenedOperationID: NewProductOperationID(),
	}, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if locked.MaturityOn != nil || locked.InterestPaidThroughOn != nil {
		t.Fatalf("null optional contract dates were not preserved: maturity %v paid-through %v", locked.MaturityOn, locked.InterestPaidThroughOn)
	}
}

func TestNewProductContractRejectsInvalidCivilDates(t *testing.T) {
	principal, err := ParseMoney("100", "USD")
	if err != nil {
		t.Fatal(err)
	}
	base := ProductContractInput{
		HouseholdID: NewHouseholdID(), AccountID: NewAccountID(), HoldingID: NewHoldingID(), InstrumentID: NewInstrumentID(),
		Kind: ProductLockedProduct, Name: "Synthetic lock", Currency: "USD", Principal: principal,
		StartOn: "2026-09-20", InterestMode: InterestNone, OpenedOperationID: NewProductOperationID(),
	}
	for _, test := range []struct {
		name   string
		mutate func(*ProductContractInput)
	}{
		{name: "start", mutate: func(input *ProductContractInput) { input.StartOn = " 2026-02-30 " }},
		{name: "maturity", mutate: func(input *ProductContractInput) { input.MaturityOn = datePtr(" 2026-02-30 ") }},
		{name: "interest paid through", mutate: func(input *ProductContractInput) { input.InterestPaidThroughOn = datePtr(" 2026-02-30 ") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := base
			test.mutate(&input)
			if _, err := NewProductContract(input, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)); err == nil {
				t.Fatal("invalid civil date unexpectedly accepted")
			}
		})
	}
}
