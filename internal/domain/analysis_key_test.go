package domain

import "testing"

func TestComponentKeyPreservesIdentity(t *testing.T) {
	holding, instrument := HoldingID("h"), InstrumentID("i")
	for _, tc := range []struct {
		id   ComponentID
		want string
	}{
		{ComponentID{AccountID: "a"}, "a/value"},
		{ComponentID{AccountID: "a", Cash: true}, "a/cash"},
		{ComponentID{AccountID: "a", Cash: true, Currency: "USD"}, "a/cash/currency:USD"},
		{ComponentID{AccountID: "a", Cash: true, Currency: "EUR", InstrumentID: &instrument}, "a/cash/currency:EUR/instrument:i"},
		{ComponentID{AccountID: "a", HoldingID: &holding, Cash: true, Currency: "USD", InstrumentID: &instrument}, "a/holding:h/instrument:i"},
		{ComponentID{AccountID: "a", InstrumentID: &instrument}, "a/value/instrument:i"},
		{ComponentID{AccountID: "a", HoldingID: &holding}, "a/holding:h"},
	} {
		if got := tc.id.Key(); got != tc.want {
			t.Fatalf("%+v: got %q, want %q", tc.id, got, tc.want)
		}
	}
}
