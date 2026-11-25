package desktop

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *marketDataPage) instrumentSourceAction(gtx layout.Context, view ui.View, instrument domain.Instrument) layout.Widget {
	if p.instrumentSourceActions == nil {
		p.instrumentSourceActions = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(p.instrumentSourceActions, instrument.ID.String())
	ready := p.request == nil && p.err == nil && !p.providerBusy()
	if ready && button.Clicked(gtx) {
		p.openInstrumentSource(instrument.ID, instrument.DisplayLabel(), button)
		gtx.Execute(op.InvalidateCmd{})
	}
	return returnMonthButton(view, button, i18n.InstrumentBindingOpen, nil, ready)
}

func (p *marketDataPage) layoutInstrumentSource(gtx layout.Context, view ui.View) (event.Tag, bool) {
	d := p.instrumentSource
	closeDraft := func() {
		if d.stopRefs != nil {
			d.stopRefs()
		}
		d.resetLookup()
		p.instrumentSource = nil
		gtx.Execute(op.InvalidateCmd{})
	}
	select {
	case response := <-d.refs:
		d.stopRefs()
		d.refs, d.stopRefs, d.refsErr = nil, nil, response.err
		d.setInstrument(response.value)
	case response := <-d.looking:
		d.stopLookup()
		d.looking, d.stopLookup, d.lookupErr = nil, nil, response.err
		d.hits = response.value
		d.hitButtons = make([]widget.Clickable, len(d.hits))
	case response := <-d.saving:
		d.saving = nil
		if response.err == nil {
			closeDraft()
			return d.from, true
		}
		d.saveProblem = instrumentBindingSaveError(response.err)
		gtx.Execute(key.FocusCmd{Tag: &d.sheet.action})
	default:
	}
	if d.searched != "" && d.searched != strings.TrimSpace(d.query.Text()) {
		d.resetLookup()
	}
	if d.saving == nil && d.sheet.dismissed(gtx, d.focusTags()...) {
		closeDraft()
		return d.from, false
	}
	title := fmt.Sprintf(view.Language.Text(i18n.InstrumentBindingTitle), d.name)
	action := i18n.NoMessage
	switch {
	case d.refsErr != nil:
		action = i18n.HistoryRetry
		if d.sheet.action.Clicked(gtx) {
			p.readInstrumentSource()
			gtx.Execute(op.InvalidateCmd{})
		}
	case d.instrument != nil && d.saving == nil && d.patch() != (application.InstrumentPatch{}):
		action = i18n.InstrumentBindingSave
		if d.sheet.action.Clicked(gtx) && !p.providerBusy() {
			if !d.validBinding() {
				d.saveProblem = i18n.InstrumentBindingRequired
				if d.provider == "" {
					gtx.Execute(key.FocusCmd{Tag: &d.providers[0]})
				} else {
					d.symbol.Focus(gtx)
				}
			} else {
				id, patch := d.id, d.patch()
				d.resetLookup()
				d.saveProblem = i18n.NoMessage
				d.saving = runHistoryRead(&p.workers, p.invalidate, func() (domain.Instrument, error) {
					return p.instrumentSources.PatchInstrument(p.ctx, id, patch)
				})
				action = i18n.NoMessage
			}
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	body := func(gtx layout.Context) layout.Dimensions {
		if d.instrument == nil {
			status := i18n.InstrumentBindingUnavailable
			if d.refs != nil {
				status = i18n.MarketLoading
			} else if d.refsErr != nil {
				status = i18n.MarketFailed
			}
			return view.Message(view.Language.Text(status), view.Colors.SecondaryInk)(gtx)
		}
		return analysisResultList(gtx, view, &d.sheet.list, title, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			return p.instrumentSourceFields(gtx, view, d)
		})
	}
	d.sheet.layoutActions(gtx, view, title, body, action, d.saving == nil)
	return nil, false
}

func (d *marketInstrumentSourceDraft) focusTags() []event.Tag {
	tags := []event.Tag{&d.query.Editor, &d.symbol.Editor, &d.market.Editor, &d.country.Editor, &d.searchButton}
	for i := range d.sources {
		tags = append(tags, &d.sources[i])
	}
	for i := range d.providers {
		tags = append(tags, &d.providers[i])
	}
	for i := range d.hitButtons {
		tags = append(tags, &d.hitButtons[i])
	}
	return tags
}

func (p *marketDataPage) instrumentSourceFields(gtx layout.Context, view ui.View, d *marketInstrumentSourceDraft) layout.Dimensions {
	l := view.Language
	editable := d.saving == nil
	if !editable {
		gtx = gtx.Disabled()
	}
	rows := []ui.ColumnChild{
		ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.QuoteHistoryConfigured), marketSourceLabel(l, d.instrument.QuoteSource)), view.Colors.SecondaryInk)),
		ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.InstrumentBindingCurrency), d.instrument.QuoteCurrency), view.Colors.SecondaryInk)),
	}
	var sources []layout.Widget
	for i, source := range marketQuoteSourceChoices {
		if editable && d.sources[i].Clicked(gtx) && source != d.source {
			d.source, d.saveProblem = source, i18n.NoMessage
			d.resetLookup()
			gtx.Execute(op.InvalidateCmd{})
		}
		sources = append(sources, analysisToggle(view, &d.sources[i], marketSourceLabel(l, source), d.source == source))
	}
	rows = append(rows, ui.Rigid(marketActionRow(view, sources)))
	if d.source == domain.QuoteSourceProvider {
		if d.instrument.UsesMetalConversion() {
			binding := marketBindingText(d.instrument.ProviderKey) + " · " + marketBindingText(d.instrument.ProviderSymbol)
			rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.InstrumentBindingMetal), binding), view.Colors.SecondaryInk)))
		} else {
			rows = append(rows, p.instrumentProviderFields(gtx, view, d, editable)...)
		}
	}
	if d.saveProblem != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(d.saveProblem), view.Colors.ErrorInk)))
	}
	if !editable {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
	}
	return (ui.Column{Gap: 8}).Layout(gtx, rows...)
}

