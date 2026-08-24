package app

import (
	"context"
	_ "embed"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/marketdata"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/ui"
	"github.com/waltwang/nestworth-go/internal/version"
)

//go:embed resources/app-icon.png
var embeddedAppIcon []byte

// App owns the desktop application lifecycle and the local business database.
type App struct {
	fyneApp  fyne.App
	window   fyne.Window
	database *sqlite.DB
}

func New() *App {
	buildNumber, _ := strconv.Atoi(version.Build)
	app.SetMetadata(fyne.AppMetadata{ID: version.AppID, Name: version.Name, Version: strings.TrimPrefix(version.Version, "v"), Build: buildNumber, Migrations: map[string]bool{"fyneDo": true}})
	fyneApp := app.NewWithID(version.AppID)
	icon := fyne.NewStaticResource("nestworth-app-icon.png", embeddedAppIcon)
	fyneApp.SetIcon(icon)
	window := fyneApp.NewWindow(version.Name)
	store := settings.DefaultStore()
	preference, loadErr := store.Load()
	if loadErr != nil {
		slog.Warn("could not load saved settings; using defaults", "error", loadErr)
	}

	databasePath := defaultDatabasePath()
	database, databaseErr := sqlite.Open(databasePath)
	var service *application.Service
	var bootstrap application.Bootstrap
	backendErr := databaseErr
	if databaseErr == nil {
		registry := application.NewMarketDataRegistryWithDefault(application.FrankfurterProviderKey,
			marketdata.NewFrankfurterProvider(nil),
			marketdata.NewYahooChartProvider(nil),
		)
		service = application.NewService(sqlite.NewRepository(database), registry)
		if err := service.SetFXProvider(preference.FXProvider); err != nil {
			// Fall back for this session only: the persisted choice stays on
			// disk so a transient provider failure cannot rewrite the user's
			// configuration silently.
			slog.Warn("configured FX provider is not available; falling back to the default for this session",
				"provider", preference.FXProvider,
				"fallback", settings.DefaultFXProvider,
				"error", err)
			preference.FXProvider = settings.DefaultFXProvider
			if fallbackErr := service.SetFXProvider(preference.FXProvider); fallbackErr != nil {
				slog.Error("default FX provider rejected at startup", "error", fallbackErr)
			}
		}
		bootstrap, backendErr = service.Bootstrap(context.Background())
	}
	controller := ui.NewControllerWithBackend(fyneApp, window, icon, store, preference, service, bootstrap, backendErr)
	if preference.Validate() == nil {
		window.Resize(fyne.NewSize(preference.WindowWidth, preference.WindowHeight))
	}
	window.SetMainMenu(ui.NewMainMenu(fyneApp, window, icon, controller.Translator()))
	window.SetContent(controller.Content())
	window.SetCloseIntercept(func() {
		controller.PersistWindowSize(window.Canvas().Size())
		window.Close()
	})

	return &App{fyneApp: fyneApp, window: window, database: database}
}

func defaultDatabasePath() string {
	if explicitPath := strings.TrimSpace(os.Getenv("NESTWORTH_DATABASE_PATH")); explicitPath != "" {
		return explicitPath
	}
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		return filepath.Join(configDir, "Nestworth", "nestworth.db")
	}
	return filepath.Join(os.TempDir(), "Nestworth", "nestworth.db")
}

// Run starts the application event loop.
func (a *App) Run() {
	a.window.ShowAndRun()
	if a.database != nil {
		_ = a.database.Close()
	}
}
