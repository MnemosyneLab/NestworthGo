package application

import (
	"context"
	"errors"
	"math/big"
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
	if err != nil || priceView.RealizedGain == nil || priceView.Available {
		t.Fatalf("missing-price account = %+v err=%v", priceView, err)
	}
	assertSignedGain(t, priceView.RealizedGain, "70", "CNY")
}

func TestAccountGainHistoricalFXFailureNeverUsesCurrentRateOrPartialSubtotal(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Known and missing historical FX")
	knownSale := f.sale("USD", "2026-08-02", 10, "1", "11")
	knownUSD := f.addHolding(account, "Known USD realized gain", "USD", "1", "1", "", knownSale)
	missingSale := f.sale("EUR", "2026-08-02", 10, "1", "6")
	missingEUR := f.addHolding(account, "Missing EUR historical FX", "EUR", "1", "1", "", missingSale)
	f.addFX("USD", "CNY", "7", "2026-08-02T09:00:00Z")
	currentUSD := f.addFX("USD", "CNY", "9", "2026-08-04T09:00:00Z")
	currentEUR := f.addFX("EUR", "CNY", "10", "2026-08-04T09:00:00Z")
	activeCNY := f.addHolding(account, "Independent base value", "CNY", "1", "2", "3")
	f.inputs.Portfolio.FXQuotes = []domain.FXQuote{currentUSD, currentEUR}
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	if view.RealizedGain != nil || view.Available || !strings.Contains(view.MissingReason, "historical") {
		t.Fatalf("missing historical FX result = %+v, want unavailable historical-FX reason", view)
	}
	if len(view.Holdings) != 3 {
		t.Fatalf("holdings = %+v, want USD, EUR, and independent CNY holdings", view.Holdings)
	}
	if view.Holdings[0].HoldingID != knownUSD.ID || view.Holdings[0].RealizedGain.Amount != "10" ||
		view.Holdings[1].HoldingID != missingEUR.ID || view.Holdings[1].RealizedGain.Amount != "5" {
		t.Fatalf("known native holding gains were lost: %+v", view.Holdings)
	}
	if view.TotalCost == nil || view.TotalCost.Amount != "2" || view.CurrentValue == nil || view.CurrentValue.Amount != "3" || view.UnrealizedGain == nil || view.UnrealizedGain.Amount != "1" {
		t.Fatalf("independent account fields were erased by missing realized FX: %+v", view)
	}
	if view.Holdings[2].HoldingID != activeCNY.ID {
		t.Fatalf("base holding order changed: %+v", view.Holdings)
	}
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

func TestAccountGainKeepsInverseFXSaleRationalsUntilAccountBoundary(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, proceeds, startingCost string
		count                        int
		want                         string
	}{
		{name: "positive three event tie", proceeds: "0.0001", startingCost: "0", count: 3, want: "0"},
		{name: "negative three event tie", proceeds: "0", startingCost: "0.0001", count: 3, want: "0"},
		{name: "below tie", proceeds: "0.0001", startingCost: "0", count: 2, want: "0"},
		{name: "above tie", proceeds: "0.0001", startingCost: "0", count: 4, want: "0.0001"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newGainReadModelFixture(t, "CNY")
			account := f.addAccount("Inverse sale FX")
			events := make([]domain.CostBasisEvent, 0, test.count)
			for index := 0; index < test.count; index++ {
				date := time.Date(2026, 8, 2+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
				events = append(events, f.sale("USD", date, 10, "1", test.proceeds))
			}
			f.addHolding(account, "Synthetic inverse FX sales", "USD", decimal.NewFromInt(int64(test.count)).String(), test.startingCost, "", events...)
			f.addInverseFX("USD", "CNY", "6", f.inputs.Portfolio.Origin.StartedAt)
			view, err := f.service().AccountGain(context.Background(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			assertSignedGain(t, view.RealizedGain, test.want, "CNY")
		})
	}
}

func TestAccountGainAggregatesMixedInverseFXAndHoldingsExactly(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Mixed exact realized FX")
	usdSales := []domain.CostBasisEvent{
		f.sale("USD", "2026-08-02", 10, "1", "0.0001"),
		f.sale("USD", "2026-08-03", 10, "1", "0.0001"),
		f.sale("USD", "2026-08-04", 10, "1", "0.0001"),
	}
	f.addHolding(account, "USD sales A", "USD", "3", "0", "", usdSales...)
	// A second holding contributes the same exact 0.00005 total. Rounding each
	// holding first would lose both halves, while one account boundary yields
	// 0.0001.
	otherUSDSales := []domain.CostBasisEvent{
		f.sale("USD", "2026-08-02", 11, "1", "0.0001"),
		f.sale("USD", "2026-08-03", 11, "1", "0.0001"),
		f.sale("USD", "2026-08-04", 11, "1", "0.0001"),
	}
	f.addHolding(account, "USD sales B", "USD", "3", "0", "", otherUSDSales...)
	f.addInverseFX("USD", "CNY", "6", f.inputs.Portfolio.Origin.StartedAt)
	view, err := f.service().AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "0.0001", "CNY")

	mixed := newGainReadModelFixture(t, "CNY")
	mixedAccount := mixed.addAccount("Mixed native currencies")
	mixed.addHolding(mixedAccount, "USD sale", "USD", "1", "0", "", mixed.sale("USD", "2026-08-02", 10, "1", "0.0001"))
	mixed.addHolding(mixedAccount, "EUR sale", "EUR", "1", "0", "", mixed.sale("EUR", "2026-08-02", 10, "1", "0.0001"))
	mixed.addInverseFX("USD", "CNY", "6", mixed.inputs.Portfolio.Origin.StartedAt)
	mixed.addInverseFX("EUR", "CNY", "3", mixed.inputs.Portfolio.Origin.StartedAt)
	mixedView, err := mixed.service().AccountGain(context.Background(), mixedAccount.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, mixedView.RealizedGain, "0", "CNY")
}

func TestAccountGainInverseTieKeepsPeriodPathScopeSemantics(t *testing.T) {
	t.Parallel()
	f := newGainReadModelFixture(t, "CNY")
	account := f.addAccount("Matching scope inverse tie")
	events := []domain.CostBasisEvent{
		f.sale("USD", "2026-08-02", 10, "1", "0.0001"),
		f.sale("USD", "2026-08-03", 10, "1", "0.0001"),
		f.sale("USD", "2026-08-04", 10, "1", "0.0001"),
	}
	f.addHolding(account, "Three inverse tie sales", "USD", "3", "0", "", events...)
	f.addInverseFX("USD", "CNY", "6", f.inputs.Portfolio.Origin.StartedAt)
	service := f.service()
	view, err := service.AccountGain(context.Background(), account.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSignedGain(t, view.RealizedGain, "0", "CNY")
	period, err := service.RealizedGainInRange(context.Background(), domain.GainScope{AccountID: &account.ID}, "2026-08-02", "2026-08-04")
	if err != nil || !period.Available || period.Total == nil {
		t.Fatalf("matching-scope period = %+v err=%v", period, err)
	}
	// The period endpoint still projects each conversion through its existing
	// finite-decimal path, which lies just above this exact half-even tie. This
	// related behavior is intentionally kept in scope as a documented limit of
	// this repair batch rather than silently changing period endpoint semantics.
	assertSignedGain(t, period.Total, "0.0001", "CNY")
}

func TestAccountGainRejectsExactAggregateJustOverMoneyLimit(t *testing.T) {
	t.Parallel()
	const max = "999999999999.9999"
	t.Run("cost", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Cost aggregate range")
		f.addHolding(account, "At limit", "CNY", max, "1", "0")
		f.addHolding(account, "Just over", "CNY", "0.00001", "1", "0")
		_, err := f.service().AccountGain(context.Background(), account.ID)
		assertExactMoneyOverflow(t, err)
	})
	t.Run("current value", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Value aggregate range")
		f.addHolding(account, "At limit", "CNY", max, "0", "1")
		f.addHolding(account, "Just over", "CNY", "0.00000001", "0", "1")
		_, err := f.service().AccountGain(context.Background(), account.ID)
		assertExactMoneyOverflow(t, err)
	})
	t.Run("exact limit accepted", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Exact aggregate limit")
		f.addHolding(account, "At limit", "CNY", max, "1", "0")
		view, err := f.service().AccountGain(context.Background(), account.ID)
		if err != nil || view.TotalCost == nil || view.TotalCost.Amount != max {
			t.Fatalf("exact limit account gain=%+v err=%v, want %s CNY", view, err, max)
		}
	})
	t.Run("realized exact limit accepted", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Exact realized limit")
		sale := f.sale("CNY", "2026-08-02", 10, "1", max)
		f.addHolding(account, "At limit realized gain", "CNY", "1", "0", "0", sale)
		view, err := f.service().AccountGain(context.Background(), account.ID)
		if err != nil || view.RealizedGain == nil || view.RealizedGain.Amount != max {
			t.Fatalf("exact realized limit account gain=%+v err=%v, want %s CNY", view.RealizedGain, err, max)
		}
	})
}

