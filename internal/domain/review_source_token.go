package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"io"
	"reflect"
	"sort"
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
