package domain

import (
	"time"
)

// CostBasisEventKind identifies one cost-bearing or quantity-reducing fact.
// The application layer builds these values from already-loaded immutable
// Activity rows; ReplayCostBasis itself performs no I/O and does not inspect a
// clock or a provider.
type CostBasisEventKind string

const (
	CostBasisStartingPoint CostBasisEventKind = "starting_point"
	CostBasisBuy           CostBasisEventKind = "buy"
	CostBasisSell          CostBasisEventKind = "sell"
	CostBasisTransferIn    CostBasisEventKind = "transfer_in"
	CostBasisTransferOut   CostBasisEventKind = "transfer_out"
	CostBasisAdjustmentIn  CostBasisEventKind = "adjustment_in"
	CostBasisAdjustmentOut CostBasisEventKind = "adjustment_out"
)

// CostBasisEvent is one Holding-scoped event in replay order. A StartingPoint
// event carries the captured quantity; startingCost is passed separately to
// ReplayCostBasis to keep the public function aligned with the release
// contract. UnitCost is accepted on the StartingPoint event as a convenient
// serialized form and is otherwise used for TransferIn/AdjustmentIn. UnitPrice
// is used by Buy/Sell. Currency is required for a Sell so a signed realized
// amount can retain its settlement currency.
type CostBasisEvent struct {
	ActivityID         ActivityID
	EffectiveAt        time.Time
	Kind               CostBasisEventKind
	Quantity           Quantity
	UnitPrice          *UnitPrice
	UnitCost           *UnitPrice
	SourceHoldingID    *HoldingID
	Fee                *Money
	Currency           CurrencyCode
	ReversesActivityID *ActivityID
}

// CostLot is the current quantity and its average per-unit cost.
type CostLot struct {
	AverageUnitCost UnitPrice
	Quantity        Quantity
}

// RealizedGainEvent records the signed result of one Sell. SignedMoney is
// intentionally distinct from Money because a loss is a valid realized result
// while persisted balances and values remain non-negative Money facts.
type RealizedGainEvent struct {
	ActivityID   ActivityID
	EffectiveAt  time.Time
	Quantity     Quantity
	UnitCost     UnitPrice
	UnitProceeds UnitPrice
	Fee          *Money
	RealizedGain SignedMoney
}

type CostBasisResult struct {
	Current  CostLot
	Realized []RealizedGainEvent
}

// CostBasisReadFilter controls historical cost-event reads. Current-position
// queries keep their own archive filters; this option exists so realized-gain
// and transfer-source replay can still load immutable facts after a Holding
// is archived.
type CostBasisReadFilter struct {
	IncludeArchivedHoldings bool
}

// ReplayCostBasis walks a Holding's Starting Point cost and ordered immutable
// Activity facts. Average costs keep the supported UnitPrice scale of eight
// fractional digits; banker's rounding is applied only when a blend divides
// past that scale (for example, 1400/15 becomes 93.33333333). No binary float
// or external state is involved.
func ReplayCostBasis(startingCost *UnitPrice, events []CostBasisEvent) (CostBasisResult, error) {
	currentQuantity, err := ParseQuantity("0")
	if err != nil {
		return CostBasisResult{}, err
	}
	zeroCost, err := ParseUnitPrice("0")
	if err != nil {
		return CostBasisResult{}, err
	}
	result := CostBasisResult{Current: CostLot{AverageUnitCost: zeroCost, Quantity: currentQuantity}, Realized: make([]RealizedGainEvent, 0)}
	if len(events) == 0 {
		if startingCost != nil {
			return CostBasisResult{}, invalidCostBasis("startingCost", "a Starting Point quantity is required")
		}
		return result, nil
	}

	excluded := reversedActivityIDs(events)
	started := false
	hasCost := false
	for _, event := range events {
		if excluded[event.ActivityID] {
			continue
		}
		switch event.Kind {
		case CostBasisStartingPoint:
			if started {
				return CostBasisResult{}, invalidCostBasis("event", "only one Starting Point is allowed")
			}
			started = true
			if event.Quantity.IsZero() {
				continue
			}
			cost := startingCost
			if cost == nil {
				cost = event.UnitCost
			}
			if cost == nil {
				return CostBasisResult{}, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "a per-unit cost is required for a positive Starting Point quantity"}
			}
			result.Current = CostLot{AverageUnitCost: *cost, Quantity: event.Quantity}
			hasCost = true
		case CostBasisBuy, CostBasisTransferIn, CostBasisAdjustmentIn:
			if event.Quantity.IsZero() {
				return CostBasisResult{}, invalidCostBasis("quantity", "cost-basis event quantity must be greater than zero")
			}
			incoming, incomingErr := eventUnitCost(event)
			if incomingErr != nil {
				return CostBasisResult{}, incomingErr
			}
			result.Current, err = blendCost(result.Current, incoming, event.Quantity, hasCost)
			if err != nil {
				return CostBasisResult{}, err
			}
			hasCost = true
		case CostBasisSell:
			if event.Quantity.IsZero() {
				return CostBasisResult{}, invalidCostBasis("quantity", "cost-basis event quantity must be greater than zero")
			}
			if !hasCost || result.Current.Quantity.IsZero() {
				return CostBasisResult{}, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "a Sell requires a resolvable average cost"}
			}
			if result.Current.Quantity.Decimal().LessThan(event.Quantity.Decimal()) {
				return CostBasisResult{}, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "Sell exceeds the cost-basis quantity"}
			}
			proceeds := event.UnitPrice
			if proceeds == nil {
				return CostBasisResult{}, invalidCostBasis("unitPrice", "Sell requires a unit price")
			}
			currency, currencyErr := ParseCurrency(event.Currency.String())
			if currencyErr != nil {
				return CostBasisResult{}, invalidCostBasis("currency", "Sell requires a settlement currency")
			}
			if event.Fee != nil && event.Fee.Currency() != currency {
				return CostBasisResult{}, invalidCostBasis("fee", "Sell fee currency must match settlement currency")
			}
			gainValue := proceeds.Decimal().Sub(result.Current.AverageUnitCost.Decimal()).Mul(event.Quantity.Decimal())
			if event.Fee != nil {
				gainValue = gainValue.Sub(event.Fee.Amount())
			}
			gain, gainErr := NewSignedMoney(gainValue, currency)
			if gainErr != nil {
				return CostBasisResult{}, gainErr
			}
			result.Realized = append(result.Realized, RealizedGainEvent{ActivityID: event.ActivityID, EffectiveAt: event.EffectiveAt, Quantity: event.Quantity, UnitCost: result.Current.AverageUnitCost, UnitProceeds: *proceeds, Fee: event.Fee, RealizedGain: gain})
			result.Current, err = reduceCost(result.Current, event.Quantity)
			if err != nil {
				return CostBasisResult{}, err
			}
			hasCost = !result.Current.Quantity.IsZero()
		case CostBasisTransferOut, CostBasisAdjustmentOut:
			if event.Quantity.IsZero() {
				return CostBasisResult{}, invalidCostBasis("quantity", "quantity-reducing event must be greater than zero")
			}
			if !hasCost || result.Current.Quantity.IsZero() {
				return CostBasisResult{}, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "a quantity reduction requires a resolvable average cost"}
			}
			result.Current, err = reduceCost(result.Current, event.Quantity)
			if err != nil {
				return CostBasisResult{}, err
			}
			hasCost = !result.Current.Quantity.IsZero()
		default:
			return CostBasisResult{}, invalidCostBasis("kind", "cost-basis event kind is not supported")
		}
	}
	if startingCost != nil && !started {
		return CostBasisResult{}, invalidCostBasis("startingCost", "startingCost requires a Starting Point event")
	}
	return result, nil
}

