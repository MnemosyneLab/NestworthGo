package desktop

import (
	"cmp"
	"fmt"
	"image"
	"slices"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *accountsPage) layoutAccountArchiveAction(gtx layout.Context, view ui.View, account domain.Account) layout.Dimensions {
	m := p.maintenance
	if m.focus == &m.archive && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: m.focus})
		m.focus = nil
	}
	if m.archive.Clicked(gtx) {
		m.startArchive(account)
		gtx.Execute(op.InvalidateCmd{})
	}
	action, variant := i18n.AccountMaintenanceArchive, ui.ButtonDestructive
	if account.ArchivedAt != nil {
		action, variant = i18n.AccountMaintenanceRestore, ui.ButtonOutline
	}
	return view.ButtonOf(&m.archive, ui.ButtonSpec{Variant: variant, Size: ui.ButtonSmall, Title: view.Language.Text(action)})(gtx)
}

func (p *accountsPage) layoutAccountArchive(gtx layout.Context, view ui.View) {
	m, l := p.maintenance, view.Language
	review := m.review
	title, summary := i18n.AccountMaintenanceRestore, i18n.AccountMaintenanceRestoreHelp
	if review.archive {
		title, summary = i18n.AccountMaintenanceArchive, i18n.AccountMaintenanceArchiveHelp
	}
	review.dialog.Layout(gtx, view, ui.ConfirmationOptions{
		Title: l.Text(title), Body: fmt.Sprintf(l.Text(summary), review.name),
		Cancel: l.Text(i18n.InstitutionsCancel), Confirm: l.Text(title), Busy: m.pending != nil, Destructive: review.archive,
	}, func() {
		m.focus, m.review = &m.archive, nil
		gtx.Execute(op.InvalidateCmd{})
	}, func() {
		p.submitAccountMaintenance(&review.archive, application.AccountInput{})
		gtx.Execute(op.InvalidateCmd{})
	})
}

func (p *accountsPage) layoutAccountSettings(gtx layout.Context, view ui.View) layout.Dimensions {
	m, l := p.maintenance, view.Language
	d := m.draft
	if d == nil {
		return layout.Dimensions{}
	}
	origin := view.Origin
	readOnly := !canEditAccount(d.record)
	role := i18n.AccountRoleAsset
	a := d.record.Account
	if a.IsLiability() {
		role = i18n.AccountRoleLiability
	}
	rows := []ui.ColumnChild{
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			label := fmt.Sprintf(l.Text(i18n.AccountMaintenanceFixed), l.Text(role), l.Text(accountTrackingLabel(a.TrackingMode, a.AccountType)), a.DefaultCurrency.String())
			return fieldPosition(gtx, &origin, view.Message(label, view.Colors.SecondaryInk))
		}),
	}
	if !readOnly {
		rows = append(rows,
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.focusName && gtx.Enabled() {
					d.name.Focus(gtx)
					d.focusName = false
				}
				return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
					return d.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCreateName})
				})
			}),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
					return d.layoutAccountSettingsOwners(gtx, view, origin, p.current.members)
				})
			}),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if d.details.Clicked(gtx) {
					d.showDetails = !d.showDetails
					gtx.Execute(op.InvalidateCmd{})
				}
				return fieldPosition(gtx, &origin, analysisDisclosure(view, &d.details, l.Text(i18n.AccountStepDetails), d.showDetails))
			}),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if !d.showDetails {
					return layout.Dimensions{}
				}
				return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
					return d.layoutAccountSettingsDetails(gtx, view, origin, p.current)
				})
			}),
		)
	} else if a.ArchivedAt != nil {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.AccountMaintenanceReadOnly), view.Colors.SecondaryInk)))
	}
	if m.issue != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(m.issue), view.Colors.ErrorInk)))
	}
	if p.err != nil {
		rows = append(rows, ui.Rigid(accountDetailButton(view, &p.reload, i18n.HistoryRetry, ui.ButtonOutline, p.read)))
	}
	if m.pending != nil || p.request != nil || m.review != nil {
		gtx = gtx.Disabled()
	}
	return (ui.Column{Gap: ui.Space8}).Layout(gtx, rows...)
}

func (p *accountsPage) accountSettingsFooter(view ui.View, submit *bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		m, l := p.maintenance, view.Language
		if m.draft == nil {
			return layout.Dimensions{}
		}
		d := m.draft
		if m.pending == nil && m.review == nil && d.cancel.Clicked(gtx) {
			m.draft, m.focus, m.issue = nil, m.entry, i18n.NoMessage
			gtx.Execute(op.InvalidateCmd{})
			return layout.Dimensions{}
		}
		if m.pending != nil || m.review != nil {
			gtx = gtx.Disabled()
		}
		if p.freshDetail() == nil {
			gtx = gtx.Disabled()
		}
		actions := []layout.Widget{
			func(gtx layout.Context) layout.Dimensions {
				return p.layoutAccountArchiveAction(gtx, view, d.record.Account)
			},
			view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)),
		}
		if canEditAccount(d.record) {
			actions = append(actions, func(gtx layout.Context) layout.Dimensions {
				if len(d.owners) == 0 {
					gtx = gtx.Disabled()
				}
				*submit = d.save.Clicked(gtx)
				return view.PrimaryButton(&d.save, l.Text(i18n.SettingsSave))(gtx)
			})
		}
		return accountDetailActions(view, actions)(gtx)
	}
}

