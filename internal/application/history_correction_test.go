package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalCorrectionReplaysSoldPositionAndAllSnapshots(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/correction.db")
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
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "2000", "CNY", "2026-09-25"); err != nil {
		t.Fatal(err)
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Fund", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendManualInstrumentQuote(ctx, instrument.ID, "1.0175", "2026-09-25T12:00:00Z", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	command := domain.TradeInput{HouseholdID: boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "1000"), Gross: mustMoney(t, "101.75", "CNY"), EffectiveAt: now}
	buy, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	sellCommand := command
	sellCommand.Side = domain.TradeSell
	sellCommand.Quantity = mustQuantity(t, "400")
	sellCommand.Gross = mustMoney(t, "407", "CNY")
	sellCommand.EffectiveAt = now
	sell, err := s.RecordChange(ctx, sellCommand)
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 2)
	if _, err := s.RebuildHistoricalSnapshots(ctx, "2026-09-25", "2026-09-28"); err != nil {
		t.Fatal(err)
	}
	before, err := repo.ListDailyValuationSnapshots(ctx, boot.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	query := analysisBaseQuery(domain.ValuationBase)
	query.From = "2026-09-26"
	query.To = "2026-09-28"
	oldAnalysis, err := s.Analyze(ctx, query)
	if err != nil || oldAnalysis.ReturnAmount == nil || oldAnalysis.ReturnAmount.CanonicalAmount() != "915.75" {
		t.Fatalf("expected original inflated return: %+v %v", oldAnalysis.ReturnAmount, err)
	}
	command.Gross = mustMoney(t, "1017.50", "CNY")
	command.EffectiveAt = now
	mutationID := domain.NewActivityID().String()
	payloadHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	fixed, err := s.FixChangeWithMutation(ctx, buy.Activity.ID, command, mutationID, payloadHash)
	if err != nil {
		t.Fatal(err)
	}
	if !fixed.Activity.EffectiveAt.Equal(buy.Activity.EffectiveAt) || !fixed.Activity.CreatedAt.Equal(now) {
		t.Fatal("lost effective/audit time distinction")
	}
	gain, err := s.HoldingGain(ctx, holding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !gain.Available || gain.Quantity != "600" || gain.TotalCost.Amount != "610.5" || gain.RealizedGain.Amount != "0" {
		t.Fatalf("wrong corrected gain: %+v", gain)
	}
	after, err := repo.ListDailyValuationSnapshots(ctx, boot.Household.ID, time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 4 {
		t.Fatalf("snapshots=%d", len(after))
	}
	for _, snapshot := range after {
		if snapshot.NetWorthAmount == nil || snapshot.NetWorthAmount.CanonicalAmount() != "2000" {
			t.Fatalf("wrong snapshot %s: %+v", snapshot.LocalDate, snapshot.NetWorthAmount)
		}
	}
	if before[0].ID != after[0].ID {
		t.Fatal("unaffected earlier snapshot changed")
	}
	state, err := s.DailySnapshotState(ctx, boot.Household.ID)
	if err != nil || state.DirtyFrom != nil {
		t.Fatal("rebuild not completed", err)
	}
	var saleCash string
	if err := db.SQL.QueryRow(`SELECT amount FROM account_cash_values WHERE projection_kind='event' AND activity_effect_id IN(SELECT id FROM activity_effects WHERE activity_id=?)`, sell.Activity.ID.String()).Scan(&saleCash); err != nil {
		t.Fatal(err)
	}
	if saleCash != "1389.5" {
		t.Fatalf("later projection=%s", saleCash)
	}
	historical, err := repo.ListActivitiesUntil(ctx, boot.Household.ID, buy.Activity.EffectiveAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(historical) != 1 || historical[0].ID != fixed.Activity.ID {
		t.Fatal("old cutoff did not see replacement")
	}
	corrected, err := s.Analyze(ctx, query)
	if err != nil || corrected.ReturnAmount == nil || !corrected.ReturnAmount.Amount().IsZero() {
		t.Fatalf("stale analysis after correction: %+v %v", corrected.ReturnAmount, err)
	}
	retry, err := s.FixChangeWithMutation(ctx, buy.Activity.ID, command, mutationID, payloadHash)
	if err != nil || retry.Activity.ID != fixed.Activity.ID || len(retry.Resulting) != len(fixed.Resulting) {
		t.Fatal("correction retry was not idempotent", err)
	}
	// Emulate an existing correction written by the old version at today's date.
	if _, err := db.SQL.Exec(`UPDATE activities SET effective_at=?,effective_local_date='2026-09-29' WHERE correction_group_id=?`, now.Format(time.RFC3339Nano), fixed.Activity.CorrectionGroupID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`UPDATE daily_valuation_snapshots SET content_hash='v2:' || content_hash`); err != nil {
		t.Fatal(err)
	}
	restarted := NewService(repo)
	restarted.setClock(func() time.Time { return now })
	restoredAnalysis, err := restarted.Analyze(ctx, query)
	if err != nil || restoredAnalysis.ReturnAmount == nil || !restoredAnalysis.ReturnAmount.Amount().IsZero() {
		t.Fatalf("legacy correction was not rebuilt: %+v %v", restoredAnalysis.ReturnAmount, err)
	}
	// A second fix follows the original root, not the first correction's creation day.
	command.Gross = mustMoney(t, "1000", "CNY")
	second, err := s.FixChange(ctx, fixed.Activity.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Activity.EffectiveAt.Equal(buy.Activity.EffectiveAt) {
		t.Fatal("correction chain moved date")
	}
	// Reducing the purchase below the subsequent sale must fail without writes.
	command.Quantity = mustQuantity(t, "100")
	var countBefore, countAfter int
	db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&countBefore)
	if _, err := s.FixChange(ctx, second.Activity.ID, command); err == nil {
		t.Fatal("invalid later sell was accepted")
	}
	db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&countAfter)
	if countBefore != countAfter {
		t.Fatal("failed correction wrote audit records")
	}
}

func TestCorrectionPreservesLaterAbsoluteBalanceObservation(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(t.TempDir() + "/balances.db")
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
	account, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	command := domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: now}
	original, err := s.RecordChange(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	observation, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, NewValue: mustMoney(t, "1200", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: now})
	if err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 1)
	command.Amount = mustMoney(t, "80", "CNY")
	corrected, err := s.FixChange(ctx, original.Activity.ID, command)
	if err != nil {
		t.Fatal(err)
	}
	if corrected.Resulting[0].Amount != "1080" {
		t.Fatalf("historical preview incorrect: %+v", corrected.Resulting)
	}
	live, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil || live.NetWorth.String() != "1200" {
		t.Fatal("later absolute balance moved", err)
	}
	events, err := repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[1].Effects[0].Money.CanonicalAmount() != "120" {
		t.Fatalf("observation delta not recalculated: %+v", events)
	}
	raw, err := repo.Activity(ctx, boot.Household.ID, observation.Activity.ID)
	if err != nil || raw.Effects[0].Money.CanonicalAmount() != "100" {
		t.Fatal("audit effect was overwritten", err)
	}
	snapshot, _, err := s.BuildDailyValuationSnapshot(ctx, "2026-09-26")
	if err != nil || snapshot.NetWorthAmount.CanonicalAmount() != "1080" {
		t.Fatal("intermediate snapshot incorrect", err)
	}
	snapshot, _, err = s.BuildDailyValuationSnapshot(ctx, "2026-09-27")
	if err != nil || snapshot.NetWorthAmount.CanonicalAmount() != "1200" {
		t.Fatal("reconciled snapshot incorrect", err)
	}
}
