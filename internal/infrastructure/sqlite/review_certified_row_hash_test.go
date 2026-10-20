package sqlite

import (
	"bytes"
	"testing"
)

func TestReviewCertifiedRowEncodingPreservesTypesAndBoundaries(t *testing.T) {
	rows := [][]any{
		{nil}, {""}, {[]byte{}}, {int64(1)}, {float64(1)}, {"1"}, {true},
		{"ab", "c"}, {"a", "bc"}, {"a\x00b"}, {"a", "b"},
		{"{\"a\":1}"}, {[]byte("{\"a\":1}")},
	}
	seen := map[string]bool{}
	var reusable []byte
	for _, row := range rows {
		encoded, err := appendCertifiedRow(reusable[:0], row)
		if err != nil {
			t.Fatal(err)
		}
		if seen[string(encoded)] {
			t.Fatalf("type/tuple collision for %#v", row)
		}
		seen[string(encoded)] = true
		stable := bytes.Clone(encoded)
		reusable = encoded
		again, err := appendCertifiedRow(nil, row)
		if err != nil || !bytes.Equal(stable, again) {
			t.Fatal("row-buffer reuse changed encoding", err)
		}
	}
	if _, err := appendCertifiedRow(nil, []any{struct{}{}}); err == nil {
		t.Fatal("unknown driver value accepted")
	}
}
