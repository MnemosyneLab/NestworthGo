package application

import (
	"sort"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Namespaced keys keep explicit other, unknown metadata and the synthetic
// remainder distinct. No classification is inferred from an instrument's name.
const accountCompositionRemainderKey = "account-composition:remainder"
const accountCompositionDetailLimit = 10

// ValueAccountCompositions opts account-page reads into this projection. Other
// valuation consumers (history, liquidity and portfolio) keep their existing
// calculation paths and do not pay for presentation-specific aggregation.
func (v *ValuationService) ValueAccountCompositions(snapshot domain.PortfolioSnapshot) ([]domain.AccountValuation, []domain.MissingInputView, error) {
	accounts, missing, err := v.ValueAccounts(snapshot)
	if err != nil || snapshot.Household == nil {
		return accounts, missing, err
	}
	instruments := compositionInstruments(snapshot)
	for i := range accounts {
		composition, err := projectAccountComposition(accounts[i], instruments, snapshot.Household.BaseCurrency)
		if err != nil {
			return nil, nil, err
		}
		accounts[i].Composition = &composition
	}
	return accounts, missing, nil
}

func compositionInstruments(snapshot domain.PortfolioSnapshot) map[domain.InstrumentID]domain.Instrument {
	instruments := make(map[domain.InstrumentID]domain.Instrument, len(snapshot.Instruments))
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	return instruments
}

func projectAccountComposition(account domain.AccountValuation, instruments map[domain.InstrumentID]domain.Instrument, currency domain.CurrencyCode) (domain.AccountComposition, error) {
	result := domain.AccountComposition{Currency: currency, Complete: account.Complete, Status: "empty", Types: []domain.AccountCompositionItem{}, Details: []domain.AccountCompositionItem{}, DetailSlices: []domain.AccountCompositionItem{}}
	if account.Account.IsLiability() || account.Account.TrackingMode != domain.TrackingHoldings {
		result.Status = "unsupported"
		return result, nil
	}
	groups := map[string]*domain.AccountCompositionItem{}
	total := decimal.Zero
	invalid := false
	for _, component := range account.Components {
		typeKey := "cash"
		key := "account-composition:cash:" + component.NativeCurrency.String()
		label := component.NativeCurrency.String()
		if component.InstrumentID != nil {
			typeKey = "unknown"
			if instrument, ok := instruments[*component.InstrumentID]; ok {
				for _, known := range domain.AllInstrumentTypes() {
					if instrument.Type == known {
						typeKey = string(known)
						break
					}
				}
			}
			key = "account-composition:instrument:" + component.InstrumentID.String()
			if component.HoldingID != nil {
				key = "account-composition:holding:" + component.HoldingID.String()
			}
			label = component.InstrumentSymbol
			if label == "" {
				label = component.InstrumentName
			}
			if label == "" {
				label = component.InstrumentID.String()
			}
		}
		item := domain.AccountCompositionItem{Key: key, TypeKey: typeKey, Label: label, DetailRefs: []string{key}, NativeAmount: component.NativeAmount, NativeCurrency: component.NativeCurrency}
		if component.Available && component.BaseAmountExact != "" {
			amount, err := decimal.NewFromString(component.BaseAmountExact)
			if err != nil {
				return result, &domain.Error{Code: domain.ErrIntegrity, Field: "amount", Message: "stored valuation amount is invalid"}
			}
			invalid = invalid || amount.IsNegative()
			total = total.Add(amount)
			item.BaseAmountExact = amount.String()
			item.BaseAmount = component.BaseAmount
			item.ValuedCount = 1
			result.ValuedCount++
		} else {
			item.MissingCount = 1
			result.MissingCount++
		}
		result.Details = append(result.Details, item)
		if groups[typeKey] == nil {
			groups[typeKey] = &domain.AccountCompositionItem{Key: "account-composition:type:" + typeKey, TypeKey: typeKey, Label: typeKey, DetailRefs: []string{}}
		}
		addCompositionItem(groups[typeKey], item)
	}
	result.Complete = account.Complete && result.MissingCount == 0
	if result.ValuedCount > 0 {
		result.TotalExact = total.String()
	}
	switch {
	case invalid:
		result.Status = "invalid"
	case result.ValuedCount == 0 && result.MissingCount > 0:
		result.Status = "unavailable"
	case total.IsPositive() && result.MissingCount > 0:
		result.Status = "partial"
	case total.IsPositive():
		result.Status = "ready"
	}
	for _, group := range groups {
		sort.Strings(group.DetailRefs)
		result.Types = append(result.Types, *group)
	}
	sortCompositionItems(result.Types)
	sortCompositionItems(result.Details)
	result.DetailSlices = append(result.DetailSlices, result.Details...)
	if len(result.Details) > accountCompositionDetailLimit {
		remainder := domain.AccountCompositionItem{Key: accountCompositionRemainderKey, DetailRefs: []string{}}
		for _, item := range result.Details[accountCompositionDetailLimit:] {
			addCompositionItem(&remainder, item)
		}
		result.DetailSlices = append(append([]domain.AccountCompositionItem{}, result.Details[:accountCompositionDetailLimit]...), remainder)
	}
	for _, rows := range [][]domain.AccountCompositionItem{result.Types, result.Details, result.DetailSlices} {
		values := map[string]decimal.Decimal{}
		for i := range rows {
			if rows[i].BaseAmountExact == "" {
				continue
			}
			amount, _ := decimal.NewFromString(rows[i].BaseAmountExact)
			signed, err := domain.NewSignedMoney(amount, currency)
			if err != nil {
				return result, err
			}
			rows[i].BaseAmount = &domain.MoneyView{Amount: signed.CanonicalAmount(), Currency: signed.Currency()}
			values[rows[i].Key] = amount
		}
		if invalid || !total.IsPositive() {
			continue
		}
		allocations, err := makeAllocations(values, nil, total, currency)
		if err != nil {
			return result, err
		}
		shares := map[string]int{}
		for _, allocation := range allocations {
			shares[allocation.Key] = allocation.ShareBPS
		}
		for i := range rows {
			rows[i].ShareBPS = shares[rows[i].Key]
		}
	}
	return result, nil
}

func addCompositionItem(group *domain.AccountCompositionItem, item domain.AccountCompositionItem) {
	group.DetailRefs = append(group.DetailRefs, item.DetailRefs...)
	group.ValuedCount += item.ValuedCount
	group.MissingCount += item.MissingCount
	if item.BaseAmountExact != "" {
		value, _ := decimal.NewFromString(item.BaseAmountExact)
		sum := decimal.Zero
		if group.BaseAmountExact != "" {
			sum, _ = decimal.NewFromString(group.BaseAmountExact)
		}
		group.BaseAmountExact = sum.Add(value).String()
	}
}

func sortCompositionItems(items []domain.AccountCompositionItem) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if (left.BaseAmountExact == "") != (right.BaseAmountExact == "") {
			return left.BaseAmountExact != ""
		}
		l, _ := decimal.NewFromString(left.BaseAmountExact)
		r, _ := decimal.NewFromString(right.BaseAmountExact)
		if !l.Equal(r) {
			return l.GreaterThan(r)
		}
		return left.Key < right.Key
	})
}
