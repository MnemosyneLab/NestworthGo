package ui

import (
	"context"
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/format"
	"github.com/waltwang/nestworth-go/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// Page identifies one of the navigation destinations in the desktop UI.
type Page string

const (
	PageOverview     = Page("overview")
	PageAccounts     = Page("accounts")
	PageInvestments  = Page("investments")
	PageMembers      = Page("members")
	PageInstitutions = Page("institutions")
	PageGroups       = Page("groups")
	PageActivity     = Page("activity")
	PageAnalytics    = Page("analytics")
	PageSettings     = Page("settings")
)

// Controller owns presentation state and delegates all business mutations and
// financial calculations to the application service.
type Controller struct {
	application fyne.App
	window      fyne.Window
	icon        fyne.Resource
	store       *settings.Store

	service               *application.Service
	bootstrap             application.Bootstrap
	backendError          error
	backendPending        bool
	retryBackend          func()
	preference            settings.Settings
	translator            *i18n.Translator
	page                  Page
	accountCategory       string
	accountMemberID       string
	accountInstitutionID  string
	accountGroupID        string
	accountOwnershipScope string
	showArchived          bool
	saveError             error
	validationError       string
	regions               map[Page]*region
	refreshCancel         context.CancelFunc
	refreshGeneration     uint64
	refreshPending        bool
	refreshProgress       string
	refreshResult         *application.RefreshResult
	refreshError          error
	retryRefresh          func()
	refreshRebuild        bool
	refreshObserver       func(application.RefreshResult, error)
}

// NewController preserves the presentation-only constructor used by UI tests.
func NewController(fyneApplication fyne.App, window fyne.Window, icon fyne.Resource, store *settings.Store, preference settings.Settings, _ error) *Controller {
	return newController(fyneApplication, window, icon, store, preference, nil, application.Bootstrap{}, nil)
}

// NewControllerWithBackend creates the live v0.1.2 shell.
func NewControllerWithBackend(fyneApplication fyne.App, window fyne.Window, icon fyne.Resource, store *settings.Store, preference settings.Settings, service *application.Service, bootstrap application.Bootstrap, backendError error) *Controller {
	return newController(fyneApplication, window, icon, store, preference, service, bootstrap, backendError)
}

func newController(fyneApplication fyne.App, window fyne.Window, icon fyne.Resource, store *settings.Store, preference settings.Settings, service *application.Service, bootstrap application.Bootstrap, backendError error) *Controller {
	if preference.Validate() != nil {
		preference = settings.Default()
	}
	controller := &Controller{application: fyneApplication, window: window, icon: icon, store: store, service: service, bootstrap: bootstrap, backendError: backendError, preference: preference, translator: i18n.New(preference.Language), page: PageOverview}
	ApplyTheme(fyneApplication, preference)
	return controller
}

// Content returns the complete shell for the current page.
func (c *Controller) Content() fyne.CanvasObject {
	return container.NewBorder(nil, nil, c.sidebar(), nil, c.mainArea())
}

// Preferences returns the current display preferences for tests and views.
func (c *Controller) Preferences() settings.Settings {
	return c.preference
}

// Translator returns the active catalog-backed translator.
func (c *Controller) Translator() *i18n.Translator {
	return c.translator
}

// CurrentPage returns the selected navigation destination.
func (c *Controller) CurrentPage() Page {
	return c.page
}

func (c *Controller) navigate(page Page) {
	if c.backendError != nil || (c.service != nil && c.bootstrap.Household == nil) {
		return
	}
	if c.page == PageAccounts && page != PageAccounts {
		c.invalidateRegion(PageAccounts)
	}
	if c.page != page && c.refreshPending {
		c.cancelRefresh()
	}
	c.page = page
	c.validationError = ""
	c.Refresh()
}

func (c *Controller) updatePreference(update func(*settings.Settings)) {
	next := c.preference
	update(&next)
	if err := next.Validate(); err != nil {
		c.validationError = c.translator.TranslateError(err)
		c.Refresh()
		return
	}
	if c.service != nil && next.FXProvider != c.preference.FXProvider {
		if err := c.service.SetFXProvider(next.FXProvider); err != nil {
			c.validationError = c.translator.TranslateError(err)
			c.Refresh()
			return
		}
	}

	c.preference = next
	c.validationError = ""
	c.translator.SetLanguage(next.Language)
	ApplyTheme(c.application, next)
	c.saveError = nil
	if c.store != nil {
		c.saveError = c.store.Save(next)
	}
	c.Refresh()
}

func (c *Controller) resetPreference() {
	next := settings.Default()
	if c.service != nil {
		if err := c.service.SetFXProvider(next.FXProvider); err != nil {
			c.validationError = c.translator.TranslateError(err)
			c.Refresh()
			return
		}
	}
	c.preference = next
	c.translator.SetLanguage(c.preference.Language)
	ApplyTheme(c.application, c.preference)
	c.validationError = ""
	c.saveError = nil
	if c.store != nil {
		c.saveError = c.store.Save(c.preference)
	}
	c.Refresh()
}
func (c *Controller) setValidationError(message string) {
	c.validationError = message
	c.RefreshContent()
}

// PersistWindowSize saves the current window size so the next launch can
// restore it (v0.1.1 window-state restoration; see
// docs/development/code-review-2026-08-21.md BUG-5/GAP-2). It intentionally
// skips Refresh(): this runs from the window close handler, where rebuilding
// the UI is both unnecessary and unsafe.
func (c *Controller) PersistWindowSize(size fyne.Size) {
	if c.store == nil {
		return
	}
	next := c.preference
	next.WindowWidth = size.Width
	next.WindowHeight = size.Height
	if next.Validate() != nil {
		return
	}
	c.preference = next
	_ = c.store.Save(next)
}

func (c *Controller) Refresh() {
	if c.window == nil {
		return
	}
	c.window.SetMainMenu(NewMainMenu(c.application, c.window, c.icon, c.translator))
	c.window.SetContent(c.Content())
}

// RefreshContent rebuilds the current page while leaving the window menu
// untouched. In-page data changes use this path so opt-in regions can retain
// their widget identity and unsaved input.
func (c *Controller) RefreshContent() {
	if c.window == nil {
		return
	}
	c.window.SetContent(container.NewBorder(nil, nil, c.sidebar(), nil, c.mainArea()))
}

func (c *Controller) reloadBackend() {
	if c.service == nil {
		return
	}
	bootstrap, err := c.service.Bootstrap(context.Background())
	if err != nil {
		c.backendError = err
		return
	}
	c.bootstrap = bootstrap
	c.backendError = nil
}

func (c *Controller) sidebar() fyne.CanvasObject {
	t := c.translator
	iconView := canvas.NewImageFromResource(c.icon)
	iconView.FillMode = canvas.ImageFillContain
	iconView.SetMinSize(fyne.NewSize(38, 38))
	brand := container.NewHBox(iconView, widget.NewLabelWithStyle(t.T("app.name"), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}))
	primaryNavigation := container.NewVBox(
		c.navButton(PageOverview, t.T("nav.overview"), fyneTheme.IconNameHome),
		c.navButton(PageAccounts, t.T("nav.accounts"), fyneTheme.IconNameAccount),
		c.navButton(PageInvestments, t.T("nav.investments"), fyneTheme.IconNameStorage),
		c.navButton(PageMembers, t.T("nav.members"), fyneTheme.IconNameAccount),
		c.navButton(PageInstitutions, t.T("nav.institutions"), fyneTheme.IconNameDocument),
		c.navButton(PageGroups, t.T("nav.groups"), fyneTheme.IconNameFolder),
		c.navButton(PageActivity, t.T("nav.activity"), fyneTheme.IconNameHistory),
		c.navButton(PageAnalytics, t.T("nav.analytics"), fyneTheme.IconNameStorage),
	)
	navigation := container.NewBorder(nil, c.navButton(PageSettings, t.T("nav.settings"), fyneTheme.IconNameSettings), nil, nil, primaryNavigation)
	content := container.NewBorder(container.NewVBox(brand, widget.NewSeparator()), nil, nil, nil, navigation)
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameMenuBackground))
	background.SetMinSize(fyne.NewSize(224, 1))
	return container.NewStack(background, container.NewPadded(content))
}

