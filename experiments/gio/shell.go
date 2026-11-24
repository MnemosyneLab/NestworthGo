package main

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type routeID string

const (
	overviewRoute         routeID = "overview"
	directoryRoute        routeID = "directory"
	componentPreviewRoute routeID = "component-preview"
)

const (
	sidebarWidth = 218
	headerHeight = 53
)

type pageLayout func(layout.Context, presentation) layout.Dimensions

type navigationItem struct {
	id     routeID
	label  messageID
	icon   *widget.Icon
	body   pageLayout
	button widget.Clickable
}

func newItem(id routeID, label messageID, iconData []byte, body pageLayout) navigationItem {
	return navigationItem{id: id, label: label, icon: mustIcon(iconData), body: body}
}

func mustIcon(iconData []byte) *widget.Icon {
	icon, err := widget.NewIcon(iconData)
	if err != nil {
		panic(err)
	}
	return icon
}

type navigationGroup struct {
	label messageID
	items []navigationItem
}

type shell struct {
	preferences    sessionPreferences
	theme          *material.Theme
	wordmark       widget.Image
	mark           widget.Image
	groups         []navigationGroup
	routes         map[routeID]*navigationItem
	activeRoute    routeID
	navigation     layout.List
	languageButton widget.Clickable
	themeButton    widget.Clickable
	overlay        popoverHost
}

func newShell(wordmark, mark image.Image) *shell {
	directory := &directoryPage{editor: newTextField(false)}
	preview := newComponentPreview()
	s := &shell{
		preferences: sessionPreferences{language: simplifiedChinese},
		theme:       material.NewTheme(),
		wordmark:    widget.Image{Src: paint.NewImageOp(wordmark), Fit: widget.Contain},
		mark:        widget.Image{Src: paint.NewImageOp(mark), Fit: widget.Contain},
		activeRoute: overviewRoute,
		navigation:  layout.List{Axis: layout.Vertical},
	}
	s.groups = []navigationGroup{
		{items: []navigationItem{
			newItem(overviewRoute, navOverview, icons.ActionHome, overview),
			newItem("accounts", navAccounts, icons.ActionAccountBalanceWallet, nil),
			newItem("available-funds", navAvailableFunds, icons.EditorAttachMoney, nil),
			newItem("portfolio", navPortfolio, icons.EditorPieChartOutlined, nil),
		}},
		{label: groupActivity, items: []navigationItem{
			newItem("tasks", navTasks, icons.ActionAssignmentTurnedIn, nil),
			newItem("history", navHistory, icons.ActionReceipt, nil),
		}},
		{label: groupInsights, items: []navigationItem{
			newItem("historical-overview", navHistoricalOverview, icons.ActionHistory, nil),
			newItem("returns", navReturns, icons.ActionTrendingUp, nil),
			newItem("asset-changes", navAssetChanges, icons.ActionCompareArrows, nil),
		}},
		{label: groupManage, items: []navigationItem{
			newItem(directoryRoute, navDirectory, icons.SocialPeopleOutline, directory.layout),
			newItem("data-maintenance", navDataMaintenance, icons.NotificationSync, nil),
			newItem("settings", navSettings, icons.ActionSettings, nil),
		}},
		{label: groupDevelopment, items: []navigationItem{
			newItem(componentPreviewRoute, navComponentPreview, icons.ActionBuild, preview.layout),
		}},
	}
	s.routes = make(map[routeID]*navigationItem)
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			s.routes[item.id] = item
		}
	}
	return s
}

