package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

func LiabilityPostingAccount(account domain.Account) bool {
	return liabilityAccount(account) || liabilityCashAccount(account)
}

func liabilityAccount(account domain.Account) bool {
	return account.ArchivedAt == nil && account.IsLiability() && account.TrackingMode == domain.TrackingBalance
}

func liabilityCashAccount(account domain.Account) bool {
	return account.ArchivedAt == nil && !account.IsLiability() &&
		(account.TrackingMode == domain.TrackingBalance || account.TrackingMode == domain.TrackingHoldings)
}

func IsDebtCounterparty(account, peer domain.Account) bool {
	if account.IsLiability() {
		return liabilityCashAccount(peer)
	}
	return liabilityAccount(peer)
}

type LiabilitySources struct {
	Account        domain.Account
	Origin         domain.HistoryOrigin
	Location       *time.Location
	Counterparties []domain.Account
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
			break
		}
	}
	if sources.Account.ID == "" {
		return LiabilitySources{}, &domain.Error{Code: domain.ErrNotFound, Field: "accountId"}
	}
	for _, row := range rows {
		if IsDebtCounterparty(sources.Account, row.Account) {
			sources.Counterparties = append(sources.Counterparties, row.Account)
		}
	}
	return sources, nil
}

func (s LiabilitySources) Counterparty(id domain.AccountID) domain.Account {
	for _, account := range s.Counterparties {
		if account.ID == id {
			return account
		}
	}
	return domain.Account{}
}

func LiabilityReasons(kind historyapi.ChangeCommandKind) []domain.ActivityReason {
	switch kind {
	case historyapi.ChangeMoneyAdded:
		return domain.LiabilityIncreaseReasons()
	case historyapi.ChangeMoneyRemoved:
		return domain.LiabilityDecreaseReasons()
	case historyapi.ChangeValueUpdate:
		return []domain.ActivityReason{domain.ReasonReconciliation}
	default:
		return nil
	}
}

type LiabilityInput struct {
	Kind         historyapi.ChangeCommandKind
	Account      domain.AccountID
	Counterparty domain.AccountID
	Reason       domain.ActivityReason
	Currency     domain.CurrencyCode
	Amount       string
	Fee          string
	Note         string
	Date, Time   string
	ExactTime    time.Time
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
	case historyapi.ChangeMoneyAdded, historyapi.ChangeMoneyRemoved, historyapi.ChangeValueUpdate:
		if !s.Account.IsLiability() {
			return request, &domain.Error{Code: domain.ErrNotFound, Field: "accountId"}
		}
		if !slices.Contains(LiabilityReasons(in.Kind), in.Reason) {
			return request, &domain.Error{Code: domain.ErrValidation, Field: "reason"}
		}
		if in.Currency != s.Account.DefaultCurrency {
			return request, &domain.Error{Code: domain.ErrTransferMismatch, Field: "currency"}
		}
		request.AccountID, request.Reason = s.Account.ID.String(), string(in.Reason)
		if in.Kind == historyapi.ChangeValueUpdate {
			request.NewValue, request.NewValueCurrency = strings.TrimSpace(in.Amount), in.Currency.String()
			if _, err := domain.ParseMoney(request.NewValue, in.Currency); err != nil {
				return request, &domain.Error{Code: domain.ErrValidation, Field: "newValue"}
			}
		} else {
			request.Currency, request.Amount = in.Currency.String(), strings.TrimSpace(in.Amount)
		}
	case historyapi.ChangeDebtDraw, historyapi.ChangeDebtPayment:
		cash, debt := s.Counterparty(in.Counterparty), s.Account
		if !s.Account.IsLiability() {
			cash, debt = debt, cash
		}
		if debt.ID == "" {
			return request, &domain.Error{Code: domain.ErrNotFound, Field: "debtAccountId"}
		}
		if cash.ID == "" {
			return request, &domain.Error{Code: domain.ErrNotFound, Field: "cashAccountId"}
		}
		if cash.DefaultCurrency != debt.DefaultCurrency {
			return request, &domain.Error{Code: domain.ErrTransferMismatch, Field: "principal"}
		}
		request.DebtAccountID, request.CashAccountID = debt.ID.String(), cash.ID.String()
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
