package domain

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// History identities are separate types so an Activity ID cannot be used as
// an Account, Holding, or observation ID by accident.
type ActivityID string
type ActivityEffectID string
type ActivityCorrectionGroupID string
type HistoryOriginID string
type HistoryOriginComponentID string
type AccountStateObservationID string
type AccountOwnershipObservationID string
type InstrumentStateObservationID string
type HoldingStateObservationID string
type InstrumentPreferenceObservationID string
type FXPreferenceObservationID string
type HoldingQuantityValueID string
type DailyValuationSnapshotID string
type DailyValuationSnapshotItemID string

func NewActivityID() ActivityID             { return ActivityID(newID()) }
func NewActivityEffectID() ActivityEffectID { return ActivityEffectID(newID()) }
func NewActivityCorrectionGroupID() ActivityCorrectionGroupID {
	return ActivityCorrectionGroupID(newID())
}
func NewHistoryOriginID() HistoryOriginID                   { return HistoryOriginID(newID()) }
func NewHistoryOriginComponentID() HistoryOriginComponentID { return HistoryOriginComponentID(newID()) }
func NewAccountStateObservationID() AccountStateObservationID {
	return AccountStateObservationID(newID())
}
func NewAccountOwnershipObservationID() AccountOwnershipObservationID {
	return AccountOwnershipObservationID(newID())
}
func NewInstrumentStateObservationID() InstrumentStateObservationID {
	return InstrumentStateObservationID(newID())
}
func NewHoldingStateObservationID() HoldingStateObservationID {
	return HoldingStateObservationID(newID())
}
func NewInstrumentPreferenceObservationID() InstrumentPreferenceObservationID {
	return InstrumentPreferenceObservationID(newID())
}
func NewFXPreferenceObservationID() FXPreferenceObservationID {
	return FXPreferenceObservationID(newID())
}
func NewHoldingQuantityValueID() HoldingQuantityValueID     { return HoldingQuantityValueID(newID()) }
func NewDailyValuationSnapshotID() DailyValuationSnapshotID { return DailyValuationSnapshotID(newID()) }
func NewDailyValuationSnapshotItemID() DailyValuationSnapshotItemID {
	return DailyValuationSnapshotItemID(newID())
}

func (id ActivityID) String() string                        { return string(id) }
func (id ActivityEffectID) String() string                  { return string(id) }
func (id ActivityCorrectionGroupID) String() string         { return string(id) }
func (id HistoryOriginID) String() string                   { return string(id) }
func (id HistoryOriginComponentID) String() string          { return string(id) }
func (id AccountStateObservationID) String() string         { return string(id) }
func (id AccountOwnershipObservationID) String() string     { return string(id) }
func (id InstrumentStateObservationID) String() string      { return string(id) }
func (id HoldingStateObservationID) String() string         { return string(id) }
func (id InstrumentPreferenceObservationID) String() string { return string(id) }
func (id FXPreferenceObservationID) String() string         { return string(id) }
func (id HoldingQuantityValueID) String() string            { return string(id) }
func (id DailyValuationSnapshotID) String() string          { return string(id) }
func (id DailyValuationSnapshotItemID) String() string      { return string(id) }

func parseHistoryID[T ~string](value, field string) (T, error) {
	return parseID[T](value, field)
}

func ParseActivityID(value string) (ActivityID, error) {
	return parseHistoryID[ActivityID](value, "activityId")
}
func ParseActivityEffectID(value string) (ActivityEffectID, error) {
	return parseHistoryID[ActivityEffectID](value, "activityEffectId")
}
func ParseHistoryOriginID(value string) (HistoryOriginID, error) {
	return parseHistoryID[HistoryOriginID](value, "historyOriginId")
}
func ParseActivityCorrectionGroupID(value string) (ActivityCorrectionGroupID, error) {
	return parseHistoryID[ActivityCorrectionGroupID](value, "correctionGroupId")
}
func ParseHistoryOriginComponentID(value string) (HistoryOriginComponentID, error) {
	return parseHistoryID[HistoryOriginComponentID](value, "historyOriginComponentId")
}
func ParseAccountStateObservationID(value string) (AccountStateObservationID, error) {
	return parseHistoryID[AccountStateObservationID](value, "accountStateObservationId")
}
func ParseAccountOwnershipObservationID(value string) (AccountOwnershipObservationID, error) {
	return parseHistoryID[AccountOwnershipObservationID](value, "accountOwnershipObservationId")
}
func ParseInstrumentStateObservationID(value string) (InstrumentStateObservationID, error) {
	return parseHistoryID[InstrumentStateObservationID](value, "instrumentStateObservationId")
}
func ParseHoldingStateObservationID(value string) (HoldingStateObservationID, error) {
	return parseHistoryID[HoldingStateObservationID](value, "holdingStateObservationId")
}
func ParseInstrumentPreferenceObservationID(value string) (InstrumentPreferenceObservationID, error) {
	return parseHistoryID[InstrumentPreferenceObservationID](value, "instrumentPreferenceObservationId")
}
func ParseFXPreferenceObservationID(value string) (FXPreferenceObservationID, error) {
	return parseHistoryID[FXPreferenceObservationID](value, "fxPreferenceObservationId")
}
func ParseHoldingQuantityValueID(value string) (HoldingQuantityValueID, error) {
	return parseHistoryID[HoldingQuantityValueID](value, "holdingQuantityValueId")
}
func ParseDailyValuationSnapshotID(value string) (DailyValuationSnapshotID, error) {
	return parseHistoryID[DailyValuationSnapshotID](value, "dailyValuationSnapshotId")
}
func ParseDailyValuationSnapshotItemID(value string) (DailyValuationSnapshotItemID, error) {
	return parseHistoryID[DailyValuationSnapshotItemID](value, "dailyValuationSnapshotItemId")
}

type ActivityKind string

const (
	ActivityCashIn           ActivityKind = "cash_in"
	ActivityCashOut          ActivityKind = "cash_out"
	ActivityCashDividend     ActivityKind = "cash_dividend"
	ActivityCashTransfer     ActivityKind = "cash_transfer"
	ActivityFXConversion     ActivityKind = "fx_conversion"
	ActivityPositionTransfer ActivityKind = "position_transfer"
	ActivityBuy              ActivityKind = "buy"
	ActivitySell             ActivityKind = "sell"
	ActivityValueUpdate      ActivityKind = "value_update"
	ActivityDebtDraw         ActivityKind = "debt_draw"
	ActivityDebtPayment      ActivityKind = "debt_payment"
	ActivityReversal         ActivityKind = "reversal"
)

func (k ActivityKind) String() string { return string(k) }

func ParseActivityKind(value string) (ActivityKind, error) {
	kind := ActivityKind(strings.TrimSpace(value))
	switch kind {
	case ActivityCashIn, ActivityCashOut, ActivityCashDividend, ActivityCashTransfer, ActivityFXConversion,
		ActivityPositionTransfer, ActivityBuy, ActivitySell, ActivityValueUpdate,
		ActivityDebtDraw, ActivityDebtPayment, ActivityReversal:
		return kind, nil
	default:
		return "", validation("kind", "is not supported")
	}
}

func AllActivityKinds() []ActivityKind {
	return []ActivityKind{
		ActivityCashIn, ActivityCashOut, ActivityCashDividend, ActivityCashTransfer, ActivityFXConversion,
		ActivityPositionTransfer, ActivityBuy, ActivitySell, ActivityValueUpdate,
		ActivityDebtDraw, ActivityDebtPayment, ActivityReversal,
	}
}

type ActivityReason string

const (
	ReasonIncome         ActivityReason = "income"
	ReasonContribution   ActivityReason = "contribution"
	ReasonGift           ActivityReason = "gift"
	ReasonOther          ActivityReason = "other"
	ReasonExpense        ActivityReason = "expense"
	ReasonFee            ActivityReason = "fee"
	ReasonTax            ActivityReason = "tax"
	ReasonReconciliation ActivityReason = "reconciliation"
	ReasonPrincipal      ActivityReason = "principal"
	ReasonInterest       ActivityReason = "interest"
)

