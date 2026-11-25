// Package snapshot renders recorded Gio operations to an image so visual
// states can be reviewed without a window server or screen-recording access.
//
// The package pulls the GPU backend into whatever imports it. Keep it out of
// production code; only tests and tooling use it.
package snapshot

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	"gioui.org/gpu/headless"
	"gioui.org/op"
)

// Render draws ops into an image of the given pixel size.
func Render(size image.Point, ops *op.Ops) (*image.RGBA, error) {
	if size.X <= 0 || size.Y <= 0 {
		return nil, fmt.Errorf("snapshot: invalid size %v", size)
	}
	window, err := headless.NewWindow(size.X, size.Y)
	if err != nil {
		return nil, fmt.Errorf("snapshot: open headless window: %w", err)
	}
	defer window.Release()
	if err := window.Frame(ops); err != nil {
		return nil, fmt.Errorf("snapshot: draw frame: %w", err)
	}
	img := image.NewRGBA(image.Rectangle{Max: size})
	if err := window.Screenshot(img); err != nil {
		return nil, fmt.Errorf("snapshot: read frame: %w", err)
	}
	return img, nil
}

// WritePNG stores img at path, creating parent directories as needed.
func WritePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
