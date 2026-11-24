package desktop

import (
	"errors"
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *institutionsPage) rowContent(gtx layout.Context, view ui.View, item domain.Institution, buttons *institutionRowButtons, archived bool) layout.Dimensions {
	changeLabel, changeName := i18n.InstitutionsArchive, i18n.InstitutionsArchiveName
	if archived {
		changeLabel, changeName = i18n.InstitutionsRestore, i18n.InstitutionsRestoreName
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, item.Name, 15, font.Medium, view.Colors.Ink)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						detail := view.Language.Text(institutionTypeLabel(item.InstitutionType))
						if archived {
							detail = fmt.Sprintf(view.Language.Text(i18n.InstitutionsArchivedType), detail)
						}
						return view.Label(gtx, detail, 13, font.Normal, view.Colors.SecondaryInk)
					}),
				)
			})
		}),
		layout.Rigid(view.NamedButton(&buttons.edit, view.Language.Text(i18n.InstitutionsEdit), fmt.Sprintf(view.Language.Text(i18n.InstitutionsEditName), item.Name))),
		layout.Rigid(view.NamedButton(&buttons.archive, view.Language.Text(changeLabel), fmt.Sprintf(view.Language.Text(changeName), item.Name))),
	)
}

func (p *institutionsPage) beginEditor(mode institutionOperation, id domain.InstitutionID, name string) {
	p.editMode, p.editID = mode, id
	p.draft.Editor.SetText(name)
	p.draft.Editor.SetCaret(p.draft.Editor.Len(), 0)
	p.kind = domain.InstitutionBank
	p.saveErr, p.focusDraft = nil, true
	p.notice, p.writeProblem = i18n.NoMessage, i18n.NoMessage
}

func (p *institutionsPage) editor(gtx layout.Context, view ui.View) layout.Dimensions {
	waiting := p.busy
	if p.focusDraft && !waiting {
		p.draft.Focus(gtx)
		p.focusDraft = false
	}
	if !waiting && (p.cancel.Clicked(gtx) || p.editEscape(gtx)) {
		if p.editMode == institutionCreate {
			p.focusAdd = true
		} else {
			p.focusRow = institutionFocus{id: p.editID}
		}
		p.editMode, p.saveErr = institutionRead, nil
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	const pad = 12
	gap := gtx.Dp(8)
	creating := p.editMode == institutionCreate
	title, save := i18n.InstitutionsEditTitle, i18n.InstitutionsSave
	if creating {
		title, save = i18n.InstitutionsAdd, i18n.InstitutionsCreate
	}
	if waiting {
		gtx = gtx.Disabled()
	}
	pickerOrigin := view.Origin.Add(image.Pt(gtx.Dp(pad), gtx.Dp(pad)))
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(pad).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := view.Label(gtx, view.Language.Text(title), 15, font.Medium, view.Colors.Ink)
					pickerOrigin.Y += dims.Size.Y + gap
					return dims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := p.draft.Layout(ui.LimitWidth(gtx, 470), view, ui.FieldOptions{
						Label:   i18n.PreviewInstitutionName,
						Problem: func(value string) string { return p.editProblem(view, value) },
					})
					pickerOrigin.Y += dims.Size.Y + gap
					return dims
				}),
			}
			if creating {
				children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					field := ui.PickerField{
						Label: i18n.InstitutionsType, Origin: pickerOrigin, Disabled: waiting,
						Previous: &p.draft.Editor, Next: &p.save,
					}
					gtx = ui.LimitWidth(gtx, 470)
					p.typePicker.Layout(gtx, view, field, ui.SelectOptions{Items: institutionTypeChoices(view)}, string(p.kind), func(value string) {
						p.kind = domain.InstitutionType(value)
					})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}))
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if waiting {
					return view.Message(view.Language.Text(i18n.InstitutionsSaving), view.Colors.SecondaryInk)(gtx)
				}
				if p.save.Clicked(gtx) {
					p.rejectedDraft = p.draft.Text()
					p.start(institutionChange{operation: p.editMode, id: p.editID, name: p.rejectedDraft, kind: p.kind})
					gtx.Execute(op.InvalidateCmd{})
				}
				return layout.Flex{Gap: gap}.Layout(gtx,
					layout.Rigid(view.PrimaryButton(&p.save, view.Language.Text(save))),
					layout.Rigid(view.Button(&p.cancel, view.Language.Text(i18n.InstitutionsCancel))),
				)
			}))
			return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx, children...)
		})
	})
}

func (p *institutionsPage) editEscape(gtx layout.Context) bool {
	dismiss := false
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &p.draft.Editor, Name: key.NameEscape},
			key.Filter{Focus: &p.typePicker.Trigger, Name: key.NameEscape},
			key.Filter{Focus: &p.save, Name: key.NameEscape},
			key.Filter{Focus: &p.cancel, Name: key.NameEscape},
		)
		if !ok {
			return dismiss
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
}

func (p *institutionsPage) editProblem(view ui.View, value string) string {
	if p.saveErr == nil {
		return ""
	}
	var problem *domain.Error
	if errors.As(p.saveErr, &problem) && problem.Code == domain.ErrValidation && problem.Field == "name" {
		if value != p.rejectedDraft {
			return ""
		}
		if problem.MaxLength > 0 {
			return fmt.Sprintf(view.Language.Text(i18n.InstitutionsNameLength), problem.MaxLength)
		}
		return view.Language.Text(i18n.PreviewNameRequired)
	}
	return view.Language.Text(i18n.InstitutionsSaveError)
}

func (p *institutionsPage) layoutConfirmation(gtx layout.Context, view ui.View) {
	item := *p.archiveTarget
	p.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: view.Language.Text(i18n.InstitutionsArchiveTitle), Body: item.Name,
		Cancel: view.Language.Text(i18n.InstitutionsCancel), Confirm: view.Language.Text(i18n.InstitutionsArchive),
	}, func() {
		p.archiveTarget = nil
		p.focusRow = institutionFocus{id: item.ID, archive: true}
		gtx.Execute(op.InvalidateCmd{})
	}, func() {
		p.archiveTarget = nil
		p.start(institutionChange{operation: institutionArchive, id: item.ID})
		gtx.Execute(op.InvalidateCmd{})
	})
}

func institutionTypeChoices(view ui.View) []ui.Choice {
	types := domain.AllInstitutionTypes()
	choices := make([]ui.Choice, 0, len(types))
	for _, kind := range types {
		choices = append(choices, ui.Choice{ID: string(kind), Label: view.Language.Text(institutionTypeLabel(kind))})
	}
	return choices
}

func institutionTypeLabel(kind domain.InstitutionType) i18n.MessageID {
	switch kind {
	case domain.InstitutionBank:
		return i18n.InstitutionBank
	case domain.InstitutionBrokerage:
		return i18n.InstitutionBrokerage
	case domain.InstitutionInsurer:
		return i18n.InstitutionInsurer
	case domain.InstitutionExchange:
		return i18n.InstitutionExchange
	case domain.InstitutionEmployer:
		return i18n.InstitutionEmployer
	case domain.InstitutionGovernment:
		return i18n.InstitutionGovernment
	default:
		return i18n.InstitutionOther
	}
}
