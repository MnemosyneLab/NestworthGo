package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type overviewReaderFunc func(context.Context, domain.AccountFilter) (domain.OverviewResult, error)

func (f overviewReaderFunc) Overview(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
	return f(ctx, filter)
}

func TestOverviewRetryKeepsIncompletePriorResultAndItsReadTime(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	responses := make(chan overviewRead)
	calls := make(chan domain.AccountFilter, 1)
	first := time.Date(2026, 8, 17, 17, 34, 0, 0, time.UTC)
	now := first
	page := newOverviewPage(ctx, overviewReaderFunc(func(ctx context.Context, filter domain.AccountFilter) (domain.OverviewResult, error) {
		calls <- filter
		select {
		case r := <-responses:
			return r.value, r.err
		case <-ctx.Done():
			return domain.OverviewResult{}, ctx.Err()
		}
	}), func() time.Time { return now }, func() {})
	t.Cleanup(func() { cancel(); page.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{overview: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.prefs.saved.Timezone = "Asia/Singapore"
	h.shell.prefs.savedLocation = time.FixedZone("Asia/Singapore", 8*60*60)
	h.frame()
	label := func(s string) { assertPageLabel(t, &h.router, s) }
	absent := func(s string) {
		if findPageLabel(&h.router, s) != nil {
			t.Fatalf("unexpected label %q", s)
		}
	}
	finish := func(r overviewRead) {
		t.Helper()
		select {
		case filter := <-calls:
			if !reflect.DeepEqual(filter, domain.AccountFilter{}) {
				t.Fatalf("Overview requested a scoped query: %+v", filter)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("Overview did not issue a read")
		}
		responses <- r
		page.workers.Wait()
		h.frame()
	}
	label("Loading valuation…")
	absent("Net worth")
	failure := overviewRead{err: errors.New("synthetic read failure")}
	finish(failure)
	label("Could not load the valuation.")
	h.click("Retry", false)
	label("Loading valuation…")
	known := &domain.MoneyView{Amount: "128.75", Currency: "AUD"}
	partial := domain.ValuationAmount{Currency: "AUD", KnownSubtotal: known, Quality: domain.ValuationQuality{Availability: domain.ValuationPartial}}
	unknown := domain.ValuationAmount{Currency: "AUD", Quality: domain.ValuationQuality{Availability: domain.ValuationNone}}
	finish(overviewRead{value: domain.OverviewResult{
		Currency: "AUD", AccountCount: 2, DisplayValuation: domain.HouseholdDisplayValuation{Assets: partial, Liabilities: unknown, NetWorth: partial},
		MissingInputs: []domain.MissingInputView{{Kind: domain.MissingFXRate, BaseCurrency: "AUD", QuoteCurrency: "USD", AccountName: "Foreign debt"}},
	}})
	label("Incomplete valuation")
	label("Base currency AUD")
	label("Net worth · Known net contribution")
	label("A$ 128.75")
	label("Unavailable")
	absent("A$ 0.00")
	label("Missing exchange rate · AUD/USD · Foreign debt")
	stamp := "Read 2026-08-18 01:34 (Asia/Singapore)"
	label(stamp)
	h.click("Settings", false)
	h.click("Back", false)
	label(stamp)
	now = first.Add(24 * time.Hour)
	h.click("Reload", false)
	label("Updating valuation…")
	finish(failure)
	label("Update failed. Showing the previous result.")
	label(stamp)
	label("Incomplete valuation")
	h.click("Retry", false)
	finish(overviewRead{value: domain.OverviewResult{Currency: "AUD", Complete: true}})
	label("Read 2026-08-19 01:34 (Asia/Singapore)")
	label("No accounts to show.")
	absent("Net worth · Known net contribution")
	absent("A$ 0.00")
	h.click("Reload", false)
	finish(overviewRead{value: domain.OverviewResult{
		Currency: "AUD", AccountCount: 1, DisplayValuation: domain.HouseholdDisplayValuation{Assets: unknown, Liabilities: unknown, NetWorth: unknown},
		MissingInputs: []domain.MissingInputView{{Kind: domain.MissingInstrumentPrice, InstrumentSymbol: "DEMO"}},
	}})
	label("Net worth")
	label("Unavailable")
	label("Missing price · DEMO")
	absent("Net worth · Known net contribution")
	absent("A$ 0.00")
}

type unavailableOverviewProvider struct{ requests int }

func (*unavailableOverviewProvider) Key() string { return application.FrankfurterProviderKey }
func (*unavailableOverviewProvider) Capabilities() application.MarketDataCapabilities {
	return application.MarketDataCapabilities{LatestFX: true}
}
func (p *unavailableOverviewProvider) LatestFX(context.Context, application.FXMarketIdentity) (application.LatestFXQuote, error) {
	p.requests++
	return application.LatestFXQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
}
func (p *unavailableOverviewProvider) LatestInstrument(context.Context, application.InstrumentMarketIdentity) (application.LatestInstrumentQuote, error) {
	p.requests++
	return application.LatestInstrumentQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable}
}

func TestOverviewMatchesWailsWithPersistedFXAndAnUnavailableProvider(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 17, 16, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	path := filepath.Join(t.TempDir(), "seed.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	seed := application.NewService(repo)
	seed.SetClock(clock)
	if err := seed.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic Overview", BaseCurrency: "AUD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := seed.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []application.AccountInput{
		{Name: "USD Cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100.05"},
		{Name: "AUD Debt", AccountType: "credit_card", BalanceSheetRole: "liability", TrackingMode: "balance", DefaultCurrency: "AUD", InitialAmount: "25.10"},
	} {
		input.IncludeInNetWorth = true
		input.OwnerIDs = []domain.MemberID{bootstrap.Members[0].ID}
		if _, err := seed.CreateAccount(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []struct{ provider, rate string }{{application.FrankfurterProviderKey, "1.2345"}, {"other-provider", "10"}} {
		rate, err := domain.ParseFxRate(q.rate)
		if err != nil {
			t.Fatal(err)
		}
		quoted := now
		quote := domain.FXQuote{
			ID: domain.NewFXQuoteID(), HouseholdID: bootstrap.Household.ID, BaseCurrency: "USD", QuoteCurrency: "AUD", Rate: rate,
			SourceKind: domain.QuoteSourceProvider, SourceKey: q.provider, QuotedAt: quoted, CreatedAt: quoted,
		}
		if q.provider == application.FrankfurterProviderKey {
			quote.ObservationKind = string(application.FXObservationDailyReference)
			quote.EffectiveDate = "2026-08-17"
			quote.ValueEffectiveAt = now.Add(-3 * time.Hour)
			quote.QuotedAt, quote.CreatedAt = quote.ValueEffectiveAt, quote.ValueEffectiveAt
		}
		if err := repo.AppendFXQuote(ctx, quote); err != nil {
			t.Fatal(err)
		}
		if q.provider == application.FrankfurterProviderKey {
			// Preserve the daily reference alongside the other provider's latest
			// observation so both survive the repository's candidate projection.
			if _, err := db.SQL.ExecContext(ctx, `UPDATE fx_quotes SET observation_kind=?, effective_date=?, value_effective_at=?, source_policy_version=?, timestamp_basis=? WHERE id=?`,
				quote.ObservationKind, quote.EffectiveDate, quote.ValueEffectiveAt.Format(time.RFC3339), domain.FrankfurterV2BlendedPolicy, string(application.TimestampBasisPolicyDerived), quote.ID.String()); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := errors.Join(db.CheckpointWAL(ctx), db.Close()); err != nil {
		t.Fatal(err)
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	openCopy := func(registry ...application.MarketDataRegistryPort) *application.Service {
		t.Helper()
		copy := filepath.Join(t.TempDir(), "independent.db")
		if err := os.WriteFile(copy, bytes, 0o600); err != nil {
			t.Fatal(err)
		}
		db, err := sqlite.OpenReadOnlyForVerifyContext(ctx, copy)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = db.Close() })
		s := application.NewService(sqlite.NewRepository(db), registry...)
		s.SetClock(clock)
		s.SetQuoteCacheTTL(time.Hour)
		return s
	}
	provider := &unavailableOverviewProvider{}
	wails := openCopy(application.NewMarketDataRegistry(provider))
	if err := wails.SetFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	ref, err := portfolio.NewService(wails).Overview(ctx, account.AccountFilterRequest{})
	if err != nil {
		t.Fatal(err)
	}
	gio := openCopy()
	if err := gio.SetStoredFXProvider(application.FrankfurterProviderKey); err != nil {
		t.Fatal(err)
	}
	page := newOverviewPage(ctx, gio, clock, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{overview: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	page.workers.Wait()
	h.frame()
	if page.current == nil {
		t.Fatalf("Gio Overview failed: %v", page.err)
	}
	value := page.current.value
	if !reflect.DeepEqual(wire.FromHouseholdDisplay(value.DisplayValuation), ref.DisplayValuation) || !reflect.DeepEqual(wire.FromMissingInputs(value.MissingInputs), ref.MissingInputs) ||
		value.Assets.String() != ref.Assets || value.Liabilities.String() != ref.Liabilities || value.NetWorth.String() != ref.NetWorth || value.Complete != ref.Complete || provider.requests != 0 {
		t.Fatalf("Gio returned %+v, Wails returned %+v, provider calls %d", value.DisplayValuation, ref, provider.requests)
	}
	if ref.NetWorth != "98.411725" || !ref.Complete || value.DisplayValuation.NetWorth.Quality.StaleCount == 0 {
		t.Fatalf("expected exact net worth with stale cached FX, got %+v", ref)
	}
	for _, label := range []string{"A$ 98.41", "A$ 123.51", "-A$ 25.10", "Some cached quotes await an update.", "Current estimate"} {
		assertPageLabel(t, &h.router, label)
	}
}