func ParseActivityReason(value string) (ActivityReason, error) {
	reason := ActivityReason(strings.TrimSpace(value))
	if reason == "" {
		return "", nil
	}
	switch reason {
	case ReasonIncome, ReasonContribution, ReasonGift, ReasonOther, ReasonExpense,
		ReasonFee, ReasonTax, ReasonReconciliation, ReasonPrincipal, ReasonInterest:
		return reason, nil
	default:
		return "", validation("reason", "is not supported")
	}
}

func MoneyInReasons() []ActivityReason {
	return []ActivityReason{ReasonIncome, ReasonContribution, ReasonGift, ReasonOther, ReasonReconciliation}
}

func MoneyOutReasons() []ActivityReason {
	return []ActivityReason{ReasonExpense, ReasonFee, ReasonTax, ReasonOther, ReasonReconciliation}
}

func ValueUpdateReasons() []ActivityReason {
	return []ActivityReason{ReasonReconciliation, ReasonOther}
}

type ActivityClassification string

const (
	ClassificationExternalInflow   ActivityClassification = "external_inflow"
	ClassificationExternalOutflow  ActivityClassification = "external_outflow"
	ClassificationIncome           ActivityClassification = "income"
	ClassificationFee              ActivityClassification = "fee"
	ClassificationInternalTransfer ActivityClassification = "internal_transfer"
	ClassificationTradePrincipal   ActivityClassification = "trade_principal"
	ClassificationDebtPrincipal    ActivityClassification = "debt_principal"
	ClassificationRemeasurement    ActivityClassification = "remeasurement"
)

type EffectTarget string

const (
	EffectTargetAccountValue    EffectTarget = "account_value"
	EffectTargetAccountCash     EffectTarget = "account_cash"
	EffectTargetHoldingQuantity EffectTarget = "holding_quantity"
)

type EffectDirection string

const (
	EffectAdded   EffectDirection = "added"
	EffectRemoved EffectDirection = "removed"
)

type EffectRole string

const (
	EffectRoleAmount       EffectRole = "amount"
	EffectRoleTransferFrom EffectRole = "transfer_from"
	EffectRoleTransferTo   EffectRole = "transfer_to"
	EffectRolePrincipal    EffectRole = "principal"
	EffectRoleFee          EffectRole = "fee"
	EffectRoleQuantity     EffectRole = "quantity"
	EffectRoleDebt         EffectRole = "debt"
)

// Activity is immutable business evidence. Effects are stored separately in
// persistence, but keeping them in this value makes Preview and tests easy to
// inspect without exposing a generic ledger representation.
type Activity struct {
	ID                 ActivityID
	HouseholdID        HouseholdID
	Kind               ActivityKind
	Reason             ActivityReason
	EffectiveAt        time.Time
	EffectiveLocalDate string
	CreatedAt          time.Time
	Note               *string
	ReversesActivityID *ActivityID
	CorrectionGroupID  *ActivityCorrectionGroupID
	TransactionFXRate  *FxRate
	TradeDetail        *TradeDetail
	DividendDetail     *DividendDetail
	Resulting          []EndpointView
	Effects            []ActivityEffect
}

type ActivityCursor struct {
	EffectiveAt time.Time
	CreatedAt   time.Time
	ID          ActivityID
}

type ActivityQuery struct {
	AccountID       *AccountID
	Kinds           []ActivityKind
	FromLocalDate   string
	ToLocalDate     string
	After           *ActivityCursor
	Limit           int
	ExcludeReversed bool
}

type ActivityPage struct {
	Activities []Activity
	Next       *ActivityCursor
	HasMore    bool
}

type ActivityEffect struct {
	ID             ActivityEffectID
	ActivityID     ActivityID
	Sequence       int
	Role           EffectRole
	Direction      EffectDirection
	Target         EffectTarget
	Classification ActivityClassification
	AccountID      *AccountID
	HoldingID      *HoldingID
	InstrumentID   *InstrumentID
	Money          *Money
	Quantity       *Quantity
	CostUnitPrice  *UnitPrice
}

func (e ActivityEffect) Magnitude() string {
	if e.Money != nil {
		return e.Money.CanonicalAmount()
	}
	if e.Quantity != nil {
		return e.Quantity.Canonical()
	}
	return "0"
}

func (e ActivityEffect) Validate() error {
	if e.Sequence < 1 || e.ActivityID == "" || e.ID == "" {
		return changeError(ErrInvalidChange, "effect", "effect identity and sequence are required")
	}
	if e.Direction != EffectAdded && e.Direction != EffectRemoved {
		return changeError(ErrInvalidChange, "effect", "effect direction is invalid")
	}
	if e.Money != nil && e.Quantity != nil {
		return changeError(ErrInvalidChange, "effect", "effect cannot contain both Money and Quantity")
	}
	if e.Money == nil && e.Quantity == nil {
		return changeError(ErrInvalidChange, "effect", "effect magnitude is required")
	}
	if (e.Money != nil && e.Money.IsZero()) || (e.Quantity != nil && e.Quantity.IsZero()) {
		return changeError(ErrInvalidChange, "effect", "effect magnitude must be greater than zero")
	}
	if e.CostUnitPrice != nil && e.Target != EffectTargetHoldingQuantity {
		return changeError(ErrInvalidChange, "effect", "cost unit price is only valid for Holding effects")
	}
	switch e.Target {
	case EffectTargetAccountValue, EffectTargetAccountCash:
		if e.AccountID == nil || e.HoldingID != nil || e.InstrumentID != nil || e.Quantity != nil || e.Money == nil {
			return changeError(ErrInvalidChange, "effect", "Account effects require one Account and Money")
		}
	case EffectTargetHoldingQuantity:
		if e.HoldingID == nil || e.InstrumentID == nil || e.AccountID != nil || e.Quantity == nil || e.Money != nil {
			return changeError(ErrInvalidChange, "effect", "Holding effects require one Holding, Instrument, and Quantity")
		}
	default:
		return changeError(ErrInvalidChange, "effect", "effect target is invalid")
	}
	return nil
}

type ChangeAccountState struct {
	ID        AccountID
	Name      string
	Currency  CurrencyCode
	Mode      TrackingMode
	Liability bool
	Archived  bool
	Current   Money
}

type ChangeHoldingState struct {
	ID                 HoldingID
	AccountID          AccountID
	InstrumentID       InstrumentID
	InstrumentName     string
	Currency           CurrencyCode
	Archived           bool
	Current            Quantity
	CostBasisAvailable bool
}

// ChangeState is the injected, read-only state used by PreviewChange. It is
// intentionally independent of SQLite and may be assembled from a snapshot.
type ChangeState struct {
	HouseholdID HouseholdID
	OriginAt    time.Time
	Timezone    string
	Now         time.Time
	Accounts    map[AccountID]ChangeAccountState
	Cash        map[AccountID]map[CurrencyCode]Money
	Holdings    map[HoldingID]ChangeHoldingState
	// Instruments is needed for the first-buy workflow, where the user has
	// selected an Instrument but no Holding exists yet.
	Instruments map[InstrumentID]Instrument
}

type EndpointView struct {
	Target    EffectTarget
	AccountID *AccountID
	HoldingID *HoldingID
	Name      string
	Amount    string
	Quantity  string
	Currency  CurrencyCode
}

type ChangePreview struct {
	Activity         Activity
	Effects          []ActivityEffect
	Resulting        []EndpointView
	DerivedRate      *FxRate
	DerivedUnitPrice *UnitPrice
}

