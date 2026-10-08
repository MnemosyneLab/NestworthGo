package analysis

import (
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestWarmWailsAnalysisPreservesGuardedPreview(t *testing.T) {
	for _, reader := range []string{"asset_change", "return_calendar"} {
		t.Run(reader, func(t *testing.T) {
			db, err := sqlite.Open(t.TempDir() + "/synthetic.db")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			app := application.NewService(sqlite.NewRepository(db))
			now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
			app.SetClock(func() time.Time { return now })
			if err := app.CompleteOnboarding(t.Context(), application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := app.Bootstrap(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			cash, err := app.CreateAccount(t.Context(), application.AccountInput{Name: "Synthetic cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "CNY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := app.StartHistory(t.Context(), "UTC"); err != nil {
				t.Fatal(err)
			}
			now = time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
			if _, err := app.RebuildHistoricalSnapshots(t.Context(), "2026-08-01", "2026-08-11"); err != nil {
				t.Fatal(err)
			}
			amount, err := domain.ParseMoney("1", "CNY")
			if err != nil {
				t.Fatal(err)
			}
			command := domain.MoneyAddedInput{HouseholdID: cash.Account.HouseholdID, AccountID: cash.Account.ID, Amount: amount, Reason: domain.ReasonIncome, EffectiveAt: now}
			_, token, err := app.PreviewChangeGuarded(t.Context(), command)
			if err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
				t.Fatal(err)
			}
			service := NewService(app)
			request := AnalysisQueryRequest{From: "2026-08-10", To: "2026-08-11"}
			if reader == "asset_change" {
				_, err = service.AssetChange(t.Context(), request)
			} else {
				_, err = service.ReturnCalendar(t.Context(), request, "2026-08", "day")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.SQL.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("warm Wails projection wrote snapshots", before, after)
			}
			_, err = app.RecordChangeGuarded(t.Context(), command, domain.NewMutationID().String(), strings.Repeat("ab", 32), token)
			if err != nil {
				t.Fatal("Wails projection invalidated original preview", err)
			}
		})
	}
}
