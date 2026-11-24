package desktop

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *accountsPage) layoutInlineInstitution(gtx layout.Context, view ui.View) layout.Dimensions {
	d, l := p.creation.draft, view.Language
	edit := d.newInstitution
	if edit.pending == nil && edit.cancel.Clicked(gtx) {
		d.newInstitution, d.focusAddInstitution = nil, true
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if edit.focusName && gtx.Enabled() {
		edit.name.Focus(gtx)
		edit.focusName = false
	}
	gtx = ui.LimitWidth(gtx, 480)
	origin := view.Origin
	gap := gtx.Dp(8)
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, l.Text(i18n.InstitutionsAdd), 19, font.Medium, view.Colors.Ink)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, view.Message(l.Text(i18n.AccountCreateInstitutionSeparate), view.Colors.SecondaryInk))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return edit.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewInstitutionName})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			width := min(gtx.Dp(170), gtx.Constraints.Max.X/2)
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
					field := ui.PickerField{Label: i18n.InstitutionsType, Origin: origin, Disabled: !gtx.Enabled(), Previous: &edit.name.Editor, Next: &edit.iconPicker.Trigger}
					edit.typePicker.Layout(gtx, view, field, ui.SelectOptions{Items: institutionTypeChoices(view)}, string(edit.kind), func(value string) {
						edit.kind = domain.InstitutionType(value)
						if !edit.customIcon {
							edit.icon = domain.DefaultIconForInstitutionType(edit.kind)
						}
					})
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(60))}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					options := ui.SelectOptions{Items: institutionIconChoices[l], Searchable: true, FoldSearch: banklogos.FoldSearch}
					value := edit.icon
					if logo, ok := directoryBankLogos[value]; ok {
						value, options.ValueLabel = logo.ID, logo.Label
					}
					field := ui.PickerField{Label: i18n.DirectoryIcon, Origin: origin.Add(image.Pt(width+gap, 0)), Disabled: !gtx.Enabled(), Previous: &edit.typePicker.Trigger, Next: &edit.save}
					edit.iconPicker.Layout(gtx, view, field, options, value, func(value string) {
						edit.icon, edit.customIcon = value, true
					})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if edit.problem == i18n.NoMessage {
				return layout.Dimensions{}
			}
			message := l.Text(edit.problem)
			if edit.nameLimit > 0 {
				message = fmt.Sprintf(message, edit.nameLimit)
			}
			return view.Message(message, view.Colors.ErrorInk)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if edit.pending != nil {
				return view.Message(l.Text(i18n.InstitutionsSaving), view.Colors.SecondaryInk)(gtx)
			}
			if edit.save.Clicked(gtx) {
				p.submitInlineInstitution()
				gtx.Execute(op.InvalidateCmd{})
			}
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Rigid(view.Button(&edit.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.PrimaryButton(&edit.save, l.Text(i18n.InstitutionsCreate))),
			)
		}),
	)
}
