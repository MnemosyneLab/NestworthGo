package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"gioui.org/app"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/desktop"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/appports"
	"github.com/waltwang/nestworth-go/internal/infrastructure/backup"
	"github.com/waltwang/nestworth-go/internal/infrastructure/marketdata"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func main() {
	go func() {
		if started, err := run(time.Now); err != nil {
			fmt.Fprintln(os.Stderr, pilot.DisplayName+":", err)
			if !started {
				if err := desktop.ShowStartupBlocked(startupMessage(err)); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func startupMessage(err error) i18n.MessageID {
	switch {
	case errors.Is(err, backup.ErrAlreadyRunning):
		return i18n.StartupInUse
	case errors.Is(err, pilot.ErrStartupRecovery):
		return i18n.StartupRecoveryPending
	}
	var bootstrap *sqlite.BootstrapError
	if errors.As(err, &bootstrap) {
		switch bootstrap.SafeError().Code {
		case domain.ErrDatabaseUpgradeRequired:
			return i18n.StartupUpgradeRequired
		case domain.ErrDatabaseFromNewerVersion:
			return i18n.StartupNewerDatabase
		case domain.ErrDatabaseIntegrityFailed:
			return i18n.StartupIntegrityFailed
		}
	}
	return i18n.StartupUnavailable
}

func run(clock func() time.Time) (started bool, result error) {
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		return false, err
	}
	wordmark, err := loadBrandImage("wordmark.png")
	if err != nil {
		return false, err
	}
	mark, err := loadBrandImage("mark.png")
	if err != nil {
		return false, err
	}
	ctx := context.Background()
	session, err := pilot.OpenSession(ctx, paths)
	if err != nil {
		return false, err
	}
	defer func() { result = errors.Join(result, session.Close()) }()
	repo := sqlite.NewRepository(session.DB).WithDefaultReviewOptimizations()
	service := application.NewService(repo)
	appports.Wire(service)
	appports.AttachSQLiteHistory(service, repo)
	service.SetLiveDatabasePath(session.DB.Path)
	service.SetClock(clock)
	prefs, err := preferences.Open(paths.SettingsPath(), session.DB, service)
	if err != nil {
		return false, err
	}
	previousLogger := slog.Default()
	profileLog, logErr := prefs.OpenLog()
	if logErr != nil {
		slog.Warn("could not configure Gio application file logging")
	}
	if profileLog != nil {
		defer func() {
			slog.SetDefault(previousLogger)
			result = errors.Join(result, profileLog.Close())
		}()
		slog.SetDefault(profileLog.Logger())
	}
	service.SetMarketDataRegistry(application.NewMarketDataRegistryWithDefault(application.FrankfurterProviderKey,
		marketdata.NewFrankfurterProvider(nil), marketdata.NewYahooChartProvider(), marketdata.NewCoinGeckoProvider(prefs.CoinGeckoKey, nil),
	))
	if err := prefs.ConfigureValuation(); err != nil {
		return false, err
	}
	if _, err := service.Bootstrap(ctx); err != nil {
		return false, err
	}
	slog.Info("Gio application starting")
	return true, desktop.Run(ctx, service, prefs, session.CashReceipts, clock, wordmark, mark)
}

func loadBrandImage(name string) (image.Image, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(filepath.Dir(executable), "..", "Resources", name)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open bundled image %q: %w", name, err)
	}
	defer file.Close()
	return png.Decode(file)
}
