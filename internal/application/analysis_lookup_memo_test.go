package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestAnalysisActivityPresenceMemoKeepsIdentityAndMisses(t *testing.T) {
	account := domain.AccountID("account")
	other := domain.AccountID("other")
	holding := domain.HoldingID("holding")
	effect := domain.ActivityEffect{AccountID: &account}
	activities := []domain.Activity{{EffectiveLocalDate: "2026-01-03", Effects: []domain.ActivityEffect{effect}}}
	u := analysisUniverse{activityPresence: make(map[componentActivityKey]bool), activityChecked: make(map[componentActivityKey]bool)}
	cashUSD := domain.ComponentID{AccountID: account, Currency: "USD", Cash: true}
	cashEUR := domain.ComponentID{AccountID: account, Currency: "EUR", Cash: true}
	held := domain.ComponentID{AccountID: account, HoldingID: &holding}
	absent := domain.ComponentID{AccountID: other}
	for _, tc := range []struct {
		component domain.ComponentID
		want      bool
	}{{cashUSD, true}, {cashEUR, true}, {held, false}, {absent, false}} {
		got := u.componentHasActivityInRange(tc.component, activities, "2026-01-01", "2026-01-31")
		if got != tc.want {
			t.Fatalf("component %+v: got %v want %v", tc.component, got, tc.want)
		}
	}
	if len(u.activityChecked) != 4 {
		t.Fatalf("memo entries=%d want 4", len(u.activityChecked))
	}
	// Both true and false are retained for repeat days in this request.
	if !u.componentHasActivityInRange(cashUSD, nil, "2026-01-01", "2026-01-31") || u.componentHasActivityInRange(absent, activities, "2026-01-01", "2026-01-31") {
		t.Fatal("memo did not retain both outcomes")
	}
	if componentHasActivityInRange(cashUSD, activities, "2026-01-04", "2026-01-31") {
		t.Fatal("effective date range changed")
	}
	holdingEffect := domain.ActivityEffect{HoldingID: &holding}
	if !componentHasActivityInRange(held, []domain.Activity{{EffectiveLocalDate: "2026-01-03", Effects: []domain.ActivityEffect{holdingEffect}}}, "2026-01-01", "2026-01-31") {
		t.Fatal("holding ID match lost")
	}
}

func TestAnalysisStampedQuoteIndexesMatchFirstWinsScans(t *testing.T) {
	instrument := domain.InstrumentID("instrument")
	otherInstrument := domain.InstrumentID("other")
	quotes := []domain.InstrumentQuote{
		{ID: "q", InstrumentID: otherInstrument, Currency: "USD"},
		{ID: "q", InstrumentID: instrument, Currency: "EUR"},
		{ID: "q", InstrumentID: instrument, Currency: "USD"},
		{ID: "q", InstrumentID: instrument, Currency: "USD"},
	}
	household := domain.HouseholdID("household")
	fx := []domain.FXQuote{
		{ID: "fx", HouseholdID: "other", BaseCurrency: "EUR", QuoteCurrency: "USD"},
		{ID: "fx", HouseholdID: household, BaseCurrency: "EUR", QuoteCurrency: "USD"},
		{ID: "inverse", HouseholdID: household, BaseCurrency: "USD", QuoteCurrency: "EUR"},
	}
	input := AnalysisInputs{Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: household, BaseCurrency: "USD"}}, InstrumentQuotes: quotes, FXQuotes: fx}
	u := analysisUniverse{}
	u.indexStampedQuotes(input)
	cutoff := time.Time{}
	for _, tc := range []struct {
		id         string
		instrument domain.InstrumentID
		currency   domain.CurrencyCode
		want       int
	}{
		{"q", instrument, "USD", 2}, {"q", instrument, "EUR", 1}, {"q", otherInstrument, "USD", 0}, {"missing", instrument, "USD", -1}, {"q", instrument, "JPY", -1},
	} {
		item := domain.DailyValuationSnapshotItem{InstrumentID: &tc.instrument, QuoteID: &tc.id, NativeCurrency: tc.currency}
		got := u.quoteForItem(item, quotes, cutoff)
		old := (analysisUniverse{}).quoteForItem(item, quotes, cutoff)
		if (got == nil) != (tc.want < 0) || got != old {
			t.Fatalf("instrument stamp %+v: indexed=%p scanned=%p", tc, got, old)
		}
		if tc.want >= 0 && got != &quotes[tc.want] {
			t.Fatalf("instrument stamp %+v did not keep first match", tc)
		}
	}
	for _, tc := range []struct {
		id     string
		native domain.CurrencyCode
		ok     bool
	}{{"fx", "EUR", false}, {"inverse", "EUR", true}, {"missing", "EUR", false}, {"inverse", "USD", true}} {
		item := domain.DailyValuationSnapshotItem{FXQuoteID: &tc.id, NativeCurrency: tc.native}
		got, ok := u.fxQuoteForItem(item, input, cutoff)
		old, oldOK := (analysisUniverse{}).fxQuoteForItem(item, input, cutoff)
		if ok != tc.ok || ok != oldOK || (got == nil) != (old == nil) {
			t.Fatalf("FX stamp %+v: indexed=%+v/%v scanned=%+v/%v", tc, got, ok, old, oldOK)
		}
		if ok && got != nil && old != nil && *got != *old {
			t.Fatalf("FX stamp %+v value changed", tc)
		}
	}
}
