package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestReadOnlyFinancialAttributionDoesNotMaintainSnapshots(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	account := overviewAccount(t, s, owner, "Synthetic read-only return", "bank_account", "asset", "balance", "CNY", "100")
	if _, err := s.StartHistory(t.Context(), "UTC"); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	if _, err := s.RecordChange(t.Context(), domain.MoneyAddedInput{HouseholdID: account.Account.HouseholdID, AccountID: account.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: *now}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 1)
	var before int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&before); err != nil {
		t.Fatal(err)
	}
	// The real database forbids every SQL write during the strict path. Missing or
	// dirty daily snapshots must be replayed in memory, never repaired as fallback.
	if _, err := db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	request := FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02"}
	result, err := s.BuildFinancialAttribution(WithReadOnlyFinancialAttribution(t.Context()), request)
	if err != nil {
		t.Fatal(err)
	}
	wantAttributionStatus(t, result, "compatible", "")
	wantOverviewAmount(t, result.Content.Change.NetWorth.Value, "50")
	wantOverviewAmount(t, attributionDriver(result.Content.Attribution, "income"), "50")
	if result.Content.Attribution.InvestmentReturn == nil {
		t.Fatal("return coverage missing")
	}
	var after int
	if err := db.SQL.QueryRow("SELECT count(*) FROM daily_valuation_snapshots").Scan(&after); err != nil || before != after {
		t.Fatal("read-only call maintained snapshots")
	}
	if _, err := db.SQL.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	normal, err := s.BuildFinancialAttribution(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	// Same authoritative projection, including exact returns, coverage, precision,
	// missingness and source basis. Only the explicit maintenance declaration differs.
	result.Content.Attribution.Basis = normal.Content.Attribution.Basis
	a, _ := jsoncodec.Marshal(result.Content.Attribution)
	b, _ := jsoncodec.Marshal(normal.Content.Attribution)
	if string(a) != string(b) {
		t.Fatalf("read-only attribution differs from maintained calculation:\n%s\n%s", a, b)
	}
	// Both paths retain the same endpoint-to-period date semantics.
	if result.Content.Attribution.Period.From != "2026-08-02" || result.Content.Attribution.Period.To != "2026-08-02" {
		t.Fatal("endpoint date shifted")
	}
}

func TestReadOnlyAttributionPriceReturnMissingAndBudget(t *testing.T) {
	s, db, owner, now := overviewFixture(t)
	a := overviewAccount(t, s, owner, "Synthetic broker", "brokerage", "asset", "holdings", "CNY", "")
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	for _, q := range []struct{ date, price string }{{"2026-08-01", "5"}, {"2026-08-02", "6"}} {
		if _, err := s.AppendManualInstrumentQuote(t.Context(), instrument.ID, q.price, q.date, false); err != nil {
			t.Fatal(err)
		}
	}
	*now = now.AddDate(0, 0, -2)
	holding, err := s.CreateHolding(t.Context(), HoldingInput{AccountID: a.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "10"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistoryWithCosts(t.Context(), "UTC", map[domain.HoldingID]string{holding.ID: "5"}); err != nil {
		t.Fatal(err)
	}
	*now = now.AddDate(0, 0, 2)
	if _, err := db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	request := FinancialComparisonRequest{LeftAsOf: "2026-08-01", RightAsOf: "2026-08-02", Scope: FinancialContextScopeRequest{Kind: "accounts", AccountIDs: []string{a.Account.ID.String()}}}
	result, err := s.BuildFinancialAttribution(WithReadOnlyFinancialAttribution(t.Context()), request)
	if err != nil {
		t.Fatal(err)
	}
	wantAttributionStatus(t, result, "compatible", "")
	wantOverviewAmount(t, result.Content.Attribution.InvestmentReturn.Amount.Value, "10")
	if result.Content.Attribution.InvestmentReturn.Rate == nil || !result.Content.Attribution.InvestmentReturn.Available {
		t.Fatal("authoritative return missing")
	}
	if _, err := db.SQL.Exec("PRAGMA query_only=OFF"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("DELETE FROM instrument_quotes"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("PRAGMA query_only=ON"); err != nil {
		t.Fatal(err)
	}
	missing, err := s.BuildFinancialAttribution(WithReadOnlyFinancialAttribution(t.Context()), request)
	if err != nil {
		t.Fatal(err)
	}
	if link := missing.Content.Attribution; link.InvestmentReturn != nil && link.InvestmentReturn.Available {
		t.Fatal("missing quote produced available return")
	}
	*now = now.AddDate(2, 0, 0)
	request.RightAsOf = "2027-08-02"
	capped, err := s.BuildFinancialAttribution(WithReadOnlyFinancialAttribution(t.Context()), request)
	if err != nil {
		t.Fatal(err)
	}
	wantAttributionStatus(t, capped, "unavailable", "read_only_period_budget_exceeded")
}
