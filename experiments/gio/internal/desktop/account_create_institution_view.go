package desktop

import (
	"fmt"
	"image"
	"slices"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (f *balanceAccountCreation) advanceFromInstitution() {
	d, c := f.draft, f.chooser
	c.selected, c.mode = d.kind, d.mode
	c.modes = accountModes(d.kind, d.role)
	if len(c.modes) == 1 {
		d.focusName = true
		f.sheet.move(accountDetails)
		return
	}
	if c.modeButtons == nil {
		c.modeButtons = map[domain.TrackingMode]*widget.Clickable{}
	}
	for _, mode := range c.modes {
		if c.modeButtons[mode] == nil {
			c.modeButtons[mode] = new(widget.Clickable)
		}
	}
	f.sheet.move(accountTracking)
}

func (p *accountsPage) layoutAccountInstitutionChooser(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	d, c := f.draft, f.chooser
	if d.newInstitution != nil {
		return p.layoutInlineInstitution(gtx, view)
	}
	switch {
	case c.cancel.Clicked(gtx):
		p.cancelAccountCreate(gtx)
		return layout.Dimensions{}
	case c.back.Clicked(gtx):
		f.sheet.move(accountType)
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	ready := d.refs == nil && d.available && !d.loadFailed
	if ready && c.next.Clicked(gtx) {
		if d.institution.ID != "" && d.institutionIndex() < 0 {
			d.problem = i18n.AccountCreateChooseInstitution
		} else {
			f.advanceFromInstitution()
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return accountStepper(gtx, view, 1) }),
		ui.Rigid(view.Body(ui.TextHeading, l.Text(i18n.AccountInstitutionStepTitle), view.Colors.Ink)),
		ui.Rigid(view.Message(l.Text(i18n.AccountInstitutionStepHelp), view.Colors.SecondaryInk)),
	}
	if ready {
		rows = append(rows, ui.Rigid(p.accountInstitutionChoices(view)))
	} else if d.refs != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AccountCreateReferencesLoading), view.Colors.SecondaryInk)))
	} else {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AccountCreateReferencesFailed), view.Colors.ErrorInk)))
		if d.retry.Clicked(gtx) {
			p.loadAccountCreationReferences()
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
	}
	if d.problem != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
			layout.Rigid(view.Button(&c.cancel, l.Text(i18n.InstitutionsCancel))),
			layout.Rigid(view.Button(&c.back, l.Text(i18n.AccountCreateBack))),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !ready {
					gtx = gtx.Disabled()
				}
				return view.PrimaryButton(&c.next, l.Text(i18n.AccountTypeContinue))(gtx)
			}),
		)
	}))
	return (ui.Column{Gap: ui.Space12}).Layout(gtx, rows...)
}

func (p *accountsPage) accountInstitutionChoices(view ui.View) layout.Widget {
	f, l := p.creation, view.Language
	d, c := f.draft, f.chooser
	return func(gtx layout.Context) layout.Dimensions {
		if d.focusAddInstitution && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &d.addInstitution})
			d.focusAddInstitution = false
		}
		if d.addInstitution.Clicked(gtx) {
			d.startInstitution()
			f.sheet.resetPosition()
			gtx.Execute(op.InvalidateCmd{})
		}
		var rows []ui.ColumnChild
		add := func(button *widget.Clickable, id, name string) {
			if button.Clicked(gtx) {
				d.chooseInstitution(id)
				gtx.Execute(op.InvalidateCmd{})
			}
			choice := ui.ButtonSpec{Variant: ui.ButtonOutline, Name: name, Title: name, Toggle: true,
				Selected: d.institution.ID.String() == id, Fill: true, Content: view.Body(ui.TextLabel, name, view.Colors.Ink)}
			if choice.Selected {
				choice.Variant = ui.ButtonSoft
			}
			rows = append(rows, ui.Rigid(view.ButtonOf(button, choice)))
		}
		add(&c.noInstitution, "", l.Text(i18n.AccountNoInstitution))
		preferred := accountInstitutionTypes[d.kind]
		for _, suggested := range []bool{true, false} {
			for _, item := range d.institutions {
				if slices.Contains(preferred, item.InstitutionType) == suggested {
					add(portfolioButton(c.institutionButtons, item.ID.String()), item.ID.String(), item.Name)
				}
			}
		}
		rows = append(rows, ui.Rigid(view.Button(&d.addInstitution, l.Text(i18n.AccountInstitutionStepNew))))
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
	}
}

func (p *accountsPage) layoutInlineInstitution(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	d := f.draft
	edit := d.newInstitution
	if edit.pending == nil && edit.cancel.Clicked(gtx) {
		d.newInstitution, d.focusAddInstitution = nil, true
		if f.sheet.step == accountInstitution {
			f.sheet.resetPosition()
		}
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	if edit.focusName && gtx.Enabled() {
		edit.name.Focus(gtx)
		edit.focusName = false
	}
	gtx, view = view.Measure(gtx, 480)
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
			confirm := i18n.InstitutionsCreate
			if f.sheet.step == accountInstitution {
				confirm = i18n.AccountTypeContinue
			}
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Rigid(view.Button(&edit.cancel, l.Text(i18n.InstitutionsCancel))),
				layout.Rigid(view.PrimaryButton(&edit.save, l.Text(confirm))),
			)
		}),
	)
}
