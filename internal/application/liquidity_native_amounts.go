package application

import (
	"slices"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// LiquidityNativeAmounts distinguishes known zero from an estimate whose
// contributing sources are incomplete. Values are in the requested currency.
type LiquidityNativeAmounts struct {
	Cash, Proceeds *decimal.Decimal
}

// NativeSourceAmounts splits selected native access estimates into accessible
// account cash and sale or redemption proceeds from holdings, including products.
// As in the desktop comparison, an incomplete contributing source hides its
// category total and does not affect the other category.
func (r LiquidityReadResult) NativeSourceAmounts(horizon string) map[domain.CurrencyCode]LiquidityNativeAmounts {
	type category struct {
		amount  decimal.Decimal
		unknown bool
	}
	type breakdown struct {
		cash, proceeds category
	}
	groups := make(map[domain.CurrencyCode]*breakdown)
	for _, source := range r.Sources {
		source := source.Result
		if source.Excluded {
			continue
		}
		group := groups[source.NativeCurrency]
		if group == nil {
			group = &breakdown{}
			groups[source.NativeCurrency] = group
		}
		part := &group.cash
		if source.Ref.Kind == domain.SourceHolding {
			part = &group.proceeds
		}
		index := slices.IndexFunc(source.BucketResults, func(result domain.LiquidityBucketResult) bool { return result.HorizonOn == horizon })
		if index < 0 {
			part.unknown = true
			continue
		}
		result := source.BucketResults[index]
		if result.Status != domain.StatusComplete || result.SelectedRoute != nil && result.NetNative == nil {
			part.unknown = true
			continue
		}
		if result.SelectedRoute != nil {
			part.amount = part.amount.Add(result.NetNative.Amount())
		}
	}
	out := make(map[domain.CurrencyCode]LiquidityNativeAmounts, len(groups))
	for currency, group := range groups {
		var amounts LiquidityNativeAmounts
		if !group.cash.unknown {
			amounts.Cash = &group.cash.amount
		}
		if !group.proceeds.unknown {
			amounts.Proceeds = &group.proceeds.amount
		}
		out[currency] = amounts
	}
	return out
}
