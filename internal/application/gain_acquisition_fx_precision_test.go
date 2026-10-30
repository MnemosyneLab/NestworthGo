package application

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (f *gainReadModelFixture) addInverseFX(native, base domain.CurrencyCode, rate string, at time.Time) domain.FXQuote {
	f.t.Helper()
	parsed, err := domain.ParseFxRate(rate)
	if err != nil {
		f.t.Fatal(err)
	}
	quote := domain.FXQuote{
		ID: domain.NewFXQuoteID(), HouseholdID: f.inputs.Portfolio.Household.ID,
		BaseCurrency: base, QuoteCurrency: native, Rate: parsed, SourceKind: domain.QuoteSourceManual,
		QuotedAt: at, CreatedAt: at,
	}
	f.inputs.HistoricalFXQuotes = append(f.inputs.HistoricalFXQuotes, quote)
	f.inputs.Portfolio.FXQuotes = append(f.inputs.Portfolio.FXQuotes, quote)
	currencyA, currencyB, err := domain.NormalizeFXPair(native, base)
	if err != nil {
		f.t.Fatal(err)
	}
	f.inputs.Portfolio.FXPreferences = append(f.inputs.Portfolio.FXPreferences, domain.FXPreference{
		HouseholdID: f.inputs.Portfolio.Household.ID, CurrencyA: currencyA, CurrencyB: currencyB, SourceKind: domain.QuoteSourceManual,
	})
	return quote
}

func TestAccountGainAcquisitionUsesOneWholeAmountInverseConversion(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, amount, rate, want string
	}{
		{name: "reported precision example", amount: "18236001.8236", rate: "8.123456789012", want: "2244857.3677"},
		{name: "maximum valid amount", amount: "999999999999.9999", rate: "7", want: "142857142857.1428"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := newGainReadModelFixture(t, "CNY")
			account := f.addAccount("Synthetic acquisition")
			f.addHolding(account, "Synthetic USD asset", "USD", test.amount, "1", "1")
			f.addInverseFX("USD", "CNY", test.rate, f.inputs.Portfolio.Origin.StartedAt)
			view, err := f.service().AccountGain(context.Background(), account.ID)
			if err != nil {
				t.Fatal(err)
			}
			if view.TotalCost == nil || view.TotalCost.Amount != test.want || !view.Available {
				t.Fatalf("account cost = %+v, available=%v reason=%q; want %s CNY", view.TotalCost, view.Available, view.MissingReason, test.want)
			}
		})
	}
}

func TestAcquisitionFXWeightedRatioRemainsExactAcrossCorrectionAndSale(t *testing.T) {
	t.Parallel()
	acquired := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	corrected := acquired.Add(24 * time.Hour)
	added := acquired.Add(48 * time.Hour)
	sold := acquired.Add(72 * time.Hour)
	householdID := domain.NewHouseholdID()
	snapshot := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}, FXPreferences: []domain.FXPreference{{HouseholdID: householdID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}}}
	rate7, _ := domain.ParseFxRate("7")
	rate8, _ := domain.ParseFxRate("8")
	quotes := []domain.FXQuote{
		{HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate7, SourceKind: domain.QuoteSourceManual, QuotedAt: acquired},
		{HouseholdID: householdID, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: rate8, SourceKind: domain.QuoteSourceManual, QuotedAt: added},
	}
	quantity1 := reconciliationQuantity(t, "1")
	quantity2 := reconciliationQuantity(t, "2")
	zero := reconciliationQuantity(t, "0")
	initialCost := reconciliationUnitCost(t, "10")
	correctedCost := reconciliationUnitCost(t, "12")
	events := []domain.CostBasisEvent{
		{Kind: domain.CostBasisStartingPoint, Quantity: quantity1, EffectiveAt: acquired},
		{Kind: domain.CostBasisCostAdjustment, Quantity: zero, UnitCost: &correctedCost, EffectiveAt: corrected},
		{Kind: domain.CostBasisBuy, Quantity: quantity2, UnitCost: &correctedCost, EffectiveAt: added},
		{Kind: domain.CostBasisSell, Quantity: quantity1, EffectiveAt: sold},
	}
	replay := &costBasisReplayContext{events: map[domain.HoldingID][]domain.CostBasisEvent{}, starting: map[domain.HoldingID]*domain.UnitPrice{}}
	service := NewGainService(nil)
	got, available, err := service.acquisitionFXRatio(context.Background(), snapshot, "USD", &initialCost, events, replay, quotes, nil)
	if err != nil || !available || got.RatString() != "23/3" {
		t.Fatalf("weighted acquisition ratio = %v available=%v err=%v; want exact 23/3", got, available, err)
	}
	// Assert the exact ratio projects a large final cost at the money boundary,
	// rather than a rounded sixteen-place intermediate unit rate.
	nativeCost := decimal.RequireFromString("999999999999.9999")
	projected := decimalAtMoneyBoundary(new(big.Rat).Mul(rationalFromDecimal(nativeCost), got))
	oracle := new(big.Rat).Mul(rationalFromDecimal(nativeCost), new(big.Rat).SetFrac64(23, 3))
	if !projected.Equal(decimalAtMoneyBoundary(oracle)) {
		t.Fatalf("exact weighted cost = %s, oracle = %s", projected, decimalAtMoneyBoundary(oracle))
	}
}

