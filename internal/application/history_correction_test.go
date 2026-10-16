package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestHistoricalCorrectionReplaysSoldPositionAndAllSnapshots(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestCorrectionZeroDeltaObservation(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "active", true: "archived"}[archived], func(t *testing.T) {
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
			other, err := s.CreateAccount(ctx, AccountInput{Name: "Other", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1200", OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if err := s.ArchiveAccount(ctx, other.Account.ID, true); err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}

			now = now.AddDate(0, 0, 1)
			cmd := domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, NewValue: mustMoney(t, "1100", "CNY"), EffectiveAt: now}
			first, err := s.RecordChange(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			now = now.AddDate(0, 0, 1)
			cmd.NewValue = mustMoney(t, "1200", "CNY")
			cmd.EffectiveAt = now
			second, err := s.RecordChange(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			now = now.AddDate(0, 0, 1)
			correctedFirst, err := s.FixChange(ctx, first.Activity.ID, cmd)
			if err != nil {
				t.Fatal(err)
			}
			events, err := repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
			if err != nil || len(events) != 2 || len(events[1].Effects) != 0 || len(events[1].RecordedEffects) != 1 {
				t.Fatalf("replay: %+v %v", events, err)
			}
			// Ordinary recording must still reject a zero delta.
			if _, err = s.RecordChange(ctx, cmd); err == nil {
				t.Fatal("new zero-delta record accepted")
			} else {
				var domainErr *domain.Error
				if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrNoChange {
					t.Fatalf("new unchanged value error: %v", err)
				}
			}
			// A later transaction must survive the metadata-only correction.
			later, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: account.Account.ID, Amount: mustMoney(t, "50", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: now})
			if err != nil {
				t.Fatal(err)
			}
			if archived {
				if err = s.ArchiveAccount(ctx, account.Account.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			note := "corrected note"
			cmd.Note = &note
			invalid := cmd
			invalid.AccountID = other.Account.ID
			if _, err = s.PreviewFixChange(ctx, second.Activity.ID, invalid); err == nil {
				t.Fatal("unrelated archived endpoint accepted")
			}
			var beforeCount, afterCount int
			if err = db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&beforeCount); err != nil {
				t.Fatal(err)
			}
			if _, err = s.FixChange(ctx, second.Activity.ID, invalid); err == nil {
				t.Fatal("unrelated archived endpoint committed")
			}
			preview, err := s.PreviewFixChange(ctx, second.Activity.ID, cmd)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Resulting) != 1 || preview.Resulting[0].Amount != "1200" {
				t.Fatalf("preview: %+v", preview)
			}
			if err = db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&afterCount); err != nil || afterCount != beforeCount {
				t.Fatal("preview/refused correction wrote history", err)
			}
			key := domain.NewActivityID().String()
			hash := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			fixed, err := s.FixChangeWithMutation(ctx, second.Activity.ID, cmd, key, hash)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := s.FixChangeWithMutation(ctx, second.Activity.ID, cmd, key, hash)
			if err != nil || retry.Activity.ID != fixed.Activity.ID {
				t.Fatalf("retry %v", err)
			}
			if err = db.SQL.QueryRow("SELECT COUNT(*) FROM activities").Scan(&afterCount); err != nil || afterCount != beforeCount+2 {
				t.Fatal("retry duplicated history", err)
			}
			var isArchived bool
			if err = db.SQL.QueryRow("SELECT archived_at IS NOT NULL FROM accounts WHERE id=?", account.Account.ID.String()).Scan(&isArchived); err != nil || isArchived != archived {
				t.Fatal("archive metadata changed", err)
			}
			events, err = repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
			if err != nil || len(events) != 3 || events[1].Note == nil || *events[1].Note != note || len(events[1].Effects) != 0 || events[2].ID != later.Activity.ID {
				t.Fatalf("corrected replay: %+v %v", events, err)
			}
			var amount string
			if err = db.SQL.QueryRow(`SELECT amount FROM account_values WHERE projection_kind='event' AND activity_effect_id IN (SELECT id FROM activity_effects WHERE activity_id=?)`, later.Activity.ID.String()).Scan(&amount); err != nil || amount != "1250" {
				t.Fatalf("later balance %s %v", amount, err)
			}
			// Re-correcting a zero-effect replacement retains endpoint evidence.
			note = "second note"
			if _, err = s.FixChange(ctx, fixed.Activity.ID, cmd); err != nil {
				t.Fatal(err)
			}
			// A later correction of the preceding observation must turn the
			// stored zero delta back into 100 without moving the 1200 target.
			cmd.NewValue = mustMoney(t, "1100", "CNY")
			if _, err = s.FixChange(ctx, correctedFirst.Activity.ID, cmd); err != nil {
				t.Fatal(err)
			}
			events, err = repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
			if err != nil || len(events) != 3 || len(events[1].Effects) != 1 || events[1].Effects[0].Money.CanonicalAmount() != "100" || events[1].Resulting[0].Amount != "1200" {
				t.Fatalf("zero fact replay: %+v %v", events, err)
			}
		})
	}
}
