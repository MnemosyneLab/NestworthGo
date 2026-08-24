package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func mustBatchQuantity(t *testing.T, value string) domain.Quantity {
	t.Helper()
	quantity, err := domain.ParseQuantity(value)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}

func TestListHoldingsByAccountsReturnsHoldingsInOneQuery(t *testing.T) {
	database, repository, _, first, instrument := seedPortfolioRepository(t)
	ctx := context.Background()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)

	members, err := repository.ListMembers(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) == 0 {
		t.Fatal("seed member missing")
	}
	owner := members[0].ID

	secondAccount, ownership, _, err := domain.NewAccount(domain.AccountInput{HouseholdID: first.HouseholdID, Name: "Second", PrimaryCategory: domain.CategoryInvestment, SecondaryCategory: domain.SecondaryBrokerageAccount, TrackingMode: domain.TrackingHoldings, DefaultCurrency: domain.CurrencyCode("CNY"), Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateAccount(ctx, secondAccount, ownership, nil); err != nil {
		t.Fatalf("second account: %v", err)
	}
	otherInstrument, err := domain.NewInstrument(domain.InstrumentInput{HouseholdID: first.HouseholdID, Name: "SPY", Type: domain.InstrumentETF, QuoteCurrency: domain.CurrencyCode("USD")}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateInstrument(ctx, otherInstrument); err != nil {
		t.Fatal(err)
	}

	firstHolding, err := domain.NewHoldingForAccount(first, instrument, mustBatchQuantity(t, "3"), nil, 1, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, firstHolding); err != nil {
		t.Fatal(err)
	}
	secondHolding, err := domain.NewHoldingForAccount(secondAccount, otherInstrument, mustBatchQuantity(t, "1"), nil, 0, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateHolding(ctx, secondHolding); err != nil {
		t.Fatal(err)
	}

	holdings, err := repository.ListHoldingsByAccounts(ctx, []domain.AccountID{first.ID, secondAccount.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(holdings) != 2 {
		t.Fatalf("holdings = %d, want 2", len(holdings))
	}
	byAccount := map[domain.AccountID]domain.Holding{}
	for _, holding := range holdings {
		byAccount[holding.AccountID] = holding
	}
	if byAccount[first.ID].ID != firstHolding.ID || byAccount[secondAccount.ID].ID != secondHolding.ID {
		t.Fatalf("grouping mismatch: %+v", holdings)
	}

	empty, err := repository.ListHoldingsByAccounts(ctx, nil)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty batch = %+v, err=%v", empty, err)
	}
	_ = database
}

// The combined save+mark method must advance the completion marker in the same
// transaction that stores the snapshot; a failure in either step leaves both
// untouched.
func TestSaveDailyValuationSnapshotAndMarkCompletedAdvancesStateAtomically(t *testing.T) {
	database, repository, household, _, _ := seedPortfolioRepository(t)
	ctx := context.Background()
	localDate := "2026-08-22"
	cutoff := time.Date(2026, 8, 22, 23, 59, 59, 999000000, time.UTC)
	if _, err := database.SQL.ExecContext(ctx, `INSERT INTO history_snapshot_state(household_id, dirty_from, last_completed_closed_on, updated_at) VALUES(?, ?, NULL, ?)`, household.ID.String(), "2026-08-20", formatTimestamp(cutoff)); err != nil {
		t.Fatal(err)
	}
	snapshot := domain.DailyValuationSnapshot{
		ID:          domain.NewDailyValuationSnapshotID(),
		HouseholdID: household.ID,
		LocalDate:   localDate,
		CutoffAt:    cutoff,
		ContentHash: "hash-1",
		Currency:    domain.CurrencyCode("CNY"),
		Complete:    true,
		Revision:    1,
		CreatedAt:   cutoff,
	}

	appended, err := repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if !appended {
		t.Fatal("first snapshot was skipped")
	}

	state, err := repository.DailySnapshotState(ctx, household.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.LastCompletedClosedOn == nil || *state.LastCompletedClosedOn != localDate {
		t.Fatalf("last completed = %v, want %s", state.LastCompletedClosedOn, localDate)
	}
	if state.DirtyFrom == nil || *state.DirtyFrom != "2026-08-20" {
		t.Fatalf("dirty from = %v, want 2026-08-20", state.DirtyFrom)
	}

	appended, err = repository.SaveDailyValuationSnapshotAndMarkCompleted(ctx, snapshot, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if appended {
		t.Fatal("identical snapshot was stored twice")
	}
	var revisions int
	if err := database.SQL.QueryRowContext(ctx, `SELECT COUNT(*) FROM daily_valuation_snapshots WHERE household_id = ? AND local_date = ?`, household.ID.String(), localDate).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 {
		t.Fatalf("snapshot revisions = %d, want 1", revisions)
	}
}
