package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestPostOriginZeroBalanceSurvivesHistoricalReplay(t *testing.T) {
	for _, initial := range []string{"0", "50", ""} {
		t.Run("initial="+initial, func(t *testing.T) {
			db, err := sqlite.Open(t.TempDir() + "/zero.db")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := sqlite.NewRepository(db)
			s := NewService(repo)
			ctx := context.Background()
			now := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
			s.setClock(func() time.Time { return now })
			if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := s.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "Asia/Singapore"); err != nil {
				t.Fatal(err)
			}
			now = now.Add(24 * time.Hour)
			inputAmount := initial
			if inputAmount == "" {
				inputAmount = "0"
			}
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Card", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: inputAmount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if initial == "" {
				// Simulate legacy missing evidence that the current API cannot create.
				if _, err := db.SQL.ExecContext(ctx, "DELETE FROM account_values WHERE account_id = ?", account.Account.ID.String()); err != nil {
					t.Fatal(err)
				}
			}
			now = now.Add(24 * time.Hour)
			if _, err := s.AppendAccountValue(ctx, account.Account.ID, "1000", ""); err != nil {
				t.Fatal(err)
			}
			now = now.Add(24 * time.Hour)
			origin, err := s.HistoryOrigin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			batch, err := repo.LoadHistoricalSnapshotBatch(ctx, origin.HouseholdID, now)
			if err != nil {
				t.Fatal(err)
			}
			replay := HistoricalReplay{repository: repo, batch: &batch}
			for _, tc := range []struct {
				day    int
				want   string
				exists bool
			}{{27, "", false}, {28, initial, true}, {29, "1000", true}} {
				cutoff := time.Date(2026, 9, tc.day, 15, 59, 59, 999000000, time.UTC)
				snapshot, err := replay.Snapshot(ctx, origin, cutoff)
				if err != nil {
					t.Fatal(err)
				}
				record, exists := accountRecordByID(snapshot.Accounts, account.Account.ID)
				if exists != tc.exists {
					t.Fatalf("day %d: exists=%v", tc.day, exists)
				}
				if !exists {
					continue
				}
				if tc.want == "" {
					if record.LatestValue != nil {
						t.Fatal("unknown balance became zero")
					}
					continue
				}
				if record.LatestValue == nil || record.LatestValue.Amount.CanonicalAmount() != tc.want {
					t.Fatalf("day %d: value=%+v want %s", tc.day, record.LatestValue, tc.want)
				}
			}
			snapshot, _, err := s.BuildDailyValuationSnapshot(ctx, "2026-09-28")
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Complete != (initial != "") {
				t.Fatalf("complete=%v initial=%q", snapshot.Complete, initial)
			}
			if initial != "" && (snapshot.LiabilitiesAmount == nil || snapshot.LiabilitiesAmount.CanonicalAmount() != initial) {
				t.Fatalf("liabilities=%+v", snapshot.LiabilitiesAmount)
			}
		})
	}
}
