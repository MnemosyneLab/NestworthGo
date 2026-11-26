package desktop

import (
	"fmt"
	"image"
	"slices"
	"strings"
	"unicode/utf8"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const catalogCryptoDefaultCurrency domain.CurrencyCode = "USD"

func (p *portfolioPage) layoutCatalogEditor(gtx layout.Context, view ui.View, d *portfolioCatalogDraft) layout.Dimensions {
	c, l := &d.config, view.Language
	create := d.instrument.ID == ""
	if c.more.Clicked(gtx) {
		c.advanced = !c.advanced
		if !c.advanced {
			c.retireReads()
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	var rows []ui.ColumnChild
	height := 0
	appendRow := func(field analysisFilterField) {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			placed := view
			placed.Origin.Y += height
			dims := field(gtx, placed)
			height += dims.Size.Y + gtx.Dp(ui.Space8)
			return dims
		}))
	}
	grid := func(fields ...analysisFilterField) analysisFilterField {
		return func(gtx layout.Context, view ui.View) layout.Dimensions {
			return (ui.Grid{MinColumn: 215, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, len(fields),
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return fields[i](gtx, view) })
		}
	}
	identity := func(i int) analysisFilterField {
		return func(gtx layout.Context, view ui.View) layout.Dimensions {
			return d.fields[i].Layout(gtx, view, ui.FieldOptions{
				Label:    catalogIdentityLabels[i],
				Disabled: c.template != "" && (i == portfolioInstrumentMarket || i == portfolioInstrumentCountry),
			})
		}
	}
	text := func(label i18n.MessageID, text string) analysisFilterField {
		return func(gtx layout.Context, view ui.View) layout.Dimensions {
			return labeledAmount(view, label, text, view.Colors.Ink)(gtx)
		}
	}
	appendRow(grid(identity(portfolioInstrumentName), identity(portfolioInstrumentShortName)))
	if !create {
		appendRow(func(gtx layout.Context, view ui.View) layout.Dimensions {
			return analysisDisclosure(view, &c.more, l.Text(i18n.PortfolioCatalogConfiguration), c.advanced)(gtx)
		})
	}
	if create || c.advanced {
		if !create && c.template != "" {
			appendRow(text(i18n.PortfolioCatalogReadType, l.Text(marketInstrumentTypes[c.kind])))
		} else {
			var types []ui.Choice
			for _, kind := range domain.AllInstrumentTypes() {
				types = append(types, ui.Choice{ID: string(kind), Label: l.Text(marketInstrumentTypes[kind])})
			}
			appendRow(analysisSelect(&c.typePicker, i18n.PortfolioCatalogType, types, string(c.kind), false, func(value string) {
				next := domain.InstrumentType(value)
				if c.kind == next {
					return
				}
				if !create {
					c.kind = next
					c.retireReads()
					return
				}
				if c.kind == domain.InstrumentCrypto || next == domain.InstrumentCrypto || c.template != "" {
					for _, i := range []int{portfolioInstrumentMarket, portfolioInstrumentCountry, portfolioInstrumentSymbol, portfolioInstrumentISIN} {
						d.fields[i].Editor.SetText("")
					}
				}
				c.setCreateType(next)
				c.currency = p.current.summary.Currency
				if next == domain.InstrumentCrypto {
					c.currency = catalogCryptoDefaultCurrency
					d.fields[portfolioInstrumentMarket].Editor.SetText(domain.CryptoMarket)
				}
			}))
		}
		if c.kind == domain.InstrumentPreciousMetal {
			appendRow(func(gtx layout.Context, view ui.View) layout.Dimensions {
				return p.layoutCatalogTemplate(gtx, view, d)
			})
		}
		if instrumentLookupProvider(c.kind) != "" {
			appendRow(func(gtx layout.Context, view ui.View) layout.Dimensions {
				return p.layoutCatalogLookup(gtx, view, d)
			})
		}
		appendRow(func(gtx layout.Context, view ui.View) layout.Dimensions {
			choices := directoryIconChoices[l]
			icon := c.icon
			if icon == "" {
				icon = domain.DefaultInstrumentIcon(c.kind)
			}
			label := ""
			if !slices.ContainsFunc(choices, func(choice ui.Choice) bool { return choice.ID == icon }) {
				label = l.Text(i18n.PortfolioCatalogCurrentIcon)
			}
			c.iconPicker.Layout(gtx, view, ui.PickerField{Label: i18n.DirectoryIcon, Origin: view.Origin}, ui.SelectOptions{
				Items: choices, Searchable: true, Clearable: true, ValueLabel: label,
			}, icon, func(value string) { c.icon = value })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		})
	}
	appendRow(grid(identity(portfolioInstrumentCountry), identity(portfolioInstrumentMarket), identity(portfolioInstrumentSymbol), identity(portfolioInstrumentISIN)))
	if create || c.advanced {
		currency := text(i18n.PortfolioCatalogReadCurrency, c.currency.String())
		if create {
			currency = analysisSelect(&c.currencyPicker, i18n.PortfolioCatalogCurrency, supportedCurrencyChoices, c.currency.String(), true, func(value string) {
				c.currency = domain.CurrencyCode(value)
				c.metal.reset()
			})
		}
		var sources []ui.Choice
		for _, source := range domain.AllQuoteSourceKinds() {
			sources = append(sources, ui.Choice{ID: string(source), Label: marketSourceLabel(l, source)})
		}
		source := analysisSelect(&c.sourcePicker, i18n.PortfolioCatalogQuoteSource, sources, string(c.source), false, func(value string) {
			c.source = domain.QuoteSourceKind(value)
			if c.source == domain.QuoteSourceProvider && c.provider == "" {
				c.provider = application.YahooFinanceProviderKey
				if c.kind == domain.InstrumentCrypto {
					c.provider = application.CoinGeckoProviderKey
				}
			}
			c.metal.reset()
		})
		appendRow(grid(currency, source))
		if c.source == domain.QuoteSourceProvider {
			if c.template != "" {
				appendRow(func(gtx layout.Context, view ui.View) layout.Dimensions {
					return p.layoutCatalogMetalPreview(gtx, view, d)
				})
			} else {
				var providers []ui.Choice
				for _, id := range application.InstrumentProviderKeys() {
					if id != application.CoinGeckoProviderKey || c.kind == domain.InstrumentCrypto {
						providers = append(providers, ui.Choice{ID: id, Label: instrumentProviderLabel(l, id)})
					}
				}
				appendRow(grid(analysisSelect(&c.providerPicker, i18n.InstrumentBindingProvider, providers, c.provider, false, func(value string) {
					if c.provider != value {
						c.provider = value
						c.providerSymbol.Editor.SetText("")
					}
				}), func(gtx layout.Context, view ui.View) layout.Dimensions {
					return c.providerSymbol.Layout(gtx, view, ui.FieldOptions{Label: i18n.InstrumentBindingSymbol})
				}))
			}
		}
	} else {
		appendRow(grid(text(i18n.PortfolioCatalogReadCurrency, c.currency.String()), text(i18n.PortfolioCatalogReadQuoteSource, marketSourceLabel(l, c.source))))
	}
	appendRow(identity(portfolioInstrumentNote))
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func (p *portfolioPage) layoutCatalogTemplate(gtx layout.Context, view ui.View, d *portfolioCatalogDraft) layout.Dimensions {
	c, l := &d.config, view.Language
	templates := []ui.Choice{
		{Label: l.Text(i18n.PortfolioCatalogMetalCustom)},
		{ID: "gold", Label: l.Text(i18n.PortfolioCatalogMetalGold)},
		{ID: "silver", Label: l.Text(i18n.PortfolioCatalogMetalSilver)},
	}
	units := []ui.Choice{
		{ID: domain.GramQuantityUnit, Label: l.Text(i18n.PortfolioCatalogGram)},
		{ID: domain.TroyOunceQuantityUnit, Label: l.Text(i18n.PortfolioCatalogTroyOunce)},
	}
	if d.instrument.ID != "" {
		if c.template == "" {
			return layout.Dimensions{}
		}
		return labeledAmount(view, i18n.PortfolioCatalogReadMetal,
			ui.ChoiceLabel(view, templates, c.template)+" · "+ui.ChoiceLabel(view, units, c.unit), view.Colors.Ink)(gtx)
	}
	template := analysisSelect(&c.templatePicker, i18n.PortfolioCatalogMetalTemplate, templates, c.template, false, func(value string) {
		d.setTemplate(value, ui.ChoiceLabel(view, templates, value))
	})
	if c.template == "" {
		return template(gtx, view)
	}
	unit := analysisSelect(&c.unitPicker, i18n.PortfolioCatalogMetalUnit, units, c.unit, false, func(value string) {
		c.unit = value
		c.metal.reset()
	})
	return (ui.Grid{MinColumn: 215, MaxColumns: 2, Gap: ui.Space8}).Layout(gtx, view, 2,
		func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
			if i == 0 {
				return template(gtx, view)
			}
			return unit(gtx, view)
		})
}

