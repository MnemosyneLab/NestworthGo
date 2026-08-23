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

func QuoteFreshness(source QuoteSourceKind, delayed bool, quotedAt, now time.Time) Freshness {
	if source == QuoteSourceManual {
		return FreshnessManual
	}
	if quotedAt.IsZero() {
		return FreshnessUnavailable
	}
	age := now.Sub(quotedAt)
	if age > 24*time.Hour {
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
	Source    QuoteSourceKind
	SourceKey string
	QuotedAt  time.Time
	Freshness Freshness
	Delayed   bool
}

type MissingInputKind string

const (
	MissingInstrumentPrice MissingInputKind = "instrument_price"
	MissingFXRate          MissingInputKind = "fx_rate"
	MissingAccountValue    MissingInputKind = "account_value"
)

type MissingInputView struct {
	Kind          MissingInputKind
	AccountID     AccountID
	InstrumentID  *InstrumentID
	BaseCurrency  CurrencyCode
	QuoteCurrency CurrencyCode
}

// ValuationComponent keeps full-precision arithmetic separate from the
// returned Money view. The application owns the authoritative calculation;
// UI layers consume this vocabulary only.
type ValuationComponent struct {
	AccountID       AccountID
	InstrumentID    *InstrumentID
	NativeAmount    string
	NativeCurrency  CurrencyCode
	BaseAmount      *MoneyView
	BaseAmountExact string
	PriceEvidence   *QuoteEvidenceView
	FXEvidence      *QuoteEvidenceView
	Available       bool
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
	Household        *Household
	Members          []Member
	Institutions     []Institution
	Groups           []Group
	Accounts         []AccountRecord
	Instruments      []Instrument
	Holdings         []Holding
	CashValues       []AccountCashValue
	InstrumentQuotes []InstrumentQuote
	FXQuotes         []FXQuote
	FXPreferences    []FXPreference
}
