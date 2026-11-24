package desktop

import (
	"context"
	"image"
	"slices"
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type directoryOperation uint8

const (
	directoryRead directoryOperation = iota
	directoryCreate
	directoryEdit
	directoryArchive
	directoryRestore
)

type directoryChange struct {
	operation directoryOperation
	id        string
	icon      *string
	name      string
	kind      domain.InstitutionType
}

type directoryResult struct {
	change   directoryChange
	rows     []directoryRow
	readErr  error
	writeErr error
}

type directoryRowButtons struct {
	view, edit, archive widget.Clickable
}

type directoryFocus struct {
	id      string
	archive bool
}

func (c directoryChange) focus() directoryFocus {
	return directoryFocus{id: c.id, archive: c.operation == directoryArchive || c.operation == directoryRestore}
}

type directoryList struct {
	ctx         context.Context
	source      directorySource
	copy        directoryCopy
	defaultIcon string
	invalidate  func()
	workers     sync.WaitGroup
	results     chan directoryResult

	started, loaded, busy bool
	readGeneration        uint64
	operation             directoryOperation
	rows                  []directoryRow
	readErr               error
	awaitingRead          directoryChange
	notice                i18n.MessageID
	writeProblem          i18n.MessageID

	includeArchived bool
	add, refresh    widget.Clickable
	archiveFilter   widget.Clickable
	search          ui.SearchField
	query           string
	list            layout.List
	buttons         map[string]*directoryRowButtons
	focusRow        directoryFocus
	focusAdd        bool
	archiveTarget   *directoryRow
	refreshExternal bool
	confirmation    ui.Confirmation
	openObject      func(directoryRef, string, event.Tag)
	restoreVisit    *directoryListVisit

	editMode           directoryOperation
	editID             string
	draft              ui.TextField
	kind               domain.InstitutionType
	typePicker         ui.Select
	iconPicker         ui.Select
	icon, originalIcon string
	iconCustomized     bool
	save, cancel       widget.Clickable
	saveErr            error
	rejectedDraft      string
	focusDraft         bool
}

func newDirectoryList(ctx context.Context, source directorySource, copy directoryCopy, defaultIcon string, invalidate func()) *directoryList {
	return &directoryList{
		ctx: ctx, source: source, copy: copy, defaultIcon: defaultIcon, invalidate: invalidate,
		results: make(chan directoryResult, 1),
		search:  ui.SearchField{Field: ui.NewTextField(true)}, draft: ui.NewTextField(true),
		list: layout.List{Axis: layout.Vertical}, buttons: make(map[string]*directoryRowButtons),
	}
}

func (p *directoryList) start(change directoryChange) {
	p.started, p.busy = true, true
	p.readGeneration++
	p.operation = change.operation
	p.notice, p.writeProblem, p.saveErr = i18n.NoMessage, i18n.NoMessage, nil
	includeArchived := p.includeArchived
	p.workers.Go(func() {
		result := directoryResult{change: change}
		switch change.operation {
		case directoryCreate:
			result.change.id, result.writeErr = p.source.create(p.ctx, change)
		case directoryEdit:
			result.writeErr = p.source.edit(p.ctx, change)
		case directoryArchive, directoryRestore:
			result.writeErr = p.source.archive(p.ctx, change.id, change.operation == directoryArchive)
		}
		// A committed command is complete even if this read fails. Retry the
		// authoritative list on its own without issuing the command again.
		if result.writeErr == nil {
			result.rows, result.readErr = p.source.list(p.ctx, includeArchived)
		}
		p.results <- result
		if p.ctx.Err() == nil {
			p.invalidate()
		}
	})
}

func (p *directoryList) receive() {
	select {
	case result := <-p.results:
		p.busy = false
		if result.writeErr != nil {
			switch result.change.operation {
			case directoryCreate, directoryEdit:
				p.saveErr, p.focusDraft = result.writeErr, true
			case directoryArchive:
				p.writeProblem, p.focusRow = p.archiveProblem(result.writeErr), result.change.focus()
			case directoryRestore:
				p.writeProblem, p.focusRow = p.copy.restoreError, result.change.focus()
			}
			return
		}
		if result.change.operation != directoryRead {
			p.editMode = directoryRead
			p.awaitingRead = result.change
		}
		p.readErr = result.readErr
		if p.readErr == nil {
			p.rows, p.loaded = result.rows, true
			p.applyWriteNotice()
		}
	default:
	}
}

func (p *directoryList) applyWriteNotice() {
	switch p.awaitingRead.operation {
	case directoryCreate:
		p.notice = p.copy.created
	case directoryEdit:
		p.notice = p.copy.saved
	case directoryArchive:
		p.notice = p.copy.archived
	case directoryRestore:
		p.notice = p.copy.restored
	default:
		return
	}
	p.focusRow = p.awaitingRead.focus()
	p.awaitingRead = directoryChange{}
}

func (p *directoryList) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.receive()
	p.applyVisit()
	controlsEnabled := !p.busy && p.editMode == directoryRead && p.archiveTarget == nil && gtx.Enabled()
	switch {
	case !p.started, controlsEnabled && (p.refreshExternal || p.refresh.Clicked(gtx)):
		p.refreshExternal = false
		p.start(directoryChange{})
	case controlsEnabled && p.archiveFilter.Clicked(gtx):
		p.setArchiveScope(!p.includeArchived)
	case controlsEnabled && p.awaitingRead.operation == directoryRead && p.add.Clicked(gtx):
		p.beginEditor(directoryCreate, directoryRow{})
	}
	if p.focusAdd {
		p.focusAdd = false
		gtx.Execute(key.FocusCmd{Tag: &p.add})
	}

	editorView := view
	gap := gtx.Dp(12)
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(ui.LimitWidth(gtx, 780),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := p.header(gtx, view)
			editorView.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.editMode != directoryRead {
				return layout.Dimensions{}
			}
			dims := p.filters(gtx, view)
			editorView.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.editMode == directoryRead {
				return layout.Dimensions{}
			}
			return p.editor(gtx, editorView)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.listContent(gtx, view) }),
	)
}

