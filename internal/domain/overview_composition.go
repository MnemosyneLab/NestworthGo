package domain

import "github.com/shopspring/decimal"

// OverviewAssetComposition partitions the known asset subtotal into positive
// and negative components. Only positive components have Sankey geometry.
type OverviewAssetComposition struct {
	Positive, Negative      decimal.Decimal
	ZeroCount, MissingCount int
	Paths                   []OverviewAssetPath
	Nodes                   []OverviewAssetNode
}

// OverviewAssetPath is the exact positive allocation in one asset class for
// an account. An empty institution ID means the account is unassigned.
type OverviewAssetPath struct {
	AssetClass      string
	InstitutionID   string
	InstitutionName string
	AccountID       string
	AccountName     string
	Amount          decimal.Decimal
}

// OverviewAssetNode identifies a real branch of the exact allocation tree.
// IDs are unique per path. Key identifies the dimension value within Kind.
type OverviewAssetNode struct {
	ID, ParentID, Kind, Key string
	Amount                  decimal.Decimal
}
