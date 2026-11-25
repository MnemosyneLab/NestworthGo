package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type balanceTargetLedger interface {
	WithRead(context.Context, func(context.Context) error) error
	WithWrite(context.Context, func(context.Context) error) error
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
	AppendAccountValue(context.Context, domain.AccountID, string, string) (domain.AccountValue, error)
	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
}

type balanceTargetPlan struct {
	account                      domain.AccountID
	observation                  domain.AccountValueID
	currency                     domain.CurrencyCode
	before, target, after, delta string
	at                           time.Time
	zone                         string
	command                      any
	mutation, payloadHash, token string
}

type balanceTargetReceipt struct {
	account     domain.AccountID
	observation domain.AccountValueID
	activity    domain.ActivityID
}

func (r balanceTargetReceipt) known() bool { return r.observation != "" || r.activity != "" }

type balanceTargetDraft struct {
	account                                      domain.Account
	amount                                       ui.TextField
	focus, uncertain                             bool
	problem                                      i18n.MessageID
	review                                       *balanceTargetPlan
	previewing                                   <-chan historyResult[balanceTargetPlan]
	confirming                                   <-chan historyResult[balanceTargetReceipt]
	prepare, confirm, retryConfirm, edit, cancel widget.Clickable
}

type accountBalanceTarget struct {
	service      balanceTargetLedger
	draft        *balanceTargetDraft
	saved        balanceTargetReceipt
	savedMessage i18n.MessageID
	unverified   domain.AccountID
	open, view   widget.Clickable
	openActivity func(domain.ActivityID, event.Tag)
}

func supportsBalanceTarget(row domain.AccountRecord) bool {
	a := row.Account
	if a.ArchivedAt != nil || a.TrackingMode != domain.TrackingBalance || row.LatestValue == nil {
		return false
	}
	return (a.AccountType == domain.TypeBankAccount && a.BalanceSheetRole == domain.RoleAsset) ||
		(a.AccountType == domain.TypeCreditCard && a.IsLiability())
}

func (p *accountsPage) canUpdateBalanceTarget() bool {
	row := p.freshDetail()
	return p.balanceTarget != nil && row != nil && supportsBalanceTarget(row.record)
}

func (p *accountsPage) startBalanceTarget() {
	row := p.freshDetail().record
	d := &balanceTargetDraft{account: row.Account, amount: ui.NewTextField(true), focus: true}
	d.amount.Editor.SetText(row.LatestValue.Amount.CanonicalAmount())
	p.balanceTarget.draft = d
}

func readBalanceTarget(ctx context.Context, service balanceTargetLedger, id domain.AccountID) (domain.AccountRecord, error) {
	rows, err := service.ListAccounts(ctx, domain.AccountFilter{AccountIDs: []domain.AccountID{id}})
	if err != nil {
		return domain.AccountRecord{}, err
	}
	if len(rows) != 1 || rows[0].Account.ID != id || !supportsBalanceTarget(rows[0]) {
		return domain.AccountRecord{}, &domain.Error{Code: domain.ErrNotFound}
	}
	return rows[0], nil
}

func reviewBalanceTarget(ctx context.Context, service balanceTargetLedger, id domain.AccountID, raw string, clock func() time.Time) (balanceTargetPlan, error) {
	var plan balanceTargetPlan
	err := service.WithRead(ctx, func(ctx context.Context) error {
		row, err := readBalanceTarget(ctx, service, id)
		if err != nil {
			return err
		}
		money, err := domain.ParseMoney(strings.TrimSpace(raw), row.Account.DefaultCurrency)
		if err != nil {
			return err
		}
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		previous := row.LatestValue.Amount
		plan = balanceTargetPlan{account: id, observation: row.LatestValue.ID, currency: money.Currency(),
			before: previous.CanonicalAmount(), target: money.CanonicalAmount()}
		if origin == nil {
			if plan.before == plan.target {
				return &domain.Error{Code: domain.ErrNoChange}
			}
			plan.after = plan.target
			if row.Account.IsLiability() {
				// Observations store the debt magnitude even when excluded
				// from net worth. Sign only the review's display amounts.
				plan.before, plan.after = previous.Amount().Neg().String(), money.Amount().Neg().String()
			}
			return nil
		}
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeValueUpdate, AccountID: id.String(), NewValue: plan.target, NewValueCurrency: money.Currency().String(),
			Reason: string(domain.ReasonReconciliation), EffectiveAt: clock().UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano),
			MutationID: domain.NewMutationID().String(),
		}
		plan.command, err = request.ToCommand(origin.HouseholdID, origin.Timezone)
		if err != nil {
			return err
		}
		plan.mutation, plan.payloadHash, err = request.MutationEnvelope()
		if err != nil {
			return err
		}
		preview, token, err := service.PreviewChangeGuardedWith(ctx, func(context.Context) (any, error) { return plan.command, nil })
		if err != nil {
			return err
		}
		if len(preview.Review) != 1 || preview.Review[0].Target != domain.EffectTargetAccountValue {
			return &domain.Error{Code: domain.ErrIntegrity}
		}
		line := preview.Review[0]
		plan.before, plan.after, plan.delta = line.Before, line.After, line.Delta
		plan.at, plan.zone, plan.token = preview.Activity.EffectiveAt, origin.Timezone, token
		return nil
	})
	return plan, err
}

