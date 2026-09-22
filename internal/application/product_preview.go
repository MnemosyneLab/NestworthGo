package application

import (
	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// Value the affected holdings using the same authority as portfolio reads. A
// missing quote remains unknown; cash/contract principal is never a fallback.
func (s *Service) productPreviewValues(snapshot domain.PortfolioSnapshot, plan productPlan) (*domain.Money, *domain.Money, error) {
	contracts := append(append([]domain.ProductContract{}, plan.beforeContracts...), plan.contracts...)
	if len(contracts) == 0 {
		return nil, nil, nil
	}
	currency := contracts[0].Currency
	targets := map[domain.HoldingID]bool{}
	for _, c := range contracts {
		if c.Currency != currency {
			return nil, nil, nil
		}
		targets[c.HoldingID] = true
	}
	value := func(snap domain.PortfolioSnapshot) (*domain.Money, error) {
		values, _, err := s.valuation.ValueAccounts(snap)
		if err != nil {
			return nil, err
		}
		components := map[domain.HoldingID]domain.ValuationComponent{}
		for _, a := range values {
			for _, c := range a.Components {
				if c.HoldingID != nil {
					components[*c.HoldingID] = c
				}
			}
		}
		total := decimal.Zero
		for _, h := range snap.Holdings {
			if !targets[h.ID] || h.Quantity.IsZero() {
				continue
			}
			c, ok := components[h.ID]
			if !ok || c.NativeAmount == "" {
				return nil, nil
			}
			raw, err := domain.ParseNativeAmount(c.NativeAmount)
			if err != nil {
				return nil, err
			}
			amount, err := decimal.NewFromString(raw)
			if err != nil {
				return nil, err
			}
			total = total.Add(amount)
		}
		m, err := domain.NewMoney(total, currency)
		return &m, err
	}
	before, err := value(snapshot)
	if err != nil {
		return nil, nil, err
	}
	afterSnapshot := snapshot
	afterSnapshot.Holdings = append(append([]domain.Holding{}, snapshot.Holdings...), plan.holdings...)
	for i := range afterSnapshot.Holdings {
		if h, ok := plan.state.Holdings[afterSnapshot.Holdings[i].ID]; ok {
			afterSnapshot.Holdings[i].Quantity = h.Current
		}
	}
	afterSnapshot.Instruments = append(append([]domain.Instrument{}, snapshot.Instruments...), plan.instruments...)
	afterSnapshot.InstrumentQuotes = append(append([]domain.InstrumentQuote{}, snapshot.InstrumentQuotes...), plan.quotes...)
	after, err := value(afterSnapshot)
	return before, after, err
}

func productNetWorthDelta(before, after *domain.Money, cashBefore, cashAfter []domain.Money) (*domain.SignedMoney, error) {
	if before == nil || after == nil {
		return nil, nil
	}
	delta := after.Amount().Sub(before.Amount())
	for _, m := range cashBefore {
		if m.Currency() != before.Currency() {
			return nil, nil
		}
		delta = delta.Sub(m.Amount())
	}
	for _, m := range cashAfter {
		if m.Currency() != before.Currency() {
			return nil, nil
		}
		delta = delta.Add(m.Amount())
	}
	result, err := domain.ParseSignedMoney(delta.String(), before.Currency())
	return &result, err
}
