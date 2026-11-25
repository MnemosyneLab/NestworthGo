package history

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// CashMovementLedger is the subset of the shared application service used by
// the cash movement form and its authoritative activity detail.
type CashMovementLedger interface {
	cashMovementReader
	ValidateReconciliationCommandLink(context.Context, *domain.ReconciliationLink, any, domain.ChangePreview) error
	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
}

type cashMovementReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
}

type CashMovementSources struct {
	Origin   domain.HistoryOrigin
	Location *time.Location
	Accounts []domain.Account
}

type CashMovement uint8

const (
	CashContribution CashMovement = iota
	CashWithdrawal
	CashTransfer
	CashConversion
)

func CanTransferTo(from, to domain.Account) bool {
	return from.ID != "" && to.ID != "" && from.ID != to.ID
}

func acceptsHoldingsCash(account domain.Account) bool {
	return account.ArchivedAt == nil && account.BalanceSheetRole == domain.RoleAsset && account.TrackingMode == domain.TrackingHoldings
}

func LoadCashMovementSources(ctx context.Context, ledger cashMovementReader, movement CashMovement) (CashMovementSources, error) {
	return loadCashMovementSources(ctx, ledger, movement, acceptsCashMovement)
}

func LoadLinkedCashTransferSources(ctx context.Context, ledger cashMovementReader) (CashMovementSources, error) {
	return loadCashMovementSources(ctx, ledger, CashTransfer, acceptsLinkedCashTransfer)
}

func loadCashMovementSources(ctx context.Context, ledger cashMovementReader, movement CashMovement, accepts func(domain.AccountRecord) bool) (CashMovementSources, error) {
	origin, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return CashMovementSources{}, err
	}
	rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return CashMovementSources{}, err
	}
	sources := CashMovementSources{Origin: origin, Location: location}
	for _, row := range rows {
		if accepts(row) && (movement != CashConversion || acceptsHoldingsCash(row.Account)) {
			sources.Accounts = append(sources.Accounts, row.Account)
		}
	}
	return sources, nil
}

func cashMovementOrigin(ctx context.Context, ledger cashMovementReader) (domain.HistoryOrigin, *time.Location, error) {
	origin, err := ledger.HistoryOrigin(ctx)
	if err != nil {
		return domain.HistoryOrigin{}, nil, err
	}
	if origin == nil {
		return domain.HistoryOrigin{}, nil, &domain.Error{Code: domain.ErrHistoryNotStarted}
	}
	location, err := time.LoadLocation(origin.Timezone)
	return *origin, location, err
}

func acceptsCashMovement(row domain.AccountRecord) bool {
	return row.Account.ArchivedAt == nil && supportsCashMovement(row)
}

func acceptsLinkedCashTransfer(row domain.AccountRecord) bool {
	// The Wails checkpoint form offers active non-liability endpoints,
	// including manual-value accounts. The shared preview validates their balance.
	return row.Account.ArchivedAt == nil && !row.Account.IsLiability()
}

func supportsCashMovement(row domain.AccountRecord) bool {
	a := row.Account
	if a.BalanceSheetRole == domain.RoleAsset && a.TrackingMode == domain.TrackingHoldings {
		return true
	}
	return row.LatestValue != nil && a.BalanceSheetRole == domain.RoleAsset && a.TrackingMode == domain.TrackingBalance &&
		(a.AccountType == domain.TypeCashOnHand || a.AccountType == domain.TypeBankAccount || a.AccountType == domain.TypeDigitalWallet)
}

type CashMovementInput struct {
	Movement    CashMovement
	Account     domain.AccountID
	Destination domain.AccountID
	Currency    domain.CurrencyCode
	ToCurrency  domain.CurrencyCode
	Amount      string
	Received    string
	Fee         string
	Date        string
	Time        string
	EffectiveAt time.Time
	Reason      domain.ActivityReason
	Note        string
	Link        *domain.ReconciliationLink
}

