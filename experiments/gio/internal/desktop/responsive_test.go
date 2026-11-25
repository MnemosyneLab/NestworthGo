package desktop

import (
	"image"
	"testing"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

var responsiveSizes = []image.Point{{700, 460}, {820, 640}, {1120, 840}, {1600, 1000}}

func TestShellAdaptsToWindowSize(t *testing.T) {
	for _, size := range responsiveSizes {
		h := newPreviewHarness(t)
		h.size = size
		h.frame()
		if got, want := h.shell.narrow, size.X < int(ui.BreakpointMedium); got != want {
			t.Errorf("%v: narrow = %v, want %v", size, got, want)
		}
	}
}

// The preview scrolls as a whole, so every control stays reachable however
// short the window is.
func TestPreviewControlsStayReachableAtEverySize(t *testing.T) {
	window := func(size image.Point) image.Rectangle { return image.Rectangle{Max: size} }
	for _, size := range responsiveSizes {
		h := newPreviewHarness(t)
		h.size = size
		h.frame()
		h.preview.scroll.ScrollBy(1) // one item holds the whole page
		h.frame()
		h.frame()
		for _, control := range []struct {
			label string
			field bool
		}{{"Submit sample", false}, {"About this form", false}, {"Effective time", true}, {"Account type", true}} {
			bounds := semanticBounds(t, &h.router, control.label, control.field)
			if bounds.Empty() || !bounds.In(window(size)) {
				t.Errorf("%v: %q is outside the window at %v", size, control.label, bounds)
			}
		}
	}
}
