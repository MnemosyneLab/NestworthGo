package application

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type classifiedAnalysisEffect struct {
	activity             domain.Activity
	effect               domain.ActivityEffect
	component            domain.ComponentID
	bucket               *domain.AttributionBucket
	amount               decimal.Decimal
	amountKnown          bool
	returnComponent      *domain.ReturnComponent
	returnAmount         decimal.Decimal
	returnKnown          bool
	dietzAmount          decimal.Decimal
	dietzKnown           bool
	potentialDietzAmount decimal.Decimal
	potentialDietzKnown  bool
	neutral              bool
	relatedHolding       *domain.HoldingID
	relatedInstrument    *domain.InstrumentID
}

// classifyActivity is the single classification pass shared by Asset Changes
// and Return Analysis. It keeps physical AssetBucket, economic
// ReturnComponent, and non-return DietzCapitalFlow assignments separate.
func (u analysisUniverse) classifyActivity(activity domain.Activity, daySnapshot domain.DailyValuationSnapshot, previousSnapshot *domain.DailyValuationSnapshot, input AnalysisInputs, query domain.AnalysisQuery) ([]classifiedAnalysisEffect, error) {
	endItems := snapshotItemsByComponent(daySnapshot, u.accounts, u.instruments)
	startItems := make(map[string]domain.DailyValuationSnapshotItem)
	if previousSnapshot != nil {
		startItems = snapshotItemsByComponent(*previousSnapshot, u.accounts, u.instruments)
	}
	endpoints := make([]domain.ComponentID, 0, len(activity.Effects))
	for _, effect := range activity.Effects {
		component, ok := componentForEffect(effect, u.accounts, u.holdings, u.instruments)
		if ok {
			endpoints = append(endpoints, component)
		}
	}
	insideCount := 0
	for _, endpoint := range endpoints {
		if u.componentInUniverse(endpoint) {
			insideCount++
		}
	}
	results := make([]classifiedAnalysisEffect, 0, len(activity.Effects))
	for _, effect := range activity.Effects {
		component, ok := componentForEffect(effect, u.accounts, u.holdings, u.instruments)
		inside := ok && u.componentInUniverse(component)
		if inside {
			amount, known, err := u.effectAmount(activity, effect, component, daySnapshot, endItems, startItems, input, query)
			if err != nil {
				return nil, err
			}
			if activity.Kind == domain.ActivityPositionTransfer && effect.Classification == domain.ClassificationRemeasurement && effect.CostUnitPrice == nil {
				// A quantity-only adjustment is not a monetary leg.  Its quantity is
				// consumed by the holding bridge (for a proven split) or surfaced as
				// a residual (for an unpriced/manual discrepancy).
				amount, known = decimal.Zero, false
			}
			bucket := assetBucketFor(activity, effect, component, insideCount, len(endpoints), u.accounts)
			neutral := bucket == nil && endpointsInUniverse(endpoints, u)
			signedAmount := signedDirection(effect.Direction, amount)
			if account, ok := u.accounts[component.AccountID]; ok && account.IsLiability() && effect.Target == domain.EffectTargetAccountValue {
				signedAmount = signedAmount.Neg()
			}
			potentialDietzAmount, potentialDietzKnown := u.potentialDietzCapitalAmount(activity, effect, signedAmount, known, component)
			classified := classifiedAnalysisEffect{activity: activity, effect: effect, component: component, bucket: bucket, amount: signedAmount, amountKnown: known, neutral: neutral, potentialDietzAmount: potentialDietzAmount, potentialDietzKnown: potentialDietzKnown}
			if returnComponent, relatedHolding, relatedInstrument, associated := u.returnAssociation(activity, effect, component); associated && relatedHolding == nil && relatedInstrument == nil && u.investmentComponentInUniverse(component) {
				classified.returnComponent = returnComponent
				classified.returnAmount = signedAmount
				classified.returnKnown = known
			}
			if dietzAmount, dietzKnown := u.dietzCapitalAmount(activity, effect, signedAmount, known, component, endpoints); dietzKnown {
				classified.dietzAmount = dietzAmount
				classified.dietzKnown = true
			}
			results = append(results, classified)
		}

		// Dividends and trade commissions are economically associated with the
		// holding/instrument, even when their cash leg is outside the selected
		// physical universe.  Add a return-only projection to that investment
		// component; do not add it to the Asset Changes waterfall a second time.
		returnComponent, relatedHolding, relatedInstrument, associated := u.returnAssociation(activity, effect, component)
		if !associated {
			continue
		}
		owner, ownerOK := u.returnOwner(relatedHolding, relatedInstrument)
		if !ownerOK || !u.investmentComponentInUniverse(owner) || (inside && owner.Key() == component.Key()) {
			continue
		}
		returnAmount, returnKnown, err := u.effectAmountForComponent(activity, effect, owner, input, query)
		if err != nil {
			return nil, err
		}
		returnAmount = signedDirection(effect.Direction, returnAmount)
		if account, ok := u.accounts[component.AccountID]; ok && account.IsLiability() && effect.Target == domain.EffectTargetAccountValue {
			returnAmount = returnAmount.Neg()
		}
		results = append(results, classifiedAnalysisEffect{activity: activity, effect: effect, component: owner, returnComponent: returnComponent, returnAmount: returnAmount, returnKnown: returnKnown, relatedHolding: relatedHolding, relatedInstrument: relatedInstrument})
	}
	_ = previousSnapshot // retained in the signature for the opening-lot bridge
	return results, nil
}

