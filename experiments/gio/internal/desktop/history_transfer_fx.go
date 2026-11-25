package desktop

import (
	"context"
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/settings"
)

type cashMovementService interface {
	history.CashMovementLedger
	CurrentFXQuote(context.Context, domain.CurrencyCode, domain.CurrencyCode) (*domain.FXQuote, error)
}

type transferPair struct {
	from, to domain.CurrencyCode
}

type transferQuote struct {
	pair       transferPair
	value      *domain.FXQuote
	pending    <-chan historyResult[*domain.FXQuote]
	cancel     context.CancelFunc
	loadFailed bool

	calculate, reload widget.Clickable
	lastInputs        [2]string
	problem           i18n.MessageID
}

func (d *cashMovementDraft) transferQuotePair() transferPair {
	if d.movement != history.CashTransfer || d.currency == "" || d.toCurrency == "" ||
		d.currency == d.toCurrency || !d.canSelectDestination(d.destination) {
		return transferPair{}
	}
	return transferPair{d.currency, d.toCurrency}
}

func (p *historyCashMovements) syncTransferQuote() {
	d := p.draft
	q := &d.fx
	pair := d.transferQuotePair()
	if q.pair == pair {
		return
	}
	q.stop()
	q.pair, q.value, q.pending, q.loadFailed, q.problem = pair, nil, nil, false, i18n.NoMessage
	if pair != (transferPair{}) {
		p.readTransferQuote()
	}
}

func (p *historyCashMovements) readTransferQuote() {
	q := &p.draft.fx
	pair := q.pair
	ctx, cancel := context.WithCancel(p.ctx)
	q.loadFailed, q.cancel = false, cancel
	q.pending = runHistoryRead(&p.workers, p.invalidate, func() (*domain.FXQuote, error) {
		// The Wails calculator uses the same current local observation for
		// the currency pair, independent of the selected posting day.
		return p.service.CurrentFXQuote(ctx, pair.from, pair.to)
	})
}

func (q *transferQuote) stop() {
	if q.cancel != nil {
		q.cancel()
		q.cancel = nil
	}
}

func (q *transferQuote) poll() {
	select {
	case result := <-q.pending:
		q.stop()
		q.pending, q.value, q.loadFailed = nil, result.value, result.err != nil
	default:
	}
}

func (d *cashMovementDraft) fillFromTransferQuote() {
	q := &d.fx
	if q.value == nil || q.loadFailed || q.pair != d.transferQuotePair() {
		return
	}
	raw := [2]string{d.amount.Text(), d.received.Text()}
	q.lastInputs, q.problem = raw, i18n.NoMessage
	read := func(text string, code domain.CurrencyCode) (domain.Money, error) {
		if strings.TrimSpace(text) == "" {
			text = "0"
		}
		return domain.ParseMoney(strings.TrimSpace(text), code)
	}
	sent, sentErr := read(raw[0], q.pair.from)
	received, receivedErr := read(raw[1], q.pair.to)
	switch {
	case sentErr != nil || receivedErr != nil:
		q.problem = i18n.TransferQuoteInput
	case sent.IsZero() && received.IsZero():
		q.problem = i18n.TransferQuoteOneAmount
	case !sent.IsZero() && !received.IsZero():
		q.problem = i18n.TransferQuoteBothAmounts
	default:
		input, target, editor := sent, q.pair.to, &d.received.Editor
		if sent.IsZero() {
			input, target, editor = received, q.pair.from, &d.amount.Editor
		}
		suggested, err := history.TransferFXAmount(input, target, *q.value)
		if err != nil {
			q.problem = i18n.TransferQuoteAmountUnavailable
			return
		}
		editor.SetText(suggested.CanonicalAmount())
		d.problem = i18n.NoMessage
	}
}

var transferQuoteSource = map[domain.QuoteSourceKind]i18n.MessageID{
	domain.QuoteSourceManual:   i18n.TransferQuoteManual,
	domain.QuoteSourceProvider: i18n.TransferQuoteProvider,
	domain.QuoteSourceAgent:    i18n.TransferQuoteAgent,
}

func (p *historyCashMovements) layoutTransferQuote(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.draft, view.Language
	q := &d.fx
	var rows []layout.FlexChild
	switch {
	case q.pending != nil:
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.TransferQuoteLoading), view.Colors.SecondaryInk)))
	case q.loadFailed:
		if q.reload.Clicked(gtx) {
			p.readTransferQuote()
		}
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.TransferQuoteFailed), view.Colors.ErrorInk)),
			layout.Rigid(view.Button(&q.reload, l.Text(i18n.TransferQuoteRetry))))
	case q.value == nil:
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.TransferQuoteMissing), view.Colors.SecondaryInk)))
	default:
		quote := *q.value
		rate := fmt.Sprintf(l.Text(i18n.TransferQuoteRate), quote.BaseCurrency, quote.Rate.Canonical(), quote.QuoteCurrency)
		rows = append(rows, layout.Rigid(view.Message(rate, view.Colors.Ink)),
			layout.Rigid(view.Message(transferQuoteEvidence(view, quote), view.Colors.SecondaryInk)),
			layout.Rigid(view.Button(&q.calculate, l.Text(i18n.TransferQuoteCalculate))))
		if q.calculate.Clicked(gtx) {
			d.fillFromTransferQuote()
			gtx.Execute(op.InvalidateCmd{})
		}
		if q.problem != i18n.NoMessage && q.lastInputs == ([2]string{d.amount.Text(), d.received.Text()}) {
			rows = append(rows, layout.Rigid(view.Message(l.Text(q.problem), view.Colors.SecondaryInk)))
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, rows...)
}

func transferQuoteEvidence(view ui.View, quote domain.FXQuote) string {
	source := view.Language.Text(transferQuoteSource[quote.SourceKind])
	if quote.SourceKey != "" && quote.SourceKey != string(quote.SourceKind) {
		source += " · " + quote.SourceKey
	}
	if day, ok := ui.ParseCivilDay(quote.EffectiveDate); ok && quote.TimestampBasis == "date_label" {
		return source + " · " + ui.FormatDay(day, view.Language, view.DateFormat)
	}
	at := format.LocalDateTime(quote.QuotedAt.In(view.TimeLocation), settings.Settings{
		DateFormat: view.DateFormat, TimeFormat: view.TimeFormat,
	}, settings.Language(view.Language))
	return source + " · " + at
}
