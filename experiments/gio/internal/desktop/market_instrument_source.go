package desktop

import (
	"context"
	"errors"
	"strings"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketInstrumentSourceService interface {
	SearchInstruments(context.Context, string, string) ([]application.InstrumentSearchHit, error)
	PatchInstrument(context.Context, domain.InstrumentID, application.InstrumentPatch) (domain.Instrument, error)
}

type marketInstrumentSourceDraft struct {
	id           domain.InstrumentID
	name         string
	from         event.Tag
	sheet        analysisSheet
	instrument   *domain.Instrument
	source       domain.QuoteSourceKind
	provider     string
	query        ui.TextField
	symbol       ui.TextField
	market       ui.TextField
	country      ui.TextField
	sources      [len(marketQuoteSourceChoices)]widget.Clickable
	providers    [2]widget.Clickable
	searchButton widget.Clickable

	refs        <-chan historyResult[*domain.Instrument]
	stopRefs    context.CancelFunc
	refsErr     error
	looking     <-chan historyResult[[]application.InstrumentSearchHit]
	stopLookup  context.CancelFunc
	searched    string
	hits        []application.InstrumentSearchHit
	hitButtons  []widget.Clickable
	lookupErr   error
	picked      string
	saving      <-chan historyResult[domain.Instrument]
	saveProblem i18n.MessageID
}

func (p *marketDataPage) openInstrumentSource(id domain.InstrumentID, name string, from event.Tag) {
	p.instrumentSource = &marketInstrumentSourceDraft{
		id: id, name: name, from: from, sheet: newAnalysisSheet(),
		query: ui.NewTextField(true), symbol: ui.NewTextField(true), market: ui.NewTextField(true), country: ui.NewTextField(true),
	}
	p.readInstrumentSource()
}

func (p *marketDataPage) readInstrumentSource() {
	d := p.instrumentSource
	ctx, cancel := context.WithCancel(p.ctx)
	d.stopRefs, d.refsErr = cancel, nil
	id := d.id
	d.refs = runHistoryRead(&p.workers, p.invalidate, func() (*domain.Instrument, error) {
		rows, err := p.reader.ListInstruments(ctx, true)
		if err != nil {
			return nil, err
		}
		for _, instrument := range rows {
			if instrument.ID != id {
				continue
			}
			owned, err := p.reader.InstrumentProductOwnership(ctx)
			if err != nil || owned[id.String()] != "" {
				return nil, err
			}
			return &instrument, nil
		}
		return nil, nil
	})
}

func (d *marketInstrumentSourceDraft) setInstrument(instrument *domain.Instrument) {
	d.instrument = instrument
	if instrument == nil {
		return
	}
	d.name, d.source, d.provider = instrument.DisplayLabel(), instrument.QuoteSource, marketBindingText(instrument.ProviderKey)
	d.symbol.Editor.SetText(marketBindingText(instrument.ProviderSymbol))
	d.market.Editor.SetText(marketBindingText(instrument.MarketCode))
	d.country.Editor.SetText(marketBindingText(instrument.CountryCode))
}

func marketBindingText(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (d *marketInstrumentSourceDraft) resetLookup() {
	if d.stopLookup != nil {
		d.stopLookup()
	}
	d.stopLookup, d.looking = nil, nil
	d.searched, d.lookupErr = "", nil
	d.hits, d.hitButtons = nil, nil
}

func (d *marketInstrumentSourceDraft) lookupProvider() string {
	switch d.instrument.Type {
	case domain.InstrumentStock, domain.InstrumentETF:
		return application.YahooFinanceProviderKey
	case domain.InstrumentCrypto:
		return application.CoinGeckoProviderKey
	}
	return ""
}

func (p *marketDataPage) lookupInstrumentSource() {
	d := p.instrumentSource
	d.resetLookup()
	d.searched = strings.TrimSpace(d.query.Text())
	query, kind := d.searched, string(d.instrument.Type)
	ctx, cancel := context.WithCancel(p.ctx)
	d.stopLookup = cancel
	d.looking = runHistoryRead(&p.workers, p.invalidate, func() ([]application.InstrumentSearchHit, error) {
		return p.instrumentSources.SearchInstruments(ctx, query, kind)
	})
}

func (d *marketInstrumentSourceDraft) canChoose(hit application.InstrumentSearchHit) bool {
	return hit.ProviderKey == d.lookupProvider() && strings.TrimSpace(hit.ProviderSymbol) != "" &&
		(hit.QuoteCurrency == "" || strings.EqualFold(hit.QuoteCurrency, d.instrument.QuoteCurrency.String())) &&
		(hit.Type == "" || hit.Type == string(d.instrument.Type))
}

func (d *marketInstrumentSourceDraft) selectHit(hit application.InstrumentSearchHit) {
	d.provider = hit.ProviderKey
	d.symbol.Editor.SetText(hit.ProviderSymbol)
	if hit.MarketCode != "" {
		d.market.Editor.SetText(hit.MarketCode)
	}
	if hit.CountryCode != "" {
		d.country.Editor.SetText(hit.CountryCode)
	}
	d.picked, d.saveProblem = instrumentBindingHitLabel(hit), i18n.NoMessage
	d.resetLookup()
}

func (d *marketInstrumentSourceDraft) changeProvider(provider string) {
	if provider != d.provider {
		d.provider, d.picked, d.saveProblem = provider, "", i18n.NoMessage
		d.symbol.Editor.SetText("")
	}
}

func (d *marketInstrumentSourceDraft) patch() application.InstrumentPatch {
	i := d.instrument
	patch := application.InstrumentPatch{}
	if d.source != i.QuoteSource {
		patch.QuoteSource = application.StringField{Op: application.FieldSet, Value: string(d.source)}
	}
	if d.source == domain.QuoteSourceProvider && !i.UsesMetalConversion() {
		patch.ProviderKey = marketBindingPatch(i.ProviderKey, d.provider)
		patch.ProviderSymbol = marketBindingPatch(i.ProviderSymbol, d.symbol.Text())
		patch.MarketCode = marketBindingPatch(i.MarketCode, d.market.Text())
		patch.CountryCode = marketBindingPatch(i.CountryCode, d.country.Text())
	}
	return patch
}

func marketBindingPatch(old *string, value string) application.StringField {
	value = strings.TrimSpace(value)
	if marketBindingText(old) == value {
		return application.StringField{}
	}
	if value == "" {
		return application.StringField{Op: application.FieldClear}
	}
	return application.StringField{Op: application.FieldSet, Value: value}
}

func (d *marketInstrumentSourceDraft) validBinding() bool {
	return d.source != domain.QuoteSourceProvider || d.instrument.UsesMetalConversion() ||
		(d.provider != "" && strings.TrimSpace(d.symbol.Text()) != "")
}

func instrumentBindingSaveError(err error) i18n.MessageID {
	var problem *domain.Error
	if errors.As(err, &problem) && problem.Code == domain.ErrValidation {
		switch problem.Field {
		case "provider", "providerKey", "providerSymbol":
			return i18n.InstrumentBindingRequired
		case "marketCode", "countryCode":
			return i18n.InstrumentBindingRoutingInvalid
		}
	}
	return i18n.InstrumentBindingSaveFailed
}
