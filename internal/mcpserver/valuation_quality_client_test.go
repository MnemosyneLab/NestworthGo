package mcpserver

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestMCPLeafPendingOneToZeroAndSharedQuote(t *testing.T) {
	fx := responseReviewFixture(t, 1)
	holdings, err := fx.app.ListHoldings(t.Context(), domain.AccountID(fx.brokerage), false)
	if err != nil || len(holdings) != 1 {
		t.Fatal(holdings, err)
	}
	instrument := holdings[0].InstrumentID
	if _, err = fx.app.UpdateInstrument(t.Context(), instrument, application.InstrumentInput{QuoteSource: "agent"}); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	read := func(want int) FinancialContextResponse {
		t.Helper()
		r := decodeContext(t, call(t, c, "get_financial_context", application.FinancialContextRequest{Scope: application.FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{fx.brokerage}}}, false))
		if r.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount != want {
			t.Fatal(r.Content.Summary)
		}
		item := decodeItem(t, call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: r.ContextID, Ref: r.Content.Scope.AccountRefs[0]}, false))
		sum := 0
		for _, p := range item.Positions {
			sum += p.PendingContribution
			if p.Kind == "account" && p.PendingContribution != 0 {
				t.Fatal("rollup counted")
			}
		}
		if sum != want {
			t.Fatalf("leaf pending explanations sum %d, want %d", sum, want)
		}
		return r
	}
	old := read(1)
	if _, err = fx.app.UpdateInstrument(t.Context(), instrument, application.InstrumentInput{QuoteSource: "manual"}); err != nil {
		t.Fatal(err)
	}
	healthy := read(0)
	item := decodeItem(t, call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: healthy.ContextID, Ref: healthy.Content.Scope.AccountRefs[0]}, false))
	for _, gap := range item.Gaps {
		if gap.Code == "stale_observation" {
			t.Fatal("manual Agent quote warned as stale", gap)
		}
	}
	for _, e := range item.Evidence {
		if e.Kind == "price" && (e.SourceKind != "agent" || e.Freshness != "manual" || e.EffectiveDate != "2026-09-01") {
			t.Fatal(e)
		}
	}
	frozen := decodeItem(t, call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: old.ContextID, Ref: old.Content.Scope.AccountRefs[0]}, false))
	pending := 0
	for _, p := range frozen.Positions {
		pending += p.PendingContribution
	}
	if pending != 1 {
		t.Fatal("source change mutated frozen context")
	}
	// One quote adopted by two distinct leaves contributes once per leaf.
	members, err := fx.app.ListMembers(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	other, err := fx.app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic shared quote account", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: other.Account.ID.String(), InstrumentID: instrument.String(), Quantity: "2", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.UpdateInstrument(t.Context(), instrument, application.InstrumentInput{QuoteSource: "agent"}); err != nil {
		t.Fatal(err)
	}
	r, err := fx.app.BuildFinancialContext(t.Context(), application.FinancialContextRequest{})
	if err != nil {
		t.Fatal(err)
	}
	prices, sum := 0, 0
	for _, e := range r.Content.Evidence {
		if e.Kind == "price" {
			prices++
		}
	}
	for _, p := range r.Content.Positions {
		sum += p.PendingContribution
	}
	if prices != 1 || sum != 2 || r.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount != 2 {
		t.Fatalf("shared quote evidence=%d leaves=%d", prices, sum)
	}
}
func TestMCPPriceAndFXPendingEvidence(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	i, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic foreign investment", Type: "crypto", QuoteCurrency: "EUR", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: i.ID.String(), Quantity: "2", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{InstrumentID: i.ID.String(), Currency: "EUR", Kind: "close", Date: "2026-09-01", Timezone: "UTC", Value: "1", SourceTitle: "Synthetic price"}, {BaseCurrency: "USD", QuoteCurrency: "EUR", Kind: "daily_reference", Date: "2026-09-01", Value: "0.9", SourceTitle: "Synthetic FX"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.SetFXPreference(t.Context(), "USD", "EUR", "agent"); err != nil {
		t.Fatal(err)
	}
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC) })
	c := ledgerSession(t, fx)
	check := func(wantPriceCarry bool) {
		t.Helper()
		capture := decodeContext(t, call(t, c, "get_financial_context", application.FinancialContextRequest{Scope: application.FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{fx.brokerage}}}, false))
		item := decodeItem(t, call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: capture.ContextID, Ref: capture.Content.Scope.AccountRefs[0]}, false))
		sum := 0
		for _, p := range item.Positions {
			sum += p.PendingContribution
			if p.Kind != "holding" {
				continue
			}
			q := p.DisplayValuation.Quality
			hasPrice, hasFX := false, false
			for _, r := range q.Reasons {
				hasPrice = hasPrice || r == domain.ReasonCarriedPrice
				hasFX = hasFX || r == domain.ReasonCarriedFX
			}
			if !hasFX || hasPrice != wantPriceCarry || p.PendingContribution != 1 {
				t.Fatal(p)
			}
		}
		if sum != 1 || capture.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount != 1 {
			t.Fatal("price + FX counted twice")
		}
	}
	check(false)
	if _, err = fx.app.UpdateInstrument(t.Context(), i.ID, application.InstrumentInput{QuoteSource: "agent"}); err != nil {
		t.Fatal(err)
	}
	check(true)
	// Fresh manual price with genuinely absent FX remains incomplete, not healthy.
	missing, err := fx.app.CreateInstrument(t.Context(), application.InstrumentInput{Name: "Synthetic missing FX", Type: "crypto", QuoteCurrency: "CHF", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: missing.ID.String(), Quantity: "1", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err = fx.app.AppendManualInstrumentQuote(t.Context(), missing.ID, "1", "2026-10-13", false); err != nil {
		t.Fatal(err)
	}
	capture := decodeContext(t, call(t, c, "get_financial_context", application.FinancialContextRequest{}, false))
	if capture.Content.Summary.NetWorth.Value != nil {
		t.Fatal("missing FX hid a real gap")
	}
}

func TestMCPClosedDayCarryKeepsEffectiveFreshness(t *testing.T) {
	fx := newLedgerFixtureWithClock(t, func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	if _, err := fx.app.CreateHolding(t.Context(), application.HoldingInput{AccountID: fx.brokerage, InstrumentID: fx.instrument, Quantity: "2", UnitCost: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.app.UpdateInstrument(t.Context(), domain.InstrumentID(fx.instrument), application.InstrumentInput{QuoteSource: "agent"}); err != nil {
		t.Fatal(err)
	}
	fx.app.SetClock(func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) })
	if _, err := fx.app.ImportAgentMarketData(t.Context(), uuid.NewString(), application.AgentMarketDataInput{Items: []application.AgentMarketDataItem{{InstrumentID: fx.instrument, Currency: "USD", Kind: "close", Date: "2026-10-16", Timezone: "UTC", Value: "1.23456789", SourceTitle: "Synthetic Friday close"}}}); err != nil {
		t.Fatal(err)
	}
	c := ledgerSession(t, fx)
	for _, date := range []string{"2026-10-17", "current"} {
		r := decodeContext(t, call(t, c, "get_financial_context", application.FinancialContextRequest{AsOf: date}, false))
		if r.Content.Summary.DisplayValuation.NetWorth.Quality.CarriedCount != 0 {
			t.Fatal(r.Content.Summary)
		}
		found := false
		for _, ref := range r.Content.Scope.AccountRefs {
			item := decodeItem(t, call(t, c, "get_financial_context_item", FinancialContextItemInput{ContextID: r.ContextID, Ref: ref}, false))
			for _, e := range item.Evidence {
				if e.Kind == "price" {
					found = true
					if !e.ClosedCarry || e.Freshness != "fresh" || e.SourceKind != "agent" {
						t.Fatal(e)
					}
				}
			}
			for _, g := range item.Gaps {
				if g.Code == "stale_observation" {
					t.Fatal("closed-day carry warned as stale", g)
				}
			}
		}
		if !found {
			t.Fatal("closed-day anchor evidence absent")
		}
	}
}
