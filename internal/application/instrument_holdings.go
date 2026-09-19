package application

import (
	"context"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// InstrumentHoldings reads one snapshot and shares the cost replay across all
// members, including transfer sources outside the visible account scope.
func (g *GainService) InstrumentHoldings(ctx context.Context) ([]domain.InstrumentHoldingsView, error) {
	snapshot, err := g.repository.ReadPortfolioSnapshot(ctx, domain.AccountFilter{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	groups := []domain.InstrumentHoldingsView{}
	if snapshot.Household == nil {
		return groups, nil
	}
	accounts := map[domain.AccountID]string{}
	for _, record := range snapshot.Accounts {
		a := record.Account
		if a.ArchivedAt == nil && a.TrackingMode == "holdings" && a.AccountType != "cash_on_hand" {
			accounts[a.ID] = a.Name
		}
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	fx, origin, err := g.gainFXInputs(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	replay := newCostBasisReplayContext(g.repository, snapshot.Holdings)
	indices := map[domain.InstrumentID]int{}
	for _, holding := range snapshot.Holdings {
		accountName, included := accounts[holding.AccountID]
		if !included || holding.ArchivedAt != nil {
			continue
		}
		instrument, ok := instruments[holding.InstrumentID]
		if !ok {
			return nil, &domain.Error{Code: domain.ErrIntegrity, Message: "holding instrument is missing"}
		}
		gain, err := g.holdingGain(ctx, snapshot, holding, instrument, replay, fx, origin)
		if err != nil {
			return nil, err
		}
		// Available also includes base-currency decomposition. Native values remain
		// valid when FX is missing; only their individual pointers determine availability.
		amounts := domain.HoldingAmounts{Quantity: gain.Quantity, TotalCost: &gain.TotalCost, CurrentValue: gain.CurrentValue, UnrealizedGain: gain.UnrealizedGain}
		if amounts.CurrentValue == nil {
			amounts.ValueMissingReason = gain.MissingReason
		}
		if amounts.UnrealizedGain == nil {
			amounts.GainMissingReason = gain.MissingReason
		}
		index, found := indices[instrument.ID]
		if !found {
			index = len(groups)
			indices[instrument.ID] = index
			name, symbol := instrumentIdentity(instrument)
			groups = append(groups, domain.InstrumentHoldingsView{InstrumentID: instrument.ID, Name: name, Symbol: symbol, QuoteCurrency: instrument.QuoteCurrency, QuantityUnit: instrument.QuantityUnit, Archived: instrument.ArchivedAt != nil, Holdings: []domain.InstrumentHoldingMember{}})
		}
		groups[index].Holdings = append(groups[index].Holdings, domain.InstrumentHoldingMember{HoldingID: holding.ID, AccountID: holding.AccountID, AccountName: accountName, Amounts: amounts})
	}
	for i := range groups {
		group := &groups[i]
		sort.Slice(group.Holdings, func(i, j int) bool {
			a, b := group.Holdings[i], group.Holdings[j]
			if a.AccountName != b.AccountName {
				return a.AccountName < b.AccountName
			}
			return a.HoldingID.String() < b.HoldingID.String()
		})
		group.Amounts, err = aggregateHoldingAmounts(group.Holdings, group.QuoteCurrency)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].InstrumentID.String() < groups[j].InstrumentID.String()
	})
	return groups, nil
}

func aggregateHoldingAmounts(members []domain.InstrumentHoldingMember, currency domain.CurrencyCode) (domain.HoldingAmounts, error) {
	result := domain.HoldingAmounts{}
	quantity, cost, value, gain := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	costComplete, valueComplete, gainComplete := true, true, true
	add := func(total *decimal.Decimal, amount string, actual domain.CurrencyCode) error {
		if actual != currency {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "holding amounts have inconsistent currencies"}
		}
		number, err := decimal.NewFromString(amount)
		if err != nil {
			return err
		}
		*total = total.Add(number)
		return nil
	}
	for _, member := range members {
		a := member.Amounts
		q, err := decimal.NewFromString(a.Quantity)
		if err != nil {
			return result, err
		}
		quantity = quantity.Add(q)
		if a.TotalCost == nil {
			costComplete = false
			result.CostMissingReason = a.CostMissingReason
		} else if err := add(&cost, a.TotalCost.Amount, a.TotalCost.Currency); err != nil {
			return result, err
		}
		if a.CurrentValue == nil {
			valueComplete = false
			result.ValueMissingReason = a.ValueMissingReason
		} else if err := add(&value, a.CurrentValue.Amount, a.CurrentValue.Currency); err != nil {
			return result, err
		}
		if a.UnrealizedGain == nil {
			gainComplete = false
			result.GainMissingReason = a.GainMissingReason
		} else if err := add(&gain, a.UnrealizedGain.Amount, a.UnrealizedGain.Currency); err != nil {
			return result, err
		}
	}
	result.Quantity = quantity.String()
	if costComplete {
		amount, err := moneyView(cost, currency)
		if err != nil {
			return result, err
		}
		result.TotalCost = &amount
	}
	if valueComplete {
		amount, err := moneyView(value, currency)
		if err != nil {
			return result, err
		}
		result.CurrentValue = &amount
	}
	if gainComplete {
		amount, err := signedMoneyView(gain, currency)
		if err != nil {
			return result, err
		}
		result.UnrealizedGain = &amount
	}
	return result, nil
}

func (s *Service) InstrumentHoldings(ctx context.Context) ([]domain.InstrumentHoldingsView, error) {
	return s.gain.InstrumentHoldings(ctx)
}