func (u analysisUniverse) returnAssociation(activity domain.Activity, effect domain.ActivityEffect, component domain.ComponentID) (*domain.ReturnComponent, *domain.HoldingID, *domain.InstrumentID, bool) {
	if activity.Kind == domain.ActivityCashIn && activity.Reason == domain.ReasonInterest && component.Cash {
		returnComponent := domain.ReturnDividendInterest
		return &returnComponent, nil, nil, true
	}
	if activity.Kind == domain.ActivityCashDividend && activity.DividendDetail != nil {
		holding := activity.DividendDetail.HoldingID
		instrument := activity.DividendDetail.InstrumentID
		returnComponent := domain.ReturnDividendInterest
		return &returnComponent, &holding, &instrument, true
	}
	if (activity.Kind == domain.ActivityCashOut || activity.Kind == domain.ActivityValueUpdate) && activity.DividendDetail != nil && (activity.Reason == domain.ReasonTax || activity.Reason == domain.ReasonFee) {
		holding := activity.DividendDetail.HoldingID
		instrument := activity.DividendDetail.InstrumentID
		returnComponent := domain.ReturnInvestmentFee
		return &returnComponent, &holding, &instrument, true
	}
	if (activity.Kind == domain.ActivityBuy || activity.Kind == domain.ActivitySell) && activity.TradeDetail != nil && effect.Role == domain.EffectRoleFee {
		holding := activity.TradeDetail.HoldingID
		instrument := activity.TradeDetail.InstrumentID
		returnComponent := domain.ReturnInvestmentFee
		return &returnComponent, &holding, &instrument, true
	}
	return nil, nil, nil, false
}

func (u analysisUniverse) returnOwner(holdingID *domain.HoldingID, instrumentID *domain.InstrumentID) (domain.ComponentID, bool) {
	if holdingID != nil {
		if holding, ok := u.holdings[*holdingID]; ok {
			instrument := holding.InstrumentID
			currency := domain.CurrencyCode("")
			if value, ok := u.instruments[instrument]; ok {
				currency = value.QuoteCurrency
			}
			return domain.ComponentID{AccountID: holding.AccountID, HoldingID: holdingID, InstrumentID: &instrument, Currency: currency}, true
		}
	}
	if instrumentID != nil {
		for _, holding := range u.holdings {
			if holding.InstrumentID != *instrumentID {
				continue
			}
			instrument := holding.InstrumentID
			currency := domain.CurrencyCode("")
			if value, ok := u.instruments[instrument]; ok {
				currency = value.QuoteCurrency
			}
			return domain.ComponentID{AccountID: holding.AccountID, HoldingID: &holding.ID, InstrumentID: &instrument, Currency: currency}, true
		}
	}
	return domain.ComponentID{}, false
}

func (u analysisUniverse) investmentComponentInUniverse(component domain.ComponentID) bool {
	_, ok := u.investmentComponentKeys[component.Key()]
	return ok
}

func endpointsInInvestmentUniverse(endpoints []domain.ComponentID, universe analysisUniverse) bool {
	if len(endpoints) == 0 {
		return false
	}
	for _, endpoint := range endpoints {
		if !universe.investmentComponentInUniverse(endpoint) {
			return false
		}
	}
	return true
}

func (u analysisUniverse) effectAmountForComponent(activity domain.Activity, effect domain.ActivityEffect, component domain.ComponentID, input AnalysisInputs, query domain.AnalysisQuery) (decimal.Decimal, bool, error) {
	if effect.Money == nil {
		return u.effectAmount(activity, effect, component, domain.DailyValuationSnapshot{}, nil, nil, input, query)
	}
	value, err := u.convertAmount(effect.Money.Amount(), effect.Money.Currency(), activity.EffectiveAt, input, query, component.Currency)
	return value, err == nil, err
}

