package application

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestFullRebuildReplacesCorruptDerivedResultsAndPreservesFacts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/rebuild.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Family", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	boot, _ := s.Bootstrap(ctx)
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}, InitialAmount: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	change, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "100", "CNY"), EffectiveAt: now})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 3)
	if _, err := s.RebuildHistoricalSnapshots(ctx, "2026-09-25", "2026-09-28"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE account_values SET amount='9999' WHERE projection_kind <> 'baseline'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `UPDATE daily_valuation_snapshots SET net_worth_amount='9999'`); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		result, err := s.RebuildDerivedData(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if result.From != "2026-09-25" || result.To != "2026-09-29" || result.SnapshotDays != 4 || result.IncompleteDays != 0 {
			t.Fatalf("unexpected result: %+v", result)
		}
		current, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		if got := current.Accounts[0].LatestValue.Amount.CanonicalAmount(); got != "1100" {
			t.Fatalf("current balance %s", got)
		}
		snapshots, err := repo.ListDailyValuationSnapshots(ctx, boot.Household.ID, time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshots) != 4 {
			t.Fatalf("snapshot count %d", len(snapshots))
		}
		for _, snapshot := range snapshots {
			want := "1100"
			if snapshot.LocalDate == "2026-09-25" {
				want = "1000"
			}
			if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != want {
				t.Fatalf("incorrect daily value %+v", snapshot)
			}
		}
		raw, err := repo.Activity(ctx, boot.Household.ID, change.Activity.ID)
		if err != nil {
			t.Fatal(err)
		}
		if raw.Effects[0].Money.CanonicalAmount() != "100" || !raw.EffectiveAt.Equal(change.Activity.EffectiveAt) {
			t.Fatal("source activity changed")
		}
		var count int
		if err := db.SQL.QueryRowContext(ctx, `SELECT count(*) FROM activities`).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("rebuild changed source ledger count %d", count)
		}
	}
	// A rebuild cannot supply a missing exchange rate. Keep the native input
	// and report that closed day as incomplete instead of inventing a total.
	_, err = s.CreateAccount(ctx, AccountInput{Name: "Foreign", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}, InitialAmount: "5"})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	result, err := s.RebuildDerivedData(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.SnapshotDays != 5 || result.IncompleteDays != 1 {
		t.Fatalf("missing FX was hidden: %+v", result)
	}

}

