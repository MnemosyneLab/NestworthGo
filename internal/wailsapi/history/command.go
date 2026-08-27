package history

import (
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
)

// ChangeCommandKind discriminates ChangeCommandRequest, mirroring the ten
// concrete domain.PreviewChange input types. Wails v3's binding
// generator produces TypeScript models from Go struct declarations, not
// from a runtime union, so this is one Go struct with every variant's
// fields optional plus a discriminator, not a Go-level sum type.
type ChangeCommandKind string

const (
	ChangeMoneyAdded         ChangeCommandKind = "money_added"
	ChangeMoneyRemoved       ChangeCommandKind = "money_removed"
	ChangeCashTransfer       ChangeCommandKind = "cash_transfer"
	ChangeFXConversion       ChangeCommandKind = "fx_conversion"
	ChangePositionTransfer   ChangeCommandKind = "position_transfer"
	ChangePositionAdjustment ChangeCommandKind = "position_adjustment"
	ChangeTrade              ChangeCommandKind = "trade"
	ChangeValueUpdate        ChangeCommandKind = "value_update"
	ChangeDebtDraw           ChangeCommandKind = "debt_draw"
	ChangeDebtPayment        ChangeCommandKind = "debt_payment"
)

// ChangeCommandRequest is the tagged union the frontend submits for every
// change kind (Record change, Preview, Fix's replacement command). Money
// fields are always split into a canonical decimal string amount plus its
// own currency field; a field irrelevant to the current Kind is simply left
// empty.
type ChangeCommandRequest struct {
	Kind ChangeCommandKind `json:"kind"`

	AccountID           string `json:"accountId,omitempty"`
	FromAccountID       string `json:"fromAccountId,omitempty"`
	ToAccountID         string `json:"toAccountId,omitempty"`
	SettlementAccountID string `json:"settlementAccountId,omitempty"`
	DebtAccountID       string `json:"debtAccountId,omitempty"`
	CashAccountID       string `json:"cashAccountId,omitempty"`

	HoldingID     string `json:"holdingId,omitempty"`
	FromHoldingID string `json:"fromHoldingId,omitempty"`
	ToHoldingID   string `json:"toHoldingId,omitempty"`
	InstrumentID  string `json:"instrumentId,omitempty"`

	Side     string `json:"side,omitempty"`
	Quantity string `json:"quantity,omitempty"`
	Added    bool   `json:"added,omitempty"`
	UnitCost string `json:"unitCost,omitempty"`

	// Amount is used by MoneyAdded/MoneyRemoved together with Currency.
	Amount   string `json:"amount,omitempty"`
	Currency string `json:"currency,omitempty"`

	Sent             string `json:"sent,omitempty"`
	SentCurrency     string `json:"sentCurrency,omitempty"`
	Received         string `json:"received,omitempty"`
	ReceivedCurrency string `json:"receivedCurrency,omitempty"`

	Sold           string `json:"sold,omitempty"`
	SoldCurrency   string `json:"soldCurrency,omitempty"`
	Bought         string `json:"bought,omitempty"`
	BoughtCurrency string `json:"boughtCurrency,omitempty"`

	Gross         string `json:"gross,omitempty"`
	GrossCurrency string `json:"grossCurrency,omitempty"`
	Fee           string `json:"fee,omitempty"`
	FeeCurrency   string `json:"feeCurrency,omitempty"`

	NewValue         string `json:"newValue,omitempty"`
	NewValueCurrency string `json:"newValueCurrency,omitempty"`

	Principal             string `json:"principal,omitempty"`
	PrincipalCurrency     string `json:"principalCurrency,omitempty"`
	InterestOrFee         string `json:"interestOrFee,omitempty"`
	InterestOrFeeCurrency string `json:"interestOrFeeCurrency,omitempty"`

	Reason      string  `json:"reason,omitempty"`
	EffectiveAt string  `json:"effectiveAt,omitempty"`
	Note        *string `json:"note,omitempty"`
}

