// Command nestworth-desktop is the Wails v3 entry point that will replace
// cmd/nestworth (Fyne) at cutover (docs/migration/wails-v3-implementation-plan.md
// Phase 6). It wires the same application.Service/settings.Store/sqlite.DB
// construction cmd/nestworth's internal/app.New() already performs, then
// registers every internal/wailsapi service as a bound Wails service.
package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	webassets "github.com/waltwang/nestworth-go"
	nestworthapp "github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/marketdata"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
	wailsaccount "github.com/waltwang/nestworth-go/internal/wailsapi/account"
	wailsanalytics "github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	wailsapp "github.com/waltwang/nestworth-go/internal/wailsapi/app"
	wailsdirectory "github.com/waltwang/nestworth-go/internal/wailsapi/directory"
	wailshistory "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	wailsholding "github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	wailshousehold "github.com/waltwang/nestworth-go/internal/wailsapi/household"
	wailsinstrument "github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	wailsmarketdata "github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	wailsmedia "github.com/waltwang/nestworth-go/internal/wailsapi/media"
	wailsportfolio "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	wailsquote "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	wailssettings "github.com/waltwang/nestworth-go/internal/wailsapi/settings"
)

func init() {
	application.RegisterEvent[wailsmarketdata.RefreshCompletedPayload](wailsmarketdata.RefreshCompletedEvent)
}

// lazyEventEmitter satisfies wailsapi/marketdata.EventEmitter without
// depending on a live *application.App at construction time: the
// marketdata service must be in application.Options.Services before
// application.New() returns the *application.App whose Event manager it
// needs, so the manager field is filled in immediately afterward, before
// app.Run() ever lets the frontend trigger a refresh. Keeping
// wailsapi/marketdata itself free of any github.com/wailsapp/wails/v3
// import is the point (technical design Sec3's dependency rule).
type lazyEventEmitter struct{ manager *application.EventManager }

func (e *lazyEventEmitter) Emit(name string, data any) {
	if e.manager != nil {
		e.manager.Emit(name, data)
	}
}

// lazyDialog is lazyEventEmitter's counterpart for wailsapi/media.Dialog.
type lazyDialog struct{ manager *application.DialogManager }

func (d *lazyDialog) OpenFile(title string) (string, error) {
	if d.manager == nil {
		return "", nil
	}
	return d.manager.OpenFile().
		SetTitle(title).
		AddFilter("Images", "*.png;*.jpg;*.jpeg;*.webp").
		PromptForSingleSelection()
}

func main() {
	store := settings.DefaultStore()
	preference, loadErr := store.Load()
	if loadErr != nil {
		slog.Warn("could not load saved settings; using defaults", "error", loadErr)
	}

	databasePath := defaultDatabasePath()
	database, databaseErr := sqlite.Open(databasePath)
	var service *nestworthapp.Service
	if databaseErr != nil {
		slog.Error("could not open the local database; the application will start with backend calls unavailable", "error", databaseErr, "path", databasePath)
	} else {
		defer database.Close()
		registry := nestworthapp.NewMarketDataRegistryWithDefault(nestworthapp.FrankfurterProviderKey,
			marketdata.NewFrankfurterProvider(nil),
			marketdata.NewYahooChartProvider(nil),
		)
		service = nestworthapp.NewService(sqlite.NewRepository(database), registry)
		if err := service.SetFXProvider(preference.FXProvider); err != nil {
			// Fall back for this session only: the persisted choice stays on
			// disk so a transient provider failure cannot rewrite the user's
			// configuration silently. Mirrors internal/app.New()'s Fyne
			// startup behavior exactly.
			slog.Warn("configured FX provider is not available; falling back to the default for this session",
				"provider", preference.FXProvider, "fallback", settings.DefaultFXProvider, "error", err)
			preference.FXProvider = settings.DefaultFXProvider
			if fallbackErr := service.SetFXProvider(preference.FXProvider); fallbackErr != nil {
				slog.Error("default FX provider rejected at startup", "error", fallbackErr)
			}
		}
		if _, err := service.Bootstrap(context.Background()); err != nil {
			slog.Error("bootstrap failed at startup", "error", err)
		}
	}

	emitter := &lazyEventEmitter{}
	dialog := &lazyDialog{}

	app := application.New(application.Options{
		Name:        version.Name,
		Description: version.Description,
		Services:    services(service, store, emitter, dialog),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(webassets.Dist),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	emitter.manager = app.Event
	dialog.manager = app.Dialog

	app.Menu.SetApplicationMenu(application.DefaultApplicationMenu())

	width, height := settings.DefaultWindowWidth, settings.DefaultWindowHeight
	if preference.Validate() == nil {
		width, height = int(preference.WindowWidth), int(preference.WindowHeight)
	}
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  version.Name,
		Width:  width,
		Height: height,
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropNormal,
			TitleBar: application.MacTitleBarDefault,
		},
		URL: "/",
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		persistWindowSize(store, preference, window)
	})

	if err := app.Run(); err != nil {
		slog.Error("application exited with an error", "error", err)
		os.Exit(1)
	}
}

// services builds the bound-service list. service is nil only when the
// local database could not be opened at all (a narrower failure than the
// ordinary "onboarding not complete" case every application.Service method
// already reports through the normal wireError contract); in that case
// only the version-only AppService is registered so the frontend can at
// least render a startup-failure screen instead of every call failing with
// an unregistered-service error.
func services(service *nestworthapp.Service, store *settings.Store, emitter wailsmarketdata.EventEmitter, dialog wailsmedia.Dialog) []application.Service {
	registered := []application.Service{
		application.NewService(wailsapp.NewService()),
	}
	if service == nil {
		return registered
	}
	return append(registered,
		application.NewService(wailshousehold.NewService(service)),
		application.NewService(wailsdirectory.NewService(service)),
		application.NewService(wailsaccount.NewService(service)),
		application.NewService(wailsportfolio.NewService(service)),
		application.NewService(wailsinstrument.NewService(service)),
		application.NewService(wailsholding.NewService(service)),
		application.NewService(wailsquote.NewService(service)),
		application.NewService(wailsanalytics.NewService(service)),
		application.NewService(wailshistory.NewService(service)),
		application.NewService(wailsmarketdata.NewService(service, emitter)),
		application.NewService(wailsmedia.NewService(service, dialog)),
		application.NewService(wailssettings.NewService(store, service)),
	)
}

func persistWindowSize(store *settings.Store, preference settings.Settings, window *application.WebviewWindow) {
	width, height := window.Size()
	if width <= 0 || height <= 0 {
		return
	}
	preference.WindowWidth = float32(width)
	preference.WindowHeight = float32(height)
	if err := preference.Validate(); err != nil {
		slog.Warn("window size out of persisted bounds; not saving", "error", err, "width", width, "height", height)
		return
	}
	if err := store.Save(preference); err != nil {
		slog.Warn("could not persist window size", "error", err)
	}
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
