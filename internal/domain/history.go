package domain

import (
	"strings"
	"time"
)

type HistoryOrigin struct {
	ID          HistoryOriginID
	HouseholdID HouseholdID
	Timezone    string
	StartedAt   time.Time
	CreatedAt   time.Time
}

func NewHistoryOrigin(householdID HouseholdID, timezone string, startedAt, createdAt time.Time) (HistoryOrigin, error) {
	if _, err := ParseHouseholdID(householdID.String()); err != nil {
		return HistoryOrigin{}, &Error{Code: ErrValidation, Field: "householdId", Message: "must be a valid Household ID"}
	}
	timezone = strings.TrimSpace(timezone)
	if timezone == "" {
		return HistoryOrigin{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "confirm a Household timezone before starting history"}
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return HistoryOrigin{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "timezone must be a valid IANA timezone"}
	}
	if startedAt.IsZero() {
		return HistoryOrigin{}, &Error{Code: ErrValidation, Field: "startedAt", Message: "start time is required"}
	}
	if createdAt.IsZero() {
		createdAt = startedAt
	}
	return HistoryOrigin{ID: NewHistoryOriginID(), HouseholdID: householdID, Timezone: timezone, StartedAt: normalizeTime(startedAt), CreatedAt: normalizeTime(createdAt)}, nil
}

type HistoryOriginComponentKind string

const (
	HistoryOriginAccountValue    HistoryOriginComponentKind = "account_value"
	HistoryOriginAccountCash     HistoryOriginComponentKind = "account_cash"
	HistoryOriginHoldingQuantity HistoryOriginComponentKind = "holding_quantity"
)

type HistoryOriginComponent struct {
	ID           HistoryOriginComponentID
	OriginID     HistoryOriginID
	Kind         HistoryOriginComponentKind
	AccountID    *AccountID
	HoldingID    *HoldingID
	InstrumentID *InstrumentID
	Amount       *Money
	Quantity     *Quantity
	CreatedAt    time.Time
}

func (c HistoryOriginComponent) Validate() error {
	if c.ID == "" || c.OriginID == "" || c.CreatedAt.IsZero() {
		return &Error{Code: ErrValidation, Field: "component", Message: "component identity and creation time are required"}
	}
	switch c.Kind {
	case HistoryOriginAccountValue, HistoryOriginAccountCash:
		if c.AccountID == nil || c.Amount == nil || c.HoldingID != nil || c.InstrumentID != nil || c.Quantity != nil {
			return &Error{Code: ErrValidation, Field: "component", Message: "Account component requires Account and Money"}
		}
	case HistoryOriginHoldingQuantity:
		if c.AccountID == nil || c.HoldingID == nil || c.InstrumentID == nil || c.Quantity == nil || c.Amount != nil {
			return &Error{Code: ErrValidation, Field: "component", Message: "Holding component requires Account, Holding, Instrument, and Quantity"}
		}
	default:
		return &Error{Code: ErrValidation, Field: "componentKind", Message: "component kind is not supported"}
	}
	return nil
}

type HistoryOriginAccountState struct {
	OriginID              HistoryOriginID
	AccountID             AccountID
	ArchivedAt            *time.Time
	IncludeInNetWorth     bool
	IncludeInInvestment   bool
	IncludeInLiquidAssets bool
	CreatedAt             time.Time
}

type HistoryOriginOwnership struct {
	OriginID  HistoryOriginID
	AccountID AccountID
	MemberID  MemberID
	ShareBPS  int
}

type HistoryOriginInstrumentPreference struct {
	OriginID     HistoryOriginID
	InstrumentID InstrumentID
	SourceKind   QuoteSourceKind
	CreatedAt    time.Time
}

type HistoryOriginFXPreference struct {
	OriginID   HistoryOriginID
	CurrencyA  CurrencyCode
	CurrencyB  CurrencyCode
	SourceKind QuoteSourceKind
	CreatedAt  time.Time
}

type HistoryOriginData struct {
	Origin                HistoryOrigin
	Components            []HistoryOriginComponent
	AccountStates         []HistoryOriginAccountState
	Ownership             []HistoryOriginOwnership
	InstrumentPreferences []HistoryOriginInstrumentPreference
	FXPreferences         []HistoryOriginFXPreference
}

