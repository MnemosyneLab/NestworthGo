package domain

// Display valuation never grants strict coverage or return eligibility.
type ValuationAvailability string
type ValuationCoverage string
type ValuationReason string

const (
	ValuationFull             ValuationAvailability = "full"
	ValuationPartial          ValuationAvailability = "partial"
	ValuationNone             ValuationAvailability = "none"
	CoverageVerified          ValuationCoverage     = "verified"
	CoverageProvisional       ValuationCoverage     = "provisional"
	CoverageUnavailable       ValuationCoverage     = "unavailable"
	ReasonCarriedNAV          ValuationReason       = "carried_nav"
	ReasonCarriedPrice        ValuationReason       = "carried_price"
	ReasonCarriedFX           ValuationReason       = "carried_fx"
	ReasonHistoryUnverified   ValuationReason       = "history_unverified"
	ReasonPriceMissing        ValuationReason       = "price_missing"
	ReasonFXMissing           ValuationReason       = "fx_missing"
	ReasonAccountValueMissing ValuationReason       = "account_value_missing"
	ReasonEvidenceUnavailable ValuationReason       = "evidence_unavailable"
)

type ValuationQuality struct {
	Availability     ValuationAvailability `json:"availability"`
	Coverage         ValuationCoverage     `json:"coverage"`
	IncludedCount    int                   `json:"includedCount"`
	ValuedCount      int                   `json:"valuedCount"`
	UnpricedCount    int                   `json:"unpricedCount"`
	ProvisionalCount int                   `json:"provisionalCount"`
	CarriedCount     int                   `json:"carriedCount"`
	StaleCount       int                   `json:"staleCount"`
	OldestMarketDate *string               `json:"oldestMarketDate"`
	NewestMarketDate *string               `json:"newestMarketDate"`
	Reasons          []ValuationReason     `json:"reasons"`
}
type ValuationAmount struct {
	Currency      CurrencyCode     `json:"currency"`
	Total         *MoneyView       `json:"total"`
	KnownSubtotal *MoneyView       `json:"knownSubtotal"`
	Quality       ValuationQuality `json:"quality"`
}
type HouseholdDisplayValuation struct {
	Assets      ValuationAmount `json:"assets"`
	Liabilities ValuationAmount `json:"liabilities"`
	NetWorth    ValuationAmount `json:"netWorth"`
}
