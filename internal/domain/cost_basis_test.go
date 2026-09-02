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
	if got, want := result.Current.AverageUnitCost.Canonical(), "93.33333333"; got != want {
		t.Fatalf("average cost = %q, want %q", got, want)
	}
	if len(result.Realized) != 1 {
		t.Fatalf("realized events = %d, want 1", len(result.Realized))
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "340"; got != want {
		t.Fatalf("realized gain = %q, want %q", got, want)
	}
}

func TestReplayCostBasisPreservesEightDecimalAverageCost(t *testing.T) {
	// 2 * 1.23456789 + 3 * 1.25000005 = 6.21913593; 6.21913593 / 5 = 1.243827186
	// which rounds half-to-even at eight places to 1.24382719.
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "2"), UnitPrice: unitPricePointer(t, "1.23456789")},
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "3"), UnitPrice: unitPricePointer(t, "1.25000005")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "5"; got != want {
		t.Fatalf("quantity = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "1.24382719"; got != want {
		t.Fatalf("eight-decimal average cost = %q, want %q", got, want)
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
	if got, want := result.Current.AverageUnitCost.Canonical(), "86.36363636"; got != want {
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
	if got, want := result.Current.AverageUnitCost.Canonical(), "86.66666667"; got != want {
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

func TestReplayCostBasisBuyFeeOnlyIncreasesAcquisitionBasis(t *testing.T) {
	// 10 * 10 = 100 gross + 10 fee => acquisition 11. Sell 10 at 15, no sell fee:
	// proceeds 150 - remaining basis 110 = 40.
	fee := mustMoney(t, "10", "USD")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &fee},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "15"), Currency: CurrencyCode("USD")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "0"; got != want {
		t.Fatalf("quantity = %q, want %q", got, want)
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "40"; got != want {
		t.Fatalf("buy-fee realized gain = %q, want %q", got, want)
	}
}

func TestReplayCostBasisSellFeeOnlyReducesProceeds(t *testing.T) {
	// Buy 10 at 10, sell 10 at 15 with fee 5: 150 - 100 - 5 = 45.
	fee := mustMoney(t, "5", "USD")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10")},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "15"), Fee: &fee, Currency: CurrencyCode("USD")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "45"; got != want {
		t.Fatalf("sell-fee realized gain = %q, want %q", got, want)
	}
}

func TestReplayCostBasisBuyAndSellFeesAreNetOfFees(t *testing.T) {
	// Buy 10 at 10 + 10 fee => 11. Sell 10 at 15 - 5 fee: 150 - 110 - 5 = 35.
	buyFee := mustMoney(t, "10", "USD")
	sellFee := mustMoney(t, "5", "USD")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &buyFee},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "15"), Fee: &sellFee, Currency: CurrencyCode("USD")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "35"; got != want {
		t.Fatalf("net-of-fees realized gain = %q, want %q", got, want)
	}
}

func TestReplayCostBasisPartialSaleAndTransferKeepFeeAdjustedAverage(t *testing.T) {
	// Buy 10 at 10 + 10 fee => 11. Partial sell 4 at 20: (20-11)*4 = 36.
	// Remaining 6 at 11 transfer out; transfer in of those 6 keeps 11.
	buyFee := mustMoney(t, "10", "USD")
	source := HoldingID("source")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &buyFee},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "4"), UnitPrice: unitPricePointer(t, "20"), Currency: CurrencyCode("USD")},
		{Kind: CostBasisTransferOut, Quantity: mustQuantity(t, "6")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Realized[0].RealizedGain.CanonicalAmount(), "36"; got != want {
		t.Fatalf("partial sale realized gain = %q, want %q", got, want)
	}
	if !result.Current.Quantity.IsZero() {
		t.Fatalf("source quantity after transfer = %s, want 0", result.Current.Quantity.Canonical())
	}
	incoming := mustUnitPrice(t, "11")
	transferred, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisTransferIn, Quantity: mustQuantity(t, "6"), UnitCost: &incoming, SourceHoldingID: &source},
	})
	if err != nil {
		t.Fatalf("transfer-in ReplayCostBasis returned error: %v", err)
	}
	if got, want := transferred.Current.AverageUnitCost.Canonical(), "11"; got != want {
		t.Fatalf("transfer-in average = %q, want %q", got, want)
	}
}

func TestReplayCostBasisReversalDropsFeeAdjustedBuy(t *testing.T) {
	buyID := ActivityID("buy")
	undoID := ActivityID("undo")
	buyFee := mustMoney(t, "10", "USD")
	starting := mustUnitPrice(t, "8")
	result, err := ReplayCostBasis(&starting, []CostBasisEvent{
		{ActivityID: ActivityID("start"), Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "2")},
		{ActivityID: buyID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &buyFee},
		{ActivityID: undoID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &buyFee, ReversesActivityID: &buyID},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.Quantity.Canonical(), "2"; got != want {
		t.Fatalf("quantity after reversal = %q, want %q", got, want)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "8"; got != want {
		t.Fatalf("average after reversal = %q, want %q", got, want)
	}
}

func TestReplayCostBasisCorrectionReplacesFeeAdjustedBuy(t *testing.T) {
	originalID := ActivityID("original-buy")
	reversalID := ActivityID("undo-original")
	originalFee := mustMoney(t, "10", "USD")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{ActivityID: originalID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &originalFee},
		{ActivityID: reversalID, Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "10"), Fee: &originalFee, ReversesActivityID: &originalID},
		{ActivityID: ActivityID("replacement"), Kind: CostBasisBuy, Quantity: mustQuantity(t, "10"), UnitPrice: unitPricePointer(t, "12")},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "12"; got != want {
		t.Fatalf("corrected average = %q, want %q", got, want)
	}
}

func TestReplayCostBasisCryptoScaleFeeAdjustedBuy(t *testing.T) {
	// (0.00000001 * 1.23456789 + 0.0001) / 0.00000001 = 10000.00123456789
	// which banker's-rounds at eight places to 10000.00123457.
	fee := mustMoney(t, "0.0001", "USD")
	result, err := ReplayCostBasis(nil, []CostBasisEvent{
		{Kind: CostBasisBuy, Quantity: mustQuantity(t, "0.00000001"), UnitPrice: unitPricePointer(t, "1.23456789"), Fee: &fee},
	})
	if err != nil {
		t.Fatalf("ReplayCostBasis returned error: %v", err)
	}
	if got, want := result.Current.AverageUnitCost.Canonical(), "10000.00123457"; got != want {
		t.Fatalf("crypto-scale average = %q, want %q", got, want)
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

func mustMoney(t *testing.T, amount, currency string) Money {
	t.Helper()
	result, err := ParseMoney(amount, CurrencyCode(currency))
	if err != nil {
		t.Fatalf("parse money %q %s: %v", amount, currency, err)
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
