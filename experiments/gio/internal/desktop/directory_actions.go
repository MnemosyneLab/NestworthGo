package desktop

import (
	"errors"
	"fmt"
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const directoryInlineActionsWidth = 540

func (p *directoryList) rowContent(gtx layout.Context, view ui.View, item directoryRow, buttons *directoryRowButtons) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return directoryTile(gtx, view, item.ID, p.glyph(item.iconKey(), item.institutionType))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = 0
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Line(gtx, ui.TextLabel, item.Name, view.Colors.Ink)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if item.institutionType == "" {
							if item.archived {
								return view.Message(view.Language.Text(i18n.DirectoryArchived), view.Colors.SecondaryInk)(gtx)
							}
							return layout.Dimensions{}
						}
						detail := view.Language.Text(directoryInstitutionTypeLabel(item.institutionType))
						if item.archived {
							detail = fmt.Sprintf(view.Language.Text(i18n.InstitutionsArchivedType), detail)
						}
						return view.Line(gtx, ui.TextCaption, detail, view.Colors.SecondaryInk)
					}),
				)
			})
		}),
		layout.Rigid(smallNamedButton(view, &buttons.view, view.Language.Text(i18n.DirectoryView), fmt.Sprintf(view.Language.Text(i18n.DirectoryViewName), item.Name))),
		layout.Rigid(smallNamedButton(view, &buttons.edit, view.Language.Text(i18n.InstitutionsEdit), fmt.Sprintf(view.Language.Text(i18n.InstitutionsEditName), item.Name))),
		layout.Rigid(directoryArchiveButton(view, item, buttons)),
	)
}

func directoryArchiveButton(view ui.View, item directoryRow, buttons *directoryRowButtons) layout.Widget {
	label, name := i18n.InstitutionsArchive, i18n.InstitutionsArchiveName
	if item.archived {
		label, name = i18n.InstitutionsRestore, i18n.InstitutionsRestoreName
	}
	return smallNamedButton(view, &buttons.archive, view.Language.Text(label), fmt.Sprintf(view.Language.Text(name), item.Name))
}

// smallNamedButton is the compact outline button for row actions.
func smallNamedButton(view ui.View, button *widget.Clickable, title, name string) layout.Widget {
	spec := view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: title, Name: name})
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return spec(gtx)
	}
}

// directoryTile is the leading mark of a directory row: the glyph on a tile
// whose tint comes from the stable row id, so a member keeps its color.
func directoryTile(gtx layout.Context, view ui.View, id string, glyph ui.Glyph) layout.Dimensions {
	sum := 0
	for _, c := range id {
		sum += int(c)
	}
	tint := view.Colors.Category[sum%len(view.Colors.Category)]
	side := gtx.Dp(36)
	bounds := image.Rectangle{Max: image.Pt(side, side)}
	ui.FillRoundRect(gtx, bounds, ui.RadiusMD, ui.Mix(view.Colors.Surface, tint, 0.14))
	inset := (side - gtx.Dp(20)) / 2
	defer op.Offset(image.Pt(inset, inset)).Push(gtx.Ops).Pop()
	ui.LayoutGlyph(gtx, glyph, 20, tint)
	return layout.Dimensions{Size: bounds.Size()}
}

func (p *directoryList) beginEditor(mode directoryOperation, row directoryRow) {
	p.editMode, p.editID = mode, row.ID
	if mode == directoryCreate {
		p.createSheet = newDirectoryCreateSheet()
		row.Name = p.createDraft.name
	}
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
	if mode == directoryCreate && p.createDraft.icon != "" {
		p.kind, p.icon, p.iconCustomized = p.createDraft.kind, p.createDraft.icon, p.createDraft.customIcon
	}
	p.saveErr, p.focusDraft = nil, true
	// Leave the list's status and viewport intact while the draft is open.
}

func (p *directoryList) endEditor() {
	switch p.editMode {
	case directoryCreate:
		p.focusAdd = true
	case directoryEdit:
		p.focusRow = directoryFocus{id: p.editID}
	}
	p.editMode, p.editID, p.saveErr, p.createSheet = directoryRead, "", nil, nil
}