func parseTimeOrZero(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, &domain.Error{Code: domain.ErrValidation, Field: "effectiveAt", Message: "must be an RFC 3339 timestamp"}
	}
	return parsed, nil
}

func parseMoneyField(amount, currency string) (domain.Money, error) {
	code, err := domain.ParseSupportedCurrency(currency)
	if err != nil {
		return domain.Money{}, err
	}
	return domain.ParseMoney(amount, code)
}

func parseOptionalMoneyField(amount, currency string) (*domain.Money, error) {
	if amount == "" {
		return nil, nil
	}
	value, err := parseMoneyField(amount, currency)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

// ToCommand reconstructs the concrete domain.*Input type PreviewChange
// switches on. householdID comes from the caller's own Bootstrap/context,
// never from client-submitted input, so a malicious or stale request
// cannot target a different Household.
func (r ChangeCommandRequest) ToCommand(householdID domain.HouseholdID) (any, error) {
	effectiveAt, err := parseTimeOrZero(r.EffectiveAt)
	if err != nil {
		return nil, err
	}
	switch r.Kind {
	case ChangeMoneyAdded:
		accountID, err := domain.ParseAccountID(r.AccountID)
		if err != nil {
			return nil, err
		}
		amount, err := parseMoneyField(r.Amount, r.Currency)
		if err != nil {
			return nil, err
		}
		reason, err := domain.ParseActivityReason(r.Reason)
		if err != nil {
			return nil, err
		}
		return domain.MoneyAddedInput{HouseholdID: householdID, AccountID: accountID, Amount: amount, Reason: reason, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeMoneyRemoved:
		accountID, err := domain.ParseAccountID(r.AccountID)
		if err != nil {
			return nil, err
		}
		amount, err := parseMoneyField(r.Amount, r.Currency)
		if err != nil {
			return nil, err
		}
		reason, err := domain.ParseActivityReason(r.Reason)
		if err != nil {
			return nil, err
		}
		return domain.MoneyRemovedInput{HouseholdID: householdID, AccountID: accountID, Amount: amount, Reason: reason, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeCashTransfer:
		fromID, err := domain.ParseAccountID(r.FromAccountID)
		if err != nil {
			return nil, err
		}
		toID, err := domain.ParseAccountID(r.ToAccountID)
		if err != nil {
			return nil, err
		}
		sent, err := parseMoneyField(r.Sent, r.SentCurrency)
		if err != nil {
			return nil, err
		}
		received, err := parseMoneyField(r.Received, r.ReceivedCurrency)
		if err != nil {
			return nil, err
		}
		return domain.CashTransferInput{HouseholdID: householdID, FromAccountID: fromID, ToAccountID: toID, Sent: sent, Received: received, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeFXConversion:
		accountID, err := domain.ParseAccountID(r.AccountID)
		if err != nil {
			return nil, err
		}
		sold, err := parseMoneyField(r.Sold, r.SoldCurrency)
		if err != nil {
			return nil, err
		}
		bought, err := parseMoneyField(r.Bought, r.BoughtCurrency)
		if err != nil {
			return nil, err
		}
		fee, err := parseOptionalMoneyField(r.Fee, r.FeeCurrency)
		if err != nil {
			return nil, err
		}
		return domain.FXConversionInput{HouseholdID: householdID, AccountID: accountID, Sold: sold, Bought: bought, Fee: fee, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangePositionTransfer:
		fromHoldingID, err := domain.ParseHoldingID(r.FromHoldingID)
		if err != nil {
			return nil, err
		}
		toHoldingID, err := domain.ParseHoldingID(r.ToHoldingID)
		if err != nil {
			return nil, err
		}
		quantity, err := domain.ParseQuantity(r.Quantity)
		if err != nil {
			return nil, err
		}
		return domain.PositionTransferInput{HouseholdID: householdID, FromHoldingID: fromHoldingID, ToHoldingID: toHoldingID, Quantity: quantity, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangePositionAdjustment:
		holdingID, err := domain.ParseHoldingID(r.HoldingID)
		if err != nil {
			return nil, err
		}
		quantity, err := domain.ParseQuantity(r.Quantity)
		if err != nil {
			return nil, err
		}
		var unitCost *domain.UnitPrice
		if r.UnitCost != "" {
			parsed, err := domain.ParseUnitPrice(r.UnitCost)
			if err != nil {
				return nil, err
			}
			unitCost = &parsed
		}
		return domain.PositionAdjustmentInput{HouseholdID: householdID, HoldingID: holdingID, Quantity: quantity, Added: r.Added, UnitCost: unitCost, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeTrade:
		settlementAccountID, err := domain.ParseAccountID(r.SettlementAccountID)
		if err != nil {
			return nil, err
		}
		var holdingID domain.HoldingID
		if r.HoldingID != "" {
			holdingID, err = domain.ParseHoldingID(r.HoldingID)
			if err != nil {
				return nil, err
			}
		}
		instrumentID, err := domain.ParseInstrumentID(r.InstrumentID)
		if err != nil {
			return nil, err
		}
		quantity, err := domain.ParseQuantity(r.Quantity)
		if err != nil {
			return nil, err
		}
		gross, err := parseMoneyField(r.Gross, r.GrossCurrency)
		if err != nil {
			return nil, err
		}
		fee, err := parseOptionalMoneyField(r.Fee, r.FeeCurrency)
		if err != nil {
			return nil, err
		}
		side, err := domain.ParseTradeSide(r.Side)
		if err != nil {
			return nil, err
		}
		return domain.TradeInput{HouseholdID: householdID, Side: side, SettlementAccountID: settlementAccountID, HoldingID: holdingID, InstrumentID: instrumentID, Quantity: quantity, Gross: gross, Fee: fee, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeValueUpdate:
		accountID, err := domain.ParseAccountID(r.AccountID)
		if err != nil {
			return nil, err
		}
		newValue, err := parseMoneyField(r.NewValue, r.NewValueCurrency)
		if err != nil {
			return nil, err
		}
		reason, err := domain.ParseActivityReason(r.Reason)
		if err != nil {
			return nil, err
		}
		return domain.ValueUpdateInput{HouseholdID: householdID, AccountID: accountID, NewValue: newValue, Reason: reason, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeDebtDraw:
		debtID, err := domain.ParseAccountID(r.DebtAccountID)
		if err != nil {
			return nil, err
		}
		cashID, err := domain.ParseAccountID(r.CashAccountID)
		if err != nil {
			return nil, err
		}
		principal, err := parseMoneyField(r.Principal, r.PrincipalCurrency)
		if err != nil {
			return nil, err
		}
		return domain.DebtDrawInput{HouseholdID: householdID, DebtAccountID: debtID, CashAccountID: cashID, Principal: principal, EffectiveAt: effectiveAt, Note: r.Note}, nil

	case ChangeDebtPayment:
		debtID, err := domain.ParseAccountID(r.DebtAccountID)
		if err != nil {
			return nil, err
		}
		cashID, err := domain.ParseAccountID(r.CashAccountID)
		if err != nil {
			return nil, err
		}
		principal, err := parseMoneyField(r.Principal, r.PrincipalCurrency)
		if err != nil {
			return nil, err
		}
		interestOrFee, err := parseOptionalMoneyField(r.InterestOrFee, r.InterestOrFeeCurrency)
		if err != nil {
			return nil, err
		}
		return domain.DebtPaymentInput{HouseholdID: householdID, DebtAccountID: debtID, CashAccountID: cashID, Principal: principal, InterestOrFee: interestOrFee, EffectiveAt: effectiveAt, Note: r.Note}, nil

	default:
		return nil, &domain.Error{Code: domain.ErrInvalidChange, Field: "kind", Message: "change kind is not supported"}
	}
}
