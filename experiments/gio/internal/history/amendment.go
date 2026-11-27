package history

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	CorrectActivity AmendmentAction = "fix"
	ReverseActivity AmendmentAction = "undo"
)

type AmendmentInput struct {
	Amount   string
	Received string
	Fee      string
	From, To domain.HoldingID
	Note     string
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
func ReviewAmendment(ctx context.Context, ledger AmendmentLedger, action AmendmentAction, id domain.ActivityID, amounts AmendmentInput, at time.Time) (ReviewedAmendment, error) {
	review := ReviewedAmendment{Action: action, original: id, mutation: domain.NewMutationID().String()}
	var request *historyapi.ChangeCommandRequest
	var err error
	switch action {
	case CorrectActivity:
		review.Preview, review.token, err = ledger.PreviewFixChangeGuardedWith(ctx, id, func(ctx context.Context) (any, error) {
			original, err := ledger.Activity(ctx, id)
			if err != nil {
				return nil, err
			}
			var replacement historyapi.ChangeCommandRequest
			if _, custody := CustodyAmendment(original); custody && original.ID == id {
				replacement = custodyReplacement(amounts)
			} else {
				movement, accounts, err := cashAmendmentTarget(ctx, ledger, id, original)
				if err != nil {
					return nil, err
				}
				replacement, err = movement.replacement(amounts, accounts)
				if err != nil {
					return nil, err
				}
				if movement.unchanged(replacement) {
					return nil, &domain.Error{Code: domain.ErrNoChange, Field: "amount"}
				}
				replacement.Note = original.Note
			}
			replacement.EffectiveAt = original.EffectiveAt.UTC().Format(time.RFC3339Nano)
			request = &replacement
			origin, err := ledger.HistoryOrigin(ctx)
			if err != nil {
				return nil, err
			}
			if origin == nil {
				return nil, &domain.Error{Code: domain.ErrMissingHistoryOrigin}
			}
			review.replacement, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
			return review.replacement, err
		})
	case ReverseActivity:
		err = ledger.WithRead(ctx, func(ctx context.Context) error {
			original, err := ledger.Activity(ctx, id)
			if err != nil {
				return err
			}
			if _, custody := CustodyAmendment(original); !custody || original.ID != id {
				_, accounts, err := cashAmendmentTarget(ctx, ledger, id, original)
				if err != nil {
					return err
				}
				if !accounts.active() {
					return &domain.Error{Code: domain.ErrConflict, Field: "accountId"}
				}
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
	digest := sha256.Sum256(append([]byte("gio_activity_amendment\n"), payload...))
	review.hash = hex.EncodeToString(digest[:])
	return review, nil
}

func cashAmendmentTarget(ctx context.Context, ledger AmendmentLedger, id domain.ActivityID, activity domain.Activity) (AmendableCash, amendmentAccounts, error) {
	if movement, supported := CashAmendment(activity); supported && activity.ID == id {
		ids := []domain.AccountID{*movement.Principal.AccountID}
		if movement.Paired() && *movement.Received.AccountID != ids[0] {
			ids = append(ids, *movement.Received.AccountID)
		}
		rows, err := ledger.ListAccounts(ctx, domain.AccountFilter{AccountIDs: ids, IncludeArchived: true})
		if err != nil {
			return AmendableCash{}, amendmentAccounts{}, err
		}
		accounts, ok, err := movement.accounts(ctx, ledger, id, rows)
		if err != nil {
			return AmendableCash{}, amendmentAccounts{}, err
		}
		if ok {
			return movement, accounts, nil
		}
	}
	return AmendableCash{}, amendmentAccounts{}, &domain.Error{Code: domain.ErrInvalidChange, Field: "activityId"}
}

func (r ReviewedAmendment) Commit(ctx context.Context, ledger AmendmentLedger) (domain.ChangePreview, error) {
	if r.Action == CorrectActivity {
		return ledger.RecordFixChangeGuarded(ctx, r.original, r.replacement, r.mutation, r.hash, r.token)
	}
	return ledger.RecordUndoChangeGuardedAt(ctx, r.original, r.effectiveAt, r.mutation, r.hash, r.token)
}
