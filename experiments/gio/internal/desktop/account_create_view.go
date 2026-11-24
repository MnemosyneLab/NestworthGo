package desktop

import (
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

var bankAccountCurrencies = func() []ui.Choice {
	var choices []ui.Choice
	for _, currency := range domain.SupportedCurrencies() {
		code := currency.String()
		choices = append(choices, ui.Choice{ID: code, Label: code})
	}
	return choices
}()

func (p *accountsPage) layoutBankAccount(gtx layout.Context, view ui.View) layout.Dimensions {
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
	return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(ui.LimitWidth(gtx, 680),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			dims := layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, l.Text(i18n.AccountCreateTitle), 19, font.Medium, view.Colors.Ink)
				}),
				layout.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name), view.Colors.SecondaryInk)),
			)
			origin.Y += dims.Size.Y + gap
			return dims
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			if !ready {
				id, ink := i18n.AccountCreateInstitutionUnavailable, view.Colors.ErrorInk
				if d.refs != nil {
					id, ink = i18n.AccountCreateReferencesLoading, view.Colors.SecondaryInk
				} else if d.loadFailed {
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
					p.loadBankAccountReferences()
				}
				buttons = append(buttons, layout.Rigid(view.Button(&d.retry, l.Text(i18n.HistoryRetry))))
			}
			if ready {
				if d.next.Clicked(gtx) {
					f.prepareReview()
					gtx.Execute(op.InvalidateCmd{})
				}
				buttons = append(buttons, layout.Rigid(view.PrimaryButton(&d.next, l.Text(i18n.AccountCreateReview))))
			}
			return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx, buttons...)
		}),
	)
}

func (d *bankAccountDraft) layoutFields(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	l := view.Language
	return layout.Flex{Gap: gtx.Dp(16)}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					d.currencySelect.Layout(gtx, view, ui.PickerField{
						Label: i18n.AccountCreateCurrencyLabel, Origin: origin, Next: &d.name.Editor,
						Disabled: !gtx.Enabled(),
					}, ui.SelectOptions{Items: bankAccountCurrencies}, d.currency.String(), func(value string) {
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
					return d.opening.Layout(gtx, view, ui.FieldOptions{Label: i18n.AccountCreateOpening, Hint: i18n.AccountCreateAmountHint})
				}),
			)
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Rigid(view.Message(l.Text(i18n.AccountCreateOwnerLabel), view.Colors.Ink)),
				layout.Rigid(view.Message(l.Text(i18n.AccountCreateEqualShares), view.Colors.SecondaryInk)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					label := i18n.AccountCreateIncluded
					if !d.included {
						label = i18n.AccountCreateExcluded
					}
					if d.include.Clicked(gtx) {
						d.included = !d.included
						gtx.Execute(op.InvalidateCmd{})
					}
					return view.TabButton(&d.include, l.Text(label), d.included)(gtx)
				}),
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
		}),
	)
}

func (p *accountsPage) layoutBankAccountReview(gtx layout.Context, view ui.View) {
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
	body := strings.Join([]string{
		r.input.Name,
		fmt.Sprintf(l.Text(i18n.AccountInstitution), d.institution.Name),
		fmt.Sprintf(l.Text(i18n.AccountCreateReviewBalance), r.money.CanonicalAmount()+" "+r.money.Currency().String()),
		fmt.Sprintf(l.Text(i18n.AccountOwners), strings.Join(owners, " / ")),
		l.Text(include),
	}, "\n")
	problem := ""
	if f.submission != nil {
		problem = l.Text(i18n.AccountCreateSaving)
	}
	f.confirmation.Layout(gtx, view, ui.ConfirmationOptions{
		Title: l.Text(i18n.AccountCreateReviewTitle), Body: body, Busy: f.submission != nil, Problem: problem,
		Cancel: l.Text(i18n.AccountCreateBackToDetails), Confirm: l.Text(i18n.AccountCreateConfirm),
	}, func() {
		f.review = nil
		gtx.Execute(op.InvalidateCmd{})
	}, p.submitBankAccount)
}
