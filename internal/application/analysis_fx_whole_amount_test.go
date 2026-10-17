package application

import (
	"math/big"
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
	wantEvent, err := domain.ConvertFXAmountExact(amount, native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	wantClose, err := domain.ConvertFXAmountExact(amount, native, base, closeFX)
	if err != nil {
		t.Fatal(err)
	}
	wholeEvent, err := moneyView(decimalAtMoneyBoundary(wantEvent), base)
	if err != nil || wholeEvent.Amount != "2244857.3677" {
		t.Fatalf("whole movement at event FX=%+v err=%v, want 2244857.3677 CNY", wholeEvent, err)
	}
	wantFX := new(big.Rat).Sub(wantClose, wantEvent)
	wantFXDecimal, projectionErr := analysisRationalDecimal(wantFX)
	if projectionErr != nil || !bridge.fx.Equal(wantFXDecimal) || bridge.exactFX.Cmp(wantFX) != 0 {
		t.Fatalf("cash FX impact=%s, want exact full-amount difference %s", bridge.exactFX.RatString(), wantFX.RatString())
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
	wantPrice, err := domain.ConvertFXAmountExact(decimal.RequireFromString(quantity.Canonical()), native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	closingNotional := closingNative
	wantAtClose, err := domain.ConvertFXAmountExact(closingNotional, native, base, closeFX)
	if err != nil {
		t.Fatal(err)
	}
	wantAtBuy, err := domain.ConvertFXAmountExact(closingNotional, native, base, eventFX)
	if err != nil {
		t.Fatal(err)
	}
	wholePrice, err := moneyView(decimalAtMoneyBoundary(wantPrice), base)
	wantPriceDecimal, projectionErr := analysisRationalDecimal(wantPrice)
	if err != nil || projectionErr != nil || wholePrice.Amount != "2244857.3677" || bridge.exactPrice.Cmp(wantPrice) != 0 || !bridge.price.Equal(wantPriceDecimal) {
		t.Fatalf("holding price=%s, direct whole conversion=%+v err=%v, want 2244857.3677 CNY", bridge.price, wholePrice, err)
	}
	wantFX := new(big.Rat).Sub(wantAtClose, wantAtBuy)
	wantFXDecimal, projectionErr := analysisRationalDecimal(wantFX)
	if projectionErr != nil || !bridge.fx.Equal(wantFXDecimal) || bridge.exactFX.Cmp(wantFX) != 0 {
		t.Fatalf("holding FX=%s, want exact whole closing amount difference %s", bridge.exactFX.RatString(), wantFX.RatString())
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
			view, viewErr := signedMoneyView(decimalAtMoneyBoundary(got), "CNY")
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
		view, viewErr := signedMoneyView(decimalAtMoneyBoundary(converted), "CNY")
		if err != nil || viewErr != nil || !available || view.Amount != test.want {
			t.Fatalf("signed tie %s -> %s available=%v err=%v viewErr=%v, want %s", test.amount, view.Amount, available, err, viewErr, test.want)
		}
	}
	maxAmount := decimal.RequireFromString("999999999999.9999")
	if _, available, err := convertAtFXQuote(maxAmount, "USD", "CNY", &direct); err == nil || available {
		t.Fatalf("overflowing direct conversion error=%v available=%v, want propagated range failure", err, available)
	}
}

func TestAnalysisCashBridgeAggregatesInverseFXMovementsExactly(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	accountID := domain.NewAccountID()
	base, native := domain.CurrencyCode("CNY"), domain.CurrencyCode("USD")
	eventAt := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	closeAt := eventAt.Add(time.Hour)
	eventFX := analysisFXQuote(t, householdID, base, native, "6", eventAt)
	closeFX := analysisFXQuote(t, householdID, base, native, "1", closeAt)
	preferences := []domain.FXPreference{{HouseholdID: householdID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}}
	effects := make([]classifiedAnalysisEffect, 9)
	for index := range effects {
		money := mustMoney(t, "0.0001", "USD")
		effects[index] = classifiedAnalysisEffect{
			activity:    domain.Activity{ID: domain.NewActivityID(), EffectiveAt: eventAt.Add(time.Duration(index) * time.Minute)},
			effect:      domain.ActivityEffect{Target: domain.EffectTargetAccountCash, Direction: domain.EffectAdded, Money: &money},
			amountKnown: true,
		}
	}
	previous := domain.DailyValuationSnapshotItem{NativeAmount: "0", NativeCurrency: native}
	current := domain.DailyValuationSnapshotItem{NativeAmount: "0.0009", NativeCurrency: native, FXQuoteID: stringPointer(closeFX.ID.String())}
	input := AnalysisInputs{
		Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: base}, FXPreferences: preferences},
		FXQuotes:  []domain.FXQuote{eventFX, closeFX},
	}
	component := domain.ComponentID{AccountID: accountID, Currency: native}
	bridge, err := computeCashBridge(component, previous, current, eventAt.Add(-time.Hour), closeAt, effects, input, domain.AnalysisQuery{Valuation: domain.ValuationBase}, analysisUniverse{})
	if err != nil || bridge.partial {
		t.Fatalf("nine movement cash bridge=%+v err=%v", bridge, err)
	}
	if bridge.fx.String() != "0.00075" || decimalAtMoneyBoundary(bridge.exactFX).String() != "0.0008" {
		t.Fatalf("nine movement FX=%s exact=%s, want exact 0.00075 and one half-even projection => 0.0008", bridge.fx, bridge.exactFX.RatString())
	}
	if bridge.exactFX.RatString() != "3/4000" {
		t.Fatalf("nine movement exact FX=%s, want 3/4000", bridge.exactFX.RatString())
	}
	for _, test := range []struct {
		name, rate, want string
		direction        domain.EffectDirection
	}{
		{name: "positive below tie", direction: domain.EffectAdded, rate: "1.99", want: "0"},
		{name: "positive above tie", direction: domain.EffectAdded, rate: "2.01", want: "0.0001"},
		{name: "negative tie", direction: domain.EffectRemoved, rate: "2", want: "0"},
		{name: "negative below tie", direction: domain.EffectRemoved, rate: "1.99", want: "0"},
		{name: "negative above tie", direction: domain.EffectRemoved, rate: "2.01", want: "-0.0001"},
	} {
		t.Run(test.name, func(t *testing.T) {
			quotedEvent := analysisFXQuote(t, householdID, base, native, test.rate, eventAt)
			one := mustMoney(t, "0.0001", "USD")
			oneEffect := []classifiedAnalysisEffect{{activity: domain.Activity{EffectiveAt: eventAt}, effect: domain.ActivityEffect{Target: domain.EffectTargetAccountCash, Direction: test.direction, Money: &one}, amountKnown: true}}
			oneCurrent := domain.DailyValuationSnapshotItem{NativeAmount: "0.0001", NativeCurrency: native, FXQuoteID: stringPointer(closeFX.ID.String())}
			input.FXQuotes = []domain.FXQuote{quotedEvent, closeFX}
			bridge, err := computeCashBridge(component, previous, oneCurrent, eventAt.Add(-time.Hour), closeAt, oneEffect, input, domain.AnalysisQuery{Valuation: domain.ValuationBase}, analysisUniverse{})
			money, moneyErr := signedMoneyView(decimalAtMoneyBoundary(bridge.exactFX), base)
			if err != nil || moneyErr != nil || bridge.partial || money.Amount != test.want {
				t.Fatalf("bridge=%+v money=%+v err=%v moneyErr=%v, want FX %s", bridge, money, err, moneyErr, test.want)
			}
		})
	}
}

func TestBuildComponentDayProjectsCashBridgeFromExactFXForBucketsReturnsAndEffects(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, opening, ending, want string
		flows                       []struct {
			amount    string
			direction domain.EffectDirection
			rate      string
		}
	}{
		{
			name: "positive just below half-even tie", opening: "0", ending: "0.0003", want: "0.0001",
			flows: []struct {
				amount    string
				direction domain.EffectDirection
				rate      string
			}{
				{amount: "1", direction: domain.EffectAdded, rate: "9999999.999999999999"},
				{amount: "1", direction: domain.EffectAdded, rate: "10000000.000000000001"},
				{amount: "2", direction: domain.EffectRemoved, rate: "10000000"},
				{amount: "0.0003", direction: domain.EffectAdded, rate: "2"},
			},
		},
		{
			name: "negative just below half-even tie", opening: "2.0003", ending: "2", want: "-0.0001",
			flows: []struct {
				amount    string
				direction domain.EffectDirection
				rate      string
			}{
				{amount: "1", direction: domain.EffectRemoved, rate: "9999999.999999999999"},
				{amount: "1", direction: domain.EffectRemoved, rate: "10000000.000000000001"},
				{amount: "2", direction: domain.EffectAdded, rate: "10000000"},
				{amount: "0.0003", direction: domain.EffectRemoved, rate: "2"},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			householdID := domain.NewHouseholdID()
			account := analysisAccount(householdID, "USD", domain.TrackingBalance, domain.RoleAsset)
			base, native := domain.CurrencyCode("CNY"), domain.CurrencyCode("USD")
			previousCutoff := time.Date(2026, 8, 1, 23, 59, 0, 0, time.UTC)
			currentCutoff := time.Date(2026, 8, 2, 23, 59, 0, 0, time.UTC)
			openFX := analysisFXQuote(t, householdID, base, native, "1", previousCutoff)
			closeFX := analysisFXQuote(t, householdID, base, native, "1", currentCutoff)
			preferences := []domain.FXPreference{{HouseholdID: householdID, CurrencyA: "CNY", CurrencyB: "USD", SourceKind: domain.QuoteSourceManual}}
			quotes := []domain.FXQuote{openFX, closeFX}
			activityEffects := make([]classifiedAnalysisEffect, 0, len(test.flows))
			for index, flow := range test.flows {
				effectiveAt := time.Date(2026, 8, 2, 12, index, 0, 0, time.UTC)
				quote := analysisFXQuote(t, householdID, base, native, flow.rate, effectiveAt)
				quotes = append(quotes, quote)
				money := mustMoney(t, flow.amount, "USD")
				activityEffects = append(activityEffects, classifiedAnalysisEffect{
					activity:    domain.Activity{ID: domain.NewActivityID(), EffectiveAt: effectiveAt},
					effect:      domain.ActivityEffect{Target: domain.EffectTargetAccountCash, Direction: flow.direction, Money: &money},
					amountKnown: true,
				})
			}
			openFXID, closeFXID := openFX.ID.String(), closeFX.ID.String()
			previous := analysisItem(t, account.ID, "USD", test.opening, test.opening, nil, nil, "", openFXID)
			current := analysisItem(t, account.ID, "USD", test.ending, test.ending, nil, nil, "", closeFXID)
			previousSnapshot, currentSnapshot := analysisSnapshot("2026-08-01", previous), analysisSnapshot("2026-08-02", current)
			previousSnapshot.CutoffAt, currentSnapshot.CutoffAt = previousCutoff, currentCutoff
			component := domain.ComponentID{AccountID: account.ID, Currency: native, Cash: true}
			universe := analysisUniverse{
				accounts:                map[domain.AccountID]domain.Account{account.ID: account},
				investmentComponentKeys: map[string]struct{}{component.Key(): {}},
			}
			input := AnalysisInputs{
				Origin: analysisOrigin(t, householdID, "UTC"),
				Portfolio: domain.PortfolioSnapshot{
					Household:     &domain.Household{ID: householdID, BaseCurrency: base},
					FXPreferences: preferences,
				},
				FXQuotes: quotes,
			}
			day, err := buildComponentDay("2026-08-02", component, previous, current, previousSnapshot, currentSnapshot, true, true, activityEffects, input, analysisBaseQuery(domain.ValuationBase), universe)
			if err != nil {
				t.Fatal(err)
			}
			bucket := day.AssetBuckets[domain.BucketFXImpact]
			ret := day.ReturnComponents[domain.ReturnFXImpact]
			var attributed *domain.SignedMoney
			for _, effect := range day.AttributedEffects {
				if effect.AssetBucket != nil && *effect.AssetBucket == domain.BucketFXImpact {
					copy := effect.Amount
					attributed = &copy
				}
			}
			if bucket.Amount().StringFixed(4) != test.want || ret.Amount().StringFixed(4) != test.want || attributed == nil || attributed.Amount().StringFixed(4) != test.want {
				t.Fatalf("bucket=%s return=%s attributed=%v, want all projections %s", bucket.Amount(), ret.Amount(), attributed, test.want)
			}
			if day.ReturnAmount == nil || day.ReturnAmount.Amount().StringFixed(4) != test.want {
				t.Fatalf("daily return amount=%v, want %s", day.ReturnAmount, test.want)
			}
			if got := day.AssetBucketExact[domain.BucketFXImpact]; !got.Equal(decimal.RequireFromString(test.want).Mul(decimal.RequireFromString("1.5"))) {
				t.Fatalf("exact FX bucket=%s, want unrounded bridge about %s", got, decimal.RequireFromString(test.want).Mul(decimal.RequireFromString("1.5")))
			}
		})
	}
}

