package domain

import "github.com/shopspring/decimal"

func liquiditySourcePositiveOrUnknown(source LiquiditySource) bool {
	if source.CurrentNativeAmount != "" {
		value, _ := decimal.NewFromString(source.CurrentNativeAmount) // validated by EvaluateLiquidity
		return value.IsPositive()
	}
	return source.CurrentNativeValue == nil || source.CurrentNativeValue.Amount().IsPositive()
}

func applyLiquidityFee(source LiquiditySource, policy LiquidityPolicy, basis string, gross Money, fee *Money) (*Money, *decimal.Decimal, bool, bool, error) {
	exact := gross.Amount()
	if basis == "current_value" && source.CurrentNativeAmount != "" {
		value, err := decimal.NewFromString(source.CurrentNativeAmount)
		if err != nil {
			return nil, nil, false, false, err
		}
		exact = value
		if !source.Managed && policy.AccessibleAmountCap != nil {
			exact = decimal.Min(exact, policy.AccessibleAmountCap.Amount())
		}
	}
	if fee == nil || fee.Currency() != gross.Currency() {
		return nil, nil, true, false, nil
	}
	diff := exact.Sub(fee.Amount())
	exceeds := diff.IsNegative()
	if exceeds {
		diff = decimal.Zero
	}
	net, err := NewMoney(diff, gross.Currency())
	if err != nil {
		return nil, nil, false, exceeds, err
	}
	return &net, &diff, false, exceeds, nil
}

func routeNativeExact(route *LiquidityRoute) decimal.Decimal {
	if route.netExact != nil {
		return *route.netExact
	}
	return route.NetNative.Amount()
}
func bucketNativeExact(row *LiquidityBucketResult) decimal.Decimal {
	if row.netExact != nil {
		return *row.netExact
	}
	return row.NetNative.Amount()
}
func bucketReserveExact(row *LiquidityBucketResult) decimal.Decimal {
	if row.reserveExact != nil {
		return *row.reserveExact
	}
	if row.AppliedReserveNative != nil {
		return row.AppliedReserveNative.Amount()
	}
	return decimal.Zero
}

func applyLiquidityReservation(net decimal.Decimal, currency CurrencyCode, requested Money) (applied, unreserved, shortfall Money, exactApplied decimal.Decimal) {
	exactApplied = decimal.Min(net, requested.Amount())
	applied, _ = NewMoney(exactApplied, currency)
	displayedNet, _ := NewMoney(net, currency)
	// Preserve the displayed identity without rounding the calculation input.
	unreserved, _ = NewMoney(displayedNet.Amount().Sub(applied.Amount()), currency)
	shortfall, _ = NewMoney(decimal.Max(requested.Amount().Sub(net), decimal.Zero), currency)
	return
}

// ConvertToBase is a linear FX conversion under one fixed valuation snapshot.
// Convert one native unit to retain its full rate, then apply it to the exact
// valuation. Passing the rounded Money projection would lose precision twice.
func convertLiquidityExact(convert ConvertToBase, amount decimal.Decimal, currency CurrencyCode) (*decimal.Decimal, bool, error) {
	unit, err := NewMoney(decimal.NewFromInt(1), currency)
	if err != nil {
		return nil, false, err
	}
	rate, ok, err := convert(unit)
	if err != nil || !ok || rate == nil {
		return nil, false, err
	}
	result := amount.Mul(*rate)
	return &result, true, nil
}
