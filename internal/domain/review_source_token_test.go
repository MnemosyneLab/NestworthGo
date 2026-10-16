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
