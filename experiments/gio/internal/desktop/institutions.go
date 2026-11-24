package desktop

import (
	"context"
	"image"
	"slices"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type institutionService interface {
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	CreateInstitution(context.Context, string, domain.InstitutionType, ...string) (domain.Institution, error)
	UpdateInstitution(context.Context, domain.InstitutionID, string) (domain.Institution, error)
	ArchiveInstitution(context.Context, domain.InstitutionID, bool) error
}

type institutionOperation uint8

const (
	institutionRead institutionOperation = iota
	institutionCreate
	institutionRename
	institutionArchive
	institutionRestore
)

type institutionChange struct {
	operation institutionOperation
	id        domain.InstitutionID
	name      string
	kind      domain.InstitutionType
}

type institutionResult struct {
	change   institutionChange
	rows     []domain.Institution
	readErr  error
	writeErr error
}

type institutionRowButtons struct {
	edit, archive widget.Clickable
}

type institutionFocus struct {
	id      domain.InstitutionID
	archive bool
}

func (c institutionChange) focus() institutionFocus {
	return institutionFocus{id: c.id, archive: c.operation == institutionArchive || c.operation == institutionRestore}
}

type institutionsPage struct {
	ctx        context.Context
	service    institutionService
	invalidate func()
	workers    sync.WaitGroup
	results    chan institutionResult

	started, loaded, busy bool
	operation             institutionOperation
	rows                  []domain.Institution
	readErr               error
	awaitingRead          institutionChange
	notice                i18n.MessageID
	writeProblem          i18n.MessageID

	includeArchived bool
	add, refresh    widget.Clickable
	archiveFilter   widget.Clickable
	search          ui.SearchField
	query           string
	list            layout.List
	buttons         map[domain.InstitutionID]*institutionRowButtons
	focusRow        institutionFocus
	focusAdd        bool
	archiveTarget   *domain.Institution
	confirmation    ui.Confirmation

	editMode      institutionOperation
	editID        domain.InstitutionID
	draft         ui.TextField
	kind          domain.InstitutionType
	typePicker    ui.Select
	save, cancel  widget.Clickable
	saveErr       error
	rejectedDraft string
	focusDraft    bool
}

func newInstitutionsPage(ctx context.Context, service institutionService, invalidate func()) *institutionsPage {
	return &institutionsPage{
		ctx: ctx, service: service, invalidate: invalidate,
		results: make(chan institutionResult, 1),
		search:  ui.SearchField{Field: ui.NewTextField(true)}, draft: ui.NewTextField(true),
		list: layout.List{Axis: layout.Vertical}, buttons: make(map[domain.InstitutionID]*institutionRowButtons),
	}
}

func (p *institutionsPage) start(change institutionChange) {
	p.started, p.busy = true, true
	p.operation = change.operation
	p.notice, p.writeProblem, p.saveErr = i18n.NoMessage, i18n.NoMessage, nil
	includeArchived := p.includeArchived
	p.workers.Go(func() {
		result := institutionResult{change: change}
		switch change.operation {
		case institutionCreate:
			row, err := p.service.CreateInstitution(p.ctx, change.name, change.kind)
			result.change.id, result.writeErr = row.ID, err
		case institutionRename:
			_, result.writeErr = p.service.UpdateInstitution(p.ctx, change.id, change.name)
		case institutionArchive, institutionRestore:
			result.writeErr = p.service.ArchiveInstitution(p.ctx, change.id, change.operation == institutionArchive)
		}
		// A committed command is complete even if this read fails. Retry the
		// authoritative list on its own without issuing the command again.
		if result.writeErr == nil {
			result.rows, result.readErr = p.service.ListInstitutions(p.ctx, includeArchived)
		}
		p.results <- result
		if p.ctx.Err() == nil {
			p.invalidate()
		}
	})
}

func (p *institutionsPage) receive() {
	select {
	case result := <-p.results:
		p.busy = false
		if result.writeErr != nil {
			switch result.change.operation {
			case institutionCreate, institutionRename:
				p.saveErr, p.focusDraft = result.writeErr, true
			case institutionArchive:
				p.writeProblem, p.focusRow = i18n.InstitutionsArchiveError, result.change.focus()
			case institutionRestore:
				p.writeProblem, p.focusRow = i18n.InstitutionsRestoreError, result.change.focus()
			}
			return
		}
		if result.change.operation != institutionRead {
			p.editMode = institutionRead
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

func (p *institutionsPage) applyWriteNotice() {
	switch p.awaitingRead.operation {
	case institutionCreate:
		p.notice = i18n.InstitutionsCreated
	case institutionRename:
		p.notice = i18n.InstitutionsSaved
	case institutionArchive:
		p.notice = i18n.InstitutionsArchived
	case institutionRestore:
		p.notice = i18n.InstitutionsRestored
	default:
		return
	}
	p.focusRow = p.awaitingRead.focus()
	p.awaitingRead = institutionChange{}
}

func (p *institutionsPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.receive()
	controlsEnabled := !p.busy && p.editMode == institutionRead && p.archiveTarget == nil && gtx.Enabled()
	switch {
	case !p.started, controlsEnabled && p.refresh.Clicked(gtx):
		p.start(institutionChange{})
	case controlsEnabled && p.archiveFilter.Clicked(gtx):
		p.includeArchived = !p.includeArchived
		p.list.Position = layout.Position{}
		p.rows, p.loaded, p.readErr = nil, false, nil
		p.start(institutionChange{})
	case controlsEnabled && p.awaitingRead.operation == institutionRead && p.add.Clicked(gtx):
		p.beginEditor(institutionCreate, "", "")
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
			dims := p.filters(gtx, view)
			editorView.Origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.editMode == institutionRead {
				return layout.Dimensions{}
			}
			return p.editor(gtx, editorView)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.listContent(gtx, view) }),
	)
}

func (p *institutionsPage) header(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, view.Language.Text(i18n.NavInstitutions), 19, font.Medium, view.Colors.Ink)
				}),
				layout.Rigid(view.Message(view.Language.Text(i18n.InstitutionsSynthetic), view.Colors.SecondaryInk)),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.busy || p.editMode != institutionRead || p.awaitingRead.operation != institutionRead {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.add, view.Language.Text(i18n.InstitutionsAdd))(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.busy || p.editMode != institutionRead {
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

func (p *institutionsPage) filters(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := i18n.InstitutionsShowArchived
			if p.includeArchived {
				label = i18n.InstitutionsHideArchived
			}
			if p.busy || p.editMode != institutionRead {
				gtx = gtx.Disabled()
			}
			return view.Button(&p.archiveFilter, view.Language.Text(label))(gtx)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return p.search.Layout(ui.LimitWidth(gtx, 470), view, i18n.PreviewSearch, i18n.InstitutionsSearchHint, func(query string) {
				p.query = query
				p.list.Position = layout.Position{}
			})
		}),
	)
}