func (c *Controller) navButton(page Page, label string, icon fyne.ThemeIconName) fyne.CanvasObject {
	button := widget.NewButtonWithIcon(label, fyneTheme.Current().Icon(icon), func() { c.navigate(page) })
	button.Alignment = widget.ButtonAlignLeading
	if c.page == page {
		button.Importance = widget.HighImportance
	} else {
		button.Importance = widget.LowImportance
	}
	return withMinSize(button, fyne.NewSize(190, 40))
}

func (c *Controller) mainArea() fyne.CanvasObject {
	header := c.pageHeader()
	var page fyne.CanvasObject
	if c.backendError != nil {
		page = NewBlockedStartupPage(c, c.backendError)
	} else if c.service != nil && c.bootstrap.Household == nil {
		page = NewOnboardingPage(c)
	} else {
		switch c.page {
		case PageSettings:
			page = NewSettingsPage(c)
		case PageAccounts:
			if c.service != nil {
				page = NewAccountsPage(c)
			} else {
				page = NewComingSoonPage(c, "page.accountsTitle", "page.accountsDescription", fyneTheme.IconNameAccount)
			}
		case PageInvestments:
			if c.service != nil {
				page = NewInvestmentsPage(c)
			} else {
				page = NewComingSoonPage(c, "page.investmentsTitle", "page.investmentsDescription", fyneTheme.IconNameStorage)
			}
		case PageMembers:
			if c.service != nil {
				page = NewMembersPage(c)
			} else {
				page = NewComingSoonPage(c, "page.membersTitle", "page.membersDescription", fyneTheme.IconNameAccount)
			}
		case PageInstitutions:
			if c.service != nil {
				page = NewInstitutionsPage(c)
			} else {
				page = NewComingSoonPage(c, "page.institutionsTitle", "page.institutionsDescription", fyneTheme.IconNameDocument)
			}
		case PageGroups:
			if c.service != nil {
				page = NewGroupsPage(c)
			} else {
				page = NewComingSoonPage(c, "page.groupsTitle", "page.groupsDescription", fyneTheme.IconNameFolder)
			}
		case PageActivity:
			page = NewComingSoonPage(c, "page.activityTitle", "page.activityDescription", fyneTheme.IconNameHistory)
		case PageAnalytics:
			page = NewComingSoonPage(c, "page.analyticsTitle", "page.analyticsDescription", fyneTheme.IconNameStorage)
		default:
			if c.service != nil {
				page = NewLiveOverview(c)
			} else {
				page = NewDashboard(c)
			}
		}
	}
	scroll := container.NewVScroll(container.NewPadded(page))
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameBackground))
	return container.NewBorder(header, nil, nil, nil, container.NewStack(background, scroll))
}
func (c *Controller) pageHeader() fyne.CanvasObject {
	t := c.translator
	title, subtitle := t.T("nav.overview"), t.T("dashboard.subtitle")
	switch c.page {
	case PageSettings:
		title, subtitle = t.T("settings.title"), t.T("settings.subtitle")
	case PageAccounts:
		title, subtitle = t.T("page.accountsTitle"), t.T("page.accountsDescription")
	case PageInvestments:
		title, subtitle = t.T("page.investmentsTitle"), t.T("page.investmentsDescription")
	case PageMembers:
		title, subtitle = t.T("page.membersTitle"), t.T("page.membersDescription")
	case PageInstitutions:
		title, subtitle = t.T("page.institutionsTitle"), t.T("page.institutionsDescription")
	case PageGroups:
		title, subtitle = t.T("page.groupsTitle"), t.T("page.groupsDescription")
	case PageActivity:
		title, subtitle = t.T("page.activityTitle"), t.T("page.activityDescription")
	case PageAnalytics:
		title, subtitle = t.T("page.analyticsTitle"), t.T("page.analyticsDescription")
	}
	titleLabel := widget.NewLabelWithStyle(title, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	subtitleLabel := widget.NewLabel(subtitle)
	subtitleLabel.Wrapping = fyne.TextWrapWord
	statusObjects := []fyne.CanvasObject{}
	if c.service == nil && c.backendError == nil {
		statusObjects = append(statusObjects, badge(t.T("common.previewData"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary))
	}
	statusObjects = append(statusObjects, widget.NewLabelWithStyle(formatLocalTime(c.preference, t.Language()), fyne.TextAlignTrailing, fyne.TextStyle{}))
	status := container.NewVBox(statusObjects...)
	background := canvas.NewRectangle(currentColor(fyneTheme.ColorNameBackground))
	background.SetMinSize(fyne.NewSize(1, 84))
	content := container.NewBorder(nil, nil, nil, status, container.NewVBox(titleLabel, subtitleLabel))
	return container.NewStack(background, container.NewPadded(content))
}

func formatLocalTime(preference settings.Settings, language settings.Language) string {
	value, err := format.DateTime(time.Now(), preference, language)
	if err != nil {
		return fmt.Sprintf("%s · %s", time.Now().Format("2006-01-02"), preference.Timezone)
	}
	return value
}

func NewComingSoonPage(c *Controller, titleKey, descriptionKey string, icon fyne.ThemeIconName) fyne.CanvasObject {
	t := c.translator
	mark := widget.NewButtonWithIcon("", fyneTheme.Current().Icon(icon), nil)
	mark.Importance = widget.HighImportance
	mark.Disable()
	markView := withMinSize(mark, fyne.NewSize(56, 56))
	title := widget.NewLabelWithStyle(t.T(titleKey), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	status := badge(t.T("common.comingSoon"), PaletteFor(c.preference.Accent).Soft, PaletteFor(c.preference.Accent).Primary)
	description := widget.NewLabel(t.T(descriptionKey))
	description.Alignment = fyne.TextAlignCenter
	description.Wrapping = fyne.TextWrapWord
	openSettings := widget.NewButtonWithIcon(t.T("common.openSettings"), fyneTheme.Current().Icon(fyneTheme.IconNameSettings), func() {
		c.navigate(PageSettings)
	})
	openSettings.Importance = widget.LowImportance

	content := container.NewVBox(
		container.NewCenter(markView),
		container.NewCenter(status),
		title,
		description,
		container.NewCenter(openSettings),
	)
	return surface(content, fyne.NewSize(520, 300))
}
