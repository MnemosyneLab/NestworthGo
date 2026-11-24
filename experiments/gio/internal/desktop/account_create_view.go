package desktop

import (
	"cmp"
	"fmt"
	"image"
	"slices"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/format"
)

var supportedCurrencyChoices = func() []ui.Choice {
	var choices []ui.Choice
	for _, currency := range domain.SupportedCurrencies() {
		code := currency.String()
		choices = append(choices, ui.Choice{ID: code, Label: code})
	}
	return choices
}()

func (p *accountsPage) layoutAccountCreation(gtx layout.Context, view ui.View) layout.Dimensions {
	f, l := p.creation, view.Language
	d := f.draft
	if d.cancel.Clicked(gtx) {
		f.discard()
		p.read()
		gtx.Execute(op.InvalidateCmd{})
		return layout.Dimensions{}
	}
	ready := d.refs == nil && d.available && !d.loadFailed
	gap := gtx.Dp(10)
	origin := view.Origin
	title := i18n.AccountCreateTitle
	contextLabel := fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name)
	if d.liability() {
		title, contextLabel = i18n.AccountCreateLiabilityTitle, l.Text(i18n.AccountCreateLiabilityHelp)
	} else if !d.scoped {
		title, contextLabel = i18n.AccountCreateAssetTitle, l.Text(i18n.AccountCreateAssetHelp)
	}
	review := false
	dims := layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(ui.LimitWidth(gtx, 680),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, l.Text(title), 19, font.Medium, view.Colors.Ink)
				}),
				layout.Rigid(view.Message(contextLabel, view.Colors.SecondaryInk)),
			)
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !ready {
				id, ink := i18n.AccountCreateInstitutionUnavailable, view.Colors.ErrorInk
				if d.refs != nil {
					id, ink = i18n.AccountCreateReferencesLoading, view.Colors.SecondaryInk
				} else if d.loadFailed || !d.scoped {
					id = i18n.AccountCreateReferencesFailed
				}
				return view.Message(l.Text(id), ink)(gtx)
			}
			return d.layoutFields(gtx, view, origin)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if !ready || d.problem == i18n.NoMessage {
				return layout.Dimensions{}
			}
			return view.Message(l.Text(d.problem), view.Colors.ErrorInk)(gtx)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			buttons := []layout.FlexChild{layout.Rigid(view.Button(&d.cancel, l.Text(i18n.InstitutionsCancel)))}
			if d.loadFailed {
				if d.retry.Clicked(gtx) {
					p.loadAccountCreationReferences()
				}
				buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
			}
			if ready {
				review = d.next.Clicked(gtx)
				buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.next, l.Text(i18n.AccountCreateReview))))
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
		}),
	)
	if review {
		f.prepareReview()
		gtx.Execute(op.InvalidateCmd{})
	}
	return dims
}

func (d *balanceAccountDraft) layoutFields(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	gap := gtx.Dp(16)
	width := (gtx.Constraints.Max.X - gap) / 2
	return layout.Flex{Gap: gap}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			fields := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d.currencySelect.Layout(gtx, view, ui.PickerField{
						Label: i18n.AccountCreateCurrencyLabel, Origin: origin, Next: &d.name.Editor,
						Disabled: !gtx.Enabled(),
					}, ui.SelectOptions{Items: supportedCurrencyChoices}, d.currency.String(), func(value string) {
						d.currency = domain.CurrencyCode(value)
					})
					return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if d.focusName && gtx.Enabled() {
						d.name.Focus(gtx)
						d.focusName = false
					}
					return d.name.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCreateName})
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := i18n.AccountCreateOpening
					if d.liability() {
						label = i18n.AccountCreateOpeningOwed
					}
					return d.opening.Layout(gtx, view, ui.FieldOptions{Label: label, Hint: i18n.AccountCreateAmountHint})
				}),
			}
			if !d.scoped {
				fields = append(fields, layout.Rigid(d.inclusion(view)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx, fields...)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			var fields []layout.FlexChild
			if !d.scoped {
				fields = d.directoryFields(view, origin.Add(image.Pt(width+gap, 0)))
			}
			fields = append(fields,
				layout.Rigid(view.Message(l.Text(i18n.AccountCreateOwnerLabel), view.Colors.Ink)),
				layout.Rigid(view.Message(l.Text(i18n.AccountCreateEqualShares), view.Colors.SecondaryInk)),
			)
			if d.scoped {
				fields = append(fields, layout.Rigid(d.inclusion(view)))
			}
			fields = append(fields,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return material.List(view.Theme, &d.ownerList).Layout(gtx, len(d.members), func(gtx layout.Context, i int) layout.Dimensions {
						member := d.members[i]
						button := d.ownerButtons[member.ID]
						if button == nil {
							button = new(widget.Clickable)
							d.ownerButtons[member.ID] = button
						}
						index := slices.Index(d.owners, member.ID)
						if button.Clicked(gtx) {
							if index < 0 {
								d.owners = append(d.owners, member.ID)
							} else {
								d.owners = slices.Delete(d.owners, index, index+1)
							}
						}
						return layout.Inset{Bottom: 4}.Layout(gtx, view.TabButton(button, member.Name, slices.Contains(d.owners, member.ID)))
					})
				}),
			)
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx, fields...)
		}),
	)
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
		return view.TabButton(&d.include, view.Language.Text(label), d.included)(gtx)
	}
}

