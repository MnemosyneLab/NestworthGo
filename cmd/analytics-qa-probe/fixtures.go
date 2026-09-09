package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type fixtureResult struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Status   string         `json:"status"` // PASS|FAIL
	Expected map[string]any `json:"expected"`
	Got      map[string]any `json:"got"`
	Detail   string         `json:"detail,omitempty"`
}

type fixturesOut struct {
	When    string          `json:"when"`
	Mode    string          `json:"mode"`
	Results []fixtureResult `json:"results"`
	AllPass bool            `json:"allPass"`
	Summary string          `json:"summary"`
}

func runFixturesProbe() error {
	outPath := envOr("NESTWORTH_PROBE_OUT", "/workspace/nestworth-analytics-qa/seed/probe-fixtures.json")
	results := []fixtureResult{
		fixtureA(),
		fixtureB(),
		fixtureC(),
		fixtureD(),
	}
	all := true
	pass, fail := 0, 0
	for _, r := range results {
		if r.Status != "PASS" {
			all = false
			fail++
		} else {
			pass++
		}
	}
	out := fixturesOut{
		When:    time.Now().UTC().Format(time.RFC3339),
		Mode:    "NESTWORTH_PROBE_MODE=fixtures (in-memory ComputeAnalysis)",
		Results: results, AllPass: all,
		Summary: fmt.Sprintf("%d PASS / %d FAIL", pass, fail),
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return err
	}
	fmt.Printf("fixtures written %s\n%s\n", outPath, string(raw))
	if !all {
		return fmt.Errorf("one or more fixtures failed")
	}
	return nil
}

func fxMust(v string) domain.FxRate {
	r, err := domain.ParseFxRate(v)
	if err != nil {
		panic(err)
	}
	return r
}

func unitMust(v string) domain.UnitPrice {
	p, err := domain.ParseUnitPrice(v)
	if err != nil {
		panic(err)
	}
	return p
}

func moneyMust(amount, currency string) domain.Money {
	m, err := domain.ParseMoney(amount, domain.CurrencyCode(currency))
	if err != nil {
		panic(err)
	}
	return m
}

func qtyMust(v string) domain.Quantity {
	q, err := domain.ParseQuantity(v)
	if err != nil {
		panic(err)
	}
	return q
}

func fixtureAccount(hh domain.HouseholdID, currency domain.CurrencyCode, tracking domain.TrackingMode) domain.Account {
	return domain.Account{
		ID: domain.NewAccountID(), HouseholdID: hh, Name: "fixture",
		AccountType: domain.TypeBankAccount, BalanceSheetRole: domain.RoleAsset,
		TrackingMode: tracking, DefaultCurrency: currency, IncludeInNetWorth: true,
	}
}

func fixtureItem(accountID domain.AccountID, currency, native, base string, holdingID *domain.HoldingID, instrumentID *domain.InstrumentID, quoteID, fxID string) domain.DailyValuationSnapshotItem {
	baseAmount := moneyMust(base, "CNY")
	item := domain.DailyValuationSnapshotItem{
		ID: domain.NewDailyValuationSnapshotItemID(), AccountID: accountID,
		HoldingID: holdingID, InstrumentID: instrumentID,
		NativeAmount: native, NativeCurrency: domain.CurrencyCode(currency),
		BaseAmount: &baseAmount, BaseAmountExact: base, Complete: true,
	}
	if quoteID != "" {
		item.QuoteID = &quoteID
	}
	if fxID != "" {
		item.FXQuoteID = &fxID
	}
	return item
}

func fixtureSnapshot(date string, items ...domain.DailyValuationSnapshotItem) domain.DailyValuationSnapshot {
	parsed, _ := time.Parse("2006-01-02", date)
	return domain.DailyValuationSnapshot{
		ID: domain.NewDailyValuationSnapshotID(), LocalDate: date,
		CutoffAt: parsed.Add(23*time.Hour + 59*time.Minute), Currency: "CNY", Complete: true, Items: items,
	}
}

func fixtureOrigin(hh domain.HouseholdID) domain.HistoryOrigin {
	origin, err := domain.NewHistoryOrigin(hh, "UTC", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		panic(err)
	}
	return origin
}

func fixtureQuery(valuation domain.Valuation, includeCash bool) domain.AnalysisQuery {
	return domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:  "2026-08-02", To: "2026-08-02",
		Valuation: valuation, Basis: domain.ReturnBasisInvestment, IncludeCash: includeCash,
	}
}

func bucketAmt(day domain.ComponentDay, bucket domain.AttributionBucket) decimal.Decimal {
	v, ok := day.AssetBuckets[bucket]
	if !ok {
		return decimal.Zero
	}
	return v.Amount()
}

