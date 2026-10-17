package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fixedGainSnapshotRepository struct {
	Repository
	inputs domain.GainSnapshot
}

func (r fixedGainSnapshotRepository) ReadGainSnapshot(context.Context) (domain.GainSnapshot, error) {
	return r.inputs, nil
}

type gainReadModelFixture struct {
	t       *testing.T
	inputs  domain.GainSnapshot
	account map[string]domain.Account
	now     time.Time
}

func newGainReadModelFixture(t *testing.T, base domain.CurrencyCode) *gainReadModelFixture {
	t.Helper()
	householdID := domain.NewHouseholdID()
	startedAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	origin, err := domain.NewHistoryOrigin(householdID, "UTC", startedAt, startedAt)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	return &gainReadModelFixture{
		t: t, now: now, account: make(map[string]domain.Account),
		inputs: domain.GainSnapshot{Portfolio: domain.PortfolioSnapshot{
			Household: &domain.Household{ID: householdID, BaseCurrency: base}, Origin: &origin,
		}},
	}
}

func (f *gainReadModelFixture) addAccount(name string) domain.Account {
	f.t.Helper()
	account := domain.Account{
		ID: domain.NewAccountID(), HouseholdID: f.inputs.Portfolio.Household.ID, Name: name,
		AccountType: domain.TypeBrokerage, BalanceSheetRole: domain.RoleAsset, TrackingMode: domain.TrackingHoldings,
		DefaultCurrency: f.inputs.Portfolio.Household.BaseCurrency, IncludeInNetWorth: true, IncludeInPortfolio: true,
	}
	f.account[name] = account
	f.inputs.Portfolio.Accounts = append(f.inputs.Portfolio.Accounts, domain.AccountRecord{Account: account})
	return account
}

func (f *gainReadModelFixture) addHolding(account domain.Account, name string, currency domain.CurrencyCode, startingQuantity, startingCost, currentQuote string, events ...domain.CostBasisEvent) domain.Holding {
	f.t.Helper()
	quantity, err := domain.ParseQuantity(startingQuantity)
	if err != nil {
		f.t.Fatal(err)
	}
	cost, err := domain.ParseUnitPrice(startingCost)
	if err != nil {
		f.t.Fatal(err)
	}
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: account.HouseholdID, Name: name, Type: domain.InstrumentETF, QuoteCurrency: currency, QuoteSource: domain.QuoteSourceManual}
	costEvents := append([]domain.CostBasisEvent{{Kind: domain.CostBasisStartingPoint, Quantity: quantity, EffectiveAt: f.inputs.Portfolio.Origin.StartedAt, UnitCost: &cost}}, events...)
	replayed, err := domain.ReplayCostBasis(&cost, costEvents)
	if err != nil {
		f.t.Fatal(err)
	}
	holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID, Quantity: replayed.Current.Quantity}
	f.inputs.Portfolio.Instruments = append(f.inputs.Portfolio.Instruments, instrument)
	f.inputs.Portfolio.Holdings = append(f.inputs.Portfolio.Holdings, holding)
	f.inputs.StartingCosts = ensureGainMap(f.inputs.StartingCosts)
	f.inputs.StartingCosts[holdingID] = &cost
	f.inputs.CostEvents = ensureGainEventMap(f.inputs.CostEvents)
	f.inputs.CostEvents[holdingID] = costEvents
	if currentQuote != "" {
		price, priceErr := domain.ParseUnitPrice(currentQuote)
		if priceErr != nil {
			f.t.Fatal(priceErr)
		}
		f.inputs.Portfolio.InstrumentQuotes = append(f.inputs.Portfolio.InstrumentQuotes, domain.InstrumentQuote{
			ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: price, Currency: currency,
			SourceKind: domain.QuoteSourceManual, QuotedAt: f.now, CreatedAt: f.now,
		})
	}
	return holding
}

func ensureGainMap(source map[domain.HoldingID]*domain.UnitPrice) map[domain.HoldingID]*domain.UnitPrice {
	if source == nil {
		return make(map[domain.HoldingID]*domain.UnitPrice)
	}
	return source
}

func ensureGainEventMap(source map[domain.HoldingID][]domain.CostBasisEvent) map[domain.HoldingID][]domain.CostBasisEvent {
	if source == nil {
		return make(map[domain.HoldingID][]domain.CostBasisEvent)
	}
	return source
}