func TestAccountGainRejectsPositiveAndNegativeRealizedAggregateBeyondMoneyLimit(t *testing.T) {
	t.Parallel()
	const max = "999999999999.9999"
	t.Run("positive", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Positive realized aggregate")
		f.addHolding(account, "Maximum gain", "CNY", "1", "0", "0", f.sale("CNY", "2026-08-02", 10, "1", max))
		f.addHolding(account, "Additional gain", "CNY", "1", "0", "0", f.sale("CNY", "2026-08-02", 11, "1", "0.0001"))
		_, err := f.service().AccountGain(context.Background(), account.ID)
		assertExactMoneyOverflow(t, err)
	})
	t.Run("negative", func(t *testing.T) {
		f := newGainReadModelFixture(t, "CNY")
		account := f.addAccount("Negative realized aggregate")
		f.addHolding(account, "Maximum loss", "CNY", "1", max, "0", f.sale("CNY", "2026-08-02", 10, "1", "0"))
		f.addHolding(account, "Additional loss", "CNY", "0.0001", "1", "0", f.sale("CNY", "2026-08-02", 11, "0.0001", "0"))
		_, err := f.service().AccountGain(context.Background(), account.ID)
		assertExactMoneyOverflow(t, err)
	})
}

func assertExactMoneyOverflow(t *testing.T, err error) {
	t.Helper()
	var problem *domain.Error
	if !errors.As(err, &problem) || problem.Code != domain.ErrDecimalOverflow {
		t.Fatalf("aggregate range error=%v, want ErrDecimalOverflow", err)
	}
}

