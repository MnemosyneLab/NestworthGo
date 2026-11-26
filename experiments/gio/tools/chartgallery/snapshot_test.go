package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/snapshot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// TestGallerySnapshots renders every chart, dataset, theme and width of the
// gallery to PNG for review. Like the desktop snapshots it needs a GPU backend
// but no window server, asserts nothing, and runs only when
// NESTWORTH_GIO_SNAPSHOT_DIR names an absolute directory:
//
//	NESTWORTH_GIO_SNAPSHOT_DIR=/tmp/chart-shots go -C experiments/gio test \
//	  -mod=readonly -count=1 -run TestGallerySnapshots ./tools/chartgallery
func TestGallerySnapshots(t *testing.T) {
	dir := os.Getenv("NESTWORTH_GIO_SNAPSHOT_DIR")
	if dir == "" {
		t.Skip("set NESTWORTH_GIO_SNAPSHOT_DIR to write chart snapshots")
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("NESTWORTH_GIO_SNAPSHOT_DIR must be absolute, got %q", dir)
	}
	sizes := []struct {
		name string
		size image.Point
	}{{"wide", image.Pt(1100, 900)}, {"narrow", image.Pt(390, 900)}}
	for chart := range chartNames {
		for variant := range variantCount(chart) {
			for _, dark := range []bool{false, true} {
				for _, size := range sizes {
					o := options{chart: chart, variant: variant, dark: dark, selectSample: variant < 2, size: size.size}
					theme := "light"
					if dark {
						theme = "dark"
					}
					name := fmt.Sprintf("%s-%s-%s-%s", chartNames[chart], variantNames[variant], theme, size.name)
					writeGalleryFrame(t, dir, name, o)
				}
			}
		}
	}
}

func writeGalleryFrame(t *testing.T, dir, name string, o options) {
	t.Helper()
	const scale = 2
	g := o.gallery()
	view := ui.View{Language: i18n.English, Colors: palette(g.dark), Theme: ui.NewTheme(), Motion: &ui.Motion{}}
	var router input.Router
	var ops op.Ops
	now := time.Unix(1, 0)
	size := o.size.Mul(scale)
	for range 3 {
		ops.Reset()
		now = now.Add(time.Second / 60)
		gtx := layout.Context{
			Ops: &ops, Source: router.Source(), Now: now,
			Metric:      unit.Metric{PxPerDp: scale, PxPerSp: scale},
			Constraints: layout.Exact(size),
		}
		paint.Fill(gtx.Ops, view.Colors.PageBackground)
		g.layout(gtx, view)
		router.Frame(&ops)
	}
	img, err := snapshot.Render(size, &ops)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.WritePNG(filepath.Join(dir, name+".png"), img); err != nil {
		t.Fatal(err)
	}
}
