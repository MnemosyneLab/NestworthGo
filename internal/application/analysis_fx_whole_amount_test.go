package application

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func analysisFXQuote(t *testing.T, householdID domain.HouseholdID, base, quote domain.CurrencyCode, rate string, at time.Time) domain.FXQuote {
	t.Helper()
	parsed, err := domain.ParseFxRate(rate)
	if err != nil {
		t.Fatal(err)
	}
	return domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: householdID, BaseCurrency: base, QuoteCurrency: quote, Rate: parsed, SourceKind: domain.QuoteSourceManual, QuotedAt: at, CreatedAt: at}
}

func TestAnalysisCashBridgeUsesWholeAmountConversionForInverseQuotes(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID()
	base, native := domain.CurrencyCode("CNY"), domain.CurrencyCode("USD")
	eventAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closeAt := eventAt.Add(time.Hour)
	eventFX := analysisFXQuote(t, householdID, base, native, "8.123456789012", eventAt)
	closeFX := analysisFXQuote(t, householdID, base, native, "1", closeAt)
	amount := decimal.RequireFromString("18236001.8236")
	previous := domain.DailyValuationSnapshotItem{NativeAmount: "0", NativeCurrency: native}
	current := domain.DailyValuationSnapshotItem{NativeAmount: amount.String(), NativeCurrency: native, FXQuoteID: stringPointer(closeFX.ID.String())}
	money := mustMoney(t, amount.String(), native.String())
	activity := domain.Activity{ID: domain.NewActivityID(), EffectiveAt: eventAt}
	effects := []classifiedAnalysisEffect{{
		activity:    activity,
		effect:      domain.ActivityEffect{Target: domain.EffectTargetAccountCash, Direction: domain.EffectAdded, Money: &money},
		amountKnown: true,
	}}
	input := AnalysisInputs{Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: base}}, FXQuotes: []domain.FXQuote{eventFX, closeFX}}
	if quote, ok := (analysisUniverse{}).fxQuoteForItem(current, input, closeAt); !ok || quote == nil {
		t.Fatalf("cash close quote selection=%+v available=%v id=%q wanted=%q quotes=%+v household=%s base=%s native=%s", quote, ok, *current.FXQuoteID, closeFX.ID.String(), input.FXQuotes, input.Portfolio.Household.ID, base, native)
	}
	component := domain.ComponentID{AccountID: accountID, Currency: native}
	bridge, err := computeCashBridge(component, previous, current, eventAt.Add(-time.Hour), closeAt, effects, input, domain.AnalysisQuery{Valuation: domain.ValuationBase}, analysisUniverse{})
	if err != nil || bridge.partial {
		t.Fatalf("cash bridge=%+v err=%v", bridge, err)
	}
	wantEvent, err := domain.ConvertFXAmount(amount, native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	wantClose, err := domain.ConvertFXAmount(amount, native, base, closeFX)
	if err != nil {
		t.Fatal(err)
	}
	wholeEvent, err := moneyView(wantEvent, base)
	if err != nil || wholeEvent.Amount != "2244857.3677" {
		t.Fatalf("whole movement at event FX=%+v err=%v, want 2244857.3677 CNY", wholeEvent, err)
	}
	if !bridge.fx.Equal(wantClose.Sub(wantEvent)) {
		t.Fatalf("cash FX impact=%s, want full-amount difference %s", bridge.fx, wantClose.Sub(wantEvent))
	}
}

