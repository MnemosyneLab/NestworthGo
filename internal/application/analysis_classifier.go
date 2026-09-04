package application

import (
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type classifiedAnalysisEffect struct {
	activity    domain.Activity
	effect      domain.ActivityEffect
	component   domain.ComponentID
	bucket      *domain.AttributionBucket
	amount      decimal.Decimal
	amountKnown bool
	neutral     bool
}

// classifyActivity is the one Phase 1a classification pass. It classifies
// physical legs for Asset Changes only. ReturnComponent and DietzCapitalFlow
// remain unset until Phase 1b; they must not be inferred by projections.
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
		if !ok || !u.componentInUniverse(component) {
			continue
		}
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
		results = append(results, classifiedAnalysisEffect{activity: activity, effect: effect, component: component, bucket: bucket, amount: signedAmount, amountKnown: known, neutral: neutral})
	}
	_ = previousSnapshot // retained in the signature for the opening-lot bridge
	return results, nil
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
