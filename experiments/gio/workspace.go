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

type navigationItem struct {
	label  messageID
	icon   *widget.Icon
	body   layout.Widget
	button widget.Clickable
}

func newItem(label messageID, iconData []byte, body layout.Widget) navigationItem {
	icon, err := widget.NewIcon(iconData)
	if err != nil {
		panic(err)
	}
	return navigationItem{label: label, icon: icon, body: body}
}

type navigationGroup struct {
	label messageID
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
	language        appLanguage
	dark            bool
	languageButton  widget.Clickable
	themeButton     widget.Clickable
}

func newWorkspace(wordmark, mark image.Image) *workspace {
	w := &workspace{
		theme:      material.NewTheme(),
		wordmark:   widget.Image{Src: paint.NewImageOp(wordmark), Fit: widget.Contain},
		mark:       widget.Image{Src: paint.NewImageOp(mark), Fit: widget.Contain},
		navigation: layout.List{Axis: layout.Vertical},
		language:   simplifiedChinese,
	}
	w.groups = []navigationGroup{
		{items: []navigationItem{
			newItem(navOverview, icons.ActionHome, w.overview),
			newItem(navAccounts, icons.ActionAccountBalanceWallet, nil),
			newItem(navAvailableFunds, icons.EditorAttachMoney, nil),
			newItem(navPortfolio, icons.EditorPieChartOutlined, nil),
		}},
		{label: groupActivity, items: []navigationItem{
			newItem(navTasks, icons.ActionAssignmentTurnedIn, nil),
			newItem(navHistory, icons.ActionReceipt, nil),
		}},
		{label: groupInsights, items: []navigationItem{
			newItem(navHistoricalOverview, icons.ActionHistory, nil),
			newItem(navReturns, icons.ActionTrendingUp, nil),
			newItem(navAssetChanges, icons.ActionCompareArrows, nil),
		}},
		{label: groupManage, items: []navigationItem{
			newItem(navDirectory, icons.SocialPeopleOutline, w.directory),
			newItem(navDataMaintenance, icons.NotificationSync, nil),
			newItem(navSettings, icons.ActionSettings, nil),
		}},
	}
	w.selected = &w.groups[0].items[0]
	return w
}

func (w *workspace) layout(gtx layout.Context) layout.Dimensions {
	if w.languageButton.Clicked(gtx) {
		w.language = w.language.alternate()
	}
	if w.themeButton.Clicked(gtx) {
		w.dark = !w.dark
	}
	gtx.Locale.Language = string(w.language)
	colors := w.colors()
	w.theme.Palette = material.Palette{
		Bg:         colors.pageBackground,
		Fg:         colors.ink,
		ContrastBg: colors.accent,
		ContrastFg: colors.pageBackground,
	}
	for _, group := range w.groups {
		for i := range group.items {
			item := &group.items[i]
			if item.button.Clicked(gtx) {
				w.selected = item
			}
		}
	}
	paint.Fill(gtx.Ops, colors.pageBackground)
	return layout.Flex{}.Layout(gtx, layout.Rigid(w.sidebar), layout.Flexed(1, w.page))
}

func (w *workspace) sidebar(gtx layout.Context) layout.Dimensions {
	colors := w.colors()
	size := image.Pt(gtx.Dp(218), gtx.Constraints.Max.Y)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.sidebarBackground, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, colors.divider, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(w.brand),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 11, Right: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.navigation.Layout(gtx, len(w.groups), func(gtx layout.Context, i int) layout.Dimensions {
					return w.group(gtx, &w.groups[i])
				})
			})
		}),
		layout.Rigid(w.sidebarPreferences),
	)
}

func (w *workspace) sidebarPreferences(gtx layout.Context) layout.Dimensions {
	colors := w.colors()
	paint.FillShape(gtx.Ops, colors.divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X-1, 1)).Op())
	themeAction := darkAppearance
	if w.dark {
		themeAction = lightAppearance
	}
	languageAction := w.language.alternate().text(languageName)
	return layout.Inset{Top: 9, Bottom: 10, Left: 11, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(0.58, w.preferenceButton(&w.languageButton, languageAction)),
			layout.Rigid(layout.Spacer{Width: 7}.Layout),
			layout.Flexed(0.42, w.preferenceButton(&w.themeButton, w.language.text(themeAction))),
		)
	})
}

func (w *workspace) preferenceButton(button *widget.Clickable, title string) layout.Widget {
	style := material.Button(w.theme, button, title)
	style.Background = w.colors().surface
	style.Color = w.colors().ink
	style.TextSize = 12
	style.CornerRadius = 7
	style.Inset = layout.Inset{Top: 7, Bottom: 7, Left: 8, Right: 8}
	return style.Layout
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
	if group.label != noMessage {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 15, Bottom: 7, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.label(gtx, w.language.text(group.label), 11, font.Medium, w.colors().secondaryInk)
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
	colors := w.colors()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(34))
	gtx.Constraints = layout.Exact(size)
	return item.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		selected := w.selected == item
		semantic.SelectedOp(selected).Add(gtx.Ops)
		bounds := image.Rectangle{Max: size}
		radius := gtx.Dp(9)
		textColor, iconColor := colors.secondaryInk, colors.secondaryInk
		weight := font.Normal
		focused := gtx.Focused(&item.button)
		switch {
		case selected:
			paint.FillShape(gtx.Ops, colors.divider, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.surface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
			marker := image.Rect(1, gtx.Dp(7), gtx.Dp(3), size.Y-gtx.Dp(7))
			paint.FillShape(gtx.Ops, colors.accent, clip.UniformRRect(marker, 1).Op(gtx.Ops))
			textColor, iconColor, weight = colors.ink, colors.accent, font.Medium
		case focused, item.button.Hovered():
			paint.FillShape(gtx.Ops, colors.hoverSurface, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
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
						return w.label(gtx, w.language.text(item.label), 13, weight, textColor)
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
	colors := w.colors()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(53))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.divider, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return w.label(gtx, w.language.text(w.selected.label), 19, font.Medium, colors.ink)
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
