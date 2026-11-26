package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type directoryPage struct {
	lists         []*directoryList
	tabs          []widget.Clickable
	active        int
	object        directoryObjectPage
	householdName string
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
		return p.activeList().layout(gtx, view, p.householdName)
	}
	gap := gtx.Dp(10)
	return ui.Column{Gap: 10}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := view.Message(view.Language.Text(i18n.DirectoryDescription), view.Colors.SecondaryInk)(gtx)
			view.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			for i := range p.tabs {
				button := &p.tabs[i]
				if gtx.Enabled() && button.Clicked(gtx) {
					p.active = i
					gtx.Execute(op.InvalidateCmd{})
				}
			}
			dims := view.TabTrack(gtx, len(p.tabs), func(gtx layout.Context, i int) layout.Dimensions {
				return view.TabSegment(&p.tabs[i], view.Language.Text(p.lists[i].copy.title), i == p.active)(gtx)
			})
			view.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			return p.activeList().layout(gtx, view, p.householdName)
		}),
	)
}
