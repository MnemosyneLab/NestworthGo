package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type directoryPage struct {
	lists  []*directoryList
	tabs   []widget.Clickable
	active int
	object directoryObjectPage
}

func newDirectoryPage(lists ...*directoryList) *directoryPage {
	return &directoryPage{lists: lists, tabs: make([]widget.Clickable, len(lists))}
}

func (p *directoryPage) activeList() *directoryList { return p.lists[p.active] }

func (p *directoryPage) objectList(ref directoryRef) *directoryList {
	if !ref.valid() {
		return nil
	}
	for _, list := range p.lists {
		if list.copy.title == ref.kind {
			return list
		}
	}
	return nil
}

func (p *directoryPage) wait() {
	p.object.workers.Wait()
	for _, list := range p.lists {
		list.workers.Wait()
	}
}

func (p *directoryPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	if len(p.lists) == 1 {
		return p.activeList().layout(gtx, view)
	}
	gap := gtx.Dp(10)
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			tabs := make([]layout.FlexChild, 0, len(p.tabs))
			for i := range p.tabs {
				button := &p.tabs[i]
				if gtx.Enabled() && button.Clicked(gtx) {
					p.active = i
					gtx.Execute(op.InvalidateCmd{})
				}
			}
			for i := range p.tabs {
				tabs = append(tabs, layout.Rigid(view.TabButton(&p.tabs[i], view.Language.Text(p.lists[i].copy.title), i == p.active)))
			}
			dims := layout.Flex{Gap: gtx.Dp(6)}.Layout(gtx, tabs...)
			view.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.activeList().layout(gtx, view)
		}),
	)
}
