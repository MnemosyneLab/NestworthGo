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

func TestShortNamePreviewEndpointsRetainShareClassIdentity(t *testing.T) {
	state, _, _, first, second := changeTestState(t)
	for index, id := range []HoldingID{first, second} {
		h := state.Holdings[id]
		h.Current = state.Holdings[first].Current
		h.InstrumentID = NewInstrumentID()
		h.InstrumentShortName = "Family fund"
		h.InstrumentSymbol = []string{"000001", "000002"}[index]
		h.InstrumentName = "Share class " + h.InstrumentSymbol
		h.InstrumentISIN = "ISIN-" + h.InstrumentSymbol
		state.Holdings[id] = h
	}
	quantity, err := ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, id := range []HoldingID{first, second} {
		preview, err := PreviewChange(state, PositionAdjustmentInput{HouseholdID: state.HouseholdID, HoldingID: id, Quantity: quantity, Added: false, EffectiveAt: state.Now})
		if err != nil {
			t.Fatal(err)
		}
		h := state.Holdings[id]
		name := preview.Resulting[0].Name
		if !strings.HasPrefix(name, "Family fund · ") || !strings.Contains(name, h.InstrumentSymbol) || !strings.Contains(name, h.InstrumentName) || !strings.Contains(name, h.InstrumentISIN) || names[name] {
			t.Fatalf("ambiguous endpoint %q", name)
		}
		if preview.Resulting[0].HoldingID == nil || *preview.Resulting[0].HoldingID != id {
			t.Fatal("endpoint identity changed")
		}
		names[name] = true
	}
}
