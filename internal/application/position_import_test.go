package application

import (
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestBackdatedPositionImportPreviewsAndCommitsWithoutCash(t *testing.T) {
	ctx, service, repository, database, bootstrap, setNow := historicalInsertionFixture(t)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "500", "CNY", "2026-09-25"); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	addedCash, err := domain.ParseMoney("25", "CNY")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, Amount: addedCash, Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(28)}); err != nil {
		t.Fatal(err)
	}
	cost, err := domain.ParseUnitPrice("7.25")
	if err != nil {
		t.Fatal(err)
	}
	command := domain.PositionImportInput{
		HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, InstrumentID: instrument.ID,
		Quantity: mustQuantity(t, "3"), UnitCost: &cost, Currency: "CNY", EffectiveAt: historicalInsertionTime(26),
	}
	preview, token, err := service.PreviewChangeGuarded(ctx, command)
	if err != nil {
		t.Fatalf("preview existing position: %v", err)
	}
	if preview.Activity.Kind != domain.ActivityPositionTransfer || len(preview.Effects) != 1 || preview.Effects[0].CostUnitPrice == nil || preview.Effects[0].CostUnitPrice.Canonical() != "7.25" {
		t.Fatalf("preview must be a costed position adjustment: %+v", preview)
	}
	if len(preview.Resulting) != 1 || preview.Resulting[0].Quantity != "3" {
		t.Fatalf("preview changed more than the position: %+v", preview.Resulting)
	}
	if holdings, err := service.ListHoldings(ctx, account.Account.ID, false); err != nil || len(holdings) != 0 {
		t.Fatalf("preview persisted a Holding: %+v, %v", holdings, err)
	}
	committed, err := service.RecordChangeGuarded(ctx, command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
	if err != nil {
		t.Fatalf("commit existing position: %v", err)
	}
	if committed.Activity.Kind != domain.ActivityPositionTransfer {
		t.Fatalf("recorded a trade instead of an adjustment: %s", committed.Activity.Kind)
	}
	holdings, err := service.ListHoldings(ctx, account.Account.ID, false)
	if err != nil || len(holdings) != 1 || holdings[0].Quantity.Canonical() != "3" {
		t.Fatalf("committed Holding = %+v, %v", holdings, err)
	}
	events, err := repository.ListCostBasisEvents(ctx, holdings[0].ID, domain.CostBasisReadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	basis, err := domain.ReplayCostBasis(nil, events)
	if err != nil || basis.Current.Quantity.Canonical() != "3" || basis.Current.AverageUnitCost.Canonical() != "7.25" {
		t.Fatalf("replayed cost basis = %+v, %v", basis, err)
	}
	var cash string
	if err := database.SQL.QueryRowContext(ctx, "SELECT amount FROM account_cash_values WHERE account_id = ? ORDER BY effective_at DESC LIMIT 1", account.Account.ID.String()).Scan(&cash); err != nil || cash != "525" {
		t.Fatalf("import changed cash: %q, %v", cash, err)
	}
	if _, err := service.PreviewChange(ctx, command); !hasDomainCode(err, domain.ErrConflict) {
		t.Fatalf("duplicate import = %v, want conflict", err)
	}
}

func TestPositionImportRequiresKnownCostAndMatchingCurrency(t *testing.T) {
	ctx, service, _, _, bootstrap, setNow := historicalInsertionFixture(t)
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "99", "2026-09-25", false); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	command := domain.PositionImportInput{HouseholdID: bootstrap.Household.ID, AccountID: account.Account.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1"), Currency: "CNY", EffectiveAt: historicalInsertionTime(29)}
	if _, err := service.PreviewChange(ctx, command); !hasDomainCode(err, domain.ErrCostBasisRequired) {
		t.Fatalf("missing cost with a market quote = %v, want cost_basis_required", err)
	}
	zeroCost, err := domain.ParseUnitPrice("0")
	if err != nil {
		t.Fatal(err)
	}
	command.UnitCost = &zeroCost
	command.Currency = "USD"
	if _, err := service.PreviewChange(ctx, command); !hasDomainCode(err, domain.ErrValidation) {
		t.Fatalf("mismatched cost currency = %v, want validation", err)
	}
	command.Currency = "CNY"
	if _, err := service.PreviewChange(ctx, command); err != nil {
		t.Fatalf("explicit zero cost should remain distinct from unknown: %v", err)
	}
}
