package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"gioui.org/app"
	"gioui.org/op"
)

func main() {
	go func() {
		if err := run(); err != nil {
			fmt.Fprintln(os.Stderr, "Nestworth could not start:", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run() error {
	wordmark, err := loadBrandImage("wordmark.png")
	if err != nil {
		return err
	}
	mark, err := loadBrandImage("mark.png")
	if err != nil {
		return err
	}
	return newShell(wordmark, mark).run()
}

func (s *shell) run() error {
	window := new(app.Window)
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
