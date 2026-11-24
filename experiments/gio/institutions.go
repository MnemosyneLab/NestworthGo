package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"slices"
	"strings"
	"sync"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type institutionService interface {
	ListInstitutions(context.Context, bool) ([]domain.Institution, error)
	UpdateInstitution(context.Context, domain.InstitutionID, string) (domain.Institution, error)
}

type institutionRequest uint8

const (
	institutionIdle institutionRequest = iota
	institutionLoading
	institutionSaving
)

type institutionResult struct {
	rows    []domain.Institution
	readErr error
	saveErr error
	written domain.InstitutionID
}

type institutionsPage struct {
	ctx        context.Context
	service    institutionService
	invalidate func()
	workers    sync.WaitGroup
	results    chan institutionResult

	started bool
	request institutionRequest
	rows    []domain.Institution
	readErr error
	search  searchField
	query   string
	list    layout.List
	buttons map[domain.InstitutionID]*widget.Clickable
	refresh widget.Clickable

	editing       domain.InstitutionID
	draft         textField
	save, cancel  widget.Clickable
	saveErr       error
	rejectedDraft string
	focusDraft    bool
	focusRow      domain.InstitutionID
	awaitingRead  domain.InstitutionID
	saved         bool
}

func newInstitutionsPage(ctx context.Context, service institutionService, invalidate func()) *institutionsPage {
	return &institutionsPage{
		ctx: ctx, service: service, invalidate: invalidate,
		results: make(chan institutionResult, 1),
		search:  searchField{field: newTextField(true)}, draft: newTextField(true),
		list: layout.List{Axis: layout.Vertical}, buttons: make(map[domain.InstitutionID]*widget.Clickable),
	}
}

func (p *institutionsPage) start(request institutionRequest, editID domain.InstitutionID, name string) {
	p.started = true
	p.request = request
	p.saved = false
	p.saveErr = nil
	p.workers.Go(func() {
		var result institutionResult
		if request == institutionSaving {
			_, result.saveErr = p.service.UpdateInstitution(p.ctx, editID, name)
			if result.saveErr == nil {
				result.written = editID
			}
		}
		// Even on save we render only the authoritative list, not the draft or
		// the mutation's response. A failed refresh can be retried independently.
		if result.saveErr == nil {
			result.rows, result.readErr = p.service.ListInstitutions(p.ctx, false)
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
		p.request = institutionIdle
		if result.saveErr != nil {
			p.saveErr = result.saveErr
			p.focusDraft = true
			return
		}
		if result.written != "" {
			p.editing = ""
			p.awaitingRead = result.written
		}
		p.rows, p.readErr = result.rows, result.readErr
		if p.readErr == nil && p.awaitingRead != "" {
			p.focusRow = p.awaitingRead
			p.awaitingRead = ""
			p.saved = true
		}
	default:
	}
}

func (p *institutionsPage) layout(gtx layout.Context, view presentation) layout.Dimensions {
	p.receive()
	if !p.started || (p.request == institutionIdle && p.editing == "" && p.refresh.Clicked(gtx)) {
		p.start(institutionLoading, "", "")
	}

	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(limitWidth(gtx, 780),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.header(gtx, view) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.search.layout(limitWidth(gtx, 470), view, previewSearch, institutionsSearchHint, func(query string) {
				p.query = query
				p.list.Position = layout.Position{}
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.editing != "" {
				return p.editor(gtx, view)
			}
			return layout.Dimensions{}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return p.listContent(gtx, view) }),
	)
}

func (p *institutionsPage) header(gtx layout.Context, view presentation) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.label(gtx, view.language.text(navInstitutions), 19, font.Medium, view.colors.ink)
				}),
				layout.Rigid(view.message(view.language.text(institutionsSynthetic), view.colors.secondaryInk)),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.request != institutionIdle || p.editing != "" {
				return layout.Dimensions{}
			}
			label := institutionsRefresh
			if p.readErr != nil {
				label = institutionsRetry
			}
			return view.button(&p.refresh, view.language.text(label))(gtx)
		}),
	)
}

func (p *institutionsPage) listContent(gtx layout.Context, view presentation) layout.Dimensions {
	switch {
	case p.request == institutionLoading:
		return view.message(view.language.text(institutionsLoading), view.colors.secondaryInk)(gtx)
	case p.readErr != nil:
		message := institutionsLoadError
		if p.awaitingRead != "" {
			message = institutionsReloadAfterSaveError
		}
		return view.message(view.language.text(message), view.colors.errorInk)(gtx)
	}

	visible := filterInstitutions(p.rows, p.query)
	restore := p.focusRow
	p.focusRow = ""
	if restore != "" {
		index := slices.IndexFunc(visible, func(row domain.Institution) bool { return row.ID == restore })
		if index >= 0 {
			p.list.ScrollTo(index)
		} else {
			p.search.field.focus(gtx)
		}
	}

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !p.saved {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: 10}.Layout(gtx, view.message(view.language.text(institutionsSaved), view.colors.ink))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if len(visible) == 0 {
				message := institutionsEmpty
				if len(p.rows) > 0 {
					message = previewNoMatches
				}
				return view.message(view.language.text(message), view.colors.secondaryInk)(gtx)
			}
			return p.list.Layout(gtx, len(visible), func(gtx layout.Context, i int) layout.Dimensions {
				item := visible[i]
				return layout.Inset{Bottom: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return p.row(gtx, view, item, restore == item.ID)
				})
			})
		}),
	)
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