// ActivityCommit is the unit used by the repository's atomic write path. A
// batch is useful for corrections, where the inverse and replacement are both
// evidence and must either be persisted together or not at all.
type ActivityCommit struct {
	Activity  Activity
	Effects   []ActivityEffect
	Resulting []EndpointView
}

type CashDividendInput struct {
	HouseholdID HouseholdID
	HoldingID   HoldingID
	Amount      Money
	EffectiveAt time.Time
	Note        *string
}

type MoneyAddedInput struct {
	HouseholdID HouseholdID
	AccountID   AccountID
	Amount      Money
	Reason      ActivityReason
	EffectiveAt time.Time
	Note        *string
}

type MoneyRemovedInput struct {
	HouseholdID HouseholdID
	AccountID   AccountID
	Amount      Money
	Reason      ActivityReason
	EffectiveAt time.Time
	Note        *string
}

type CashTransferInput struct {
	HouseholdID   HouseholdID
	FromAccountID AccountID
	ToAccountID   AccountID
	Sent          Money
	Received      Money
	Fee           *Money
	EffectiveAt   time.Time
	Note          *string
}

// FXConversionInput records an in-account currency exchange as one atomic
// activity. Sold and bought amounts are facts from the broker confirmation;
// the derived rate is evidence for analysis only.
type FXConversionInput struct {
	HouseholdID HouseholdID
	AccountID   AccountID
	Sold        Money
	Bought      Money
	Fee         *Money
	EffectiveAt time.Time
	Note        *string
}

type PositionTransferInput struct {
	HouseholdID   HouseholdID
	FromHoldingID HoldingID
	ToHoldingID   HoldingID
	Quantity      Quantity
	EffectiveAt   time.Time
	Note          *string
}

// PositionAdjustmentInput is used by the post-History creation workflow to
// reconcile a newly-created Holding atomically with its initial quantity.
// It deliberately uses the existing position effect storage shape while
// keeping the operation out of ordinary transfer forms.
type PositionAdjustmentInput struct {
	HouseholdID HouseholdID
	HoldingID   HoldingID
	Quantity    Quantity
	Added       bool
	UnitCost    *UnitPrice
	EffectiveAt time.Time
	Note        *string
}

type TradeSide string

const (
	TradeBuy  TradeSide = "buy"
	TradeSell TradeSide = "sell"
)

func ParseTradeSide(value string) (TradeSide, error) {
	side := TradeSide(strings.TrimSpace(value))
	switch side {
	case TradeBuy, TradeSell:
		return side, nil
	default:
		return "", validation("side", "is not supported")
	}
}

func AllTradeSides() []TradeSide {
	return []TradeSide{TradeBuy, TradeSell}
}

type TradeInput struct {
	HouseholdID         HouseholdID
	Side                TradeSide
	SettlementAccountID AccountID
	HoldingID           HoldingID
	InstrumentID        InstrumentID
	Quantity            Quantity
	Gross               Money
	Fee                 *Money
	EffectiveAt         time.Time
	Note                *string
}

// TradeDetail is the normalized, queryable trade payload stored beside the
// effect ledger. It is deliberately separate from ActivityEffect because the
// gross total, unit price, and optional fee describe one trade as a whole.
type TradeDetail struct {
	Side         TradeSide
	InstrumentID InstrumentID
	HoldingID    HoldingID
	Quantity     Quantity
	Gross        Money
	UnitPrice    UnitPrice
	Fee          *Money
}

// DividendDetail is the queryable cash-dividend payload stored beside the
// cash effect. The effect itself cannot carry Holding identity.
type DividendDetail struct {
	HoldingID    HoldingID
	InstrumentID InstrumentID
	Amount       Money
}

type ValueUpdateInput struct {
	HouseholdID HouseholdID
	AccountID   AccountID
	NewValue    Money
	Reason      ActivityReason
	EffectiveAt time.Time
	Note        *string
}

type DebtDrawInput struct {
	HouseholdID   HouseholdID
	DebtAccountID AccountID
	CashAccountID AccountID
	Principal     Money
	EffectiveAt   time.Time
	Note          *string
}

type DebtPaymentInput struct {
	HouseholdID   HouseholdID
	DebtAccountID AccountID
	CashAccountID AccountID
	Principal     Money
	InterestOrFee *Money
	EffectiveAt   time.Time
	Note          *string
}

func PreviewChange(state ChangeState, command any) (ChangePreview, error) {
	switch input := command.(type) {
	case MoneyAddedInput:
		return buildMoneyChange(state, input, true)
	case MoneyRemovedInput:
		return buildMoneyChange(state, input, false)
	case CashDividendInput:
		return buildCashDividend(state, input)
	case CashTransferInput:
		return buildCashTransfer(state, input)
	case FXConversionInput:
		return buildFXConversion(state, input)
	case PositionTransferInput:
		return buildPositionTransfer(state, input)
	case PositionAdjustmentInput:
		return buildPositionAdjustment(state, input)
	case TradeInput:
		return buildTrade(state, input)
	case ValueUpdateInput:
		return buildValueUpdate(state, input)
	case DebtDrawInput:
		return buildDebtDraw(state, input)
	case DebtPaymentInput:
		return buildDebtPayment(state, input)
	default:
		return ChangePreview{}, changeError(ErrInvalidChange, "command", "change type is not supported")
	}
}

// ApplyEffects applies already validated effects to an in-memory state and
// returns the resulting endpoint views. It is shared by undo/fix so those
// paths use the same exact decimal and negative-state rules as normal change
// previews.
func ApplyEffects(state ChangeState, effects []ActivityEffect) (ChangeState, []EndpointView, error) {
	state = cloneChangeState(state)
	views := make([]EndpointView, 0, len(effects))
	for _, effect := range effects {
		if err := effect.Validate(); err != nil {
			return ChangeState{}, nil, err
		}
		var view EndpointView
		if effect.Money != nil {
			if effect.AccountID == nil {
				return ChangeState{}, nil, changeError(ErrInvalidChange, "effect", "money effect requires an Account")
			}
			account, ok := state.Accounts[*effect.AccountID]
			if !ok {
				return ChangeState{}, nil, &Error{Code: ErrNotFound, Message: "Account was not found"}
			}
			if effect.Target != accountTarget(account) {
				return ChangeState{}, nil, changeError(ErrInvalidChange, "effect", "effect target does not match the Account")
			}
			var err error
			view, err = state.accountAmount(account, *effect.Money, effect.Direction)
			if err != nil {
				return ChangeState{}, nil, err
			}
			updated, err := ParseMoney(view.Amount, view.Currency)
			if err != nil {
				return ChangeState{}, nil, err
			}
			if effect.Target == EffectTargetAccountValue {
				account.Current = updated
				state.Accounts[account.ID] = account
			} else {
				if state.Cash[account.ID] == nil {
					state.Cash[account.ID] = make(map[CurrencyCode]Money)
				}
				state.Cash[account.ID][updated.Currency()] = updated
			}
		} else {
			if effect.HoldingID == nil || effect.Quantity == nil {
				return ChangeState{}, nil, changeError(ErrInvalidChange, "effect", "quantity effect is incomplete")
			}
			holding, ok := state.Holdings[*effect.HoldingID]
			if !ok {
				return ChangeState{}, nil, &Error{Code: ErrNotFound, Message: "Holding was not found"}
			}
			current := holding.Current.Decimal()
			if effect.Direction == EffectAdded {
				current = current.Add(effect.Quantity.Decimal())
			} else {
				current = current.Sub(effect.Quantity.Decimal())
				if current.IsNegative() {
					return ChangeState{}, nil, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "Holding does not have enough quantity"}
				}
			}
			updated, err := NewQuantity(current)
			if err != nil {
				return ChangeState{}, nil, err
			}
			holding.Current = updated
			state.Holdings[holding.ID] = holding
			holdingID := holding.ID
			view = EndpointView{Target: EffectTargetHoldingQuantity, HoldingID: &holdingID, Name: holding.InstrumentName, Quantity: updated.Canonical()}
		}
		views = upsertEndpointView(views, view)
	}
	return state, views, nil
}

