package desktop

import (
	"errors"
	"slices"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountFirstBuy struct {
	service  history.FirstBuyLedger
	open     widget.Clickable
	draft    *firstBuyDraft
	onPosted func(domain.Activity, event.Tag)
}

type firstBuyDraft struct {
	account                      domain.Account
	instrument                   domain.InstrumentID
	quantity, gross, fee         ui.TextField
	day, time                    string
	selectInstrument             ui.Select
	datePicker                   ui.DatePicker
	timePicker                   ui.TimePicker
	cancel, retry, preview, edit widget.Clickable
	confirm, retryConfirm        widget.Clickable
	sourceProblem, problem       i18n.MessageID
	sources                      *history.FirstBuySources
	loading                      <-chan historyResult[history.FirstBuySources]
	previewing                   <-chan historyResult[history.ReviewedFirstBuy]
	review                       *history.ReviewedFirstBuy
	posting                      <-chan historyResult[domain.ChangePreview]
	uncertain                    bool
}

func (p *accountsPage) canFirstBuy() bool {
	row := p.freshDetail()
	return p.firstBuy != nil && p.firstBuy.onPosted != nil && row != nil &&
		history.FirstBuyAccount(row.record.Account) && len(p.current.holdings) == 0
}

func (p *accountsPage) startFirstBuy() {
	f := p.firstBuy
	f.draft = &firstBuyDraft{account: p.freshDetail().record.Account,
		quantity: ui.NewTextField(true), gross: ui.NewTextField(true), fee: ui.NewTextField(true)}
	p.loadFirstBuySources()
}

func (d *firstBuyDraft) chooseInstrument(id domain.InstrumentID) {
	if d.instrument != id {
		d.instrument, d.problem = id, i18n.NoMessage
		d.quantity.Editor.SetText("")
		d.gross.Editor.SetText("")
		d.fee.Editor.SetText("")
	}
}

func (p *accountsPage) loadFirstBuySources() {
	f, d := p.firstBuy, p.firstBuy.draft
	d.sources, d.sourceProblem = nil, i18n.NoMessage
	id := d.account.ID
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.FirstBuySources, error) {
		return history.LoadFirstBuySources(p.ctx, f.service, id)
	})
}

func (p *accountsPage) previewFirstBuy() {
	f, d := p.firstBuy, p.firstBuy.draft
	if d.previewing != nil || d.sources == nil || d.instrument == "" {
		return
	}
	input := history.FirstBuyInput{Account: d.account.ID, Instrument: d.instrument, Quantity: d.quantity.Text(),
		Gross: d.gross.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.ReviewedFirstBuy, error) {
		return history.ReviewFirstBuy(p.ctx, f.service, input)
	})
}

func (p *accountsPage) postFirstBuy() {
	f, d := p.firstBuy, p.firstBuy.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	plan := *d.review
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (domain.ChangePreview, error) {
		return plan.Commit(p.ctx, f.service)
	})
}

func (p *accountsPage) firstBuyUnresolved() bool {
	return p.firstBuy != nil && p.firstBuy.draft != nil && (p.firstBuy.draft.posting != nil || p.firstBuy.draft.uncertain)
}

func (p *accountsPage) pollFirstBuy() {
	f := p.firstBuy
	if f == nil || f.draft == nil {
		return
	}
	d := f.draft
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sourceProblem = firstBuyProblem(result.err, i18n.BuySourcesFailed)
		} else {
			d.sources = &result.value
			d.account = result.value.Account
			if d.day == "" {
				now := p.clock().In(result.value.Location)
				d.day, d.time = now.Format(time.DateOnly), now.Format("15:04")
			}
			if d.instrument != "" && !slices.ContainsFunc(result.value.Instruments, func(i domain.Instrument) bool { return i.ID == d.instrument }) {
				d.chooseInstrument("")
				d.problem = i18n.BuyInstrumentUnavailable
			}
			if len(result.value.Instruments) == 1 && d.instrument == "" {
				d.chooseInstrument(result.value.Instruments[0].ID)
			}
		}
	default:
	}
	select {
	case result := <-d.previewing:
		d.previewing = nil
		if result.err != nil {
			d.problem = firstBuyProblem(result.err, i18n.BuyPreviewFailed)
			if d.problem == i18n.BuyScopeUnavailable || d.problem == i18n.BuyInstrumentUnavailable {
				p.loadFirstBuySources()
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
			f.draft = nil
			f.onPosted(result.value.Activity, &f.open)
			return
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// The shared service checks committed receipts before the
			// guard, so this mutation can be discarded on a stale token.
			d.review, d.uncertain, d.problem = nil, false, i18n.BuyStale
			p.loadFirstBuySources()
		} else {
			d.uncertain = true
		}
	default:
	}
}

func firstBuyProblem(err error, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Field {
	case "settlementCash":
		return i18n.BuyCashRequired
	case "settlementAccountId", "holdings":
		return i18n.BuyScopeUnavailable
	}
	switch issue.Code {
	case domain.ErrHistoryNotStarted:
		return i18n.BuyHistoryRequired
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	}
	switch issue.Field {
	case "quantity":
		return i18n.BuyInvalidQuantity
	case "gross":
		return i18n.BuyInvalidGross
	case "fee":
		return i18n.BuyInvalidFee
	case "instrumentId":
		return i18n.BuyInstrumentUnavailable
	}
	return fallback
}
