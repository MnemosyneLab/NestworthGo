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
	movement                                          history.CashMovement
	sources                                           *history.CashMovementSources
	loading                                           <-chan historyResult[history.CashMovementSources]
	sourceError                                       i18n.MessageID
	account, destination                              domain.AccountID
	currency, toCurrency                              domain.CurrencyCode
	amount, received, fee, note                       ui.TextField
	reason                                            domain.ActivityReason
	reasonPicker                                      ui.Select
	incoming                                          bool
	send, receive                                     widget.Clickable
	defaultTime                                       time.Time
	focusReview                                       bool
	day, time                                         string
	focusAmount                                       bool
	accountPicker, destinationPicker                  ui.Select
	currencyPicker, toCurrencyPicker                  ui.Select
	datePicker                                        ui.DatePicker
	timePicker                                        ui.TimePicker
	cancel, retrySources, preview, edit, post, useNow widget.Clickable
	fx                                                transferQuote

	previewing <-chan historyResult[history.ReviewedCashMovement]
	review     *history.ReviewedCashMovement
	posting    <-chan historyResult[cashMovementPost]
	uncertain  bool
	problem    i18n.MessageID
}

type cashMovementPost struct {
	result       domain.ChangePreview
	receiptError bool
}

type historyCashMovements struct {
	ctx        context.Context
	service    cashMovementService
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	draft      *cashMovementDraft
	recovery   *cashReceiptRecovery
	linked     *checkpointCashScope
	account    domain.AccountID
}

func newHistoryCashMovements(ctx context.Context, service cashMovementService, clock func() time.Time, invalidate func()) *historyCashMovements {
	return &historyCashMovements{ctx: ctx, service: service, clock: clock, invalidate: invalidate}
}

func (p *historyCashMovements) start(movement history.CashMovement) {
	p.draft = &cashMovementDraft{
		movement: movement, amount: ui.NewTextField(true), received: ui.NewTextField(true), fee: ui.NewTextField(true),
		focusAmount: movement == history.CashContribution || movement == history.CashWithdrawal,
	}
	if p.linked != nil {
		p.draft.focusAmount = true
		p.draft.note = ui.NewTextField(true)
		p.draft.reason = domain.ReasonOther
		switch movement {
		case history.CashValueUpdate:
			p.draft.amount.Editor.SetText(p.linked.value())
			if p.linked.liability {
				p.draft.reason = domain.ReasonReconciliation
			}
		case history.CashContribution:
			if p.linked.card {
				p.draft.reason = domain.ReasonCardPurchase
			}
		case history.CashWithdrawal:
			if p.linked.card {
				p.draft.reason = domain.ReasonCardRefund
			}
		}
	}
	if p.account != "" {
		p.draft.note = ui.NewTextField(true)
		p.draft.reason = domain.ReasonOther
	}

	p.loadSources()
}

func (p *historyCashMovements) discard() {
	p.draft.fx.stop()
	p.draft = nil
}

func (p *historyCashMovements) loadSources() {
	d := p.draft
	d.sourceError = i18n.NoMessage
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.CashMovementSources, error) {
		if p.linked != nil {
			return history.LoadCheckpointMovementSources(p.ctx, p.service, d.movement)
		}
		return history.LoadCashMovementSources(p.ctx, p.service, d.movement)
	})
}