func TestAnalysisHoldingBridgeConvertsWholePriceAndClosingAmounts(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	accountID, holdingID, instrumentID := domain.NewAccountID(), domain.NewHoldingID(), domain.NewInstrumentID()
	base, native := domain.CurrencyCode("CNY"), domain.CurrencyCode("USD")
	eventAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closeAt := eventAt.Add(time.Hour)
	eventFX := analysisFXQuote(t, householdID, base, native, "8.123456789012", eventAt)
	closeFX := analysisFXQuote(t, householdID, base, native, "1", closeAt)
	quantity := mustQuantity(t, "18236001.8236")
	priceAtBuy, closePrice := mustUnitPrice(t, "1"), mustUnitPrice(t, "2")
	closingNative := decimal.RequireFromString(quantity.Canonical()).Mul(closePrice.Decimal())
	closeQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closePrice, Currency: native, SourceKind: domain.QuoteSourceManual, QuotedAt: closeAt}
	previous := domain.DailyValuationSnapshotItem{HoldingID: &holdingID, InstrumentID: &instrumentID, NativeAmount: "0", NativeCurrency: native}
	current := domain.DailyValuationSnapshotItem{HoldingID: &holdingID, InstrumentID: &instrumentID, NativeAmount: closingNative.String(), NativeCurrency: native, QuoteID: stringPointer(closeQuote.ID.String()), FXQuoteID: stringPointer(closeFX.ID.String())}
	gross := mustMoney(t, quantity.Canonical(), native.String())
	activity := domain.Activity{ID: domain.NewActivityID(), Kind: domain.ActivityBuy, EffectiveAt: eventAt, TradeDetail: &domain.TradeDetail{Side: domain.TradeBuy, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: quantity, Gross: gross, UnitPrice: priceAtBuy}}
	classified := []classifiedAnalysisEffect{{activity: activity}}
	input := AnalysisInputs{Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: base}}, FXQuotes: []domain.FXQuote{eventFX, closeFX}, InstrumentQuotes: []domain.InstrumentQuote{closeQuote}}
	if quote, ok := (analysisUniverse{}).fxQuoteForItem(current, input, closeAt); !ok || quote == nil {
		t.Fatalf("holding close FX selection=%+v available=%v id=%q wanted=%q quotes=%+v household=%s base=%s native=%s", quote, ok, *current.FXQuoteID, closeFX.ID.String(), input.FXQuotes, input.Portfolio.Household.ID, base, native)
	}
	component := domain.ComponentID{AccountID: accountID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: native}
	bridge, err := computeHoldingBridge(component, previous, current, eventAt.Add(-time.Hour), closeAt, classified, input, domain.AnalysisQuery{Valuation: domain.ValuationBase}, analysisUniverse{})
	if err != nil || bridge.partial {
		t.Fatalf("holding bridge=%+v err=%v", bridge, err)
	}
	wantPrice, err := domain.ConvertFXAmount(decimal.RequireFromString(quantity.Canonical()), native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	closingNotional := closingNative
	wantAtClose, err := domain.ConvertFXAmount(closingNotional, native, base, closeFX)
	if err != nil {
		t.Fatal(err)
	}
	wantAtBuy, err := domain.ConvertFXAmount(closingNotional, native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	wholePrice, err := moneyView(wantPrice, base)
	if err != nil || wholePrice.Amount != "2244857.3677" || !bridge.price.Equal(wantPrice) {
		t.Fatalf("holding price=%s, direct whole conversion=%+v err=%v, want 2244857.3677 CNY", bridge.price, wholePrice, err)
	}
	if !bridge.fx.Equal(wantAtClose.Sub(wantAtBuy)) {
		t.Fatalf("holding FX=%s, want whole closing amount difference %s", bridge.fx, wantAtClose.Sub(wantAtBuy))
	}
}

func TestAnalysisWholeAmountConversionPreservesDirectionAndPropagatesOverflow(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	at := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	direct := analysisFXQuote(t, householdID, "USD", "CNY", "8.123456789012", at)
	inverse := analysisFXQuote(t, householdID, "CNY", "USD", "8.123456789012", at)
	for _, test := range []struct {
		name, amount, from, want string
		quote                    domain.FXQuote
	}{
		{name: "direct positive", amount: "1.25", from: "USD", want: "10.1543", quote: direct},
		{name: "direct negative", amount: "-1.25", from: "USD", want: "-10.1543", quote: direct},
		{name: "inverse positive", amount: "18236001.8236", from: "USD", want: "2244857.3677", quote: inverse},
		{name: "inverse negative", amount: "-18236001.8236", from: "USD", want: "-2244857.3677", quote: inverse},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, available, err := convertAtFXQuote(decimal.RequireFromString(test.amount), domain.CurrencyCode(test.from), "CNY", &test.quote)
			view, viewErr := signedMoneyView(got, "CNY")
			if err != nil || viewErr != nil || !available || view.Amount != test.want {
				t.Fatalf("whole conversion=%s view=%+v available=%v err=%v viewErr=%v, want %s", got, view, available, err, viewErr, test.want)
			}
		})
	}
	tieQuote := analysisFXQuote(t, householdID, "CNY", "USD", "1", at)
	for _, test := range []struct{ amount, want string }{
		{amount: "0.01015", want: "0.0102"},
		{amount: "0.01005", want: "0.01"},
		{amount: "-0.01015", want: "-0.0102"},
		{amount: "-0.01005", want: "-0.01"},
	} {
		converted, available, err := convertAtFXQuote(decimal.RequireFromString(test.amount), "USD", "CNY", &tieQuote)
		view, viewErr := signedMoneyView(converted, "CNY")
		if err != nil || viewErr != nil || !available || view.Amount != test.want {
			t.Fatalf("signed tie %s -> %s available=%v err=%v viewErr=%v, want %s", test.amount, view.Amount, available, err, viewErr, test.want)
		}
	}
	maxAmount := decimal.RequireFromString("999999999999.9999")
	if _, available, err := convertAtFXQuote(maxAmount, "USD", "CNY", &direct); err == nil || available {
		t.Fatalf("overflowing direct conversion error=%v available=%v, want propagated range failure", err, available)
	}
}

func stringPointer(value string) *string { return &value }
