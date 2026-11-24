package desktop

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type routeID string

const (
	overviewRoute         routeID = "overview"
	accountsRoute         routeID = "accounts"
	historyRoute          routeID = "history"
	directoryRoute        routeID = "directory"
	settingsRoute         routeID = "settings"
	componentPreviewRoute routeID = "component-preview"
)

const (
	sidebarWidth     = 224
	sidebarRailWidth = 64
	headerHeight     = 53
)

type pageLayout func(layout.Context, ui.View) layout.Dimensions

type ledgerPages struct {
	prefs     *preferenceState
	directory *directoryPage
	history   *historyPage
	overview  *overviewPage
	accounts  *accountsPage
}

type navigationItem struct {
	id     routeID
	label  i18n.MessageID
	icon   *widget.Icon
	body   pageLayout
	button widget.Clickable
	hint   ui.Tooltip
}

func newItem(id routeID, label i18n.MessageID, iconData []byte, body pageLayout) navigationItem {
	return navigationItem{id: id, label: label, icon: ui.MustIcon(iconData), body: body}
}

func (item *navigationItem) available() bool {
	return item.body != nil || item.id == historyRoute || item.id == directoryRoute || item.id == overviewRoute
}

type navigationGroup struct {
	label i18n.MessageID
	items []navigationItem
}

type shell struct {
	prefs                                 *preferenceState
	onboarding                            *onboardingPage
	settingsPage                          *settingsPage
	directory                             *directoryPage
	historyPage                           *historyPage
	overviewPage                          *overviewPage
	accounts                              *accountsPage
	componentPreview                      *componentPreview
	preferences                           sessionPreferences
	theme                                 *material.Theme
	wordmark                              widget.Image
	mark                                  widget.Image
	groups                                []navigationGroup
	routes                                map[routeID]*navigationItem
	trail                                 navigationTrail
	pendingNavigation                     *navigationRequest
	restoreFocus                          bool
	headingFocus                          byte
	back, forward                         widget.Clickable
	backHint, forwardHint                 ui.Tooltip
	recentPicker                          ui.Select
	navigation                            layout.List
	languagePicker                        ui.Select
	themeButton                           widget.Clickable
	railButton                            widget.Clickable
	languageHint, appearanceHint          ui.Tooltip
	wideCollapsed, narrowExpanded, narrow bool
	overlay                               ui.Overlay
}

func newShell(wordmark, mark image.Image, pages ledgerPages) *shell {
	preview := newComponentPreview()
	var directoryLayout, historyLayout, overviewLayout, accountsLayout pageLayout
	if pages.directory != nil {
		directoryLayout = pages.directory.layout
	}
	if pages.history != nil {
		historyLayout = pages.history.layout
	}
	if pages.overview != nil {
		overviewLayout = pages.overview.layout
	}
	if pages.accounts != nil {
		accountsLayout = pages.accounts.layout
	}
	s := &shell{
		prefs:            pages.prefs,
		settingsPage:     newSettingsPage(pages.prefs),
		directory:        pages.directory,
		historyPage:      pages.history,
		overviewPage:     pages.overview,
		accounts:         pages.accounts,
		componentPreview: preview,
		preferences:      sessionPreferences{language: i18n.SimplifiedChinese},
		theme:            material.NewTheme(),
		wordmark:         widget.Image{Src: paint.NewImageOp(wordmark), Fit: widget.Contain},
		mark:             widget.Image{Src: paint.NewImageOp(mark), Fit: widget.Contain},
		trail:            newNavigationTrail(overviewRoute),
		navigation:       layout.List{Axis: layout.Vertical},
	}
	s.groups = []navigationGroup{
		{items: []navigationItem{
			newItem(overviewRoute, i18n.NavOverview, icons.ActionHome, overviewLayout),
			newItem(accountsRoute, i18n.NavAccounts, icons.ActionAccountBalanceWallet, accountsLayout),
			newItem("available-funds", i18n.NavAvailableFunds, icons.EditorAttachMoney, nil),
			newItem("portfolio", i18n.NavPortfolio, icons.EditorPieChartOutlined, nil),
		}},
		{label: i18n.GroupActivity, items: []navigationItem{
			newItem("tasks", i18n.NavTasks, icons.ActionAssignmentTurnedIn, nil),
			newItem(historyRoute, i18n.NavHistory, icons.ActionReceipt, historyLayout),
		}},
		{label: i18n.GroupInsights, items: []navigationItem{
			newItem("historical-overview", i18n.NavHistoricalOverview, icons.ActionHistory, nil),
			newItem("returns", i18n.NavReturns, icons.ActionTrendingUp, nil),
			newItem("asset-changes", i18n.NavAssetChanges, icons.ActionCompareArrows, nil),
		}},
		{label: i18n.GroupManage, items: []navigationItem{
			newItem(directoryRoute, i18n.NavDirectory, icons.SocialPeopleOutline, directoryLayout),
			newItem("data-maintenance", i18n.NavDataMaintenance, icons.NotificationSync, nil),
			newItem(settingsRoute, i18n.NavSettings, icons.ActionSettings, s.settingsPage.layout),
			newItem(componentPreviewRoute, i18n.NavComponentPreview, icons.ActionBuild, preview.layout),
		}},
	}
	s.routes = make(map[routeID]*navigationItem)
	for _, group := range s.groups {
		for i := range group.items {
			item := &group.items[i]
			s.routes[item.id] = item
		}
	}
	if s.directory != nil {
		for _, list := range s.directory.lists {
			list.openObject = func(ref directoryRef, name string, from event.Tag) {
				s.pendingNavigation = &navigationRequest{
					target: navigationTarget{route: directoryRoute, object: ref},
					name:   name, focus: from,
				}
			}
		}
	}
	if s.accounts != nil {
		s.accounts.open = func(target navigationTarget, name string, from event.Tag) {
			keepArchiveFilter := s.activeRoute() == accountsRoute && target.account == "" && s.accounts.archived
			s.pendingNavigation = &navigationRequest{target: target, name: name, focus: from, accountsArchived: keepArchiveFilter}
		}
		s.accounts.backToList = func() {
			if previous := s.trail.adjacent(-1); previous != nil && previous.target.route == accountsRoute && !previous.target.hasObject() {
				s.pendingNavigation = &navigationRequest{step: -1}
				return
			}
			s.accounts.open(navigationTarget{route: accountsRoute}, "", nil)
		}
		if s.directory != nil {
			s.directory.object.openAccounts = func(id domain.InstitutionID, name string, from event.Tag) {
				s.accounts.open(navigationTarget{route: accountsRoute, institution: id}, name, from)
			}
		}
	}
	return s
}