func balanceTypeChoices(l i18n.Language, role domain.BalanceSheetRole) []ui.Choice {
	var result []ui.Choice
	for _, option := range domain.LegalAccountCombinations() {
		if option.BalanceSheetRole == role && option.TrackingMode == domain.TrackingBalance {
			result = append(result, ui.Choice{ID: option.AccountType.String(), Label: l.Text(accountTypes[option.AccountType])})
		}
	}
	return result
}

func (d *balanceAccountDraft) directoryFields(view ui.View, origin image.Point) []layout.FlexChild {
	l := view.Language
	typeLabel := i18n.AccountCreateAssetType
	if d.liability() {
		typeLabel = i18n.AccountCreateLiabilityType
	}
	institutions := []ui.Choice{{Label: l.Text(i18n.AccountNoInstitution)}}
	for _, item := range d.institutions {
		institutions = append(institutions, ui.Choice{ID: item.ID.String(), Label: item.Name})
	}
	slices.SortFunc(institutions[1:], func(a, b ui.Choice) int { return cmp.Or(cmp.Compare(a.Label, b.Label), cmp.Compare(a.ID, b.ID)) })
	institutionOptions := ui.SelectOptions{Items: institutions, Searchable: true}
	if d.institution.ID != "" && d.institutionIndex() < 0 {
		institutionOptions.ValueLabel = d.institution.Name + " · " + l.Text(i18n.AccountReferenceUnavailable)
		institutionOptions.Invalid = true
	}
	return []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			d.typeSelect.Layout(gtx, view, ui.PickerField{Label: typeLabel, Origin: origin, Disabled: !gtx.Enabled()},
				ui.SelectOptions{Items: balanceTypeChoices(l, d.role)}, d.kind.String(), func(value string) { d.kind = domain.AccountType(value) })
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			field := ui.PickerField{Label: i18n.AccountMaintenanceInstitution, Origin: origin.Add(image.Pt(0, gtx.Dp(66))), Disabled: !gtx.Enabled()}
			d.institutionSelect.Layout(gtx, view, field, institutionOptions, d.institution.ID.String(), func(value string) {
				d.institution = domain.Institution{}
				for _, item := range d.institutions {
					if item.ID.String() == value {
						d.institution = item
						break
					}
				}
				if d.problem == i18n.AccountCreateChooseInstitution {
					d.problem = i18n.NoMessage
				}
			})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(60))}
		}),
	}
}

func (p *accountsPage) layoutAccountCreationReview(gtx layout.Context, view ui.View) {
	f, l := p.creation, view.Language
	r, d := f.review, f.draft
	owners := make([]string, 0, min(3, len(r.shares)))
	for _, share := range r.shares[:min(3, len(r.shares))] {
		if i := slices.IndexFunc(d.members, func(member domain.Member) bool { return member.ID == share.MemberID }); i >= 0 {
			owners = append(owners, d.members[i].Name+" · "+format.ShareBPS(share.ShareBPS))
		}
	}
	if len(r.shares) > 3 {
		owners = append(owners, fmt.Sprintf(l.Text(i18n.AccountCreateMoreOwners), len(r.shares)-3))
	}
	include := i18n.AccountCreateExcluded
	if r.input.IncludeInNetWorth {
		include = i18n.AccountCreateIncluded
	}
	title, confirm, amount, saving := i18n.AccountCreateReviewTitle, i18n.AccountCreateConfirm, i18n.AccountCreateReviewBalance, i18n.AccountCreateSaving
	institution := fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name)
	lines := []string{r.input.Name}
	if d.liability() {
		title, confirm, amount, saving = i18n.AccountCreateLiabilityReview, i18n.AccountCreateLiabilityConfirm, i18n.AccountCreateReviewOwed, i18n.SettingsPending
	} else if !d.scoped {
		title, confirm, saving = i18n.AccountCreateAssetReview, i18n.AccountCreateAssetConfirm, i18n.SettingsPending
	}
	if !d.scoped {
		lines = append(lines, l.Text(accountTypes[d.kind]))
		if d.institution.ID == "" {
			institution = l.Text(i18n.AccountNoInstitution)
		}
	}
	body := strings.Join(append(lines,
		institution,
		fmt.Sprintf(l.Text(amount), r.money.CanonicalAmount()+" "+r.money.Currency().String()),
		fmt.Sprintf(l.Text(i18n.AccountOwners), strings.Join(owners, " / ")),
		l.Text(include),
	), "\n")
	problem := ""
	if f.submission != nil {
		problem = l.Text(saving)
	}
	f.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: l.Text(title), Body: body, Busy: f.submission != nil, Problem: problem,
		Cancel: l.Text(i18n.AccountCreateBackToDetails), Confirm: l.Text(confirm),
	}, func() {
		f.review = nil
		gtx.Execute(op.InvalidateCmd{})
	}, p.submitNewAccount)
}
