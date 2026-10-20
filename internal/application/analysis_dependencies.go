package application

import "github.com/waltwang/nestworth-go/internal/domain"

// planAnalysisQuoteDependencies changes only quote hydration. Classification runs
// before output filtering, so all activity instruments (including opposite
// transfer legs, corrections and cost effects) remain dependencies. Snapshot and
// portfolio metadata are never clipped. Unknown references retain the full set.
func planAnalysisQuoteDependencies(input AnalysisInputs, query domain.AnalysisQuery) []domain.InstrumentID {
	full := make([]domain.InstrumentID, 0, len(input.Portfolio.Instruments))
	known := map[domain.InstrumentID]bool{}
	for _, instrument := range input.Portfolio.Instruments {
		full = append(full, instrument.ID)
		known[instrument.ID] = true
	}
	if query.Scope.Kind != domain.ScopeAccount || query.Scope.ID == "" {
		return full
	}
	accounts := map[domain.AccountID]bool{}
	for _, record := range input.Portfolio.Accounts {
		accounts[record.Account.ID] = true
	}
	selected := domain.AccountID(query.Scope.ID)
	if !accounts[selected] {
		return full
	}
	needed := map[domain.InstrumentID]bool{}
	holdings := map[domain.HoldingID]domain.Holding{}
	for _, holding := range input.Portfolio.Holdings {
		holdings[holding.ID] = holding
		if holding.AccountID == selected {
			needed[holding.InstrumentID] = true
		}
	}
	for _, snapshot := range input.Snapshots {
		for _, item := range snapshot.Items {
			if !accounts[item.AccountID] {
				return full
			}
			if item.AccountID == selected {
				if item.HoldingID != nil && item.InstrumentID == nil {
					return full
				}
				if item.InstrumentID != nil {
					needed[*item.InstrumentID] = true
				}
			}
		}
	}
	for _, activity := range input.Activities {
		if _, err := domain.ParseActivityKind(activity.Kind.String()); err != nil {
			return full
		}
		if activity.ProductContext != nil {
			needed[activity.ProductContext.InstrumentID] = true
		}
		if activity.TradeDetail != nil {
			needed[activity.TradeDetail.InstrumentID] = true
		}
		if activity.DividendDetail != nil {
			needed[activity.DividendDetail.InstrumentID] = true
		}
		for _, effects := range [][]domain.ActivityEffect{activity.Effects, activity.RecordedEffects} {
			for _, effect := range effects {
				if effect.AccountID != nil && !accounts[*effect.AccountID] {
					return full
				}
				if effect.InstrumentID != nil {
					needed[*effect.InstrumentID] = true
				}
				if effect.HoldingID != nil {
					holding, ok := holdings[*effect.HoldingID]
					if !ok || !accounts[holding.AccountID] {
						return full
					}
					needed[holding.InstrumentID] = true
				}
			}
		}
	}
	for id := range needed {
		if !known[id] {
			return full
		}
	}
	result := make([]domain.InstrumentID, 0, len(needed))
	for _, id := range full {
		if needed[id] {
			result = append(result, id)
		}
	}
	return result
}
