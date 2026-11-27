package desktop

import (
	"cmp"
	"fmt"
	"image"
	"slices"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

// formCard is a titled section of the creation form. The body receives a view
// whose origin is its own top-left corner, so selects inside keep their popups
// anchored whatever the heading and padding measure.
func formCard(gtx layout.Context, view ui.View, origin image.Point, heading string, body func(gtx layout.Context, view ui.View) layout.Dimensions) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	pad, gap := gtx.Dp(ui.Space20), gtx.Dp(ui.Space12)
	return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(ui.Space20).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			inner := view
			return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					dims := view.Line(gtx, ui.TextHeading, heading, view.Colors.Ink)
					inner.Origin = origin.Add(image.Pt(pad, pad+dims.Size.Y+gap))
					return dims
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return body(gtx, inner) }),
			)
		})
	})
}

func (p *accountsPage) layoutAccountCreation(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	d := f.draft
	if d.newInstitution != nil {
		return p.layoutInlineInstitution(gtx, view)
	}
	if d.cancel.Clicked(gtx) {
		p.cancelAccountCreate(gtx)
		return layout.Dimensions{}
	}
	if c := f.chooser; c != nil && c.back.Clicked(gtx) {
		f.advanceFromInstitution()
		if len(c.modes) == 1 {
			f.sheet.move(accountInstitution)
		}
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	ready := d.refs == nil && d.available && !d.loadFailed
	gap := gtx.Dp(ui.Space16)
	title := i18n.AccountCreateTitle
	contextLabel := fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name)
	if d.shell() {
		title = i18n.AccountCreateShellTitle
		if !d.scoped {
			contextLabel = l.Text(i18n.AccountCreateShellHelp)
		}
	} else if d.liability() {
		title, contextLabel = i18n.AccountCreateLiabilityTitle, l.Text(i18n.AccountCreateLiabilityHelp)
	} else if !d.scoped {
		title, contextLabel = i18n.AccountCreateAssetTitle, l.Text(i18n.AccountCreateAssetHelp)
	}
	kindLabel := ""
	if f.chooser != nil {
		institution := l.Text(i18n.AccountNoInstitution)
		if d.institution.ID != "" {
			institution = d.institution.Name
		}
		kindLabel = institution + " · " + l.Text(accountTypes[d.kind]) + " · " + l.Text(accountTrackingLabel(d.mode, d.kind))
		if !d.liability() {
			contextLabel = ""
		}
	}
	review := false
	gtx, view = view.Measure(gtx, 720)
	origin := view.Origin
	dims := ui.Column{Gap: ui.Space16}.Layout(gtx,
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return accountStepper(gtx, view, 2) }),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, l.Text(title), view.Colors.Ink))}
					if kindLabel != "" {
						rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, kindLabel, view.Colors.SecondaryInk)))
					}
					if contextLabel != "" {
						rows = append(rows, ui.Rigid(view.Body(ui.TextCaption, contextLabel, view.Colors.SecondaryInk)))
					}
					return (ui.Column{Gap: ui.Space4}).Layout(gtx, rows...)
				}),
			)
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !ready {
				id, ink := i18n.AccountCreateInstitutionUnavailable, view.Colors.ErrorInk
				if d.refs != nil {
					id, ink = i18n.AccountCreateReferencesLoading, view.Colors.SecondaryInk
				} else if d.loadFailed || !d.scoped {
					id = i18n.AccountCreateReferencesFailed
				}
				return view.Message(l.Text(id), ink)(gtx)
			}
			if d.showOpeningOptions {
				return d.layoutOpeningOptions(gtx, view, origin)
			}
			return d.layoutFields(gtx, view, origin)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			var lines []layout.FlexChild
			if d.savedInstitution != "" {
				label := fmt.Sprintf(l.Text(i18n.AccountCreateInstitutionSaved), d.savedInstitution)
				lines = append(lines, layout.Rigid(view.Message(label, view.Colors.SecondaryInk)))
			}
			if !ready || d.problem == i18n.NoMessage {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx, lines...)
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx, append(lines, layout.Rigid(view.Message(l.Text(d.problem), view.Colors.ErrorInk)))...)
		}),
		ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			leading := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
			if f.chooser != nil {
				leading = append(leading, layout.Rigid(view.Button(&f.chooser.back, l.Text(i18n.AccountCreateBack))))
			}
			var trailing []layout.FlexChild
			if d.loadFailed {
				if d.retry.Clicked(gtx) {
					p.loadAccountCreationReferences()
				}
				trailing = append(trailing, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
			}
			if ready {
				label := i18n.AccountCreateOpeningOptions
				if d.showOpeningOptions {
					label = i18n.AccountCreateBackToDetails
				}
				if d.options.Clicked(gtx) {
					d.showOpeningOptions = !d.showOpeningOptions
					gtx.Execute(op.InvalidateCmd{})
				}
				trailing = append(trailing, layout.Rigid(view.Button(&d.options, l.Text(label))))
				review = d.next.Clicked(gtx)
				trailing = append(trailing, layout.Rigid(view.PrimaryButton(&d.next, l.Text(i18n.AccountCreateReview))))
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Flex{Spacing: layout.SpaceBetween, Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx, leading...)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(ui.Space8)}.Layout(gtx, trailing...)
				}),
			)
		}),
	)
	if review {
		f.prepareReview()
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

// layoutFields lays out the two sections of the details step: the account
// itself and its owners.
func (d *balanceAccountDraft) layoutFields(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	gap := gtx.Dp(ui.Space16)
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := formCard(gtx, view, origin, l.Text(i18n.AccountCreateSectionBasics), d.layoutBasics)
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formCard(gtx, view, origin, l.Text(i18n.AccountCreateOwnerLabel), func(gtx layout.Context, view ui.View) layout.Dimensions {
				shareMode := i18n.AccountCreateEqualShares
				if d.custom {
					shareMode = i18n.AccountMaintenanceCustom
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(view.Body(ui.TextCaption, l.Text(shareMode), view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return d.layoutOwnerList(gtx, view, d.members, false)
					}),
				)
			})
		}),
	)
}