func (p *institutionsPage) row(gtx layout.Context, view presentation, item domain.Institution, restoreFocus bool) layout.Dimensions {
	button := p.buttons[item.ID]
	if button == nil {
		button = new(widget.Clickable)
		p.buttons[item.ID] = button
	}
	disabled := p.editing != "" || p.request != institutionIdle
	if restoreFocus && !disabled {
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	if !disabled && button.Clicked(gtx) {
		p.editing = item.ID
		p.draft.editor.SetText(item.Name)
		p.draft.editor.SetCaret(p.draft.editor.Len(), 0)
		p.saveErr = nil
		p.saved = false
		p.focusDraft = true
		gtx.Execute(op.InvalidateCmd{})
	}
	if disabled {
		gtx = gtx.Disabled()
	}
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, gtx.Dp(56)))
	return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(fmt.Sprintf(view.language.text(institutionsEditName), item.Name)).Add(gtx.Ops)
		border := view.colors.divider
		if gtx.Focused(button) || (!disabled && button.Hovered()) {
			border = view.colors.accent
		}
		return view.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							gtx.Constraints.Min.Y = 0
							return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return view.label(gtx, item.Name, 15, font.Medium, view.colors.ink)
								}),
								layout.Rigid(view.message(view.language.text(institutionTypeLabel(item.InstitutionType)), view.colors.secondaryInk)),
							)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if disabled {
							return layout.Dimensions{}
						}
						return layout.Center.Layout(gtx, view.message(view.language.text(institutionsEdit), view.colors.secondaryInk))
					}),
				)
			})
		})
	})
}

func (p *institutionsPage) editor(gtx layout.Context, view presentation) layout.Dimensions {
	waiting := p.request == institutionSaving
	if p.focusDraft && !waiting {
		p.draft.focus(gtx)
		p.focusDraft = false
	}
	if !waiting && (p.cancel.Clicked(gtx) || p.editEscape(gtx)) {
		p.focusRow, p.editing = p.editing, ""
		p.saveErr = nil
		return layout.Dimensions{}
	}
	border := view.colors.divider
	return view.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.label(gtx, view.language.text(institutionsEditTitle), 15, font.Medium, view.colors.ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return p.draft.layout(limitWidth(gtx, 470), view, fieldOptions{
						label: previewInstitutionName, disabled: waiting,
						problem: func(value string) string { return p.editProblem(view, value) },
					})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if waiting {
						return view.message(view.language.text(institutionsSaving), view.colors.secondaryInk)(gtx)
					}
					if p.save.Clicked(gtx) {
						p.rejectedDraft = p.draft.text()
						p.start(institutionSaving, p.editing, p.rejectedDraft)
						gtx.Execute(op.InvalidateCmd{})
					}
					return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
						layout.Rigid(view.button(&p.save, view.language.text(institutionsSave))),
						layout.Rigid(view.button(&p.cancel, view.language.text(institutionsCancel))),
					)
				}),
			)
		})
	})
}

func (p *institutionsPage) editEscape(gtx layout.Context) bool {
	dismiss := false
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &p.draft.editor, Name: key.NameEscape},
			key.Filter{Focus: &p.save, Name: key.NameEscape},
			key.Filter{Focus: &p.cancel, Name: key.NameEscape},
		)
		if !ok {
			return dismiss
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
}

func (p *institutionsPage) editProblem(view presentation, value string) string {
	if p.saveErr == nil || value != p.rejectedDraft {
		return ""
	}
	var problem *domain.Error
	if errors.As(p.saveErr, &problem) && problem.Code == domain.ErrValidation && problem.Field == "name" {
		if problem.MaxLength > 0 {
			return fmt.Sprintf(view.language.text(institutionsNameLength), problem.MaxLength)
		}
		return view.language.text(previewNameRequired)
	}
	return view.language.text(institutionsSaveError)
}

func institutionTypeLabel(kind domain.InstitutionType) messageID {
	switch kind {
	case domain.InstitutionBank:
		return institutionBank
	case domain.InstitutionBrokerage:
		return institutionBrokerage
	case domain.InstitutionInsurer:
		return institutionInsurer
	case domain.InstitutionExchange:
		return institutionExchange
	case domain.InstitutionEmployer:
		return institutionEmployer
	case domain.InstitutionGovernment:
		return institutionGovernment
	default:
		return institutionOther
	}
}
