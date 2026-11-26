package desktop

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
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
	noticeName            string
	writeProblem          i18n.MessageID

	includeArchived bool
	add, refresh    widget.Clickable
	archiveFilter   widget.Bool
	search          ui.SearchField
	query           string
	list            widget.List
	buttons         map[string]*directoryRowButtons
	focusRow        directoryFocus
	focusAdd        bool
	archiveTarget   *directoryRow
	refreshExternal bool
	confirmation    ui.Confirmation
	openObject      func(directoryRef, string, event.Tag)
	restoreVisit    *directoryListVisit

	editMode           directoryOperation
	createSheet        *directoryCreateSheet
	createDraft        directoryCreateDraft
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
		list: widget.List{List: layout.List{Axis: layout.Vertical}}, buttons: make(map[string]*directoryRowButtons),
	}
}

func (p *directoryList) start(change directoryChange) {
	p.started, p.busy = true, true
	p.readGeneration++
	p.operation = change.operation
	p.saveErr = nil
	if change.operation != directoryCreate && change.operation != directoryEdit {
		p.notice, p.writeProblem = i18n.NoMessage, i18n.NoMessage
	}
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
		switch result.change.operation {
		case directoryCreate:
			p.createDraft = directoryCreateDraft{}
			p.endEditor()
		case directoryEdit:
			p.endEditor()
		}
		if result.change.operation != directoryRead {
			p.notice, p.writeProblem = i18n.NoMessage, i18n.NoMessage
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
		p.notice, p.noticeName = i18n.DirectoryCreated, strings.TrimSpace(p.awaitingRead.name)
	case directoryEdit:
		p.notice = i18n.DirectorySaved
	case directoryArchive:
		p.notice = i18n.DirectoryArchived
	case directoryRestore:
		p.notice = i18n.DirectoryRestoredNotice
	default:
		return
	}
	if p.awaitingRead.operation != directoryCreate {
		p.focusRow = p.awaitingRead.focus()
	}
	p.awaitingRead = directoryChange{}
}

// Poll before the shell lays out its background so the opening frame already
// treats the new editor as modal and the successful frame restores Add focus.
func (p *directoryList) updateCreateTrigger(gtx layout.Context) {
	p.receive()
	if p.canAdd(gtx) && p.add.Clicked(gtx) {
		p.beginEditor(directoryCreate, directoryRow{})
		gtx.Execute(op.InvalidateCmd{})
	}
}

func (p *directoryList) canAdd(gtx layout.Context) bool {
	return gtx.Enabled() && !p.busy && p.editMode == directoryRead && p.archiveTarget == nil && p.awaitingRead.operation == directoryRead
}

func (p *directoryList) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.applyVisit()
	controlsEnabled := !p.busy && p.archiveTarget == nil && gtx.Enabled()
	if controlsEnabled {
		p.archiveFilter.Update(gtx)
	}
	switch {
	case !p.started, controlsEnabled && (p.refreshExternal || p.refresh.Clicked(gtx)):
		p.refreshExternal = false
		p.start(directoryChange{})
	case controlsEnabled && p.archiveFilter.Value != p.includeArchived:
		p.setArchiveScope(p.archiveFilter.Value)
	}
	if p.focusAdd && gtx.Enabled() {
		p.focusAdd = false
		gtx.Execute(key.FocusCmd{Tag: &p.add})
	}

	return ui.Column{Gap: 12}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := p.toolbar(gtx, view)
			view.Origin.Y += dims.Size.Y + gtx.Dp(12)
			return dims
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions { return p.listContent(gtx, view) }),
	)
}

// The toolbar keeps its footprint while a side sheet is open or a row is edited.
func (p *directoryList) toolbar(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	rows := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.End, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return p.search.Layout(ui.LimitWidth(gtx, 470), view, p.copy.search, p.copy.searchHint, func(query string) {
							p.query = query
							p.list.Position = layout.Position{}
						})
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.busy {
						gtx = gtx.Disabled()
					}
					return view.Checkbox(&p.archiveFilter, l.Text(i18n.InstitutionsShowArchived))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.busy {
						gtx = gtx.Disabled()
					}
					label := i18n.InstitutionsRefresh
					if p.readErr != nil {
						label = i18n.InstitutionsRetry
					}
					return view.Button(&p.refresh, l.Text(label))(gtx)
				}),
			)
		}),
	}
	if p.includeArchived {
		rows = append(rows, layout.Rigid(view.Message(l.Text(i18n.DirectoryRestoreHint), view.Colors.SecondaryInk)))
	}
	rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		if !p.canAdd(gtx) {
			gtx = gtx.Disabled()
		}
		return view.PrimaryButton(&p.add, l.Text(p.copy.add))(gtx)
	}))
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx, rows...)
}

