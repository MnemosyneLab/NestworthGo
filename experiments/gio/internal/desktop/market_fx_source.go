package desktop

import (
	"context"
	"fmt"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketFXSourceWriter interface {
	SetFXPreference(context.Context, string, string, string) (domain.FXPreference, error)
}

var marketFXSourceOptions = [...]domain.QuoteSourceKind{domain.QuoteSourceManual, domain.QuoteSourceProvider, domain.QuoteSourceAgent}

type marketFXSourceDraft struct {
	sheet         analysisSheet
	pair          marketFXPair
	old, selected domain.QuoteSourceKind
	from          event.Tag
	options       [len(marketFXSourceOptions)]widget.Clickable
	request       <-chan historyResult[domain.FXPreference]
	err           error
}

func (p *marketDataPage) fxSourceAction(gtx layout.Context, view ui.View, row marketFXRow) layout.Widget {
	if p.fxSourceActions == nil {
		p.fxSourceActions = make(map[marketFXPair]*widget.Clickable)
	}
	button := p.fxSourceActions[row.pair]
	if button == nil {
		button = &widget.Clickable{}
		p.fxSourceActions[row.pair] = button
	}
	ready := p.request == nil && p.err == nil && !p.providerBusy()
	if button.Clicked(gtx) && ready {
		p.fxSourceDraft = &marketFXSourceDraft{sheet: newAnalysisSheet(), pair: row.pair, old: row.source, selected: row.source, from: button}
		gtx.Execute(op.InvalidateCmd{})
	}
	return returnMonthButton(view, button, i18n.MarketFXChangeSource, nil, ready)
}

func (p *marketDataPage) layoutFXSource(gtx layout.Context, view ui.View) (event.Tag, *domain.FXPreference) {
	d := p.fxSourceDraft
	select {
	case outcome := <-d.request:
		d.request, d.err = nil, outcome.err
		if outcome.err == nil {
			p.fxSourceDraft = nil
			gtx.Execute(op.InvalidateCmd{})
			return d.from, &outcome.value
		}
	default:
	}
	l := view.Language
	tags := make([]event.Tag, len(d.options))
	for i := range d.options {
		tags[i] = &d.options[i]
	}
	if d.request == nil && d.sheet.dismissed(gtx, tags...) {
		p.fxSourceDraft = nil
		gtx.Execute(op.InvalidateCmd{})
		return d.from, nil
	}
	for i, source := range marketFXSourceOptions {
		if d.request == nil && d.options[i].Clicked(gtx) {
			d.selected, d.err = source, nil
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	action := i18n.NoMessage
	if d.request == nil && d.selected != "" && d.selected != d.old {
		action = i18n.MarketFXSaveSource
		if d.sheet.action.Clicked(gtx) && !p.providerBusy() {
			pair, source := d.pair, d.selected
			d.err = nil
			d.request = runHistoryRead(&p.workers, p.invalidate, func() (domain.FXPreference, error) {
				return p.fxSourceWriter.SetFXPreference(p.ctx, pair.first.String(), pair.second.String(), string(source))
			})
			action = i18n.NoMessage
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	title := fmt.Sprintf(l.Text(i18n.MarketFXSourceTitle), d.pair.first, d.pair.second)
	body := func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &d.sheet.list, title, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			rows := []ui.ColumnChild{
				ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.QuoteHistoryConfigured), marketSourceLabel(l, d.old)), view.Colors.SecondaryInk)),
				ui.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.request != nil {
						gtx = gtx.Disabled()
					}
					return (ui.Wrap{Gap: 7}).Layout(gtx, len(d.options), func(gtx layout.Context, i int) layout.Dimensions {
						source := marketFXSourceOptions[i]
						return analysisToggle(view, &d.options[i], marketSourceLabel(l, source), d.selected == source)(gtx)
					})
				}),
			}
			if d.selected == domain.QuoteSourceProvider && d.selected != d.old && p.latest.service != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketFXProviderRefresh), view.Colors.SecondaryInk)))
			}
			if d.err != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketFXSourceFailed), view.Colors.ErrorInk)))
			}
			if d.request != nil {
				rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.MarketFXSourceSaving), view.Colors.SecondaryInk)))
			}
			return (ui.Column{Gap: 8}).Layout(gtx, rows...)
		})
	}
	d.sheet.layoutActions(gtx, view, title, body, action, d.request == nil)
	return nil, nil
}
