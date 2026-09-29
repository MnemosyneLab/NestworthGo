package domain

import "time"

// DerivedDataRebuild is a complete, staged replacement for one household's
// projections. Activities, opening balances, observations, and market data
// remain the source facts and are never replaced by this operation.
type DerivedDataRebuild struct {
	HouseholdID     HouseholdID
	Projections     []ActivityProjection
	Current         []EndpointView
	Snapshots       []DailyValuationSnapshot
	InputGeneration int
	AsOf            time.Time
	ClosedThrough   string
}