func (plan balanceTargetPlan) commit(ctx context.Context, service balanceTargetLedger) (balanceTargetReceipt, error) {
	receipt := balanceTargetReceipt{account: plan.account}
	if plan.command != nil {
		result, err := service.RecordChangeGuarded(ctx, plan.command, plan.mutation, plan.payloadHash, plan.token)
		receipt.activity = result.Activity.ID
		return receipt, err
	}
	err := service.WithWrite(ctx, func(ctx context.Context) error {
		// The legacy append has no mutation receipt. Keep its baseline-only
		// decision and the reviewed observation under the same write permit.
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		row, err := readBalanceTarget(ctx, service, plan.account)
		if err != nil {
			return err
		}
		if origin != nil || row.LatestValue.ID != plan.observation || row.Account.DefaultCurrency != plan.currency {
			return &domain.Error{Code: domain.ErrStalePreview}
		}
		result, err := service.AppendAccountValue(ctx, plan.account, plan.target, "")
		receipt.observation = result.ID
		return err
	})
	return receipt, err
}

func (p *accountsPage) previewBalanceTarget() {
	f, d := p.balanceTarget, p.balanceTarget.draft
	id, raw := d.account.ID, d.amount.Text()
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (balanceTargetPlan, error) {
		return reviewBalanceTarget(p.ctx, f.service, id, raw, p.clock)
	})
}

func (p *accountsPage) confirmBalanceTarget() {
	f, d := p.balanceTarget, p.balanceTarget.draft
	plan := *d.review
	f.unverified = ""
	d.confirming = runHistoryRead(&p.workers, p.invalidate, func() (balanceTargetReceipt, error) {
		return plan.commit(p.ctx, f.service)
	})
}

func (p *accountsPage) pollBalanceTarget() balanceTargetReceipt {
	f := p.balanceTarget
	if f == nil || f.draft == nil {
		return balanceTargetReceipt{}
	}
	d := f.draft
	copy := balanceTargetCopyFor(d.account)
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = copy.problem(result.err, copy.previewFailed)
		} else {
			d.review = &result.value
		}
	default:
	}
	select {
	case result := <-d.confirming:
		d.confirming = nil
		if result.value.known() {
			f.saved, f.savedMessage, f.draft = result.value, copy.saved, nil
			return result.value
		}
		d.problem = copy.problem(result.err, i18n.NoMessage)
		switch {
		case d.problem != i18n.NoMessage:
			d.review, d.uncertain, d.focus = nil, false, true
		case d.review.command != nil:
			d.uncertain = true
		default:
			// An unkeyed append may already have committed. Return to the
			// account and read its latest observation without reissuing it.
			f.unverified, f.saved, f.draft = d.account.ID, balanceTargetReceipt{}, nil
			return balanceTargetReceipt{account: d.account.ID}
		}
	default:
	}
	return balanceTargetReceipt{}
}

func (p *accountsPage) balanceTargetUnresolved() bool {
	f := p.balanceTarget
	return f != nil && f.draft != nil && (f.draft.confirming != nil || f.draft.uncertain)
}

func (copy balanceTargetCopy) problem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrNoChange:
		return copy.noChange
	case domain.ErrStalePreview:
		return i18n.HistoryAmendmentStale
	case domain.ErrCashBalanceTimeConflict:
		return i18n.AccountValueTimeConflict
	case domain.ErrNotFound, domain.ErrConflict:
		return i18n.AccountUnavailable
	case domain.ErrValidation:
		if issue.Field == "amount" {
			return copy.invalidTarget
		}
		return copy.previewFailed
	case domain.ErrInvalidChange, domain.ErrInvalidChangeTime, domain.ErrInsufficientBalance:
		return copy.previewFailed
	}
	return fallback
}