func findComp(result domain.PeriodAnalysisResult, key string) (domain.ComponentDay, bool) {
	for _, day := range result.Days {
		if day.Component.Key() == key {
			return day, true
		}
	}
	return domain.ComponentDay{}, false
}

func fixtureA() fixtureResult {
	// Modified Dietz: noon +90000 contribution; begin 10000 holding +0 cash; end 11000 holding +90000 cash
	// return +1000; denom 55000; daily ≈ 1.81818%
	wantRate := decimal.NewFromInt(1).Div(decimal.NewFromInt(55))
	hh := domain.NewHouseholdID()
	account := fixtureAccount(hh, "USD", domain.TrackingHoldings)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: hh, Name: "QQQ", Type: domain.InstrumentETF, QuoteCurrency: "USD"}
	holding := domain.Holding{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}
	prev := fixtureSnapshot("2026-08-01",
		fixtureItem(account.ID, "USD", "0", "0", nil, nil, "", ""),
		fixtureItem(account.ID, "USD", "10000", "10000", &holdingID, &instrumentID, "", ""),
	)
	cur := fixtureSnapshot("2026-08-02",
		fixtureItem(account.ID, "USD", "90000", "90000", nil, nil, "", ""),
		fixtureItem(account.ID, "USD", "11000", "11000", &holdingID, &instrumentID, "", ""),
	)
	activityID := domain.NewActivityID()
	amount := moneyMust("90000", "USD")
	activity := domain.Activity{
		ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonContribution,
		EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02",
		Effects: []domain.ActivityEffect{{
			ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount,
			Direction: domain.EffectAdded, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationExternalInflow,
			AccountID: &account.ID, Money: &amount,
		}},
	}
	openP, closeP := unitMust("10000"), unitMust("11000")
	openQ := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: openP, Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQ := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: closeP, Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	input := application.AnalysisInputs{
		Origin: fixtureOrigin(hh),
		Portfolio: domain.PortfolioSnapshot{
			Household:   &domain.Household{ID: hh, BaseCurrency: "USD"},
			Accounts:    []domain.AccountRecord{{Account: account}},
			Instruments: []domain.Instrument{instrument}, Holdings: []domain.Holding{holding},
		},
		Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity},
		InstrumentQuotes: []domain.InstrumentQuote{openQ, closeQ},
	}
	// Also assert pure ModifiedDietzRate helper
	flowMoney, _ := domain.ParseSignedMoney("90000", "USD")
	dayStart := time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)
	rate, rated := application.ModifiedDietzRate(decimal.NewFromInt(10000), decimal.NewFromInt(1000),
		[]domain.DietzCapitalFlow{{Amount: flowMoney, EffectiveAt: dayStart.Add(12 * time.Hour)}},
		dayStart, dayStart.AddDate(0, 0, 1))

	result, err := application.ComputeAnalysis(input, fixtureQuery(domain.ValuationBase, true))
	fr := fixtureResult{
		ID: "A", Name: "Modified Dietz ~1.81818%",
		Expected: map[string]any{"rate": wantRate.String(), "returnAmount": "1000", "helperRated": true},
	}
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = err.Error()
		return fr
	}
	gotRate := ""
	if result.ReturnRate != nil {
		gotRate = result.ReturnRate.String()
	}
	gotAmt := ""
	if result.ReturnAmount != nil {
		gotAmt = result.ReturnAmount.Amount().String()
	}
	fr.Got = map[string]any{"rate": gotRate, "returnAmount": gotAmt, "helperRate": rate.String(), "helperRated": rated}
	ok := rated && rate.Sub(wantRate).Abs().LessThanOrEqual(decimal.RequireFromString("0.000000000000001")) &&
		result.ReturnAmount != nil && result.ReturnAmount.Amount().Equal(decimal.NewFromInt(1000)) &&
		result.ReturnRate != nil && result.ReturnRate.Sub(wantRate).Abs().LessThanOrEqual(decimal.RequireFromString("0.000000000000001"))
	if ok {
		fr.Status = "PASS"
	} else {
		fr.Status = "FAIL"
		fr.Detail = "Dietz rate or return amount mismatch"
	}
	return fr
}