func (s *shell) layout(gtx layout.Context) layout.Dimensions {
	s.prefs.update()
	s.preferences = s.prefs.appearance()
	if p := s.onboarding; p != nil {
		p.update()
		if p.opened {
			s.onboarding = nil
			if p.attempted && s.accounts != nil {
				s.trail = newNavigationTrail(accountsRoute)
				s.accounts.enter(s.trail.current().target, false, layout.Position{})
				s.prefs.reload()
			}
		}
	}
	if s.accounts != nil && s.activeRoute() == accountsRoute {
		if s.accounts.pollAccountMaintenance() && s.overviewPage != nil {
			s.overviewPage.needsRefresh = true
		}
		if record := s.accounts.pollAccountCreation(); record.Account.ID != "" {
			s.accounts.createdID = record.Account.ID
			s.accounts.createdMessage = i18n.AccountCreateSucceeded
			focus := &s.accounts.createBank
			if record.Account.IsLiability() {
				s.accounts.createdMessage = i18n.AccountCreateLiabilitySucceeded
				focus = &s.accounts.createLiability
			}
			if s.overviewPage != nil {
				s.overviewPage.needsRefresh = true
			}
			s.pendingNavigation = &navigationRequest{
				target: navigationTarget{route: accountsRoute, account: record.Account.ID},
				name:   record.Account.Name, focus: focus,
			}
		}
	}
	windowContext := gtx
	if s.settingsPage.confirming || s.confirmingArchive() || s.confirmingAccountCreation() || s.accountMaintenanceBlocking() {
		gtx = gtx.Disabled()
	}
	s.overlay.BeginFrame(gtx)
	s.narrow = gtx.Constraints.Max.X < gtx.Dp(960)
	if s.railButton.Clicked(gtx) {
		if s.narrow {
			s.narrowExpanded = !s.narrowExpanded
		} else {
			s.wideCollapsed = !s.wideCollapsed
		}
	}
	gtx.Locale.Language = string(s.preferences.language)
	colors := s.preferences.colors()
	s.theme.Palette = material.Palette{
		Bg:         colors.PageBackground,
		Fg:         colors.Ink,
		ContrastBg: colors.Accent,
		ContrastFg: colors.OnAccent,
	}
	view := ui.View{
		Language: s.preferences.language, Colors: colors, Theme: s.theme, Overlay: &s.overlay,
		DateFormat: s.prefs.saved.DateFormat, DecimalSeparator: s.prefs.saved.DecimalSeparator, GroupingSeparator: s.prefs.saved.GroupingSeparator,
		TimeFormat: s.prefs.saved.TimeFormat, WeekStart: s.prefs.saved.WeekStart,
		TimeLocation: s.prefs.displayLocation(),
	}
	paint.Fill(gtx.Ops, colors.PageBackground)
	if p := s.onboarding; p != nil {
		dims := p.layout(gtx, view, s.prefs, s.wordmark)
		s.overlay.Layout(gtx, view)
		return dims
	}
	s.updateNavigation(gtx)
	entry := s.trail.current()
	if entry.target.route == directoryRoute && entry.target.hasObject() && s.directory.object.update() {
		s.trail.rename(entry.target, s.directory.object.name())
	}
	if entry.target.route == accountsRoute && s.accounts != nil && s.accounts.update(s.preferences.language) {
		s.trail.rename(entry.target, s.accounts.name())
	}
	dims := layout.Flex{}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebar(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions { return s.page(gtx, view) }),
	)
	s.overlay.Layout(gtx, view)
	if s.settingsPage.confirming {
		s.settingsPage.layoutConfirmation(windowContext, view)
	} else if s.confirmingArchive() {
		s.directory.activeList().layoutConfirmation(windowContext, view)
	} else if s.confirmingAccountCreation() {
		s.accounts.layoutAccountCreationReview(windowContext, view)
	} else if s.accountMaintenanceBlocking() && s.accounts.maintenance.review != nil {
		s.accounts.layoutAccountArchive(windowContext, view)
	}
	return dims
}