func (d *accountSettingsDraft) layoutAccountSettingsDetails(gtx layout.Context, view ui.View, origin image.Point, current *accountsResult) layout.Dimensions {
	l := view.Language
	institutions := []ui.Choice{{Label: l.Text(i18n.AccountNoInstitution)}}
	for _, entry := range current.institutions {
		if entry.ArchivedAt == nil || entry.ID.String() == d.institution {
			institutions = append(institutions, ui.Choice{ID: entry.ID.String(), Label: accountReference(l, entry.Name, entry.ArchivedAt != nil), Disabled: entry.ArchivedAt != nil})
		}
	}
	groups := []ui.Choice{{Label: l.Text(i18n.AccountNoGroup)}}
	for _, entry := range current.groups {
		if entry.ArchivedAt == nil || entry.ID.String() == d.group {
			groups = append(groups, ui.Choice{ID: entry.ID.String(), Label: accountReference(l, entry.Name, entry.ArchivedAt != nil), Disabled: entry.ArchivedAt != nil})
		}
	}
	for _, options := range [][]ui.Choice{institutions[1:], groups[1:]} {
		slices.SortFunc(options, func(a, b ui.Choice) int { return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.ID, b.ID)) })
	}
	picker := func(label i18n.MessageID, selectBox *ui.Select, options ui.SelectOptions, value string, assign func(string)) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			selectBox.Layout(gtx, view, ui.PickerField{Label: label, Origin: origin, Disabled: !gtx.Enabled()}, options, value, assign)
			origin.Y += gtx.Dp(68)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		})
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		picker(i18n.AccountMaintenanceInstitution, &d.institutionPicker, ui.SelectOptions{Items: institutions, Searchable: true}, d.institution, func(id string) { d.institution = id }),
		picker(i18n.AccountMaintenanceGroup, &d.groupPicker, ui.SelectOptions{Items: groups, Searchable: true}, d.group, func(id string) { d.group = id }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layoutAccountIcon(gtx, view, origin, &d.iconPicker, d.icon, func(id string) { d.icon = id })
		}),
	)
}

func (d *accountSettingsDraft) layoutAccountSettingsOwners(gtx layout.Context, view ui.View, origin image.Point, entries map[domain.MemberID]domain.Member) layout.Dimensions {
	l := view.Language
	var members []domain.Member
	retained := d.record.Ownership.Shares()
	for id, entry := range entries {
		if entry.ArchivedAt == nil || slices.ContainsFunc(retained, func(share domain.OwnershipShare) bool { return share.MemberID == id }) {
			members = append(members, entry)
		}
	}
	slices.SortFunc(members, func(a, b domain.Member) int {
		return cmp.Or(cmp.Compare(a.SortOrder, b.SortOrder), cmp.Compare(a.Name, b.Name), cmp.Compare(a.ID, b.ID))
	})
	return ui.Column{Gap: 6}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			a := d.record.Account
			choices := accountTypeChoices(l, a.BalanceSheetRole, a.TrackingMode)
			choices = slices.DeleteFunc(choices, func(choice ui.Choice) bool { return !compatibleAccountType(a, domain.AccountType(choice.ID)) })
			d.typePicker.Layout(gtx, view, ui.PickerField{Label: i18n.AccountMaintenanceType, Origin: origin, Disabled: !gtx.Enabled() || len(choices) == 1},
				ui.SelectOptions{Items: choices}, d.kind.String(), func(id string) { d.kind = domain.AccountType(id) })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.include.Clicked(gtx) {
				d.included = !d.included
			}
			label := i18n.AccountCreateIncluded
			if !d.included {
				label = i18n.AccountCreateExcluded
			}
			return view.TabButton(&d.include, l.Text(label), d.included)(gtx)
		}),
		ui.Rigid(view.Message(l.Text(i18n.AccountCreateOwnerLabel), view.Colors.Ink)),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions { return d.layoutShareMode(gtx, view) }),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if len(members) > 12 {
				gtx.Constraints.Max.Y = min(gtx.Constraints.Max.Y, gtx.Dp(280))
			}
			return d.layoutOwnerList(gtx, view, members, true)
		}),
	)
}

func accountIconOptions(view ui.View, icon string) (ui.SelectOptions, string) {
	fallback := ui.Choice{Label: view.Language.Text(i18n.AccountMaintenanceDefaultIcon)}
	choices := append([]ui.Choice{fallback}, institutionIconChoices[view.Language]...)
	options := ui.SelectOptions{Items: choices, Searchable: true, FoldSearch: banklogos.FoldSearch}
	if choice, ok := directoryBankLogos[icon]; ok {
		icon, options.ValueLabel = choice.ID, choice.Label
	}
	return options, icon
}

func layoutAccountIcon(gtx layout.Context, view ui.View, origin image.Point, picker *ui.Select, icon string, assign func(string)) layout.Dimensions {
	options, value := accountIconOptions(view, icon)
	picker.Layout(gtx, view, ui.PickerField{Label: i18n.DirectoryIcon, Origin: origin, Disabled: !gtx.Enabled()}, options, value, assign)
	return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
}

func accountGlyph(a domain.Account, institutions map[domain.InstitutionID]domain.Institution) ui.Glyph {
	icon := refValue(a.IconKey)
	if icon == "" && a.InstitutionID != nil {
		icon = refValue(institutions[*a.InstitutionID].IconKey)
	}
	if logo, ok := directoryBankLogos[icon]; ok {
		return logo.Glyph
	}
	if glyph := directoryGlyphs[icon]; glyph != nil {
		return glyph
	}
	return directoryGlyphs[domain.DefaultAccountIcon(a.AccountType)]
}