func (d *cashMovementDraft) receiveSources(sources history.CashMovementSources, now time.Time, linked *checkpointCashScope) {
	first := d.sources == nil
	d.sources = &sources
	switch {
	case linked != nil:
		if d.day == "" {
			d.captureCurrentTime(now)
		}
		if d.movement == history.CashTransfer {
			d.chooseLinkedTransferPeer(linked, d.linkedTransferPeer())
			return
		}
		if d.account != linked.account {
			d.chooseSource(linked.account)
		}
		if d.movement.Debt() {
			account, _ := d.accountByID(d.account)
			peer, _ := d.accountByID(d.destination)
			if !history.IsDebtCounterparty(account, peer) {
				d.destination = ""
			}
			return
		}
		if linked.currency != "" && d.currency != linked.currency && (first || d.movement != history.CashConversion) {
			d.chooseCurrency(linked.currency)
		}
	case first:
		local := now.In(sources.Location)
		d.day, d.time = local.Format(time.DateOnly), local.Format("15:04")
		if d.movement.Debt() {
			var cashID domain.AccountID
			for _, account := range sources.Accounts {
				if !account.IsLiability() {
					if cashID != "" {
						cashID = ""
						break
					}
					cashID = account.ID
				}
			}
			d.chooseSource(cashID)
		} else if len(sources.Accounts) == 1 {
			d.chooseSource(sources.Accounts[0].ID)
		}
	}
	if d.movement.Debt() && d.destination != "" {
		account, _ := d.accountByID(d.account)
		peer, _ := d.accountByID(d.destination)
		if !history.IsDebtCounterparty(account, peer) {
			d.destination = ""
		}
	}
	if d.movement == history.CashTransfer && d.destination != "" && !d.canSelectDestination(d.destination) {
		d.chooseDestination("")
	}
}

func (d *cashMovementDraft) hasDebtEndpoints() bool {
	var cash, debt bool
	for _, account := range d.sources.Accounts {
		if account.IsLiability() {
			debt = true
		} else {
			cash = true
		}
	}
	return cash && debt
}

func (d *cashMovementDraft) linkedTransferPeer() domain.AccountID {
	if d.incoming {
		return d.account
	}
	return d.destination
}

func (d *cashMovementDraft) chooseLinkedTransferDirection(linked *checkpointCashScope, incoming bool) {
	d.incoming = incoming
	d.amount.Editor.SetText("")
	d.received.Editor.SetText("")
	d.fee.Editor.SetText("")
	d.chooseLinkedTransferPeer(linked, "")
}

func (d *cashMovementDraft) chooseLinkedTransferPeer(linked *checkpointCashScope, id domain.AccountID) {
	scope, _ := d.accountByID(linked.account)
	peer, _ := d.accountByID(id)
	if !history.CanTransferTo(scope, peer) {
		peer = domain.Account{}
	}
	scopedCurrency := linked.currency
	if scopedCurrency == "" {
		scopedCurrency = scope.DefaultCurrency
	}
	if d.incoming {
		d.account, d.destination = peer.ID, scope.ID
		d.chooseCurrency(peer.DefaultCurrency)
		d.chooseToCurrency(scopedCurrency)
	} else {
		d.account, d.destination = scope.ID, peer.ID
		d.chooseCurrency(scopedCurrency)
		d.chooseToCurrency(peer.DefaultCurrency)
	}
}

func (d *cashMovementDraft) captureCurrentTime(now time.Time) {
	d.defaultTime = now.In(d.sources.Location)
	d.day, d.time, d.problem = d.defaultTime.Format(time.DateOnly), d.defaultTime.Format(time.TimeOnly), i18n.NoMessage
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
	if _, ok := d.accountByID(d.account); !ok || p.account != "" && d.account != p.account {
		d.problem = i18n.ContributionInvalidAccount
		return
	}
	if d.movement == history.CashTransfer && !d.canSelectDestination(d.destination) {
		d.problem = i18n.TransferInvalidDestination
		return
	}
	if d.movement.Debt() && d.destination == "" {
		d.problem = i18n.LiabilityDebtRequired
		if p.linked != nil && p.linked.liability {
			d.problem = i18n.LiabilityCashRequired
		}
		return
	}
	input := history.CashMovementInput{
		Movement: d.movement, Account: d.account, Destination: d.destination,
		Currency: d.currency, ToCurrency: d.toCurrency,
		Amount: d.amount.Text(), Received: d.received.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time,
	}
	if p.account != "" {
		input.Reason, input.Note = d.reason, d.note.Text()
	}
	if p.linked != nil {
		input.Link = p.linked.link()
		if input.Link == nil {
			return
		}
		input.Reason, input.Note = d.reason, d.note.Text()
		if d.day == d.defaultTime.Format(time.DateOnly) && d.time == d.defaultTime.Format(time.TimeOnly) {
			input.EffectiveAt = d.defaultTime
		}
	}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.ReviewedCashMovement, error) {
		return history.ReviewCashMovement(p.ctx, p.service, input)
	})
}

