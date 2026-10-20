package sqlite

import (
	"fmt"
	"testing"
)

func publicationEqualityFixture(t *testing.T, values []any, result bool) *reviewPublicationEquality {
	t.Helper()
	key, err := appendCertifiedRow(nil, values[:5])
	if err != nil {
		t.Fatal(err)
	}
	image := append([]any(nil), values[:len(values)-1]...)
	e := &reviewPublicationEquality{inputs: map[string][]any{}, results: map[string][]any{}}
	if result {
		e.results[string(key)] = image
	} else {
		e.inputs[string(key)] = image
	}
	return e
}

func TestReviewPublicationEqualityRequiresEveryParameter(t *testing.T) {
	for _, result := range []bool{false, true} {
		values := []any{"household", "instrument", "UTC/v1", "2026-10-01", "rule:r/v:1", "1", "CNY", "stock", "100", "50", `{"exactFX":"2"}`, `[]`, "fingerprint", "old-time"}
		if result {
			values = []any{"rule", int64(1), "instrument", "UTC/v1", "2026-10-01", "true", nil, `{"truth":"true"}`, "fingerprint", "old-time"}
		}
		for column := range values {
			t.Run(fmt.Sprintf("result=%v/column=%d", result, column), func(t *testing.T) {
				e := publicationEqualityFixture(t, values, result)
				changed := append([]any(nil), values...)
				switch value := changed[column].(type) {
				case int64:
					changed[column] = value + 1
				default:
					changed[column] = fmt.Sprint(value) + " changed"
				}
				var omit bool
				var err error
				if result {
					omit, err = e.omitResult(changed)
				} else {
					omit, err = e.omitInput(changed)
				}
				if err != nil || omit != (column == len(values)-1) {
					t.Fatal("non-timestamp parameter mismatch was omitted", omit, err)
				}
			})
		}
	}
}

func TestReviewPublicationEqualityConsumesImagesAfterMismatchAndEquality(t *testing.T) {
	values := []any{"rule", int64(1), "instrument", "UTC/v1", "2026-10-01", "true", nil, `{"truth":"true"}`, "fp", "at"}
	for _, changedFirst := range []bool{false, true} {
		e := publicationEqualityFixture(t, values, true)
		first := append([]any(nil), values...)
		if changedFirst {
			first[7] = `{"truth":"false"}`
		}
		if omit, err := e.omitResult(first); err != nil || omit == changedFirst {
			t.Fatal("incorrect first comparison", omit, err)
		}
		if omit, err := e.omitResult(values); err != nil || omit {
			t.Fatal("repeated key reused an obsolete image", omit, err)
		}
	}
	for _, replacement := range []any{"", []byte{}, "null"} {
		e := publicationEqualityFixture(t, values, true)
		changed := append([]any(nil), values...)
		changed[6] = replacement
		if omit, err := e.omitResult(changed); err != nil || omit {
			t.Fatal("SQL NULL was equated with a non-NULL value", omit, err)
		}
	}
	if omit, err := (*reviewPublicationEquality)(nil).omitResult(values); err != nil || omit {
		t.Fatal("unknown provenance enabled omission", omit, err)
	}
}
