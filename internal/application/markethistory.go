package application

import "time"

// Historical market-data types encode the vNext adapter-mapping contract.
// Persistence, provider history methods, and secret storage remain later
// steps; these types make fixture qualification executable now.

// MarketDate is an inclusive YYYY-MM-DD market-session or FX-reference label.
// It is not an economic timestamp and must not be copied onto a household
// snapshot date.
type MarketDate string

type DateRange struct {
	Start MarketDate
	End   MarketDate
}

type PriceBasis string

const (
	PriceBasisTiingoRawClose PriceBasis = "tiingo_raw_close_v1"
	PriceBasisYahooClose     PriceBasis = "yahoo_close_unverified"
	PriceBasisUnsupported    PriceBasis = "unsupported"
)

type TimestampBasis string

const (
	TimestampBasisSessionClose        TimestampBasis = "session_close"
	TimestampBasisPolicyDerived       TimestampBasis = "policy_derived"
	TimestampBasisObservedPublication TimestampBasis = "observed_publication"
	TimestampBasisUnknown             TimestampBasis = "unknown"
)

type CoverageStatus string

const (
	CoverageNoObservation CoverageStatus = "no_observation"
	CoveragePending       CoverageStatus = "pending"
	CoverageUncertain     CoverageStatus = "uncertain"
)

type MappingStatus string

const (
	MappingMapped      MappingStatus = "mapped"
	MappingPending     MappingStatus = "pending"
	MappingUncertain   MappingStatus = "uncertain"
	MappingUnsupported MappingStatus = "unsupported"
	MappingInvalid     MappingStatus = "invalid"
)

type InstrumentObservationKind string

const (
	InstrumentObservationManual   InstrumentObservationKind = "manual"
	InstrumentObservationRealtime InstrumentObservationKind = "realtime"
	InstrumentObservationClose    InstrumentObservationKind = "close"
	InstrumentObservationLegacy   InstrumentObservationKind = "legacy"
)

type FXObservationKind string

const (
	FXObservationManual         FXObservationKind = "manual"
	FXObservationLatest         FXObservationKind = "latest"
	FXObservationDailyReference FXObservationKind = "daily_reference"
	FXObservationLegacy         FXObservationKind = "legacy"
)

type ResponseEvidence struct {
	Adapter         string
	AdapterVersion  string
	SourcePolicy    string
	RequestIdentity string
	PriceBasis      PriceBasis
	TimestampBasis  TimestampBasis
	SessionPolicy   string
}

type InstrumentDailyObservation struct {
	MarketDate        MarketDate
	Value             string
	Currency          string
	ValueEffectiveAt  time.Time
	ProviderTimestamp time.Time
	Kind              InstrumentObservationKind
	PriceBasis        PriceBasis
	TimestampBasis    TimestampBasis
	SplitFactor       string
	DividendCash      string
}

type FXDailyObservation struct {
	MarketDate       MarketDate
	Rate             string
	BaseCurrency     string
	QuoteCurrency    string
	ValueEffectiveAt time.Time
	Kind             FXObservationKind
	TimestampBasis   TimestampBasis
	Derived          bool
	SourcePolicy     string
}

type HistoryBatch[T any] struct {
	Observations    []T
	VerifiedRanges  []DateRange
	PendingRanges   []DateRange
	UncertainRanges []DateRange
	NextCheckAt     *time.Time
	Evidence        ResponseEvidence
}

type MappingOutcome[T any] struct {
	Status MappingStatus
	Reason string
	Batch  HistoryBatch[T]
}
