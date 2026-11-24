package desktop

import (
	"cmp"
	"fmt"
	"image"
	"slices"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/banklogos"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *accountsPage) accountMaintenanceActions(gtx layout.Context, view ui.View) []layout.FlexChild {
	m, row := p.maintenance, p.freshDetail()
	if m == nil || row == nil || !accountMaintenanceSupported(row.record.Account) {
		return nil
	}
	a, l := row.record.Account, view.Language
	if m.focus != nil && gtx.Enabled() {
		gtx.Execute(key.FocusCmd{Tag: m.focus})
		m.focus = nil
	}
	var result []layout.FlexChild
	if canEditAccount(row.record) {
		if m.edit.Clicked(gtx) {
			m.startEdit(row.record)
			gtx.Execute(op.InvalidateCmd{})
		}
		result = append(result, layout.Rigid(view.Button(&m.edit, l.Text(i18n.AccountMaintenanceEdit))))
	}
	action := i18n.AccountMaintenanceArchive
	if a.ArchivedAt != nil {
		action = i18n.AccountMaintenanceRestore
	}
	if m.archive.Clicked(gtx) {
		m.startArchive(a)
		gtx.Execute(op.InvalidateCmd{})
	}
	return append(result, layout.Rigid(view.Button(&m.archive, l.Text(action))))
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
		Cancel: l.Text(i18n.InstitutionsCancel), Confirm: l.Text(title), Busy: m.pending != nil,
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
	gtx = ui.LimitWidth(gtx, 760)
	origin := view.Origin
	submit := false
	dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, l.Text(i18n.AccountMaintenanceEdit), 19, font.Medium, view.Colors.Ink)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			role := i18n.AccountRoleAsset
			if d.record.Account.IsLiability() {
				role = i18n.AccountRoleLiability
			}
			a := d.record.Account
			label := fmt.Sprintf(l.Text(i18n.AccountMaintenanceFixed), l.Text(role), l.Text(accountTrackingLabel(a.TrackingMode, a.AccountType)), a.DefaultCurrency.String())
			return fieldPosition(gtx, &origin, view.Message(label, view.Colors.SecondaryInk))
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if m.pending != nil || p.request != nil || p.err != nil {
				gtx = gtx.Disabled()
			}
			gap := gtx.Dp(16)
			width := (gtx.Constraints.Max.X - gap) / 2
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X, gtx.Constraints.Min.X = width, width
					return d.layoutAccountSettingsDetails(gtx, view, origin, p.current)
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return d.layoutAccountSettingsOwners(gtx, view, origin.Add(image.Pt(width+gap, 0)), p.current.members)
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			var lines []layout.FlexChild
			if m.issue != i18n.NoMessage {
				lines = append(lines, layout.Rigid(view.Message(l.Text(m.issue), view.Colors.ErrorInk)))
			}
			if p.err != nil {
				lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.AccountLoadError), view.Colors.ErrorInk)))
			}
			if m.pending != nil {
				lines = append(lines, layout.Rigid(view.Message(l.Text(i18n.SettingsPending), view.Colors.SecondaryInk)))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if m.pending != nil {
				gtx = gtx.Disabled()
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.cancel.Clicked(gtx) {
						m.draft, m.focus, m.issue = nil, &m.edit, i18n.NoMessage
						gtx.Execute(op.InvalidateCmd{})
					}
					return view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel))(gtx)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if p.err != nil {
						if p.reload.Clicked(gtx) {
							p.read()
						}
						return view.Button(&p.reload, l.Text(i18n.HistoryRetry))(gtx)
					}
					if p.freshDetail() == nil || len(d.owners) == 0 {
						gtx = gtx.Disabled()
					}
					submit = d.save.Clicked(gtx)
					return view.PrimaryButton(&d.save, l.Text(i18n.SettingsSave))(gtx)
				}),
			)
		}),
	)
	if submit && m.draft != nil {
		input, issue := d.input()
		m.issue = issue
		if issue == i18n.NoMessage {
			p.submitAccountMaintenance(nil, input)
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
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
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.focusName && gtx.Enabled() {
				d.name.Focus(gtx)
				d.focusName = false
			}
			return fieldPosition(gtx, &origin, func(gtx layout.Context) layout.Dimensions {
				return d.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCreateName})
			})
		}),
		picker(i18n.AccountMaintenanceInstitution, &d.institutionPicker, ui.SelectOptions{Items: institutions, Searchable: true}, d.institution, func(id string) { d.institution = id }),
		picker(i18n.AccountMaintenanceGroup, &d.groupPicker, ui.SelectOptions{Items: groups, Searchable: true}, d.group, func(id string) { d.group = id }),
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
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(8)
			width := (gtx.Constraints.Max.X - gap) / 2
			return layout.Flex{Gap: gap}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = width
					a := d.record.Account
					choices := accountTypeChoices(l, a.BalanceSheetRole, a.TrackingMode)
					choices = slices.DeleteFunc(choices, func(choice ui.Choice) bool { return !compatibleAccountType(a, domain.AccountType(choice.ID)) })
					d.typePicker.Layout(gtx, view, ui.PickerField{Label: i18n.AccountMaintenanceType, Origin: origin, Disabled: !gtx.Enabled() || len(choices) == 1},
						ui.SelectOptions{Items: choices}, d.kind.String(), func(id string) { d.kind = domain.AccountType(id) })
					return layout.Dimensions{Size: image.Pt(width, gtx.Dp(60))}
				}),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return layoutAccountIcon(gtx, view, origin.Add(image.Pt(width+gap, 0)), &d.iconPicker, d.icon, func(id string) { d.icon = id })
				}),
			)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.include.Clicked(gtx) {
				d.included = !d.included
			}
			label := i18n.AccountCreateIncluded
			if !d.included {
				label = i18n.AccountCreateExcluded
			}
			return view.TabButton(&d.include, l.Text(label), d.included)(gtx)
		}),
		layout.Rigid(view.Message(l.Text(i18n.AccountCreateOwnerLabel), view.Colors.Ink)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return d.layoutShareMode(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
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