func (p *directoryList) nameEditor(gtx layout.Context, view ui.View, hint i18n.MessageID) layout.Dimensions {
	// Do not take focus until the field can receive input.
	if gtx.Enabled() && p.focusDraft {
		p.draft.Focus(gtx)
		p.focusDraft = false
	}
	return p.draft.Layout(gtx, view, ui.FieldOptions{
		Label: p.copy.name, Hint: hint, Disabled: !gtx.Enabled(),
		Problem: func(value string) string { return p.editProblem(view, value) },
	})
}

func (p *directoryList) submitEditor(gtx layout.Context) {
	p.rejectedDraft = p.draft.Text()
	change := directoryChange{operation: p.editMode, id: p.editID, name: p.rejectedDraft, kind: p.kind}
	if p.editMode == directoryCreate || p.icon != p.originalIcon {
		icon := p.icon
		change.icon = &icon
	}
	p.start(change)
	gtx.Execute(op.InvalidateCmd{})
}

func (p *directoryList) inlineEditor(gtx layout.Context, view ui.View, row directoryRow, buttons *directoryRowButtons) layout.Dimensions {
	l := view.Language
	iconOrigin := view.Origin
	if gtx.Enabled() && p.save.Clicked(gtx) {
		p.submitEditor(gtx)
		gtx = gtx.Disabled()
	}
	save := view.PrimaryButton(&p.save, l.Text(i18n.InstitutionsSave))
	actions := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(save),
			layout.Rigid(view.Button(&p.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(directoryArchiveButton(view, row, buttons)),
		)
	}
	name := func(gtx layout.Context) layout.Dimensions {
		return p.nameEditor(ui.LimitWidth(gtx, 320), view, i18n.NoMessage)
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			leading := func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return directoryTile(gtx, view, row.ID, p.glyph(row.iconKey(), row.institutionType))
					}),
					layout.Flexed(1, name),
				)
			}
			axis := layout.Horizontal
			if gtx.Constraints.Max.X < gtx.Dp(directoryInlineActionsWidth) {
				axis = layout.Vertical
			}
			var dims layout.Dimensions
			if axis == layout.Horizontal {
				dims = layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
					layout.Flexed(1, leading), layout.Rigid(actions))
			} else {
				dims = layout.Flex{Axis: axis, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(leading), layout.Rigid(actions))
			}
			iconOrigin.Y += dims.Size.Y + gtx.Dp(ui.Space12)
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx = ui.LimitWidth(gtx, 360)
			p.layoutIconPicker(gtx, view, ui.PickerField{
				Label: i18n.DirectoryIcon, Origin: iconOrigin, Previous: &buttons.archive, Next: &p.draft.Editor, Disabled: !gtx.Enabled(),
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
	)
}

func (p *directoryList) isInstitutions() bool { return p.copy.title == i18n.NavInstitutions }

func (p *directoryList) layoutIconPicker(gtx layout.Context, view ui.View, field ui.PickerField) {
	field.Name = i18n.DirectoryChooseIcon
	field.Glyph = p.glyph(p.icon, p.kind)
	options := ui.SelectOptions{Items: directoryIconChoices[view.Language], Searchable: true, SearchLabel: i18n.DirectorySearchIcons}
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
		return view.Language.Text(i18n.DirectoryNameRequired)
	}
	if p.editMode == directoryCreate {
		return view.Language.Text(i18n.DirectoryCreateError)
	}
	return view.Language.Text(i18n.DirectorySaveError)
}

func (p *directoryList) layoutConfirmation(gtx layout.Context, view ui.View) {
	item := *p.archiveTarget
	p.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: view.Language.Text(i18n.InstitutionsArchive), Body: item.Name,
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
	return institutionChoices(view, institutionTypeLabel)
}

func institutionChoices(view ui.View, label func(domain.InstitutionType) i18n.MessageID) []ui.Choice {
	types := domain.AllInstitutionTypes()
	choices := make([]ui.Choice, 0, len(types))
	for _, kind := range types {
		choices = append(choices, ui.Choice{ID: string(kind), Label: view.Language.Text(label(kind))})
	}
	return choices
}

func directoryInstitutionTypeLabel(kind domain.InstitutionType) i18n.MessageID {
	if kind == domain.InstitutionInsurer {
		return i18n.DirectoryInstitutionInsurer
	}
	return institutionTypeLabel(kind)
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
