package main

import (
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	pageBackground    = color.NRGBA{R: 248, G: 246, B: 243, A: 255}
	sidebarBackground = color.NRGBA{R: 242, G: 240, B: 236, A: 255}
	selectedSurface   = color.NRGBA{R: 254, G: 253, B: 252, A: 255}
	hoverSurface      = color.NRGBA{R: 234, G: 231, B: 226, A: 255}
	divider           = color.NRGBA{R: 223, G: 219, B: 212, A: 255}
	ink               = color.NRGBA{R: 32, G: 27, B: 23, A: 255}
	secondaryInk      = color.NRGBA{R: 95, G: 88, B: 82, A: 255}
	accent            = color.NRGBA{R: 52, G: 72, B: 166, A: 255}
)

type navigationItem struct {
	label  string
	icon   *widget.Icon
	body   layout.Widget
	button widget.Clickable
}

func newItem(label string, iconData []byte, body layout.Widget) navigationItem {
	icon, err := widget.NewIcon(iconData)
	if err != nil {
		panic(err)
	}
	return navigationItem{label: label, icon: icon, body: body}
}

type navigationGroup struct {
	label string
	items []navigationItem
}

type workspace struct {
	theme           *material.Theme
	wordmark        widget.Image
	mark            widget.Image
	groups          []navigationGroup
	selected        *navigationItem
	navigation      layout.List
	directoryEditor widget.Editor
}

func newWorkspace(wordmark, mark image.Image) *workspace {
	w := &workspace{
		theme:      material.NewTheme(),
		wordmark:   widget.Image{Src: paint.NewImageOp(wordmark), Fit: widget.Contain},
		mark:       widget.Image{Src: paint.NewImageOp(mark), Fit: widget.Contain},
		navigation: layout.List{Axis: layout.Vertical},
	}
	w.groups = []navigationGroup{
		{items: []navigationItem{
			newItem("概览", icons.ActionHome, w.overview),
			newItem("账户", icons.ActionAccountBalanceWallet, nil),
			newItem("可用资金", icons.EditorAttachMoney, nil),
			newItem("投资资产", icons.EditorPieChartOutlined, nil),
		}},
		{label: "活动", items: []navigationItem{
			newItem("待办核查", icons.ActionAssignmentTurnedIn, nil),
			newItem("账务记录", icons.ActionReceipt, nil),
		}},
		{label: "洞察", items: []navigationItem{
			newItem("资产回溯", icons.ActionHistory, nil),
			newItem("收益分析", icons.ActionTrendingUp, nil),
			newItem("资产变动", icons.ActionCompareArrows, nil),
		}},
		{label: "管理", items: []navigationItem{
			newItem("名录", icons.SocialPeopleOutline, w.directory),
			newItem("数据维护", icons.NotificationSync, nil),
			newItem("设置", icons.ActionSettings, nil),
		}},
	}
	w.selected = &w.groups[0].items[0]
	return w
}

func (w *workspace) layout(gtx layout.Context) layout.Dimensions {
	for _, group := range w.groups {
		for i := range group.items {
			item := &group.items[i]
			if item.button.Clicked(gtx) {
				w.selected = item
			}
		}
	}
	paint.Fill(gtx.Ops, pageBackground)
	return layout.Flex{}.Layout(gtx, layout.Rigid(w.sidebar), layout.Flexed(1, w.page))
}

func (w *workspace) sidebar(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Dp(218), gtx.Constraints.Max.Y)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, sidebarBackground, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, divider, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(w.brand),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 11, Right: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.navigation.Layout(gtx, len(w.groups), func(gtx layout.Context, i int) layout.Dimensions {
					return w.group(gtx, &w.groups[i])
				})
			})
		}),
	)
}

func (w *workspace) brand(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: 15, Bottom: 17, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				side := gtx.Dp(27)
				gtx.Constraints = layout.Exact(image.Pt(side, side))
				return w.mark.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(116), gtx.Dp(30)))
				return w.wordmark.Layout(gtx)
			}),
		)
	})
}

func (w *workspace) group(gtx layout.Context, group *navigationGroup) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(group.items)+1)
	if group.label != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 15, Bottom: 7, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.label(gtx, group.label, 11, font.Medium, secondaryInk)
			})
		}))
	}
	for i := range group.items {
		item := &group.items[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.item(gtx, item)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (w *workspace) item(gtx layout.Context, item *navigationItem) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(34))
	gtx.Constraints = layout.Exact(size)
	return item.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		selected := w.selected == item
		semantic.SelectedOp(selected).Add(gtx.Ops)
		bounds := image.Rectangle{Max: size}
		radius := gtx.Dp(9)
		textColor, iconColor := secondaryInk, secondaryInk
		weight := font.Normal
		focused := gtx.Focused(&item.button)
		switch {
		case selected:
			paint.FillShape(gtx.Ops, divider, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, selectedSurface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
			marker := image.Rect(1, gtx.Dp(7), gtx.Dp(3), size.Y-gtx.Dp(7))
			paint.FillShape(gtx.Ops, accent, clip.UniformRRect(marker, 1).Op(gtx.Ops))
			textColor, iconColor, weight = ink, accent, font.Medium
		case focused, item.button.Hovered():
			paint.FillShape(gtx.Ops, hoverSurface, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		}
		return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = 0
			return layout.Inset{Left: 11}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						side := gtx.Dp(17)
						gtx.Constraints = layout.Exact(image.Pt(side, side))
						return item.icon.Layout(gtx, iconColor)
					}),
					layout.Rigid(layout.Spacer{Width: 12}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return w.label(gtx, item.label, 13, weight, textColor)
					}),
				)
			})
		})
	})
}

func (w *workspace) page(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(w.pageHeader),
		layout.Flexed(1, w.pageBody),
	)
}

func (w *workspace) pageHeader(gtx layout.Context) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(53))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, divider, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return w.label(gtx, w.selected.label, 19, font.Medium, ink)
		})
	})
}

func (w *workspace) pageBody(gtx layout.Context) layout.Dimensions {
	if w.selected.body == nil {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Top: 26, Left: 28, Right: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(560))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return w.selected.body(gtx)
		})
	})
}

func (w *workspace) label(gtx layout.Context, text string, size unit.Sp, weight font.Weight, fg color.NRGBA) layout.Dimensions {
	label := material.Label(w.theme, size, text)
	label.Color = fg
	label.Font.Weight = weight
	label.MaxLines = 1
	return label.Layout(gtx)
}
