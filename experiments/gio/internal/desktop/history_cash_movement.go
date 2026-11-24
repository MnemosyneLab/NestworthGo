package desktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type cashMovementDraft struct {
	movement                                  history.CashMovement
	sources                                   *history.CashMovementSources
	loading                                   <-chan historyResult[history.CashMovementSources]
	sourceError                               i18n.MessageID
	account, destination                      domain.AccountID
	currency, toCurrency                      domain.CurrencyCode
	amount, received, fee                     ui.TextField
	day, time                                 string
	focusAmount                               bool
	accountPicker, destinationPicker          ui.Select
	currencyPicker, toCurrencyPicker          ui.Select
	datePicker                                ui.DatePicker
	timePicker                                ui.TimePicker
	cancel, retrySources, preview, edit, post widget.Clickable

	previewing <-chan historyResult[history.ReviewedCashMovement]
	review     *history.ReviewedCashMovement
	posting    <-chan historyResult[domain.ChangePreview]
	uncertain  bool
	problem    i18n.MessageID
}

type historyCashMovements struct {
	ctx        context.Context
	service    history.CashMovementLedger
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	actions    [len(cashMovementLabels)]widget.Clickable
	draft      *cashMovementDraft
}

func newHistoryCashMovements(ctx context.Context, service history.CashMovementLedger, clock func() time.Time, invalidate func()) *historyCashMovements {
	return &historyCashMovements{ctx: ctx, service: service, clock: clock, invalidate: invalidate}
}

func (p *historyCashMovements) start(movement history.CashMovement) {
	p.draft = &cashMovementDraft{
		movement: movement, amount: ui.NewTextField(true), received: ui.NewTextField(true), fee: ui.NewTextField(true),
		focusAmount: movement != history.CashTransfer && movement != history.CashConversion,
	}
	p.loadSources()
}

func (p *historyCashMovements) loadSources() {
	d := p.draft
	d.sourceError = i18n.NoMessage
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.CashMovementSources, error) {
		return history.LoadCashMovementSources(p.ctx, p.service, d.movement)
	})
}

func (d *cashMovementDraft) receiveSources(sources history.CashMovementSources, now time.Time) {
	first := d.sources == nil
	d.sources = &sources
	if first {
		local := now.In(sources.Location)
		d.day, d.time = local.Format(time.DateOnly), local.Format("15:04")
		if len(sources.Accounts) == 1 {
			d.chooseSource(sources.Accounts[0].ID)
		}
	}
	if d.movement == history.CashTransfer && d.destination != "" && !d.canSelectDestination(d.destination) {
		d.chooseDestination("")
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

func (d *cashMovementDraft) chooseSource(id domain.AccountID) {
	changed := d.account != id
	d.account, d.problem = id, i18n.NoMessage
	if !changed {
		return
	}
	after, _ := d.accountByID(id)
	d.chooseCurrency(after.DefaultCurrency)
	if d.movement == history.CashTransfer && !d.canSelectDestination(d.destination) {
		d.chooseDestination("")
	}
}

func (d *cashMovementDraft) chooseDestination(id domain.AccountID) {
	changed := d.destination != id
	d.destination, d.problem = id, i18n.NoMessage
	if !changed {
		return
	}
	after, _ := d.accountByID(id)
	d.chooseToCurrency(after.DefaultCurrency)
}

func (d *cashMovementDraft) chooseCurrency(code domain.CurrencyCode) {
	if d.currency != "" && d.currency != code {
		d.amount.Editor.SetText("")
		d.fee.Editor.SetText("")
	}
	d.currency, d.problem = code, i18n.NoMessage
	if d.toCurrency == code {
		d.received.Editor.SetText("")
		if d.movement == history.CashConversion {
			d.toCurrency = ""
		}
	}
}

func (d *cashMovementDraft) chooseToCurrency(code domain.CurrencyCode) {
	if d.toCurrency != code {
		d.received.Editor.SetText("")
	}
	d.toCurrency, d.problem = code, i18n.NoMessage
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
	input := history.CashMovementInput{
		Movement: d.movement, Account: d.account, Destination: d.destination,
		Currency: d.currency, ToCurrency: d.toCurrency,
		Amount: d.amount.Text(), Received: d.received.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time,
	}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.ReviewedCashMovement, error) {
		return history.ReviewCashMovement(p.ctx, p.service, input)
	})
}

func (p *historyCashMovements) post() {
	d := p.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	review := *d.review
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (domain.ChangePreview, error) {
		return review.Commit(p.ctx, p.service)
	})
}

func (p *historyCashMovements) writing() bool {
	return p != nil && p.draft != nil && p.draft.posting != nil
}

func (p *historyCashMovements) update() *domain.Activity {
	d := p.draft
	if d == nil {
		return nil
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
			return &result.value.Activity
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
	return nil
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
	case "received":
		return i18n.TransferInvalidReceived
	case "currency":
		return i18n.CashCurrencyRequired
	case "toCurrency":
		if movement == history.CashConversion {
			return i18n.ConversionCurrencyRequired
		}
		return i18n.CashCurrencyRequired
	case "amount":
		return i18n.ContributionInvalidAmount
	}
	return fallback
}
