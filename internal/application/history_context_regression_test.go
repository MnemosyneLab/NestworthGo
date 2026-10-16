package application

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestCorrectionTransferFeeIntent(t *testing.T) {
	for _, destinationCurrency := range []domain.CurrencyCode{"USD", "EUR"} {
		t.Run(destinationCurrency.String(), func(t *testing.T) {
			for _, tc := range []struct{ name, fee, want string }{{"omitted", "", "895"}, {"zero", "0", "900"}, {"changed", "7", "893"}} {
				t.Run(tc.name, func(t *testing.T) {
					ctx := context.Background()
					db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
					if err != nil {
						t.Fatal(err)
					}
					defer db.Close()
					repo := sqlite.NewRepository(db)
					s := NewService(repo)
					now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
					s.setClock(func() time.Time { return now })
					if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
						t.Fatal(err)
					}
					boot, _ := s.Bootstrap(ctx)
					create := func(name, amount string) domain.AccountID {
						a, e := s.CreateAccount(ctx, AccountInput{Name: name, AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: map[string]string{"A": "USD", "B": destinationCurrency.String()}[name], InitialAmount: amount, IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
						if e != nil {
							t.Fatal(e)
						}
						return a.Account.ID
					}
					a, b := create("A", "1000"), create("B", "0")
					if _, err := s.StartHistory(ctx, "UTC"); err != nil {
						t.Fatal(err)
					}
					now = now.Add(time.Hour)
					fee := mustMoney(t, "5", "USD")
					cmd := domain.CashTransferInput{HouseholdID: boot.Household.ID, FromAccountID: a, ToAccountID: b, Sent: mustMoney(t, "100", "USD"), Received: mustMoney(t, "100", destinationCurrency.String()), Fee: &fee, EffectiveAt: now}
					original, err := s.RecordChange(ctx, cmd)
					if err != nil {
						t.Fatal(err)
					}
					now = now.Add(time.Hour)
					note := "Only note"
					cmd.Note = &note
					cmd.Fee = nil
					if tc.fee != "" {
						f := mustMoney(t, tc.fee, "USD")
						cmd.Fee = &f
					}
					preview, err := s.PreviewFixChange(ctx, original.Activity.ID, cmd)
					if err != nil {
						t.Fatal(err)
					}
					if preview.Resulting[0].Amount != tc.want {
						t.Fatalf("preview source=%s want %s", preview.Resulting[0].Amount, tc.want)
					}
					// Preview/cancel is read-only.
					before, _ := repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
					if len(before) != 1 {
						t.Fatal("preview wrote history")
					}
					key := domain.NewActivityID().String()
					hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
					fixed, err := s.FixChangeWithMutation(ctx, original.Activity.ID, cmd, key, hash)
					if err != nil {
						t.Fatal(err)
					}
					if fixed.Resulting[0].Amount != tc.want {
						t.Fatal("preview/commit mismatch")
					}
					retry, err := s.FixChangeWithMutation(ctx, original.Activity.ID, cmd, key, hash)
					if err != nil || retry.Activity.ID != fixed.Activity.ID {
						t.Fatalf("retry: %v", err)
					}
					snapshot, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{})
					if err != nil {
						t.Fatal(err)
					}
					for _, r := range snapshot.Accounts {
						if r.Account.ID == a && r.LatestValue.Amount.CanonicalAmount() != tc.want {
							t.Fatal("wrong persisted source balance")
						}
						if r.Account.ID == b && r.LatestValue.Amount.CanonicalAmount() != "100" {
							t.Fatal("wrong persisted destination balance")
						}
					}
				})
			}

		})
	}
}

func TestCorrectionSoldOutArchivedPosition(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "closed", true: "archived"}[archived], func(t *testing.T) {
			ctx := context.Background()
			db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			repo := sqlite.NewRepository(db)
			s := NewService(repo)
			now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
			s.setClock(func() time.Time { return now })
			if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			boot, _ := s.Bootstrap(ctx)
			account, err := s.CreateAccount(ctx, AccountInput{Name: "Broker", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, IncludeInPortfolio: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.AppendAccountCashValue(ctx, account.Account.ID, "1000", "USD", "2026-09-25"); err != nil {
				t.Fatal(err)
			}
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic Fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Hour)
			cmd := domain.TradeInput{HouseholdID: boot.Household.ID, Side: domain.TradeBuy, SettlementAccountID: account.Account.ID, HoldingID: holding.ID, InstrumentID: instrument.ID, Quantity: mustQuantity(t, "10"), Gross: mustMoney(t, "100", "USD"), EffectiveAt: now}
			if _, err := s.RecordChange(ctx, cmd); err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Hour)
			cmd.Side = domain.TradeSell
			cmd.EffectiveAt = now
			sell, err := s.RecordChange(ctx, cmd)
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Hour)
			if archived {
				if err := s.ArchiveHolding(ctx, holding.ID, true); err != nil {
					t.Fatal(err)
				}
				if err := s.ArchiveInstrument(ctx, instrument.ID, true); err != nil {
					t.Fatal(err)
				}
				if err := s.ArchiveAccount(ctx, account.Account.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			note := "Only note"
			cmd.Note = &note
			preview, err := s.PreviewFixChange(ctx, sell.Activity.ID, cmd)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := repo.ListActivitiesUntil(ctx, boot.Household.ID, now)
			if len(before) != 2 {
				t.Fatal("preview wrote history")
			}
			fixed, err := s.FixChange(ctx, sell.Activity.ID, cmd)
			if err != nil {
				t.Fatal(err)
			}
			if preview.Resulting[0].Amount != fixed.Resulting[0].Amount {
				t.Fatal("preview/commit mismatch")
			}
			snapshot, err := repo.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
			if err != nil {
				t.Fatal(err)
			}
			if snapshot.Holdings[0].Quantity.Canonical() != "0" {
				t.Fatal("position reopened")
			}
			if (snapshot.Holdings[0].ArchivedAt != nil) != archived {
				t.Fatal("archive metadata changed")
			}
			if _, err := s.PreviewChange(ctx, cmd); err == nil {
				t.Fatal("new sale should still fail")
			}
		})
	}
}