func (s *shell) layout(gtx layout.Context) layout.Dimensions {
	s.overlay.beginFrame(gtx)
	if s.languageButton.Clicked(gtx) {
		s.preferences.language = s.preferences.language.alternate()
	}
	if s.themeButton.Clicked(gtx) {
		s.preferences.dark = !s.preferences.dark
	}
	gtx.Locale.Language = string(s.preferences.language)
	colors := s.preferences.colors()
	s.theme.Palette = material.Palette{
		Bg:         colors.pageBackground,
		Fg:         colors.ink,
		ContrastBg: colors.accent,
		ContrastFg: colors.pageBackground,
	}
	view := presentation{language: s.preferences.language, colors: colors, theme: s.theme, overlay: &s.overlay}
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			if item.button.Clicked(gtx) {
				s.activeRoute = item.id
			}
		}
	}
	paint.Fill(gtx.Ops, colors.pageBackground)
	dims := layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebar(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return s.page(gtx, view) }),
	)
	s.overlay.layout(gtx, view)
	return dims
}

func (s *shell) sidebar(gtx layout.Context, view presentation) layout.Dimensions {
	colors := view.colors
	size := image.Pt(gtx.Dp(sidebarWidth), gtx.Constraints.Max.Y)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.sidebarBackground, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, colors.divider, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(s.brand),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 11, Right: 12, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.navigation.Layout(gtx, len(s.groups), func(gtx layout.Context, i int) layout.Dimensions {
					return s.group(gtx, view, &s.groups[i])
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebarPreferences(gtx, view) }),
	)
}

func (s *shell) sidebarPreferences(gtx layout.Context, view presentation) layout.Dimensions {
	paint.FillShape(gtx.Ops, view.colors.divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X-1, 1)).Op())
	themeAction := darkAppearance
	if s.preferences.dark {
		themeAction = lightAppearance
	}
	languageAction := view.language.alternate().text(languageName)
	return layout.Inset{Top: 9, Bottom: 10, Left: 11, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{}.Layout(gtx,
			layout.Flexed(0.58, view.button(&s.languageButton, languageAction)),
			layout.Rigid(layout.Spacer{Width: 7}.Layout),
			layout.Flexed(0.42, view.button(&s.themeButton, view.language.text(themeAction))),
		)
	})
}

func (s *shell) brand(gtx layout.Context) layout.Dimensions {
	return layout.Inset{Top: 15, Bottom: 17, Left: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				side := gtx.Dp(27)
				gtx.Constraints = layout.Exact(image.Pt(side, side))
				return s.mark.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: 8}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints = layout.Exact(image.Pt(gtx.Dp(116), gtx.Dp(30)))
				return s.wordmark.Layout(gtx)
			}),
		)
	})
}

func (s *shell) group(gtx layout.Context, view presentation, group *navigationGroup) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(group.items)+1)
	if group.label != noMessage {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 15, Bottom: 7, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.label(gtx, view.language.text(group.label), 11, font.Medium, view.colors.secondaryInk)
			})
		}))
	}
	for i := range group.items {
		item := &group.items[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 3}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.item(gtx, view, item)
			})
		}))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (s *shell) item(gtx layout.Context, view presentation, item *navigationItem) layout.Dimensions {
	colors := view.colors
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(34))
	gtx.Constraints = layout.Exact(size)
	return item.button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		selected := s.activeRoute == item.id
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
						return view.label(gtx, view.language.text(item.label), 13, weight, textColor)
					}),
				)
			})
		})
	})
}

func (s *shell) page(gtx layout.Context, view presentation) layout.Dimensions {
	route := s.routes[s.activeRoute]
	view.origin = image.Pt(gtx.Dp(sidebarWidth), gtx.Dp(headerHeight))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return pageHeader(gtx, view, route.label) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return pageBody(gtx, view, route.body) }),
	)
}

func pageHeader(gtx layout.Context, view presentation, title messageID) layout.Dimensions {
	colors := view.colors
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(headerHeight))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.divider, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: 28}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.label(gtx, view.language.text(title), 19, font.Medium, colors.ink)
		})
	})
}

func pageBody(gtx layout.Context, view presentation, body pageLayout) layout.Dimensions {
	if body == nil {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		inset := layout.Inset{Top: 26, Left: 28, Right: 28, Bottom: 16}
		view.origin = view.origin.Add(image.Pt(gtx.Dp(inset.Left), gtx.Dp(inset.Top)))
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return body(gtx, view)
		})
	})
}