func (f *gainReadModelFixture) sale(currency domain.CurrencyCode, date string, hour int, quantity, proceeds string) domain.CostBasisEvent {
	f.t.Helper()
	parsedQuantity, err := domain.ParseQuantity(quantity)
	if err != nil {
		f.t.Fatal(err)
	}
	unitPrice, err := domain.ParseUnitPrice(proceeds)
	if err != nil {
		f.t.Fatal(err)
	}
	when, err := time.Parse(time.RFC3339, date+"T00:00:00Z")
	if err != nil {
		f.t.Fatal(err)
	}
	when = when.Add(time.Duration(hour) * time.Hour)
	return domain.CostBasisEvent{ActivityID: domain.NewActivityID(), Kind: domain.CostBasisSell, Quantity: parsedQuantity, UnitPrice: &unitPrice, Currency: currency, EffectiveAt: when}
}

func (f *gainReadModelFixture) addFX(native, base domain.CurrencyCode, rate, at string) domain.FXQuote {
	f.t.Helper()
	parsedRate, err := domain.ParseFxRate(rate)
	if err != nil {
		f.t.Fatal(err)
	}
	quotedAt, err := time.Parse(time.RFC3339, at)
	if err != nil {
		f.t.Fatal(err)
	}
	quote := domain.FXQuote{
		ID: domain.NewFXQuoteID(), HouseholdID: f.inputs.Portfolio.Household.ID,
		BaseCurrency: native, QuoteCurrency: base, Rate: parsedRate, SourceKind: domain.QuoteSourceManual,
		QuotedAt: quotedAt, CreatedAt: quotedAt,
	}
	f.inputs.HistoricalFXQuotes = append(f.inputs.HistoricalFXQuotes, quote)
	currencyA, currencyB, err := domain.NormalizeFXPair(native, base)
	if err != nil {
		f.t.Fatal(err)
	}
	f.inputs.Portfolio.FXPreferences = append(f.inputs.Portfolio.FXPreferences, domain.FXPreference{
		HouseholdID: f.inputs.Portfolio.Household.ID, CurrencyA: currencyA, CurrencyB: currencyB, SourceKind: domain.QuoteSourceManual,
	})
	return quote
}

func (f *gainReadModelFixture) service() *GainService {
	return NewGainService(fixedGainSnapshotRepository{inputs: f.inputs}, func() time.Time { return f.now })
}

func assertSignedGain(t *testing.T, got *domain.SignedMoneyView, amount string, currency domain.CurrencyCode) {
	t.Helper()
	if got == nil || got.Amount != amount || got.Currency != currency {
		t.Fatalf("gain = %+v, want %s %s", got, amount, currency)
	}
}

func TestAccountGainUsesEachSaleHistoricalFXAndMatchesPeriodWithinSameScope(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	accountA, accountB := f.addAccount("Primary"), f.addAccount("Secondary")
	sale1 := f.sale("USD", "2026-08-02", 10, "1", "11")
	sale2 := f.sale("USD", "2026-08-03", 10, "1", "11")
	holdingA := f.addHolding(accountA, "Synthetic USD holding", "USD", "2", "1", "", sale1, sale2)
	saleB := f.sale("USD", "2026-08-02", 10, "1", "6")
	f.addHolding(accountB, "Synthetic second holding", "USD", "1", "1", "", saleB)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	f.addFX("USD", "CNY", "8", "2026-08-03T09:00:00Z")
	f.addFX("USD", "CNY", "9", "2026-08-04T09:00:00Z")
	// The newer current quote is intentionally distinct; sale conversion must
	// remain tied to the FX available at each sale's EffectiveAt.
	f.inputs.Portfolio.FXQuotes = []domain.FXQuote{f.inputs.HistoricalFXQuotes[2]}
	service := f.service()
	views, err := service.AccountGains(context.Background(), nil)
	if err != nil || len(views) != 2 {
		t.Fatalf("AccountGains = %+v err=%v", views, err)
	}
	byAccount := map[domain.AccountID]domain.AccountGainView{}
	for _, view := range views {
		byAccount[view.AccountID] = view
	}
	primary := byAccount[accountA.ID]
	assertSignedGain(t, primary.RealizedGain, "150", "CNY")
	if len(primary.Holdings) != 1 || primary.Holdings[0].HoldingID != holdingA.ID {
		t.Fatalf("primary holdings = %+v", primary.Holdings)
	}
	if primary.Holdings[0].RealizedGain.Amount != "20" || primary.Holdings[0].RealizedGain.Currency != "USD" {
		t.Fatalf("holding realized gain = %+v, want native USD 20", primary.Holdings[0].RealizedGain)
	}
	assertSignedGain(t, byAccount[accountB.ID].RealizedGain, "35", "CNY")
	for _, account := range []domain.Account{accountA, accountB} {
		one, singleErr := service.AccountGain(context.Background(), account.ID)
		if singleErr != nil {
			t.Fatal(singleErr)
		}
		assertSignedGain(t, one.RealizedGain, byAccount[account.ID].RealizedGain.Amount, "CNY")
	}
	period, err := service.RealizedGainInRange(context.Background(), domain.GainScope{AccountID: &accountA.ID}, "2026-08-02", "2026-08-03")
	if err != nil || !period.Available || len(period.ByAccount) != 1 {
		t.Fatalf("matching-scope period = %+v err=%v", period, err)
	}
	assertSignedGain(t, period.Total, "150", "CNY")
	assertSignedGain(t, &period.ByAccount[0].Gain, "150", "CNY")
}