func (s *shell) confirmingAccountCreation() bool {
	return s.activeRoute() == accountsRoute && s.accounts != nil && s.accounts.creation != nil && s.accounts.creation.review != nil
}

func (s *shell) accountMaintenanceBlocking() bool {
	if s.activeRoute() != accountsRoute || s.accounts == nil || s.accounts.maintenance == nil {
		return false
	}
	m := s.accounts.maintenance
	return m.pending != nil || m.review != nil
}

func (s *shell) confirmingArchive() bool {
	return s.activeRoute() == directoryRoute && !s.trail.current().target.hasObject() &&
		s.directory != nil && s.directory.activeList().archiveTarget != nil
}

func (s *shell) sidebar(gtx layout.Context, view ui.View) layout.Dimensions {
	colors := view.Colors
	size := image.Pt(gtx.Dp(s.sidebarWidth()), gtx.Constraints.Max.Y)
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.SidebarBackground, clip.Rect{Max: size}.Op())
	paint.FillShape(gtx.Ops, colors.Divider, clip.Rect(image.Rect(size.X-1, 0, size.X, size.Y)).Op())

	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebarBrand(gtx, view) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 12, Right: 12, Top: 4, Bottom: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return s.sidebarNavigation(gtx, view)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.sidebarPreferences(gtx, view, size.Y) }),
	)
}

func (s *shell) sidebarNavigation(gtx layout.Context, view ui.View) layout.Dimensions {
	height := gtx.Constraints.Max.Y
	return s.navigation.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		// Fill the viewport when there is room and scroll all the groups together
		// when their natural height exceeds it.
		gtx.Constraints.Min.Y = height
		last := len(s.groups) - 1
		gap := gtx.Dp(20)
		return layout.Flex{Axis: layout.Vertical, Spacing: layout.SpaceBetween, Gap: gap}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				groups := make([]layout.FlexChild, 0, last)
				for i := range s.groups[:last] {
					groups = append(groups, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return s.group(gtx, view, &s.groups[i])
					}))
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gap}.Layout(gtx, groups...)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(0, 0, gtx.Constraints.Max.X, 1)).Op())
				return layout.Inset{Top: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return s.group(gtx, view, &s.groups[last])
				})
			}),
		)
	})
}

func (s *shell) group(gtx layout.Context, view ui.View, group *navigationGroup) layout.Dimensions {
	children := make([]layout.FlexChild, 0, len(group.items)+1)
	if group.label != i18n.NoMessage && !s.collapsed() {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 6, Left: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, view.Language.Text(group.label), 11, font.Medium, view.Colors.SecondaryInk)
			})
		}))
	}
	for i := range group.items {
		item := &group.items[i]
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.item(gtx, view, item)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx, children...)
}

