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

	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)

	PreviewChangeGuardedWith(context.Context, func(context.Context) (any, error)) (domain.ChangePreview, string, error)
	RecordChangeGuarded(context.Context, any, string, string, string) (domain.ChangePreview, error)
}

type balanceTargetInput struct {
	amount, day, clock, note string
	reason                   domain.ActivityReason
}

type balanceTargetPlan struct {
	account domain.AccountID

	currency                     domain.CurrencyCode
	before, target, after, delta string
	at                           time.Time
	zone                         string
	reason                       domain.ActivityReason
	note                         *string
	command                      any
	mutation, payloadHash, token string
}

type balanceTargetReceipt struct {
	account domain.AccountID

	activity domain.ActivityID
}

func (r balanceTargetReceipt) known() bool { return r.activity != "" }

type balanceTargetDraft struct {
	account                                      domain.Account
	amount, note                                 ui.TextField
	day, clock                                   string
	reason                                       domain.ActivityReason
	reasonPicker                                 ui.Select
	datePicker                                   ui.DatePicker
	timePicker                                   ui.TimePicker
	useNow                                       widget.Clickable
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

	open, view   widget.Clickable
	restore      *widget.Clickable
	entry        *widget.Clickable
	openActivity func(domain.ActivityID, event.Tag)
}

func supportsBalanceTarget(row domain.AccountRecord) bool {
	a := row.Account
	if a.ArchivedAt != nil || row.LatestValue == nil {
		return false
	}
	return a.TrackingMode == domain.TrackingManualValue || a.TrackingMode == domain.TrackingBalance
}

func (p *accountsPage) canUpdateBalanceTarget() bool {
	row := p.freshDetail()
	return p.balanceTarget != nil && row != nil && p.current.historyZone != nil && supportsBalanceTarget(row.record)
}

func (p *accountsPage) startBalanceTarget() {
	row := p.freshDetail().record
	d := &balanceTargetDraft{account: row.Account, amount: ui.NewTextField(true), note: ui.NewTextField(true),
		reason: domain.ReasonReconciliation, focus: true}
	d.amount.Editor.SetText(row.LatestValue.Amount.CanonicalAmount())
	p.balanceTarget.draft, p.balanceTarget.entry = d, &p.balanceTarget.open
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

func reviewBalanceTarget(ctx context.Context, service balanceTargetLedger, id domain.AccountID, input balanceTargetInput, clock func() time.Time) (balanceTargetPlan, error) {
	var plan balanceTargetPlan
	err := service.WithRead(ctx, func(ctx context.Context) error {
		row, err := readBalanceTarget(ctx, service, id)
		if err != nil {
			return err
		}
		money, err := domain.ParseMoney(strings.TrimSpace(input.amount), row.Account.DefaultCurrency)
		if err != nil {
			return err
		}
		origin, err := service.HistoryOrigin(ctx)
		if err != nil {
			return err
		}
		previous := row.LatestValue.Amount
		plan = balanceTargetPlan{account: id, currency: money.Currency(),
			before: previous.CanonicalAmount(), target: money.CanonicalAmount()}
		if origin == nil {
			return &domain.Error{Code: domain.ErrMissingHistoryOrigin}
		}
		reason := input.reason
		if reason == "" || row.Account.IsLiability() {
			reason = domain.ReasonReconciliation
		}
		request := historyapi.ChangeCommandRequest{
			Kind: historyapi.ChangeValueUpdate, AccountID: id.String(), NewValue: plan.target, NewValueCurrency: money.Currency().String(),
			Reason: string(reason), EffectiveLocalDate: input.day, EffectiveLocalTime: input.clock,
			MutationID: domain.NewMutationID().String(),
		}
		if input.day == "" && input.clock == "" {
			request.EffectiveAt = clock().UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)
		}
		if note := strings.TrimSpace(input.note); note != "" {
			request.Note = &note
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
		plan.reason, plan.note = preview.Activity.Reason, preview.Activity.Note
		return nil
	})
	return plan, err
}

func (plan balanceTargetPlan) commit(ctx context.Context, service balanceTargetLedger) (balanceTargetReceipt, error) {
	receipt := balanceTargetReceipt{account: plan.account}
	result, err := service.RecordChangeGuarded(ctx, plan.command, plan.mutation, plan.payloadHash, plan.token)
	receipt.activity = result.Activity.ID
	return receipt, err
}

func (p *accountsPage) previewBalanceTarget() {
	f, d := p.balanceTarget, p.balanceTarget.draft
	id := d.account.ID
	input := balanceTargetInput{amount: d.amount.Text(), day: d.day, clock: d.clock, reason: d.reason, note: d.note.Text()}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (balanceTargetPlan, error) {
		return reviewBalanceTarget(p.ctx, f.service, id, input, p.clock)
	})
}

func (p *accountsPage) confirmBalanceTarget() {
	f, d := p.balanceTarget, p.balanceTarget.draft
	plan := *d.review
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
			f.saved, f.savedMessage, f.draft, f.restore = result.value, copy.saved, nil, f.entry
			p.restoreAccountRecord(f.entry)
			return result.value
		}
		d.problem = copy.problem(result.err, i18n.NoMessage)
		switch {
		case d.problem != i18n.NoMessage:
			d.review, d.uncertain, d.focus = nil, false, true
		default:
			d.uncertain = true
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
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInvalidChange, domain.ErrInsufficientBalance:
		return copy.previewFailed
	}
	return fallback
}
