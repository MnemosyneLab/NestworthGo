package application

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type refreshFakeProvider struct {
	key         string
	mu          sync.Mutex
	instruments map[string]struct {
		quote LatestInstrumentQuote
		err   error
	}
	fx map[string]struct {
		quote LatestFXQuote
		err   error
	}
	calls []string
}

func newRefreshFakeProvider() *refreshFakeProvider {
	return &refreshFakeProvider{key: "fake", instruments: make(map[string]struct {
		quote LatestInstrumentQuote
		err   error
	}), fx: make(map[string]struct {
		quote LatestFXQuote
		err   error
	})}
}

func (p *refreshFakeProvider) Key() string { return p.key }

func (p *refreshFakeProvider) Capabilities() MarketDataCapabilities {
	return MarketDataCapabilities{LatestInstrument: true, LatestFX: true}
}

func (p *refreshFakeProvider) LatestInstrument(_ context.Context, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "instrument:"+identity.ProviderSymbol)
	item, ok := p.instruments[identity.ProviderSymbol]
	if !ok {
		return LatestInstrumentQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable, Message: "provider is unavailable"}
	}
	return item.quote, item.err
}

func (p *refreshFakeProvider) LatestFX(_ context.Context, identity FXMarketIdentity) (LatestFXQuote, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := identity.BaseCurrency.String() + "/" + identity.QuoteCurrency.String()
	p.calls = append(p.calls, "fx:"+key)
	item, ok := p.fx[key]
	if !ok {
		return LatestFXQuote{}, &domain.Error{Code: domain.ErrProviderUnavailable, Message: "provider is unavailable"}
	}
	return item.quote, item.err
}

func (p *refreshFakeProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func (p *refreshFakeProvider) callNames() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.calls...)
}

type refreshAliasProvider struct {
	key    string
	target *refreshFakeProvider
}

type blockingMarketDataRegistry struct {
	delegate MarketDataRegistryPort
	entered  chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (r *blockingMarketDataRegistry) Resolve(key string) (MarketDataProvider, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.release
	return r.delegate.Resolve(key)
}

func (r *blockingMarketDataRegistry) Default() (MarketDataProvider, error) {
	return r.delegate.Default()
}

func (r *blockingMarketDataRegistry) Providers() []MarketDataProvider {
	return r.delegate.Providers()
}

func (p *refreshAliasProvider) Key() string { return p.key }

func (p *refreshAliasProvider) Capabilities() MarketDataCapabilities {
	return p.target.Capabilities()
}

func (p *refreshAliasProvider) LatestInstrument(ctx context.Context, identity InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	quote, err := p.target.LatestInstrument(ctx, identity)
	quote.SourceKey = p.key
	return quote, err
}

func (p *refreshAliasProvider) LatestFX(ctx context.Context, identity FXMarketIdentity) (LatestFXQuote, error) {
	quote, err := p.target.LatestFX(ctx, identity)
	quote.SourceKey = p.key
	return quote, err
}

func newRefreshFixture(t *testing.T) (*sqlite.DB, *Service, *refreshFakeProvider, domain.AccountRecord, domain.Instrument) {
	t.Helper()
	database, err := sqlite.Open(t.TempDir() + "/refresh.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	fake := newRefreshFakeProvider()
	service := NewService(sqlite.NewRepository(database), NewMarketDataRegistry(fake))
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Refresh", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true, Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}}})
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: "QQQ"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "3"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "10", "USD", "2026-08-23"); err != nil {
		t.Fatal(err)
	}
	price, _ := domain.ParseUnitPrice("700")
	fake.instruments["QQQ"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: "fake", QuotedAt: now}, err: nil}
	rate, _ := domain.ParseFxRate("6.9")
	fake.fx["USD/CNY"] = struct {
		quote LatestFXQuote
		err   error
	}{quote: LatestFXQuote{Rate: rate, BaseCurrency: "USD", QuoteCurrency: "CNY", SourceKey: "fake", QuotedAt: now}, err: nil}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "provider"); err != nil {
		t.Fatal(err)
	}
	return database, service, fake, account, instrument
}

