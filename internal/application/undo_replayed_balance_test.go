package application

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestUndoBalanceAndCashAfterBackfill(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"balance", "holdings"} {
		for _, adjustment := range []string{"1200", "1000"} {
			for _, backfill := range []string{"80", "-80", "180", "100"} {
				t.Run(mode+"/target="+adjustment+"/backfill="+backfill, func(t *testing.T) {
					ctx, s, repo, db, boot, setNow := historicalInsertionFixture(t)
					kind, initial := "bank_account", "1000"
					if mode == "holdings" {
						kind, initial = "brokerage", ""
					}
					a, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: kind, BalanceSheetRole: "asset", TrackingMode: mode, DefaultCurrency: "CNY", InitialAmount: initial, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
					if err != nil {
						t.Fatal(err)
					}
					if mode == "holdings" {
						if _, err := s.AppendAccountCashValue(ctx, a.Account.ID, "1000", "CNY", "2026-09-25"); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := s.StartHistory(ctx, "UTC"); err != nil {
						t.Fatal(err)
					}
					setNow(historicalInsertionTime(29))
					if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(26)}); err != nil {
						t.Fatal(err)
					}
					var adjustmentCommand any = domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, NewValue: mustMoney(t, adjustment, "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)}
					if mode == "holdings" {
						// Cash reconciliation records a relative delta, rather than a balance
						// observation. It must continue to undo that delta after a backfill.
						adjustmentCommand = domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)}
						if adjustment == "1000" {
							adjustmentCommand = domain.MoneyRemovedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)}
						}
					}
					adjusted, err := s.RecordChange(ctx, adjustmentCommand)
					if err != nil {
						t.Fatal(err)
					}
					delta := decimal.RequireFromString(backfill)
					var command any = domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, delta.Abs().String(), "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(27)}
					if delta.IsNegative() {
						command = domain.MoneyRemovedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, delta.Abs().String(), "CNY"), Reason: domain.ReasonExpense, EffectiveAt: historicalInsertionTime(27)}
					}
					if _, err := s.RecordChange(ctx, command); err != nil {
						t.Fatal(err)
					}
					want := decimal.NewFromInt(1100).Add(delta).String()
					preview, token, err := s.PreviewUndoChangeGuarded(ctx, adjusted.Activity.ID)
					if err != nil {
						t.Fatal(err)
					}
					if len(preview.Resulting) != 1 || preview.Resulting[0].Amount != want {
						t.Fatalf("preview=%+v want %s", preview.Resulting, want)
					}
					mutation, hash := domain.NewMutationID().String(), strings.Repeat("ab", 32)
					undone, err := s.RecordUndoChangeGuardedAt(ctx, adjusted.Activity.ID, preview.Activity.EffectiveAt, mutation, hash, token)
					if err != nil {
						t.Fatal(err)
					}
					if len(undone.Resulting) != 1 || undone.Resulting[0].Amount != want {
						t.Fatalf("commit=%+v want %s", undone.Resulting, want)
					}
					assertBalance := func(service *Service, repository *sqlite.Repository, expected string) {
						t.Helper()
						current, err := service.Overview(ctx, domain.AccountFilter{})
						if err != nil || current.NetWorth.String() != expected {
							t.Fatalf("current=%s want %s err=%v", current.NetWorth, expected, err)
						}
						origin, err := service.HistoryOrigin(ctx)
						if err != nil {
							t.Fatal(err)
						}
						replay, err := (HistoricalReplay{repository: repository}).Snapshot(ctx, origin, service.clock())
						if err != nil {
							t.Fatal(err)
						}
						if mode == "balance" {
							record, ok := accountRecordByID(replay.Accounts, a.Account.ID)
							if !ok || record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != expected {
								t.Fatalf("replay=%+v want %s", record.LatestValue, expected)
							}
						} else {
							if len(replay.CashValues) != 1 || replay.CashValues[0].Amount.CanonicalAmount() != expected {
								t.Fatalf("cash replay=%+v want %s", replay.CashValues, expected)
							}
						}
					}
					assertBalance(s, repo, want)
					original, err := repo.Activity(ctx, boot.Household.ID, adjusted.Activity.ID)
					if err != nil || len(original.Effects) != 1 || original.Effects[0].Money.CanonicalAmount() != "100" || original.Effects[0].Direction != adjusted.Effects[0].Direction {
						t.Fatalf("immutable original changed: %+v %v", original, err)
					}
					for range 2 {
						if _, err := s.RebuildDerivedData(ctx); err != nil {
							t.Fatal(err)
						}
						assertBalance(s, repo, want)
					}
					path := db.Path
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					reopened, err := sqlite.Open(path)
					if err != nil {
						t.Fatal(err)
					}
					defer reopened.Close()
					restartedRepo := sqlite.NewRepository(reopened)
					restarted := NewService(restartedRepo)
					restarted.setClock(func() time.Time { return historicalInsertionTime(29) })
					replayed, err := restarted.RecordUndoChangeGuardedAt(ctx, adjusted.Activity.ID, preview.Activity.EffectiveAt, mutation, hash, token)
					if err != nil || replayed.Activity.ID != undone.Activity.ID || len(replayed.Resulting) != 1 || replayed.Resulting[0].Amount != want {
						t.Fatalf("durable retry=%+v %v", replayed, err)
					}
					assertBalance(restarted, restartedRepo, want)
					restarted.setClock(func() time.Time { return historicalInsertionTime(30) })
					if _, err := restarted.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, "1", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(27).Add(time.Minute)}); err != nil {
						t.Fatal(err)
					}
					assertBalance(restarted, restartedRepo, decimal.RequireFromString(want).Add(decimal.NewFromInt(1)).String())
				})
			}
		}
	}
}