func (u analysisUniverse) dietzCapitalAmount(activity domain.Activity, effect domain.ActivityEffect, amount decimal.Decimal, known bool, component domain.ComponentID, endpoints []domain.ComponentID) (decimal.Decimal, bool) {
	amount, known = u.potentialDietzCapitalAmount(activity, effect, amount, known, component)
	if !known || !u.investmentComponentInUniverse(component) {
		return decimal.Zero, false
	}
	if activity.Kind == domain.ActivityBuy || activity.Kind == domain.ActivitySell || activity.Kind == domain.ActivityCashTransfer || activity.Kind == domain.ActivityFXConversion || activity.Kind == domain.ActivityDebtDraw || activity.Kind == domain.ActivityDebtPayment {
		if endpointsInInvestmentUniverse(endpoints, u) {
			return decimal.Zero, false
		}
		return amount, true
	}
	return amount, true
}

// potentialDietzCapitalAmount retains a signed capital leg independently of
// the current InvestmentUniverse.  A household-scope trade is internal for
// the calendar, but its holding leg is capital entering an instrument group
// and its cash leg is capital leaving a cash group.  Contribution folding
// uses these candidates to re-evaluate each group as its own universe.
func (u analysisUniverse) potentialDietzCapitalAmount(activity domain.Activity, effect domain.ActivityEffect, amount decimal.Decimal, known bool, component domain.ComponentID) (decimal.Decimal, bool) {
	if !known || !u.potentialInvestmentComponent(component) {
		return decimal.Zero, false
	}
	if returnComponent, _, _, associated := u.returnAssociation(activity, effect, component); associated && returnComponent != nil && *returnComponent == domain.ReturnInvestmentFee {
		// An associated commission/tax is investment performance, not new
		// capital. An unassociated fee remains a negative Dietz flow below.
		return decimal.Zero, false
	}
	if activity.Kind == domain.ActivityCashDividend || activity.Kind == domain.ActivityValueUpdate || activity.Kind == domain.ActivityPositionTransfer {
		return decimal.Zero, false
	}
	if activity.Kind == domain.ActivityCashIn && activity.Reason == domain.ReasonInterest {
		return decimal.Zero, false
	}
	if (activity.Kind == domain.ActivityBuy || activity.Kind == domain.ActivitySell) && effect.Role == domain.EffectRoleFee {
		return decimal.Zero, false
	}
	if effect.Money == nil && activity.TradeDetail == nil {
		return decimal.Zero, false
	}
	return amount, true
}

func (u analysisUniverse) potentialInvestmentComponent(component domain.ComponentID) bool {
	if component.InstrumentID != nil {
		return true
	}
	if !component.Cash {
		return false
	}
	account, ok := u.accounts[component.AccountID]
	return ok && !account.IsLiability()
}

func assetBucketFor(activity domain.Activity, effect domain.ActivityEffect, component domain.ComponentID, insideCount, endpointCount int, accounts map[domain.AccountID]domain.Account) *domain.AttributionBucket {
	account, hasAccount := accounts[component.AccountID]
	if !hasAccount {
		return nil
	}
	// Semantic reasons take precedence over funding/principal labels.
	if activity.Kind == domain.ActivityCashDividend {
		bucket := domain.BucketDividendInterest
		return &bucket
	}
	if activity.Kind == domain.ActivityCashIn && activity.Reason == domain.ReasonInterest {
		bucket := domain.BucketDividendInterest
		return &bucket
	}
	if activity.Kind == domain.ActivityCashIn && (activity.Reason == domain.ReasonIncome || activity.Reason == domain.ReasonGift) {
		bucket := domain.BucketIncome
		return &bucket
	}
	if activity.Kind == domain.ActivityCashOut && activity.Reason == domain.ReasonExpense {
		bucket := domain.BucketSpending
		return &bucket
	}
	if activity.Kind == domain.ActivityCashOut && activity.Reason == domain.ReasonInterest {
		bucket := domain.BucketSpending
		return &bucket
	}
	if activity.Kind == domain.ActivityDebtPayment && effect.Role == domain.EffectRoleFee {
		// DebtPaymentInput stores InterestOrFee as a fee-role cash leg while
		// keeping the activity reason at principal for the paired repayment.
		// The product contract treats that leg as loan interest paid in cash,
		// therefore it is Spending rather than Fee.
		bucket := domain.BucketSpending
		return &bucket
	}
	if activity.Reason == domain.ReasonFee || activity.Reason == domain.ReasonTax || effect.Classification == domain.ClassificationFee {
		bucket := domain.BucketFee
		return &bucket
	}
	if activity.Kind == domain.ActivityValueUpdate || effect.Classification == domain.ClassificationRemeasurement {
		// An unpriced position adjustment is a quantity-only observation.  It
		// may be a split/reverse split, which the holding bridge can recognise
		// when the quoted notional is preserved.  Do not call it an adjustment
		// before that bridge has had a chance to prove the restatement.
		if activity.Kind == domain.ActivityPositionTransfer && effect.Classification == domain.ClassificationRemeasurement && effect.CostUnitPrice == nil {
			return nil
		}
		if account.IsLiability() && effect.Target == domain.EffectTargetAccountValue && activity.Reason == domain.ReasonInterest {
			bucket := domain.BucketLiabilityImpact
			return &bucket
		}
		bucket := domain.BucketAdjustment
		return &bucket
	}

	principal := effect.Classification == domain.ClassificationExternalInflow || effect.Classification == domain.ClassificationExternalOutflow || effect.Classification == domain.ClassificationTradePrincipal || effect.Classification == domain.ClassificationDebtPrincipal || effect.Classification == domain.ClassificationInternalTransfer || activity.Reason == domain.ReasonContribution || activity.Reason == domain.ReasonPrincipal
	if principal && (endpointCount == 1 || insideCount < endpointCount) {
		bucket := domain.BucketExternalFlow
		return &bucket
	}
	if account.IsLiability() && effect.Target == domain.EffectTargetAccountValue && effect.Classification != domain.ClassificationDebtPrincipal {
		bucket := domain.BucketLiabilityImpact
		return &bucket
	}
	return nil
}

