package application

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

type goldenValuationFixture struct {
	database   *sqlite.DB
	service    *Service
	repository Repository
	now        time.Time
	account    domain.AccountRecord
	qqq        domain.Instrument
	es3        domain.Instrument
	identityID domain.AccountID
}

func newGoldenValuationFixture(t *testing.T, includeES3Quote bool) goldenValuationFixture {
	t.Helper()
	database, err := sqlite.Open(t.TempDir() + "/valuation.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Golden Household", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	owner := bootstrap.Members[0].ID
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Golden Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	qqq, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "QQQ", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual",
		CountryCode: "US", ProviderKey: "yahoo_finance", ProviderSymbol: "QQQ",
	})
	if err != nil {
		t.Fatal(err)
	}
	es3, err := service.CreateInstrument(ctx, InstrumentInput{
		Name: "ES3", Type: "stock", QuoteCurrency: "SGD", QuoteSource: "manual", CountryCode: "SG",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: qqq.ID.String(), Quantity: "3", SortOrder: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: es3.ID.String(), Quantity: "1000", SortOrder: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "5000", "SGD", "2026-08-23"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, qqq.ID, "700", "2026-08-23", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetInstrumentQuoteSource(ctx, qqq.ID, "manual"); err != nil {
		t.Fatal(err)
	}
	if includeES3Quote {
		if _, err := service.AppendManualInstrumentQuote(ctx, es3.ID, "4", "2026-08-23", false); err != nil {
			t.Fatal(err)
		}
		if err := service.SetInstrumentQuoteSource(ctx, es3.ID, "manual"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.AppendManualFXQuote(ctx, "SGD", "CNY", "5.3", "2026-08-23"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualFXQuote(ctx, "USD", "CNY", "6.9", "2026-08-23"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "SGD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetFXPreference(ctx, "USD", "CNY", "manual"); err != nil {
		t.Fatal(err)
	}

	identityAccount, err := service.CreateAccount(ctx, AccountInput{
		Name: "Local Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: false, IncludeInPortfolio: false,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	identityInstrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Local ETF", Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual", CountryCode: "CN"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: identityAccount.Account.ID.String(), InstrumentID: identityInstrument.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, identityInstrument.ID, "10", "2026-08-23", false); err != nil {
		t.Fatal(err)
	}

	// This account proves that excluded balance-sheet data is still readable but
	// cannot leak into Portfolio or Overview totals.
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Excluded Balance", AccountType: "bank_account", BalanceSheetRole: "asset",
		TrackingMode: "balance", DefaultCurrency: "CNY", IncludeInNetWorth: false, IncludeInPortfolio: false,
		Ownership: []domain.OwnershipShare{{MemberID: owner, ShareBPS: domain.TotalOwnershipBPS}}, InitialAmount: "999999",
	}); err != nil {
		t.Fatal(err)
	}

	return goldenValuationFixture{database: database, service: service, repository: repository, now: now, account: account, qqq: qqq, es3: es3, identityID: identityAccount.Account.ID}
}

func TestValuationServiceMatchesGoldenPortfolioAndPartialSubtotal(t *testing.T) {
	ctx := context.Background()
	complete := newGoldenValuationFixture(t, true)
	portfolio, err := complete.service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, portfolio.ValuedSubtotal, "35700", "CNY")
	if !portfolio.Complete || len(portfolio.MissingInputs) != 0 {
		t.Fatalf("complete portfolio = %+v", portfolio)
	}
	account, err := complete.service.AccountValuation(ctx, complete.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, account.BaseValue, "62190", "CNY")
	if !account.Complete || len(account.Components) != 3 {
		t.Fatalf("complete account = %+v", account)
	}
	overview, err := complete.service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !overview.Complete || len(overview.MissingInputs) != 0 || !overview.Assets.Equal(decimal.RequireFromString("62190")) || !overview.NetWorth.Equal(decimal.RequireFromString("62190")) {
		t.Fatalf("complete overview = %+v", overview)
	}

	partial := newGoldenValuationFixture(t, false)
	partialPortfolio, err := partial.service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, partialPortfolio.ValuedSubtotal, "14500", "CNY")
	if partialPortfolio.Complete || len(partialPortfolio.MissingInputs) != 1 {
		t.Fatalf("partial portfolio = %+v", partialPortfolio)
	}
	missing := partialPortfolio.MissingInputs[0]
	if missing.Kind != domain.MissingInstrumentPrice || missing.AccountID != partial.account.Account.ID || missing.InstrumentID == nil || *missing.InstrumentID != partial.es3.ID {
		t.Fatalf("partial missing input = %+v", missing)
	}
	partialAccount, err := partial.service.AccountValuation(ctx, partial.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, partialAccount.BaseValue, "40990", "CNY")
	if partialAccount.Complete || len(partialAccount.MissingInputs) != 1 {
		t.Fatalf("partial account = %+v", partialAccount)
	}
	partialOverview, err := partial.service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if partialOverview.Complete || len(partialOverview.MissingInputs) != 1 || !partialOverview.Assets.Equal(decimal.RequireFromString("40990")) {
		t.Fatalf("partial overview = %+v", partialOverview)
	}
	for _, component := range partialAccount.Components {
		if component.InstrumentID != nil && *component.InstrumentID == partial.es3.ID && (component.Available || component.BaseAmount != nil || component.BaseAmountExact != "") {
			t.Fatalf("missing ES3 was zero-filled: %+v", component)
		}
	}
}

func TestValuationServiceResolvesSourcesOrientationAndFreshness(t *testing.T) {
	ctx := context.Background()
	fixture := newGoldenValuationFixture(t, true)
	manual, err := fixture.service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, manual.ValuedSubtotal, "35700", "CNY")
	manualAccount, err := fixture.service.AccountValuation(ctx, fixture.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	manualQQQ := valuationComponentFor(t, manualAccount, fixture.qqq.ID)
	if manualQQQ.PriceEvidence == nil || manualQQQ.PriceEvidence.Source != domain.QuoteSourceManual || manualQQQ.FXEvidence == nil || manualQQQ.FXEvidence.Source != domain.QuoteSourceManual {
		t.Fatalf("manual provenance = %+v", manualQQQ)
	}

	if _, err := fixture.service.AppendManualFXQuote(ctx, "CNY", "USD", "0.144927536232", "2026-08-24"); err != nil {
		t.Fatal(err)
	}
	inverse, err := fixture.service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, inverse.ValuedSubtotal, "35700", "CNY")

	providerPrice, err := domain.ParseUnitPrice("700")
	if err != nil {
		t.Fatal(err)
	}
	staleQuote, err := domain.NewInstrumentQuote(fixture.qqq, domain.InstrumentQuoteInput{
		UnitPrice: providerPrice, Currency: fixture.qqq.QuoteCurrency, SourceKind: domain.QuoteSourceProvider,
		SourceKey: "yahoo_finance", QuotedAt: fixture.now.Add(-48 * time.Hour), Delayed: false,
	}, fixture.now.Add(-47*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.AppendInstrumentQuote(ctx, staleQuote); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.SetInstrumentQuoteSource(ctx, fixture.qqq.ID, "provider"); err != nil {
		t.Fatal(err)
	}
	providerRate, err := domain.ParseFxRate("6.9")
	if err != nil {
		t.Fatal(err)
	}
	household, err := fixture.service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	providerFX, err := domain.NewFXQuote(domain.FXQuoteInput{
		HouseholdID: household.Household.ID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: providerRate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: "yahoo_finance", QuotedAt: fixture.now.Add(4 * time.Hour),
	}, fixture.now.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.AppendFXQuote(ctx, providerFX); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.SetFXPreference(ctx, "USD", "CNY", "provider"); err != nil {
		t.Fatal(err)
	}
	provider, err := fixture.service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, provider.ValuedSubtotal, "35700", "CNY")
	providerAccount, err := fixture.service.AccountValuation(ctx, fixture.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	providerQQQ := valuationComponentFor(t, providerAccount, fixture.qqq.ID)
	if providerQQQ.PriceEvidence == nil || providerQQQ.PriceEvidence.Source != domain.QuoteSourceProvider || providerQQQ.PriceEvidence.Freshness != domain.FreshnessStale {
		t.Fatalf("provider/stale price provenance = %+v", providerQQQ.PriceEvidence)
	}
	if providerQQQ.FXEvidence == nil || providerQQQ.FXEvidence.Source != domain.QuoteSourceProvider {
		t.Fatalf("provider FX provenance = %+v", providerQQQ.FXEvidence)
	}

	delayedQuote, err := domain.NewInstrumentQuote(fixture.qqq, domain.InstrumentQuoteInput{
		UnitPrice: providerPrice, Currency: fixture.qqq.QuoteCurrency, SourceKind: domain.QuoteSourceProvider,
		SourceKey: "yahoo_finance", QuotedAt: fixture.now.Add(-time.Hour), Delayed: true,
	}, fixture.now.Add(-50*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.repository.AppendInstrumentQuote(ctx, delayedQuote); err != nil {
		t.Fatal(err)
	}
	delayedAccount, err := fixture.service.AccountValuation(ctx, fixture.account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	delayedQQQ := valuationComponentFor(t, delayedAccount, fixture.qqq.ID)
	if delayedQQQ.PriceEvidence == nil || delayedQQQ.PriceEvidence.Freshness != domain.FreshnessDelayed || !delayedQQQ.PriceEvidence.Delayed {
		t.Fatalf("delayed price provenance = %+v", delayedQQQ.PriceEvidence)
	}

	identityAccount, err := fixture.service.AccountValuation(ctx, fixture.identityID)
	if err != nil {
		t.Fatal(err)
	}
	if len(identityAccount.Components) != 1 || identityAccount.Components[0].FXEvidence != nil || identityAccount.Components[0].PriceEvidence == nil || identityAccount.Components[0].PriceEvidence.Source != domain.QuoteSourceManual {
		t.Fatalf("identity conversion evidence = %+v", identityAccount.Components)
	}
}

func TestHistoricalInstrumentQuoteSelectionRequiresCanonicalProvenance(t *testing.T) {
	instrumentID := domain.NewInstrumentID()
	provider := domain.TiingoProviderKey
	instrument := domain.Instrument{
		ID: instrumentID, QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider,
		ProviderKey: &provider, ProviderBindingRevision: 2,
	}
	cutoff := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	validEffective := time.Date(2026, time.September, 4, 20, 0, 0, 0, time.UTC)
	price := func(value string) domain.UnitPrice {
		parsed, err := domain.ParseUnitPrice(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	quotes := []domain.InstrumentQuote{
		{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price("999"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationRealtime), ValueEffectiveAt: cutoff.Add(-time.Minute), BindingRevision: 2, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)},
		{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price("998"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-04", ValueEffectiveAt: validEffective, BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)},
		{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price("997"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-04", ValueEffectiveAt: validEffective, BindingRevision: 2, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: "old-policy", TimestampBasis: string(TimestampBasisSessionClose)},
		{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price("186"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-04", ValueEffectiveAt: validEffective, BindingRevision: 2, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)},
		{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price("10000"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationLegacy), QuotedAt: cutoff.Add(-time.Hour)},
	}
	selected := selectHistoricalInstrumentQuote(instrument, quotes, cutoff)
	if selected == nil || selected.UnitPrice.Canonical() != "186" {
		t.Fatalf("selected historical quote = %+v", selected)
	}
}

func TestHistoricalCarryForwardRequiresVerifiedCoverage(t *testing.T) {
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID()
	instrumentID := domain.NewInstrumentID()
	holdingID := domain.NewHoldingID()
	provider := domain.TiingoProviderKey
	market := "US"
	quantity, err := domain.ParseQuantity("10")
	if err != nil {
		t.Fatal(err)
	}
	price, err := domain.ParseUnitPrice("186")
	if err != nil {
		t.Fatal(err)
	}
	household := &domain.Household{ID: householdID, BaseCurrency: "USD"}
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "Historical asset", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderBindingRevision: 1, MarketCode: &market}
	account := domain.Account{ID: accountID, HouseholdID: householdID, Name: "Brokerage", TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true}
	holding := domain.Holding{ID: holdingID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price, Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-04", ValueEffectiveAt: time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC), QuotedAt: time.Date(2026, 9, 4, 20, 1, 0, 0, time.UTC), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose)}
	snapshot := domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}, InstrumentQuotes: []domain.InstrumentQuote{quote}}
	cutoff := time.Date(2026, 9, 7, 23, 59, 59, 999000000, time.UTC)
	valuation := NewValuationService(nil, func() time.Time { return cutoff })
	valuation.SetHistorical(true)
	accounts, _, err := valuation.ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Complete || len(accounts[0].MissingInputs) != 1 || accounts[0].MissingInputs[0].Kind != domain.MissingHistoryCoverage {
		t.Fatalf("unqueried history = %+v", accounts)
	}
	if accounts[0].Components[0].BaseAmountExact != "1860" {
		t.Fatalf("carry-forward amount = %s, want 1860", accounts[0].Components[0].BaseAmountExact)
	}

	snapshot.InstrumentHistoryCoverage = []domain.InstrumentHistoryCoverage{{
		InstrumentID: instrumentID, ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: string(PriceBasisTiingoRawClose),
		CloseMarketDates: []string{"2026-09-04"}, NoObservationDates: []string{"2026-09-05", "2026-09-06", "2026-09-07"},
	}}
	accounts, _, err = valuation.ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || !accounts[0].Complete || len(accounts[0].MissingInputs) != 0 {
		t.Fatalf("verified no-observation history = %+v", accounts)
	}

	snapshot.InstrumentHistoryCoverage[0].UnverifiedDates = []string{"2026-09-07"}
	accounts, _, err = valuation.ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Complete || len(accounts[0].MissingInputs) != 1 {
		t.Fatalf("pending history = %+v", accounts)
	}
}

func TestHistoricalMarketDateUsesLateCloseAndRebuildQuality(t *testing.T) {
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID()
	instrumentID := domain.NewInstrumentID()
	holdingID := domain.NewHoldingID()
	provider := domain.TiingoProviderKey
	quantity, err := domain.ParseQuantity("10")
	if err != nil {
		t.Fatal(err)
	}
	priceForTest := func(value string) domain.UnitPrice {
		parsed, parseErr := domain.ParseUnitPrice(value)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		return parsed
	}
	lateClose := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	previous := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: priceForTest("180"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-08", ValueEffectiveAt: lateClose.Add(-24 * time.Hour), BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose), Revision: 1}
	current := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: priceForTest("186"), Currency: "USD", SourceKind: domain.QuoteSourceProvider, SourceKey: provider, ObservationKind: string(InstrumentObservationClose), EffectiveDate: "2026-09-09", ValueEffectiveAt: lateClose, BindingRevision: 1, PriceBasis: string(PriceBasisTiingoRawClose), SourcePolicyVersion: string(PriceBasisTiingoRawClose), TimestampBasis: string(TimestampBasisSessionClose), Revision: 1}
	household := &domain.Household{ID: householdID, BaseCurrency: "USD"}
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: householdID, Name: "Late close", QuoteCurrency: "USD", QuoteSource: domain.QuoteSourceProvider, ProviderKey: &provider, ProviderBindingRevision: 1}
	account := domain.Account{ID: accountID, HouseholdID: householdID, Name: "Brokerage", TrackingMode: domain.TrackingHoldings, DefaultCurrency: "USD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true}
	holding := domain.Holding{ID: holdingID, AccountID: accountID, InstrumentID: instrumentID, Quantity: quantity}
	valuation := NewValuationService(nil, func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) })
	valuation.SetHistorical(true)
	valuation.SetHistoricalMarketDate("2026-09-09")
	snapshot := domain.PortfolioSnapshot{Household: household, Accounts: []domain.AccountRecord{{Account: account}}, Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding}, InstrumentQuotes: []domain.InstrumentQuote{previous}, InstrumentHistoryCoverage: []domain.InstrumentHistoryCoverage{{InstrumentID: instrumentID, ProviderKey: provider, BindingRevision: 1, SourcePolicyVersion: string(PriceBasisTiingoRawClose), CloseMarketDates: []string{"2026-09-08"}, UnverifiedDates: []string{"2026-09-09"}}}}
	accounts, _, err := valuation.ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || accounts[0].Complete || accounts[0].Components[0].BaseAmountExact != "1800" || len(accounts[0].MissingInputs) != 1 || accounts[0].MissingInputs[0].Kind != domain.MissingHistoryCoverage {
		t.Fatalf("before late close = %+v", accounts)
	}

	snapshot.InstrumentQuotes = append(snapshot.InstrumentQuotes, current)
	snapshot.InstrumentHistoryCoverage[0].CloseMarketDates = []string{"2026-09-08", "2026-09-09"}
	snapshot.InstrumentHistoryCoverage[0].UnverifiedDates = nil
	accounts, _, err = valuation.ValueAccounts(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 1 || !accounts[0].Complete || accounts[0].Components[0].BaseAmountExact != "1860" || len(accounts[0].MissingInputs) != 0 {
		t.Fatalf("after late close = %+v", accounts)
	}
}

func TestHistoricalFXQuoteSelectionRequiresCanonicalProvenance(t *testing.T) {
	householdID := domain.NewHouseholdID()
	preference := domain.FXPreference{HouseholdID: householdID, CurrencyA: "SGD", CurrencyB: "USD", SourceKind: domain.QuoteSourceProvider}
	cutoff := time.Date(2026, time.September, 7, 0, 0, 0, 0, time.UTC)
	effective := time.Date(2026, time.September, 4, 23, 59, 59, 999000000, time.UTC)
	rate := func(value string) domain.FxRate {
		parsed, err := domain.ParseFxRate(value)
		if err != nil {
			t.Fatal(err)
		}
		return parsed
	}
	quotes := []domain.FXQuote{
		{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate("99"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationLatest), ValueEffectiveAt: cutoff.Add(-time.Minute), SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, TimestampBasis: string(TimestampBasisPolicyDerived)},
		{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate("1.35"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-04", ValueEffectiveAt: effective, SourcePolicyVersion: "old-policy", TimestampBasis: string(TimestampBasisPolicyDerived)},
		{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: rate("1.351"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-04", ValueEffectiveAt: effective, SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, TimestampBasis: string(TimestampBasisPolicyDerived)},
	}
	selected := selectHistoricalFXQuote(preference, quotes, "USD", "SGD", domain.FrankfurterProviderKey, cutoff)
	if selected == nil || selected.Rate.Canonical() != "1.351" {
		t.Fatalf("selected historical FX quote = %+v", selected)
	}
}

func TestHistoricalFXMarketDateReconcilesPendingReference(t *testing.T) {
	householdID := domain.NewHouseholdID()
	preference := domain.FXPreference{HouseholdID: householdID, CurrencyA: "SGD", CurrencyB: "USD", SourceKind: domain.QuoteSourceProvider}
	parseRate := func(value string) domain.FxRate {
		rate, err := domain.ParseFxRate(value)
		if err != nil {
			t.Fatal(err)
		}
		return rate
	}
	quotes := []domain.FXQuote{{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: parseRate("1.34"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-08", ValueEffectiveAt: time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC), SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, TimestampBasis: string(TimestampBasisPolicyDerived), Revision: 1}}
	coverage := domain.PortfolioSnapshot{FXHistoryCoverage: []domain.FXHistoryCoverage{{BaseCurrency: "USD", QuoteCurrency: "SGD", ProviderKey: domain.FrankfurterProviderKey, SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, DailyReferenceDates: []string{"2026-09-08"}, UnverifiedDates: []string{"2026-09-09"}}}}
	cashAccount := domain.Account{ID: domain.NewAccountID(), HouseholdID: householdID, Name: "SGD cash", TrackingMode: domain.TrackingBalance, DefaultCurrency: "SGD", BalanceSheetRole: domain.RoleAsset, IncludeInNetWorth: true}
	cashAmount, err := domain.NewMoney(decimal.NewFromInt(100), "SGD")
	if err != nil {
		t.Fatal(err)
	}
	cashValue, err := domain.NewAccountValue(cashAccount, cashAmount, time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 9, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	coverage.Household = &domain.Household{ID: householdID, BaseCurrency: "USD"}
	coverage.Accounts = []domain.AccountRecord{{Account: cashAccount, LatestValue: &cashValue}}
	coverage.FXPreferences = []domain.FXPreference{preference}
	coverage.FXQuotes = quotes
	selected := selectHistoricalFXQuoteAtMarketDate(preference, quotes, "USD", "SGD", domain.FrankfurterProviderKey, "2026-09-09", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if selected == nil || selected.Rate.Canonical() != "1.34" || historicalFXCoverageCompleteAtMarketDate(coverage, *selected, "2026-09-09") {
		t.Fatalf("pending reference = quote=%+v coverage=%v", selected, historicalFXCoverageCompleteAtMarketDate(coverage, *selected, "2026-09-09"))
	}
	valuation := NewValuationService(nil, func() time.Time { return time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC) })
	valuation.SetHistorical(true)
	valuation.SetHistoricalMarketDate("2026-09-09")
	accountValues, _, err := valuation.ValueAccounts(coverage)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountValues) != 1 || accountValues[0].Complete || len(accountValues[0].MissingInputs) != 1 || accountValues[0].MissingInputs[0].Kind != domain.MissingHistoryCoverage {
		t.Fatalf("pending FX snapshot quality = %+v", accountValues)
	}

	quotes = append(quotes, domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "SGD", Rate: parseRate("1.35"), SourceKind: domain.QuoteSourceProvider, SourceKey: domain.FrankfurterProviderKey, ObservationKind: string(FXObservationDailyReference), EffectiveDate: "2026-09-09", ValueEffectiveAt: time.Date(2026, 9, 10, 0, 30, 0, 0, time.UTC), SourcePolicyVersion: domain.FrankfurterV2BlendedPolicy, TimestampBasis: string(TimestampBasisPolicyDerived), Revision: 1})
	coverage.FXHistoryCoverage[0].DailyReferenceDates = []string{"2026-09-08", "2026-09-09"}
	coverage.FXHistoryCoverage[0].UnverifiedDates = nil
	coverage.FXQuotes = quotes
	selected = selectHistoricalFXQuoteAtMarketDate(preference, quotes, "USD", "SGD", domain.FrankfurterProviderKey, "2026-09-09", time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC))
	if selected == nil || selected.Rate.Canonical() != "1.35" || !historicalFXCoverageCompleteAtMarketDate(coverage, *selected, "2026-09-09") {
		t.Fatalf("verified reference = quote=%+v coverage=%v", selected, historicalFXCoverageCompleteAtMarketDate(coverage, *selected, "2026-09-09"))
	}
	accountValues, _, err = valuation.ValueAccounts(coverage)
	if err != nil {
		t.Fatal(err)
	}
	if len(accountValues) != 1 || !accountValues[0].Complete || len(accountValues[0].MissingInputs) != 0 {
		t.Fatalf("verified FX snapshot quality = %+v", accountValues)
	}
}

func TestSelectFXQuoteUsesTheCurrentProviderSourceKey(t *testing.T) {
	householdID := domain.NewHouseholdID()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	rate, err := domain.ParseFxRate("6.9")
	if err != nil {
		t.Fatal(err)
	}
	preference, err := domain.NewFXPreference(householdID, "USD", "CNY", domain.QuoteSourceProvider, now)
	if err != nil {
		t.Fatal(err)
	}
	oldQuote, err := domain.NewFXQuote(domain.FXQuoteInput{
		HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: "old-provider", QuotedAt: now.Add(time.Hour),
	}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	currentQuote, err := domain.NewFXQuote(domain.FXQuoteInput{
		HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: "frankfurter", QuotedAt: now,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	selected := selectFXQuote(preference, []domain.FXQuote{currentQuote, oldQuote}, "USD", "CNY", "frankfurter", nil)
	if selected == nil || selected.SourceKey != "frankfurter" {
		t.Fatalf("selected FX quote = %+v, want current provider quote", selected)
	}
}

func TestSelectFXQuoteUsesDefaultProviderWhenPreferenceMissing(t *testing.T) {
	householdID := domain.NewHouseholdID()
	now := time.Date(2026, 8, 23, 12, 0, 0, 0, time.UTC)
	rate, err := domain.ParseFxRate("7")
	if err != nil {
		t.Fatal(err)
	}
	quote, err := domain.NewFXQuote(domain.FXQuoteInput{
		HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate,
		SourceKind: domain.QuoteSourceProvider, SourceKey: "frankfurter", QuotedAt: now,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	implicit := domain.FXPreference{HouseholdID: householdID, CurrencyA: "USD", CurrencyB: "CNY", SourceKind: domain.QuoteSourceProvider}
	selected := selectFXQuote(implicit, []domain.FXQuote{quote}, "USD", "CNY", "frankfurter", nil)
	if selected == nil || selected.SourceKey != "frankfurter" {
		t.Fatalf("selected FX quote = %+v, want default provider quote", selected)
	}
	cutoff := now.Add(-time.Hour)
	if later := selectFXQuote(implicit, []domain.FXQuote{quote}, "USD", "CNY", "frankfurter", &cutoff); later != nil {
		t.Fatalf("quote after cutoff was selected: %+v", later)
	}
}

func TestValuationAggregatesFullPrecisionBeforeMoneyBoundaryAndSkipsArchived(t *testing.T) {
	ctx := context.Background()
	database, err := sqlite.Open(t.TempDir() + "/precision.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	repository := sqlite.NewRepository(database)
	service := NewService(repository)
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Precision", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Precision Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset",
		TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"First", "Second"} {
		instrument, createErr := service.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "etf", QuoteCurrency: "CNY", QuoteSource: "manual"})
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: instrument.ID.String(), Quantity: "1"}); createErr != nil {
			t.Fatal(createErr)
		}
		if _, createErr := service.AppendManualInstrumentQuote(ctx, instrument.ID, "0.00005", "2026-08-23", false); createErr != nil {
			t.Fatal(createErr)
		}
	}
	portfolio, err := service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, portfolio.ValuedSubtotal, "0.0001", "CNY")
	accountValue, err := service.AccountValuation(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, accountValue.BaseValue, "0.0001", "CNY")
	for _, component := range accountValue.Components {
		if component.BaseAmountExact != "0.00005" || component.BaseAmount == nil || component.BaseAmount.Amount != "0" {
			t.Fatalf("component precision = %+v", component)
		}
	}
	overview, err := service.Overview(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if !overview.Assets.Equal(decimal.RequireFromString("0.0001")) || !overview.Complete {
		t.Fatalf("precision overview = %+v", overview)
	}

	archived, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Archived", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: archived.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, archived.ID, "999", "2026-08-23", false); err != nil {
		t.Fatal(err)
	}
	withArchived, err := service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	assertMoneyView(t, withArchived.ValuedSubtotal, "999.0001", "CNY")
	if err := service.ArchiveInstrument(ctx, archived.ID, true); err != nil {
		t.Fatal(err)
	}
	afterArchive, err := service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	// Archiving an Instrument stops new writes but retains an active Holding's
	// valuation evidence until the Holding itself is archived or reaches zero.
	assertMoneyView(t, afterArchive.ValuedSubtotal, "999.0001", "CNY")
}

func TestZeroQuantityHoldingIsAvailableWithoutPriceOrFX(t *testing.T) {
	database, err := sqlite.Open(t.TempDir() + "/zero-holding.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	service.setClock(func() time.Time { return now })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Zero", BaseCurrency: "CNY", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "CNY", IncludeInNetWorth: true, IncludeInPortfolio: true,
		Ownership: []domain.OwnershipShare{{MemberID: bootstrap.Members[0].ID, ShareBPS: domain.TotalOwnershipBPS}},
	})
	if err != nil {
		t.Fatal(err)
	}
	zeroInstrument, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Closed Position", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual", Symbol: "CLOSED"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: zeroInstrument.ID.String(), Quantity: "0"}); err != nil {
		t.Fatal(err)
	}
	nonZero, err := service.CreateInstrument(ctx, InstrumentInput{Name: "Needs Price", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreateHolding(ctx, HoldingInput{AccountID: account.Account.ID.String(), InstrumentID: nonZero.ID.String(), Quantity: "1"}); err != nil {
		t.Fatal(err)
	}

	portfolio, err := service.Portfolio(ctx, domain.AccountFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if portfolio.Complete {
		t.Fatal("non-zero holding without a price unexpectedly completed the portfolio")
	}
	if len(portfolio.MissingInputs) != 1 || portfolio.MissingInputs[0].InstrumentID == nil || *portfolio.MissingInputs[0].InstrumentID != nonZero.ID {
		t.Fatalf("missing inputs = %+v, want only the non-zero holding", portfolio.MissingInputs)
	}
	if portfolio.MissingInputs[0].InstrumentName != "Needs Price" {
		t.Fatalf("missing instrument identity = %+v", portfolio.MissingInputs[0])
	}
	accountValue, err := service.AccountValuation(ctx, account.Account.ID)
	if err != nil {
		t.Fatal(err)
	}
	zeroComponent := valuationComponentFor(t, accountValue, zeroInstrument.ID)
	if !zeroComponent.Available || zeroComponent.BaseAmount == nil || zeroComponent.BaseAmount.Amount != "0" || zeroComponent.BaseAmountExact != "0" || zeroComponent.NativeAmount != "0" || zeroComponent.InstrumentName != "Closed Position" || zeroComponent.InstrumentSymbol != "CLOSED" {
		t.Fatalf("zero holding component = %+v", zeroComponent)
	}
}

func valuationComponentFor(t *testing.T, account domain.AccountValuation, instrumentID domain.InstrumentID) domain.ValuationComponent {
	t.Helper()
	for _, component := range account.Components {
		if component.InstrumentID != nil && *component.InstrumentID == instrumentID {
			return component
		}
	}
	t.Fatalf("instrument %s not found in account valuation", instrumentID)
	return domain.ValuationComponent{}
}

func assertMoneyView(t *testing.T, money *domain.MoneyView, amount string, currency domain.CurrencyCode) {
	t.Helper()
	if money == nil || money.Amount != amount || money.Currency != currency {
		t.Fatalf("money view = %+v, want %s %s", money, amount, currency)
	}
}

func TestMakeAllocationsSortsByAmountDescending(t *testing.T) {
	result, err := makeAllocations(
		map[string]decimal.Decimal{"etf": decimal.RequireFromString("100"), "stock": decimal.RequireFromString("900")},
		map[string]string{"etf": "ETF", "stock": "Stock"},
		decimal.RequireFromString("1000"),
		"USD",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 || result[0].Key != "stock" || result[1].Key != "etf" {
		t.Fatalf("order = %+v, want stock then etf", result)
	}
}

func TestSortAccountValuationsOrdersByBaseValueDescending(t *testing.T) {
	accounts := []domain.AccountValuation{
		{Account: domain.Account{Name: "Small"}, BaseValue: &domain.MoneyView{Amount: "100", Currency: "USD"}},
		{Account: domain.Account{Name: "Large"}, BaseValue: &domain.MoneyView{Amount: "900", Currency: "USD"}},
		{Account: domain.Account{Name: "Missing"}},
	}
	sortAccountValuations(accounts)
	if accounts[0].Account.Name != "Large" || accounts[1].Account.Name != "Small" || accounts[2].Account.Name != "Missing" {
		t.Fatalf("order = %+v", accounts)
	}
}
