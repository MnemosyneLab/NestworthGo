package history

import (
	"context"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func LiabilityPostingAccount(account domain.Account) bool {
	return account.ArchivedAt == nil && account.IsLiability() && account.TrackingMode == domain.TrackingBalance
}

func liabilityCashAccount(account domain.Account) bool {
	return account.ArchivedAt == nil && !account.IsLiability() &&
		(account.TrackingMode == domain.TrackingBalance || account.TrackingMode == domain.TrackingHoldings)
}

type LiabilitySources struct {
	Account  domain.Account
	Origin   domain.HistoryOrigin
	Location *time.Location
	Cash     []domain.Account
}

func LoadLiabilitySources(ctx context.Context, ledger CashMovementLedger, id domain.AccountID) (LiabilitySources, error) {
	origin, location, err := cashMovementOrigin(ctx, ledger)
	if err != nil {
		return LiabilitySources{}, err
	}
	rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return LiabilitySources{}, err
	}
	sources := LiabilitySources{Origin: origin, Location: location}
	for _, row := range rows {
		if row.Account.ID == id && LiabilityPostingAccount(row.Account) {
			sources.Account = row.Account
		}
		if liabilityCashAccount(row.Account) {
			sources.Cash = append(sources.Cash, row.Account)
		}
	}
	if sources.Account.ID == "" {
		return LiabilitySources{}, &domain.Error{Code: domain.ErrNotFound, Field: "debtAccountId"}
	}
	return sources, nil
}

type LiabilityInput struct {
	Kind       historyapi.ChangeCommandKind
	Account    domain.AccountID
	Cash       domain.AccountID
	Currency   domain.CurrencyCode
	Amount     string
	Fee        string
	Note       string
	Date, Time string
	ExactTime  time.Time
}

type LiabilityReview struct {
	Preview  domain.ChangePreview
	Timezone *time.Location
	command  any
	mutation string
	hash     string
	token    string
}

func ReviewLiability(ctx context.Context, ledger CashMovementLedger, in LiabilityInput) (LiabilityReview, error) {
	var review LiabilityReview
	preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		sources, err := LoadLiabilitySources(ctx, ledger, in.Account)
		if err != nil {
			return nil, err
		}
		request, err := liabilityRequest(sources, in)
		if err != nil {
			return nil, err
		}
		request.MutationID = domain.NewMutationID().String()
		review.command, err = request.ToCommand(sources.Origin.HouseholdID, sources.Origin.Timezone)
		if err != nil {
			return nil, err
		}
		review.mutation, review.hash, err = request.MutationEnvelope()
		review.Timezone = sources.Location
		return review.command, err
	})
	if err != nil {
		return LiabilityReview{}, err
	}
	review.Preview, review.token = preview, token
	return review, nil
}

func (r LiabilityReview) Commit(ctx context.Context, ledger CashMovementLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(ctx, r.command, r.mutation, r.hash, r.token)
}

func liabilityRequest(s LiabilitySources, in LiabilityInput) (historyapi.ChangeCommandRequest, error) {
	request := historyapi.ChangeCommandRequest{
		Kind: in.Kind, EffectiveLocalDate: in.Date, EffectiveLocalTime: in.Time,
	}
	if !in.ExactTime.IsZero() {
		request.EffectiveAt = in.ExactTime.UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
		request.EffectiveLocalDate, request.EffectiveLocalTime = "", ""
	} else if in.Date == "" || in.Time == "" {
		return request, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		request.Note = &note
	}
	switch in.Kind {
	case historyapi.ChangeMoneyAdded, historyapi.ChangeMoneyRemoved:
		if s.Account.AccountType != domain.TypeCreditCard {
			return request, &domain.Error{Code: domain.ErrNotFound, Field: "debtAccountId"}
		}
		if in.Currency != s.Account.DefaultCurrency {
			return request, &domain.Error{Code: domain.ErrTransferMismatch, Field: "currency"}
		}
		request.AccountID, request.Currency, request.Amount = s.Account.ID.String(), in.Currency.String(), strings.TrimSpace(in.Amount)
		request.Reason = string(domain.ReasonCardPurchase)
		if in.Kind == historyapi.ChangeMoneyRemoved {
			request.Reason = string(domain.ReasonCardRefund)
		}
	case historyapi.ChangeDebtDraw, historyapi.ChangeDebtPayment:
		var cash domain.Account
		for _, account := range s.Cash {
			if account.ID == in.Cash {
				cash = account
				break
			}
		}
		if cash.ID == "" {
			return request, &domain.Error{Code: domain.ErrNotFound, Field: "cashAccountId"}
		}
		if cash.DefaultCurrency != s.Account.DefaultCurrency {
			return request, &domain.Error{Code: domain.ErrTransferMismatch, Field: "principal"}
		}
		request.DebtAccountID, request.CashAccountID = s.Account.ID.String(), cash.ID.String()
		request.Principal, request.PrincipalCurrency = strings.TrimSpace(in.Amount), cash.DefaultCurrency.String()
		if in.Kind == historyapi.ChangeDebtPayment {
			request.InterestOrFee, request.InterestOrFeeCurrency = strings.TrimSpace(in.Fee), request.PrincipalCurrency
			if request.InterestOrFee != "" {
				if _, err := domain.ParseMoney(request.InterestOrFee, cash.DefaultCurrency); err != nil {
					return request, &domain.Error{Code: domain.ErrValidation, Field: "interestOrFee"}
				}
			}
		}
	default:
		return request, &domain.Error{Code: domain.ErrInvalidChange, Field: "kind"}
	}
	return request, nil
}
