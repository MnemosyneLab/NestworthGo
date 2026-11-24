package desktop

import (
	"errors"
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *directoryList) rowContent(gtx layout.Context, view ui.View, item directoryRow, buttons *directoryRowButtons) layout.Dimensions {
	changeLabel, changeName := i18n.InstitutionsArchive, i18n.InstitutionsArchiveName
	if item.archived {
		changeLabel, changeName = i18n.InstitutionsRestore, i18n.InstitutionsRestoreName
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return ui.LayoutGlyph(gtx, p.glyph(item.iconKey(), item.institutionType), 20, view.Colors.Accent)
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, item.Name, 15, font.Medium, view.Colors.Ink)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if item.institutionType == "" {
							if item.archived {
								return view.Message(view.Language.Text(i18n.DirectoryArchived), view.Colors.SecondaryInk)(gtx)
							}
							return layout.Dimensions{}
						}
						detail := view.Language.Text(institutionTypeLabel(item.institutionType))
						if item.archived {
							detail = fmt.Sprintf(view.Language.Text(i18n.InstitutionsArchivedType), detail)
						}
						return view.Label(gtx, detail, 13, font.Normal, view.Colors.SecondaryInk)
					}),
				)
			})
		}),
		layout.Rigid(view.NamedButton(&buttons.view, view.Language.Text(i18n.DirectoryView), fmt.Sprintf(view.Language.Text(i18n.DirectoryViewName), item.Name))),
		layout.Rigid(view.NamedButton(&buttons.edit, view.Language.Text(i18n.InstitutionsEdit), fmt.Sprintf(view.Language.Text(i18n.InstitutionsEditName), item.Name))),
		layout.Rigid(view.NamedButton(&buttons.archive, view.Language.Text(changeLabel), fmt.Sprintf(view.Language.Text(changeName), item.Name))),
	)
}

func (p *directoryList) beginEditor(mode directoryOperation, row directoryRow) {
	p.editMode, p.editID = mode, row.ID
	p.draft.Editor.SetText(row.Name)
	p.draft.Editor.SetCaret(p.draft.Editor.Len(), 0)
	p.kind = row.institutionType
	if p.isInstitutions() && mode == directoryCreate {
		p.kind = domain.InstitutionBank
	}
	p.icon = row.iconKey()
	if p.icon == "" {
		p.icon = p.fallbackIcon(p.kind)
	}
	p.originalIcon, p.iconCustomized = p.icon, false
	p.saveErr, p.focusDraft = nil, true
	p.notice, p.writeProblem = i18n.NoMessage, i18n.NoMessage
}

func (p *directoryList) editor(gtx layout.Context, view ui.View) layout.Dimensions {
	waiting := p.busy
	if p.focusDraft && !waiting {
		p.draft.Focus(gtx)
		p.focusDraft = false
	}
	if !waiting && (p.cancel.Clicked(gtx) || p.editEscape(gtx)) {
		if p.editMode == directoryCreate {
			p.focusAdd = true
		} else {
			p.focusRow = directoryFocus{id: p.editID}
		}
		p.editMode, p.saveErr = directoryRead, nil
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	const pad = 12
	gap := gtx.Dp(8)
	creating := p.editMode == directoryCreate
	title, save := p.copy.edit, i18n.InstitutionsSave
	if creating {
		title, save = p.copy.add, i18n.InstitutionsCreate
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
						Label:   p.copy.name,
						Problem: func(value string) string { return p.editProblem(view, value) },
					})
					pickerOrigin.Y += dims.Size.Y + gap
					return dims
				}),
			}
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.editorPickers(gtx, view, pickerOrigin, waiting)
			}))
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if waiting {
					return view.Message(view.Language.Text(i18n.InstitutionsSaving), view.Colors.SecondaryInk)(gtx)
				}
				if p.save.Clicked(gtx) {
					p.rejectedDraft = p.draft.Text()
					change := directoryChange{operation: p.editMode, id: p.editID, name: p.rejectedDraft, kind: p.kind}
					if creating || p.icon != p.originalIcon {
						icon := p.icon
						change.icon = &icon
					}
					p.start(change)
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

func (p *directoryList) isInstitutions() bool { return p.copy.title == i18n.NavInstitutions }

func (p *directoryList) editorPickers(gtx layout.Context, view ui.View, origin image.Point, disabled bool) layout.Dimensions {
	field := ui.PickerField{
		Label: i18n.DirectoryIcon, Origin: origin, Disabled: disabled,
		Previous: &p.draft.Editor, Next: &p.save,
	}
	gtx = ui.LimitWidth(gtx, 470)
	height := gtx.Dp(60)
	if !p.isInstitutions() || p.editMode != directoryCreate {
		p.layoutIconPicker(gtx, view, field)
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
	}
	gap := gtx.Dp(10)
	width := min(gtx.Dp(170), gtx.Constraints.Max.X/2)
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			typeField := field
			typeField.Label, typeField.Next = i18n.InstitutionsType, &p.iconPicker.Trigger
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			p.typePicker.Layout(gtx, view, typeField, ui.SelectOptions{Items: institutionTypeChoices(view)}, string(p.kind), func(value string) {
				p.kind = domain.InstitutionType(value)
				if !p.iconCustomized {
					p.icon = p.fallbackIcon(p.kind)
				}
			})
			return layout.Dimensions{Size: image.Pt(width, height)}
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			field.Previous = &p.typePicker.Trigger
			field.Origin.X += width + gap
			p.layoutIconPicker(gtx, view, field)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, height)}
		}),
	)
}

func (p *directoryList) layoutIconPicker(gtx layout.Context, view ui.View, field ui.PickerField) {
	field.Glyph = p.glyph(p.icon, p.kind)
	options := ui.SelectOptions{Items: directoryIconChoices[view.Language], Searchable: true}
	value := p.icon
	if p.isInstitutions() {
		options.Items, options.FoldSearch = institutionIconChoices[view.Language], banklogos.FoldSearch
	}
	if logo, ok := directoryBankLogos[p.icon]; ok {
		value, options.ValueLabel = logo.ID, logo.Label
	}
	p.iconPicker.Layout(gtx, view, field, options, value, func(value string) {
		p.icon, p.iconCustomized = value, true
	})
}

func (p *directoryList) editEscape(gtx layout.Context) bool {
	dismiss := false
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &p.draft.Editor, Name: key.NameEscape},
			key.Filter{Focus: &p.typePicker.Trigger, Name: key.NameEscape},
			key.Filter{Focus: &p.iconPicker.Trigger, Name: key.NameEscape},
			key.Filter{Focus: &p.save, Name: key.NameEscape},
			key.Filter{Focus: &p.cancel, Name: key.NameEscape},
		)
		if !ok {
			return dismiss
		}
		dismiss = dismiss || e.(key.Event).State == key.Press
	}
}

func (p *directoryList) editProblem(view ui.View, value string) string {
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
	return view.Language.Text(p.copy.saveError)
}

func (p *directoryList) layoutConfirmation(gtx layout.Context, view ui.View) {
	item := *p.archiveTarget
	p.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: view.Language.Text(p.copy.archiveTitle), Body: item.Name,
		Cancel: view.Language.Text(i18n.InstitutionsCancel), Confirm: view.Language.Text(i18n.InstitutionsArchive),
	}, func() {
		p.archiveTarget = nil
		p.focusRow = directoryFocus{id: item.ID, archive: true}
		gtx.Execute(op.InvalidateCmd{})
	}, func() {
		p.archiveTarget = nil
		p.start(directoryChange{operation: directoryArchive, id: item.ID})
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
