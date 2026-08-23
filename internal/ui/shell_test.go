package ui

import (
	"context"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestControllerNavigatesAndUpdatesPreferences(t *testing.T) {
	application := test.NewTempApp(t)
	window := test.NewTempWindow(t, container.NewVBox())
	store := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	controller := NewController(application, window, nil, store, settings.Default(), nil)
	window.SetContent(controller.Content())

	if controller.CurrentPage() != PageOverview {
		t.Fatalf("initial page = %q, want overview", controller.CurrentPage())
	}
	controller.navigate(PageSettings)
	if controller.CurrentPage() != PageSettings {
		t.Fatalf("page after navigate = %q, want settings", controller.CurrentPage())
	}
	if window.Content() == nil {
		t.Fatal("settings content is nil")
	}

	controller.updatePreference(func(next *settings.Settings) {
		next.Language = settings.LanguageZhTW
		next.Appearance = settings.AppearanceDark
		next.Accent = settings.AccentForest
	})
	if got := controller.Preferences(); got.Language != settings.LanguageZhTW || got.Appearance != settings.AppearanceDark || got.Accent != settings.AccentForest {
		t.Fatalf("preferences after update = %#v", got)
	}
	if got := controller.Translator().T("option.language.zhTW"); got != "正體中文" {
		t.Fatalf("active translator language label = %q", got)
	}
}

func TestControllerContentIsAFyneObjectForEachPage(t *testing.T) {
	application := test.NewTempApp(t)
	window := test.NewTempWindow(t, container.NewVBox())
	controller := NewController(application, window, nil, nil, settings.Default(), nil)
	for _, page := range []Page{PageOverview, PageAccounts, PageInvestments, PageActivity, PageAnalytics, PageSettings} {
		controller.navigate(page)
		if content := controller.Content(); content == nil {
			t.Fatalf("Content() for %q is nil", page)
		}
	}
	_ = fyne.NewSize(1, 1)
}

func TestSettingsRowsOnlyPlaceDividersBetweenOptions(t *testing.T) {
	application := test.NewTempApp(t)
	defer application.Quit()

	rows := settingsRows(
		settingsRow("First", widget.NewSelect([]string{"One"}, nil)),
		settingsRow("Second", widget.NewSelect([]string{"Two"}, nil)),
		settingsRow("Last", widget.NewSelect([]string{"Three"}, nil)),
	)
	containerRows, ok := rows.(*fyne.Container)
	if !ok {
		t.Fatalf("settingsRows returned %T, want *fyne.Container", rows)
	}
	if got, want := len(containerRows.Objects), 5; got != want {
		t.Fatalf("settingsRows object count = %d, want %d", got, want)
	}
	for index := 1; index < len(containerRows.Objects); index += 2 {
		if _, ok := containerRows.Objects[index].(*canvas.Rectangle); !ok {
			t.Fatalf("settingsRows object %d = %T, want separator", index, containerRows.Objects[index])
		}
	}
	if _, ok := containerRows.Objects[len(containerRows.Objects)-1].(*canvas.Rectangle); ok {
		t.Fatal("settingsRows must not end with a separator")
	}
}

// Regression test for docs/development/code-review-2026-08-21.md BUG-5/GAP-2:
// the window size must be persisted so the next launch can restore it.
func TestPersistWindowSizeSavesToStore(t *testing.T) {
	application := test.NewTempApp(t)
	window := test.NewTempWindow(t, container.NewVBox())
	store := settings.NewStore(filepath.Join(t.TempDir(), "settings.json"))
	controller := NewController(application, window, nil, store, settings.Default(), nil)

	controller.PersistWindowSize(fyne.NewSize(1440, 900))

	saved, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if saved.WindowWidth != 1440 || saved.WindowHeight != 900 {
		t.Fatalf("saved window size = %vx%v, want 1440x900", saved.WindowWidth, saved.WindowHeight)
	}
}

func TestLiveAccountsPageBuildsWithoutRecursiveRefresh(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	service := application.NewService(sqlite.NewRepository(database))
	if err := service.CompleteOnboarding(context.Background(), application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	bootstrap, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	controller := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), service, bootstrap, nil)
	if page := NewAccountsPage(controller); page == nil {
		t.Fatal("Accounts page is nil")
	}
}

func TestAccountsPageDoesNotRenderCreateForm(t *testing.T) {
	fyneApplication := test.NewTempApp(t)
	defer fyneApplication.Quit()
	window := test.NewTempWindow(t, container.NewVBox())
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "nestworth.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	service := application.NewService(sqlite.NewRepository(database))
	if err := service.CompleteOnboarding(context.Background(), application.OnboardingInput{HouseholdName: "Test", BaseCurrency: "CNY", MemberNames: []string{"Alice"}}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	bootstrap, err := service.Bootstrap(context.Background())
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	controller := NewControllerWithBackend(fyneApplication, window, nil, nil, settings.Default(), service, bootstrap, nil)
	controller.page = PageAccounts

	if containsForm(NewAccountsPage(controller)) {
		t.Fatal("account creation form should be opened from a modal")
	}
}

func containsForm(object fyne.CanvasObject) bool {
	if _, ok := object.(*widget.Form); ok {
		return true
	}
	if nested, ok := object.(*fyne.Container); ok {
		for _, child := range nested.Objects {
			if containsForm(child) {
				return true
			}
		}
	}
	return false
}