func TestAcquisitionFXRatioPropagatesTransferReplayFailure(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	snapshot := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}}
	quantity := reconciliationQuantity(t, "1")
	sourceID := domain.NewHoldingID()
	events := []domain.CostBasisEvent{{Kind: domain.CostBasisTransferIn, Quantity: quantity, SourceHoldingID: &sourceID, EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)}}
	replay := &costBasisReplayContext{events: make(map[domain.HoldingID][]domain.CostBasisEvent), starting: make(map[domain.HoldingID]*domain.UnitPrice)}
	_, available, err := NewGainService(nil).acquisitionFXRatio(context.Background(), snapshot, "USD", nil, events, replay, nil, nil)
	var problem *domain.Error
	if available || !errors.As(err, &problem) || problem.Code != domain.ErrIntegrity {
		t.Fatalf("transfer replay failure: available=%v err=%v, want propagated ErrIntegrity", available, err)
	}
}

func TestAcquisitionFXRatioCancellationIsPropagated(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	snapshot := domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}}
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		t.Fatal(err)
	}
	starting := domain.CostBasisEvent{Kind: domain.CostBasisStartingPoint, Quantity: quantity, EffectiveAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, available, err := NewGainService(nil).acquisitionFXRatio(ctx, snapshot, "USD", nil, []domain.CostBasisEvent{starting}, &costBasisReplayContext{}, nil, nil)
	if available || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled acquisition ratio available=%v err=%v, want context.Canceled", available, err)
	}
}

func BenchmarkAcquisitionFXRatioManyInverseQuotes(b *testing.B) {
	householdID := domain.NewHouseholdID()
	start := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	snapshot := domain.PortfolioSnapshot{
		Household:     &domain.Household{ID: householdID, BaseCurrency: "CNY"},
		FXPreferences: []domain.FXPreference{{HouseholdID: householdID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}},
	}
	quantity, err := domain.ParseQuantity("1")
	if err != nil {
		b.Fatal(err)
	}
	sellQuantity, err := domain.ParseQuantity("8")
	if err != nil {
		b.Fatal(err)
	}
	remainingSell, err := domain.ParseQuantity("9")
	if err != nil {
		b.Fatal(err)
	}
	quotes := make([]domain.FXQuote, 32)
	events := make([]domain.CostBasisEvent, 0, 34)
	for index := 0; index < 32; index++ {
		at := start.Add(time.Duration(index) * time.Hour)
		rate, parseErr := domain.ParseFxRate(fmt.Sprintf("6.%012d", index+1))
		if parseErr != nil {
			b.Fatal(parseErr)
		}
		quotes[index] = domain.FXQuote{
			ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: "CNY", QuoteCurrency: "USD",
			Rate: rate, SourceKind: domain.QuoteSourceManual, QuotedAt: at, CreatedAt: at,
		}
		kind := domain.CostBasisBuy
		if index == 0 {
			kind = domain.CostBasisStartingPoint
		} else {
			events = append(events, domain.CostBasisEvent{Kind: kind, Quantity: quantity, EffectiveAt: at})
			continue
		}
		events = append(events, domain.CostBasisEvent{Kind: kind, Quantity: quantity, EffectiveAt: at})
	}
	events = append(events,
		domain.CostBasisEvent{Kind: domain.CostBasisSell, Quantity: sellQuantity, EffectiveAt: start.Add(33 * time.Hour)},
		domain.CostBasisEvent{Kind: domain.CostBasisSell, Quantity: remainingSell, EffectiveAt: start.Add(34 * time.Hour)},
	)
	startingCost, err := domain.ParseUnitPrice("1")
	if err != nil {
		b.Fatal(err)
	}
	for index := range events {
		if events[index].Kind == domain.CostBasisBuy {
			events[index].UnitCost = &startingCost
		}
	}
	replay := &costBasisReplayContext{events: make(map[domain.HoldingID][]domain.CostBasisEvent), starting: make(map[domain.HoldingID]*domain.UnitPrice)}
	service := NewGainService(nil)
	ctx := context.Background()
	result, available, err := service.acquisitionFXRatio(ctx, snapshot, "USD", &startingCost, events, replay, quotes, nil)
	if err != nil || !available {
		b.Fatalf("benchmark setup acquisition ratio available=%v err=%v", available, err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		if _, available, err := service.acquisitionFXRatio(ctx, snapshot, "USD", &startingCost, events, replay, quotes, nil); err != nil || !available {
			b.Fatalf("acquisition ratio available=%v err=%v", available, err)
		}
	}
	b.ReportMetric(float64(len(events)), "events/op")
	b.ReportMetric(float64(result.Denom().BitLen()), "denom-bits")
}