func cloneChangeState(state ChangeState) ChangeState {
	clone := state
	clone.Accounts = make(map[AccountID]ChangeAccountState, len(state.Accounts))
	for id, value := range state.Accounts {
		clone.Accounts[id] = value
	}
	clone.Cash = make(map[AccountID]map[CurrencyCode]Money, len(state.Cash))
	for accountID, values := range state.Cash {
		clone.Cash[accountID] = make(map[CurrencyCode]Money, len(values))
		for currency, value := range values {
			clone.Cash[accountID][currency] = value
		}
	}
	clone.Holdings = make(map[HoldingID]ChangeHoldingState, len(state.Holdings))
	for id, value := range state.Holdings {
		clone.Holdings[id] = value
	}
	clone.Instruments = make(map[InstrumentID]Instrument, len(state.Instruments))
	for id, value := range state.Instruments {
		clone.Instruments[id] = value
	}
	return clone
}

func upsertEndpointView(views []EndpointView, candidate EndpointView) []EndpointView {
	for index, existing := range views {
		if existing.Target == candidate.Target && ((existing.AccountID != nil && candidate.AccountID != nil && *existing.AccountID == *candidate.AccountID && existing.Currency == candidate.Currency) || (existing.HoldingID != nil && candidate.HoldingID != nil && *existing.HoldingID == *candidate.HoldingID)) {
			views[index] = candidate
			return views
		}
	}
	return append(views, candidate)
}

func InverseChange(state ChangeState, original Activity, effects []ActivityEffect) (ChangePreview, error) {
	activity, err := state.newActivity(original.HouseholdID, ActivityReversal, ReasonOther, state.Now, nil)
	if err != nil {
		return ChangePreview{}, err
	}
	activity.ReversesActivityID = &original.ID
	inverse := make([]ActivityEffect, 0, len(effects))
	for index := len(effects) - 1; index >= 0; index-- {
		effect := effects[index]
		effect.ID = NewActivityEffectID()
		effect.ActivityID = activity.ID
		effect.Sequence = len(inverse) + 1
		if effect.Direction == EffectAdded {
			effect.Direction = EffectRemoved
		} else {
			effect.Direction = EffectAdded
		}
		inverse = append(inverse, effect)
	}
	_, resulting, err := ApplyEffects(state, inverse)
	if err != nil {
		return ChangePreview{}, err
	}
	activity.Effects = inverse
	return ChangePreview{Activity: activity, Effects: inverse, Resulting: resulting}, nil
}

func buildMoneyChange(state ChangeState, input any, added bool) (ChangePreview, error) {
	var household HouseholdID
	var accountID AccountID
	var amount Money
	var reason ActivityReason
	var effectiveAt time.Time
	var note *string
	switch value := input.(type) {
	case MoneyAddedInput:
		household, accountID, amount, reason, effectiveAt, note = value.HouseholdID, value.AccountID, value.Amount, value.Reason, value.EffectiveAt, value.Note
	case MoneyRemovedInput:
		household, accountID, amount, reason, effectiveAt, note = value.HouseholdID, value.AccountID, value.Amount, value.Reason, value.EffectiveAt, value.Note
	default:
		return ChangePreview{}, changeError(ErrInvalidChange, "command", "money change is not supported")
	}
	if reason == "" {
		reason = ReasonOther
	}
	if added {
		if reason != ReasonIncome && reason != ReasonContribution && reason != ReasonGift && reason != ReasonOther && reason != ReasonReconciliation {
			return ChangePreview{}, changeError(ErrInvalidChange, "reason", "reason is not allowed for Money added")
		}
	} else if reason != ReasonExpense && reason != ReasonFee && reason != ReasonTax && reason != ReasonOther && reason != ReasonReconciliation {
		return ChangePreview{}, changeError(ErrInvalidChange, "reason", "reason is not allowed for Money removed")
	}
	account, err := state.account(accountID, household)
	if err != nil {
		return ChangePreview{}, err
	}
	if amount.IsZero() {
		return ChangePreview{}, changeError(ErrInvalidChange, "amount", "must be greater than zero")
	}
	// Simple accounts stay locked to the Account default currency. Composite
	// (holdings) cash is a per-currency ledger: the default currency is only
	// the input/display context, not a write restriction.
	if account.Mode != TrackingHoldings && amount.Currency() != account.Currency {
		return ChangePreview{}, changeError(ErrInvalidChange, "amount", "currency must match the Account")
	}
	activity, err := state.newActivity(household, activityKind(added), reason, effectiveAt, note)
	if err != nil {
		return ChangePreview{}, err
	}
	classification := classifyMoney(reason, added)
	direction := EffectRemoved
	if added {
		direction = EffectAdded
	}
	effect := accountEffect(activity.ID, 1, EffectRoleAmount, direction, accountTarget(account), classification, account.ID, amount, nil)
	result, err := state.accountAmount(account, amount, direction)
	if err != nil {
		return ChangePreview{}, err
	}
	activity.Effects = []ActivityEffect{effect}
	return ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: []EndpointView{result}}, nil
}