func TestRationalMoneyViewsCheckExactRangeBeforeRounding(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, value, want string
	}{
		{name: "positive limit", value: "999999999999.9999", want: "999999999999.9999"},
		{name: "negative limit", value: "-999999999999.9999", want: "-999999999999.9999"},
		{name: "positive tie", value: "0.01005", want: "0.01"},
		{name: "positive odd tie", value: "0.01015", want: "0.0102"},
		{name: "negative tie", value: "-0.01005", want: "-0.01"},
		{name: "negative odd tie", value: "-0.01015", want: "-0.0102"},
		{name: "zero", value: "0", want: "0"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value, ok := new(big.Rat).SetString(test.value)
			if !ok {
				t.Fatal("invalid rational test input")
			}
			view, err := rationalSignedMoneyView(value, "CNY")
			if err != nil || view.Amount != test.want {
				t.Fatalf("rational view=%+v err=%v, want %s", view, err, test.want)
			}
		})
	}
	for _, value := range []string{"999999999999.99991", "-999999999999.99991"} {
		rational, _ := new(big.Rat).SetString(value)
		if _, err := rationalSignedMoneyView(rational, "CNY"); err == nil {
			t.Fatalf("rational value %s rounded inside the domain limit", value)
		} else {
			assertExactMoneyOverflow(t, err)
		}
	}
}