type ReviewedCashMovement struct {
	Preview  domain.ChangePreview
	Timezone *time.Location
	command  any
	mutation string
	hash     string
	token    string
	link     *domain.ReconciliationLink
}

// ReceiptIdentity returns the original mutation identity of the frozen command.
func (r ReviewedCashMovement) ReceiptIdentity() (mutationID, payloadHash string) {
	return r.mutation, r.hash
}

// ReviewCashMovement freezes a Wails History request and its mutation
// envelope. Accounts and Origin are read under the same shared preview gate
// that establishes the state token used at confirmation.
func ReviewCashMovement(ctx context.Context, ledger CashMovementLedger, input CashMovementInput) (ReviewedCashMovement, error) {
	var review ReviewedCashMovement
	preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		origin, location, err := cashMovementOrigin(ctx, ledger)
		if err != nil {
			return nil, err
		}
		accepts := acceptsCashMovement
		if input.Link != nil && input.Movement == CashTransfer {
			accepts = acceptsLinkedCashTransfer
		}
		account, err := cashMovementAccountMatching(ctx, ledger, input.Account, "accountId", accepts)
		if err != nil {
			return nil, err
		}
		if input.EffectiveAt.IsZero() && (input.Date == "" || input.Time == "") {
			return nil, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
		}
		var destination domain.Account
		if input.Movement == CashTransfer {
			destination, err = cashMovementAccountMatching(ctx, ledger, input.Destination, "toAccountId", accepts)
			if err != nil {
				return nil, err
			}
		}
		request, err := cashMovementRequest(input, account, destination, false)
		if err != nil {
			return nil, err
		}
		request.MutationID = domain.NewMutationID().String()
		review.command, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
		if err != nil {
			return nil, err
		}
		review.mutation, review.hash, err = request.MutationEnvelope()
		review.Timezone = location
		if request.ReconciliationLink != nil {
			link := *request.ReconciliationLink
			review.link = &link
		}
		return review.command, err
	})
	if err != nil {
		return ReviewedCashMovement{}, err
	}
	review.Preview, review.token = preview, token
	if review.link != nil {
		if err := ledger.ValidateReconciliationCommandLink(ctx, review.link, review.command, preview); err != nil {
			return ReviewedCashMovement{}, err
		}
	}
	return review, nil
}

func cashMovementAccount(ctx context.Context, ledger CashMovementLedger, id domain.AccountID, field string) (domain.Account, error) {
	return cashMovementAccountMatching(ctx, ledger, id, field, acceptsCashMovement)
}

func cashMovementAccountMatching(ctx context.Context, ledger CashMovementLedger, id domain.AccountID, field string, accepts func(domain.AccountRecord) bool) (domain.Account, error) {
	if id != "" {
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
		if err != nil {
			return domain.Account{}, err
		}
		if len(rows) == 1 && rows[0].Account.ID == id && accepts(rows[0]) {
			return rows[0].Account, nil
		}
	}
	return domain.Account{}, &domain.Error{Code: domain.ErrValidation, Field: field}
}

