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

func (p *availableFundsPage) layoutPolicySheet(gtx layout.Context, view ui.View) {
	d, l := p.policy, view.Language
	busy := d.write != nil || d.checking
	conflict := d.problem == i18n.FundsPolicyConflict
	view.Overlay = &d.overlay
	d.overlay.BeginFrame(gtx)

	if !d.confirming && d.overlay.Trigger == nil {
		if !busy && (fundsPolicyDismissed(gtx, d) || d.cancel.Clicked(gtx)) {
			p.cancelPolicy(false)
			gtx.Execute(op.InvalidateCmd{})
			return
		}
		if d.reload.Clicked(gtx) {
			if d.checking && p.err != nil {
				d.footerFocus = &d.reload
				p.read()
			} else if conflict && d.latest != nil {
				p.startPolicy(*d.latest, d.title, d.entry)
				gtx.Execute(op.InvalidateCmd{})
				return
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		var first, last event.Tag = &d.sheet.close, &d.save
		switch {
		case d.checking && p.err != nil:
			first, last = &d.reload, &d.reload
		case conflict && d.latest != nil:
			last = &d.reload
		case conflict:
			last = &d.cancel
		}
		if !busy || d.checking && p.err != nil {
			catalogSheetWrapFocus(gtx, first, last)
		}
	}

	footer := func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space8}).Layout(gtx,
			ui.Rigid(p.policyStatus(view, busy)), ui.Rigid(p.policyFooter(view, busy, conflict)))
	}
	panel := func(gtx layout.Context) layout.Dimensions {
		d.sheet.layoutPanel(gtx, view, l.Text(i18n.FundsPolicyTitle), func(gtx layout.Context, view ui.View) layout.Dimensions {
			return p.layoutPolicyBody(gtx, view, gtx.Enabled() && !busy && !conflict)
		}, footer, !busy)
		return layout.Dimensions{Size: gtx.Constraints.Max}
	}
	if d.confirming {
		accessibility.Hidden(gtx, func(gtx layout.Context) layout.Dimensions { return panel(gtx.Disabled()) })
		accessibility.Modal(gtx, &d.confirmation, l.Text(i18n.FundsPolicyResetTitle), func(gtx layout.Context) layout.Dimensions {
			p.layoutPolicyConfirmation(gtx, view)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		})
		return
	}
	panel(gtx)
	d.overlay.Layout(gtx, view)
}

func (p *availableFundsPage) policyStatus(view ui.View, busy bool) layout.Widget {
	d := p.policy
	problem, ink := d.problem, view.Colors.ErrorInk
	switch {
	case d.checking && p.err != nil:
		problem = i18n.FundsPolicyReadFailed
	case busy:
		problem, ink = i18n.FundsPolicyChecking, view.Colors.SecondaryInk
	}
	if problem == i18n.NoMessage {
		return func(layout.Context) layout.Dimensions { return layout.Dimensions{} }
	}
	return view.Message(view.Language.Text(problem), ink)
}

func fundsPolicyDismissed(gtx layout.Context, d *fundsPolicyDraft) bool {
	dismiss := d.sheet.dismissed(gtx)
	for {
		e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
		if !ok {
			return dismiss
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
}

func (p *availableFundsPage) layoutPolicyBody(gtx layout.Context, view ui.View, ready bool) layout.Dimensions {
	d := p.policy
	bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(gtx.Constraints.Max)}
	view = view.WithScrollAnchors(&d.sheet.list)
	d.overlay.ScrollOutsidePanel(gtx, &d.sheet.list, bounds)
	dims := view.List(&d.sheet.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		if !ready {
			gtx = gtx.Disabled()
		}
		origin := view.Origin
		fields := []layout.Widget{view.Message(d.title, view.Colors.SecondaryInk)}
		fields = append(fields, d.formFields(gtx, view, &origin, p.localDate, ready)...)
		rows := make([]ui.ColumnChild, 0, len(fields))
		for _, field := range fields {
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, field)
			}))
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	})
	d.overlay.OffsetScrollAnchor(&d.sheet.list, bounds)
	// Explicit radio-group Tab jumps use FocusCmd, so reveal the measured
	// target within this list just as ordinary window Tab scrolling does.
	if start, end := d.revealBounds[0], d.revealBounds[1]; end > start {
		position := &d.sheet.list.Position
		visibleStart := bounds.Min.Y + position.Offset
		visibleEnd := visibleStart + bounds.Dy()
		switch {
		case start < visibleStart:
			position.Offset += start - visibleStart
			gtx.Execute(op.InvalidateCmd{})
		case end > visibleEnd:
			position.Offset += end - visibleEnd
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	d.revealFocus, d.revealBounds = nil, [2]int{}
	return dims
}
