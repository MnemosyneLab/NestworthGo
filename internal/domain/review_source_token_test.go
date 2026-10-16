package domain

import "testing"

func TestReviewHistoricalFingerprintChangesOnCostCorrectionAndDeletion(t *testing.T) {
	a, b := NewHoldingID(), NewHoldingID()
	batch := ReviewHistoricalBatch{CostEvents: map[HoldingID][]CostBasisEvent{a: {}, b: {}}}
	first, err := ReviewHistoricalFingerprint(batch)
	if err != nil {
		t.Fatal(err)
	}
	reversed := ReviewHistoricalBatch{CostEvents: map[HoldingID][]CostBasisEvent{b: {}, a: {}}}
	same, err := ReviewHistoricalFingerprint(reversed)
	if err != nil || same != first {
		t.Fatalf("map insertion order changed source token: %s %s %v", first, same, err)
	}
	delete(batch.CostEvents, b)
	deleted, err := ReviewHistoricalFingerprint(batch)
	if err != nil || deleted == first {
		t.Fatal("deletion was invisible to source token")
	}
	q, err := ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	batch.CostEvents[a] = []CostBasisEvent{{Kind: CostBasisBuy, Quantity: q}}
	corrected, err := ReviewHistoricalFingerprint(batch)
	if err != nil || corrected == deleted {
		t.Fatal("cost change was invisible to source token")
	}
}

func TestReviewHistoricalFingerprintChangesWhenExistingQuoteValuesAreCorrected(t *testing.T) {
	firstPrice, err := ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	correctedPrice, err := ParseUnitPrice("101")
	if err != nil {
		t.Fatal(err)
	}
	firstRate, err := ParseFxRate("2")
	if err != nil {
		t.Fatal(err)
	}
	correctedRate, err := ParseFxRate("2.1")
	if err != nil {
		t.Fatal(err)
	}
	batch := ReviewHistoricalBatch{History: HistoricalSnapshotBatch{
		InstrumentQuoteFacts: []InstrumentQuote{{UnitPrice: firstPrice}},
		FXQuoteFacts:         []FXQuote{{Rate: firstRate}},
	}}
	initial, err := ReviewHistoricalFingerprint(batch)
	if err != nil {
		t.Fatal(err)
	}
	batch.History.InstrumentQuoteFacts[0].UnitPrice = correctedPrice
	correctedPriceFingerprint, err := ReviewHistoricalFingerprint(batch)
	if err != nil || correctedPriceFingerprint == initial {
		t.Fatalf("changing an existing quote price was invisible to source token: %s %s %v", initial, correctedPriceFingerprint, err)
	}
	batch.History.InstrumentQuoteFacts[0].UnitPrice = firstPrice
	batch.History.FXQuoteFacts[0].Rate = correctedRate
	correctedRateFingerprint, err := ReviewHistoricalFingerprint(batch)
	if err != nil || correctedRateFingerprint == initial {
		t.Fatalf("changing an existing FX rate was invisible to source token: %s %s %v", initial, correctedRateFingerprint, err)
	}
}

func TestReviewHistoricalFingerprintChangesWhenExistingExactPositionAndAmountValuesAreCorrected(t *testing.T) {
	firstQuantity, err := ParseQuantity("2")
	if err != nil {
		t.Fatal(err)
	}
	correctedQuantity, err := ParseQuantity("2.5")
	if err != nil {
		t.Fatal(err)
	}
	currency, err := ParseCurrency("CNY")
	if err != nil {
		t.Fatal(err)
	}
	firstAmount, err := ParseMoney("100", currency)
	if err != nil {
		t.Fatal(err)
	}
	correctedAmount, err := ParseMoney("101", currency)
	if err != nil {
		t.Fatal(err)
	}
	holdingID := NewHoldingID()
	batch := ReviewHistoricalBatch{
		History:    HistoricalSnapshotBatch{Portfolio: PortfolioSnapshot{CashValues: []AccountCashValue{{Amount: firstAmount}}}},
		CostEvents: map[HoldingID][]CostBasisEvent{holdingID: {{Quantity: firstQuantity}}},
	}
	initial, err := ReviewHistoricalFingerprint(batch)
	if err != nil {
		t.Fatal(err)
	}
	event := batch.CostEvents[holdingID][0]
	event.Quantity = correctedQuantity
	batch.CostEvents[holdingID][0] = event
	quantityFingerprint, err := ReviewHistoricalFingerprint(batch)
	if err != nil || quantityFingerprint == initial {
		t.Fatalf("changing an existing cost-event quantity was invisible to source token: %s %s %v", initial, quantityFingerprint, err)
	}
	batch.History.Portfolio.CashValues[0].Amount = correctedAmount
	amountFingerprint, err := ReviewHistoricalFingerprint(batch)
	if err != nil || amountFingerprint == quantityFingerprint {
		t.Fatalf("changing an existing account amount was invisible to source token: %s %s %v", quantityFingerprint, amountFingerprint, err)
	}
}