func endpointsInUniverse(endpoints []domain.ComponentID, universe analysisUniverse) bool {
	if len(endpoints) == 0 {
		return false
	}
	for _, endpoint := range endpoints {
		if !universe.componentInUniverse(endpoint) {
			return false
		}
	}
	return true
}

func signedDirection(direction domain.EffectDirection, amount decimal.Decimal) decimal.Decimal {
	if direction == domain.EffectRemoved {
		return amount.Neg()
	}
	return amount
}

func (u analysisUniverse) effectAmount(activity domain.Activity, effect domain.ActivityEffect, component domain.ComponentID, daySnapshot domain.DailyValuationSnapshot, endItems, startItems map[string]domain.DailyValuationSnapshotItem, input AnalysisInputs, query domain.AnalysisQuery) (decimal.Decimal, bool, error) {
	if effect.Money != nil {
		value, err := u.convertAmount(effect.Money.Amount(), effect.Money.Currency(), activity.EffectiveAt, input, query, component.Currency)
		if err != nil && query.Valuation == domain.ValuationBase && effect.Money.Currency() != input.Portfolio.Household.BaseCurrency {
			return decimal.Zero, false, nil
		}
		return value, err == nil, err
	}
	if effect.Quantity == nil {
		return decimal.Zero, false, nil
	}
	if effect.HoldingID == nil {
		return decimal.Zero, false, nil
	}
	if activity.TradeDetail != nil && activity.TradeDetail.HoldingID == *effect.HoldingID {
		value, err := u.convertAmount(activity.TradeDetail.Gross.Amount(), activity.TradeDetail.Gross.Currency(), activity.EffectiveAt, input, query, component.Currency)
		if err != nil && query.Valuation == domain.ValuationBase && activity.TradeDetail.Gross.Currency() != input.Portfolio.Household.BaseCurrency {
			return decimal.Zero, false, nil
		}
		return value, err == nil, err
	}
	if effect.CostUnitPrice != nil {
		value, err := domain.MultiplyQuantityAndUnitPrice(*effect.Quantity, *effect.CostUnitPrice)
		if err != nil {
			return decimal.Zero, false, err
		}
		value, err = u.convertAmount(value, component.Currency, activity.EffectiveAt, input, query, component.Currency)
		return value, err == nil, err
	}
	if activity.Kind == domain.ActivityPositionTransfer {
		items := endItems
		cutoff := daySnapshot.CutoffAt
		if effect.Direction == domain.EffectRemoved {
			items = startItems
		}
		if item, ok := items[component.Key()]; ok {
			value, available, err := snapshotItemValue(item, u.accounts[component.AccountID], query.Valuation, input.Portfolio.Household.BaseCurrency)
			if err != nil || !available {
				return decimal.Zero, available, err
			}
			endQuantity, err := inferredQuantity(item, u.quoteForItem(item, input.InstrumentQuotes, cutoff))
			if err != nil || endQuantity.IsZero() {
				return value, true, err
			}
			portion := value.Mul(effect.Quantity.Decimal()).Div(endQuantity)
			return portion, true, nil
		}
	}
	return decimal.Zero, false, nil
}

func activityLocalDate(activity domain.Activity, timezone string) string {
	if activity.EffectiveLocalDate != "" {
		return activity.EffectiveLocalDate
	}
	if timezone == "" {
		return activity.EffectiveAt.UTC().Format("2006-01-02")
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return activity.EffectiveAt.UTC().Format("2006-01-02")
	}
	return activity.EffectiveAt.In(location).Format("2006-01-02")
}
