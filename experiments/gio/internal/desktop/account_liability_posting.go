package desktop

import (
	"errors"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	historyapi "github.com/waltwang/nestworth-go/internal/wailsapi/history"
)

type accountLiabilityPosting struct {
	service  history.CashMovementLedger
	onPosted func(domain.Activity, i18n.MessageID, event.Tag)
	open     widget.Clickable
	restore  bool
	draft    *liabilityPostingDraft
}

type liabilityPostingDraft struct {
	account                             domain.Account
	kind                                historyapi.ChangeCommandKind
	cash                                domain.AccountID
	currency                            domain.CurrencyCode
	amount, fee, note                   ui.TextField
	kinds                               [4]widget.Clickable
	cashPicker, currencyPicker          ui.Select
	datePicker                          ui.DatePicker
	timePicker                          ui.TimePicker
	cancel, useNow, retry, preview      widget.Clickable
	edit, confirm, retryConfirm         widget.Clickable
	date, time                          string
	exactTime                           time.Time
	sourceProblem, problem              i18n.MessageID
	sources                             *history.LiabilitySources
	loading                             <-chan historyResult[history.LiabilitySources]
	previewing                          <-chan historyResult[history.LiabilityReview]
	review                              *history.LiabilityReview
	posting                             <-chan historyResult[domain.ChangePreview]
	uncertain, focusAmount, focusReview bool
}

func (p *accountsPage) canPostLiability() bool {
	row := p.freshDetail()
	return p.liabilityPosting != nil && p.liabilityPosting.onPosted != nil && row != nil && history.LiabilityPostingAccount(row.record.Account)
}

func (p *accountsPage) startLiabilityPosting() {
	a := p.freshDetail().record.Account
	d := &liabilityPostingDraft{account: a, kind: historyapi.ChangeDebtDraw, currency: a.DefaultCurrency,
		amount: ui.NewTextField(true), fee: ui.NewTextField(true), note: ui.NewTextField(false), focusAmount: true}
	if a.AccountType == domain.TypeCreditCard {
		d.kind = historyapi.ChangeMoneyAdded
	}
	p.liabilityPosting.draft = d
	p.loadLiabilityPosting()
}

func (p *accountsPage) loadLiabilityPosting() {
	f, d := p.liabilityPosting, p.liabilityPosting.draft
	d.sourceProblem = i18n.NoMessage
	id := d.account.ID
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.LiabilitySources, error) {
		return history.LoadLiabilitySources(p.ctx, f.service, id)
	})
}

func (d *liabilityPostingDraft) debt() bool {
	return d.kind == historyapi.ChangeDebtDraw || d.kind == historyapi.ChangeDebtPayment
}

func (d *liabilityPostingDraft) captureTime(now time.Time) {
	d.exactTime = now.In(d.sources.Location)
	d.date, d.time = d.exactTime.Format(time.DateOnly), d.exactTime.Format("15:04")
}

func (p *accountsPage) previewLiabilityPosting() {
	f, d := p.liabilityPosting, p.liabilityPosting.draft
	in := history.LiabilityInput{
		Kind: d.kind, Account: d.account.ID, Cash: d.cash, Currency: d.currency,
		Amount: d.amount.Text(), Fee: d.fee.Text(), Note: d.note.Text(),
		Date: d.date, Time: d.time, ExactTime: d.exactTime,
	}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.LiabilityReview, error) {
		return history.ReviewLiability(p.ctx, f.service, in)
	})
}

func (p *accountsPage) confirmLiabilityPosting() {
	f, d := p.liabilityPosting, p.liabilityPosting.draft
	review := *d.review
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (domain.ChangePreview, error) {
		return review.Commit(p.ctx, f.service)
	})
}

func (p *accountsPage) liabilityPostingUnresolved() bool {
	f := p.liabilityPosting
	return f != nil && f.draft != nil && (f.draft.posting != nil || f.draft.uncertain)
}

func (p *accountsPage) pollLiabilityPosting() {
	f := p.liabilityPosting
	if f == nil || f.draft == nil {
		return
	}
	d := f.draft
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sources = nil
			d.sourceProblem = liabilityPostingProblem(result.err, i18n.AccountLoadError)
		} else {
			d.sources, d.account = &result.value, result.value.Account
			if d.date == "" {
				d.captureTime(p.clock())
			}
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = liabilityPostingProblem(result.err, i18n.LiabilityPreviewFailed)
		} else {
			d.review, d.focusReview = &result.value, true
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		var issue *domain.Error
		if result.err == nil && result.value.Activity.ID != "" {
			f.draft = nil
			f.onPosted(result.value.Activity, i18n.LiabilityPosted, &f.open)
		} else if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// A committed mutation is replayed before checking the token.
			d.review, d.uncertain, d.focusAmount = nil, false, true
			d.problem = i18n.LiabilityStalePreview
			p.loadLiabilityPosting()
		} else {
			d.uncertain, d.focusReview = true, true
		}
	default:
	}
}

func liabilityPostingProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrHistoryNotStarted:
		return i18n.CashMovementHistoryRequired
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	case domain.ErrTransferMismatch:
		return i18n.LiabilityCurrencyMatch
	}
	switch issue.Field {
	case "amount", "principal":
		return i18n.ContributionInvalidAmount
	case "interestOrFee":
		return i18n.LiabilityInvalidInterest
	case "note":
		return i18n.HistoryRepeatNoteTooLong
	case "accountId", "debtAccountId":
		return i18n.AccountUnavailable
	case "cashAccountId":
		return i18n.LiabilityCashRequired
	}
	return fallback
}