func TestAccountGainKeepsKnownRealizedGainIndependentOfCurrentInputs(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Synthetic account")
	sale := f.sale("USD", "2026-08-02", 10, "1", "11")
	holding := f.addHolding(account, "Unpriced synthetic holding", "USD", "2", "1", "12", sale)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	// Current valuation has a price but no current FX. The historical sale FX
	// remains sufficient to report the realized gain.
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.Available || view.MissingReason != "current foreign-exchange rate is unavailable" {
		t.Fatalf("current availability = %+v, want missing current FX", view)
	}
	assertSignedGain(t, view.RealizedGain, "70", "CNY")
	if len(view.Holdings) != 1 || view.Holdings[0].HoldingID != holding.ID || view.Holdings[0].RealizedGain.Amount != "10" {
		t.Fatalf("native holding gain = %+v", view.Holdings)
	}

	priceMissing := newGainReadModelFixture(t, "CNY")
	priceMissingAccount := priceMissing.addAccount("Missing price")
	priceSale := priceMissing.sale("USD", "2026-08-02", 10, "1", "11")
	priceMissing.addHolding(priceMissingAccount, "No current price", "USD", "2", "1", "", priceSale)
	priceMissing.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	priceView, err := priceMissing.service().AccountGain(context.Background(), priceMissingAccount.ID)
	if err != nil || view.RealizedGain == nil || priceView.Available {
		t.Fatalf("missing-price account = %+v err=%v", priceView, err)
	}
	assertSignedGain(t, priceView.RealizedGain, "70", "CNY")
}

func TestAccountGainHistoricalFXFailureNeverUsesCurrentRateOrPartialSubtotal(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Missing history FX")
	sale1 := f.sale("USD", "2026-08-02", 10, "1", "11")
	sale2 := f.sale("USD", "2026-08-03", 10, "1", "11")
	holding := f.addHolding(account, "Synthetic USD holding", "USD", "2", "1", "", sale1, sale2)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	current := f.addFX("USD", "CNY", "9", "2026-08-04T09:00:00Z")
	f.inputs.HistoricalFXQuotes = []domain.FXQuote{current}
	f.inputs.Portfolio.FXQuotes = []domain.FXQuote{current}
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.RealizedGain != nil || view.Available || !strings.Contains(view.MissingReason, "historical") {
		t.Fatalf("missing historical FX result = %+v, want unavailable historical-FX reason", view)
	}
	if len(view.Holdings) != 1 || view.Holdings[0].RealizedGain.Amount != "20" {
		t.Fatalf("native realized gain was lost: %+v", view.Holdings)
	}
	_ = holding
}

func TestAccountGainKnownZeroNeedsNoHistoricalFXAndBaseCurrencyNeedsNoQuote(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("No sales")
	f.addHolding(account, "Unsold foreign holding", "USD", "1", "1", "")
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "0", "CNY")

	base := newGainReadModelFixture(t, "CNY")
	baseAccount := base.addAccount("Base currency")
	baseSale := base.sale("CNY", "2026-08-02", 10, "1", "11")
	base.addHolding(baseAccount, "Synthetic base holding", "CNY", "2", "1", "", baseSale)
	baseView, err := base.service().AccountGain(context.Background(), baseAccount.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, baseView.RealizedGain, "10", "CNY")
}