func fixtureB() fixtureResult {
	// Foreign holding: $100→$110 @ 7.0→7.2 → Price +70, FX +22 (base); native Price +10 FX 0
	hh := domain.NewHouseholdID()
	account := fixtureAccount(hh, "CNY", domain.TrackingHoldings)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: hh, Type: domain.InstrumentStock, QuoteCurrency: "USD", Name: "foreign"}
	openQ := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitMust("100"), Currency: "USD", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	closeQ := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitMust("110"), Currency: "USD", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	openFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: hh, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxMust("7"), QuotedAt: openQ.QuotedAt}
	closeFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: hh, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxMust("7.2"), QuotedAt: closeQ.QuotedAt}
	prev := fixtureSnapshot("2026-08-01", fixtureItem(account.ID, "USD", "100", "700", &holdingID, &instrumentID, openQ.ID.String(), openFX.ID.String()))
	cur := fixtureSnapshot("2026-08-02", fixtureItem(account.ID, "USD", "110", "792", &holdingID, &instrumentID, closeQ.ID.String(), closeFX.ID.String()))
	input := application.AnalysisInputs{
		Origin: fixtureOrigin(hh),
		Portfolio: domain.PortfolioSnapshot{
			Household:   &domain.Household{ID: hh, BaseCurrency: "CNY"},
			Accounts:    []domain.AccountRecord{{Account: account}},
			Instruments: []domain.Instrument{instrument},
			Holdings:    []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}},
		},
		Snapshots:        []domain.DailyValuationSnapshot{prev, cur},
		InstrumentQuotes: []domain.InstrumentQuote{openQ, closeQ},
		FXQuotes:         []domain.FXQuote{openFX, closeFX},
	}
	fr := fixtureResult{ID: "B", Name: "Foreign holding Price 70 / FX 22 base", Expected: map[string]any{"basePrice": "70", "baseFX": "22", "nativePrice": "10", "nativeFX": "0"}}
	baseResult, err := application.ComputeAnalysis(input, fixtureQuery(domain.ValuationBase, true))
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = err.Error()
		return fr
	}
	baseDay := baseResult.Days[0]
	nativeResult, err := application.ComputeAnalysis(input, fixtureQuery(domain.ValuationNative, true))
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = err.Error()
		return fr
	}
	nativeDay := nativeResult.Days[0]
	bp, bf := bucketAmt(baseDay, domain.BucketPriceChange), bucketAmt(baseDay, domain.BucketFXImpact)
	np, nf := bucketAmt(nativeDay, domain.BucketPriceChange), bucketAmt(nativeDay, domain.BucketFXImpact)
	fr.Got = map[string]any{"basePrice": bp.String(), "baseFX": bf.String(), "nativePrice": np.String(), "nativeFX": nf.String()}
	if bp.Equal(decimal.NewFromInt(70)) && bf.Equal(decimal.NewFromInt(22)) && np.Equal(decimal.NewFromInt(10)) && nf.IsZero() {
		fr.Status = "PASS"
	} else {
		fr.Status = "FAIL"
		fr.Detail = "Price/FX attribution mismatch"
	}
	return fr
}

func fixtureC() fixtureResult {
	// TrackingBalance cash: income +10 USD @7.1 → Income 71, FX 21, Residual 0
	hh := domain.NewHouseholdID()
	account := fixtureAccount(hh, "USD", domain.TrackingBalance)
	when := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	activityID := domain.NewActivityID()
	amount := moneyMust("10", "USD")
	activity := domain.Activity{
		ID: activityID, Kind: domain.ActivityCashIn, Reason: domain.ReasonIncome,
		EffectiveAt: when, EffectiveLocalDate: "2026-08-02",
		Effects: []domain.ActivityEffect{{
			ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRoleAmount,
			Direction: domain.EffectAdded, Target: domain.EffectTargetAccountValue, Classification: domain.ClassificationIncome,
			AccountID: &account.ID, Money: &amount,
		}},
	}
	openFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: hh, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxMust("7"), QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	eventFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: hh, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxMust("7.1"), QuotedAt: when}
	closeFX := domain.FXQuote{ID: domain.NewFXQuoteID(), HouseholdID: hh, BaseCurrency: "USD", QuoteCurrency: "CNY", Rate: fxMust("7.2"), QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	prev := fixtureSnapshot("2026-08-01", fixtureItem(account.ID, "USD", "100", "700", nil, nil, "", openFX.ID.String()))
	cur := fixtureSnapshot("2026-08-02", fixtureItem(account.ID, "USD", "110", "792", nil, nil, "", closeFX.ID.String()))
	input := application.AnalysisInputs{
		Origin: fixtureOrigin(hh),
		Portfolio: domain.PortfolioSnapshot{
			Household: &domain.Household{ID: hh, BaseCurrency: "CNY"},
			Accounts:  []domain.AccountRecord{{Account: account}},
		},
		Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{activity},
		FXQuotes: []domain.FXQuote{openFX, eventFX, closeFX},
	}
	fr := fixtureResult{ID: "C", Name: "Foreign cash Income 71 / FX 21", Expected: map[string]any{"income": "71", "fx": "21", "residual": nil}}
	result, err := application.ComputeAnalysis(input, fixtureQuery(domain.ValuationBase, true))
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = err.Error()
		return fr
	}
	day := result.Days[0]
	inc, fx := bucketAmt(day, domain.BucketIncome), bucketAmt(day, domain.BucketFXImpact)
	var residual any
	if day.Residual != nil {
		residual = day.Residual.Amount.Amount().String()
	}
	fr.Got = map[string]any{"income": inc.String(), "fx": fx.String(), "residual": residual, "status": string(day.Status)}
	if inc.Equal(decimal.NewFromInt(71)) && fx.Equal(decimal.NewFromInt(21)) && day.Residual == nil && day.Status == domain.CompletenessOK {
		fr.Status = "PASS"
	} else {
		fr.Status = "FAIL"
		fr.Detail = "cash income/FX/residual mismatch"
	}
	return fr
}