func buildCashDividend(state ChangeState, input CashDividendInput) (ChangePreview, error) {
	holding, err := state.holding(input.HoldingID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	account, err := state.account(holding.AccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if account.Mode != TrackingHoldings {
		return ChangePreview{}, changeError(ErrInvalidChange, "holdingId", "cash dividends require a Holdings Account")
	}
	if input.Amount.IsZero() {
		return ChangePreview{}, changeError(ErrInvalidChange, "amount", "must be greater than zero")
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityCashDividend, ReasonIncome, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	effect := cashEffect(activity.ID, 1, EffectRoleAmount, EffectAdded, ClassificationIncome, account.ID, input.Amount)
	result, err := state.cash(account.ID, input.Amount, EffectAdded)
	if err != nil {
		return ChangePreview{}, err
	}
	activity.Effects = []ActivityEffect{effect}
	activity.DividendDetail = &DividendDetail{HoldingID: holding.ID, InstrumentID: holding.InstrumentID, Amount: input.Amount}
	return ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: []EndpointView{result}}, nil
}

func activityKind(added bool) ActivityKind {
	if added {
		return ActivityCashIn
	}
	return ActivityCashOut
}

func classifyMoney(reason ActivityReason, added bool) ActivityClassification {
	if reason == ReasonReconciliation {
		return ClassificationRemeasurement
	}
	if added && reason == ReasonIncome {
		return ClassificationIncome
	}
	if !added && (reason == ReasonFee || reason == ReasonTax) {
		return ClassificationFee
	}
	if added {
		return ClassificationExternalInflow
	}
	return ClassificationExternalOutflow
}

func (state ChangeState) account(id AccountID, household HouseholdID) (ChangeAccountState, error) {
	if household != state.HouseholdID {
		return ChangeAccountState{}, changeError(ErrInvalidChange, "householdId", "Household does not match")
	}
	account, ok := state.Accounts[id]
	if !ok {
		return ChangeAccountState{}, &Error{Code: ErrNotFound, Message: "Account was not found"}
	}
	if account.Archived {
		return ChangeAccountState{}, &Error{Code: ErrConflict, Field: "accountId", Message: "Account is archived"}
	}
	if account.Current.Currency() != account.Currency {
		return ChangeAccountState{}, &Error{Code: ErrIntegrity, Message: "Account state currency is invalid"}
	}
	return account, nil
}

func (state ChangeState) holding(id HoldingID, household HouseholdID) (ChangeHoldingState, error) {
	if household != state.HouseholdID {
		return ChangeHoldingState{}, changeError(ErrInvalidChange, "householdId", "Household does not match")
	}
	holding, ok := state.Holdings[id]
	if !ok {
		return ChangeHoldingState{}, &Error{Code: ErrNotFound, Message: "Holding was not found"}
	}
	if holding.Archived {
		return ChangeHoldingState{}, &Error{Code: ErrConflict, Field: "holdingId", Message: "Holding is archived"}
	}
	return holding, nil
}

func (state ChangeState) newActivity(household HouseholdID, kind ActivityKind, reason ActivityReason, effectiveAt time.Time, note *string) (Activity, error) {
	if household != state.HouseholdID {
		return Activity{}, changeError(ErrInvalidChange, "householdId", "Household does not match")
	}
	now := state.Now
	// Every production constructor sets Now explicitly (Service.now defaults to
	// time.Now), so a zero clock is a programming error rather than a case for
	// an implicit fallback.
	if now.IsZero() {
		return Activity{}, changeError(ErrInvalidChange, "now", "change state requires an explicit current time")
	}
	if effectiveAt.IsZero() {
		effectiveAt = now
	}
	timezone := strings.TrimSpace(state.Timezone)
	// Whitespace-only and empty timezones are treated identically so UI input
	// cannot slip past the requirement by containing spaces.
	if timezone == "" {
		return Activity{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "confirm a Household timezone before recording a change"}
	}
	zone, err := time.LoadLocation(timezone)
	if err != nil {
		return Activity{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "confirm a Household timezone before recording a change"}
	}
	effectiveAt = normalizeTime(effectiveAt)
	now = normalizeTime(now)
	if effectiveAt.After(now) {
		return Activity{}, &Error{Code: ErrInvalidChangeTime, Field: "effectiveAt", Message: "change time cannot be in the future"}
	}
	if !state.OriginAt.IsZero() && effectiveAt.Before(normalizeTime(state.OriginAt)) {
		return Activity{}, &Error{Code: ErrInvalidChangeTime, Field: "effectiveAt", Message: "change time cannot precede the Starting point"}
	}
	cleanNote, err := validateNote("note", note)
	if err != nil {
		return Activity{}, err
	}
	return Activity{ID: NewActivityID(), HouseholdID: household, Kind: kind, Reason: reason, EffectiveAt: effectiveAt, EffectiveLocalDate: effectiveAt.In(zone).Format(dateLayout), CreatedAt: now, Note: cleanNote}, nil
}

func (state ChangeState) applyMoney(account ChangeAccountState, amount Money, direction EffectDirection) (EndpointView, error) {
	if amount.Currency() != account.Currency {
		return EndpointView{}, &Error{Code: ErrTransferMismatch, Field: "amount", Message: "amount currency must match the Account currency"}
	}
	current := account.Current.Amount()
	if direction == EffectAdded {
		current = current.Add(amount.Amount())
	} else {
		current = current.Sub(amount.Amount())
		if current.IsNegative() {
			return EndpointView{}, &Error{Code: ErrInsufficientBalance, Field: "amount", Message: fmt.Sprintf("%s does not have enough balance", account.Name)}
		}
	}
	updated, err := NewMoney(current, account.Currency)
	if err != nil {
		return EndpointView{}, err
	}
	id := account.ID
	return EndpointView{Target: EffectTargetAccountValue, AccountID: &id, Name: account.Name, Amount: updated.CanonicalAmount(), Currency: updated.Currency()}, nil
}

func (state ChangeState) accountAmount(account ChangeAccountState, amount Money, direction EffectDirection) (EndpointView, error) {
	if account.Mode == TrackingHoldings {
		return state.cash(account.ID, amount, direction)
	}
	return state.applyMoney(account, amount, direction)
}

func accountTarget(account ChangeAccountState) EffectTarget {
	if account.Mode == TrackingHoldings {
		return EffectTargetAccountCash
	}
	return EffectTargetAccountValue
}

func accountEffect(activityID ActivityID, sequence int, role EffectRole, direction EffectDirection, target EffectTarget, classification ActivityClassification, accountID AccountID, amount Money, quantity *Quantity) ActivityEffect {
	return ActivityEffect{ID: NewActivityEffectID(), ActivityID: activityID, Sequence: sequence, Role: role, Direction: direction, Target: target, Classification: classification, AccountID: &accountID, Money: &amount, Quantity: quantity}
}

func cashEffect(activityID ActivityID, sequence int, role EffectRole, direction EffectDirection, classification ActivityClassification, accountID AccountID, amount Money) ActivityEffect {
	return ActivityEffect{ID: NewActivityEffectID(), ActivityID: activityID, Sequence: sequence, Role: role, Direction: direction, Target: EffectTargetAccountCash, Classification: classification, AccountID: &accountID, Money: &amount}
}

func holdingEffect(activityID ActivityID, sequence int, role EffectRole, direction EffectDirection, classification ActivityClassification, holding ChangeHoldingState, quantity Quantity) ActivityEffect {
	holdingID, instrumentID := holding.ID, holding.InstrumentID
	return ActivityEffect{ID: NewActivityEffectID(), ActivityID: activityID, Sequence: sequence, Role: role, Direction: direction, Target: EffectTargetHoldingQuantity, Classification: classification, HoldingID: &holdingID, InstrumentID: &instrumentID, Quantity: &quantity}
}

func (state ChangeState) cash(accountID AccountID, amount Money, direction EffectDirection) (EndpointView, error) {
	account, ok := state.Accounts[accountID]
	if !ok {
		return EndpointView{}, &Error{Code: ErrNotFound, Message: "Account was not found"}
	}
	byCurrency := state.Cash[accountID]
	current := decimal.Zero
	if existing, exists := byCurrency[amount.Currency()]; exists {
		current = existing.Amount()
	}
	if direction == EffectAdded {
		current = current.Add(amount.Amount())
	} else {
		current = current.Sub(amount.Amount())
		if current.IsNegative() {
			return EndpointView{}, &Error{Code: ErrInsufficientBalance, Field: "amount", Message: fmt.Sprintf("%s does not have enough cash", account.Name)}
		}
	}
	updated, err := NewMoney(current, amount.Currency())
	if err != nil {
		return EndpointView{}, err
	}
	id := accountID
	return EndpointView{Target: EffectTargetAccountCash, AccountID: &id, Name: account.Name, Amount: updated.CanonicalAmount(), Currency: updated.Currency()}, nil
}

func buildCashTransfer(state ChangeState, input CashTransferInput) (ChangePreview, error) {
	from, err := state.account(input.FromAccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	to, err := state.account(input.ToAccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if input.Sent.IsZero() || input.Received.IsZero() {
		return ChangePreview{}, changeError(ErrTransferMismatch, "amount", "sent and received amounts must be greater than zero")
	}
	if input.FromAccountID == input.ToAccountID {
		return ChangePreview{}, changeError(ErrTransferMismatch, "accountId", "source and destination Accounts must differ")
	}
	if input.Sent.Currency() == input.Received.Currency() && !input.Sent.Amount().Equal(input.Received.Amount()) {
		return ChangePreview{}, changeError(ErrTransferMismatch, "received", "same-currency transfers must conserve their amount")
	}
	if input.Fee != nil {
		if input.Fee.IsZero() {
			return ChangePreview{}, changeError(ErrInvalidChange, "fee", "fee must be greater than zero")
		}
		if input.Fee.Currency() != input.Sent.Currency() {
			return ChangePreview{}, changeError(ErrInvalidChange, "fee", "fee currency must match sent currency")
		}
	}
	rate, err := NewFxRate(input.Received.Amount().Div(input.Sent.Amount()).Round(12))
	if err != nil {
		return ChangePreview{}, err
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityCashTransfer, ReasonOther, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	totalSent := input.Sent
	if input.Fee != nil {
		totalSent, err = totalSent.Add(*input.Fee)
		if err != nil {
			return ChangePreview{}, err
		}
	}
	fromView, err := state.accountAmount(from, totalSent, EffectRemoved)
	if err != nil {
		return ChangePreview{}, err
	}
	toView, err := state.accountAmount(to, input.Received, EffectAdded)
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{accountEffect(activity.ID, 1, EffectRoleTransferFrom, EffectRemoved, accountTarget(from), ClassificationInternalTransfer, from.ID, input.Sent, nil), accountEffect(activity.ID, 2, EffectRoleTransferTo, EffectAdded, accountTarget(to), ClassificationInternalTransfer, to.ID, input.Received, nil)}
	if input.Fee != nil {
		effects = append(effects, accountEffect(activity.ID, 3, EffectRoleFee, EffectRemoved, accountTarget(from), ClassificationFee, from.ID, *input.Fee, nil))
	}
	activity.TransactionFXRate = &rate
	activity.Effects = effects
	return ChangePreview{Activity: activity, Effects: effects, Resulting: []EndpointView{fromView, toView}, DerivedRate: &rate}, nil
}

func buildFXConversion(state ChangeState, input FXConversionInput) (ChangePreview, error) {
	account, err := state.account(input.AccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if account.Mode != TrackingHoldings {
		return ChangePreview{}, changeError(ErrInvalidChange, "accountId", "FX conversion requires a multi-currency cash account")
	}
	if input.Sold.IsZero() || input.Bought.IsZero() {
		return ChangePreview{}, changeError(ErrInvalidChange, "amount", "sold and bought amounts must be greater than zero")
	}
	if input.Sold.Currency() == input.Bought.Currency() {
		return ChangePreview{}, changeError(ErrInvalidChange, "currency", "sold and bought currencies must differ")
	}
	if input.Fee != nil {
		if input.Fee.IsZero() {
			return ChangePreview{}, changeError(ErrInvalidChange, "fee", "fee must be greater than zero")
		}
		if input.Fee.Currency() != input.Sold.Currency() {
			return ChangePreview{}, changeError(ErrInvalidChange, "fee", "fee currency must match sold currency")
		}
	}
	totalSold := input.Sold
	if input.Fee != nil {
		totalSold, err = totalSold.Add(*input.Fee)
		if err != nil {
			return ChangePreview{}, err
		}
	}
	fromView, err := state.accountAmount(account, totalSold, EffectRemoved)
	if err != nil {
		return ChangePreview{}, err
	}
	toView, err := state.accountAmount(account, input.Bought, EffectAdded)
	if err != nil {
		return ChangePreview{}, err
	}
	rate, err := NewFxRate(input.Bought.Amount().Div(input.Sold.Amount()).Round(12))
	if err != nil {
		return ChangePreview{}, err
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityFXConversion, ReasonOther, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{
		cashEffect(activity.ID, 1, EffectRoleTransferFrom, EffectRemoved, ClassificationInternalTransfer, account.ID, input.Sold),
		cashEffect(activity.ID, 2, EffectRoleTransferTo, EffectAdded, ClassificationInternalTransfer, account.ID, input.Bought),
	}
	if input.Fee != nil {
		effects = append(effects, cashEffect(activity.ID, 3, EffectRoleFee, EffectRemoved, ClassificationFee, account.ID, *input.Fee))
	}
	activity.TransactionFXRate = &rate
	activity.Effects = effects
	return ChangePreview{Activity: activity, Effects: effects, Resulting: []EndpointView{fromView, toView}, DerivedRate: &rate}, nil
}

func buildPositionTransfer(state ChangeState, input PositionTransferInput) (ChangePreview, error) {
	from, err := state.holding(input.FromHoldingID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	to, err := state.holding(input.ToHoldingID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if from.InstrumentID != to.InstrumentID {
		return ChangePreview{}, changeError(ErrTransferMismatch, "instrumentId", "position transfer requires the same Instrument")
	}
	if input.Quantity.IsZero() {
		return ChangePreview{}, changeError(ErrTransferMismatch, "quantity", "quantity must be greater than zero")
	}
	if from.Current.Decimal().LessThan(input.Quantity.Decimal()) {
		return ChangePreview{}, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "source Holding does not have enough quantity"}
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityPositionTransfer, ReasonOther, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	fromQty, err := NewQuantity(from.Current.Decimal().Sub(input.Quantity.Decimal()))
	if err != nil {
		return ChangePreview{}, err
	}
	toQty, err := NewQuantity(to.Current.Decimal().Add(input.Quantity.Decimal()))
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{holdingEffect(activity.ID, 1, EffectRoleTransferFrom, EffectRemoved, ClassificationInternalTransfer, from, input.Quantity), holdingEffect(activity.ID, 2, EffectRoleTransferTo, EffectAdded, ClassificationInternalTransfer, to, input.Quantity)}
	activity.Effects = effects
	fromID, toID := from.ID, to.ID
	return ChangePreview{Activity: activity, Effects: effects, Resulting: []EndpointView{{Target: EffectTargetHoldingQuantity, HoldingID: &fromID, Name: from.InstrumentName, Quantity: fromQty.Canonical()}, {Target: EffectTargetHoldingQuantity, HoldingID: &toID, Name: to.InstrumentName, Quantity: toQty.Canonical()}}}, nil
}

func buildPositionAdjustment(state ChangeState, input PositionAdjustmentInput) (ChangePreview, error) {
	holding, err := state.holding(input.HoldingID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if input.Quantity.IsZero() {
		return ChangePreview{}, changeError(ErrInvalidChange, "quantity", "quantity must be greater than zero")
	}
	if input.Added && input.UnitCost == nil && !holding.CostBasisAvailable {
		return ChangePreview{}, &Error{Code: ErrCostBasisRequired, Field: "unitCost", Message: "a per-unit cost is required when added quantity has no existing cost"}
	}
	direction := EffectAdded
	current := holding.Current.Decimal()
	if !input.Added {
		direction = EffectRemoved
		current = current.Sub(input.Quantity.Decimal())
		if current.IsNegative() {
			return ChangePreview{}, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "Holding does not have enough quantity"}
		}
	} else {
		current = current.Add(input.Quantity.Decimal())
	}
	quantity, err := NewQuantity(current)
	if err != nil {
		return ChangePreview{}, err
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityPositionTransfer, ReasonReconciliation, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	effect := holdingEffect(activity.ID, 1, EffectRoleQuantity, direction, ClassificationRemeasurement, holding, input.Quantity)
	if input.Added && input.UnitCost != nil {
		effect.CostUnitPrice = input.UnitCost
	}
	activity.Effects = []ActivityEffect{effect}
	holdingID := holding.ID
	preview := ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: []EndpointView{{Target: EffectTargetHoldingQuantity, HoldingID: &holdingID, Name: holding.InstrumentName, Quantity: quantity.Canonical()}}}
	if input.Added && input.UnitCost != nil {
		preview.DerivedUnitPrice = input.UnitCost
	}
	return preview, nil
}

func buildTrade(state ChangeState, input TradeInput) (ChangePreview, error) {
	if input.Side != TradeBuy && input.Side != TradeSell {
		return ChangePreview{}, &Error{Code: ErrInvalidTrade, Field: "side", Message: "trade side is not supported"}
	}
	account, err := state.account(input.SettlementAccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	holding, err := state.holding(input.HoldingID, input.HouseholdID)
	if err != nil && input.HoldingID == "" && input.HouseholdID == state.HouseholdID {
		for _, candidate := range state.Holdings {
			if !candidate.Archived && candidate.AccountID == account.ID && candidate.InstrumentID == input.InstrumentID {
				holding = candidate
				err = nil
				break
			}
		}
	}
	if err != nil {
		if input.HoldingID != "" || input.Side != TradeBuy {
			return ChangePreview{}, err
		}
		instrument, instrumentOK := state.Instruments[input.InstrumentID]
		if !instrumentOK {
			return ChangePreview{}, &Error{Code: ErrNotFound, Field: "instrumentId", Message: "Instrument was not found"}
		}
		if instrument.HouseholdID != state.HouseholdID {
			return ChangePreview{}, changeError(ErrInvalidChange, "instrumentId", "Instrument does not belong to the Household")
		}
		if instrument.ArchivedAt != nil {
			return ChangePreview{}, &Error{Code: ErrConflict, Field: "instrumentId", Message: "Instrument is archived"}
		}
		zero, zeroErr := NewQuantity(decimal.Zero)
		if zeroErr != nil {
			return ChangePreview{}, zeroErr
		}
		holding = ChangeHoldingState{ID: NewHoldingID(), AccountID: account.ID, InstrumentID: instrument.ID, InstrumentName: instrument.Name, Currency: instrument.QuoteCurrency, Current: zero, CostBasisAvailable: false}
	}
	if holding.AccountID != account.ID || holding.InstrumentID != input.InstrumentID || input.Quantity.IsZero() || input.Gross.IsZero() {
		return ChangePreview{}, &Error{Code: ErrInvalidTrade, Field: "trade", Message: "Holding, Instrument, quantity, and gross total must match"}
	}
	if account.Mode != TrackingHoldings {
		return ChangePreview{}, &Error{Code: ErrInvalidTrade, Field: "settlementAccountId", Message: "trades require a Holdings Account"}
	}
	if input.Gross.Currency() != holding.Currency {
		return ChangePreview{}, &Error{Code: ErrInvalidTrade, Field: "gross", Message: "settlement currency must match the Instrument quote currency"}
	}
	if input.Fee != nil && (input.Fee.Currency() != input.Gross.Currency() || input.Fee.Amount().IsNegative()) {
		return ChangePreview{}, &Error{Code: ErrInvalidTrade, Field: "fee", Message: "fee currency must match the settlement currency"}
	}
	unitPrice, err := UnitPriceFromExact(input.Gross.Amount().Div(input.Quantity.Decimal()))
	if err != nil {
		return ChangePreview{}, err
	}
	activityKind := ActivityBuy
	if input.Side == TradeSell {
		activityKind = ActivitySell
	}
	activity, err := state.newActivity(input.HouseholdID, activityKind, ReasonPrincipal, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	quantityDirection := EffectAdded
	cashDirection := EffectRemoved
	if input.Side == TradeSell {
		quantityDirection = EffectRemoved
		cashDirection = EffectAdded
		if holding.Current.Decimal().LessThan(input.Quantity.Decimal()) {
			return ChangePreview{}, &Error{Code: ErrInsufficientQuantity, Field: "quantity", Message: "Holding does not have enough quantity"}
		}
	}
	quantity := input.Quantity
	currentQuantity := holding.Current.Decimal()
	if quantityDirection == EffectAdded {
		currentQuantity = currentQuantity.Add(quantity.Decimal())
	} else {
		currentQuantity = currentQuantity.Sub(quantity.Decimal())
	}
	newQuantity, err := NewQuantity(currentQuantity)
	if err != nil {
		return ChangePreview{}, err
	}
	cashView, err := state.cash(account.ID, input.Gross, cashDirection)
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{holdingEffect(activity.ID, 1, EffectRoleQuantity, quantityDirection, ClassificationTradePrincipal, holding, quantity), cashEffect(activity.ID, 2, EffectRolePrincipal, cashDirection, ClassificationTradePrincipal, account.ID, input.Gross)}
	views := []EndpointView{cashView}
	holdingID := holding.ID
	views = append(views, EndpointView{Target: EffectTargetHoldingQuantity, HoldingID: &holdingID, Name: holding.InstrumentName, Quantity: newQuantity.Canonical()})
	if input.Fee != nil && !input.Fee.IsZero() {
		feeView, feeErr := state.cashAfter(account.ID, *input.Fee, EffectRemoved, input.Gross, cashDirection)
		if feeErr != nil {
			return ChangePreview{}, feeErr
		}
		effects = append(effects, cashEffect(activity.ID, 3, EffectRoleFee, EffectRemoved, ClassificationFee, account.ID, *input.Fee))
		views[0] = feeView
	}
	tradeDetail := &TradeDetail{Side: input.Side, InstrumentID: input.InstrumentID, HoldingID: holding.ID, Quantity: input.Quantity, Gross: input.Gross, UnitPrice: unitPrice, Fee: input.Fee}
	activity.TradeDetail = tradeDetail
	activity.Effects = effects
	return ChangePreview{Activity: activity, Effects: effects, Resulting: views, DerivedUnitPrice: &unitPrice}, nil
}

func (state ChangeState) cashAfter(accountID AccountID, amount Money, direction EffectDirection, first Money, firstDirection EffectDirection) (EndpointView, error) {
	firstView, err := state.cash(accountID, first, firstDirection)
	if err != nil {
		return EndpointView{}, err
	}
	current, parseErr := decimal.NewFromString(firstView.Amount)
	if parseErr != nil {
		return EndpointView{}, parseErr
	}
	if direction == EffectAdded {
		current = current.Add(amount.Amount())
	} else {
		current = current.Sub(amount.Amount())
	}
	if current.IsNegative() {
		return EndpointView{}, &Error{Code: ErrInsufficientBalance, Field: "fee", Message: "Account does not have enough cash for the trade and fee"}
	}
	updated, err := NewMoney(current, amount.Currency())
	if err != nil {
		return EndpointView{}, err
	}
	firstView.Amount, firstView.Currency = updated.CanonicalAmount(), updated.Currency()
	return firstView, nil
}

func buildValueUpdate(state ChangeState, input ValueUpdateInput) (ChangePreview, error) {
	account, err := state.account(input.AccountID, input.HouseholdID)
	if err != nil {
		return ChangePreview{}, err
	}
	if account.Mode == TrackingHoldings || input.NewValue.Currency() != account.Currency {
		return ChangePreview{}, changeError(ErrInvalidChange, "newValue", "value update requires a matching non-Holdings Account")
	}
	if input.Reason == "" {
		input.Reason = ReasonReconciliation
	}
	if input.Reason != ReasonReconciliation && input.Reason != ReasonOther {
		return ChangePreview{}, changeError(ErrInvalidChange, "reason", "reason is not allowed for a value update")
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityValueUpdate, input.Reason, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	delta := input.NewValue.Amount().Sub(account.Current.Amount())
	if delta.IsZero() {
		return ChangePreview{}, &Error{Code: ErrNoChange, Message: "the new value is unchanged"}
	}
	amount, err := NewMoney(delta.Abs(), account.Currency)
	if err != nil {
		return ChangePreview{}, err
	}
	direction := EffectAdded
	if delta.IsNegative() {
		direction = EffectRemoved
	}
	effect := accountEffect(activity.ID, 1, EffectRoleAmount, direction, accountTarget(account), ClassificationRemeasurement, account.ID, amount, nil)
	result, err := state.applyMoney(account, amount, direction)
	if err != nil {
		return ChangePreview{}, err
	}
	activity.Effects = []ActivityEffect{effect}
	return ChangePreview{Activity: activity, Effects: activity.Effects, Resulting: []EndpointView{result}}, nil
}

func buildDebtDraw(state ChangeState, input DebtDrawInput) (ChangePreview, error) {
	debt, cash, err := validateDebtEndpoints(state, input.HouseholdID, input.DebtAccountID, input.CashAccountID, input.Principal)
	if err != nil {
		return ChangePreview{}, err
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityDebtDraw, ReasonPrincipal, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	debtView, err := state.applyMoney(debt, input.Principal, EffectAdded)
	if err != nil {
		return ChangePreview{}, err
	}
	cashView, err := state.accountAmount(cash, input.Principal, EffectAdded)
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{accountEffect(activity.ID, 1, EffectRoleDebt, EffectAdded, EffectTargetAccountValue, ClassificationDebtPrincipal, debt.ID, input.Principal, nil), accountEffect(activity.ID, 2, EffectRolePrincipal, EffectAdded, accountTarget(cash), ClassificationDebtPrincipal, cash.ID, input.Principal, nil)}
	activity.Effects = effects
	return ChangePreview{Activity: activity, Effects: effects, Resulting: []EndpointView{debtView, cashView}}, nil
}

func buildDebtPayment(state ChangeState, input DebtPaymentInput) (ChangePreview, error) {
	debt, cash, err := validateDebtEndpoints(state, input.HouseholdID, input.DebtAccountID, input.CashAccountID, input.Principal)
	if err != nil {
		return ChangePreview{}, err
	}
	activity, err := state.newActivity(input.HouseholdID, ActivityDebtPayment, ReasonPrincipal, input.EffectiveAt, input.Note)
	if err != nil {
		return ChangePreview{}, err
	}
	debtView, err := state.applyMoney(debt, input.Principal, EffectRemoved)
	if err != nil {
		return ChangePreview{}, err
	}
	var cashView EndpointView
	if cash.Mode == TrackingHoldings {
		cashView, err = state.cash(cash.ID, input.Principal, EffectRemoved)
	} else {
		cashView, err = state.accountAmount(cash, input.Principal, EffectRemoved)
	}
	if err != nil {
		return ChangePreview{}, err
	}
	effects := []ActivityEffect{accountEffect(activity.ID, 1, EffectRoleDebt, EffectRemoved, EffectTargetAccountValue, ClassificationDebtPrincipal, debt.ID, input.Principal, nil), accountEffect(activity.ID, 2, EffectRolePrincipal, EffectRemoved, accountTarget(cash), ClassificationDebtPrincipal, cash.ID, input.Principal, nil)}
	if input.InterestOrFee != nil && !input.InterestOrFee.IsZero() {
		if input.InterestOrFee.Currency() != input.Principal.Currency() {
			return ChangePreview{}, changeError(ErrInvalidChange, "interestOrFee", "interest or fee currency must match principal")
		}
		feeView, feeErr := subtractFromEndpoint(cashView, *input.InterestOrFee)
		if feeErr != nil {
			return ChangePreview{}, feeErr
		}
		cashView = feeView
		effects = append(effects, accountEffect(activity.ID, 3, EffectRoleFee, EffectRemoved, accountTarget(cash), ClassificationFee, cash.ID, *input.InterestOrFee, nil))
	}
	activity.Effects = effects
	return ChangePreview{Activity: activity, Effects: effects, Resulting: []EndpointView{debtView, cashView}}, nil
}

// validateDebtEndpoints is shared by draw and payment so both directions use
// the same balance-sheet endpoint contract. A debt is always a liability;
// cash must be a distinct non-liability Balance or Holdings Account. The
// latter is represented by AccountCash effects rather than AccountValue
// effects, and the caller chooses the direction-specific arithmetic.
func validateDebtEndpoints(state ChangeState, householdID HouseholdID, debtID, cashID AccountID, principal Money) (ChangeAccountState, ChangeAccountState, error) {
	debt, err := state.account(debtID, householdID)
	if err != nil {
		return ChangeAccountState{}, ChangeAccountState{}, err
	}
	cash, err := state.account(cashID, householdID)
	if err != nil {
		return ChangeAccountState{}, ChangeAccountState{}, err
	}
	if debt.ID == cash.ID {
		return ChangeAccountState{}, ChangeAccountState{}, changeError(ErrInvalidChange, "cashAccountId", "debt and cash Accounts must differ")
	}
	if !debt.Liability {
		return ChangeAccountState{}, ChangeAccountState{}, changeError(ErrInvalidChange, "debtAccountId", "debt Account must be a liability")
	}
	if cash.Liability || (cash.Mode != TrackingBalance && cash.Mode != TrackingHoldings) {
		return ChangeAccountState{}, ChangeAccountState{}, changeError(ErrInvalidChange, "cashAccountId", "cash Account must be a non-liability Balance or Holdings Account")
	}
	if cash.Currency != principal.Currency() {
		return ChangeAccountState{}, ChangeAccountState{}, changeError(ErrTransferMismatch, "principal", "principal currency must match the cash Account")
	}
	if principal.IsZero() {
		return ChangeAccountState{}, ChangeAccountState{}, changeError(ErrInvalidChange, "principal", "principal must be greater than zero")
	}
	return debt, cash, nil
}

func subtractFromEndpoint(view EndpointView, amount Money) (EndpointView, error) {
	if amount.IsZero() || amount.Currency() != view.Currency {
		return EndpointView{}, changeError(ErrInvalidChange, "interestOrFee", "interest or fee currency must match principal")
	}
	current, err := decimal.NewFromString(view.Amount)
	if err != nil {
		return EndpointView{}, &Error{Code: ErrIntegrity, Message: "cash endpoint amount is invalid"}
	}
	current = current.Sub(amount.Amount())
	if current.IsNegative() {
		return EndpointView{}, &Error{Code: ErrInsufficientBalance, Field: "interestOrFee", Message: "cash Account does not have enough money for principal and fee"}
	}
	updated, err := NewMoney(current, view.Currency)
	if err != nil {
		return EndpointView{}, err
	}
	view.Amount = updated.CanonicalAmount()
	return view, nil
}

func changeError(code ErrorCode, field, message string) error {
	return &Error{Code: code, Field: field, Message: message}
}

// ResolveLocalDateTime converts a wall-clock date and time in an IANA zone to
// UTC. It refuses both DST gaps and repeated wall times instead of silently
// choosing Go's normalization.
func ResolveLocalDateTime(date, clock, timezone string) (time.Time, error) {
	timezoneName := strings.TrimSpace(timezone)
	if timezoneName == "" {
		return time.Time{}, &Error{Code: ErrHistoryTimezoneRequired, Field: "timezone", Message: "confirm a Household timezone before recording a change"}
	}
	location, err := time.LoadLocation(timezoneName)
	if err != nil {
		return time.Time{}, changeError(ErrInvalidChangeTime, "timezone", "timezone must be a valid IANA timezone")
	}
	parsedDate, err := time.ParseInLocation(dateLayout, strings.TrimSpace(date), location)
	if err != nil {
		return time.Time{}, changeError(ErrInvalidChangeTime, "date", "date must use YYYY-MM-DD")
	}
	parsedClock, err := time.Parse("15:04", strings.TrimSpace(clock))
	if err != nil {
		return time.Time{}, changeError(ErrInvalidChangeTime, "time", "time must use HH:MM")
	}
	wall := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), parsedClock.Hour(), parsedClock.Minute(), 0, 0, location)
	wallParts := func(value time.Time) string { return value.In(location).Format("2006-01-02 15:04") }
	wallText := strings.TrimSpace(date) + " " + strings.TrimSpace(clock)
	offsets := map[int]struct{}{}
	for delta := -36 * time.Hour; delta <= 36*time.Hour; delta += 15 * time.Minute {
		candidate := wall.Add(delta)
		_, offset := candidate.Zone()
		offsets[offset] = struct{}{}
	}
	candidates := make([]time.Time, 0, len(offsets))
	for offset := range offsets {
		candidate := time.Date(wall.Year(), wall.Month(), wall.Day(), wall.Hour(), wall.Minute(), 0, 0, time.UTC).Add(-time.Duration(offset) * time.Second)
		if wallParts(candidate) == wallText {
			candidates = append(candidates, candidate)
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	if len(candidates) == 0 {
		return time.Time{}, changeError(ErrInvalidChangeTime, "time", "local time does not exist in the selected timezone")
	}
	if len(candidates) > 1 {
		return time.Time{}, changeError(ErrInvalidChangeTime, "time", "local time is ambiguous in the selected timezone")
	}
	return normalizeTime(candidates[0]), nil
}

func ResolveLocalTime(date, clock, timezone string) (time.Time, error) {
	return ResolveLocalDateTime(date, clock, timezone)
}
