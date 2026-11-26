package desktop

import (
	"context"
	"strings"

	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioInstrumentConfig struct {
	kind           domain.InstrumentType
	currency       domain.CurrencyCode
	source         domain.QuoteSourceKind
	provider       string
	template, unit string
	icon           string
	providerSymbol ui.TextField
	advanced       bool
	more           widget.Clickable
	typePicker     ui.Select
	currencyPicker ui.Select
	sourcePicker   ui.Select
	providerPicker ui.Select
	templatePicker ui.Select
	unitPicker     ui.Select
	iconPicker     ui.Select
	lookup         portfolioCatalogLookup
	metal          portfolioCatalogMetalPreview
}

func newPortfolioInstrumentConfig(item domain.Instrument) portfolioInstrumentConfig {
	config := portfolioInstrumentConfig{
		kind: item.Type, currency: item.QuoteCurrency, source: item.QuoteSource,
		provider: productString(item.ProviderKey), template: item.MetalTemplate, unit: item.QuantityUnit, icon: productString(item.IconKey),
		providerSymbol: ui.NewTextField(true),
		lookup:         portfolioCatalogLookup{query: ui.NewTextField(true)},
	}
	config.providerSymbol.Editor.SetText(productString(item.ProviderSymbol))
	if item.ID == "" {
		if config.currency == "" {
			config.currency = domain.SupportedCurrencies()[0]
		}
		config.setCreateType(domain.InstrumentStock)
	}
	return config
}

func (c *portfolioInstrumentConfig) setCreateType(kind domain.InstrumentType) {
	c.kind, c.template, c.unit = kind, "", ""
	c.provider = instrumentLookupProvider(kind)
	c.source = domain.QuoteSourceManual
	if c.provider != "" {
		c.source = domain.QuoteSourceProvider
	}
	c.providerSymbol.Editor.SetText("")
	c.retireReads()
}

func (d *portfolioCatalogDraft) setTemplate(template string, label string) {
	c := &d.config
	c.template, c.unit, c.source, c.provider = template, "", domain.QuoteSourceManual, ""
	c.providerSymbol.Editor.SetText("")
	d.fields[portfolioInstrumentMarket].Editor.SetText("")
	if template != "" {
		c.unit, c.source, c.provider = "g", domain.QuoteSourceProvider, application.YahooFinanceProviderKey
		c.providerSymbol.Editor.SetText(domain.MetalProviderSymbol(template))
		d.fields[portfolioInstrumentName].Editor.SetText(label)
		d.fields[portfolioInstrumentMarket].Editor.SetText(domain.MetalFuturesMarket)
		d.fields[portfolioInstrumentSymbol].Editor.SetText("")
		d.fields[portfolioInstrumentCountry].Editor.SetText("")
		d.fields[portfolioInstrumentISIN].Editor.SetText("")
	}
	c.retireReads()
}

func (d *portfolioCatalogDraft) createInput() application.InstrumentInput {
	value := func(i int) string { return strings.TrimSpace(d.fields[i].Text()) }
	c := &d.config
	request := application.InstrumentInput{
		Name: value(portfolioInstrumentName), ShortName: productOptional(value(portfolioInstrumentShortName)),
		Type: string(c.kind), QuoteCurrency: c.currency.String(), QuoteSource: string(c.source),
		MetalTemplate: c.template, QuantityUnit: c.unit, IconKey: c.icon,
		Symbol: value(portfolioInstrumentSymbol), MarketCode: value(portfolioInstrumentMarket),
		CountryCode: value(portfolioInstrumentCountry), ISIN: value(portfolioInstrumentISIN), Note: productOptional(value(portfolioInstrumentNote)),
	}
	if c.source == domain.QuoteSourceProvider || c.template != "" {
		request.ProviderKey, request.ProviderSymbol = c.provider, strings.TrimSpace(c.providerSymbol.Text())
	}
	return request
}

func (d *portfolioCatalogDraft) instrumentPatch() application.InstrumentPatch {
	current, c := d.instrument, &d.config
	patch := d.identityPatch()
	if c.kind != current.Type {
		patch.Type = application.StringField{Op: application.FieldSet, Value: string(c.kind)}
	}
	if c.source != current.QuoteSource {
		patch.QuoteSource = application.StringField{Op: application.FieldSet, Value: string(c.source)}
	}
	if c.source == domain.QuoteSourceProvider && c.template == "" {
		patch.ProviderKey = marketBindingPatch(current.ProviderKey, c.provider)
		patch.ProviderSymbol = marketBindingPatch(current.ProviderSymbol, c.providerSymbol.Text())
	}
	patch.IconKey = marketBindingPatch(current.IconKey, c.icon)
	return patch
}

func (c *portfolioCatalog) cancelDraft() {
	if c.draft != nil {
		c.draft.config.retireReads()
		c.draft = nil
	}
}

func catalogInstrumentValidation(field string) i18n.MessageID {
	switch field {
	case "name":
		return i18n.PortfolioCatalogNameRequired
	case "shortName":
		return i18n.PortfolioCatalogShortNameInvalid
	case "provider", "providerKey", "providerSymbol":
		return i18n.InstrumentBindingRequired
	case "marketCode", "countryCode":
		return i18n.InstrumentBindingRoutingInvalid
	default:
		return i18n.PortfolioCatalogWriteFailed
	}
}

type portfolioCatalogLookup struct {
	query    ui.TextField
	search   widget.Clickable
	request  <-chan historyResult[[]application.InstrumentSearchHit]
	cancel   context.CancelFunc
	searched string
	hits     []application.InstrumentSearchHit
	buttons  []widget.Clickable
	err      error
}

type portfolioCatalogMetalPreview struct {
	load    widget.Clickable
	request <-chan historyResult[application.LatestInstrumentQuote]
	cancel  context.CancelFunc
	current *application.LatestInstrumentQuote
	err     error
}

func (q *portfolioCatalogLookup) reset() {
	if q.cancel != nil {
		q.cancel()
	}
	q.request, q.cancel, q.searched = nil, nil, ""
	q.hits, q.buttons, q.err = nil, nil, nil
}

func (m *portfolioCatalogMetalPreview) reset() {
	if m.cancel != nil {
		m.cancel()
	}
	m.request, m.cancel, m.current, m.err = nil, nil, nil, nil
}

func (c *portfolioInstrumentConfig) retireReads() {
	c.lookup.reset()
	c.metal.reset()
}

func (c *portfolioInstrumentConfig) poll() {
	q := &c.lookup
	select {
	case result := <-q.request:
		q.cancel()
		q.request, q.cancel = nil, nil
		q.hits, q.err = result.value, result.err
		q.buttons = make([]widget.Clickable, len(q.hits))
	default:
	}
	m := &c.metal
	select {
	case result := <-m.request:
		m.reset()
		m.err = result.err
		if result.err == nil {
			m.current = &result.value
		}
	default:
	}
}

func (p *portfolioPage) lookupCatalogInstrument(d *portfolioCatalogDraft, text string) {
	q := &d.config.lookup
	q.reset()
	ctx, cancel := context.WithCancel(p.ctx)
	kind := string(d.config.kind)
	q.cancel, q.searched = cancel, text
	q.request = runHistoryRead(&p.workers, p.invalidate, func() ([]application.InstrumentSearchHit, error) {
		return p.catalog.ledger.SearchInstruments(ctx, text, kind)
	})
}

func (d *portfolioCatalogDraft) chooseCatalogHit(hit application.InstrumentSearchHit) {
	c := &d.config
	c.source, c.provider = domain.QuoteSourceProvider, hit.ProviderKey
	c.providerSymbol.Editor.SetText(hit.ProviderSymbol)
	d.fields[portfolioInstrumentName].Editor.SetText(hit.Name)
	d.fields[portfolioInstrumentSymbol].Editor.SetText(hit.Symbol)
	d.fields[portfolioInstrumentMarket].Editor.SetText(hit.MarketCode)
	d.fields[portfolioInstrumentCountry].Editor.SetText(hit.CountryCode)
	if d.instrument.ID == "" && hit.QuoteCurrency != "" {
		c.currency = domain.CurrencyCode(hit.QuoteCurrency)
	}
	c.lookup.query.Editor.SetText(instrumentBindingHitLabel(hit))
	c.lookup.reset()
}

func (p *portfolioPage) previewCatalogMetal(d *portfolioCatalogDraft) {
	c := &d.config
	m := &c.metal
	m.reset()
	template, unit, currency := c.template, c.unit, c.currency.String()
	ctx, cancel := context.WithCancel(p.ctx)
	m.cancel = cancel
	m.request = runHistoryRead(&p.workers, p.invalidate, func() (application.LatestInstrumentQuote, error) {
		quote, _, err := p.catalog.ledger.PreviewMetalQuote(ctx, template, unit, currency)
		return quote, err
	})
}
