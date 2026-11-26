package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func (p *portfolioPage) closeCatalogSheet(gtx layout.Context) {
	c, d := &p.catalog, p.catalog.draft
	c.focus = d.returnTo
	external := d.external
	c.cancelDraft()
	if external && c.finishExternal != nil {
		c.finishExternal(i18n.NoMessage)
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *portfolioPage) layoutCatalogSheet(gtx layout.Context, view ui.View) {
	d := p.catalog.draft
	d.config.poll()
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)

	create := d.instrument.ID == ""
	title, submit := i18n.PortfolioCatalogEdit, i18n.InstitutionsSave
	action, prompt := i18n.InstitutionsArchive, i18n.PortfolioCatalogArchive
	if create {
		title, submit = i18n.PortfolioCatalogAdd, i18n.PortfolioCatalogAdd
	}
	if d.instrument.ArchivedAt != nil {
		action, prompt = i18n.InstitutionsRestore, i18n.PortfolioCatalogRestore
	}
	busy := d.pending != nil
	changed := d.issue == i18n.PortfolioCatalogChanged
	if d.opening {
		d.opening = false
		if create {
			d.focusName = true
		} else {
			gtx.Execute(key.FocusCmd{Tag: &d.archive})
		}
	}
	if !busy && d.overlay.Trigger == nil {
		dismiss := d.sheet.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			dismiss = dismiss || e.(key.Event).State == key.Press
		}
		switch {
		case d.confirming && (dismiss || d.cancel.Clicked(gtx)):
			d.confirming, d.issue = false, i18n.NoMessage
			gtx.Execute(key.FocusCmd{Tag: &d.archive})
			gtx.Execute(op.InvalidateCmd{})
		case dismiss:
			p.closeCatalogSheet(gtx)
			return
		case !changed && !create && !d.confirming && d.archive.Clicked(gtx):
			d.confirming = true
			gtx.Execute(key.FocusCmd{Tag: &d.cancel})
			gtx.Execute(op.InvalidateCmd{})
		case !changed && d.confirming && d.confirm.Clicked(gtx):
			p.saveCatalogInstrument(true)
			busy = true
		case !changed && !d.confirming && (d.save.Clicked(gtx) || catalogSheetSubmittedFromField(gtx, d)):
			p.saveCatalogInstrument(false)
			busy = true
		}
	}
	if d.focusName && !busy && !d.confirming && !changed {
		d.focusName = false
		d.sheet.list.Position = layout.Position{}
		d.fields[portfolioInstrumentName].Focus(gtx)
	}
	if d.confirming {
		title = prompt
	}
	var last event.Tag
	switch {
	case changed:
		last = &d.sheet.close
	case d.confirming:
		last = &d.confirm
	default:
		last = &d.save
	}
	if !busy && d.overlay.Trigger == nil {
		catalogSheetWrapFocus(gtx, &d.sheet.close, last)
	}

	l := view.Language
	var footer layout.Widget
	if !changed {
		switch {
		case busy:
			footer = func(gtx layout.Context) layout.Dimensions {
				return view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)(gtx)
			}
		case d.confirming:
			footer = func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))),
					layout.Rigid(view.PrimaryButton(&d.confirm, l.Text(action))))
			}
		default:
			footer = view.PrimaryButton(&d.save, l.Text(submit))
		}
	}
	d.sheet.layoutPanel(gtx, view, l.Text(title), func(gtx layout.Context, view ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
		view = view.WithScrollAnchors(&d.sheet.list)
		d.overlay.ScrollOutsidePanel(gtx, &d.sheet.list, bounds)
		dims := view.List(&d.sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			return p.catalogSheetBody(gtx, view, d, action)
		})
		d.overlay.OffsetScrollAnchor(&d.sheet.list, bounds)
		return dims
	}, footer, !busy)
	d.overlay.Layout(gtx, view)
}

func (p *portfolioPage) catalogSheetBody(gtx layout.Context, view ui.View, d *portfolioCatalogDraft, action i18n.MessageID) layout.Dimensions {
	l := view.Language
	if d.confirming || d.issue == i18n.PortfolioCatalogChanged {
		ink := view.Colors.Ink
		text := d.instrument.DisplayLabel()
		if d.issue != i18n.NoMessage {
			ink, text = view.Colors.ErrorInk, l.Text(d.issue)
		}
		return view.Message(text, ink)(gtx)
	}
	origin := view.Origin
	var rows []ui.ColumnChild
	appendRow := func(row layout.Widget) {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := row(gtx)
			origin.Y += dims.Size.Y + gtx.Dp(ui.Space12)
			return dims
		}))
	}
	if d.instrument.ID != "" {
		appendRow(func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, view.Button(&d.archive, l.Text(action)))
		})
		appendRow(fundsCard(view, d.instrument.DisplayLabel(),
			view.Message(d.instrument.QuoteCurrency.String()+" · "+marketSourceLabel(l, d.instrument.QuoteSource), view.Colors.SecondaryInk)))
		appendRow(view.Body(ui.TextHeading, l.Text(i18n.PortfolioFormDetails), view.Colors.Ink))
	}
	if d.issue != i18n.NoMessage && d.issue != i18n.PortfolioCatalogNameRequired {
		appendRow(view.Message(l.Text(d.issue), view.Colors.ErrorInk))
	}
	appendRow(func(gtx layout.Context) layout.Dimensions {
		view.Origin = origin
		return accessibility.Section(gtx, &d.fields, l.Text(i18n.PortfolioFormLabel), func(gtx layout.Context) layout.Dimensions {
			return p.layoutCatalogEditor(gtx, view, d)
		})
	})
	return (ui.Column{Gap: ui.Space12}).Layout(gtx, rows...)
}

func catalogSheetSubmittedFromField(gtx layout.Context, d *portfolioCatalogDraft) bool {
	var filters []event.Filter
	for i := range d.fields {
		if d.fields[i].Editor.SingleLine {
			filters = append(filters,
				key.Filter{Focus: &d.fields[i].Editor, Name: key.NameReturn},
				key.Filter{Focus: &d.fields[i].Editor, Name: key.NameEnter})
		}
	}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			return false
		}
		if e.(key.Event).State == key.Press {
			return true
		}
	}
}

func catalogSheetWrapFocus(gtx layout.Context, first, last event.Tag) {
	for {
		e, ok := gtx.Event(key.Filter{Focus: first, Name: key.NameTab, Required: key.ModShift}, key.Filter{Focus: last, Name: key.NameTab})
		if !ok {
			return
		}
		if e.(key.Event).State == key.Press {
			target := first
			if gtx.Focused(first) {
				target = last
			}
			gtx.Execute(key.FocusCmd{Tag: target})
		}
	}
}
