// Command nestworth is the Wails v3 desktop application. It wires
// application.Service/settings.Store/sqlite.DB construction, then registers
// every internal/wailsapi service as a bound Wails service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	webassets "github.com/waltwang/nestworth-go"
	nestworthapp "github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/marketdata"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/version"
	wailsaccount "github.com/waltwang/nestworth-go/internal/wailsapi/account"
	wailsanalysis "github.com/waltwang/nestworth-go/internal/wailsapi/analysis"
	wailsanalytics "github.com/waltwang/nestworth-go/internal/wailsapi/analytics"
	wailsapp "github.com/waltwang/nestworth-go/internal/wailsapi/app"
	wailscatalog "github.com/waltwang/nestworth-go/internal/wailsapi/catalog"
	wailsdata "github.com/waltwang/nestworth-go/internal/wailsapi/data"
	wailsdirectory "github.com/waltwang/nestworth-go/internal/wailsapi/directory"
	wailshistory "github.com/waltwang/nestworth-go/internal/wailsapi/history"
	wailsholding "github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	wailshousehold "github.com/waltwang/nestworth-go/internal/wailsapi/household"
	wailsinstrument "github.com/waltwang/nestworth-go/internal/wailsapi/instrument"
	wailsmarketdata "github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
	"github.com/waltwang/nestworth-go/internal/wailsapi/native"
	wailsportfolio "github.com/waltwang/nestworth-go/internal/wailsapi/portfolio"
	wailsquote "github.com/waltwang/nestworth-go/internal/wailsapi/quote"
	wailsrecovery "github.com/waltwang/nestworth-go/internal/wailsapi/recovery"
	wailssettings "github.com/waltwang/nestworth-go/internal/wailsapi/settings"
)

func init() {
	application.RegisterEvent[wailsmarketdata.RefreshCompletedPayload](wailsmarketdata.RefreshCompletedEvent)
	application.RegisterEvent[wailsmarketdata.SyncJobDTO](wailsmarketdata.SyncStartedEvent)
	application.RegisterEvent[wailsmarketdata.SyncJobDTO](wailsmarketdata.SyncProgressEvent)
	application.RegisterEvent[wailsmarketdata.SyncJobDTO](wailsmarketdata.SyncItemEvent)
	application.RegisterEvent[wailsmarketdata.SyncJobDTO](wailsmarketdata.SyncCompletedEvent)
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

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	store := settings.DefaultStore()
	preference, loadErr := store.Load()
	if loadErr != nil {
		slog.Warn("could not load saved settings; using defaults")
	}

	databasePath := defaultDatabasePath()
	var database *sqlite.DB
	defer func() {
		if database != nil {
			_ = database.Close()
		}
	}()
	var service *nestworthapp.Service
	var startupErr error
	var foundSchema, supportedSchema int
	var hasSchema bool
	if err := backup.ReconcileOnStartup(databasePath, store); err != nil {
		slog.Error("restore journal could not be reconciled")
		startupErr = &domain.Error{Code: domain.ErrDatabaseUnavailable, Field: "database", Message: "the local database could not be opened"}
	} else {
		// Restore reconciliation may have merged selected settings from the
		// backup. Load them before constructing services and the main window.
		if refreshed, refreshErr := store.Load(); refreshErr == nil {
			preference = refreshed
		} else {
			slog.Warn("could not reload settings after restore reconciliation")
		}
		var databaseErr error
		database, databaseErr = sqlite.Open(databasePath)
		if databaseErr != nil {
			var bootstrap *sqlite.BootstrapError
			if errors.As(databaseErr, &bootstrap) {
				slog.Error("could not open the local database; the application will start with recovery available", "status", bootstrap.Status)
				startupErr = bootstrap.SafeError()
				foundSchema, supportedSchema = bootstrap.Found, bootstrap.Supported
				hasSchema = true
			} else {
				slog.Error("could not open the local database; the application will start with recovery available")
				startupErr = &domain.Error{Code: domain.ErrDatabaseUnavailable, Field: "database", Message: "the local database could not be opened"}
			}
		} else {
			registry := nestworthapp.NewMarketDataRegistryWithDefault(nestworthapp.FrankfurterProviderKey,
				marketdata.NewFrankfurterProvider(nil),
				marketdata.NewYahooChartProvider(nil),
				marketdata.NewTiingoProvider(func() (string, error) {
					current, err := store.Load()
					if err != nil {
						return "", err
					}
					return current.TiingoAPIKey, nil
				}, nil),
			)
			repo := sqlite.NewRepository(database)
			service = nestworthapp.NewService(repo, registry)
			appports.Wire(service)
			appports.AttachSQLiteHistory(service, repo)
			service.SetLiveDatabasePath(databasePath)
			if err := service.SetFXProvider(preference.FXProvider); err != nil {
				// Fall back for this session only: the persisted choice stays on
				// disk so a transient provider failure cannot rewrite the user's
				// configuration silently.
				slog.Warn("configured FX provider is not available; falling back to the default for this session")
				preference.FXProvider = settings.DefaultFXProvider
				if fallbackErr := service.SetFXProvider(preference.FXProvider); fallbackErr != nil {
					slog.Error("default FX provider rejected at startup")
				}
			}
			service.SetQuoteCacheTTL(preference.QuoteCacheTTLDuration())
			service.SetUILanguage(string(preference.Language))
			if _, err := service.Bootstrap(context.Background()); err != nil {
				slog.Error("bootstrap failed at startup")
				startupErr = err
				_ = database.Close()
				database = nil
				service = nil
			}
		}
	}

	emitter := &lazyEventEmitter{}
	platform := &lazyPlatform{}
	var marketdataService *wailsmarketdata.Service
	if service != nil {
		marketdataService = wailsmarketdata.NewService(service, emitter)
	}
	appService := wailsapp.NewService(startupErr)
	if hasSchema {
		appService = wailsapp.NewServiceWithSchema(startupErr, foundSchema, supportedSchema)
	}
	recoveryUseCase := nestworthapp.NewRecovery(databasePath, backup.NewRuntime(), service)
	recoveryService := wailsrecovery.NewService(recoveryUseCase, platform, platform, refreshGate(marketdataService))
	defer recoveryService.Shutdown()
	var dataService *wailsdata.Service
	if service != nil {
		dataService = wailsdata.NewService(service, store, platform, marketdataService)
		defer dataService.Shutdown()
	}
	app := application.New(application.Options{
		Name:        version.Name,
		Description: version.Description,
		Services:    services(service, store, recoveryService, dataService, marketdataService, appService),
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(webassets.Dist),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})
	emitter.manager = app.Event
	platform.setApp(app)

	app.Menu.SetApplicationMenu(application.DefaultApplicationMenu())

	width, height := windowSizeFromSettings(preference)
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
	window.OnWindowEvent(events.Common.WindowDidResize, func(_ *application.WindowEvent) {
		persistWindowSize(store, window)
	})
	window.OnWindowEvent(events.Common.WindowClosing, func(_ *application.WindowEvent) {
		persistWindowSize(store, window)
	})

	if err := app.Run(); err != nil {
		slog.Error("application exited with an error")
		return err
	}
	return nil
}

