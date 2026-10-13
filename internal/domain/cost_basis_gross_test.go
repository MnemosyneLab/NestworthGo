package domain

import "testing"

func TestReplayCostBasisUsesGrossForRealizedProceeds(t *testing.T) {
	for _, tc := range []struct{ name, quantity, price, gross, fee, cost, want string }{
		{"large full", "100000000", "1", "100000000.49", "0", "1", "0.49"},
		{"large partial fee", "50000000", "1", "50000000.24", "0.1", "1", "0.14"},
		{"loss", "50000000", "1", "49999999.76", "0.1", "1", "-0.34"},
		{"zero proceeds", "2", "0", "0", "0.1", "1", "-2.1"},
		{"final money rounding", "0.5", "2", "1", "0", "1.9999", "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cost := mustUnitPrice(t, tc.cost)
			gross := mustMoney(t, tc.gross, "USD")
			fee := mustMoney(t, tc.fee, "USD")
			result, err := ReplayCostBasis(&cost, []CostBasisEvent{
				{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "100000000")},
				{Kind: CostBasisSell, Quantity: mustQuantity(t, tc.quantity), UnitPrice: unitPricePointer(t, tc.price), Gross: &gross, Fee: &fee, Currency: "USD"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Realized[0].RealizedGain.CanonicalAmount(); got != tc.want {
				t.Fatalf("realized=%s want %s", got, tc.want)
			}
		})
	}
}

func TestReplayCostBasisBuyAddsFeeBeforeUnitCostRounding(t *testing.T) {
	for _, tc := range []struct{ name, quantity, price, gross, fee, want string }{
		{"gross below half and fee crosses half", "100000000", "1", "100000000.49", "0.02", "1.00000001"},
		{"eight decimal cost", "3", "1.00003333", "3.0001", "0.0001", "1.00006667"},
		{"above half without intermediate rounding", "2000000000000", "0", "10000", "0.0001", "0.00000001"},
		{"even half rounds down", "2000000000000", "0", "10000", "0", "0"},
		{"odd half rounds up", "2000000000000", "0.00000002", "30000", "0", "0.00000002"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gross := mustMoney(t, tc.gross, "USD")
			fee := mustMoney(t, tc.fee, "USD")
			result, err := ReplayCostBasis(nil, []CostBasisEvent{{Kind: CostBasisBuy, Quantity: mustQuantity(t, tc.quantity), UnitPrice: unitPricePointer(t, tc.price), Gross: &gross, Fee: &fee, Currency: "USD"}})
			if err != nil {
				t.Fatal(err)
			}
			if got := result.Current.AverageUnitCost.Canonical(); got != tc.want {
				t.Fatalf("basis=%s want %s", got, tc.want)
			}
		})
	}
}

func TestReplayCostBasisRejectsMismatchedGrossAndFeeCurrency(t *testing.T) {
	cost := mustUnitPrice(t, "1")
	for _, kind := range []CostBasisEventKind{CostBasisBuy, CostBasisSell} {
		for _, grossCurrency := range []string{"USD", "CNY"} {
			gross := mustMoney(t, "10", grossCurrency)
			fee := mustMoney(t, "1", "CNY")
			_, err := ReplayCostBasis(&cost, []CostBasisEvent{
				{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "10")},
				{Kind: kind, Quantity: mustQuantity(t, "1"), UnitPrice: unitPricePointer(t, "10"), Gross: &gross, Fee: &fee, Currency: "USD"},
			})
			if err == nil {
				t.Fatalf("kind=%s accepted mismatched trade currency", kind)
			}
		}
	}
}

func TestReplayCostBasisLegacyUnitPriceOnlySellKeepsLargeZeroGain(t *testing.T) {
	// This pure replay input was supported before persisted-gross replay. It
	// cannot be persisted as a trade because its gross exceeds Money's cap.
	cost := mustUnitPrice(t, "1000")
	result, err := ReplayCostBasis(&cost, []CostBasisEvent{
		{Kind: CostBasisStartingPoint, Quantity: mustQuantity(t, "1000000000")},
		{Kind: CostBasisSell, Quantity: mustQuantity(t, "1000000000"), UnitPrice: unitPricePointer(t, "1000"), Currency: "CNY"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Current.Quantity.IsZero() || result.Realized[0].RealizedGain.CanonicalAmount() != "0" {
		t.Fatalf("legacy replay=%+v", result)
	}
	if _, err := ParseMoney("1000000000000", "CNY"); err == nil {
		t.Fatal("persisted gross cap was weakened")
	}
}
