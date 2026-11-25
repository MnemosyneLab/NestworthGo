package desktop

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountInvestment struct {
	service                                      history.InvestmentLedger
	buy, sell, dividend, importOpen, custodyOpen widget.Clickable
	custodyHoldings                              map[domain.HoldingID]*widget.Clickable
	restore                                      *widget.Clickable
	draft                                        *accountInvestmentDraft
	onPosted                                     func(domain.Activity, i18n.MessageID, event.Tag)
}

type investmentRuntime struct {
	ctx        context.Context
	workers    *sync.WaitGroup
	clock      func() time.Time
	invalidate func()
}

func (p *accountsPage) investmentRuntime() investmentRuntime {
	return investmentRuntime{ctx: p.ctx, workers: &p.workers, clock: p.clock, invalidate: p.invalidate}
}

type accountInvestmentDraft struct {
	kind                                 domain.ActivityKind
	custody                              *custodyTransferDraft
	existing                             bool
	account                              domain.Account
	instrument                           domain.InstrumentID
	currency                             domain.CurrencyCode
	quantity, gross, fee, unitCost, note ui.TextField
	day, time                            string
	defaultTime                          time.Time
	selectInstrument                     ui.Select
	selectAccount                        ui.Select
	accounts                             []domain.Account
	pickAccount, focusAccount            bool
	datePicker                           ui.DatePicker
	timePicker                           ui.TimePicker
	cancel, retry, preview, edit         widget.Clickable
	confirm, retryConfirm                widget.Clickable
	useNow                               widget.Clickable
	sourceProblem, problem               i18n.MessageID
	sources                              *history.InvestmentSources
	loading                              <-chan historyResult[history.InvestmentSources]
	previewing                           <-chan historyResult[history.ReviewedInvestment]
	review                               *history.ReviewedInvestment
	posting                              <-chan historyResult[domain.ChangePreview]
	uncertain, focusReview, focusInput   bool
}

func (p *accountsPage) canFirstBuy() bool {
	return p.canTradeAccount() && len(p.current.holdings) == 0
}

func (p *accountsPage) canSell() bool {
	if !p.canTradeAccount() {
		return false
	}
	for _, holding := range p.current.holdings {
		if holding.ArchivedAt == nil && !holding.Quantity.IsZero() && !p.current.managed[holding.InstrumentID] {
			return true
		}
	}
	return false
}

func (p *accountsPage) canRecordDividend() bool {
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
		if p.current.managed[*component.InstrumentID] {
			return false
		}
		h, ok := p.current.holdings[*component.HoldingID]
		return ok && h.ArchivedAt == nil
	})
}

func (p *accountsPage) canTradeAccount() bool {
	row := p.freshDetail()
	return p.investment != nil && p.investment.onPosted != nil && row != nil && history.TradeAccount(row.record.Account)
}

func (p *accountsPage) canImportPosition() bool {
	return p.canTradeAccount() && !p.current.beforeHistory
}

func (f *accountInvestment) custodyHoldingAction(tag event.Tag) bool {
	for _, button := range f.custodyHoldings {
		if tag == button {
			return true
		}
	}
	return false
}

