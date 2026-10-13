package application

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// projectAccountDisplayValue never derives a native value from rounded BaseValue.
// NativeAmount remains known when only household FX is missing. All components,
// including cash and managed products, contribute on the same terms.
func (v *ValuationService) projectAccountDisplayValue(snapshot domain.PortfolioSnapshot, account domain.AccountValuation) (domain.AccountDisplayValue, error) {
	result := domain.AccountDisplayValue{Currency: account.Account.DefaultCurrency, Complete: true, MissingInputs: []domain.MissingInputView{}, FXEvidence: []domain.QuoteEvidenceView{}}
	total := decimal.Zero
	seenEvidence := map[string]bool{}
	for _, component := range account.Components {
		if component.NativeAmount == "" {
			result.MissingCount++
			// Preserve reasons why the native value itself is unavailable,
			// including any FX needed to establish a converted quote.
			for _, missing := range account.MissingInputs {
				if sameOptionalInstrumentID(missing.InstrumentID, component.InstrumentID) {
					result.MissingInputs = append(result.MissingInputs, missing)
				}
			}
			continue
		}
		native, err := decimal.NewFromString(component.NativeAmount)
		if err != nil {
			return result, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "stored native valuation amount is invalid"}
		}
		amount, evidence, missing, err := v.convertAccountDisplay(snapshot, account.Account.ID, native, component.NativeCurrency, result.Currency)
		if err != nil {
			return result, err
		}
		if len(missing) > 0 {
			result.MissingCount++
			for i := range missing {
				missing[i].InstrumentID = cloneInstrumentID(component.InstrumentID)
			}
			result.MissingInputs = append(result.MissingInputs, missing...)
			continue
		}
		result.ValuedCount++
		total = total.Add(amount)
		for _, item := range evidence {
			if !seenEvidence[item.ObservationID] {
				seenEvidence[item.ObservationID] = true
				result.FXEvidence = append(result.FXEvidence, item)
			}
		}
	}
	result.Complete = result.MissingCount == 0
	sortMissing(result.MissingInputs)
	result.MissingInputs = deduplicateMissing(result.MissingInputs)
	// An empty holdings account is known zero; an unknown component is not.
	if result.ValuedCount > 0 || result.Complete {
		result.TotalExact = total.String()
		value, err := moneyView(total, result.Currency)
		if err != nil {
			return result, err
		}
		result.Value = &value
	}
	return result, nil
}

func sameOptionalInstrumentID(a, b *domain.InstrumentID) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

// Policy: select the eligible persisted direct/inverse pair first, using the
// existing source preference, provider and observation ordering. If absent,
// bridge only through household currency, and only when there is no explicit
// preference for the requested pair. Each leg honors its own preference. No
// network fetch, arbitrary graph search, or rounded household total is used.
func (v *ValuationService) convertAccountDisplay(snapshot domain.PortfolioSnapshot, accountID domain.AccountID, native decimal.Decimal, from, to domain.CurrencyCode) (decimal.Decimal, []domain.QuoteEvidenceView, []domain.MissingInputView, error) {
	if native.IsZero() || from == to {
		return native, nil, nil, nil
	}
	amount, evidence, missing, err := v.convertTo(snapshot, accountID, native, from, to)
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	if missing == nil {
		return amount, []domain.QuoteEvidenceView{*evidence}, nil, nil
	}
	base := snapshot.Household.BaseCurrency
	if from == base || to == base || findFXPreference(snapshot.FXPreferences, from, to) != nil {
		return decimal.Zero, nil, []domain.MissingInputView{*missing}, nil
	}
	intermediate, first, firstMissing, err := v.convertTo(snapshot, accountID, native, from, base)
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	if firstMissing != nil {
		return decimal.Zero, nil, []domain.MissingInputView{*firstMissing}, nil
	}
	converted, second, secondMissing, err := v.convertTo(snapshot, accountID, intermediate, base, to)
	if err != nil {
		return decimal.Zero, nil, nil, err
	}
	if secondMissing != nil {
		return decimal.Zero, nil, []domain.MissingInputView{*secondMissing}, nil
	}
	return converted, []domain.QuoteEvidenceView{*first, *second}, nil, nil
}