func TestFXProviderSelectionRoutesRequiredFXOnly(t *testing.T) {
	_, service, fake, _, _ := newRefreshFixture(t)
	alias := &refreshAliasProvider{key: "alternate", target: fake}
	service.SetMarketDataRegistry(NewMarketDataRegistry(fake, alias))
	if err := service.SetFXProvider(alias.key); err != nil {
		t.Fatalf("SetFXProvider() error = %v", err)
	}
	if got := service.FXProviderKey(); got != alias.key {
		t.Fatalf("FXProviderKey() = %q, want %q", got, alias.key)
	}

	result, err := service.RefreshRequiredFX(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshFetched)
	if names := fake.callNames(); len(names) != 1 || names[0] != "fx:USD/CNY" {
		t.Fatalf("required FX calls = %v, want one FX call", names)
	}
	quotes, err := service.repository.ListFXQuotes(context.Background(), resultHouseholdID(t, service))
	if err != nil {
		t.Fatal(err)
	}
	if len(quotes) != 1 || quotes[0].SourceKey != alias.key {
		t.Fatalf("selected FX quote = %#v, want source %q", quotes, alias.key)
	}
}

func resultHouseholdID(t *testing.T, service *Service) domain.HouseholdID {
	t.Helper()
	household, err := service.repository.Household(context.Background())
	if err != nil || household == nil {
		t.Fatalf("Household() = %#v, %v", household, err)
	}
	return household.ID
}

func TestSetFXProviderRejectsMissingOrFXIncapableProvider(t *testing.T) {
	service := NewService(nil, NewMarketDataRegistry(newRefreshFakeProvider()))
	if err := service.SetFXProvider("missing"); err == nil {
		t.Fatal("SetFXProvider(missing) error = nil")
	}

	instrumentOnly := &instrumentOnlyProvider{}
	service.SetMarketDataRegistry(NewMarketDataRegistry(instrumentOnly))
	if err := service.SetFXProvider(instrumentOnly.Key()); err == nil {
		t.Fatal("SetFXProvider(instrument-only) error = nil")
	}
}

func TestRefreshUsesOneRegistrySnapshotDuringRegistrySwap(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	oldRegistry := service.MarketDataRegistry()
	registry := &blockingMarketDataRegistry{delegate: oldRegistry, entered: make(chan struct{}), release: make(chan struct{})}
	service.SetMarketDataRegistry(registry)

	done := make(chan RefreshResult, 1)
	go func() {
		result, err := service.RefreshInstrument(context.Background(), instrument.ID)
		if err != nil {
			done <- RefreshResult{Items: []RefreshTargetResult{{Status: RefreshFailed, ErrorCode: domain.ErrUnavailable}}}
			return
		}
		done <- result
	}()
	select {
	case <-registry.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach the registry resolve barrier")
	}

	// The in-flight refresh must keep using the registry it copied before this
	// swap. The replacement does not contain the instrument's provider key.
	service.SetMarketDataRegistry(NewMarketDataRegistry())
	close(registry.release)
	select {
	case result := <-done:
		assertRefreshStatus(t, result, "instrument:"+instrument.ID.String(), RefreshFetched)
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not finish after releasing the registry")
	}
	if calls := fake.callNames(); len(calls) == 0 {
		t.Fatal("the refresh did not call the provider from its registry snapshot")
	}

	service.SetMarketDataRegistry(nil)
	result, err := service.RefreshInstrument(context.Background(), instrument.ID)
	if err != nil {
		t.Fatalf("refresh with nil registry returned transport error: %v", err)
	}
	assertRefreshStatus(t, result, "instrument:"+instrument.ID.String(), RefreshFailed)
	if got := result.Items[0].ErrorCode; got != domain.ErrUnavailable {
		t.Fatalf("nil registry error code = %q, want %q", got, domain.ErrUnavailable)
	}
}

