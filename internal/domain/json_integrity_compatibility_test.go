//go:build jsoncompat

package domain

import (
	"bytes"
	stdjson "encoding/json"
	"testing"
	"time"

	json "github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Exact-byte comparison for small synthetic integrity-bearing domain values.
// No output directory, snapshot generator or external ledger is involved.
func TestJSONIntegrityBytesMatchStdlib(t *testing.T) {
	stamp := time.Date(2026, 10, 20, 12, 34, 56, 123456789, time.UTC)
	price, _ := ParseUnitPrice("123.45678901")
	rate, _ := ParseFXRate("2.123456789123")
	quantity, _ := ParseQuantity("123456789012345678.12345678")
	money, _ := ParseMoney("999999999999.9999", "USD")
	holding := HoldingID("11111111-1111-4111-8111-111111111111")
	batch := ReviewHistoricalBatch{History: HistoricalSnapshotBatch{InstrumentQuoteFacts: []InstrumentQuote{{UnitPrice: price}}, FXQuoteFacts: []FXQuote{{Rate: rate}}, Portfolio: PortfolioSnapshot{CashValues: []AccountCashValue{{Amount: money}}}}, CostEvents: map[HoldingID][]CostBasisEvent{holding: {{Quantity: quantity}}}}
	suffix := &ReviewSuffixCapture{Prefix: []ReviewSuffixDay{{Key: ReviewDailyEvidenceKey{On: "2026-10-20"}, Evidence: ReviewReusableDailyEvidence{Input: ReviewDailyInput{SelectedFX: map[CurrencyCode]ReviewSelectedFX{"USD": {Quote: FXQuote{Rate: rate}}}}, CoverageMissing: []string{"<>&\u2028"}}}}}
	cert := ReviewTemporalCertificate{EvaluatedAt: stamp, NextTransitionAt: stamp.Add(time.Hour), PolicyVersion: ReviewTemporalPolicyVersion, DependencyScope: ReviewTemporalDependencyScope, ClockEpoch: "synthetic", Source: &ReviewSourceCheckpoint{Token: SourceRevisionToken{HouseholdID: "22222222-2222-4222-8222-222222222222", Epoch: "0123456789abcdef0123456789abcdef", Revision: 9007199254740993}}, ConfigRevision: 2, RuleRevisions: map[ReviewRuleID]int{"z": 2, "a": 1}, CertifiedMAC: "synthetic", SuffixNonQuoteFingerprint: "synthetic"}
	maturity := "2027-01-20"
	annual, _ := ParseAnnualRatePercent("2.5000")
	contract := ProductContract{ID: "11111111-1111-4111-8111-111111111111", HouseholdID: "22222222-2222-4222-8222-222222222222", AccountID: "33333333-3333-4333-8333-333333333333", HoldingID: holding, InstrumentID: "44444444-4444-4444-8444-444444444444", Kind: ProductTermDeposit, Name: "Synthetic <>&\u2028", Currency: "USD", Principal: money, StartOn: "2026-10-20", MaturityOn: &maturity, InterestMode: InterestSimpleAct365, AnnualRate: &annual, State: ProductStateOpen, OpenedOperationID: "55555555-5555-4555-8555-555555555555", Revision: 1, CreatedAt: stamp, UpdatedAt: stamp}
	values := map[string]any{"historical": batch, "suffix": suffix, "certificate": cert, "contract": contract, "receipt": ProductTermsReceipt{Contract: contract, RecordedAt: stamp}, "numeric": map[string]any{"revision": int64(9007199254740993), "nil": []string(nil), "empty": []string{}, "decimal": quantity.Canonical()}}
	for name, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		previous, err := stdjson.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(previous, raw) {
			t.Fatalf("integrity bytes changed for %s: old=%s current=%s", name, previous, raw)
		}
	}
	var restored ProductContract
	raw, _ := json.Marshal(contract)
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.Principal.CanonicalAmount() != money.CanonicalAmount() || restored.AnnualRate.Canonical() != annual.Canonical() {
		t.Fatal("custom contract decoder changed exact decimal values")
	}
}
