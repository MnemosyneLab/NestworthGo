package desktop

import (
	"context"
	"slices"
	"sync"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// A directory object is identified by its entity type and stored ID.
// A row name is only a display label and can change between visits.
type directoryRef struct {
	kind i18n.MessageID
	id   string
}

func (r directoryRef) valid() bool {
	var err error
	switch r.kind {
	case i18n.NavMembers:
		_, err = domain.ParseMemberID(r.id)
	case i18n.NavInstitutions:
		_, err = domain.ParseInstitutionID(r.id)
	case i18n.NavDirectoryGroups:
		_, err = domain.ParseGroupID(r.id)
	default:
		return false
	}
	return err == nil
}

type directoryObjectResult struct {
	rows       []directoryRow
	row        *directoryRow
	err        error
	generation uint64
}

type directoryObjectPage struct {
	list         *directoryList
	ref          directoryRef
	row          *directoryRow
	err          error
	result       <-chan directoryObjectResult
	cancel       context.CancelFunc
	workers      sync.WaitGroup
	reload       widget.Clickable
	accounts     widget.Clickable
	openAccounts func(domain.InstitutionID, string, event.Tag)
}

func (p *directoryObjectPage) enter(ref directoryRef, list *directoryList) {
	p.leave()
	p.ref, p.list = ref, list
	p.row, p.err = nil, nil
	if list != nil {
		p.read()
	}
}

func (p *directoryObjectPage) leave() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.result = nil
}

func (p *directoryObjectPage) read() {
	if p.list == nil || p.result != nil || p.list.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.list.ctx)
	read, id, invalidate := p.list.source.list, p.ref.id, p.list.invalidate
	generation := p.list.readGeneration
	result := make(chan directoryObjectResult, 1)
	p.cancel, p.result, p.err = cancel, result, nil
	p.workers.Go(func() {
		rows, err := read(ctx, true)
		if ctx.Err() != nil {
			return
		}
		reply := directoryObjectResult{err: err, rows: rows, generation: generation}
		if err == nil {
			if index := slices.IndexFunc(rows, func(row directoryRow) bool { return row.ID == id }); index != -1 {
				row := rows[index]
				reply.row = &row
			}
		}
		result <- reply
		if ctx.Err() == nil {
			invalidate()
		}
	})
}

// A successful empty read invalidates the old object, while a read error
// leaves any previously shown row available for comparison with the retry.
func (p *directoryObjectPage) update() bool {
	select {
	case result := <-p.result:
		p.leave()
		p.err = result.err
		if p.err == nil {
			p.row = result.row
			p.updateList(result)
			return true
		}
	default:
	}
	return false
}

func (p *directoryObjectPage) updateList(result directoryObjectResult) {
	list := p.list
	if list.busy || !list.loaded || list.readErr != nil || list.readGeneration != result.generation {
		return
	}
	// The object read already has this tab's authoritative collection.
	// Share it with the list only if it cannot replace a newer request or
	// hide an unresolved post-write reload failure.
	rows := result.rows
	if !list.includeArchived {
		rows = slices.DeleteFunc(rows, func(row directoryRow) bool { return row.archived })
	}
	list.rows = rows
}

func (p *directoryObjectPage) name() string {
	if p.row != nil {
		return p.row.Name
	}
	return ""
}

func (p *directoryObjectPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	if p.result == nil && p.reload.Clicked(gtx) {
		p.read()
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(12)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
				layout.Flexed(1, p.status(view)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.list == nil {
						return layout.Dimensions{}
					}
					label := i18n.InstitutionsRefresh
					if p.err != nil {
						label = i18n.InstitutionsRetry
					}
					if p.result != nil {
						gtx = gtx.Disabled()
					}
					return view.Button(&p.reload, view.Language.Text(label))(gtx)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.row == nil {
				return layout.Dimensions{}
			}
			return p.details(gtx, view)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.openAccounts == nil || p.ref.kind != i18n.NavInstitutions || p.row == nil || p.result != nil || p.err != nil {
				return layout.Dimensions{}
			}
			if p.accounts.Clicked(gtx) {
				p.openAccounts(domain.InstitutionID(p.ref.id), p.row.Name, &p.accounts)
				gtx.Execute(op.InvalidateCmd{})
			}
			return view.Button(&p.accounts, view.Language.Text(i18n.AccountViewInstitution))(gtx)
		}),
	)
}

func (p *directoryObjectPage) status(view ui.View) layout.Widget {
	message, color := i18n.DirectoryObjectUnavailable, view.Colors.SecondaryInk
	switch {
	case p.result != nil:
		message = i18n.DirectoryObjectLoading
	case p.err != nil:
		message, color = i18n.DirectoryObjectLoadError, view.Colors.ErrorInk
	case p.row != nil:
		message = p.ref.kind
	}
	return view.Message(view.Language.Text(message), color)
}

func (p *directoryObjectPage) details(gtx layout.Context, view ui.View) layout.Dimensions {
	row := *p.row
	state := i18n.DirectoryObjectActive
	if row.archived {
		state = i18n.DirectoryArchived
	}
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return ui.LayoutGlyph(gtx, p.list.glyph(row.iconKey(), row.institutionType), 24, view.Colors.Accent)
						}),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return view.Label(gtx, row.Name, 18, font.Medium, view.Colors.Ink)
						}),
					)
				}),
				layout.Rigid(view.Message(view.Language.Text(state), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if row.institutionType == "" {
						return layout.Dimensions{}
					}
					return view.Message(view.Language.Text(institutionTypeLabel(row.institutionType)), view.Colors.SecondaryInk)(gtx)
				}),
			)
		})
	})
}