func (p *historyCashMovements) valueUnchanged() bool {
	d := p.draft
	if d.movement != history.CashValueUpdate {
		return false
	}
	current, err := domain.ParseMoney(p.linked.value(), d.currency)
	if err != nil {
		return false
	}
	target, err := domain.ParseMoney(d.amount.Text(), d.currency)
	return err == nil && current.Amount().Equal(target.Amount())
}

func (p *historyCashMovements) post() {
	d := p.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	review := *d.review
	recoverable := d.movement == history.CashTransfer || d.movement == history.CashConversion
	id, digest := review.ReceiptIdentity()
	if recoverable && p.recovery != nil {
		p.recovery.liveMutation = id
	}
	d.problem = i18n.NoMessage
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (cashMovementPost, error) {
		if recoverable && p.recovery != nil {
			if err := p.recovery.store.Remember(p.ctx, id, digest); err != nil {
				return cashMovementPost{receiptError: true}, err
			}
		}
		result, err := review.Commit(p.ctx, p.service)
		return cashMovementPost{result: result}, err
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
	d.fx.poll()
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sourceError = cashMovementProblem(d.movement, result.err, i18n.ContributionSourcesFailed)
		} else {
			d.receiveSources(result.value, p.clock(), p.linked)
			if p.account != "" {
				d.chooseSource(p.account)
			}
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = cashMovementProblem(d.movement, result.err, p.formLabels().previewFailed)
			if p.linked != nil && staleReconciliationLink(result.err) {
				d.problem = i18n.HoldingReconChanged
				p.linked.refresh()
			} else if d.problem == i18n.ContributionInvalidAccount || d.problem == i18n.TransferInvalidDestination ||
				d.movement.Debt() && (d.problem == i18n.LiabilityCashRequired || d.problem == i18n.LiabilityDebtRequired) {
				p.loadSources()
			}
		} else {
			d.review, d.focusReview = &result.value, p.linked != nil
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		activity := result.value.result.Activity
		if result.err == nil && activity.ID != "" {
			if p.recovery != nil && isCashReceiptActivity(activity) {
				id, _ := d.review.ReceiptIdentity()
				p.recovery.remember(cashReceiptNotice{mutation: id, message: i18n.CashReceiptRecorded, activity: activity.ID})
			}
			p.discard()
			return &activity
		}
		if result.value.receiptError {
			// The failed attempt never reached the ledger. If an older
			// attempt lost its response, keep its frozen review retryable.
			d.problem = i18n.CashReceiptSaveFailed
			break
		}
		var issue *domain.Error
		if (errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview) ||
			p.linked != nil && staleReconciliationLink(result.err) {
			// Replay runs before the guard. A stale token here means this
			// mutation has no durable outcome and needs a new preview.
			d.review, d.uncertain, d.problem = nil, false, p.formLabels().stale
			p.loadSources()
			if p.linked != nil {
				d.focusAmount = true
				p.linked.refresh()
			}
		} else {
			// A transport or storage failure may follow the commit. Keep the
			// exact request frozen until the idempotent retry resolves it.
			d.uncertain, d.focusReview = true, p.linked != nil
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
		if movement.Debt() || movement == history.CashValueUpdate {
			return i18n.CashMovementHistoryRequired
		}
		return cashMovementLabels[movement].historyRequired
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	case domain.ErrTransferMismatch:
		if movement.Debt() {
			return i18n.LiabilityCurrencyMatch
		}
	case domain.ErrNoChange:
		if movement == history.CashValueUpdate {
			return i18n.ReconciliationValueUnchanged
		}
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
	case "amount", "principal":
		return i18n.ContributionInvalidAmount
	case "newValue":
		return i18n.AccountCreateAmountInvalid
	case "interestOrFee":
		return i18n.LiabilityInvalidInterest
	case "debtAccountId":
		return i18n.LiabilityDebtRequired
	case "cashAccountId":
		return i18n.LiabilityCashRequired
	case "counterparty":
		return i18n.ContributionInvalidAccount
	}
	return fallback
}