func (p *directoryList) listContent(gtx layout.Context, view ui.View) layout.Dimensions {
	visible := filterDirectoryRows(p.rows, p.query)
	if p.editMode == directoryEdit && !p.busy && p.loaded && !slices.ContainsFunc(visible, func(row directoryRow) bool { return row.ID == p.editID }) {
		p.endEditor()
	}
	restore := p.focusRow
	if gtx.Enabled() {
		p.focusRow = directoryFocus{}
	} else {
		restore = directoryFocus{}
	}
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

	return ui.Column{Gap: 8}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			message, isError := p.listStatus()
			view.Origin.Y += gtx.Dp(8)
			if message == i18n.NoMessage {
				return layout.Dimensions{}
			}
			ink := view.Colors.SecondaryInk
			if isError {
				ink = view.Colors.ErrorInk
			}
			content := view.Language.Text(message)
			if message == i18n.DirectoryCreated {
				content = fmt.Sprintf(content, p.noticeName)
			}
			dims := view.Message(content, ink)(gtx)
			view.Origin.Y += dims.Size.Y
			return dims
		}),
		ui.Fill(func(gtx layout.Context) layout.Dimensions {
			if !p.loaded {
				return layout.Dimensions{}
			}
			if len(visible) == 0 {
				if strings.TrimSpace(p.query) != "" && len(p.rows) > 0 {
					return view.Message(view.Language.Text(p.copy.noMatches), view.Colors.SecondaryInk)(gtx)
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space4)}.Layout(gtx,
					layout.Rigid(view.Body(ui.TextHeading, view.Language.Text(i18n.DirectoryEmpty), view.Colors.Ink)),
					layout.Rigid(view.Message(view.Language.Text(p.copy.empty), view.Colors.SecondaryInk)),
				)
			}
			return p.layoutRows(gtx, view, visible, restore)
		}),
	)
}

func (p *directoryList) layoutRows(gtx layout.Context, view ui.View, visible []directoryRow, restore directoryFocus) layout.Dimensions {
	var heights map[int]int
	editIndex := -1
	if p.editMode == directoryEdit {
		heights = make(map[int]int)
	}
	dims := pageListGap(gtx, view, &p.list, view.Language.Text(p.copy.title), len(visible), ui.Space8, func(gtx layout.Context, i int) layout.Dimensions {
		dims := p.row(gtx, view, visible[i], restore)
		if heights != nil {
			heights[i] = dims.Size.Y + gtx.Dp(ui.Space8)
			if visible[i].ID == p.editID {
				editIndex = i
			}
		}
		return dims
	})
	if view.Overlay.Trigger == &p.iconPicker.Trigger {
		// The virtual list positions its recorded rows after laying them out.
		// Translate the picker by the measured heights of preceding visible rows.
		pos := p.list.Position
		if editIndex < pos.First || editIndex >= pos.First+pos.Count {
			view.Overlay.Close()
		} else {
			y := -pos.Offset
			for i := pos.First; i < editIndex; i++ {
				y += heights[i]
			}
			bounds := image.Rectangle{Min: view.Origin, Max: view.Origin.Add(dims.Size)}
			view.Overlay.OffsetAnchor(&p.iconPicker.Trigger, image.Pt(0, y), bounds)
		}
	}
	return dims
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
	if p.busy || p.archiveTarget != nil || p.readErr != nil {
		gtx = gtx.Disabled()
	}
	editing := p.editMode == directoryEdit && p.editID == item.ID
	if editing && gtx.Enabled() && view.Overlay.Trigger == nil && (p.cancel.Clicked(gtx) || p.editEscape(gtx)) {
		p.endEditor()
		restore = p.focusRow
		p.focusRow = directoryFocus{}
		editing = false
		gtx.Execute(op.InvalidateCmd{})
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
		case !editing && p.openObject != nil && buttons.view.Clicked(gtx):
			p.openObject(directoryRef{kind: p.copy.title, id: item.ID}, item.Name, &buttons.view)
			gtx.Execute(op.InvalidateCmd{})
		case !editing && buttons.edit.Clicked(gtx):
			p.beginEditor(directoryEdit, item)
			editing = true
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
	gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, gtx.Dp(64))
	return accessibility.Row(gtx, buttons, func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 12, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if editing {
					view.Origin = view.Origin.Add(image.Pt(gtx.Dp(16), gtx.Dp(12)))
					return p.inlineEditor(gtx, view, item, buttons)
				}
				return p.rowContent(gtx, view, item, buttons)
			})
		})
	})
}
