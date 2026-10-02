package history_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/history"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	"github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

// Verify the exact-default and manual-minute contracts through real Wails APIs
// and temp-file SQLite. Frontend tests cover choosing these request shapes.
func TestExactDefaultTimeAndManualMinuteBoundaries(t *testing.T) {
	for _, fixture := range []struct{ zone, origin string }{
		{"UTC", "2026-10-02T12:34:45.000Z"}, {"Asia/Shanghai", "2026-10-02T12:34:45.000Z"},
		{"UTC", "2026-10-02T12:34:00.123Z"}, {"Asia/Shanghai", "2026-10-02T12:34:00.123Z"},
	} {
		for _, kind := range []history.ChangeCommandKind{history.ChangeMoneyAdded, history.ChangePositionTransfer} {
			t.Run(fixture.zone+"/"+fixture.origin+"/"+string(kind), func(t *testing.T) {
				zone := fixture.zone
				ctx := context.Background()
				app := wailstest.NewService(t)
				now, err := time.Parse(time.RFC3339Nano, fixture.origin)
				if err != nil {
					t.Fatal(err)
				}
				app.SetClock(func() time.Time { return now })
				hs := household.NewService(app)
				if err := hs.CompleteOnboarding(ctx, household.CompleteOnboardingRequest{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
					t.Fatal(err)
				}
				bootstrap, err := hs.Bootstrap(ctx)
				if err != nil {
					t.Fatal(err)
				}
				create := func(name, kind, mode, amount string) string {
					t.Helper()
					a, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{Name: name, AccountType: kind, BalanceSheetRole: "asset", TrackingMode: mode, DefaultCurrency: "USD", OwnerIDs: []string{bootstrap.Members[0].ID}, InitialAmount: amount, IncludeInNetWorth: true})
					if err != nil {
						t.Fatal(err)
					}
					return a.Account.ID
				}
				cash := create("Cash", "bank_account", "balance", "100")
				source := create("Source", "brokerage", "holdings", "")
				destination := create("Destination", "brokerage", "holdings", "")
				inst, err := instrument.NewService(app).CreateInstrument(ctx, instrument.InstrumentRequest{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				sourceHolding, err := holding.NewService(app).CreateHolding(ctx, holding.CreateHoldingRequest{AccountID: source, InstrumentID: inst.ID, Quantity: "10", UnitCost: "10"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := quote.NewService(app).AppendManualInstrumentQuote(ctx, inst.ID, "10", now.Format(time.RFC3339), false); err != nil {
					t.Fatal(err)
				}
				api := history.NewService(app)
				origin, err := api.StartHistory(ctx, zone)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("origin=%s", origin.StartedAt)
				location, _ := time.LoadLocation(zone)
				date, minute := now.In(location).Format("2006-01-02"), now.In(location).Format("15:04")
				requests := []history.ChangeCommandRequest{
					{Kind: history.ChangeMoneyAdded, AccountID: cash, Amount: "1", Currency: "USD", Reason: "contribution"},
					{Kind: history.ChangePositionTransfer, FromHoldingID: sourceHolding.ID, ToAccountID: destination, Quantity: "1"},
				}
				for _, base := range requests {
					if base.Kind != kind {
						continue
					}
					t.Run(string(base.Kind), func(t *testing.T) {
						now = time.Date(2026, 10, 2, 12, 34, 50, 789000000, time.UTC)
						exact := base
						exact.EffectiveAt = now.Format(time.RFC3339Nano)
						preview, err := api.PreviewChange(ctx, exact)
						if err != nil {
							t.Fatalf("immediate exact preview: %v", err)
						}
						// Advancing across a minute between preview and commit cannot drift the command.
						now = now.Add(time.Minute)
						recorded, err := api.RecordChange(ctx, exact)
						if err != nil {
							t.Fatalf("immediate exact commit: %v", err)
						}
						stored, err := api.Activity(ctx, recorded.Activity.ID)
						if err != nil {
							t.Fatal(err)
						}
						if stored.EffectiveAt != exact.EffectiveAt || preview.Activity.EffectiveAt != exact.EffectiveAt {
							t.Fatalf("effective timestamp changed: command=%s preview=%s persisted=%s", exact.EffectiveAt, preview.Activity.EffectiveAt, stored.EffectiveAt)
						}
						t.Logf("exact timestamp preserved across Preview, delayed Confirm and SQLite reload: %s", stored.EffectiveAt)
						reject := func(request history.ChangeCommandRequest, message string) {
							t.Helper()
							_, e := api.PreviewChange(ctx, request)
							if e == nil || !strings.Contains(e.Error(), message) {
								t.Fatalf("preview must reject %s: %v", message, e)
							}
							_, e = api.RecordChange(ctx, request)
							if e == nil || !strings.Contains(e.Error(), message) {
								t.Fatalf("commit must reject %s: %v", message, e)
							}
						}
						local := base
						local.EffectiveLocalDate = date
						local.EffectiveLocalTime = minute
						reject(local, "cannot precede the Starting point")
						preOrigin := base
						originInstant, _ := time.Parse(time.RFC3339Nano, fixture.origin)
						preOrigin.EffectiveAt = originInstant.Add(-time.Millisecond).Format(time.RFC3339Nano)
						reject(preOrigin, "cannot precede the Starting point")
						future := base
						future.EffectiveAt = now.Add(time.Second).Format(time.RFC3339Nano)
						reject(future, "cannot be in the future")
						local.EffectiveLocalTime = now.In(location).Format("15:04")
						if _, err := api.PreviewChange(ctx, local); err != nil {
							t.Fatalf("legal manual minute: %v", err)
						}

					})
				}
			})
		}
	}
}
