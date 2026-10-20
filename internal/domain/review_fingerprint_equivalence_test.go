package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"reflect"
	"sort"
	"testing"
)

func TestReviewScalarWalkerMatchesImmutableLegacy(t *testing.T) {
	price, _ := ParseUnitPrice("123.4567")
	rate, _ := ParseFXRate("2.123456789123")
	quantity, _ := ParseQuantity("2.5")
	currency, _ := ParseCurrency("CNY")
	money, _ := ParseMoney("123.45", currency)
	holding := NewHoldingID()
	batch := ReviewHistoricalBatch{History: HistoricalSnapshotBatch{InstrumentQuoteFacts: []InstrumentQuote{{UnitPrice: price}}, FXQuoteFacts: []FXQuote{{Rate: rate}}, Portfolio: PortfolioSnapshot{CashValues: []AccountCashValue{{Amount: money}}}}, CostEvents: map[HoldingID][]CostBasisEvent{holding: {{Quantity: quantity}}}}
	suffix := &ReviewSuffixCapture{Prefix: []ReviewSuffixDay{{Evidence: ReviewReusableDailyEvidence{Input: ReviewDailyInput{SelectedFX: map[CurrencyCode]ReviewSelectedFX{currency: {Quote: FXQuote{Rate: rate}}}}}}}}
	var missing *UnitPrice
	values := []any{nil, missing, price, &price, quantity, money, rate, batch, &batch, suffix, []any{price, nil, &rate}, [2]UnitPrice{price, price}, map[string]any{"z": quantity, "a": money, "nil": missing}, struct {
		Nested any
		hidden UnitPrice
	}{batch, price}}
	for i, value := range values {
		before, after := sha256.New(), sha256.New()
		if err := writeLegacyReviewFingerprintScalars(before, reflect.ValueOf(value)); err != nil {
			t.Fatal(err)
		}
		if err := writeReviewFingerprintScalars(after, reflect.ValueOf(value)); err != nil {
			t.Fatal(err)
		}
		if string(before.Sum(nil)) != string(after.Sum(nil)) {
			t.Fatalf("scalar byte stream changed for case %d (%T)", i, value)
		}
	}
}

// Immutable pre-optimization walker from 14294bc. It independently preserves
// interface/ptr/nil traversal, scalar precedence, map order and private-field
// handling; equality compares the actual emitted scalar bytes via SHA-256.
func writeLegacyReviewFingerprintScalars(digest hash.Hash, value reflect.Value) error {
	if !value.IsValid() {
		return nil
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil
		}
		value = value.Elem()
	}
	if value.CanInterface() {
		if amount, ok := value.Interface().(reviewFingerprintAmount); ok {
			writeReviewFingerprintScalar(digest, "amount:"+amount.Currency().String(), amount.CanonicalAmount())
			return nil
		}
		if canonical, ok := value.Interface().(reviewFingerprintCanonical); ok {
			writeReviewFingerprintScalar(digest, value.Type().String(), canonical.Canonical())
			return nil
		}
	}
	switch value.Kind() {
	case reflect.Struct:
		for index := 0; index < value.NumField(); index++ {
			field := value.Type().Field(index)
			if field.PkgPath != "" { // private fields are represented by scalar methods above
				continue
			}
			if err := writeLegacyReviewFingerprintScalars(digest, value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := writeLegacyReviewFingerprintScalars(digest, value.Index(index)); err != nil {
				return err
			}
		}
	case reflect.Map:
		keys := value.MapKeys()
		sort.Slice(keys, func(i, j int) bool {
			left, _ := json.Marshal(keys[i].Interface())
			right, _ := json.Marshal(keys[j].Interface())
			return string(left) < string(right)
		})
		for _, key := range keys {
			if err := writeLegacyReviewFingerprintScalars(digest, value.MapIndex(key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestReviewStreamedSuffixMatchesCompleteLegacyDigest(t *testing.T) {
	rate, _ := ParseFXRate("2.123456789123")
	currency, _ := ParseCurrency("CNY")
	day := ReviewSuffixDay{Key: ReviewDailyEvidenceKey{On: "2026-10-01"}, Evidence: ReviewReusableDailyEvidence{Input: ReviewDailyInput{SelectedFX: map[CurrencyCode]ReviewSelectedFX{currency: {Quote: FXQuote{Rate: rate}}}}, CoverageMissing: []string{"a<&>\""}}}
	for _, suffix := range []*ReviewSuffixCapture{{}, {Prefix: []ReviewSuffixDay{}}, {Prefix: []ReviewSuffixDay{day, day}, Plan: ReviewInvalidationPlan{Reason: "a<&>\"", Dirty: []ReviewDirtyPair{{From: "2026-10-01"}}}}} {
		raw, err := json.Marshal(suffix)
		if err != nil {
			t.Fatal(err)
		}
		legacy := sha256.New()
		legacy.Write(raw)
		legacy.Write([]byte{0})
		if err = writeLegacyReviewFingerprintScalars(legacy, reflect.ValueOf(suffix)); err != nil {
			t.Fatal(err)
		}
		actual, err := ReviewSuffixFingerprint(suffix)
		if err != nil {
			t.Fatal(err)
		}
		if actual != hex.EncodeToString(legacy.Sum(nil)) {
			t.Fatal("streaming changed complete JSON/private-scalar digest")
		}
	}
}
