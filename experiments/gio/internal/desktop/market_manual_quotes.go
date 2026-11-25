package desktop

import (
	"context"
	"errors"
	"strings"
	"time"

	"gioui.org/io/event"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketQuoteWriter interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	AppendManualInstrumentQuote(context.Context, domain.InstrumentID, string, string, bool) (domain.InstrumentQuote, error)
	AppendManualFXQuoteWithReceipt(context.Context, string, string, string, string) (domain.FXQuote, error)
}

type marketManualTarget struct {
	instrumentID domain.InstrumentID
	base, quote  domain.CurrencyCode
	date         string
}

type marketManualQuotes struct {
	service marketQuoteWriter
	clock   func() time.Time
	draft   *marketManualDraft
	latest  marketManualReceipt
	addFX   widget.Clickable
	prices  map[string]*widget.Clickable
}

type marketManualReferences struct {
	instrument *domain.Instrument
	location   *time.Location
}

type marketManualDraft struct {
	target                  marketManualTarget
	source                  event.Tag
	sheet                   analysisSheet
	overlay                 ui.Overlay
	basePicker, quotePicker ui.Select
	datePicker              ui.DatePicker
	amount                  ui.TextField
	base, quote             domain.CurrencyCode
	date                    string
	refs                    *marketManualReferences
	lookup                  <-chan historyResult[marketManualReferences]
	stopLookup              context.CancelFunc
	lookupErr               error
	saving                  <-chan historyResult[marketManualReceipt]
	problem                 i18n.MessageID
}

type marketManualReceipt struct {
	instrument domain.InstrumentQuote
	fx         domain.FXQuote
	name       string
	partial    bool
}

func (r marketManualReceipt) saved() bool { return r.instrument.ID != "" || r.fx.ID != "" }

func manualPriceAvailable(instrument domain.Instrument) bool {
	return instrument.ID != "" && instrument.ArchivedAt == nil && instrument.QuoteSource != domain.QuoteSourceAgent
}

func (p *marketDataPage) openManualQuote(target marketManualTarget, source event.Tag) {
	m := &p.manual
	if m.service == nil || m.draft != nil {
		return
	}
	base, quote := target.base, target.quote
	if target.instrumentID == "" && base == "" && quote == "" {
		codes := domain.SupportedCurrencies()
		base, quote = codes[0], codes[1]
	}
	m.draft = &marketManualDraft{target: target, source: source, sheet: newAnalysisSheet(),
		amount: ui.NewTextField(true), base: base, quote: quote, date: target.date}
	p.loadManualQuoteReferences()
}

func (p *marketDataPage) loadManualQuoteReferences() {
	draft := p.manual.draft
	ctx, cancel := context.WithCancel(p.ctx)
	draft.stopLookup, draft.lookupErr = cancel, nil
	target := draft.target
	draft.lookup = runHistoryRead(&p.workers, p.invalidate, func() (marketManualReferences, error) {
		origin, err := p.manual.service.HistoryOrigin(ctx)
		if err != nil {
			return marketManualReferences{}, err
		}
		refs := marketManualReferences{location: time.UTC}
		if origin != nil {
			refs.location, err = time.LoadLocation(origin.Timezone)
			if err != nil {
				return marketManualReferences{}, err
			}
		}
		if target.instrumentID != "" {
			all, err := p.reader.ListInstruments(ctx, true)
			if err != nil {
				return refs, err
			}
			for _, instrument := range all {
				if instrument.ID != target.instrumentID || !manualPriceAvailable(instrument) {
					continue
				}
				owned, err := p.reader.InstrumentProductOwnership(ctx)
				if err != nil {
					return refs, err
				}
				if owned[instrument.ID.String()] == "" {
					refs.instrument = &instrument
				}
				break
			}
		}
		return refs, nil
	})
}

func (d *marketManualDraft) available() bool {
	return d.refs != nil && (d.target.instrumentID == "" || d.refs.instrument != nil)
}

func (d *marketManualDraft) validPair() bool {
	return d.target.instrumentID != "" || d.base != "" && d.quote != "" && d.base != d.quote
}

func (d *marketManualDraft) cancelRead() {
	if d.stopLookup != nil {
		d.stopLookup()
		d.stopLookup = nil
	}
	d.lookup = nil
}

func (d *marketManualDraft) validate(today string) i18n.MessageID {
	if d.date != "" {
		if _, valid := ui.ParseCivilDay(d.date); !valid || d.date > today {
			return i18n.MarketManualInvalidDate
		}
	}
	amount := strings.TrimSpace(d.amount.Text())
	if d.target.instrumentID != "" {
		if _, err := domain.ParseUnitPrice(amount); err != nil {
			return i18n.MarketManualInvalidPrice
		}
	} else {
		if !d.validPair() {
			return i18n.MarketManualInvalidPair
		}
		if _, err := domain.ParseFxRate(amount); err != nil {
			return i18n.MarketManualInvalidRate
		}
	}
	return i18n.NoMessage
}

func (p *marketDataPage) saveManualQuote() {
	m, d := &p.manual, p.manual.draft
	if d == nil || !d.available() || d.saving != nil || strings.TrimSpace(d.amount.Text()) == "" {
		return
	}
	today := m.clock().In(d.refs.location).Format(time.DateOnly)
	d.problem = d.validate(today)
	if d.problem != i18n.NoMessage {
		return
	}
	amount, date, base, currency := strings.TrimSpace(d.amount.Text()), d.date, d.base.String(), d.quote.String()
	instrument := d.refs.instrument
	d.saving = runHistoryRead(&p.workers, p.invalidate, func() (marketManualReceipt, error) {
		if instrument != nil {
			quote, err := m.service.AppendManualInstrumentQuote(p.ctx, instrument.ID, amount, date, false)
			return marketManualReceipt{instrument: quote, name: instrument.DisplayLabel()}, err
		}
		quote, err := m.service.AppendManualFXQuoteWithReceipt(p.ctx, base, currency, amount, date)
		return marketManualReceipt{fx: quote, partial: err != nil && quote.ID != ""}, err
	})
}

func manualQuoteError(err error) i18n.MessageID {
	var detail *domain.Error
	if errors.As(err, &detail) {
		switch detail.Field {
		case "quotedAt", "localDate":
			return i18n.MarketManualInvalidDate
		case "unitPrice":
			return i18n.MarketManualInvalidPrice
		case "fxRate":
			return i18n.MarketManualInvalidRate
		case "currencyPair":
			return i18n.MarketManualInvalidPair
		}
	}
	return i18n.MarketManualSaveFailed
}