type instrumentOnlyProvider struct{}

func (*instrumentOnlyProvider) Key() string { return "instrument-only" }

func (*instrumentOnlyProvider) Capabilities() MarketDataCapabilities {
	return MarketDataCapabilities{LatestInstrument: true}
}

func (*instrumentOnlyProvider) LatestInstrument(context.Context, InstrumentMarketIdentity) (LatestInstrumentQuote, error) {
	return LatestInstrumentQuote{}, nil
}

func (*instrumentOnlyProvider) LatestFX(context.Context, FXMarketIdentity) (LatestFXQuote, error) {
	return LatestFXQuote{}, nil
}

func TestRefreshDeduplicatesTargetsAndProviderObservations(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	ctx := context.Background()
	result, err := service.RefreshAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshFetched)
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshFetched)
	if fake.callCount() != 2 {
		t.Fatalf("provider calls = %v", fake.callNames())
	}
	quotes, err := service.repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("instrument quote count = %d, err = %v", len(quotes), err)
	}

	result, err = service.RefreshAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshCached)
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshCached)
	quotes, err = service.repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 1 {
		t.Fatalf("duplicate observation changed history: %d, err = %v", len(quotes), err)
	}

	price, _ := domain.ParseUnitPrice("701")
	fake.instruments["QQQ"] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: "fake", QuotedAt: quotes[0].QuotedAt}, err: nil}
	result, err = service.RefreshInstrument(ctx, instrument.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshFetched)
	quotes, err = service.repository.ListInstrumentQuotes(ctx, instrument.ID)
	if err != nil || len(quotes) != 2 {
		t.Fatalf("changed same-time observation was not appended: %d, err = %v", len(quotes), err)
	}
}

func TestRefreshSkipsManualTargetsAndStopsAfterRateLimit(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	ctx := context.Background()
	if err := service.SetInstrumentQuoteSource(ctx, instrument.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	result, err := service.RefreshAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshSkipped)
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshSkipped)
	if fake.callCount() != 0 {
		t.Fatalf("manual targets contacted provider: %v", fake.callNames())
	}

	first := instrument
	second, err := service.CreateInstrument(ctx, InstrumentInput{Name: "ES3", Type: "stock", QuoteCurrency: "USD", QuoteSource: "provider", ProviderKey: "fake", ProviderSymbol: "ES3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, first.ID, "provider"); err != nil {
		t.Fatal(err)
	}
	rateID, rateSymbol := first.ID, "QQQ"
	successID, successSymbol := second.ID, "ES3"
	if instrumentTargetKey(successID) < instrumentTargetKey(rateID) {
		rateID, rateSymbol, successID, successSymbol = successID, successSymbol, rateID, rateSymbol
	}
	fake.instruments[rateSymbol] = struct {
		quote LatestInstrumentQuote
		err   error
	}{err: &domain.Error{Code: domain.ErrProviderRateLimit, Message: "provider rate limit reached"}}
	price, _ := domain.ParseUnitPrice("4")
	fake.instruments[successSymbol] = struct {
		quote LatestInstrumentQuote
		err   error
	}{quote: LatestInstrumentQuote{Price: price, Currency: "USD", SourceKey: "fake", QuotedAt: time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)}, err: nil}
	result, err = service.RefreshAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RateLimited {
		t.Fatal("rate-limited refresh did not report RateLimited")
	}
	if fake.callCount() != 1 {
		t.Fatalf("later target was started after rate limit: %v", fake.callNames())
	}
	assertRefreshStatus(t, result, instrumentTargetKey(rateID), RefreshRateLimited)
	assertRefreshStatus(t, result, instrumentTargetKey(successID), RefreshSkipped)
}

