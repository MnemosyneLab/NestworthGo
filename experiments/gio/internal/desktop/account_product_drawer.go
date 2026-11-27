package desktop

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountProductVisit struct {
	id           domain.ProductContractID
	name         string
	trigger      event.Tag
	position     layout.Position
	saved        i18n.MessageID
	receipt      domain.ActivityID
	receiptFocus event.Tag
}

type accountProductDrawer struct {
	accountProductVisit
	panel   analysisSheet
	overlay ui.Overlay
}

func (s *shell) openAccountProduct(id domain.ProductContractID, name string, trigger event.Tag) {
	s.enterAccountProduct(accountProductVisit{id: id, name: name, trigger: trigger})
}

func (s *shell) enterAccountProduct(visit accountProductVisit) {
	drawer := &accountProductDrawer{accountProductVisit: visit, panel: newAnalysisSheet()}
	drawer.panel.modal, drawer.panel.width = true, accountSheetWidth
	s.accountProduct = drawer
	p := s.funds.productPage
	p.enter(visit.id, visit.position, false)
	p.saved, p.focusFallback = visit.saved, &drawer.panel.close
}

func (s *shell) leaveAccountProduct() {
	if s.accountProduct == nil {
		return
	}
	p := s.funds.productPage
	p.leave()
	p.receipt, p.valuation, p.terms = nil, nil, nil
	p.focusFallback = &s.headingFocus
	s.accountProduct = nil
}

func (s *shell) openProductActivity(id domain.ActivityID, focus event.Tag) {
	s.historyPage.reads.open(id, focus)
	s.pendingNavigation.accountProductReceipt = s.accountProduct != nil
}

func (s *shell) openProductFromHistory(id domain.ProductContractID, name string, focus event.Tag) {
	if source := s.trail.adjacent(-1); source != nil && source.accountProduct.id == id &&
		source.accountProduct.receipt == s.trail.current().target.activity {
		s.pendingNavigation = &navigationRequest{step: -1, focus: focus}
		return
	}
	s.funds.openProduct(id, name, focus)
}

func (s *shell) layoutAccountProduct(gtx layout.Context, view ui.View) {
	drawer, p := s.accountProduct, s.funds.productPage
	p.poll()
	if p.err == nil && p.current != nil && p.current.detail != nil && p.current.detail.Contract.ID == drawer.id {
		drawer.name = productName(*p.current.detail)
	}
	if p.terms != nil {
		p.layoutTermsSheet(gtx, view)
		return
	}
	view = view.WithTextSelection(&s.textSelection)
	view.Overlay = &drawer.overlay
	drawer.overlay.BeginFrame(gtx)
	canClose := !p.unresolved()
	scroll := &p.list
	switch {
	case p.receipt != nil && p.receipt.plan != nil:
		scroll = &p.receipt.reviewList
	case p.receipt != nil || p.valuation != nil:
		scroll = &drawer.panel.list
	}
	if drawer.overlay.Trigger == nil {
		drawer.scrollFromClose(gtx, &scroll.Position)
		if productSheetDismissed(gtx, &drawer.panel) && canClose {
			entry := s.trail.current()
			entry.accountProduct, entry.focus = accountProductVisit{}, drawer.trigger
			s.restoreFocus = true
			s.leaveAccountProduct()
			gtx.Execute(op.InvalidateCmd{})
			return
		}
	}
	focus := ui.PageFocus{Target: drawer.receiptFocus}
	view.Focus = &focus
	drawer.panel.layoutPanel(gtx, view, drawer.name, func(gtx layout.Context, body ui.View) layout.Dimensions {
		if scroll == &drawer.panel.list {
			// Draft forms can be taller than the drawer. Review and detail views
			// already scroll their own content and keep their actions visible.
			bounds := image.Rectangle{Min: body.Origin, Max: body.Origin.Add(gtx.Constraints.Max)}
			body = body.WithScrollAnchors(&drawer.panel.list)
			drawer.overlay.ScrollOutsidePanel(gtx, &drawer.panel.list, bounds)
			dims := body.List(&drawer.panel.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
				return p.layoutContent(gtx, body, true)
			})
			drawer.overlay.OffsetScrollAnchor(&drawer.panel.list, bounds)
			return dims
		}
		drawer.panel.list.Position = layout.Position{}
		return p.layoutContent(gtx, body, true)
	}, nil, canClose)
	drawer.overlay.Layout(gtx, view)
	if focus.Target != nil && focus.TargetDrawn {
		gtx.Execute(key.FocusCmd{Tag: focus.Target})
		drawer.receiptFocus = nil
	}
}

func (d *accountProductDrawer) scrollFromClose(gtx layout.Context, position *layout.Position) {
	for {
		event, ok := gtx.Event(
			key.Filter{Focus: &d.panel.close, Name: key.NamePageUp, Optional: key.ModShift},
			key.Filter{Focus: &d.panel.close, Name: key.NamePageDown, Optional: key.ModShift},
		)
		if !ok {
			return
		}
		press := event.(key.Event)
		if press.State == key.Press {
			if press.Name == key.NamePageUp {
				position.Offset -= gtx.Dp(180)
			} else {
				position.Offset += gtx.Dp(180)
			}
		}
	}
}
