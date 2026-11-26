package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type portfolioTab uint8

const (
	portfolioHoldingsTab portfolioTab = iota
	portfolioInstruments
	portfolioAllocationTab
)

var portfolioTabLabels = [...]i18n.MessageID{i18n.PortfolioMyHoldings, i18n.MarketInstruments, i18n.PortfolioAllocationTab}

type portfolioTabs struct {
	active    portfolioTab
	positions [len(portfolioTabLabels)]layout.Position
	buttons   [len(portfolioTabLabels)]widget.Clickable
}

type portfolioTabsVisit struct {
	active     portfolioTab
	positions  [len(portfolioTabLabels)]layout.Position
	allocation portfolioAllocationVisit
}

func (p *portfolioPage) portfolioTabsVisit() portfolioTabsVisit {
	return portfolioTabsVisit{active: p.tabs.active, positions: p.tabs.positions, allocation: p.allocation.visit()}
}

func (p *portfolioPage) restorePortfolioTabs(visit portfolioTabsVisit) {
	p.tabs.active, p.tabs.positions = visit.active, visit.positions
	p.allocation.restore(visit.allocation)
}

func (p *portfolioPage) layoutPortfolioTabs(gtx layout.Context, view ui.View) layout.Dimensions {
	t := &p.tabs
	for i := range t.buttons {
		if t.buttons[i].Clicked(gtx) && portfolioTab(i) != t.active {
			t.positions[t.active] = p.list.Position
			t.active = portfolioTab(i)
			p.list.Position = t.positions[i]
			p.current = nil
			p.read()
			view.RestorePageScroll(0)
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	return view.TabTrack(gtx, len(t.buttons), func(gtx layout.Context, i int) layout.Dimensions {
		return view.TabSegment(&t.buttons[i], view.Language.Text(portfolioTabLabels[i]), portfolioTab(i) == t.active)(gtx)
	})
}