func (p *marketDataPage) instrumentProviderFields(gtx layout.Context, view ui.View, d *marketInstrumentSourceDraft, editable bool) []ui.ColumnChild {
	l := view.Language
	providers := application.InstrumentProviderKeys()
	var actions []layout.Widget
	for i, provider := range providers {
		if provider == application.CoinGeckoProviderKey && d.instrument.Type != domain.InstrumentCrypto {
			continue
		}
		if editable && d.providers[i].Clicked(gtx) {
			d.changeProvider(provider)
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, analysisToggle(view, &d.providers[i], instrumentProviderLabel(l, provider), d.provider == provider))
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, l.Text(i18n.InstrumentBindingProvider), view.Colors.Ink)),
		ui.Rigid(marketActionRow(view, actions))}
	if d.lookupProvider() != "" {
		rows = append(rows, p.instrumentLookupFields(gtx, view, d, editable)...)
	}
	for _, field := range []struct {
		editor *ui.TextField
		label  i18n.MessageID
	}{
		{&d.symbol, i18n.InstrumentBindingSymbol},
		{&d.market, i18n.InstrumentBindingMarket},
		{&d.country, i18n.InstrumentBindingCountry},
	} {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return field.editor.Layout(gtx, view, ui.FieldOptions{Label: field.label, Disabled: !editable})
		}))
	}
	return rows
}

func (p *marketDataPage) instrumentLookupFields(gtx layout.Context, view ui.View, d *marketInstrumentSourceDraft, editable bool) []ui.ColumnChild {
	l := view.Language
	var rows []ui.ColumnChild
	inactive := !editable || d.looking != nil
	query := strings.TrimSpace(d.query.Text())
	ready := editable && utf8.RuneCountInString(query) >= 2 && d.looking == nil
	if ready && d.searchButton.Clicked(gtx) {
		p.lookupInstrumentSource()
		gtx.Execute(op.InvalidateCmd{})
	}
	rows = append(rows,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return d.query.Layout(gtx, view, ui.FieldOptions{Label: i18n.InstrumentBindingQuery, Disabled: !editable})
		}),
		ui.Rigid(returnMonthButton(view, &d.searchButton, i18n.InstrumentBindingLookup, nil, ready)))
	if d.picked != "" {
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.InstrumentBindingChosen), d.picked), view.Colors.Ink)))
	}
	var status i18n.MessageID
	switch {
	case d.looking != nil:
		status = i18n.InstrumentBindingLooking
	case d.lookupErr != nil:
		status = i18n.InstrumentBindingLookupFailed
	case d.searched != "" && len(d.hits) == 0:
		status = i18n.InstrumentBindingLookupEmpty
	}
	if status != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(status), view.Colors.SecondaryInk)))
	}
	for i, hit := range d.hits {
		button := &d.hitButtons[i]
		compatible := d.canChoose(hit)
		if compatible && !inactive && button.Clicked(gtx) {
			d.selectHit(hit)
			d.symbol.Focus(gtx)
			gtx.Execute(op.InvalidateCmd{})
			break
		}
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if inactive || !compatible {
				gtx = gtx.Disabled()
			}
			label := instrumentBindingHitLabel(hit)
			if !compatible {
				label = fmt.Sprintf(l.Text(i18n.InstrumentBindingLookupMismatch), label)
			}
			return view.Button(button, label)(gtx)
		}))
	}
	return rows
}

func instrumentBindingHitLabel(hit application.InstrumentSearchHit) string {
	return domain.InstrumentSelectionLabel(hit.Name, hit.Symbol, "", hit.ProviderSymbol, hit.MarketCode, hit.QuoteCurrency)
}

func instrumentProviderLabel(l i18n.Language, provider string) string {
	switch provider {
	case application.YahooFinanceProviderKey:
		return l.Text(i18n.InstrumentBindingYahoo)
	case application.CoinGeckoProviderKey:
		return l.Text(i18n.SettingsCoinGecko)
	}
	return provider
}