func TestBuildComponentDayReconcilesNonFXResidualOnDecimalPath(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	account := analysisAccount(householdID, "CNY", domain.TrackingBalance, domain.RoleAsset)
	previous := analysisItem(t, account.ID, "CNY", "100", "100", nil, nil, "", "")
	current := analysisItem(t, account.ID, "CNY", "110", "110", nil, nil, "", "")
	previousSnapshot, currentSnapshot := analysisSnapshot("2026-08-01", previous), analysisSnapshot("2026-08-02", current)
	component := domain.ComponentID{AccountID: account.ID, Currency: "CNY", Cash: true}
	universe := analysisUniverse{accounts: map[domain.AccountID]domain.Account{account.ID: account}}
	input := AnalysisInputs{
		Origin:    analysisOrigin(t, householdID, "UTC"),
		Portfolio: domain.PortfolioSnapshot{Household: &domain.Household{ID: householdID, BaseCurrency: "CNY"}},
	}
	day, err := buildComponentDay("2026-08-02", component, previous, current, previousSnapshot, currentSnapshot, true, true, nil, input, analysisBaseQuery(domain.ValuationBase), universe)
	if err != nil {
		t.Fatal(err)
	}
	if day.Residual == nil || day.Residual.Amount.Amount().StringFixed(4) != "10.0000" || day.AssetBuckets[domain.BucketResidual].Amount().StringFixed(4) != "10.0000" {
		t.Fatalf("base-currency residual=%+v buckets=%+v, want decimal residual 10.0000", day.Residual, day.AssetBuckets)
	}
}