// layoutBasics is the account section: its name, type, institution, currency
// and opening amount in a grid, then the net worth toggle.
func (d *balanceAccountDraft) layoutBasics(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	gap := gtx.Dp(ui.Space12)
	at := view.Origin
	step := func(dims layout.Dimensions) layout.Dimensions {
		at.Y += dims.Size.Y + gap
		return dims
	}
	var cells []func(gtx layout.Context, view ui.View) layout.Dimensions
	if !d.scoped || d.shell() {
		cells = append(cells, d.directoryCells(view)...)
	}
	cells = append(cells, func(gtx layout.Context, view ui.View) layout.Dimensions {
		next := &d.name.Editor
		if d.mode != domain.TrackingHoldings {
			next = &d.opening.Editor
		}
		d.currencySelect.Layout(gtx, view, ui.PickerField{
			Label: i18n.AccountCreateCurrencyLabel, Origin: view.Origin, Next: next, Previous: &d.name.Editor,
			Disabled: !gtx.Enabled(),
		}, ui.SelectOptions{Items: supportedCurrencyChoices}, d.currency.String(), func(value string) {
			d.currency = domain.CurrencyCode(value)
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	})
	if d.mode != domain.TrackingHoldings {
		cells = append(cells, func(gtx layout.Context, view ui.View) layout.Dimensions {
			label := i18n.AccountCreateOpening
			if d.liability() {
				label = i18n.AccountCreateOpeningOwed
			} else if d.mode == domain.TrackingManualValue {
				label = i18n.AccountCreateOpeningValuation
			}
			return d.opening.Layout(gtx, view, ui.FieldOptions{Label: label, Hint: i18n.AccountCreateAmountHint})
		})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.focusName && gtx.Enabled() {
				d.name.Focus(gtx)
				d.focusName = false
			}
			return step(d.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCreateName}))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			grid := view
			grid.Origin = at
			return step((ui.Grid{MinColumn: 240, MaxColumns: 2, Gap: ui.Space12}).Layout(gtx, grid, len(cells),
				func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return cells[i](gtx, view) }))
		}),
	}
	if d.mode == domain.TrackingHoldings {
		children = append(children, layout.Rigid(view.Body(ui.TextCaption, l.Text(i18n.AccountCreateHoldingsEmpty), view.Colors.SecondaryInk)))
	}
	if !d.scoped {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if d.focusAddInstitution && gtx.Enabled() {
				gtx.Execute(key.FocusCmd{Tag: &d.addInstitution})
				d.focusAddInstitution = false
			}
			if d.addInstitution.Clicked(gtx) {
				d.startInstitution()
				gtx.Execute(op.InvalidateCmd{})
			}
			gtx.Constraints.Min = image.Point{}
			return view.ButtonOf(&d.addInstitution, ui.ButtonSpec{
				Variant: ui.ButtonGhost, Size: ui.ButtonSmall, Title: l.Text(i18n.InstitutionsAdd), Name: l.Text(i18n.InstitutionsAdd),
			})(gtx)
		}))
	}
	children = append(children, layout.Rigid(d.inclusion(view)))
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx, children...)
}

