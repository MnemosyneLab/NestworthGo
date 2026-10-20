package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"reflect"
	"sort"
	"sync"
)

// ReviewHistoricalFingerprint covers the complete consistent financial read:
// origin, current directory/holding facts, effective activities, corrections,
// quotes, FX, metadata and cost events. Unlike MAX(updated_at), a deletion or
// withdrawal changes this token. The publisher recaptures it in its own SQL
// transaction before writing a completed review projection.
func ReviewHistoricalFingerprint(batch ReviewHistoricalBatch) (string, error) {
	raw, err := json.Marshal(batch)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, _ = digest.Write(raw)
	_, _ = digest.Write([]byte{0})
	// Several exact domain values intentionally keep their decimal storage
	// private. encoding/json therefore emits them as empty objects. Add their
	// canonical values in deterministic field/slice/map order so edits to an
	// existing price, FX rate, quantity, or amount invalidate the source token.
	if err := writeReviewFingerprintScalars(digest, reflect.ValueOf(batch)); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

type reviewFingerprintCanonical interface{ Canonical() string }
type reviewFingerprintAmount interface {
	CanonicalAmount() string
	Currency() CurrencyCode
}

var reviewFingerprintAmountType = reflect.TypeFor[reviewFingerprintAmount]()
var reviewFingerprintCanonicalType = reflect.TypeFor[reviewFingerprintCanonical]()

// Only immutable Go type metadata is memoized. Callers traverse the fixed
// HistoricalBatch/Suffix schema; no ledger values, IDs or dates are retained.
var reviewFingerprintScalarTypes sync.Map // reflect.Type -> uint8 (amount=1, canonical=2)
func reviewFingerprintScalarKind(t reflect.Type) uint8 {
	if cached, ok := reviewFingerprintScalarTypes.Load(t); ok {
		return cached.(uint8)
	}
	var kind uint8
	if t.Implements(reviewFingerprintAmountType) {
		kind = 1
	} else if t.Implements(reviewFingerprintCanonicalType) {
		kind = 2
	}
	cached, _ := reviewFingerprintScalarTypes.LoadOrStore(t, kind)
	return cached.(uint8)
}

func writeReviewFingerprintScalars(digest hash.Hash, value reflect.Value) error {
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
		// Check the immutable type before boxing: most nested values are not
		// financial scalars. Interface() on each addressable struct copies it.
		kind := reviewFingerprintScalarKind(value.Type())
		if kind == 1 {
			amount := value.Interface().(reviewFingerprintAmount)
			writeReviewFingerprintScalar(digest, "amount:"+amount.Currency().String(), amount.CanonicalAmount())
			return nil
		}
		if kind == 2 {
			canonical := value.Interface().(reviewFingerprintCanonical)
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
			if err := writeReviewFingerprintScalars(digest, value.Field(index)); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if err := writeReviewFingerprintScalars(digest, value.Index(index)); err != nil {
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
			if err := writeReviewFingerprintScalars(digest, value.MapIndex(key)); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeReviewFingerprintScalar(digest io.Writer, typeName, value string) {
	_, _ = io.WriteString(digest, typeName)
	_, _ = digest.Write([]byte{':'})
	_, _ = io.WriteString(digest, value)
	_, _ = digest.Write([]byte{0})
}
