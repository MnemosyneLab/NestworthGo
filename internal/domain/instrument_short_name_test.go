package domain

import (
	"strings"
	"testing"
	"time"
)

func TestInstrumentUniversalDisplayRuleAndValidation(t *testing.T) {
	for _, tc := range []struct{ name, symbol, alias, want string }{
		{"Full fund A", "001234", "  家庭基金 🌿  ", "家庭基金 🌿"},
		{" Full name ", " CODE ", "\u2003\t\n", "CODE"},
		{" Full name ", "  ", "", "Full name"},
		{"", "", "", ""},
		{"Full name", " CODE ", "\u0085\uFEFF", "CODE"},
		{strings.Repeat("长", 120), "", "", strings.Repeat("长", 120)},
	} {
		if got := InstrumentDisplayLabel(tc.name, tc.symbol, tc.alias); got != tc.want {
			t.Fatalf("label %q want %q", got, tc.want)
		}
	}
	for _, kind := range AllInstrumentTypes() {
		in := InstrumentInput{Name: "Full name", ShortName: "  家庭 🌿  ", Type: kind, QuoteCurrency: "USD"}
		i, err := NewInstrument(in, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if i.ShortName != "家庭 🌿" {
			t.Fatalf("%s alias %q", kind, i.ShortName)
		}
	}
	if _, err := NormalizeInstrumentShortName(strings.Repeat("界", 120)); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeInstrumentShortName(strings.Repeat("界", 121)); err == nil {
		t.Fatal("accepted overlong alias")
	}
}