func TestRefreshRateLimitStopsOnlyTargetsOwnedByThatProvider(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	other := newRefreshFakeProvider()
	other.key = "other"
	other.fx["USD/CNY"] = struct {
		quote LatestFXQuote
		err   error
	}{err: &domain.Error{Code: domain.ErrProviderRateLimit, Message: "provider rate limit reached"}}
	service.SetMarketDataRegistry(NewMarketDataRegistry(fake, other))
	if err := service.SetFXProvider(other.Key()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(context.Background(), "USD", "CNY", "provider"); err != nil {
		t.Fatal(err)
	}

	result, err := service.RefreshAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshRateLimited)
	assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshFetched)
	if got := other.callNames(); len(got) != 1 || got[0] != "fx:USD/CNY" {
		t.Fatalf("rate-limited provider calls = %v", got)
	}
	if got := fake.callNames(); len(got) != 1 || got[0] != "instrument:QQQ" {
		t.Fatalf("independent provider calls = %v", got)
	}
}

func TestRefreshRejectsProviderTimesBeforeWritingQuotes(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	for _, testCase := range []struct {
		name string
		when time.Time
	}{
		{name: "future", when: now.Add(domain.QuoteClockSkewTolerance + time.Second)},
		{name: "largest unix value", when: time.Unix(1<<63-1, 0)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			fake.instruments["QQQ"] = struct {
				quote LatestInstrumentQuote
				err   error
			}{quote: LatestInstrumentQuote{Price: mustUnitPrice(t, "700"), Currency: "USD", SourceKey: fake.Key(), QuotedAt: testCase.when}}
			result, err := service.RefreshInstrument(context.Background(), instrument.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertRefreshStatus(t, result, instrumentTargetKey(instrument.ID), RefreshFailed)
			quotes, listErr := service.repository.ListInstrumentQuotes(context.Background(), instrument.ID)
			if listErr != nil {
				t.Fatal(listErr)
			}
			if len(quotes) != 0 {
				t.Fatalf("invalid provider time wrote %d quote(s)", len(quotes))
			}
		})
	}
}

func mustUnitPrice(t *testing.T, value string) domain.UnitPrice {
	t.Helper()
	price, err := domain.ParseUnitPrice(value)
	if err != nil {
		t.Fatal(err)
	}
	return price
}

func TestRefreshForeignInstrumentIsSkippedWithoutProviderCall(t *testing.T) {
	_, service, fake, _, _ := newRefreshFixture(t)
	foreignID := domain.InstrumentID("00000000-0000-7000-8000-000000000000")
	result, err := service.RefreshInstrument(context.Background(), foreignID)
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, instrumentTargetKey(foreignID), RefreshSkipped)
	if fake.callCount() != 0 {
		t.Fatalf("foreign instrument contacted provider: %v", fake.callNames())
	}
}

func TestRefreshRequiredFXDoesNotStartInstrumentTargets(t *testing.T) {
	_, service, fake, _, instrument := newRefreshFixture(t)
	result, err := service.RefreshRequiredFX(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertRefreshStatus(t, result, "fx:CNY/USD", RefreshFetched)
	for _, item := range result.Items {
		if item.Kind == RefreshInstrumentTarget {
			t.Fatalf("FX-only refresh returned instrument target %#v", item)
		}
	}
	if fake.callCount() != 1 || fake.callNames()[0] != "fx:USD/CNY" {
		t.Fatalf("FX-only provider calls = %v; instrument = %s", fake.callNames(), instrument.ID)
	}
}

func assertRefreshStatus(t *testing.T, result RefreshResult, key string, status RefreshStatus) {
	t.Helper()
	for _, item := range result.Items {
		if item.TargetKey == key {
			if item.Status != status {
				t.Fatalf("target %s status = %q code=%q, want %q; all=%#v", key, item.Status, item.ErrorCode, status, result.Items)
			}
			return
		}
	}
	t.Fatalf("target %s not present in %#v", key, result.Items)
}