func (f *accountInvestment) action(d *accountInvestmentDraft) *widget.Clickable {
	if d.custody != nil {
		if d.custody.scope.action != nil {
			return d.custody.scope.action
		}
		return &f.custodyOpen
	}
	switch d.kind {
	case domain.ActivitySell:
		return &f.sell
	case domain.ActivityCashDividend:
		return &f.dividend
	case domain.ActivityPositionTransfer:
		return &f.importOpen
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

var positionImportLabels = tradeLabels{
	action: i18n.OpeningPositionOpen, review: i18n.OpeningPositionReview, confirm: i18n.OpeningPositionConfirm, posted: i18n.PositionImportPosted,
	quantity: i18n.OpeningPositionQuantity, scope: i18n.OpeningPositionUnavailable, history: i18n.PositionImportHistoryRequired, empty: i18n.OpeningPositionEmpty,
	previewFailed: i18n.PositionImportReviewFailed, stale: i18n.PositionImportStale,
}

func (d *accountInvestmentDraft) labels() tradeLabels {
	if d.custody != nil {
		return custodyTransferLabels
	}
	switch d.kind {
	case domain.ActivitySell:
		return sellLabels
	case domain.ActivityCashDividend:
		return dividendLabels
	case domain.ActivityPositionTransfer:
		return positionImportLabels
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
	f.draft = newAccountInvestmentDraft(kind, p.freshDetail().record.Account)
	f.draft.existing = !p.canFirstBuy()
	f.loadSources(p.investmentRuntime())
}

func newAccountInvestmentDraft(kind domain.ActivityKind, account domain.Account) *accountInvestmentDraft {
	return &accountInvestmentDraft{kind: kind, account: account, focusInput: true,
		quantity: ui.NewTextField(true), gross: ui.NewTextField(true), fee: ui.NewTextField(true), unitCost: ui.NewTextField(true), note: ui.NewTextField(true)}
}

func (d *accountInvestmentDraft) chooseInstrument(id domain.InstrumentID) {
	var currency domain.CurrencyCode
	if d.sources != nil {
		for _, instrument := range d.sources.Instruments {
			if instrument.ID == id {
				currency = instrument.QuoteCurrency
				break
			}
		}
	}
	if d.instrument != id || d.currency != currency {
		d.instrument, d.currency, d.problem = id, currency, i18n.NoMessage
		d.quantity.Editor.SetText("")
		d.gross.Editor.SetText("")
		d.fee.Editor.SetText("")
		d.unitCost.Editor.SetText("")
	}
}

func (d *accountInvestmentDraft) captureTime(now time.Time) {
	now = now.In(d.sources.Location)
	d.day, d.time = now.Format(time.DateOnly), now.Format("15:04")
	if d.kind != domain.ActivityCashDividend {
		d.time, d.defaultTime = now.Format(time.TimeOnly), now
	}
}

func (f *accountInvestment) loadSources(run investmentRuntime) {
	d := f.draft
	d.sources, d.sourceProblem = nil, i18n.NoMessage
	if d.custody != nil {
		f.loadCustodySources(run)
		return
	}
	id, kind := d.account.ID, d.kind
	d.loading = runHistoryRead(run.workers, run.invalidate, func() (history.InvestmentSources, error) {
		return history.LoadInvestmentSources(run.ctx, f.service, id, kind)
	})
}

func (f *accountInvestment) preview(run investmentRuntime) {
	d := f.draft
	if d.custody != nil {
		f.previewCustody(run)
		return
	}
	if d.previewing != nil || d.sources == nil || d.instrument == "" {
		return
	}
	kind := d.kind
	side := domain.TradeBuy
	if kind == domain.ActivitySell {
		side = domain.TradeSell
	}
	input := history.TradeInput{Side: side, Account: d.account.ID, Instrument: d.instrument,
		Holding: d.sources.Holdings[d.instrument].ID, Currency: d.currency, Quantity: d.quantity.Text(),
		Gross: d.gross.Text(), Fee: d.fee.Text(), Note: d.note.Text(), Date: d.day, Time: d.time}
	// Keep the actual default instant, including its sub-second position
	// after History Origin. Edited wall times resolve in the Origin timezone.
	if d.day == d.defaultTime.Format(time.DateOnly) && d.time == d.defaultTime.Format(time.TimeOnly) {
		input.EffectiveAt = d.defaultTime
	}
	unitCost := d.unitCost.Text()
	d.problem = i18n.NoMessage
	d.previewing = runHistoryRead(run.workers, run.invalidate, func() (history.ReviewedInvestment, error) {
		switch kind {
		case domain.ActivityCashDividend:
			return history.ReviewDividend(run.ctx, f.service, history.DividendInput{
				Account: input.Account, Instrument: input.Instrument, Holding: input.Holding,
				Amount: input.Gross, Date: input.Date, Time: input.Time,
			})
		case domain.ActivityPositionTransfer:
			return history.ReviewPositionImport(run.ctx, f.service, history.PositionImportInput{
				Account: input.Account, Instrument: input.Instrument, Currency: input.Currency, Quantity: input.Quantity,
				UnitCost: unitCost, Note: input.Note, Date: input.Date, Time: input.Time, EffectiveAt: input.EffectiveAt,
			})
		}
		return history.ReviewTrade(run.ctx, f.service, input)
	})
}

func (f *accountInvestment) post(run investmentRuntime) {
	d := f.draft
	if d.review == nil || d.posting != nil || run.ctx.Err() != nil {
		return
	}
	plan := *d.review
	d.posting = runHistoryRead(run.workers, run.invalidate, func() (domain.ChangePreview, error) {
		return plan.Commit(run.ctx, f.service)
	})
}

func (f *accountInvestment) unresolved() bool {
	return f != nil && f.draft != nil && (f.draft.posting != nil || f.draft.uncertain)
}

func (f *accountInvestment) poll(run investmentRuntime) {
	if f == nil || f.draft == nil {
		return
	}
	d := f.draft
	labels := d.labels()
	f.pollCustodySources(run)
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
				d.captureTime(run.clock())
			}
			if d.instrument != "" && !slices.ContainsFunc(result.value.Instruments, func(i domain.Instrument) bool { return i.ID == d.instrument }) {
				d.chooseInstrument("")
				d.problem = i18n.BuyInstrumentUnavailable
			}
			if d.instrument != "" {
				old := d.currency
				d.chooseInstrument(d.instrument)
				if d.currency != old {
					d.problem = i18n.TradeCurrencyChanged
				}
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
			if d.problem == labels.scope || d.problem == i18n.BuyInstrumentUnavailable || d.problem == i18n.TradeCurrencyChanged {
				f.loadSources(run)
			}
		} else {
			d.review, d.focusReview = &result.value, true
		}
	default:
	}
	select {
	case result := <-d.posting:
		d.posting = nil
		if result.err == nil && result.value.Activity.ID != "" {
			f.draft = nil
			f.onPosted(result.value.Activity, labels.posted, f.action(d))
			return
		}
		var issue *domain.Error
		if errors.As(result.err, &issue) && issue.Code == domain.ErrStalePreview {
			// The shared service checks committed receipts before the
			// guard, so this mutation can be discarded on a stale token.
			d.review, d.uncertain, d.problem = nil, false, labels.stale
			d.focusInput = true
			f.loadSources(run)
		} else {
			d.uncertain, d.focusReview = true, true
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
	case "grossCurrency", "currency":
		return i18n.TradeCurrencyChanged
	case "settlementAccountId", "holdings", "holdingId", "fromHoldingId", "toHoldingId", "toAccountId":
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
	case "unitCost":
		return i18n.PositionImportInvalidCost
	case "note":
		return i18n.HistoryRepeatNoteTooLong
	case "instrumentId":
		return i18n.BuyInstrumentUnavailable
	case "amount":
		return i18n.DividendInvalidAmount
	}
	return fallback
}
