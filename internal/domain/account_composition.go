package domain

// AccountComposition is projected from the same snapshot and exact component
// amounts as account valuation. Empty exact amounts mean unavailable, not zero.
// Shares use TotalExact as their denominator, never rounded MoneyView values.
type AccountComposition struct {
	Currency     CurrencyCode
	TotalExact   string
	Complete     bool
	Status       string // ready, partial, empty, unavailable, invalid, unsupported
	ValuedCount  int
	MissingCount int
	Types        []AccountCompositionItem
	Details      []AccountCompositionItem
	DetailSlices []AccountCompositionItem
}

type AccountCompositionItem struct {
	Key             string
	TypeKey         string
	Label           string
	DetailRefs      []string
	BaseAmount      *MoneyView
	BaseAmountExact string
	ShareBPS        int
	NativeAmount    string
	NativeCurrency  CurrencyCode
	ValuedCount     int
	MissingCount    int
}