func (p *portfolioPage) layoutCatalogLookup(gtx layout.Context, view ui.View, d *portfolioCatalogDraft) layout.Dimensions {
	q, l := &d.config.lookup, view.Language
	query := strings.TrimSpace(q.query.Text())
	if q.searched != "" && q.searched != query {
		q.reset()
	}
	ready := q.request == nil && utf8.RuneCountInString(query) >= 2
	if ready && q.search.Clicked(gtx) {
		p.lookupCatalogInstrument(d, query)
		gtx.Execute(op.InvalidateCmd{})
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return q.query.Layout(gtx, view, ui.FieldOptions{Label: i18n.InstrumentBindingQuery})
		}),
		ui.Rigid(returnMonthButton(view, &q.search, i18n.InstrumentBindingLookup, nil, ready)),
	}
	status := i18n.NoMessage
	switch {
	case q.request != nil:
		status = i18n.InstrumentBindingLooking
	case q.err != nil:
		status = i18n.InstrumentBindingLookupFailed
	case q.searched != "" && len(q.hits) == 0:
		status = i18n.InstrumentBindingLookupEmpty
	}
	if status != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
	}
	for i, hit := range q.hits {
		valid := hit.ProviderKey == instrumentLookupProvider(d.config.kind) && strings.TrimSpace(hit.ProviderSymbol) != "" &&
			(hit.Type == "" || hit.Type == string(d.config.kind))
		if hit.QuoteCurrency != "" {
			_, err := domain.ParseCurrency(hit.QuoteCurrency)
			valid = valid && err == nil && (d.instrument.ID == "" || hit.QuoteCurrency == d.config.currency.String())
		}
		button := &q.buttons[i]
		if valid && button.Clicked(gtx) {
			d.chooseCatalogHit(hit)
			d.fields[portfolioInstrumentName].Focus(gtx)
			gtx.Execute(op.InvalidateCmd{})
			break
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !valid {
				gtx = gtx.Disabled()
			}
			return layout.W.Layout(gtx, view.Button(button, instrumentBindingHitLabel(hit)))
		}))
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func (p *portfolioPage) layoutCatalogMetalPreview(gtx layout.Context, view ui.View, d *portfolioCatalogDraft) layout.Dimensions {
	c, l := &d.config, view.Language
	m := &c.metal
	if m.request == nil && m.load.Clicked(gtx) {
		p.previewCatalogMetal(d)
		gtx.Execute(op.InvalidateCmd{})
	}
	rows := []ui.ColumnChild{
		ui.Rigid(view.Message(instrumentProviderLabel(l, application.YahooFinanceProviderKey)+" · "+domain.MetalProviderSymbol(c.template), view.Colors.SecondaryInk)),
		ui.Rigid(returnMonthButton(view, &m.load, i18n.PortfolioCatalogMetalPreview, nil, m.request == nil)),
	}
	switch {
	case m.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketLoading), view.Colors.SecondaryInk)))
	case m.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.PortfolioCatalogMetalUnavailable), view.Colors.ErrorInk)))
	case m.current != nil:
		amount := fmt.Sprintf(l.Text(i18n.PortfolioCurrentPrice), m.current.Currency, portfolioDecimal(view, m.current.Price.Canonical()))
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, fmt.Sprintf(l.Text(catalogMetalPriceUnit(c.unit)), amount), view.Colors.Ink)))
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func catalogMetalPriceUnit(unit string) i18n.MessageID {
	if unit == "g" {
		return i18n.QuoteHistoryPerGram
	}
	return i18n.PortfolioCatalogPricePerTroyOunce
}