// These observation values are append-only effective state/preferences. They
// let later reconstruction select the latest valid fact without mutating the
// original onboarding snapshot.
type AccountStateObservation struct {
	ID                    AccountStateObservationID
	AccountID             AccountID
	EffectiveAt           time.Time
	ArchivedAt            *time.Time
	IncludeInNetWorth     bool
	IncludeInInvestment   bool
	IncludeInLiquidAssets bool
	ActivityID            *ActivityID
	CreatedAt             time.Time
	Ownership             []OwnershipShare
}

type InstrumentPreferenceObservation struct {
	ID           InstrumentPreferenceObservationID
	InstrumentID InstrumentID
	SourceKind   QuoteSourceKind
	EffectiveAt  time.Time
	ActivityID   *ActivityID
	CreatedAt    time.Time
}

type FXPreferenceObservation struct {
	ID          FXPreferenceObservationID
	HouseholdID HouseholdID
	CurrencyA   CurrencyCode
	CurrencyB   CurrencyCode
	SourceKind  QuoteSourceKind
	EffectiveAt time.Time
	ActivityID  *ActivityID
	CreatedAt   time.Time
}

type InstrumentStateObservation struct {
	ID           InstrumentStateObservationID
	InstrumentID InstrumentID
	EffectiveAt  time.Time
	ArchivedAt   *time.Time
	ActivityID   *ActivityID
	CreatedAt    time.Time
}

type HoldingStateObservation struct {
	ID          HoldingStateObservationID
	HoldingID   HoldingID
	EffectiveAt time.Time
	ArchivedAt  *time.Time
	ActivityID  *ActivityID
	CreatedAt   time.Time
}

type DailyValuationSnapshot struct {
	ID                DailyValuationSnapshotID
	HouseholdID       HouseholdID
	LocalDate         string
	CutoffAt          time.Time
	Revision          int
	SupersedesID      *DailyValuationSnapshotID
	ContentHash       string
	AssetsAmount      *Money
	LiabilitiesAmount *Money
	NetWorthAmount    *Money
	Currency          CurrencyCode
	Complete          bool
	ComponentCount    int
	MissingCount      int
	GenerationReason  string
	CreatedAt         time.Time
	Items             []DailyValuationSnapshotItem
}

type DailyValuationSnapshotItem struct {
	ID                        DailyValuationSnapshotItemID
	SnapshotID                DailyValuationSnapshotID
	AccountID                 AccountID
	HoldingID                 *HoldingID
	InstrumentID              *InstrumentID
	NativeAmount              string
	NativeCurrency            CurrencyCode
	BaseAmount                *Money
	QuoteID                   *string
	FXQuoteID                 *string
	StateObservationID        *AccountStateObservationID
	PreferenceObservationID   *InstrumentPreferenceObservationID
	FXPreferenceObservationID *FXPreferenceObservationID
	Complete                  bool
	MissingReason             *string
}

// HistoricalSnapshotBatch is the immutable read input for one bounded
// historical rebuild. The application filters these candidates by each day's
// cutoff while the repository guarantees that all fields came from one read
// transaction.
type HistoricalSnapshotBatch struct {
	Origin                      HistoryOrigin
	OriginData                  HistoryOriginData
	Portfolio                   PortfolioSnapshot
	AccountStateObservations    []AccountStateObservation
	InstrumentStateObservations []InstrumentStateObservation
	HoldingStateObservations    []HoldingStateObservation
	InstrumentPreferenceFacts   []InstrumentPreferenceObservation
	FXPreferenceFacts           []FXPreferenceObservation
	FXPreferences               []FXPreference
	Activities                  []Activity
	InstrumentQuoteFacts        []InstrumentQuote
	FXQuoteFacts                []FXQuote
}

type DailySnapshotState struct {
	HouseholdID           HouseholdID
	DirtyFrom             *string
	LastCompletedClosedOn *string
}

type TrendRange string

const (
	Trend30Days  TrendRange = "30d"
	TrendOneYear TrendRange = "1y"
	TrendAllTime TrendRange = "all"
)

type NetWorthTrendPoint struct {
	LocalDate string
	Value     *Money
	Complete  bool
}

type NetWorthTrend struct {
	Range    TrendRange
	Currency CurrencyCode
	Points   []NetWorthTrendPoint
}
