package desktop

import (
	"errors"
	"strings"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyAmendmentDraft struct {
	action      history.AmendmentAction
	movement    history.AmendableCash
	custody     *custodyAmendmentDraft
	amount      ui.TextField
	received    ui.TextField
	fee         ui.TextField
	scroll      widget.List
	review      *history.ReviewedAmendment
	preview     <-chan historyResult[history.ReviewedAmendment]
	commit      <-chan historyResult[domain.ChangePreview]
	problem     i18n.MessageID
	uncertain   bool
	focus       bool
	reviewFocus bool

	cancel, prepare, confirm, edit widget.Clickable
}

func (r *historyReads) startAmendment(action history.AmendmentAction) {
	d := r.detail
	if d.record == nil || !d.record.Amendable || (action == history.ReverseActivity && !d.record.Reversible) {
		return
	}
	movement, supported := history.CashAmendment(d.record.Activity)
	custody, isCustody := history.CustodyAmendment(d.record.Activity)
	if !supported && !isCustody {
		return
	}
	form := &historyAmendmentDraft{
		action: action, movement: movement, focus: action == history.CorrectActivity,
		amount: ui.NewTextField(true), received: ui.NewTextField(true), fee: ui.NewTextField(true),
		scroll: widget.List{List: layout.List{Axis: layout.Vertical}},
	}
	d.amendment = form
	if isCustody {
		form.custody = newCustodyAmendment(custody, d.record.Activity.Note)
		form.amount.Editor.SetText(custody.From.Quantity.Canonical())
		if action == history.CorrectActivity {
			r.loadCustodyAmendment(form)
		}
	} else {
		form.amount.Editor.SetText(movement.Principal.Money.CanonicalAmount())
	}
	if supported && movement.Paired() {
		form.received.Editor.SetText(movement.Received.Money.CanonicalAmount())
		if movement.Fee.Money != nil {
			form.fee.Editor.SetText(movement.Fee.Money.CanonicalAmount())
		}
	}
	if action == history.ReverseActivity {
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
	amounts := history.AmendmentInput{Amount: form.amount.Text(), Received: form.received.Text(), Fee: form.fee.Text()}
	if c := form.custody; c != nil && form.action == history.CorrectActivity {
		if !c.selected() || strings.TrimSpace(amounts.Amount) == "" {
			return
		}
		amounts.From, amounts.To, amounts.Note = c.from, c.to, c.note.Text()
	}
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
	if c := form.custody; c != nil {
		select {
		case result := <-c.loading:
			c.loading = nil
			c.sources, c.readFailed = result.value, result.err != nil
		default:
		}
	}
	select {
	case result := <-form.preview:
		form.preview = nil
		if result.err != nil {
			form.problem = amendmentProblem(result.err, i18n.HistoryAmendmentPreviewFailed)
		} else {
			form.review = &result.value
			form.reviewFocus = form.custody != nil
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
			if form.action == history.CorrectActivity {
				notice, focus = i18n.HistoryCorrectionSaved, &d.correct
				if result.err != nil {
					notice = i18n.HistoryCorrectionSavedRefreshFailed
				}
			}
			if form.custody != nil {
				focus = &d.linked[2]
				if form.action == history.CorrectActivity {
					focus = &d.linked[3]
				}
			}
			d.amendment = nil
			p.showPosted(result.value.Activity, notice, focus)
			return
		}
		problem := amendmentProblem(result.err, i18n.NoMessage)
		if problem != i18n.NoMessage {
			form.review, form.uncertain, form.problem = nil, false, problem
			if form.custody != nil && form.action == history.CorrectActivity {
				form.focus = true
				if problem == i18n.HistoryAmendmentStale {
					p.reads.loadCustodyAmendment(form)
				}
			}
			form.scroll.Position = layout.Position{}
		} else {
			form.uncertain = true
			form.reviewFocus = form.custody != nil
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
	case domain.ErrInsufficientQuantity:
		return i18n.SellInsufficientQuantity
	case domain.ErrTransferMismatch:
		if issue.Field == "quantity" {
			return i18n.CustodyAmendmentInvalidQuantity
		}
		return i18n.HistoryAmendmentRefused
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
		case "quantity":
			return i18n.CustodyAmendmentInvalidQuantity
		}
		return i18n.HistoryAmendmentRefused
	default:
		return fallback
	}
}
