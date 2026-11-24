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

type accountTrade struct {
	service   history.TradeLedger
	buy, sell widget.Clickable
	draft     *accountTradeDraft
	onPosted  func(domain.Activity, i18n.MessageID, event.Tag)
}

type accountTradeDraft struct {
	side                         domain.TradeSide
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
	sources                      *history.TradeSources
	loading                      <-chan historyResult[history.TradeSources]
	previewing                   <-chan historyResult[history.ReviewedTrade]
	review                       *history.ReviewedTrade
	posting                      <-chan historyResult[domain.ChangePreview]
	uncertain                    bool
}

func (p *accountsPage) canFirstBuy() bool {
	return p.canTradeAccount() && len(p.current.holdings) == 0
}

func (p *accountsPage) canSell() bool {
	if !p.canTradeAccount() {
		return false
	}
	row := p.current.rows[0]
	if row.value == nil {
		return false
	}
	return slices.ContainsFunc(row.value.Components, func(component domain.ValuationComponent) bool {
		if component.InstrumentID == nil || component.HoldingID == nil || component.NativeCurrency != row.record.Account.DefaultCurrency {
			return false
		}
		h, ok := p.current.holdings[*component.HoldingID]
		return ok && h.ArchivedAt == nil && !h.Quantity.IsZero()
	})
}

func (p *accountsPage) canTradeAccount() bool {
	row := p.freshDetail()
	return p.trade != nil && p.trade.onPosted != nil && row != nil && history.TradeAccount(row.record.Account)
}

func (f *accountTrade) action(side domain.TradeSide) *widget.Clickable {
	if side == domain.TradeSell {
		return &f.sell
	}
	return &f.buy
}

type tradeLabels struct {
	action, review, confirm, posted i18n.MessageID
	quantity, gross, grossFact      i18n.MessageID
	scope, history, empty           i18n.MessageID
	previewFailed, stale            i18n.MessageID
}

var buyLabels = tradeLabels{
	action: i18n.BuyOpen, review: i18n.BuyReview, confirm: i18n.BuyConfirm, posted: i18n.BuyPosted,
	quantity: i18n.BuyQuantity, gross: i18n.BuyGross, grossFact: i18n.BuyGrossFact,
	scope: i18n.BuyScopeUnavailable, history: i18n.BuyHistoryRequired, empty: i18n.BuyNoInstruments,
	previewFailed: i18n.BuyPreviewFailed, stale: i18n.BuyStale,
}

var sellLabels = tradeLabels{
	action: i18n.SellOpen, review: i18n.SellReview, confirm: i18n.SellConfirm, posted: i18n.SellPosted,
	quantity: i18n.SellQuantity, gross: i18n.SellGross, grossFact: i18n.SellGrossFact,
	scope: i18n.SellScopeUnavailable, history: i18n.SellHistoryRequired, empty: i18n.SellNoInstruments,
	previewFailed: i18n.SellPreviewFailed, stale: i18n.SellStale,
}

func (d *accountTradeDraft) labels() tradeLabels {
	if d.side == domain.TradeSell {
		return sellLabels
	}
	return buyLabels
}

func (p *accountsPage) startTrade(side domain.TradeSide) {
	f := p.trade
	f.draft = &accountTradeDraft{side: side, account: p.freshDetail().record.Account,
		quantity: ui.NewTextField(true), gross: ui.NewTextField(true), fee: ui.NewTextField(true)}
	p.loadTradeSources()
}

func (d *accountTradeDraft) chooseInstrument(id domain.InstrumentID) {
	if d.instrument != id {
		d.instrument, d.problem = id, i18n.NoMessage
		d.quantity.Editor.SetText("")
		d.gross.Editor.SetText("")
		d.fee.Editor.SetText("")
	}
}

func (p *accountsPage) loadTradeSources() {
	f, d := p.trade, p.trade.draft
	d.sources, d.sourceProblem = nil, i18n.NoMessage
	id, side := d.account.ID, d.side
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.TradeSources, error) {
		return history.LoadTradeSources(p.ctx, f.service, id, side)
	})
}

func (p *accountsPage) previewTrade() {
	f, d := p.trade, p.trade.draft
	if d.previewing != nil || d.sources == nil || d.instrument == "" {
		return
	}
	input := history.TradeInput{Side: d.side, Account: d.account.ID, Instrument: d.instrument, Quantity: d.quantity.Text(),
		Gross: d.gross.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.ReviewedTrade, error) {
		return history.ReviewTrade(p.ctx, f.service, input)
	})
}

func (p *accountsPage) postTrade() {
	f, d := p.trade, p.trade.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	plan := *d.review
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (domain.ChangePreview, error) {
		return plan.Commit(p.ctx, f.service)
	})
}

func (p *accountsPage) tradeUnresolved() bool {
	return p.trade != nil && p.trade.draft != nil && (p.trade.draft.posting != nil || p.trade.draft.uncertain)
}

func (p *accountsPage) pollTrade() {
	f := p.trade
	if f == nil || f.draft == nil {
		return
	}
	d := f.draft
	labels := d.labels()
	select {
	case result := <-d.loading:
		d.loading = nil
		if result.err != nil {
			d.sourceProblem = tradeProblem(result.err, labels, i18n.BuySourcesFailed)
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
			d.problem = tradeProblem(result.err, labels, labels.previewFailed)
			if d.problem == labels.scope || d.problem == i18n.BuyInstrumentUnavailable {
				p.loadTradeSources()
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
			f.onPosted(result.value.Activity, labels.posted, f.action(d.side))
			return
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// The shared service checks committed receipts before the
			// guard, so this mutation can be discarded on a stale token.
			d.review, d.uncertain, d.problem = nil, false, labels.stale
			p.loadTradeSources()
		} else {
			d.uncertain = true
		}
	default:
	}
}

func tradeProblem(err error, labels tradeLabels, fallback i18n.MessageID) i18n.MessageID {
	var issue *domain.Error
	if !errors.As(err, &issue) {
		return fallback
	}
	switch issue.Field {
	case "settlementCash":
		return i18n.BuyCashRequired
	case "settlementAccountId", "holdings":
		return labels.scope
	case "saleSequence":
		return i18n.SellDateSequence
	}
	switch issue.Code {
	case domain.ErrHistoryNotStarted:
		return labels.history
	case domain.ErrInvalidChangeTime:
		return i18n.ContributionInvalidTime
	case domain.ErrInsufficientBalance:
		return i18n.CashMovementInsufficient
	case domain.ErrInsufficientQuantity:
		return i18n.SellInsufficientQuantity
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
