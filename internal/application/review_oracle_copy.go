package application

import (
	"reflect"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Freeze the full DTO graph before running a second planner. Public pointers,
// maps and slices must not alias captured/bounded evidence. Domain values with
// private fields (exact decimals and time.Time) are immutable values: copying
// them as values preserves precision and clock/zone identity. JSON round trips
// would lose private decimal scalars and are not an oracle copy mechanism.
func cloneReviewOracleRules(rules []domain.ReviewRulePublication) []domain.ReviewRulePublication {
	var copyValue func(reflect.Value) reflect.Value
	copyValue = func(v reflect.Value) reflect.Value {
		switch v.Kind() {
		case reflect.Pointer:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			result := reflect.New(v.Type().Elem())
			result.Elem().Set(copyValue(v.Elem()))
			return result
		case reflect.Slice:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			result := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
			for i := 0; i < v.Len(); i++ {
				result.Index(i).Set(copyValue(v.Index(i)))
			}
			return result
		case reflect.Map:
			if v.IsNil() {
				return reflect.Zero(v.Type())
			}
			result := reflect.MakeMapWithSize(v.Type(), v.Len())
			iter := v.MapRange()
			for iter.Next() {
				result.SetMapIndex(iter.Key(), copyValue(iter.Value()))
			}
			return result
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				if v.Type().Field(i).PkgPath != "" {
					return v
				}
			}
			result := reflect.New(v.Type()).Elem()
			for i := 0; i < v.NumField(); i++ {
				result.Field(i).Set(copyValue(v.Field(i)))
			}
			return result
		default:
			return v
		}
	}
	return copyValue(reflect.ValueOf(rules)).Interface().([]domain.ReviewRulePublication)
}