func TestAnalysisExactCashResidualUsesRoundedMoneyForItsBucket(t *testing.T) {
	t.Parallel()
	householdID := domain.NewHouseholdID()
	account := analysisAccount(householdID, "USD", domain.TrackingBalance, domain.RoleAsset)
	base, native := domain.CurrencyCode("CNY"), domain.CurrencyCode("USD")
	previous := analysisSnapshot("2026-08-01", analysisItem(t, account.ID, "USD", "0", "0", nil, nil, "", ""))
	current := analysisSnapshot("2026-08-02", analysisItem(t, account.ID, "USD", "0.1", "0.05", nil, nil, "", ""))
	rates := []struct {
		amount, rate string
		direction    domain.EffectDirection
		class        domain.ActivityClassification
	}{
		{amount: "1", rate: "9999999.999999999999", direction: domain.EffectAdded, class: domain.ClassificationExternalInflow},
		{amount: "1", rate: "10000000.000000000001", direction: domain.EffectAdded, class: domain.ClassificationExternalInflow},
		{amount: "2", rate: "10000000", direction: domain.EffectRemoved, class: domain.ClassificationExternalOutflow},
		{amount: "0.0003", rate: "2", direction: domain.EffectAdded, class: domain.ClassificationExternalInflow},
	}
	activities := make([]domain.Activity, 0, len(rates))
	quotes := make([]domain.FXQuote, 0, len(rates)+1)
	for index, rate := range rates {
		at := time.Date(2026, 8, 2, 12, index, 0, 0, time.UTC)
		activityID := domain.NewActivityID()
		kind := domain.ActivityCashIn
		if rate.direction == domain.EffectRemoved {
			kind = domain.ActivityCashOut
		}
		activities = append(activities, domain.Activity{
			ID: activityID, Kind: kind, Reason: domain.ReasonContribution,
			EffectiveAt: at, EffectiveLocalDate: "2026-08-02",
			Effects: []domain.ActivityEffect{analysisEffect(t, activityID, 1, account.ID, rate.amount, "USD", rate.direction, domain.EffectTargetAccountCash, rate.class)},
		})
		quotes = append(quotes, analysisFXQuote(t, householdID, base, native, rate.rate, at))
	}
	closeFX := analysisFXQuote(t, householdID, base, native, "2", current.CutoffAt)
	quotes = append(quotes, closeFX)
	current.Items[0].FXQuoteID = stringPointer(closeFX.ID.String())
	input := AnalysisInputs{
		Origin: analysisOrigin(t, householdID, "UTC"),
		Portfolio: domain.PortfolioSnapshot{
			Household:     &domain.Household{ID: householdID, BaseCurrency: base},
			Accounts:      []domain.AccountRecord{{Account: account}},
			FXPreferences: []domain.FXPreference{{HouseholdID: householdID, CurrencyA: base, CurrencyB: native, SourceKind: domain.QuoteSourceManual}},
		},
		Snapshots:  []domain.DailyValuationSnapshot{previous, current},
		Activities: activities,
		FXQuotes:   quotes,
	}
	result, err := ComputeAnalysis(input, analysisBaseQuery(domain.ValuationBase))
	if err != nil {
		t.Fatal(err)
	}
	component := domain.ComponentID{AccountID: account.ID, Currency: native, Cash: true}
	day := analysisFindDay(t, result, component)
	if day.Residual == nil || day.Residual.Amount.Amount().StringFixed(4) != "0.0499" {
		t.Fatalf("residual=%+v, want exact residual above the half-even tie to round to 0.0499", day.Residual)
	}
	residualBucket, ok := day.AssetBuckets[domain.BucketResidual]
	if !ok || residualBucket.Amount().StringFixed(4) != day.Residual.Amount.Amount().StringFixed(4) {
		t.Fatalf("residual bucket=%+v residual=%+v, want equal rounded amounts", residualBucket, day.Residual)
	}
	if exact := day.AssetBucketExact[domain.BucketResidual]; !exact.Equal(decimal.RequireFromString("0.04985")) || exact.Equal(residualBucket.Amount()) {
		t.Fatalf("exact residual bucket=%s public residual bucket=%s, want retained bounded decimal 0.04985", exact, residualBucket.Amount())
	}
	directEffects := decimal.Zero
	for _, effect := range day.AttributedEffects {
		if effect.AssetBucket != nil && *effect.AssetBucket == domain.BucketExternalFlow {
			directEffects = directEffects.Add(effect.Amount.Amount())
		}
	}
	directBucket, ok := day.AssetBuckets[domain.BucketExternalFlow]
	if !ok || directBucket.Amount().StringFixed(4) != "0.0002" || !directEffects.Equal(directBucket.Amount()) {
		t.Fatalf("classified direct effects=%s direct bucket=%+v, want matching 0.0002 projections", directEffects, directBucket)
	}
	if exactDirect := day.AssetBucketExact[domain.BucketExternalFlow]; !exactDirect.Equal(decimal.RequireFromString("0.00015")) {
		t.Fatalf("exact classified direct bucket=%s, want unrounded 0.00015", exactDirect)
	}
	exactTotal := decimal.Zero
	for _, amount := range day.AssetBucketExact {
		exactTotal = exactTotal.Add(amount)
	}
	if want := day.EndingValue.Amount().Sub(day.BeginningValue.Amount()); !exactTotal.Equal(want) {
		t.Fatalf("exact effects plus residual=%s, snapshot change=%s; want exact attribution identity", exactTotal, want)
	}
}

func stringPointer(value string) *string { return &value }
