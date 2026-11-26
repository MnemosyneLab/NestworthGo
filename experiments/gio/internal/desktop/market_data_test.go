package desktop

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	quoteapi "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wire"
)

type retryMarketReads struct {
	*application.Service
	failFX bool
}

func (r *retryMarketReads) ListFXPreferences(ctx context.Context) ([]domain.FXPreference, error) {
	if r.failFX {
		r.failFX = false
		return nil, errors.New("synthetic FX read error")
	}
	return r.Service.ListFXPreferences(ctx)
}

func TestMarketDataSavedReadsMatchWails(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "market.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provider := &unavailableOverviewProvider{}
	service := application.NewService(sqlite.NewRepository(db), application.NewMarketDataRegistry(provider))
	now := time.Date(2026, time.November, 4, 8, 0, 0, 0, time.UTC)
	service.SetClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic market data", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic cash", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance",
		DefaultCurrency: "JPY", InitialAmount: "100", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	}); err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Synthetic market fund", Type: "etf", MarketCode: "US", QuoteCurrency: "USD", QuoteSource: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateInstrument(ctx, application.InstrumentInput{
		Name: "Unquoted bond", Type: "bond", QuoteCurrency: "EUR", QuoteSource: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, instrument.ID, "12.34567891", "2026-11-03", true); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "EUR", "USD", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "EUR", "0.92345678", now.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	ref := quoteapi.NewService(service)
	wantQuote, err := ref.CurrentInstrumentQuote(ctx, instrument.ID.String())
	if err != nil || wantQuote == nil || wantQuote.UnitPrice != "12.34567891" || !wantQuote.Delayed {
		t.Fatalf("Wails instrument quote = %+v, %v", wantQuote, err)
	}
	wantFX, err := ref.CurrentFXQuote(ctx, "EUR", "USD")
	if err != nil || wantFX == nil || wantFX.Rate != "0.92345678" || wantFX.BaseCurrency != "USD" {
		t.Fatalf("Wails oriented FX quote = %+v, %v", wantFX, err)
	}

	page := &marketDataPage{ctx: ctx, reader: &retryMarketReads{Service: service, failFX: true}, invalidate: func() {}}
	t.Cleanup(page.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{marketData: page})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.frame()
	h.click("Data maintenance", false)
	page.workers.Wait()
	h.frame()
	current := page.current[marketInstruments]
	if current == nil || len(current.instruments) != 2 || current.instruments[0].quote == nil ||
		wire.FromInstrumentQuote(*current.instruments[0].quote) != *wantQuote || current.instruments[1].quote != nil {
		t.Fatalf("Gio instrument rows = %+v", current)
	}
	h.click("Search instruments", true)
	h.router.Queue(key.EditEvent{Text: "Unquoted"})
	h.frame()
	h.frame()
	if findPageLabel(&h.router, "Unquoted bond") == nil || findPageLabel(&h.router, "Synthetic market fund") != nil {
		t.Fatalf("instrument search %q did not narrow the list: found bond=%t found fund=%t", page.search.Field.Text(), findPageLabel(&h.router, "Unquoted bond") != nil, findPageLabel(&h.router, "Synthetic market fund") != nil)
	}
	h.click("FX rates", false)
	page.workers.Wait()
	h.frame()
	if page.err == nil || page.current[marketFX] != nil {
		t.Fatal("the first FX read failure was hidden")
	}
	h.click("Retry", false)
	page.workers.Wait()
	h.frame()
	current = page.current[marketFX]
	if current == nil || len(current.fx) != 2 || current.fx[0].quote == nil ||
		wire.FromFXQuote(*current.fx[0].quote) != *wantFX || current.fx[0].source != domain.QuoteSourceManual ||
		current.fx[1] != (marketFXRow{pair: marketFXPair{first: "JPY", second: "USD"}}) {
		t.Fatalf("Gio FX rows = %+v", current)
	}
	h.click("Overview", false)
	h.click("Back", false)
	page.workers.Wait()
	h.frame()
	if page.tab != marketFX || page.search.Field.Text() != "Unquoted" {
		t.Fatal("Back lost the Market data tab or instrument search")
	}
	if provider.requests != 0 {
		t.Fatalf("saved market data requested a provider %d times", provider.requests)
	}
}
