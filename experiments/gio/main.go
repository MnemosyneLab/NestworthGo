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
	"gioui.org/op"
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
	paths, err := pilotPathsFromEnv()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if args[0] == "--prepare-pilot-fixture" {
		return preparePilotFixture(ctx, paths)
	}
	if _, err := verifyPilotFixture(ctx, paths); err != nil {
		return err
	}
	fmt.Println("Verified the isolated Gio Institutions QA fixture.")
	return nil
}

func run() (result error) {
	paths, err := pilotPathsFromEnv()
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	db, err := openPilot(ctx, paths)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, db.CheckpointWAL(context.Background()), db.Close())
	}()
	window := new(app.Window)
	directory := newInstitutionsPage(ctx, application.NewService(sqlite.NewRepository(db)), window.Invalidate)
	defer func() {
		cancel()
		directory.workers.Wait()
	}()
	return newShell(wordmark, mark, directory.layout).run(window)
}

func (s *shell) run(window *app.Window) error {
	window.Option(app.Title("Nestworth"), app.Size(1120, 840), app.MinSize(700, 460))
	var ops op.Ops
	for {
		switch event := window.Event().(type) {
		case app.DestroyEvent:
			return event.Err
		case app.ConfigEvent:
			if !event.Config.Focused && s.overlay.trigger != nil {
				s.overlay.close()
				window.Invalidate()
			}
		case app.FrameEvent:
			gtx := app.NewContext(&ops, event)
			s.layout(gtx)
			event.Frame(gtx.Ops)
		}
	}
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
