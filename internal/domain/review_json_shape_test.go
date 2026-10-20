package domain

import (
	"reflect"
	"testing"
)

// Review fingerprints and session MACs serialize these typed capture shapes.
// Keep binary floats out of them: Sonic encodes negative zero differently
// from the previous JSON engine, while financial values use exact decimals.
func TestReviewIntegrityJSONShapesHaveNoBinaryFloats(t *testing.T) {
	roots := []any{
		ReviewHistoricalBatch{}, ReviewSuffixCapture{}, ReviewTemporalCertificate{},
		ReviewProjectionCapture{}, ReviewCertifiedCapture{}, ReviewDailyInput{},
		ReviewDailyResult{}, ProductContract{}, ProductTermsReceipt{},
	}
	seen := map[reflect.Type]bool{}
	var check func(reflect.Type, string)
	check = func(typ reflect.Type, path string) {
		if seen[typ] {
			return
		}
		seen[typ] = true
		switch typ.Kind() {
		case reflect.Float32, reflect.Float64:
			t.Errorf("binary float in integrity JSON shape: %s", path)
		case reflect.Pointer, reflect.Slice, reflect.Array:
			check(typ.Elem(), path+"[]")
		case reflect.Map:
			check(typ.Key(), path+"{key}")
			check(typ.Elem(), path+"{value}")
		case reflect.Struct:
			for i := 0; i < typ.NumField(); i++ {
				field := typ.Field(i)
				check(field.Type, path+"."+field.Name)
			}
		}
	}
	for _, root := range roots {
		typ := reflect.TypeOf(root)
		check(typ, typ.String())
	}
}
