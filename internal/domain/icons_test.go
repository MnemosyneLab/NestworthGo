package domain

import (
	"reflect"
	"testing"
)

func TestSupportedIconKeysAreSortedAndValidate(t *testing.T) {
	keys := SupportedIconKeys()
	if len(keys) == 0 {
		t.Fatal("supported icon catalog is empty")
	}
	if !reflect.DeepEqual(keys, append([]string(nil), sortedStrings(keys)...)) {
		t.Fatalf("icon keys are not sorted: %v", keys)
	}
	for _, key := range keys {
		if err := ValidateIconKey(key); err != nil {
			t.Fatalf("ValidateIconKey(%q): %v", key, err)
		}
	}
	if err := ValidateIconKey("not-in-catalog"); err == nil {
		t.Fatal("unknown icon key was accepted")
	}
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	for i := 0; i < len(result); i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}
