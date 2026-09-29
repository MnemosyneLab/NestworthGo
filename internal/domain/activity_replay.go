package domain

import "github.com/shopspring/decimal"

func replayMoneyKey(target EffectTarget, account AccountID, currency CurrencyCode) string {
	return string(target) + ":" + account.String() + ":" + currency.String()
}

// ReplayMoneyEffects retains absolute balance observations when an earlier fact
// changes. Only their derived delta changes; recorded observation values remain
// intact. Subsequent undo records invert the recalculated delta as well.
func ReplayMoneyEffects(activities []Activity, components []HistoryOriginComponent) ([]Activity, error) {
	balances := map[string]decimal.Decimal{}
	for _, c := range components {
		if c.AccountID == nil || c.Amount == nil {
			continue
		}
		target := EffectTargetAccountValue
		if c.Kind == HistoryOriginAccountCash {
			target = EffectTargetAccountCash
		}
		balances[replayMoneyKey(target, *c.AccountID, c.Amount.Currency())] = c.Amount.Amount()
	}
	result := make([]Activity, 0, len(activities))
	prior := map[ActivityID][]ActivityEffect{}
	for _, a := range activities {
		a.Effects = append([]ActivityEffect{}, a.Effects...)
		if a.ReversesActivityID != nil {
			if original, ok := prior[*a.ReversesActivityID]; ok {
				if len(original) == 0 {
					a.Effects = nil
				}
				// Keep audit effect identities while using the replayed magnitudes.
				for i := range a.Effects {
					for _, e := range original {
						if sameReplayEndpoint(a.Effects[i], e) {
							a.Effects[i].Money = e.Money
							a.Effects[i].Quantity = e.Quantity
							a.Effects[i].Direction = EffectAdded
							if e.Direction == EffectAdded {
								a.Effects[i].Direction = EffectRemoved
							}
							break
						}
					}
				}
			}
		}
		effects := make([]ActivityEffect, 0, len(a.Effects))
		for _, e := range a.Effects {
			if e.Money != nil && e.AccountID != nil {
				key := replayMoneyKey(e.Target, *e.AccountID, e.Money.Currency())
				if a.Kind == ActivityValueUpdate {
					for _, v := range a.Resulting {
						if v.AccountID != nil && *v.AccountID == *e.AccountID && v.Target == e.Target && v.Currency == e.Money.Currency() {
							target, err := decimal.NewFromString(v.Amount)
							if err != nil {
								return nil, err
							}
							delta := target.Sub(balances[key])
							money, err := NewMoney(delta.Abs(), v.Currency)
							if err != nil {
								return nil, err
							}
							e.Money = &money
							e.Direction = EffectAdded
							if delta.IsNegative() {
								e.Direction = EffectRemoved
							}
							break
						}
					}
				}
				if e.Money.IsZero() {
					continue
				}
				delta := e.Money.Amount()
				if e.Direction == EffectRemoved {
					delta = delta.Neg()
				}
				balances[key] = balances[key].Add(delta)
			}
			effects = append(effects, e)
		}
		a.Effects = effects
		prior[a.ID] = effects
		result = append(result, a)
	}
	return result, nil
}

func sameReplayEndpoint(a, b ActivityEffect) bool {
	if a.Target != b.Target || a.Role != b.Role {
		return false
	}
	if a.HoldingID != nil && b.HoldingID != nil {
		return *a.HoldingID == *b.HoldingID
	}
	return a.AccountID != nil && b.AccountID != nil && *a.AccountID == *b.AccountID && a.Money != nil && b.Money != nil && a.Money.Currency() == b.Money.Currency()
}
