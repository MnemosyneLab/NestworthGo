package domain

import "time"

// Freshness describes the selected observation without changing whether its
// value is usable. Stale and delayed values remain valued; unavailable values
// are represented by a missing input instead.
type Freshness string

const (
	FreshnessFresh       Freshness = "fresh"
	FreshnessDelayed     Freshness = "delayed"
	FreshnessStale       Freshness = "stale"
	FreshnessManual      Freshness = "manual"
	FreshnessUnavailable Freshness = "unavailable"
)

const (
	// QuoteClockSkewTolerance is the amount of provider clock skew that can be
	// tolerated without treating an observation as unusable. Provider adapters
	// and the application boundary both use the same limit before a quote can
	// be persisted.
	QuoteClockSkewTolerance = 5 * time.Minute
)

// ProviderObservationEarliest is the lower bound for externally supplied
// provider observations. Manual observations are not subject to this
// provider-only bound.
func ProviderObservationEarliest() time.Time {
	return time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func QuoteFreshness(source QuoteSourceKind, delayed bool, quotedAt, now time.Time, ttl time.Duration) Freshness {
	if source == QuoteSourceManual {
		return FreshnessManual
	}
	if quotedAt.IsZero() || now.IsZero() || quotedAt.Before(ProviderObservationEarliest()) {
		return FreshnessUnavailable
	}
	age := now.Sub(quotedAt)
	if quotedAt.After(now.Add(QuoteClockSkewTolerance)) {
		return FreshnessUnavailable
	}
	if ttl <= 0 {
		ttl = 12 * time.Hour
	}
	if age >= ttl {
		return FreshnessStale
	}
	if delayed {
		return FreshnessDelayed
	}
	return FreshnessFresh
}

type MoneyView struct {
	Amount   string
	Currency CurrencyCode
}

type QuoteEvidenceView struct {
	ObservationID string
	Source        QuoteSourceKind
	SourceKey     string
	QuotedAt      time.Time
	Freshness     Freshness
	Delayed       bool
}

type MissingInputKind string

const (
	MissingInstrumentPrice MissingInputKind = "instrument_price"
	MissingInstrument      MissingInputKind = "missing_instrument"
	MissingFXRate          MissingInputKind = "fx_rate"
	MissingAccountValue    MissingInputKind = "account_value"
	MissingHistoryCoverage MissingInputKind = "history_coverage"
)

type MissingInputView struct {
	Kind             MissingInputKind
	AccountID        AccountID
	AccountName      string
	InstrumentID     *InstrumentID
	InstrumentName   string
	InstrumentSymbol string
	QuoteSource      QuoteSourceKind
	BaseCurrency     CurrencyCode
	QuoteCurrency    CurrencyCode
}

// ValuationComponent keeps full-precision arithmetic separate from the
// returned Money view. The application owns the authoritative calculation;
// UI layers consume this vocabulary only.
type ValuationComponent struct {
	AccountID                 AccountID
	HoldingID                 *HoldingID
	InstrumentID              *InstrumentID
	StateObservationID        *AccountStateObservationID
	FXPreferenceObservationID *FXPreferenceObservationID
	InstrumentName            string
	InstrumentSymbol          string
	NativeAmount              string
	NativeCurrency            CurrencyCode
	BaseAmount                *MoneyView
	BaseAmountExact           string
	PriceEvidence             *QuoteEvidenceView
	FXEvidence                *QuoteEvidenceView
	PreferenceObservationID   *InstrumentPreferenceObservationID
	Available                 bool
}

type ValuationResult struct {
	Native        *MoneyView
	Base          *MoneyView
	Complete      bool
	Components    []ValuationComponent
	MissingInputs []MissingInputView
}

type AccountValuation struct {
	Account         Account
	Ownership       Ownership
	InstitutionName string
	GroupName       string
	BaseValue       *MoneyView
	Complete        bool
	Components      []ValuationComponent
	MissingInputs   []MissingInputView
}

type AllocationView struct {
	Key      string
	Label    string
	Amount   MoneyView
	ShareBPS int
}

type PortfolioValuation struct {
	Currency         CurrencyCode
	ValuedSubtotal   *MoneyView
	Complete         bool
	Accounts         []AccountValuation
	MissingInputs    []MissingInputView
	ByCurrency       []AllocationView
	ByCountry        []AllocationView
	ByInstrumentType []AllocationView
}

// PortfolioSnapshot is the single read boundary consumed by valuation and
// portfolio summaries. Infrastructure populates it in one read transaction;
// callers must not query one quote or Holding at a time.
type PortfolioSnapshot struct {
	Household                 *Household
	Origin                    *HistoryOrigin
	Members                   []Member
	Institutions              []Institution
	Groups                    []Group
	Accounts                  []AccountRecord
	Instruments               []Instrument
	Holdings                  []Holding
	CashValues                []AccountCashValue
	InstrumentQuotes          []InstrumentQuote
	FXQuotes                  []FXQuote
	FXPreferences             []FXPreference
	InstrumentHistoryCoverage []InstrumentHistoryCoverage
	FXHistoryCoverage         []FXHistoryCoverage
}
