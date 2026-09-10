package sqlite

import (
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

type DateSpan struct {
	Start string
	End   string
}

type InstrumentHistoryObservation struct {
	MarketDate        string
	Value             string
	Currency          string
	ValueEffectiveAt  time.Time
	ProviderTimestamp time.Time
	Kind              string
	PriceBasis        string
	TimestampBasis    string
	SplitFactor       string
	DividendCash      string
}

type InstrumentHistoryCommit struct {
	HouseholdID    domain.HouseholdID
	InstrumentID   domain.InstrumentID
	ProviderKey    string
	ProviderSymbol string
	QuoteCurrency  domain.CurrencyCode
	Market         string
	Status         string
	Reason         string
	Adapter        string
	SourcePolicy   string
	FetchedAt      time.Time
	Observations   []InstrumentHistoryObservation
	VerifiedRanges []DateSpan
	PendingRanges  []DateSpan
	NextCheckAt    *time.Time
}

type FXHistoryObservation struct {
	MarketDate       string
	Rate             string
	BaseCurrency     string
	QuoteCurrency    string
	ValueEffectiveAt time.Time
	Kind             string
	TimestampBasis   string
}

type FXHistoryCommit struct {
	HouseholdID    domain.HouseholdID
	ProviderKey    string
	BaseCurrency   domain.CurrencyCode
	QuoteCurrency  domain.CurrencyCode
	Status         string
	Reason         string
	Adapter        string
	SourcePolicy   string
	FetchedAt      time.Time
	Observations   []FXHistoryObservation
	VerifiedRanges []DateSpan
	NextCheckAt    *time.Time
}

type HistoryCommitResult struct {
	PersistedObservations int
	NewRevisions          int
	CoverageDays          int
	CanonicalSlots        int
	InputGeneration       int
	Unchanged             bool
}

const (
	observationKindClose          = "close"
	observationKindDailyReference = "daily_reference"
	coverageStatusNoObservation   = "no_observation"
	coverageStatusPending         = "pending"
	mappingStatusMapped           = "mapped"
	priceBasisTiingoRawClose      = "tiingo_raw_close_v1"
)

// refuseFailClosedHistory is the persist-side fail-closed gate. Invalid and
// unsupported batches, and any Tiingo observation that is not raw `close`
// (`tiingo_raw_close_v1`), commit nothing: no quotes, slots, coverage, or
// input-generation bump. adjClose-only, unsupported markets, and malformed
// payloads are refused here even if a caller skipped the application check.
func refuseFailClosedHistory(status, reason, adapter string, tiingoValues []string) error {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "invalid", "unsupported":
		return failClosedPersistError(reason)
	}
	if strings.Contains(strings.ToLower(adapter), "tiingo") {
		for _, value := range tiingoValues {
			if value != "" && value != priceBasisTiingoRawClose {
				return failClosedPersistError("unsupported_price_basis")
			}
		}
	}
	return nil
}

func failClosedPersistError(reason string) error {
	message := strings.TrimSpace(reason)
	if message == "" {
		message = "fail-closed mapping"
	}
	return &domain.Error{Code: domain.ErrValidation, Field: "mapping", Message: "refusing to persist a fail-closed market-data batch: " + message}
}

func sourcePolicyVersion(policy string) string {
	if strings.TrimSpace(policy) != "" {
		return policy
	}
	return "unspecified"
}

func inclusiveDates(span DateSpan) ([]string, error) {
	start, err := domain.ParseMarketDate(span.Start)
	if err != nil {
		return nil, err
	}
	end, err := domain.ParseMarketDate(span.End)
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
	dates := make([]string, 0, int(last.Sub(current).Hours()/24)+1)
	for !current.After(last) {
		dates = append(dates, current.Format("2006-01-02"))
		current = current.AddDate(0, 0, 1)
	}
	return dates, nil
}
