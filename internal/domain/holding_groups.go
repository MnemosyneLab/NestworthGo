package domain

// HoldingGroupKey is the presentation category a holding list is grouped by.
// It is derived only from an instrument's type and contract ownership; it never
// influences a valuation, cost or gain calculation.
type HoldingGroupKey string

const (
	HoldingGroupStock      HoldingGroupKey = "stock"
	HoldingGroupFund       HoldingGroupKey = "fund"
	HoldingGroupBond       HoldingGroupKey = "bond"
	HoldingGroupBankWealth HoldingGroupKey = "bank_wealth"
	HoldingGroupMetal      HoldingGroupKey = "precious_metal"
	HoldingGroupCrypto     HoldingGroupKey = "crypto"
	HoldingGroupCashLike   HoldingGroupKey = "cash_like"
	HoldingGroupOther      HoldingGroupKey = "other"
)

// HoldingGroupOrder is the default display order, from the most familiar
// market instruments to contract-based and miscellaneous holdings.
var HoldingGroupOrder = []HoldingGroupKey{
	HoldingGroupStock, HoldingGroupFund, HoldingGroupBond, HoldingGroupBankWealth,
	HoldingGroupMetal, HoldingGroupCrypto, HoldingGroupCashLike, HoldingGroupOther,
}

// HoldingGroupFor classifies one instrument. Instruments owned by a managed
// product contract are cash-like regardless of their stored type, because they
// are valued by contract rather than by a unit price.
func HoldingGroupFor(instrumentType InstrumentType, managed bool) HoldingGroupKey {
	if managed {
		return HoldingGroupCashLike
	}
	switch instrumentType {
	case InstrumentStock:
		return HoldingGroupStock
	case InstrumentETF, InstrumentMutualFund:
		return HoldingGroupFund
	case InstrumentBond:
		return HoldingGroupBond
	case InstrumentBankInvestmentProduct:
		return HoldingGroupBankWealth
	case InstrumentPreciousMetal:
		return HoldingGroupMetal
	case InstrumentCrypto:
		return HoldingGroupCrypto
	default:
		return HoldingGroupOther
	}
}

// HoldingCurrencySubtotal sums active positions that share one native
// currency. Currencies are never converted or merged: a group holding several
// currencies reports one subtotal for each of them.
type HoldingCurrencySubtotal struct {
	Currency        CurrencyCode
	InstrumentCount int
	Amounts         HoldingAmounts
}

// InstrumentHoldingGroupView is one collapsible type section of the holdings
// list. Instruments keep the order produced by the instrument read model.
type InstrumentHoldingGroupView struct {
	Key         HoldingGroupKey
	Instruments []InstrumentHoldingsView
	Subtotals   []HoldingCurrencySubtotal
}