func (p *directoryList) header(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, view.Message(view.Language.Text(i18n.InstitutionsSynthetic), view.Colors.SecondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.busy || p.editMode != directoryRead || p.awaitingRead.operation != directoryRead {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.add, view.Language.Text(p.copy.add))(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.busy || p.editMode != directoryRead {
				gtx = gtx.Disabled()
			}
			label := i18n.InstitutionsRefresh
			if p.readErr != nil {
				label = i18n.InstitutionsRetry
			}
			return view.Button(&p.refresh, view.Language.Text(label))(gtx)
		}),
	)
}

func (p *directoryList) filters(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := i18n.InstitutionsShowArchived
			if p.includeArchived {
				label = i18n.InstitutionsHideArchived
			}
			if p.busy || p.editMode != directoryRead {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.archiveFilter, view.Language.Text(label))(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.search.Layout(ui.LimitWidth(gtx, 470), view, p.copy.search, p.copy.searchHint, func(query string) {
				p.query = query
				p.list.Position = layout.Position{}
			})
		}),
	)
}

func (p *directoryList) listContent(gtx layout.Context, view ui.View) layout.Dimensions {
	visible := filterDirectoryRows(p.rows, p.query)
	restore := p.focusRow
	p.focusRow = directoryFocus{}
	if restore.id != "" {
		index := slices.IndexFunc(visible, func(row directoryRow) bool { return row.ID == restore.id })
		if index >= 0 {
			position := p.list.Position
			end := position.First + position.Count
			if position.OffsetLast < 0 {
				end--
			}
			if index < position.First || index >= end || index == position.First && position.Offset > 0 {
				p.list.ScrollTo(index)
			}
		} else {
			p.search.Field.Focus(gtx)
		}
	}

	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			message, isError := p.listStatus()
			if message == i18n.NoMessage {
				return layout.Dimensions{}
			}
			ink := view.Colors.SecondaryInk
			if isError {
				ink = view.Colors.ErrorInk
			}
			return view.Message(view.Language.Text(message), ink)(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !p.loaded {
				return layout.Dimensions{}
			}
			if len(visible) == 0 {
				message := p.copy.empty
				switch {
				case strings.TrimSpace(p.query) != "" && len(p.rows) > 0:
					message = p.copy.noMatches
				case p.includeArchived:
					message = p.copy.emptyAll
				}
				return view.Message(view.Language.Text(message), view.Colors.SecondaryInk)(gtx)
			}
			return p.list.Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return p.row(gtx, view, visible[i], restore)
				})
			})
		}),
	)
}

func (p *directoryList) listStatus() (i18n.MessageID, bool) {
	switch {
	case p.busy && p.operation == directoryRead:
		return p.copy.loading, false
	case p.busy && p.editMode == directoryRead:
		return i18n.InstitutionsSaving, false
	case p.readErr != nil && p.awaitingRead.operation != directoryRead:
		return i18n.InstitutionsReloadAfterSaveError, true
	case p.readErr != nil:
		return p.copy.loadError, true
	case p.writeProblem != i18n.NoMessage:
		return p.writeProblem, true
	default:
		return p.notice, false
	}
}

func normalizeDirectoryQuery(query string) string {
	return strings.ToLower(strings.TrimSpace(query))
}

func filterDirectoryRows(rows []directoryRow, query string) []directoryRow {
	query = normalizeDirectoryQuery(query)
	if query == "" {
		return rows
	}
	var matches []directoryRow
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), query) {
			matches = append(matches, row)
		}
	}
	return matches
}

func (p *directoryList) row(gtx layout.Context, view ui.View, item directoryRow, restore directoryFocus) layout.Dimensions {
	buttons := p.buttons[item.ID]
	if buttons == nil {
		buttons = new(directoryRowButtons)
		p.buttons[item.ID] = buttons
	}
	archived := item.archived
	if p.busy || p.editMode != directoryRead || p.archiveTarget != nil || p.readErr != nil {
		gtx = gtx.Disabled()
	}
	if gtx.Enabled() {
		if restore.id == item.ID {
			target := &buttons.edit
			if restore.archive {
				target = &buttons.archive
			}
			gtx.Execute(key.FocusCmd{Tag: target})
		}
		switch {
		case p.openObject != nil && buttons.view.Clicked(gtx):
			p.openObject(directoryRef{kind: p.copy.title, id: item.ID}, item.Name, &buttons.view)
			gtx.Execute(op.InvalidateCmd{})
		case buttons.edit.Clicked(gtx):
			p.beginEditor(directoryEdit, item)
			gtx.Execute(op.InvalidateCmd{})
		case buttons.archive.Clicked(gtx):
			if archived {
				p.start(directoryChange{operation: directoryRestore, id: item.ID})
			} else {
				p.archiveTarget = &item
				p.confirmation.Open()
			}
			gtx.Execute(op.InvalidateCmd{})
		}
	}
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, gtx.Dp(56))
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return p.rowContent(gtx, view, item, buttons)
		})
	})
}