func (p *institutionsPage) listContent(gtx layout.Context, view ui.View) layout.Dimensions {
	visible := filterInstitutions(p.rows, p.query)
	restore := p.focusRow
	p.focusRow = institutionFocus{}
	if restore.id != "" {
		index := slices.IndexFunc(visible, func(row domain.Institution) bool { return row.ID == restore.id })
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
				message := i18n.InstitutionsEmpty
				switch {
				case strings.TrimSpace(p.query) != "" && len(p.rows) > 0:
					message = i18n.PreviewNoMatches
				case p.includeArchived:
					message = i18n.InstitutionsEmptyAll
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

func (p *institutionsPage) listStatus() (i18n.MessageID, bool) {
	switch {
	case p.busy && p.operation == institutionRead:
		return i18n.InstitutionsLoading, false
	case p.busy && p.editMode == institutionRead:
		return i18n.InstitutionsSaving, false
	case p.readErr != nil && p.awaitingRead.operation != institutionRead:
		return i18n.InstitutionsReloadAfterSaveError, true
	case p.readErr != nil:
		return i18n.InstitutionsLoadError, true
	case p.writeProblem != i18n.NoMessage:
		return p.writeProblem, true
	default:
		return p.notice, false
	}
}

func filterInstitutions(rows []domain.Institution, query string) []domain.Institution {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return rows
	}
	var matches []domain.Institution
	for _, row := range rows {
		if strings.Contains(strings.ToLower(row.Name), query) {
			matches = append(matches, row)
		}
	}
	return matches
}

func (p *institutionsPage) row(gtx layout.Context, view ui.View, item domain.Institution, restore institutionFocus) layout.Dimensions {
	buttons := p.buttons[item.ID]
	if buttons == nil {
		buttons = new(institutionRowButtons)
		p.buttons[item.ID] = buttons
	}
	archived := item.ArchivedAt != nil
	if p.busy || p.editMode != institutionRead || p.archiveTarget != nil || p.readErr != nil {
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
		case buttons.edit.Clicked(gtx):
			p.beginEditor(institutionRename, item.ID, item.Name)
			gtx.Execute(op.InvalidateCmd{})
		case buttons.archive.Clicked(gtx):
			if archived {
				p.start(institutionChange{operation: institutionRestore, id: item.ID})
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
			return p.rowContent(gtx, view, item, buttons, archived)
		})
	})
}