func (d *balanceAccountDraft) inclusion(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if d.include.Clicked(gtx) {
			d.included = !d.included
			gtx.Execute(op.InvalidateCmd{})
		}
		label := i18n.AccountCreateIncluded
		if !d.included {
			label = i18n.AccountCreateExcluded
		}
		return ownerToggle(view, &d.include, view.Language.Text(label), d.included)(gtx)
	}
}

// directoryCells are the selects that classify the account: tracking method,
// type and institution.
func (d *balanceAccountDraft) directoryCells(view ui.View) []func(gtx layout.Context, view ui.View) layout.Dimensions {
	l := view.Language
	typeLabel := i18n.AccountCreateAssetType
	if d.liability() {
		typeLabel = i18n.AccountCreateLiabilityType
	}
	var cells []func(gtx layout.Context, view ui.View) layout.Dimensions
	picker := func(draw func(gtx layout.Context, view ui.View)) func(gtx layout.Context, view ui.View) layout.Dimensions {
		return func(gtx layout.Context, view ui.View) layout.Dimensions {
			draw(gtx, view)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	cells = append(cells, picker(func(gtx layout.Context, view ui.View) {
		d.typeSelect.Layout(gtx, view, ui.PickerField{Label: typeLabel, Origin: view.Origin, Disabled: !gtx.Enabled()},
			ui.SelectOptions{Items: accountTypeChoices(l, d.role, d.mode)}, d.kind.String(), func(value string) { d.selectType(domain.AccountType(value)) })
	}))
	if modes := d.trackingChoices(l); len(modes) > 1 {
		cells = append(cells, picker(func(gtx layout.Context, view ui.View) {
			d.modeSelect.Layout(gtx, view, ui.PickerField{Label: i18n.AccountTrackingMethod, Origin: view.Origin, Disabled: !gtx.Enabled()},
				ui.SelectOptions{Items: modes}, string(d.mode), func(value string) { d.selectMode(domain.TrackingMode(value)) })
		}))
	}
	if d.scoped {
		return cells
	}
	institutions := []ui.Choice{{Label: l.Text(i18n.AccountNoInstitution)}}
	var others []ui.Choice
	for _, item := range d.institutions {
		choice := ui.Choice{ID: item.ID.String(), Label: item.Name, SearchText: l.Text(institutionTypeLabel(item.InstitutionType))}
		if slices.Contains(accountInstitutionTypes[d.kind], item.InstitutionType) {
			institutions = append(institutions, choice)
		} else {
			others = append(others, choice)
		}
	}
	institutions = append(institutions, others...)
	institutionOptions := ui.SelectOptions{Items: institutions, Searchable: true}
	if d.institution.ID != "" && d.institutionIndex() < 0 {
		institutionOptions.ValueLabel = d.institution.Name + " · " + l.Text(i18n.AccountReferenceUnavailable)
		institutionOptions.Invalid = true
	}
	return append(cells, picker(func(gtx layout.Context, view ui.View) {
		field := ui.PickerField{Label: i18n.AccountMaintenanceInstitution, Origin: view.Origin, Disabled: !gtx.Enabled()}
		d.institutionSelect.Layout(gtx, view, field, institutionOptions, d.institution.ID.String(), d.chooseInstitution)
	}))
}

// trackingChoices are the methods the open form may switch to. Institution
// entry keeps its valued-or-holdings pair. An unscoped form lists only the
// modes that are legal for the account's type and role, including balance.
func (d *balanceAccountDraft) trackingChoices(l i18n.Language) []ui.Choice {
	if d.scoped {
		if !d.shell() {
			return nil
		}
		return []ui.Choice{
			{ID: string(domain.TrackingManualValue), Label: l.Text(i18n.AccountTrackingManual)},
			{ID: string(domain.TrackingHoldings), Label: l.Text(i18n.AccountTrackingHoldings)},
		}
	}
	var choices []ui.Choice
	for _, mode := range accountModes(d.kind, d.role) {
		choices = append(choices, ui.Choice{ID: string(mode), Label: l.Text(accountTrackingLabel(mode, d.kind))})
	}
	return choices
}

// layoutOpeningOptions is the optional second page of details: grouping, the
// opening date, the icon and custom ownership shares.
func (d *balanceAccountDraft) layoutOpeningOptions(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	gap := gtx.Dp(ui.Space16)
	groups := []ui.Choice{{Label: l.Text(i18n.AccountNoGroup)}}
	for _, group := range d.groups {
		groups = append(groups, ui.Choice{ID: group.ID.String(), Label: group.Name})
	}
	slices.SortFunc(groups[1:], func(a, b ui.Choice) int { return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.ID, b.ID)) })
	choices := ui.SelectOptions{Items: groups, Searchable: true}
	if d.group != "" && !slices.ContainsFunc(groups, func(choice ui.Choice) bool { return choice.ID == d.group }) {
		choices.Invalid, choices.ValueLabel = true, l.Text(i18n.AccountReferenceUnavailable)
	}
	picker := func(draw func(gtx layout.Context, view ui.View)) func(gtx layout.Context, view ui.View) layout.Dimensions {
		return func(gtx layout.Context, view ui.View) layout.Dimensions {
			draw(gtx, view)
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
		}
	}
	cells := []func(gtx layout.Context, view ui.View) layout.Dimensions{picker(func(gtx layout.Context, view ui.View) {
		d.groupSelect.Layout(gtx, view, ui.PickerField{Label: i18n.AccountMaintenanceGroup, Origin: view.Origin, Disabled: !gtx.Enabled()}, choices, d.group, func(id string) {
			d.group, d.groupExplicit = id, true
			if d.problem == i18n.AccountCreateGroupUnavailable {
				d.problem = i18n.NoMessage
			}
		})
	})}
	if d.openingDates.Min != "" {
		dates := d.openingDates
		dates.Format, dates.WeekStart = view.DateFormat, time.Monday
		if view.WeekStart == "sunday" {
			dates.WeekStart = time.Sunday
		}
		cells = append(cells, picker(func(gtx layout.Context, view ui.View) {
			d.datePicker.Layout(gtx, view, ui.PickerField{Label: i18n.AccountCreateOpeningDate, Origin: view.Origin, Disabled: !gtx.Enabled()}, dates, d.openedOn, func(day string) {
				d.openedOn = day
				if d.problem == i18n.AccountCreateDateInvalid {
					d.problem = i18n.NoMessage
				}
			})
		}))
	}
	cells = append(cells, func(gtx layout.Context, view ui.View) layout.Dimensions {
		return layoutAccountIcon(gtx, view, view.Origin, &d.iconPicker, d.icon, func(icon string) { d.icon = icon })
	})
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := formCard(gtx, view, origin, l.Text(i18n.AccountCreateOpeningOptions), func(gtx layout.Context, view ui.View) layout.Dimensions {
				return (ui.Grid{MinColumn: 240, MaxColumns: 2, Gap: ui.Space12}).Layout(gtx, view, len(cells),
					func(gtx layout.Context, view ui.View, i int) layout.Dimensions { return cells[i](gtx, view) })
			})
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return formCard(gtx, view, origin, l.Text(i18n.AccountCreateOwnerLabel), func(gtx layout.Context, view ui.View) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min = image.Point{}
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return d.layoutShareMode(gtx, view) })
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return d.layoutOwnerList(gtx, view, d.members, true)
					}),
				)
			})
		}),
	)
}