func cashMovementRequest(input CashMovementInput, account, destination domain.Account, allowZeroFee bool) (historyapi.ChangeCommandRequest, error) {
	if input.Movement == CashConversion && account.TrackingMode != domain.TrackingHoldings {
		return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "accountId"}
	}
	currency, err := movementCurrency(account, input.Currency, "currency")
	if err != nil {
		return historyapi.ChangeCommandRequest{}, err
	}
	amount, err := domain.ParseMoney(strings.TrimSpace(input.Amount), currency)
	if err != nil || amount.IsZero() {
		return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
	}
	request := historyapi.ChangeCommandRequest{ReconciliationLink: input.Link}
	if input.EffectiveAt.IsZero() {
		request.EffectiveLocalDate, request.EffectiveLocalTime = input.Date, input.Time
	} else {
		request.EffectiveAt = input.EffectiveAt.UTC().Format(time.RFC3339Nano)
	}
	if note := strings.TrimSpace(input.Note); note != "" {
		request.Note = &note
	}
	switch input.Movement {
	case CashContribution, CashWithdrawal:
		request.Kind, request.Reason = historyapi.ChangeMoneyAdded, string(domain.ReasonContribution)
		if input.Movement == CashWithdrawal {
			// Wails has no dedicated withdrawal reason. Other records a
			// general external outflow without labeling it as an expense.
			request.Kind, request.Reason = historyapi.ChangeMoneyRemoved, string(domain.ReasonOther)
		}
		if input.Reason != "" {
			request.Reason = string(input.Reason)
		}
		request.AccountID, request.Amount, request.Currency = account.ID.String(), amount.CanonicalAmount(), currency.String()
	case CashTransfer, CashConversion:
		if input.Movement == CashConversion {
			destination = account
		} else if !CanTransferTo(account, destination) {
			return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "toAccountId"}
		}
		targetCurrency, err := movementCurrency(destination, input.ToCurrency, "toCurrency")
		if err != nil {
			return historyapi.ChangeCommandRequest{}, err
		}
		if input.Movement == CashConversion && targetCurrency == currency {
			return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "toCurrency"}
		}
		received := amount
		if targetCurrency != currency {
			received, err = domain.ParseMoney(strings.TrimSpace(input.Received), targetCurrency)
			if err != nil || received.IsZero() {
				return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "received"}
			}
		}
		if input.Movement == CashTransfer {
			request.Kind = historyapi.ChangeCashTransfer
			request.FromAccountID, request.ToAccountID = account.ID.String(), destination.ID.String()
			request.Sent, request.Received = amount.CanonicalAmount(), received.CanonicalAmount()
			request.SentCurrency, request.ReceivedCurrency = currency.String(), targetCurrency.String()
		} else {
			request.Kind, request.AccountID = historyapi.ChangeFXConversion, account.ID.String()
			request.Sold, request.Bought = amount.CanonicalAmount(), received.CanonicalAmount()
			request.SoldCurrency, request.BoughtCurrency = currency.String(), targetCurrency.String()
		}
		request.FeeCurrency = currency.String()
		if raw := strings.TrimSpace(input.Fee); raw != "" {
			fee, err := domain.ParseMoney(raw, currency)
			if err != nil || (fee.IsZero() && !allowZeroFee) {
				return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "fee"}
			}
			request.Fee = fee.CanonicalAmount()
		}
	default:
		return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "kind"}
	}
	return request, nil
}

func movementCurrency(account domain.Account, selected domain.CurrencyCode, field string) (domain.CurrencyCode, error) {
	if account.TrackingMode == domain.TrackingHoldings || selected != "" {
		code, err := domain.ParseSupportedCurrency(selected.String())
		if err != nil || (account.TrackingMode != domain.TrackingHoldings && code != account.DefaultCurrency) {
			return "", &domain.Error{Code: domain.ErrValidation, Field: field}
		}
		return code, nil
	}
	return account.DefaultCurrency, nil
}

// Commit and its retries use the frozen envelope. The guarded application
// path consults the durable mutation record before checking a stale token.
func (r ReviewedCashMovement) Commit(ctx context.Context, ledger CashMovementLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(domain.WithReconciliationLink(ctx, r.link), r.command, r.mutation, r.hash, r.token)
}

func CashMovementOf(kind domain.ActivityKind, reason domain.ActivityReason) (CashMovement, bool) {
	switch {
	case kind == domain.ActivityCashIn && reason == domain.ReasonContribution:
		return CashContribution, true
	case kind == domain.ActivityCashOut && reason == domain.ReasonOther:
		return CashWithdrawal, true
	case kind == domain.ActivityCashTransfer:
		return CashTransfer, true
	case kind == domain.ActivityFXConversion:
		return CashConversion, true
	default:
		return 0, false
	}
}
