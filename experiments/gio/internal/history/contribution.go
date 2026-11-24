package history

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

// ContributionLedger is the subset of the shared application service used by
// the contribution form and its authoritative activity detail.
type ContributionLedger interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
	Activity(context.Context, domain.ActivityID) (domain.Activity, error)
}

type ContributionSources struct {
	Origin   domain.HistoryOrigin
	Location *time.Location
	Accounts []domain.Account
}

func LoadContributionSources(ctx context.Context, ledger ContributionLedger) (ContributionSources, error) {
	origin, location, err := contributionOrigin(ctx, ledger)
	if err != nil {
		return ContributionSources{}, err
	}
	rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{})
	if err != nil {
		return ContributionSources{}, err
	}
	sources := ContributionSources{Origin: origin, Location: location}
	for _, row := range rows {
		if acceptsContribution(row) {
			sources.Accounts = append(sources.Accounts, row.Account)
		}
	}
	return sources, nil
}

func contributionOrigin(ctx context.Context, ledger ContributionLedger) (domain.HistoryOrigin, *time.Location, error) {
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

func acceptsContribution(row domain.AccountRecord) bool {
	a := row.Account
	return row.LatestValue != nil && a.ArchivedAt == nil && a.BalanceSheetRole == domain.RoleAsset && a.TrackingMode == domain.TrackingBalance &&
		(a.AccountType == domain.TypeCashOnHand || a.AccountType == domain.TypeBankAccount || a.AccountType == domain.TypeDigitalWallet)
}

type ContributionInput struct {
	Account domain.AccountID
	Amount  string
	Date    string
	Time    string
}

type ReviewedContribution struct {
	Preview  domain.ChangePreview
	Timezone *time.Location
	command  any
	mutation string
	hash     string
	token    string
}

// ReviewContribution freezes a Wails Money Added request and its mutation
// envelope. Account and Origin are read under the same shared preview gate
// that establishes the state token used at confirmation.
func ReviewContribution(ctx context.Context, ledger ContributionLedger, input ContributionInput) (ReviewedContribution, error) {
	var review ReviewedContribution
	preview, token, err := ledger.PreviewChangeGuardedWith(ctx, func(ctx context.Context) (any, error) {
		origin, location, err := contributionOrigin(ctx, ledger)
		if err != nil {
			return nil, err
		}
		if input.Account == "" {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "accountId"}
		}
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{input.Account}})
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 || rows[0].Account.ID != input.Account || !acceptsContribution(rows[0]) {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "accountId"}
		}
		if input.Date == "" || input.Time == "" {
			return nil, &domain.Error{Code: domain.ErrInvalidChangeTime, Field: "effectiveLocalDateTime"}
		}
		currency := rows[0].Account.DefaultCurrency
		amount, err := domain.ParseMoney(strings.TrimSpace(input.Amount), currency)
		if err != nil || amount.IsZero() {
			return nil, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
		}
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeMoneyAdded, AccountID: input.Account.String(),
			Amount: amount.CanonicalAmount(), Currency: currency.String(), Reason: string(domain.ReasonContribution),
			EffectiveLocalDate: input.Date, EffectiveLocalTime: input.Time, MutationID: domain.NewMutationID().String(),
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
		return ReviewedContribution{}, err
	}
	review.Preview, review.token = preview, token
	return review, nil
}

// Commit and its retries use the frozen envelope. The guarded application
// path consults the durable mutation record before checking a stale token.
func (r ReviewedContribution) Commit(ctx context.Context, ledger ContributionLedger) (domain.ChangePreview, error) {
	return ledger.RecordChangeGuarded(ctx, r.command, r.mutation, r.hash, r.token)
}

func IsContribution(kind domain.ActivityKind, reason domain.ActivityReason) bool {
	return kind == domain.ActivityCashIn && reason == domain.ReasonContribution
}

type ContributionDetail struct {
	Activity domain.Activity
	Account  string
	Location *time.Location
}

func LoadContributionDetail(ctx context.Context, ledger ContributionLedger, id domain.ActivityID) (ContributionDetail, error) {
	activity, err := ledger.Activity(ctx, id)
	if err != nil {
		return ContributionDetail{}, err
	}
	if activity.ID != id || !IsContribution(activity.Kind, activity.Reason) {
		return ContributionDetail{}, &domain.Error{Code: domain.ErrNotFound}
	}
	_, location, err := contributionOrigin(ctx, ledger)
	if err != nil {
		return ContributionDetail{}, err
	}
	detail := ContributionDetail{Activity: activity, Location: location}
	index := slices.IndexFunc(activity.Effects, func(effect domain.ActivityEffect) bool { return effect.AccountID != nil })
	if index >= 0 {
		accountID := *activity.Effects[index].AccountID
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{accountID}, IncludeArchived: true})
		if err != nil {
			return ContributionDetail{}, err
		}
		if len(rows) == 1 && rows[0].Account.ID == accountID {
			detail.Account = rows[0].Account.Name
		}
	}
	return detail, nil
}