func TestRebuildDerivedDataKeepsCostAdjustmentsOutOfCurrentProjection(t *testing.T) {
	t.Parallel()
	f := newAccountingGainFixture(t, "3", "10")
	ctx := t.Context()
	bank, err := f.service.CreateAccount(ctx, AccountInput{
		Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: true,
		OwnerIDs: []domain.MemberID{f.boot.Members[0].ID}, InitialAmount: "500",
	})
	if err != nil {
		t.Fatal(err)
	}
	effective := f.now
	if _, err := f.service.RecordChange(ctx, domain.PositionCostAdjustmentInput{
		HouseholdID: f.boot.Household.ID, HoldingID: f.holdings[0].ID,
		UnitCost: reconciliationUnitCost(t, "20"), EffectiveAt: effective,
	}); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	sell := domain.TradeInput{
		HouseholdID: f.boot.Household.ID, Side: domain.TradeSell,
		SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID,
		InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "1"),
		Gross: mustMoney(t, "30", "CNY"), EffectiveAt: f.now,
	}
	posted, err := f.service.RecordChange(ctx, sell)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	sell.Gross = mustMoney(t, "33", "CNY")
	if _, err := f.service.FixChange(ctx, posted.Activity.ID, sell); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Hour)
	buy := domain.TradeInput{
		HouseholdID: f.boot.Household.ID, Side: domain.TradeBuy,
		SettlementAccountID: f.accounts[0].Account.ID, HoldingID: f.holdings[0].ID,
		InstrumentID: f.instrument.ID, Quantity: mustQuantity(t, "1"),
		Gross: mustMoney(t, "21", "CNY"), EffectiveAt: f.now,
	}
	reversed, err := f.service.RecordChange(ctx, buy)
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(time.Minute)
	if _, err := f.service.UndoChange(ctx, reversed.Activity.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ArchiveHolding(ctx, f.holdings[1].ID, true); err != nil {
		t.Fatal(err)
	}
	f.now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	if _, err := f.service.RebuildHistoricalSnapshots(ctx, "2026-09-25", "2026-09-26"); err != nil {
		t.Fatal(err)
	}

	checkOutputs := func() {
		t.Helper()
		gain, err := f.service.HoldingGain(ctx, f.holdings[0].ID)
		if err != nil || gain.Quantity != "2" || gain.TotalCost.Amount != "40" || gain.RealizedGain.Amount != "13" {
			t.Fatalf("cost basis after replay: gain=%+v err=%v", gain, err)
		}
		period, err := f.service.RealizedGainInRange(ctx, domain.GainScope{}, "2026-09-25", "2026-09-25")
		if err != nil || !period.Available || period.Total == nil || period.Total.Amount != "13" {
			t.Fatalf("realized gain after replay: period=%+v err=%v", period, err)
		}
		portfolio, err := f.repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
		if err != nil {
			t.Fatal(err)
		}
		var cash string
		for _, value := range portfolio.CashValues {
			if value.AccountID == f.accounts[0].Account.ID && value.Amount.Currency() == "CNY" {
				cash = value.Amount.CanonicalAmount()
			}
		}
		if cash != "1000000033" {
			t.Fatalf("ordinary cash projection = %s", cash)
		}
		balance := ""
		for _, record := range portfolio.Accounts {
			if record.Account.ID == bank.Account.ID && record.LatestValue != nil {
				balance = record.LatestValue.Amount.CanonicalAmount()
			}
		}
		if balance != "500" {
			t.Fatalf("ordinary account balance projection = %s", balance)
		}
		quantities := map[domain.HoldingID]string{}
		for _, holding := range portfolio.Holdings {
			quantities[holding.ID] = holding.Quantity.String()
		}
		if len(quantities) != len(f.holdings) || quantities[f.holdings[0].ID] != "2" || quantities[f.holdings[1].ID] != "0" || quantities[f.holdings[2].ID] != "0" {
			t.Fatalf("ordinary current quantities = %+v", quantities)
		}
	}

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := f.service.RebuildDerivedData(ctx); err != nil {
			t.Fatalf("rebuild attempt %d: %v", attempt+1, err)
		}
		checkOutputs()
	}

	// Force a late publication error after event/current rows have been staged.
	// The whole transaction must preserve the prior projections, snapshots,
	// generation state, holding quantities, and source activity facts.
	if _, err := f.db.SQL.ExecContext(ctx, `UPDATE account_cash_values SET amount='777' WHERE projection_kind='event'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.ExecContext(ctx, `UPDATE holdings SET quantity='77' WHERE id=?`, f.holdings[0].ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.ExecContext(ctx, `UPDATE daily_valuation_snapshots SET net_worth_amount='777' WHERE household_id=?`, f.boot.Household.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.ExecContext(ctx, `CREATE TRIGGER fail_rebuild_snapshot BEFORE INSERT ON daily_valuation_snapshots BEGIN SELECT RAISE(ABORT, 'injected snapshot publication failure'); END`); err != nil {
		t.Fatal(err)
	}
	capture := func(query string, args ...any) []string {
		t.Helper()
		rows, err := f.db.SQL.QueryContext(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			out = append(out, fmt.Sprint(values))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	queries := []struct {
		name string
		sql  string
		args []any
	}{
		{"balances", `SELECT id,amount,projection_kind FROM account_values WHERE account_id IN (SELECT id FROM accounts WHERE household_id=?) ORDER BY id`, []any{f.boot.Household.ID.String()}},
		{"events", `SELECT id,amount,projection_kind FROM account_cash_values WHERE account_id=? ORDER BY id`, []any{f.accounts[0].Account.ID.String()}},
		{"current", `SELECT id,quantity,projection_kind FROM holding_quantity_values WHERE holding_id IN (SELECT id FROM holdings WHERE account_id IN (SELECT id FROM accounts WHERE household_id=?)) ORDER BY id`, []any{f.boot.Household.ID.String()}},
		{"holdings", `SELECT id,quantity FROM holdings WHERE account_id IN (SELECT id FROM accounts WHERE household_id=?) ORDER BY id`, []any{f.boot.Household.ID.String()}},
		{"snapshots", `SELECT id,local_date,net_worth_amount,input_generation FROM daily_valuation_snapshots WHERE household_id=? ORDER BY local_date,revision`, []any{f.boot.Household.ID.String()}},
		{"items", `SELECT i.id,i.snapshot_id,i.account_id,i.holding_id,i.native_amount,i.base_amount FROM daily_valuation_snapshot_items i JOIN daily_valuation_snapshots s ON s.id=i.snapshot_id WHERE s.household_id=? ORDER BY i.id`, []any{f.boot.Household.ID.String()}},
		{"state", `SELECT household_id,dirty_from,last_completed_closed_on,updated_at,dirty_to,input_generation,resolver_policy_version FROM history_snapshot_state WHERE household_id=?`, []any{f.boot.Household.ID.String()}},
		{"facts", `SELECT id,activity_id,sequence,target,quantity,amount,cost_unit_price FROM activity_effects WHERE activity_id IN (SELECT id FROM activities WHERE household_id=?) ORDER BY activity_id,sequence`, []any{f.boot.Household.ID.String()}},
	}
	before := make([][]string, len(queries))
	for i, query := range queries {
		before[i] = capture(query.sql, query.args...)
	}
	if _, err := f.service.RebuildDerivedData(ctx); err == nil {
		t.Fatal("injected publication failure was not returned")
	}
	for i, query := range queries {
		if after := capture(query.sql, query.args...); fmt.Sprint(after) != fmt.Sprint(before[i]) {
			t.Errorf("%s changed across failed rebuild:\nbefore=%v\nafter=%v", query.name, before[i], after)
		}
	}
}