func TestAccountGainAggregatesLossesAcrossNativeCurrencies(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Mixed currencies")
	usdLoss := f.sale("USD", "2026-08-02", 10, "1", "0")
	eurGain := f.sale("EUR", "2026-08-03", 10, "1", "6")
	f.addHolding(account, "USD loss", "USD", "1", "10", "", usdLoss)
	f.addHolding(account, "EUR gain", "EUR", "1", "1", "", eurGain)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	f.addFX("EUR", "CNY", "9", "2026-08-03T09:00:00Z")
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "-25", "CNY")
}

func TestAccountGainHonorsReplayReversalAndCorrection(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Corrections")
	original := f.sale("USD", "2026-08-02", 10, "1", "11")
	reversal := f.sale("USD", "2026-08-02", 11, "1", "11")
	reversal.ReversesActivityID = &original.ActivityID
	replacement := f.sale("USD", "2026-08-03", 10, "1", "6")
	f.addHolding(account, "Corrected sale", "USD", "1", "1", "", original, reversal, replacement)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	f.addFX("USD", "CNY", "8", "2026-08-03T09:00:00Z")
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "40", "CNY")
	if len(view.Holdings) != 1 || view.Holdings[0].RealizedGain.Amount != "5" {
		t.Fatalf("replayed native gain = %+v", view.Holdings)
	}
}

func TestAccountGainArchiveScopeExcludesArchivedHoldingsButKeepsArchivedInstruments(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Archive scope")
	archivedInstrumentSale := f.sale("USD", "2026-08-02", 10, "1", "11")
	activeArchivedInstrument := f.addHolding(account, "Archived instrument still held", "USD", "1", "1", "", archivedInstrumentSale)
	archivedAt := time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)
	f.inputs.Portfolio.Instruments[0].ArchivedAt = &archivedAt
	archivedHoldingSale := f.sale("USD", "2026-08-02", 11, "1", "101")
	archivedHolding := f.addHolding(account, "Archived holding", "USD", "1", "1", "", archivedHoldingSale)
	f.inputs.Portfolio.Holdings[1].ArchivedAt = &archivedAt
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "70", "CNY")
	if len(view.Holdings) != 1 || view.Holdings[0].HoldingID != activeArchivedInstrument.ID {
		t.Fatalf("account holdings = %+v, archived holding %s should be excluded", view.Holdings, archivedHolding.ID)
	}
	// Period realized gain intentionally uses the historical event scope and
	// includes archived holdings, so compare its value independently here.
	period, err := f.service().RealizedGainInRange(context.Background(), domain.GainScope{AccountID: &account.ID}, "2026-08-02", "2026-08-02")
	if err != nil || period.Total == nil || period.Total.Amount != "770" {
		t.Fatalf("period archive scope = %+v err=%v, want both immutable holding histories", period, err)
	}
}

func TestAccountGainPropagatesFXConversionOverflow(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Overflow")
	sale := f.sale("USD", "2026-08-02", 10, "1", "999999999999.9999")
	f.addHolding(account, "Maximum gain", "USD", "1", "0", "", sale)
	f.addFX("USD", "CNY", "2", "2026-08-02T09:00:00Z")
	_, err := f.service().AccountGain(context.Background(), account.ID)
	var problem *domain.Error
	if !errors.As(err, &problem) || problem.Code != domain.ErrDecimalOverflow {
		t.Fatalf("overflow error = %v, want ErrDecimalOverflow", err)
	}
}

func TestAccountGainRetainsFractionalAggregateUntilOneMoneyBoundary(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Fractional aggregate")
	sale1 := f.sale("USD", "2026-08-02", 10, "1", "1.0001")
	sale2 := f.sale("USD", "2026-08-03", 10, "1", "1.0001")
	f.addHolding(account, "Fractional sales", "USD", "2", "1", "", sale1, sale2)
	f.addFX("USD", "CNY", "0.5", "2026-08-02T09:00:00Z")
	f.addFX("USD", "CNY", "0.5", "2026-08-03T09:00:00Z")
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Each event's exact converted gain is 0.00005; summed first, the result
	// rounds once to 0.0001. Per-event Money projection would produce zero.
	assertSignedGain(t, view.RealizedGain, "0.0001", "CNY")
	if !decimal.RequireFromString("0.00005").Add(decimal.RequireFromString("0.00005")).Equal(decimal.RequireFromString("0.0001")) {
		t.Fatal("synthetic decimal oracle is inconsistent")
	}
}
