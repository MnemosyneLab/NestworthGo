package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"gioui.org/app"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/desktop"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/pilot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/preferences"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func main() {
	if len(os.Args) > 1 {
		if err := pilotCommand(os.Args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	go func() {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, pilot.DisplayName+" could not start:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func pilotCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("expected one pilot command")
	}
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
	switch args[0] {
	case "--print-pilot-root":
		fmt.Println(paths.RootPath())
	case "--prepare-pilot-fixture":
		return pilot.Prepare(ctx, paths)
	case "--ensure-pilot-fixture":
		return pilot.Ensure(ctx, paths)
	case "--verify-pilot-fixture":
		if err := pilot.Verify(ctx, paths); err != nil {
			return err
		}
		fmt.Println("Verified the isolated Gio Institutions QA fixture.")
	default:
		return fmt.Errorf("unknown pilot command %q", args[0])
	}
	return nil
}

func run() (result error) {
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		return err
	}
	wordmark, err := loadBrandImage("wordmark.png")
	if err != nil {
		return err
	}
	mark, err := loadBrandImage("mark.png")
	if err != nil {
		return err
	}
	ctx := context.Background()
	db, unlock, err := pilot.OpenSession(ctx, paths)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, db.CheckpointWAL(context.Background()), db.Close(), unlock())
	}()
	service := application.NewService(sqlite.NewRepository(db))
	prefs, err := preferences.Open(paths.SettingsPath(), db, service)
	if err != nil {
		return err
	}
	return desktop.Run(ctx, service, prefs, wordmark, mark)
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