func TestUndoReplayedBalanceCommitIsAtomic(t *testing.T) {
	t.Parallel()
	ctx, s, repo, db, boot, setNow := historicalInsertionFixture(t)
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	setNow(historicalInsertionTime(29))
	income := domain.MoneyAddedInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, Amount: mustMoney(t, "100", "CNY"), Reason: domain.ReasonIncome, EffectiveAt: historicalInsertionTime(26)}
	if _, err := s.RecordChange(ctx, income); err != nil {
		t.Fatal(err)
	}
	adjusted, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, NewValue: mustMoney(t, "1200", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)})
	if err != nil {
		t.Fatal(err)
	}
	income.Amount, income.EffectiveAt = mustMoney(t, "80", "CNY"), historicalInsertionTime(27)
	if _, err := s.RecordChange(ctx, income); err != nil {
		t.Fatal(err)
	}
	_, token, err := s.PreviewUndoChangeGuarded(ctx, adjusted.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.ExecContext(ctx, `CREATE TRIGGER fail_undo_receipt BEFORE INSERT ON activity_mutation_keys BEGIN SELECT RAISE(ABORT,'synthetic receipt failure'); END`); err != nil {
		t.Fatal(err)
	}
	mutation, hash := domain.NewMutationID().String(), strings.Repeat("ab", 32)
	if _, err := s.RecordUndoChangeGuarded(ctx, adjusted.Activity.ID, mutation, hash, token); err == nil {
		t.Fatal("receipt failure accepted")
	}
	current, err := s.Overview(ctx, domain.AccountFilter{})
	if err != nil || current.NetWorth.String() != "1200" || activityCount(t, db) != 3 {
		t.Fatalf("partial undo publication: value=%s activities=%d err=%v", current.NetWorth, activityCount(t, db), err)
	}
	hasUndo, err := repo.ActivityHasReversal(ctx, boot.Household.ID, adjusted.Activity.ID)
	if err != nil || hasUndo {
		t.Fatalf("failed undo retained reversal: %v %v", hasUndo, err)
	}
	if _, err := db.SQL.ExecContext(ctx, `DROP TRIGGER fail_undo_receipt`); err != nil {
		t.Fatal(err)
	}
	_, token, err = s.PreviewUndoChangeGuarded(ctx, adjusted.Activity.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordUndoChangeGuarded(ctx, adjusted.Activity.ID, mutation, hash, token); err != nil {
		t.Fatal(err)
	}
	current, err = s.Overview(ctx, domain.AccountFilter{})
	if err != nil || current.NetWorth.String() != "1180" || activityCount(t, db) != 4 {
		t.Fatalf("retry: %+v %v", current, err)
	}
}

func TestHistoricalUndoPreservesLaterAbsoluteObservation(t *testing.T) {
	t.Parallel()
	for _, earlier := range []bool{false, true} {
		t.Run(map[bool]string{false: "current", true: "before_later_observation"}[earlier], func(t *testing.T) {
			ctx, s, repo, db, boot, setNow := historicalInsertionFixture(t)
			a, err := s.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			setNow(historicalInsertionTime(30))
			first, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, NewValue: mustMoney(t, "1200", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(26)})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.RecordChange(ctx, domain.ValueUpdateInput{HouseholdID: boot.Household.ID, AccountID: a.Account.ID, NewValue: mustMoney(t, "1500", "CNY"), Reason: domain.ReasonReconciliation, EffectiveAt: historicalInsertionTime(28)}); err != nil {
				t.Fatal(err)
			}
			at, want, wantAt := historicalInsertionTime(30), "1300", "1300"
			if earlier {
				at, want, wantAt = historicalInsertionTime(27), "1500", "1000"
			}
			preview, token, err := s.PreviewUndoChangeGuardedAt(ctx, first.Activity.ID, at)
			if err != nil {
				t.Fatal(err)
			}
			if len(preview.Resulting) != 1 || preview.Resulting[0].Amount != wantAt {
				t.Fatalf("effective-time preview=%+v want %s", preview.Resulting, wantAt)
			}
			mutation, hash := domain.NewMutationID().String(), strings.Repeat("ab", 32)
			committed, err := s.RecordUndoChangeGuardedAt(ctx, first.Activity.ID, at, mutation, hash, token)
			if err != nil || len(committed.Resulting) != 1 || committed.Resulting[0].Amount != wantAt {
				t.Fatalf("commit=%+v err=%v", committed, err)
			}
			for range 2 {
				current, err := s.Overview(ctx, domain.AccountFilter{})
				if err != nil || current.NetWorth.String() != want {
					t.Fatalf("current=%s want %s err=%v", current.NetWorth, want, err)
				}
				origin, err := s.HistoryOrigin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				replay, err := (HistoricalReplay{repository: repo}).Snapshot(ctx, origin, s.clock())
				if err != nil {
					t.Fatal(err)
				}
				record, ok := accountRecordByID(replay.Accounts, a.Account.ID)
				if !ok || record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != want {
					t.Fatalf("replay=%+v", record.LatestValue)
				}
				if _, err := s.RebuildDerivedData(ctx); err != nil {
					t.Fatal(err)
				}
			}
			restarted := NewService(sqlite.NewRepository(db))
			restarted.setClock(func() time.Time { return historicalInsertionTime(30) })
			retry, err := restarted.RecordUndoChangeGuardedAt(ctx, first.Activity.ID, at, mutation, hash, token)
			if err != nil || retry.Activity.ID != committed.Activity.ID || len(retry.Resulting) != 1 || retry.Resulting[0].Amount != wantAt {
				t.Fatalf("retry=%+v err=%v", retry, err)
			}
			if _, _, err := s.PreviewUndoChangeGuardedAt(ctx, first.Activity.ID, historicalInsertionTime(25)); !hasDomainCode(err, domain.ErrAlreadyUndone) {
				t.Fatalf("repeat undo=%v", err)
			}
		})
	}
}
