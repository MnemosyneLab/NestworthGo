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
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
	Activity(context.Context, domain.ActivityID) (domain.Activity, error)
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
)

func CanTransferTo(from, to domain.Account) bool {
	return from.ID != "" && to.ID != "" && from.ID != to.ID
}

func LoadCashMovementSources(ctx context.Context, ledger CashMovementLedger) (CashMovementSources, error) {
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
		if acceptsCashMovement(row) {
			sources.Accounts = append(sources.Accounts, row.Account)
		}
	}
	return sources, nil
}

func cashMovementOrigin(ctx context.Context, ledger CashMovementLedger) (domain.HistoryOrigin, *time.Location, error) {
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
	a := row.Account
	return row.LatestValue != nil && a.ArchivedAt == nil && a.BalanceSheetRole == domain.RoleAsset && a.TrackingMode == domain.TrackingBalance &&
		(a.AccountType == domain.TypeCashOnHand || a.AccountType == domain.TypeBankAccount || a.AccountType == domain.TypeDigitalWallet)
}

type CashMovementInput struct {
	Movement    CashMovement
	Account     domain.AccountID
	Destination domain.AccountID
	Amount      string
	Received    string
	Fee         string
	Date        string
	Time        string
}

type ReviewedCashMovement struct {
	Preview  domain.ChangePreview
	Timezone *time.Location
	command  any
	mutation string
	hash     string
	token    string
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
		account, err := cashMovementAccount(ctx, ledger, input.Account, "accountId")
		if err != nil {
			return nil, err
		}
		if input.Date == "" || input.Time == "" {
			return nil, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
		}
		request, err := cashMovementRequest(ctx, ledger, input, account)
		if err != nil {
			return nil, err
		}
		review.command, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
		if err != nil {
			return nil, err
		}
		review.mutation, review.hash, err = request.MutationEnvelope()
		review.Timezone = location
		return review.command, err
	})
	if err != nil {
		return ReviewedCashMovement{}, err
	}
	review.Preview, review.token = preview, token
	return review, nil
}

func cashMovementAccount(ctx context.Context, ledger CashMovementLedger, id domain.AccountID, field string) (domain.Account, error) {
	if id != "" {
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
		if err != nil {
			return domain.Account{}, err
		}
		if len(rows) == 1 && rows[0].Account.ID == id && acceptsCashMovement(rows[0]) {
			return rows[0].Account, nil
		}
	}
	return domain.Account{}, &domain.Error{Code: domain.ErrValidation, Field: field}
}

func cashMovementRequest(ctx context.Context, ledger CashMovementLedger, input CashMovementInput, account domain.Account) (historyapi.ChangeCommandRequest, error) {
	currency := account.DefaultCurrency
	amount, err := domain.ParseMoney(strings.TrimSpace(input.Amount), currency)
	if err != nil || amount.IsZero() {
		return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
	}
	request := historyapi.ChangeCommandRequest{
		EffectiveLocalDate: input.Date, EffectiveLocalTime: input.Time, MutationID: domain.NewMutationID().String(),
	}
	switch input.Movement {
	case CashContribution, CashWithdrawal:
		request.Kind, request.Reason = historyapi.ChangeMoneyAdded, string(domain.ReasonContribution)
		if input.Movement == CashWithdrawal {
			// Wails has no dedicated withdrawal reason. Other records a
			// general external outflow without labeling it as an expense.
			request.Kind, request.Reason = historyapi.ChangeMoneyRemoved, string(domain.ReasonOther)
		}
		request.AccountID, request.Amount, request.Currency = account.ID.String(), amount.CanonicalAmount(), currency.String()
	case CashTransfer:
		destination, err := cashMovementAccount(ctx, ledger, input.Destination, "toAccountId")
		if err != nil {
			return historyapi.ChangeCommandRequest{}, err
		}
		if !CanTransferTo(account, destination) {
			return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "toAccountId"}
		}
		request.Kind = historyapi.ChangeCashTransfer
		request.FromAccountID, request.ToAccountID = account.ID.String(), destination.ID.String()
		received := amount
		if destination.DefaultCurrency != currency {
			received, err = domain.ParseMoney(strings.TrimSpace(input.Received), destination.DefaultCurrency)
			if err != nil || received.IsZero() {
				return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "received"}
			}
		}
		request.Sent, request.Received = amount.CanonicalAmount(), received.CanonicalAmount()
		request.SentCurrency, request.ReceivedCurrency, request.FeeCurrency = currency.String(), received.Currency().String(), currency.String()
		if raw := strings.TrimSpace(input.Fee); raw != "" {
			fee, err := domain.ParseMoney(raw, currency)
			if err != nil || fee.IsZero() {
				return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrValidation, Field: "fee"}
			}
			request.Fee = fee.CanonicalAmount()
		}
	default:
		return historyapi.ChangeCommandRequest{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "kind"}
	}
	return request, nil
}

// Commit and its retries use the frozen envelope. The guarded application
// path consults the durable mutation record before checking a stale token.
func (r ReviewedCashMovement) Commit(ctx context.Context, ledger CashMovementLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(ctx, r.command, r.mutation, r.hash, r.token)
}

func CashMovementOf(kind domain.ActivityKind, reason domain.ActivityReason) (CashMovement, bool) {
	switch {
	case kind == domain.ActivityCashIn && reason == domain.ReasonContribution:
		return CashContribution, true
	case kind == domain.ActivityCashOut && reason == domain.ReasonOther:
		return CashWithdrawal, true
	case kind == domain.ActivityCashTransfer:
		return CashTransfer, true
	default:
		return 0, false
	}
}

type CashMovementDetail struct {
	Activity domain.Activity
	Accounts map[domain.AccountID]string
	Location *time.Location
}

func LoadCashMovementDetail(ctx context.Context, ledger CashMovementLedger, id domain.ActivityID) (CashMovementDetail, error) {
	activity, err := ledger.Activity(ctx, id)
	if err != nil {
		return CashMovementDetail{}, err
	}
	if _, supported := CashMovementOf(activity.Kind, activity.Reason); activity.ID != id || !supported {
		return CashMovementDetail{}, &domain.Error{Code: domain.ErrNotFound}
	}
	_, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return CashMovementDetail{}, err
	}
	detail := CashMovementDetail{Activity: activity, Location: location, Accounts: make(map[domain.AccountID]string)}
	var ids []domain.AccountID
	for _, effect := range activity.Effects {
		if effect.AccountID != nil {
			id := *effect.AccountID
			if _, exists := detail.Accounts[id]; !exists {
				ids = append(ids, id)
				detail.Accounts[id] = ""
			}
		}
	}
	if len(ids) > 0 {
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: ids, IncludeArchived: true})
		if err != nil {
			return CashMovementDetail{}, err
		}
		for _, row := range rows {
			detail.Accounts[row.Account.ID] = row.Account.Name
		}
	}
	return detail, nil
}