func (s *shell) item(gtx layout.Context, view ui.View, item *navigationItem) layout.Dimensions {
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(36))
	gtx.Constraints = layout.Exact(size)
	if !item.available() {
		gtx = gtx.Disabled()
	}
	return accessibility.Button(gtx, &item.button, view.Language.Text(item.label), "", false, func(gtx layout.Context) layout.Dimensions {
		return item.layoutButton(gtx, s, view, size)
	})
}

func (item *navigationItem) layoutButton(gtx layout.Context, s *shell, view ui.View, size image.Point) layout.Dimensions {
	colors := view.Colors
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		pointer.CursorPointer.Add(gtx.Ops)
		selected := s.activeRoute() == item.id
		semantic.SelectedOp(selected).Add(gtx.Ops)
		bounds := image.Rectangle{Max: size}
		radius := gtx.Dp(9)
		textColor, iconColor := colors.SecondaryInk, colors.SecondaryInk
		weight := font.Normal
		focused := gtx.Focused(&item.button)
		switch {
		case selected:
			paint.FillShape(gtx.Ops, colors.Divider, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
			paint.FillShape(gtx.Ops, colors.Surface, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
			marker := image.Rect(1, gtx.Dp(7), gtx.Dp(3), size.Y-gtx.Dp(7))
			paint.FillShape(gtx.Ops, colors.Accent, clip.UniformRRect(marker, 1).Op(gtx.Ops))
			textColor, iconColor, weight = colors.Ink, colors.Accent, font.Medium
		case focused, item.button.Hovered():
			paint.FillShape(gtx.Ops, colors.HoverSurface, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
		}
		item.hint.Layout(gtx, view, s.collapsed() && item.button.Hovered(), view.Language.Text(item.label))
		if s.collapsed() {
			semantic.LabelOp(view.Language.Text(item.label)).Add(gtx.Ops)
			return layout.Center.Layout(gtx, sidebarIcon(item.icon, iconColor, 18))
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
						return view.Label(gtx, view.Language.Text(item.label), 13, weight, textColor)
					}),
				)
			})
		})
	}
	if gtx.Enabled() {
		return item.button.Layout(gtx, content)
	}
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	semantic.EnabledOp(false).Add(gtx.Ops)
	return content(gtx)
}

func (s *shell) page(gtx layout.Context, view ui.View) layout.Dimensions {
	entry := s.trail.current()
	route := s.routes[entry.target.route]
	body := route.body
	if entry.target.route == directoryRoute && entry.target.hasObject() {
		body = s.directory.object.layout
	}
	view.Origin = image.Pt(gtx.Dp(s.sidebarWidth()), gtx.Dp(headerHeight))
	focus := ui.PageFocus{Target: entry.focus}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.pageHeader(gtx, view, route.label) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			view.Focus = &focus
			return pageBody(gtx, view, body)
		}),
	)
	s.finishPageFocus(gtx, focus)
	return dims
}

func (s *shell) pageHeader(gtx layout.Context, view ui.View, title i18n.MessageID) layout.Dimensions {
	colors := view.Colors
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(headerHeight))
	gtx.Constraints = layout.Exact(size)
	paint.FillShape(gtx.Ops, colors.Divider, clip.Rect(image.Rect(0, size.Y-1, size.X, size.Y)).Op())
	return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.Y = 0
		return layout.Inset{Left: 20, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gap := gtx.Dp(8)
			x := view.Origin.X + gtx.Dp(20)
			direction := func(step int) layout.Widget {
				return func(gtx layout.Context) layout.Dimensions {
					dims := s.navigationButton(gtx, view, step)
					x += dims.Size.X + gap
					return dims
				}
			}
			return layout.Flex{Alignment: layout.Middle, Gap: gap}.Layout(gtx,
				layout.Rigid(direction(-1)),
				layout.Rigid(direction(1)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions { return s.recentButton(gtx, view, x) }),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					for {
						if _, ok := gtx.Event(key.FocusFilter{Target: &s.headingFocus}); !ok {
							break
						}
					}
					defer clip.Rect{Max: gtx.Constraints.Max}.Push(gtx.Ops).Pop()
					event.Op(gtx.Ops, &s.headingFocus)
					return view.Label(gtx, view.Language.Text(title), 19, font.Medium, colors.Ink)
				}),
			)
		})
	})
}

func pageBody(gtx layout.Context, view ui.View, body pageLayout) layout.Dimensions {
	if body == nil {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	}
	return layout.NW.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		inset := layout.Inset{Top: 26, Left: 28, Right: 28, Bottom: 16}
		view.Origin = view.Origin.Add(image.Pt(gtx.Dp(inset.Left), gtx.Dp(inset.Top)))
		return inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return body(gtx, view)
		})
	})
}
