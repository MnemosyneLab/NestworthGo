package desktop

import (
	"fmt"
	"slices"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type accountOwnerChoice struct {
	selected widget.Clickable
	percent  ui.TextField
}

type accountOwnershipDraft struct {
	custom        bool
	owners        []domain.MemberID
	ownerControls map[domain.MemberID]*accountOwnerChoice
	ownerList     widget.List
	shares        widget.Clickable
}

func newAccountOwnershipDraft() accountOwnershipDraft {
	return accountOwnershipDraft{
		ownerControls: make(map[domain.MemberID]*accountOwnerChoice),
		ownerList:     widget.List{List: layout.List{Axis: layout.Vertical}},
	}
}

func (d *accountOwnershipDraft) owner(id domain.MemberID) *accountOwnerChoice {
	option := d.ownerControls[id]
	if option == nil {
		option = &accountOwnerChoice{percent: ui.NewTextField(true)}
		d.ownerControls[id] = option
	}
	return option
}

func (d *accountOwnershipDraft) ownership() (domain.Ownership, i18n.MessageID) {
	if len(d.owners) == 0 {
		return domain.Ownership{}, i18n.AccountCreateOwnersRequired
	}
	if !d.custom {
		ownership, err := domain.EqualOwnership(d.owners)
		if err != nil {
			return domain.Ownership{}, i18n.AccountMaintenanceSharesInvalid
		}
		return ownership, i18n.NoMessage
	}
	shares := make([]domain.OwnershipShare, 0, len(d.owners))
	for _, id := range d.owners {
		bps, err := domain.PercentToBasisPoints(d.owner(id).percent.Text())
		if err != nil {
			return domain.Ownership{}, i18n.AccountMaintenanceSharesInvalid
		}
		shares = append(shares, domain.OwnershipShare{MemberID: id, ShareBPS: bps})
	}
	ownership, err := domain.ParseOwnership(shares)
	if err != nil {
		return domain.Ownership{}, i18n.AccountMaintenanceSharesInvalid
	}
	return ownership, i18n.NoMessage
}

func (d *accountOwnershipDraft) layoutShareMode(gtx layout.Context, view ui.View) layout.Dimensions {
	if d.shares.Clicked(gtx) {
		d.custom = !d.custom
		if equal, err := domain.EqualOwnership(d.owners); d.custom && err == nil {
			for _, share := range equal.Shares() {
				d.owner(share.MemberID).percent.Editor.SetText(fmt.Sprintf("%d.%02d", share.ShareBPS/100, share.ShareBPS%100))
			}
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	label := i18n.AccountMaintenanceEqual
	if d.custom {
		label = i18n.AccountMaintenanceCustom
	}
	return view.TabButton(&d.shares, view.Language.Text(label), d.custom)(gtx)
}

func (d *accountOwnershipDraft) layoutOwnerList(gtx layout.Context, view ui.View, members []domain.Member, editShares bool) layout.Dimensions {
	return view.List(&d.ownerList).Layout(gtx, len(members), func(gtx layout.Context, i int) layout.Dimensions {
		member := members[i]
		choice := d.owner(member.ID)
		index := slices.Index(d.owners, member.ID)
		if choice.selected.Clicked(gtx) {
			if index >= 0 {
				d.owners = slices.Delete(d.owners, index, index+1)
			} else {
				d.owners = append(d.owners, member.ID)
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		name := accountReference(view.Language, member.Name, member.ArchivedAt != nil)
		selected := slices.Contains(d.owners, member.ID)
		return layout.Inset{Bottom: 4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			if !editShares || !d.custom || !selected {
				return view.TabButton(&choice.selected, name, selected)(gtx)
			}
			return layout.Flex{Alignment: layout.End, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Flexed(1, view.TabButton(&choice.selected, name, true)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return choice.percent.Layout(ui.LimitWidth(gtx, 88), view, ui.FieldOptions{
						Label: i18n.AccountMaintenanceShare, AccessibleName: fmt.Sprintf(view.Language.Text(i18n.AccountMaintenanceShareFor), member.Name),
					})
				}),
			)
		})
	})
}
