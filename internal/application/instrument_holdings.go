package application

import (
	"context"
	"sort"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// InstrumentHoldings reads one snapshot and shares the cost replay across all
// members, including transfer sources outside the visible account scope.
func (g *GainService) InstrumentHoldings(ctx context.Context, accountIDs ...domain.AccountID) ([]domain.InstrumentHoldingsView, error) {
	groups, _, err := g.instrumentHoldings(ctx, accountIDs...)
	return groups, err
}

func (g *GainService) instrumentHoldings(ctx context.Context, accountIDs ...domain.AccountID) ([]domain.InstrumentHoldingsView, map[domain.HoldingID]*exactHoldingGain, error) {
	inputs, err := g.repository.ReadGainSnapshot(ctx)
	snapshot := inputs.Portfolio
	if err != nil {
		return nil, nil, err
	}
	groups := []domain.InstrumentHoldingsView{}
	if snapshot.Household == nil {
		return groups, map[domain.HoldingID]*exactHoldingGain{}, nil
	}
	accounts := map[domain.AccountID]string{}
	for _, record := range snapshot.Accounts {
		a := record.Account
		included := len(accountIDs) == 0 && a.ArchivedAt == nil
		for _, id := range accountIDs {
			included = included || a.ID == id
		}
		if included && a.TrackingMode == "holdings" && a.AccountType != "cash_on_hand" {
			accounts[a.ID] = a.Name
		}
	}
	instruments := map[domain.InstrumentID]domain.Instrument{}
	for _, instrument := range snapshot.Instruments {
		instruments[instrument.ID] = instrument
	}
	fx, origin := inputs.HistoricalFXQuotes, snapshot.Origin
	replay := newGainSnapshotReplay(inputs)

	indices := map[domain.InstrumentID]int{}
	exact := make(map[domain.HoldingID]*exactHoldingGain)
	for _, holding := range snapshot.Holdings {
		accountName, included := accounts[holding.AccountID]
		if !included || holding.ArchivedAt != nil {
			continue
		}
		instrument, ok := instruments[holding.InstrumentID]
		if !ok {
			return nil, nil, &domain.Error{Code: domain.ErrIntegrity, Message: "holding instrument is missing"}
		}
		amountsExact := &exactHoldingGain{}
		gain, err := g.holdingGain(ctx, snapshot, holding, instrument, replay, fx, origin, amountsExact)
		exact[holding.ID] = amountsExact
		if err != nil {
			return nil, nil, err
		}
		// Available also includes base-currency decomposition. Native values remain
		// valid when FX is missing; only their individual pointers determine availability.
		amounts := domain.HoldingAmounts{Quantity: gain.Quantity, TotalCost: gain.TotalCost, CurrentValue: gain.CurrentValue, UnrealizedGain: gain.UnrealizedGain, CostMissingReason: gain.CostMissingReason}
		if amounts.CurrentValue == nil {
			amounts.ValueMissingReason = gain.MissingReason
		}
		if amounts.UnrealizedGain == nil {
			amounts.GainMissingReason = gain.MissingReason
			if gain.TotalCost == nil {
				amounts.GainMissingReason = gain.CostMissingReason
			}
		}
		index, found := indices[instrument.ID]
		if !found {
			index = len(groups)
			indices[instrument.ID] = index
			name, symbol := instrumentIdentity(instrument)
			groups = append(groups, domain.InstrumentHoldingsView{InstrumentID: instrument.ID, Type: instrument.Type, Name: name, ShortName: instrument.ShortName, Symbol: symbol, QuoteCurrency: instrument.QuoteCurrency, QuantityUnit: instrument.QuantityUnit, Archived: instrument.ArchivedAt != nil, Holdings: []domain.InstrumentHoldingMember{}})
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
		group.Amounts, err = aggregateHoldingAmounts(group.Holdings, group.QuoteCurrency, exact)
		if err != nil {
			return nil, nil, err
		}
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name != groups[j].Name {
			return groups[i].Name < groups[j].Name
		}
		return groups[i].InstrumentID.String() < groups[j].InstrumentID.String()
	})
	return groups, exact, nil
}

func aggregateHoldingAmounts(members []domain.InstrumentHoldingMember, currency domain.CurrencyCode, exact map[domain.HoldingID]*exactHoldingGain) (domain.HoldingAmounts, error) {
	result := domain.HoldingAmounts{}
	quantity, cost, value, gain := decimal.Zero, decimal.Zero, decimal.Zero, decimal.Zero
	costComplete, valueComplete, gainComplete := true, true, true
	add := func(total *decimal.Decimal, amount decimal.Decimal, actual domain.CurrencyCode) error {
		if actual != currency {
			return &domain.Error{Code: domain.ErrIntegrity, Message: "holding amounts have inconsistent currencies"}
		}
		*total = total.Add(amount)
		return nil
	}
	for _, member := range members {
		a := member.Amounts
		precise, ok := exact[member.HoldingID]
		if !ok {
			return result, &domain.Error{Code: domain.ErrIntegrity, Message: "exact holding amounts are missing"}
		}
		q, err := decimal.NewFromString(a.Quantity)
		if err != nil {
			return result, err
		}
		quantity = quantity.Add(q)
		if a.TotalCost == nil {
			costComplete = false
			result.CostMissingReason = a.CostMissingReason
		} else if err := add(&cost, precise.Cost, a.TotalCost.Currency); err != nil {
			return result, err
		}
		if a.CurrentValue == nil {
			valueComplete = false
			result.ValueMissingReason = a.ValueMissingReason
		} else if err := add(&value, precise.Value, a.CurrentValue.Currency); err != nil {
			return result, err
		}
		if a.UnrealizedGain == nil {
			gainComplete = false
			result.GainMissingReason = a.GainMissingReason
		} else if err := add(&gain, precise.Unrealized, a.UnrealizedGain.Currency); err != nil {
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

func (s *Service) AccountHoldingDetails(ctx context.Context, accountIDs ...domain.AccountID) ([]domain.InstrumentHoldingsView, error) {
	return s.gain.InstrumentHoldings(ctx, accountIDs...)
}

func (s *Service) InstrumentHoldings(ctx context.Context) ([]domain.InstrumentHoldingsView, error) {
	return s.gain.InstrumentHoldings(ctx)
}

// InstrumentHoldingGroups groups the same read model by instrument type and
// adds per-currency subtotals. Subtotals add the exact per-holding decimals
// already produced for each instrument; they do not convert currencies and omit
// closed (zero-quantity) positions, which cannot contribute value or cost.
func (g *GainService) InstrumentHoldingGroups(ctx context.Context, managed map[string]string, accountIDs ...domain.AccountID) ([]domain.InstrumentHoldingGroupView, error) {
	views, exact, err := g.instrumentHoldings(ctx, accountIDs...)
	if err != nil {
		return nil, err
	}
	return groupInstrumentHoldings(views, exact, managed)
}

func groupInstrumentHoldings(views []domain.InstrumentHoldingsView, exact map[domain.HoldingID]*exactHoldingGain, managed map[string]string) ([]domain.InstrumentHoldingGroupView, error) {
	buckets := map[domain.HoldingGroupKey][]domain.InstrumentHoldingsView{}
	for _, view := range views {
		_, owned := managed[view.InstrumentID.String()]
		key := domain.HoldingGroupFor(view.Type, owned)
		buckets[key] = append(buckets[key], view)
	}
	groups := []domain.InstrumentHoldingGroupView{}
	for _, key := range domain.HoldingGroupOrder {
		instruments := buckets[key]
		if len(instruments) == 0 {
			continue
		}
		byCurrency := map[domain.CurrencyCode][]domain.InstrumentHoldingMember{}
		counted := map[domain.CurrencyCode]map[domain.InstrumentID]struct{}{}
		for _, instrument := range instruments {
			for _, member := range instrument.Holdings {
				quantity, err := decimal.NewFromString(member.Amounts.Quantity)
				if err != nil {
					return nil, err
				}
				if quantity.IsZero() {
					continue
				}
				byCurrency[instrument.QuoteCurrency] = append(byCurrency[instrument.QuoteCurrency], member)
				if counted[instrument.QuoteCurrency] == nil {
					counted[instrument.QuoteCurrency] = map[domain.InstrumentID]struct{}{}
				}
				counted[instrument.QuoteCurrency][instrument.InstrumentID] = struct{}{}
			}
		}
		currencies := make([]domain.CurrencyCode, 0, len(byCurrency))
		for currency := range byCurrency {
			currencies = append(currencies, currency)
		}
		sort.Slice(currencies, func(i, j int) bool { return currencies[i] < currencies[j] })
		subtotals := make([]domain.HoldingCurrencySubtotal, 0, len(currencies))
		for _, currency := range currencies {
			amounts, err := aggregateHoldingAmounts(byCurrency[currency], currency, exact)
			if err != nil {
				return nil, err
			}
			subtotals = append(subtotals, domain.HoldingCurrencySubtotal{Currency: currency, InstrumentCount: len(counted[currency]), Amounts: amounts})
		}
		groups = append(groups, domain.InstrumentHoldingGroupView{Key: key, Instruments: instruments, Subtotals: subtotals})
	}
	return groups, nil
}

// InstrumentHoldingGroups is the type-grouped Portfolio holdings read model.
func (s *Service) InstrumentHoldingGroups(ctx context.Context) ([]domain.InstrumentHoldingGroupView, error) {
	managed, err := s.InstrumentProductOwnership(ctx)
	if err != nil {
		return nil, err
	}
	return s.gain.InstrumentHoldingGroups(ctx, managed)
}

// AccountHoldingGroups is the same grouping for one account, including an
// archived account, matching AccountHoldingDetails scope.
func (s *Service) AccountHoldingGroups(ctx context.Context, accountID domain.AccountID) ([]domain.InstrumentHoldingGroupView, error) {
	managed, err := s.InstrumentProductOwnership(ctx)
	if err != nil {
		return nil, err
	}
	return s.gain.InstrumentHoldingGroups(ctx, managed, accountID)
}
