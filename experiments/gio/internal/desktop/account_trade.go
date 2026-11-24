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

type accountInvestment struct {
	service             history.InvestmentLedger
	buy, sell, dividend widget.Clickable
	draft               *accountInvestmentDraft
	onPosted            func(domain.Activity, i18n.MessageID, event.Tag)
}

type accountInvestmentDraft struct {
	kind                         domain.ActivityKind
	existing                     bool
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
	sources                      *history.InvestmentSources
	loading                      <-chan historyResult[history.InvestmentSources]
	previewing                   <-chan historyResult[history.ReviewedInvestment]
	review                       *history.ReviewedInvestment
	posting                      <-chan historyResult[domain.ChangePreview]
	uncertain                    bool
}

func (p *accountsPage) canFirstBuy() bool {
	return p.canTradeAccount() && len(p.current.holdings) == 0
}

func (p *accountsPage) canSell() bool {
	return p.hasInvestmentHolding(domain.ActivitySell)
}

func (p *accountsPage) hasInvestmentHolding(kind domain.ActivityKind) bool {
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
		return ok && h.ArchivedAt == nil && (kind != domain.ActivitySell || !h.Quantity.IsZero())
	})
}

func (p *accountsPage) canTradeAccount() bool {
	row := p.freshDetail()
	return p.investment != nil && p.investment.onPosted != nil && row != nil && history.TradeAccount(row.record.Account)
}

func (f *accountInvestment) action(kind domain.ActivityKind) *widget.Clickable {
	switch kind {
	case domain.ActivitySell:
		return &f.sell
	case domain.ActivityCashDividend:
		return &f.dividend
	default:
		return &f.buy
	}
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

var dividendLabels = tradeLabels{
	action: i18n.DividendOpen, review: i18n.DividendReview, confirm: i18n.DividendConfirm, posted: i18n.DividendPosted,
	scope: i18n.DividendHoldingUnavailable, history: i18n.DividendHistoryRequired, empty: i18n.DividendNoHoldings,
	previewFailed: i18n.DividendPreviewFailed, stale: i18n.DividendStale,
}

func (d *accountInvestmentDraft) labels() tradeLabels {
	switch d.kind {
	case domain.ActivitySell:
		return sellLabels
	case domain.ActivityCashDividend:
		return dividendLabels
	}
	labels := buyLabels
	if d.existing {
		labels.action = i18n.BuyExistingOpen
		labels.scope = i18n.BuyExistingScope
	}
	return labels
}

func (p *accountsPage) startInvestment(kind domain.ActivityKind) {
	f := p.investment
	f.draft = &accountInvestmentDraft{kind: kind, existing: !p.canFirstBuy(), account: p.freshDetail().record.Account,
		quantity: ui.NewTextField(true), gross: ui.NewTextField(true), fee: ui.NewTextField(true)}
	p.loadInvestmentSources()
}

func (d *accountInvestmentDraft) chooseInstrument(id domain.InstrumentID) {
	if d.instrument != id {
		d.instrument, d.problem = id, i18n.NoMessage
		d.quantity.Editor.SetText("")
		d.gross.Editor.SetText("")
		d.fee.Editor.SetText("")
	}
}

func (p *accountsPage) loadInvestmentSources() {
	f, d := p.investment, p.investment.draft
	d.sources, d.sourceProblem = nil, i18n.NoMessage
	id, kind := d.account.ID, d.kind
	d.loading = runHistoryRead(&p.workers, p.invalidate, func() (history.InvestmentSources, error) {
		return history.LoadInvestmentSources(p.ctx, f.service, id, kind)
	})
}

func (p *accountsPage) previewInvestment() {
	f, d := p.investment, p.investment.draft
	if d.previewing != nil || d.sources == nil || d.instrument == "" {
		return
	}
	kind := d.kind
	side := domain.TradeBuy
	if kind == domain.ActivitySell {
		side = domain.TradeSell
	}
	input := history.TradeInput{Side: side, Account: d.account.ID, Instrument: d.instrument,
		Holding: d.sources.Holdings[d.instrument].ID, Quantity: d.quantity.Text(),
		Gross: d.gross.Text(), Fee: d.fee.Text(), Date: d.day, Time: d.time}
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(&p.workers, p.invalidate, func() (history.ReviewedInvestment, error) {
		if kind == domain.ActivityCashDividend {
			return history.ReviewDividend(p.ctx, f.service, history.DividendInput{
				Account: input.Account, Instrument: input.Instrument, Holding: input.Holding,
				Amount: input.Gross, Date: input.Date, Time: input.Time,
			})
		}
		return history.ReviewTrade(p.ctx, f.service, input)
	})
}

func (p *accountsPage) postInvestment() {
	f, d := p.investment, p.investment.draft
	if d.review == nil || d.posting != nil || p.ctx.Err() != nil {
		return
	}
	plan := *d.review
	d.posting = runHistoryRead(&p.workers, p.invalidate, func() (domain.ChangePreview, error) {
		return plan.Commit(p.ctx, f.service)
	})
}

func (p *accountsPage) investmentUnresolved() bool {
	return p.investment != nil && p.investment.draft != nil && (p.investment.draft.posting != nil || p.investment.draft.uncertain)
}

func (p *accountsPage) pollInvestment() {
	f := p.investment
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
			d.existing = len(result.value.Holdings) > 0
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
				p.loadInvestmentSources()
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
			f.onPosted(result.value.Activity, labels.posted, f.action(d.kind))
			return
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// The shared service checks committed receipts before the
			// guard, so this mutation can be discarded on a stale token.
			d.review, d.uncertain, d.problem = nil, false, labels.stale
			p.loadInvestmentSources()
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
	case "settlementAccountId", "holdings", "holdingId":
		return labels.scope
	case "saleSequence":
		return i18n.SellDateSequence
	case "buySequence":
		return i18n.BuyDateSequence
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
	case "amount":
		return i18n.DividendInvalidAmount
	}
	return fallback
}
