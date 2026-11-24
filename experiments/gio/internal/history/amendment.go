package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type AmendmentLedger interface {
	DetailReader
	WithRead(context.Context, func(context.Context) error) error
	PreviewFixChangeGuardedWith(context.Context, domain.ActivityID, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	PreviewUndoChangeGuardedAt(context.Context, domain.ActivityID, time.Time) (domain.ChangePreview, string, error)
	RecordFixChangeGuarded(context.Context, domain.ActivityID, any, string, string, string) (domain.ChangePreview, error)
	RecordUndoChangeGuardedAt(context.Context, domain.ActivityID, time.Time, string, string, string) (domain.ChangePreview, error)
}

type AmendmentAction string

const (
	CorrectCash AmendmentAction = "fix"
	ReverseCash AmendmentAction = "undo"
)

// CashAmendmentEffect limits the Gio action to an ordinary deposit or
// withdrawal on a total-balance endpoint. Other History records stay readable.
func CashAmendmentEffect(activity domain.Activity) (domain.ActivityEffect, bool) {
	if activity.ReversesActivityID != nil || activity.ProductContext != nil || len(activity.Effects) != 1 {
		return domain.ActivityEffect{}, false
	}
	effect := activity.Effects[0]
	if effect.AccountID == nil || effect.Role != domain.EffectRoleAmount || effect.Target != domain.EffectTargetAccountValue ||
		effect.Money == nil || effect.Money.IsZero() {
		return domain.ActivityEffect{}, false
	}
	cashIn := activity.Kind == domain.ActivityCashIn && activity.Reason == domain.ReasonContribution &&
		effect.Direction == domain.EffectAdded && effect.Classification == domain.ClassificationExternalInflow
	cashOut := activity.Kind == domain.ActivityCashOut && activity.Reason == domain.ReasonOther &&
		effect.Direction == domain.EffectRemoved && effect.Classification == domain.ClassificationExternalOutflow
	return effect, cashIn || cashOut
}

func acceptsCashAmendment(row domain.AccountRecord, effect domain.ActivityEffect) bool {
	return row.Account.ID == *effect.AccountID && row.Account.TrackingMode == domain.TrackingBalance &&
		row.Account.DefaultCurrency == effect.Money.Currency() && acceptsCashMovement(row)
}

type ReviewedAmendment struct {
	Preview     domain.ChangePreview
	Action      AmendmentAction
	original    domain.ActivityID
	replacement any
	effectiveAt time.Time
	mutation    string
	hash        string
	token       string
}

// ReviewAmendment freezes the target, exact replacement, reviewed effective
// instant and mutation envelope. The application owns replay and projection.
func ReviewAmendment(ctx context.Context, ledger AmendmentLedger, action AmendmentAction, id domain.ActivityID, amount string, at time.Time) (ReviewedAmendment, error) {
	review := ReviewedAmendment{Action: action, original: id, mutation: domain.NewMutationID().String()}
	var request *historyapi.ChangeCommandRequest
	var err error
	switch action {
	case CorrectCash:
		review.Preview, review.token, err = ledger.PreviewFixChangeGuardedWith(ctx, id, func(ctx context.Context) (any, error) {
			original, effect, err := amendmentTarget(ctx, ledger, id)
			if err != nil {
				return nil, err
			}
			money, err := domain.ParseMoney(strings.TrimSpace(amount), effect.Money.Currency())
			if err != nil || money.IsZero() {
				return nil, &domain.Error{Code: domain.ErrValidation, Field: "amount"}
			}
			if money.CanonicalAmount() == effect.Money.CanonicalAmount() {
				return nil, &domain.Error{Code: domain.ErrNoChange, Field: "amount"}
			}
			kind := historyapi.ChangeMoneyAdded
			if original.Kind == domain.ActivityCashOut {
				kind = historyapi.ChangeMoneyRemoved
			}
			request = &historyapi.ChangeCommandRequest{
				Kind: kind, AccountID: effect.AccountID.String(), Amount: money.CanonicalAmount(),
				Currency: money.Currency().String(), Reason: string(original.Reason),
				EffectiveAt: original.EffectiveAt.UTC().Format(time.RFC3339Nano), Note: original.Note,
			}
			origin, err := ledger.HistoryOrigin(ctx)
			if err != nil {
				return nil, err
			}
			if origin == nil {
				return nil, &domain.Error{Code: domain.ErrHistoryNotStarted}
			}
			review.replacement, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
			return review.replacement, err
		})
	case ReverseCash:
		err = ledger.WithRead(ctx, func(ctx context.Context) error {
			if _, _, err := amendmentTarget(ctx, ledger, id); err != nil {
				return err
			}
			preview, token, err := ledger.PreviewUndoChangeGuardedAt(ctx, id, at.UTC())
			review.Preview, review.token = preview, token
			return err
		})
	default:
		return ReviewedAmendment{}, &domain.Error{Code: domain.ErrValidation, Field: "action"}
	}
	if err != nil {
		return ReviewedAmendment{}, err
	}
	review.effectiveAt = review.Preview.Activity.EffectiveAt.UTC()
	payload, err := jsoncodec.Marshal(struct {
		Action      AmendmentAction
		Original    domain.ActivityID
		EffectiveAt time.Time
		Replacement *historyapi.ChangeCommandRequest
	}{action, id, review.effectiveAt, request})
	if err != nil {
		return ReviewedAmendment{}, err
	}
	digest := sha256.Sum256(append([]byte("gio_cash_amendment\n"), payload...))
	review.hash = hex.EncodeToString(digest[:])
	return review, nil
}

func amendmentTarget(ctx context.Context, ledger AmendmentLedger, id domain.ActivityID) (domain.Activity, domain.ActivityEffect, error) {
	activity, err := ledger.Activity(ctx, id)
	if err != nil {
		return domain.Activity{}, domain.ActivityEffect{}, err
	}
	if effect, supported := CashAmendmentEffect(activity); supported && activity.ID == id {
		accounts, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{*effect.AccountID}})
		if err != nil {
			return domain.Activity{}, domain.ActivityEffect{}, err
		}
		if len(accounts) == 1 && acceptsCashAmendment(accounts[0], effect) {
			return activity, effect, nil
		}
	}
	return domain.Activity{}, domain.ActivityEffect{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId"}
}

func (r ReviewedAmendment) Commit(ctx context.Context, ledger AmendmentLedger) (domain.ChangePreview, error) {
	if r.Action == CorrectCash {
		return ledger.RecordFixChangeGuarded(ctx, r.original, r.replacement, r.mutation, r.hash, r.token)
	}
	return ledger.RecordUndoChangeGuardedAt(ctx, r.original, r.effectiveAt, r.mutation, r.hash, r.token)
}