func fixtureD() fixtureResult {
	// Scope: instrument buy = External Flow +100; household in-kind transfer neutral; account receiving = external +100
	hh := domain.NewHouseholdID()
	account := fixtureAccount(hh, "CNY", domain.TrackingHoldings)
	instrumentID, holdingID := domain.NewInstrumentID(), domain.NewHoldingID()
	instrument := domain.Instrument{ID: instrumentID, HouseholdID: hh, Type: domain.InstrumentStock, QuoteCurrency: "CNY", Name: "scoped"}
	quote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitMust("100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 2, 23, 0, 0, 0, time.UTC)}
	activityID := domain.NewActivityID()
	qty := qtyMust("1")
	gross := moneyMust("100", "CNY")
	buy := domain.Activity{
		ID: activityID, Kind: domain.ActivityBuy, Reason: domain.ReasonPrincipal,
		EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02",
		TradeDetail: &domain.TradeDetail{Side: domain.TradeBuy, InstrumentID: instrumentID, HoldingID: holdingID, Quantity: qty, Gross: gross, UnitPrice: unitMust("100")},
		Effects: []domain.ActivityEffect{
			{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 1, Role: domain.EffectRolePrincipal, Direction: domain.EffectRemoved, Target: domain.EffectTargetAccountCash, Classification: domain.ClassificationTradePrincipal, AccountID: &account.ID, Money: &gross},
			{ID: domain.NewActivityEffectID(), ActivityID: activityID, Sequence: 2, Role: domain.EffectRolePrincipal, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationTradePrincipal, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &qty},
		},
	}
	prev := fixtureSnapshot("2026-08-01",
		fixtureItem(account.ID, "CNY", "100", "100", nil, nil, "", ""),
		fixtureItem(account.ID, "CNY", "0", "0", &holdingID, &instrumentID, "", ""),
	)
	cur := fixtureSnapshot("2026-08-02",
		fixtureItem(account.ID, "CNY", "0", "0", nil, nil, "", ""),
		fixtureItem(account.ID, "CNY", "100", "100", &holdingID, &instrumentID, quote.ID.String(), ""),
	)
	input := application.AnalysisInputs{
		Origin: fixtureOrigin(hh),
		Portfolio: domain.PortfolioSnapshot{
			Household:   &domain.Household{ID: hh, BaseCurrency: "CNY"},
			Accounts:    []domain.AccountRecord{{Account: account}},
			Instruments: []domain.Instrument{instrument},
			Holdings:    []domain.Holding{{ID: holdingID, AccountID: account.ID, InstrumentID: instrumentID}},
		},
		Snapshots: []domain.DailyValuationSnapshot{prev, cur}, Activities: []domain.Activity{buy},
		InstrumentQuotes: []domain.InstrumentQuote{quote},
	}
	instQuery := fixtureQuery(domain.ValuationBase, true)
	instQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: instrumentID.String()}
	fr := fixtureResult{ID: "D", Name: "Scope semantics instrument external +100 / transfer relative", Expected: map[string]any{"instrumentExternalFlow": "100", "accountExternalOnReceive": "100", "householdTransferPrice": "0"}}

	instResult, err := application.ComputeAnalysis(input, instQuery)
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = "instrument scope: " + err.Error()
		return fr
	}
	holdingKey := domain.ComponentID{AccountID: account.ID, HoldingID: &holdingID, InstrumentID: &instrumentID, Currency: "CNY"}.Key()
	holdingDay, ok := findComp(instResult, holdingKey)
	if !ok && len(instResult.Days) > 0 {
		holdingDay = instResult.Days[0]
		ok = true
	}
	instExt := bucketAmt(holdingDay, domain.BucketExternalFlow)

	// In-kind transfer household neutral + account receive external
	source := fixtureAccount(hh, "CNY", domain.TrackingHoldings)
	receiver := fixtureAccount(hh, "CNY", domain.TrackingHoldings)
	fromHolding, toHolding := domain.NewHoldingID(), domain.NewHoldingID()
	openQuote := domain.InstrumentQuote{ID: domain.NewInstrumentQuoteID(), InstrumentID: instrumentID, UnitPrice: unitMust("100"), Currency: "CNY", QuotedAt: time.Date(2026, 8, 1, 23, 0, 0, 0, time.UTC)}
	xferID := domain.NewActivityID()
	xferQty := qtyMust("1")
	xfer := domain.Activity{
		ID: xferID, Kind: domain.ActivityPositionTransfer, Reason: domain.ReasonOther,
		EffectiveAt: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC), EffectiveLocalDate: "2026-08-02",
		Effects: []domain.ActivityEffect{
			{ID: domain.NewActivityEffectID(), ActivityID: xferID, Sequence: 1, Direction: domain.EffectRemoved, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &fromHolding, InstrumentID: &instrumentID, Quantity: &xferQty},
			{ID: domain.NewActivityEffectID(), ActivityID: xferID, Sequence: 2, Direction: domain.EffectAdded, Target: domain.EffectTargetHoldingQuantity, Classification: domain.ClassificationInternalTransfer, HoldingID: &toHolding, InstrumentID: &instrumentID, Quantity: &xferQty},
		},
	}
	xferPrev := fixtureSnapshot("2026-08-01",
		fixtureItem(source.ID, "CNY", "100", "100", &fromHolding, &instrumentID, openQuote.ID.String(), ""),
		fixtureItem(receiver.ID, "CNY", "0", "0", &toHolding, &instrumentID, "", ""),
	)
	xferCur := fixtureSnapshot("2026-08-02",
		fixtureItem(source.ID, "CNY", "0", "0", &fromHolding, &instrumentID, quote.ID.String(), ""),
		fixtureItem(receiver.ID, "CNY", "100", "100", &toHolding, &instrumentID, quote.ID.String(), ""),
	)
	xferInput := application.AnalysisInputs{
		Origin: fixtureOrigin(hh),
		Portfolio: domain.PortfolioSnapshot{
			Household:   &domain.Household{ID: hh, BaseCurrency: "CNY"},
			Accounts:    []domain.AccountRecord{{Account: source}, {Account: receiver}},
			Instruments: []domain.Instrument{instrument},
			Holdings: []domain.Holding{
				{ID: fromHolding, AccountID: source.ID, InstrumentID: instrumentID},
				{ID: toHolding, AccountID: receiver.ID, InstrumentID: instrumentID},
			},
		},
		Snapshots: []domain.DailyValuationSnapshot{xferPrev, xferCur}, Activities: []domain.Activity{xfer},
		InstrumentQuotes: []domain.InstrumentQuote{openQuote, quote},
	}
	hhResult, err := application.ComputeAnalysis(xferInput, fixtureQuery(domain.ValuationBase, true))
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = "household transfer: " + err.Error()
		return fr
	}
	hhPriceOK := true
	for _, day := range hhResult.Days {
		if !bucketAmt(day, domain.BucketPriceChange).IsZero() || day.Residual != nil {
			hhPriceOK = false
		}
	}
	acctQuery := fixtureQuery(domain.ValuationBase, true)
	acctQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeAccount, ID: receiver.ID.String()}
	acctInput := xferInput
	acctInput.Portfolio.Accounts = []domain.AccountRecord{{Account: receiver}}
	acctResult, err := application.ComputeAnalysis(acctInput, acctQuery)
	if err != nil {
		fr.Status = "FAIL"
		fr.Detail = "account transfer: " + err.Error()
		return fr
	}
	recvExt := decimal.Zero
	if len(acctResult.Days) > 0 {
		recvExt = bucketAmt(acctResult.Days[0], domain.BucketExternalFlow)
	}
	fr.Got = map[string]any{
		"instrumentExternalFlow":     instExt.String(),
		"accountExternalOnReceive":   recvExt.String(),
		"householdTransferPriceZero": hhPriceOK,
		"instrumentDayOK":            ok,
	}
	if instExt.Equal(decimal.NewFromInt(100)) && recvExt.Equal(decimal.NewFromInt(100)) && hhPriceOK {
		fr.Status = "PASS"
	} else {
		fr.Status = "FAIL"
		fr.Detail = "scope semantics mismatch"
	}
	return fr
}
