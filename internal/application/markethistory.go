package application

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// Historical market-data types encode the vNext adapter-mapping and persist
// contracts. Live provider HTTP remains a later step; these types make
// fixture qualification and offline persistence executable now.

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

// InstrumentHistoryProvider is an optional history capability. It is not part
// of MarketDataProvider so latest-only fakes and adapters stay source
// compatible.
type InstrumentHistoryProvider interface {
	InstrumentDailyHistory(context.Context, InstrumentMarketIdentity, DateRange) (HistoryBatch[InstrumentDailyObservation], error)
}

// FXHistoryProvider is the FX counterpart of InstrumentHistoryProvider.
type FXHistoryProvider interface {
	FXDailyHistory(context.Context, FXMarketIdentity, DateRange) (HistoryBatch[FXDailyObservation], error)
}

type CommitInstrumentHistoryRequest struct {
	HouseholdID  domain.HouseholdID
	InstrumentID domain.InstrumentID
	Identity     InstrumentMarketIdentity
	Market       string
	Outcome      MappingOutcome[InstrumentDailyObservation]
	FetchedAt    time.Time
}

type CommitFXHistoryRequest struct {
	HouseholdID domain.HouseholdID
	Identity    FXMarketIdentity
	Outcome     MappingOutcome[FXDailyObservation]
	FetchedAt   time.Time
}

type CommitHistoryResult struct {
	PersistedObservations int
	NewRevisions          int
	CoverageDays          int
	CanonicalSlots        int
	InputGeneration       int
	Unchanged             bool
}

func (c MarketDataCapabilities) SupportsInstrumentHistory() bool {
	return c.InstrumentDailyHistory
}

func (c MarketDataCapabilities) SupportsFXHistory() bool {
	return c.FXDailyHistory
}

func SourcePolicyVersion(evidence ResponseEvidence) string {
	if policy := strings.TrimSpace(evidence.SourcePolicy); policy != "" {
		return policy
	}
	if evidence.PriceBasis != "" {
		return string(evidence.PriceBasis)
	}
	return "unspecified"
}

func InclusiveMarketDates(r DateRange) ([]MarketDate, error) {
	start, err := domain.ParseMarketDate(string(r.Start))
	if err != nil {
		return nil, err
	}
	end, err := domain.ParseMarketDate(string(r.End))
	if err != nil {
		return nil, err
	}
	if start > end {
		return nil, &domain.Error{Code: domain.ErrValidation, Field: "dateRange", Message: "range end must not precede start"}
	}
	current, err := time.Parse("2006-01-02", start)
	if err != nil {
		return nil, err
	}
	last, err := time.Parse("2006-01-02", end)
	if err != nil {
		return nil, err
	}
	dates := make([]MarketDate, 0, int(last.Sub(current).Hours()/24)+1)
	for !current.After(last) {
		dates = append(dates, MarketDate(current.Format("2006-01-02")))
		current = current.AddDate(0, 0, 1)
	}
	return dates, nil
}

// RefuseFailClosedInstrumentHistory returns a validation error when a mapped
// batch must not be persisted. Invalid, unsupported, and Tiingo adjClose-only
// or unknown-session outcomes commit nothing.
func RefuseFailClosedInstrumentHistory(outcome MappingOutcome[InstrumentDailyObservation]) error {
	if err := refuseFailClosedStatus(outcome.Status, outcome.Reason); err != nil {
		return err
	}
	for _, observation := range outcome.Batch.Observations {
		if strings.TrimSpace(observation.Value) == "" {
			return failClosedPersistError(outcome.Reason, "empty_observation_value")
		}
		if observation.PriceBasis == PriceBasisUnsupported {
			return failClosedPersistError("unsupported_price_basis", "unsupported_price_basis")
		}
		if isTiingoAdapter(outcome.Batch.Evidence.Adapter) && observation.PriceBasis != PriceBasisTiingoRawClose {
			return failClosedPersistError("unsupported_price_basis", "unsupported_price_basis")
		}
	}
	return nil
}

func RefuseFailClosedFXHistory(outcome MappingOutcome[FXDailyObservation]) error {
	if err := refuseFailClosedStatus(outcome.Status, outcome.Reason); err != nil {
		return err
	}
	for _, observation := range outcome.Batch.Observations {
		if strings.TrimSpace(observation.Rate) == "" {
			return failClosedPersistError(outcome.Reason, "empty_observation_value")
		}
	}
	return nil
}

func refuseFailClosedStatus(status MappingStatus, reason string) error {
	switch status {
	case MappingInvalid, MappingUnsupported:
		return failClosedPersistError(reason, reason)
	}
	return nil
}

func isTiingoAdapter(adapter string) bool {
	return strings.Contains(strings.ToLower(adapter), "tiingo")
}

func failClosedPersistError(reason, fallback string) error {
	message := strings.TrimSpace(reason)
	if message == "" {
		message = fallback
	}
	return &domain.Error{Code: domain.ErrValidation, Field: "mapping", Message: "refusing to persist a fail-closed market-data batch: " + message}
}
