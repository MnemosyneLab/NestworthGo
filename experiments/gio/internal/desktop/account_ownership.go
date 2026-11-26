package desktop

import (
	"fmt"
	"image"
	"slices"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
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
	row := func(gtx layout.Context, i int) layout.Dimensions {
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
				return ownerToggle(view, &choice.selected, name, selected)(gtx)
			}
			return layout.Flex{Alignment: layout.End, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Flexed(1, ownerToggle(view, &choice.selected, name, true)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return choice.percent.Layout(ui.LimitWidth(gtx, 88), view, ui.FieldOptions{
						Label: i18n.AccountMaintenanceShare, AccessibleName: fmt.Sprintf(view.Language.Text(i18n.AccountMaintenanceShareFor), member.Name),
					})
				}),
			)
		})
	}
	// A page that scrolls as a whole needs no inner scroller, which would also
	// reserve a scroll track beside the rows.
	if ui.HeightUnbounded(gtx) && len(members) <= 12 {
		children := make([]layout.FlexChild, len(members))
		for i := range members {
			children[i] = layout.Rigid(func(gtx layout.Context) layout.Dimensions { return row(gtx, i) })
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	}
	return view.List(&d.ownerList).Layout(gtx, len(members), row)
}

// ownerToggle is one owner as a full-width choice with a check box, so the
// state shows without relying on the tint alone.
func ownerToggle(view ui.View, button *widget.Clickable, name string, selected bool) layout.Widget {
	variant := ui.ButtonOutline
	if selected {
		variant = ui.ButtonSoft
	}
	return view.ButtonOf(button, ui.ButtonSpec{
		Variant: variant, Name: name, Toggle: true, Selected: selected, Fill: true,
		Content: func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return checkBox(gtx, view, selected) }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					return view.Line(gtx, ui.TextLabel, name, view.Colors.Ink)
				}),
			)
		},
	})
}

func checkBox(gtx layout.Context, view ui.View, checked bool) layout.Dimensions {
	side := gtx.Dp(16)
	bounds := image.Rectangle{Max: image.Pt(side, side)}
	colors := view.Colors
	if checked {
		ui.FillRoundRect(gtx, bounds, ui.RadiusSM-2, colors.Accent)
		defer op.Offset(image.Pt(gtx.Dp(2), gtx.Dp(2))).Push(gtx.Ops).Pop()
		ui.LayoutGlyph(gtx, lucide.Check, 12, colors.OnAccent)
	} else {
		ui.FillRoundRect(gtx, bounds, ui.RadiusSM-2, colors.Surface)
		ui.StrokeRoundRect(gtx, bounds, ui.RadiusSM-2, 1, ui.Mix(colors.Surface, colors.Ink, 0.35))
	}
	return layout.Dimensions{Size: bounds.Size()}
}
