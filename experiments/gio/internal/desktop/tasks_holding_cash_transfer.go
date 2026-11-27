package desktop

import (
	"image"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func (p *historyCashMovements) layoutCheckpointTransferFields(view ui.View, origin *image.Point) []layout.FlexChild {
	d, scope, l := p.draft, p.linked, view.Language
	account, _ := d.accountByID(scope.account)
	var peers []ui.Choice
	for _, candidate := range d.sources.Accounts {
		if history.CanTransferTo(account, candidate) {
			peers = append(peers, ui.Choice{ID: candidate.ID.String(), Label: candidate.Name + " · " + candidate.DefaultCurrency.String(), SearchText: candidate.Name})
		}
	}
	return []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, view.Message(account.Name, view.Colors.Ink))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, func(gtx layout.Context) layout.Dimensions {
				if d.send.Clicked(gtx) && d.incoming {
					d.chooseLinkedTransferDirection(scope, false)
					gtx.Execute(op.InvalidateCmd{})
				}
				if d.receive.Clicked(gtx) && !d.incoming {
					d.chooseLinkedTransferDirection(scope, true)
					gtx.Execute(op.InvalidateCmd{})
				}
				return layout.Flex{Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Rigid(view.TabButton(&d.send, l.Text(i18n.CustodySend), !d.incoming)),
					layout.Rigid(view.TabButton(&d.receive, l.Text(i18n.CustodyReceive), d.incoming)))
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return fieldPosition(gtx, origin, func(gtx layout.Context) layout.Dimensions {
				if len(peers) == 0 {
					return view.Message(l.Text(i18n.TransferNoDestination), view.Colors.SecondaryInk)(gtx)
				}
				picker, label, previous := &d.destinationPicker, i18n.TransferTo, &d.send
				if d.incoming {
					picker, label, previous = &d.accountPicker, i18n.TransferFrom, &d.receive
				}
				if d.focusAmount && d.linkedTransferPeer() == "" && gtx.Enabled() {
					gtx.Execute(key.FocusCmd{Tag: &picker.Trigger})
					d.focusAmount = false
				}
				field := ui.PickerField{Label: label, Origin: *origin, Disabled: !gtx.Enabled(), Next: &d.amount.Editor, Previous: previous}
				picker.Layout(gtx, view, field, ui.SelectOptions{Items: peers, Searchable: true}, d.linkedTransferPeer().String(), func(value string) {
					d.chooseLinkedTransferPeer(scope, domain.AccountID(value))
					gtx.Execute(op.InvalidateCmd{})
				})
				return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
			})
		}),
	}
}

func (p *historyCashMovements) checkpointDebtPeer(view ui.View, origin image.Point, account domain.Account) layout.Widget {
	d, l := p.draft, view.Language
	var peers []ui.Choice
	for _, candidate := range d.sources.Accounts {
		if history.IsDebtCounterparty(account, candidate) {
			peers = append(peers, ui.Choice{ID: candidate.ID.String(), Label: candidate.Name + " · " + candidate.DefaultCurrency.String(), SearchText: candidate.Name})
		}
	}
	label, empty := i18n.LiabilityChooseDebt, i18n.LiabilityNoDebt
	if account.IsLiability() {
		label, empty = i18n.LiabilityChooseCash, i18n.LiabilityNoCash
	}
	if len(peers) == 0 {
		return view.Message(l.Text(empty), view.Colors.SecondaryInk)
	}
	picker := &d.destinationPicker
	return func(gtx layout.Context) layout.Dimensions {
		if d.focusAmount && d.destination == "" && gtx.Enabled() {
			gtx.Execute(key.FocusCmd{Tag: &picker.Trigger})
			d.focusAmount = false
		}
		field := ui.PickerField{Label: label, Origin: origin, Disabled: !gtx.Enabled() || account.ID == "", Next: &d.amount.Editor}
		if p.linked == nil {
			field.Previous = &d.accountPicker.Trigger
		}
		picker.Layout(gtx, view, field, ui.SelectOptions{Items: peers, Searchable: true}, d.destination.String(), func(value string) {
			if d.destination != domain.AccountID(value) {
				d.destination, d.problem = domain.AccountID(value), i18n.NoMessage
				d.amount.Editor.SetText("")
				d.fee.Editor.SetText("")
				gtx.Execute(op.InvalidateCmd{})
			}
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))}
	}
}