// services builds the bound-service list. service is nil only when the
// local database could not be opened at all (a narrower failure than the
// ordinary "onboarding not complete" case every application.Service method
// already reports through the normal wireError contract); in that case
// only AppService and CatalogService are registered so the frontend can
// render BlockedStartupPage from Startup() without calling unregistered
// services. Catalog is always available because it is a static vocabulary.
func services(service *nestworthapp.Service, store *settings.Store, recovery *wailsrecovery.Service, data *wailsdata.Service, marketdataService *wailsmarketdata.Service, appService *wailsapp.Service) []application.Service {
	registered := []application.Service{
		application.NewService(appService),
		application.NewService(wailscatalog.NewService()),
		application.NewService(recovery),
	}
	if service == nil || data == nil {
		return registered
	}
	if marketdataService == nil {
		marketdataService = wailsmarketdata.NewService(service, nil)
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
		application.NewService(wailsanalysis.NewService(service)),
		application.NewService(wailshistory.NewService(service)),
		application.NewService(marketdataService),
		application.NewService(wailssettings.NewService(store, service)),
		application.NewService(data),
	)
}

func refreshGate(service *wailsmarketdata.Service) native.RefreshGate {
	if service == nil {
		return native.NoopRefresh{}
	}
	return service
}

func persistWindowSize(store *settings.Store, window *application.WebviewWindow) {
	width, height := window.Size()
	if width <= 0 || height <= 0 {
		return
	}
	if err := persistWindowSizeValue(store, width, height); err != nil {
		slog.Warn("could not persist window size")
	}
}

func windowSizeFromSettings(preference settings.Settings) (int, int) {
	if preference.Validate() == nil {
		return int(preference.WindowWidth), int(preference.WindowHeight)
	}
	return settings.DefaultWindowWidth, settings.DefaultWindowHeight
}

func persistWindowSizeValue(store *settings.Store, width, height int) error {
	preference, loadStatus, err := store.LoadWithStatus()
	if err != nil {
		return fmt.Errorf("load settings before persisting window size: %w", err)
	}
	if loadStatus == settings.LoadStatusRecovered {
		return fmt.Errorf("saved settings require explicit recovery before persisting window size")
	}
	preference.WindowWidth = float32(width)
	preference.WindowHeight = float32(height)
	if err := preference.Validate(); err != nil {
		slog.Warn("window size out of persisted bounds; not saving")
		return err
	}
	return store.Save(preference)
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
