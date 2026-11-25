package desktop

import (
	"errors"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyAmendmentDraft struct {
	action    history.AmendmentAction
	movement  history.AmendableCash
	amount    ui.TextField
	received  ui.TextField
	fee       ui.TextField
	scroll    widget.List
	review    *history.ReviewedAmendment
	preview   <-chan historyResult[history.ReviewedAmendment]
	commit    <-chan historyResult[domain.ChangePreview]
	problem   i18n.MessageID
	uncertain bool
	focus     bool

	cancel, prepare, confirm, edit widget.Clickable
}

func (r *historyReads) startAmendment(action history.AmendmentAction) {
	d := r.detail
	if d.record == nil || !d.record.Amendable || (action == history.ReverseCash && !d.record.Reversible) {
		return
	}
	movement, supported := history.CashAmendment(d.record.Activity)
	if !supported {
		return
	}
	form := &historyAmendmentDraft{
		action: action, movement: movement, focus: action == history.CorrectCash,
		amount: ui.NewTextField(true), received: ui.NewTextField(true), fee: ui.NewTextField(true),
		scroll: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	form.amount.Editor.SetText(movement.Principal.Money.CanonicalAmount())
	if movement.Paired() {
		form.received.Editor.SetText(movement.Received.Money.CanonicalAmount())
		if movement.Fee.Money != nil {
			form.fee.Editor.SetText(movement.Fee.Money.CanonicalAmount())
		}
	}
	d.amendment = form
	if action == history.ReverseCash {
		r.previewAmendment()
	}
}

func (r *historyReads) previewAmendment() {
	d := r.detail
	form := d.amendment
	if form.preview != nil || form.review != nil || form.commit != nil {
		return
	}
	at := r.clock().UTC()
	amounts := history.AmendmentAmounts{Amount: form.amount.Text(), Received: form.received.Text(), Fee: form.fee.Text()}
	form.problem = i18n.NoMessage
	form.preview = runHistoryRead(&r.workers, r.invalidate, func() (history.ReviewedAmendment, error) {
		return history.ReviewAmendment(r.ctx, r.service, form.action, d.id, amounts, at)
	})
}

func (r *historyReads) commitAmendment() {
	form := r.detail.amendment
	if form.commit != nil || form.review == nil {
		return
	}
	review := *form.review
	form.commit = runHistoryRead(&r.workers, r.invalidate, func() (domain.ChangePreview, error) {
		return review.Commit(r.ctx, r.service)
	})
}

func (p *historyPage) amendmentUnresolved() bool {
	if p.reads == nil || p.reads.detail == nil || p.reads.detail.amendment == nil {
		return false
	}
	form := p.reads.detail.amendment
	return form.commit != nil || form.uncertain
}

func (p *historyPage) pollAmendment() {
	d := p.reads.detail
	if d == nil || d.amendment == nil {
		return
	}
	form := d.amendment
	select {
	case result := <-form.preview:
		form.preview = nil
		if result.err != nil {
			form.problem = amendmentProblem(result.err, i18n.HistoryAmendmentPreviewFailed)
		} else {
			form.review = &result.value
			form.scroll.Position = layout.Position{}
		}
	default:
	}
	select {
	case result := <-form.commit:
		form.commit = nil
		// The shared fix path can return the saved replacement with an
		// error from its subsequent historical snapshot refresh.
		if result.value.Activity.ID != "" {
			notice := i18n.HistoryReversalSaved
			focus := &d.reverse
			if form.action == history.CorrectCash {
				notice, focus = i18n.HistoryCorrectionSaved, &d.correct
				if result.err != nil {
					notice = i18n.HistoryCorrectionSavedRefreshFailed
				}
			}
			d.amendment = nil
			p.showPosted(result.value.Activity, notice, focus)
			return
		}
		problem := amendmentProblem(result.err, i18n.NoMessage)
		if problem != i18n.NoMessage {
			form.review, form.uncertain, form.problem = nil, false, problem
			form.scroll.Position = layout.Position{}
		} else {
			form.uncertain = true
		}
	default:
	}
}

func amendmentProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrStalePreview:
		return i18n.HistoryAmendmentStale
	case domain.ErrNoChange:
		return i18n.HistoryAmendmentNoChange
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	case domain.ErrAlreadyUndone, domain.ErrNotFound:
		return i18n.HistoryAmendmentUnavailable
	case domain.ErrInvalidChange:
		if issue.Field == "activityId" {
			return i18n.HistoryAmendmentUnavailable
		}
		return i18n.HistoryAmendmentRefused
	case domain.ErrConflict:
		if issue.Field == "accountId" {
			return i18n.HistoryReversalArchived
		}
		return i18n.HistoryAmendmentRefused
	case domain.ErrCannotFixChange, domain.ErrInvalidChangeTime:
		return i18n.HistoryAmendmentRefused
	case domain.ErrValidation:
		switch issue.Field {
		case "amount":
			return i18n.ContributionInvalidAmount
		case "received":
			return i18n.TransferInvalidReceived
		case "fee":
			return i18n.TransferInvalidFee
		}
		return i18n.HistoryAmendmentRefused
	default:
		return fallback
	}
}
