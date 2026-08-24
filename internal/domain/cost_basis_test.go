package domain

import (
	"testing"
	"time"
)

func TestReplayCostBasisGoldenAverageCostAndRealizedGain(t *testing.T) {
	startingCost := mustUnitPrice(t, "80")
	quantityTen := mustQuantity(t, "10")
	quantityFive := mustQuantity(t, "5")
	quantitySix := mustQuantity(t, "6")
	buyPrice := mustUnitPrice(t, "120")
	sellPrice := mustUnitPrice(t, "150")
	result, err := ReplayCostBasis(&startingCost, []CostBasisEvent{
		{ActivityID: ActivityID("starting"), Kind: CostBasisStartingPoint, Quantity: quantityTen},
		{ActivityID: ActivityID("buy"), Kind: CostBasisBuy, Quantity: quantityFive, UnitPrice: &buyPrice},
		{ActivityID: ActivityID("sell"), Kind: CostBasisSell, Quantity: quantitySix, UnitPrice: &sellPrice, Currency: CurrencyCode("USD")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "9"; got != want {
		t.Fatalf("remaining quantity = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "93.33"; got != want {
		t.Fatalf("average cost = %q, want %q", got, want)
	}
	if len(result.Realized) != 1 {
		t.Fatalf("realized events = %d, want 1", len(result.Realized))
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "340.02"; got != want {
		t.Fatalf("realized gain = %q, want %q", got, want)
	}
}

func TestReplayCostBasisHandlesAllQuantityEventKinds(t *testing.T) {
	startingCost := mustUnitPrice(t, "80")
	result, err := ReplayCostBasis(&startingCost, []CostBasisEvent{
		{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "10")},
		{Kind: CostBasisTransferOut, Quantity: mustQuantity(t, "2")},
		{Kind: CostBasisTransferIn, Quantity: mustQuantity(t, "2"), UnitCost: unitPricePointer(t, "100")},
		{Kind: CostBasisAdjustmentIn, Quantity: mustQuantity(t, "1"), UnitCost: unitPricePointer(t, "110")},
		{Kind: CostBasisAdjustmentOut, Quantity: mustQuantity(t, "1")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "10"; got != want {
		t.Fatalf("quantity = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "86.36"; got != want {
		t.Fatalf("average cost = %q, want %q", got, want)
	}
}

func TestReplayCostBasisExcludesReversedPairsAndKeepsFixReplacement(t *testing.T) {
	startingCost := mustUnitPrice(t, "80")
	originalID := ActivityID("original-buy")
	reversalID := ActivityID("undo-original")
	replacementPrice := mustUnitPrice(t, "100")
	result, err := ReplayCostBasis(&startingCost, []CostBasisEvent{
		{ActivityID: ActivityID("starting"), Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "10")},
		{ActivityID: originalID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "5"), UnitPrice: unitPricePointer(t, "120")},
		{ActivityID: reversalID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "5"), UnitPrice: unitPricePointer(t, "120"), ReversesActivityID: &originalID},
		{ActivityID: ActivityID("replacement"), Kind: CostBasisBuy, Quantity: mustQuantity(t, "5"), UnitPrice: &replacementPrice},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "15"; got != want {
		t.Fatalf("quantity = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "86.67"; got != want {
		t.Fatalf("replacement average cost = %q, want %q", got, want)
	}
}

func TestReplayCostBasisResetsAfterQuantityReachesZero(t *testing.T) {
	startingCost := mustUnitPrice(t, "80")
	result, err := ReplayCostBasis(&startingCost, []CostBasisEvent{
		{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "2")},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "2"), UnitPrice: unitPricePointer(t, "100"), Currency: CurrencyCode("USD")},
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "1"), UnitPrice: unitPricePointer(t, "50")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "1"; got != want {
		t.Fatalf("quantity = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "50"; got != want {
		t.Fatalf("reset average cost = %q, want %q", got, want)
	}
}

func TestReplayCostBasisEmitsSignedLoss(t *testing.T) {
	startingCost := mustUnitPrice(t, "80")
	result, err := ReplayCostBasis(&startingCost, []CostBasisEvent{
		{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "1")},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "1"), UnitPrice: unitPricePointer(t, "70"), Currency: CurrencyCode("USD")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "-10"; got != want {
		t.Fatalf("realized loss = %q, want %q", got, want)
	}
}

func TestPositionAdjustmentCostRequirementAndHistoryComponentValidation(t *testing.T) {
	quantity, _ := ParseQuantity("1")
	household := HouseholdID("household")
	holding := HoldingID("holding")
	state := ChangeState{HouseholdID: household, Now: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), Timezone: "UTC", Holdings: map[HoldingID]ChangeHoldingState{holding: {ID: holding, InstrumentID: InstrumentID("instrument"), InstrumentName: "Fixture ETF", Currency: CurrencyCode("USD"), Current: mustQuantity(t, "0")}}}
	if _, err := PreviewChange(state, PositionAdjustmentInput{HouseholdID: household, HoldingID: holding, Quantity: quantity, Added: true}); err == nil || err.(*Error).Code != ErrCostBasisRequired {
		t.Fatalf("missing added UnitCost error = %v, want cost_basis_required", err)
	}
	if preview, err := PreviewChange(state, PositionAdjustmentInput{HouseholdID: household, HoldingID: holding, Quantity: quantity, Added: true, UnitCost: unitPricePointer(t, "80")}); err != nil || preview.DerivedUnitPrice == nil || preview.DerivedUnitPrice.Canonical() != "80" {
		t.Fatalf("added UnitCost rejected: %v", err)
	}
	state.Holdings[holding] = ChangeHoldingState{ID: holding, InstrumentID: InstrumentID("instrument"), InstrumentName: "Fixture ETF", Currency: CurrencyCode("USD"), Current: mustQuantity(t, "2"), CostBasisAvailable: true}
	if _, err := PreviewChange(state, PositionAdjustmentInput{HouseholdID: household, HoldingID: holding, Quantity: quantity, Added: false}); err != nil {
		t.Fatalf("decrease unexpectedly requires UnitCost: %v", err)
	}
	if preview, err := PreviewChange(state, PositionAdjustmentInput{HouseholdID: household, HoldingID: holding, Quantity: quantity, Added: false, UnitCost: unitPricePointer(t, "99")}); err != nil || preview.DerivedUnitPrice != nil {
		t.Fatalf("decrease unexpectedly changed UnitCost: preview=%+v err=%v", preview, err)
	}

	component := HistoryOriginComponent{ID: HistoryOriginComponentID("component"), OriginID: HistoryOriginID("origin"), Kind: HistoryOriginHoldingQuantity, AccountID: accountIDPointer("account"), HoldingID: &holding, InstrumentID: instrumentIDPointer("instrument"), Quantity: &quantity, CreatedAt: time.Now()}
	if err := component.Validate(); err == nil || err.(*Error).Code != ErrCostBasisRequired {
		t.Fatalf("positive component without UnitCost error = %v, want cost_basis_required", err)
	}
	zero := mustQuantity(t, "0")
	component.Quantity = &zero
	if err := component.Validate(); err != nil {
		t.Fatalf("zero component without UnitCost rejected: %v", err)
	}
}

func mustQuantity(t *testing.T, value string) Quantity {
	t.Helper()
	result, err := ParseQuantity(value)
	if err != nil {
		t.Fatalf("parse quantity %q: %v", value, err)
	}
	return result
}

func mustUnitPrice(t *testing.T, value string) UnitPrice {
	t.Helper()
	result, err := ParseUnitPrice(value)
	if err != nil {
		t.Fatalf("parse unit price %q: %v", value, err)
	}
	return result
}

func unitPricePointer(t *testing.T, value string) *UnitPrice {
	t.Helper()
	result := mustUnitPrice(t, value)
	return &result
}

func accountIDPointer(value string) *AccountID {
	result := AccountID(value)
	return &result
}

func instrumentIDPointer(value string) *InstrumentID {
	result := InstrumentID(value)
	return &result
}
