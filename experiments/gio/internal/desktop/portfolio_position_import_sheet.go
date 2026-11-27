package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type portfolioPositionImportSheet struct {
	panel   analysisSheet
	overlay ui.Overlay
}

type portfolioPositionReceipt struct {
	activity domain.ActivityID
	message  i18n.MessageID
	view     widget.Clickable
	dismiss  widget.Clickable
}

func (s *shell) portfolioPositionImportOpen() bool {
	return s.activeRoute() == portfolioRoute && s.portfolio != nil &&
		s.portfolio.investment != nil && s.portfolio.investment.draft != nil &&
		s.portfolio.investment.draft.contextualSheet
}

func (p *portfolioPage) layoutPositionImportSheet(gtx layout.Context, view ui.View) {
	f := p.investment
	if f == nil || f.draft == nil || !f.draft.contextualSheet {
		p.positionSheet = nil
		return
	}
	if p.positionSheet == nil {
		sheet := &portfolioPositionImportSheet{panel: newAnalysisSheet()}
		sheet.panel.width, sheet.panel.modal, sheet.panel.focus = 520, true, false
		p.positionSheet = sheet
	}
	s := p.positionSheet
	view.Overlay = &s.overlay
	s.overlay.BeginFrame(gtx)
	canClose := !f.unresolved()
	if s.overlay.Trigger == nil {
		dismiss := s.panel.dismissed(gtx)
		for {
			e, ok := gtx.Event(key.Filter{Name: key.NameEscape})
			if !ok {
				break
			}
			if press, ok := e.(key.Event); ok && press.State == key.Press {
				dismiss = true
			}
		}
		if dismiss && canClose {
			// Reuse the form's cancel path so it restores the initiating toolbar focus.
			f.draft.cancel.Click()
		}
		last := event.Tag(&f.draft.cancel)
		switch {
		case f.draft.review != nil && f.draft.posting != nil:
			last = &s.panel.close
		case f.draft.review != nil && f.draft.uncertain:
			last = &f.draft.retryConfirm
		case f.draft.review != nil:
			last = &f.draft.confirm
		case f.draft.sourceProblem != i18n.NoMessage:
			last = &f.draft.retry
		case f.draft.previewing == nil && f.draft.investmentFieldsVisible():
			last = &f.draft.preview
		}
		catalogSheetWrapFocus(gtx, &s.panel.close, last)
	}
	title := f.draft.labels().action
	if f.draft.review != nil {
		title = f.draft.labels().review
	}
	s.panel.layoutPanel(gtx, view, view.Language.Text(title), func(gtx layout.Context, v ui.View) layout.Dimensions {
		bounds := image.Rectangle{Min: v.Origin, Max: v.Origin.Add(gtx.Constraints.Max)}
		v = v.WithScrollAnchors(&s.panel.list)
		s.overlay.ScrollOutsidePanel(gtx, &s.panel.list, bounds)
		dims := v.List(&s.panel.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			if !canClose {
				gtx = gtx.Disabled()
			}
			return f.layoutForm(gtx, v, p.investmentRuntime())
		})
		s.overlay.OffsetScrollAnchor(&s.panel.list, bounds)
		return dims
	}, nil, canClose)
	if f.draft == nil {
		p.positionSheet = nil
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	s.overlay.Layout(gtx, view)
}

func (p *portfolioPage) layoutPositionImportReceipt(gtx layout.Context, view ui.View) layout.Dimensions {
	r := p.positionReceipt
	if r == nil {
		return layout.Dimensions{}
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	l := view.Language
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Flexed(1, view.Message(l.Text(r.message), view.Colors.Ink)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.openPostedActivity == nil {
						gtx = gtx.Disabled()
					}
					return view.Button(&r.view, l.Text(i18n.PositionImportViewActivity))(gtx)
				}),
				layout.Rigid(view.Button(&r.dismiss, l.Text(i18n.FieldDismiss))),
			)
		})
	})
}

func (p *portfolioPage) openPositionReceipt(gtx layout.Context) {
	if r := p.positionReceipt; r != nil && p.openPostedActivity != nil {
		focus := event.Tag(&r.view)
		if p.investment != nil {
			focus = &p.investment.importOpen
		}
		p.openPostedActivity(r.activity, focus)
	}
	gtx.Execute(op.InvalidateCmd{})
}