func reversedActivityIDs(events []CostBasisEvent) map[ActivityID]bool {
	excluded := make(map[ActivityID]bool)
	for _, event := range events {
		if event.ReversesActivityID == nil {
			continue
		}
		excluded[event.ActivityID] = true
		excluded[*event.ReversesActivityID] = true
	}
	return excluded
}

func eventUnitCost(event CostBasisEvent) (*UnitPrice, error) {
	switch event.Kind {
	case CostBasisBuy:
		if event.UnitPrice == nil {
			return nil, invalidCostBasis("unitPrice", "Buy requires a unit price")
		}
		return event.UnitPrice, nil
	case CostBasisTransferIn, CostBasisAdjustmentIn:
		if event.UnitCost == nil {
			return nil, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "an incoming quantity requires a per-unit cost"}
		}
		return event.UnitCost, nil
	default:
		return nil, invalidCostBasis("kind", "event does not provide an incoming cost")
	}
}

func blendCost(current CostLot, incoming *UnitPrice, incomingQuantity Quantity, hasCost bool) (CostLot, error) {
	if incoming == nil {
		return CostLot{}, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "an incoming quantity requires a per-unit cost"}
	}
	total, err := NewQuantity(current.Quantity.Decimal().Add(incomingQuantity.Decimal()))
	if err != nil {
		return CostLot{}, err
	}
	if current.Quantity.IsZero() || !hasCost {
		return CostLot{AverageUnitCost: *incoming, Quantity: total}, nil
	}
	oldValue, err := current.Quantity.Multiply(current.AverageUnitCost)
	if err != nil {
		return CostLot{}, err
	}
	incomingValue, err := incomingQuantity.Multiply(*incoming)
	if err != nil {
		return CostLot{}, err
	}
	sum := oldValue.Add(incomingValue).Div(total.Decimal())
	average, err := UnitPriceFromExact(sum)
	if err != nil {
		return CostLot{}, err
	}
	return CostLot{AverageUnitCost: average, Quantity: total}, nil
}

func reduceCost(current CostLot, quantity Quantity) (CostLot, error) {
	if current.Quantity.Decimal().LessThan(quantity.Decimal()) {
		return CostLot{}, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "quantity reduction exceeds the current cost-basis quantity"}
	}
	remaining, err := NewQuantity(current.Quantity.Decimal().Sub(quantity.Decimal()))
	if err != nil {
		return CostLot{}, err
	}
	if remaining.IsZero() {
		zero, zeroErr := ParseUnitPrice("0")
		if zeroErr != nil {
			return CostLot{}, zeroErr
		}
		return CostLot{AverageUnitCost: zero, Quantity: remaining}, nil
	}
	return CostLot{AverageUnitCost: current.AverageUnitCost, Quantity: remaining}, nil
}

func invalidCostBasis(field, message string) error {
	return &Error{Code: ErrInvalidChange, Field: field, Message: message}
}
