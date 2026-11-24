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
			fmt.Fprintln(os.Stderr, "Nestworth could not start:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func pilotCommand(args []string) error {
	if len(args) != 1 || (args[0] != "--prepare-pilot-fixture" && args[0] != "--verify-pilot-fixture") {
		return errors.New("expected --prepare-pilot-fixture or --verify-pilot-fixture")
	}
	paths, err := pilot.PathsFromEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if args[0] == "--prepare-pilot-fixture" {
		return pilot.Prepare(ctx, paths)
	}
	if err := pilot.Verify(ctx, paths); err != nil {
		return err
	}
	fmt.Println("Verified the isolated Gio Institutions QA fixture.")
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
	db, err := pilot.Open(ctx, paths)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, db.CheckpointWAL(context.Background()), db.Close())
	}()
	return desktop.Run(ctx, application.NewService(sqlite.NewRepository(db)), wordmark, mark)
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
