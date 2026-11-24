package desktop

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type cashMovementResult[T any] struct {
	value T
	err   error
}

type cashMovementDraft struct {
	movement                                  history.CashMovement
	sources                                   *history.CashMovementSources
	loading                                   <-chan cashMovementResult[history.CashMovementSources]
	sourceError                               i18n.MessageID
	account, destination                      domain.AccountID
	amount, fee                               ui.TextField
	day, time                                 string
	focusAmount                               bool
	accountPicker, destinationPicker          ui.Select
	datePicker                                ui.DatePicker
	timePicker                                ui.TimePicker
	cancel, retrySources, preview, edit, post widget.Clickable

	previewing <-chan cashMovementResult[history.ReviewedCashMovement]
	review     *history.ReviewedCashMovement
	posting    <-chan cashMovementResult[domain.ChangePreview]
	uncertain  bool
	problem    i18n.MessageID
}

type cashMovementVisit struct {
	id          domain.ActivityID
	movement    history.CashMovement
	posted      bool
	pending     <-chan cashMovementResult[history.CashMovementDetail]
	record      *history.CashMovementDetail
	err         error
	back, retry widget.Clickable
}

type historyCashMovements struct {
	ctx        context.Context
	service    history.CashMovementLedger
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	actions    [len(cashMovementLabels)]widget.Clickable
	buttons    map[domain.ActivityID]*widget.Clickable
	draft      *cashMovementDraft
	detail     *cashMovementVisit
}

func newHistoryCashMovements(ctx context.Context, service history.CashMovementLedger, clock func() time.Time, invalidate func()) *historyCashMovements {
	return &historyCashMovements{ctx: ctx, service: service, clock: clock, invalidate: invalidate, buttons: make(map[domain.ActivityID]*widget.Clickable)}
}

func cashMovementRead[T any](p *historyCashMovements, read func() (T, error)) <-chan cashMovementResult[T] {
	results := make(chan cashMovementResult[T], 1)
	p.workers.Go(func() {
		value, err := read()
		results <- cashMovementResult[T]{value: value, err: err}
		p.invalidate()
	})
	return results
}

func (p *historyCashMovements) start(movement history.CashMovement) {
	p.draft = &cashMovementDraft{movement: movement, amount: ui.NewTextField(true), fee: ui.NewTextField(true), focusAmount: movement != history.CashTransfer}
	p.loadSources()
}

func (p *historyCashMovements) loadSources() {
	d := p.draft
	d.sourceError = i18n.NoMessage
	d.loading = cashMovementRead(p, func() (history.CashMovementSources, error) {
		return history.LoadCashMovementSources(p.ctx, p.service)
	})
}

func (d *cashMovementDraft) receiveSources(sources history.CashMovementSources, now time.Time) {
	first := d.sources == nil
	d.sources = &sources
	if first {
		local := now.In(sources.Location)
		d.day, d.time = local.Format(time.DateOnly), local.Format("15:04")
		if len(sources.Accounts) == 1 {
			d.account = sources.Accounts[0].ID
		}
	}
	if !d.canSelectDestination(d.destination) {
		d.destination = ""
	}
}

func (d *cashMovementDraft) accountByID(id domain.AccountID) (domain.Account, bool) {
	if d.sources != nil {
		for _, account := range d.sources.Accounts {
			if account.ID == id {
				return account, true
			}
		}
	}
	return domain.Account{}, false
}

func (d *cashMovementDraft) canSelectDestination(id domain.AccountID) bool {
	from, _ := d.accountByID(d.account)
	to, _ := d.accountByID(id)
	return history.CanTransferTo(from, to)
}

func (p *historyCashMovements) preview() {
	d := p.draft
	if d.previewing != nil {
		return
	}
	if _, ok := d.accountByID(d.account); !ok {
		d.problem = i18n.ContributionInvalidAccount
		return
	}
	if d.movement == history.CashTransfer && !d.canSelectDestination(d.destination) {
		d.problem = i18n.TransferInvalidDestination
		return
	}
	input := history.CashMovementInput{Movement: d.movement, Account: d.account, Destination: d.destination, Amount: d.amount.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time}
	d.problem = i18n.NoMessage
	d.previewing = cashMovementRead(p, func() (history.ReviewedCashMovement, error) {
		return history.ReviewCashMovement(p.ctx, p.service, input)
	})
}

func (p *historyCashMovements) post() {
	d := p.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	review := *d.review
	d.posting = cashMovementRead(p, func() (domain.ChangePreview, error) {
		return review.Commit(p.ctx, p.service)
	})
}

func (p *historyCashMovements) writing() bool {
	return p != nil && p.draft != nil && p.draft.posting != nil
}

func (p *historyCashMovements) open(id domain.ActivityID, movement history.CashMovement, posted bool) {
	p.detail = &cashMovementVisit{id: id, movement: movement, posted: posted}
	p.readDetail()
}

func (p *historyCashMovements) readDetail() {
	visit := p.detail
	id := visit.id
	visit.err = nil
	visit.pending = cashMovementRead(p, func() (history.CashMovementDetail, error) {
		return history.LoadCashMovementDetail(p.ctx, p.service, id)
	})
}

func (p *historyCashMovements) update() bool {
	if visit := p.detail; visit != nil {
		select {
		case result := <-visit.pending:
			visit.pending, visit.err = nil, result.err
			if result.err == nil {
				visit.record = &result.value
			}
		default:
		}
	}
	d := p.draft
	if d == nil {
		return false
	}
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sourceError = cashMovementProblem(d.movement, result.err, i18n.ContributionSourcesFailed)
		} else {
			d.receiveSources(result.value, p.clock())
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = cashMovementProblem(d.movement, result.err, cashMovementLabels[d.movement].previewFailed)
			if d.problem == i18n.ContributionInvalidAccount || d.problem == i18n.TransferInvalidDestination {
				p.loadSources()
			}
		} else {
			d.review = &result.value
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.Activity.ID != "" {
			p.draft = nil
			p.open(result.value.Activity.ID, d.movement, true)
			return true
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// Replay runs before the guard. A stale token here means this
			// mutation has no durable outcome and needs a new preview.
			d.review, d.uncertain, d.problem = nil, false, cashMovementLabels[d.movement].stale
			p.loadSources()
		} else {
			// A transport or storage failure may follow the commit. Keep the
			// exact request frozen until the idempotent retry resolves it.
			d.uncertain = true
		}
	default:
	}
	return false
}

func cashMovementProblem(movement history.CashMovement, err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Code {
	case domain.ErrHistoryNotStarted:
		return cashMovementLabels[movement].historyRequired
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	}
	switch issue.Field {
	case "accountId", "account":
		return i18n.ContributionInvalidAccount
	case "toAccountId":
		return i18n.TransferInvalidDestination
	case "fee":
		return i18n.TransferInvalidFee
	case "amount":
		return i18n.ContributionInvalidAmount
	}
	return fallback
}

func (p *historyCashMovements) detailButton(id domain.ActivityID) *widget.Clickable {
	if p.buttons[id] == nil {
		p.buttons[id] = new(widget.Clickable)
	}
	return p.buttons[id]
}

func (p *historyCashMovements) pruneButtons(rows []history.Row) {
	for id := range p.buttons {
		if !slices.ContainsFunc(rows, func(row history.Row) bool { return row.ID == id }) {
			delete(p.buttons, id)
		}
	}
}
