// Command chartgallery opens a native Gio window that shows the shared chart
// components (internal/ui/charts) with synthetic data. It reads no ledger and
// opens no database; it is a developer tool for reviewing chart layout, themes,
// keyboard and pointer behavior.
//
// Usage, from experiments/gio:
//
//	go run ./tools/chartgallery
//	go run ./tools/chartgallery -dark -chart donut -variant 1 -size 390x844
//
// The window title and process are those of a plain development binary, not an
// application bundle, so it never touches the Nestworth Gio profile.
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"strconv"
	"strings"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type options struct {
	dark           bool
	chart, variant int
	selectSample   bool
	size           image.Point
}

func parseOptions(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("chartgallery", flag.ContinueOnError)
	chart := fs.String("chart", "line", "initial chart: "+strings.Join(chartNames[:], ", "))
	variant := fs.Int("variant", 0, "initial dataset: 0 standard, 1 edge cases, 2 no values, 3 empty")
	size := fs.String("size", "1100x900", "initial window size in dp, WIDTHxHEIGHT")
	fs.BoolVar(&o.dark, "dark", false, "start in the dark appearance")
	fs.BoolVar(&o.selectSample, "select", false, "start with a sample item selected")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.chart = -1
	for i, name := range chartNames {
		if name == *chart {
			o.chart = i
		}
	}
	if o.chart < 0 {
		return o, fmt.Errorf("unknown chart %q", *chart)
	}
	if *variant < 0 || *variant >= len(variantNames) {
		return o, fmt.Errorf("variant %d is out of range", *variant)
	}
	o.variant = *variant
	w, h, ok := strings.Cut(*size, "x")
	width, errW := strconv.Atoi(w)
	height, errH := strconv.Atoi(h)
	if !ok || errW != nil || errH != nil || width < 200 || height < 200 {
		return o, fmt.Errorf("size %q must look like 1100x900", *size)
	}
	o.size = image.Pt(width, height)
	return o, nil
}

func (o options) gallery() *gallery {
	g := newGallery()
	g.chart, g.variant, g.dark = o.chart, o.variant, o.dark
	if o.selectSample {
		g.selected[g.chart] = g.sampleSelection()
	}
	return g
}

func main() {
	o, err := parseOptions(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	go func() {
		window := new(app.Window)
		window.Option(app.Title("Chart gallery"), app.Size(unit.Dp(o.size.X), unit.Dp(o.size.Y)))
		if err := run(window, o.gallery()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func run(window *app.Window, g *gallery) error {
	theme := ui.NewTheme()
	var ops op.Ops
	var motion ui.Motion
	for {
		switch e := window.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			view := ui.View{Language: i18n.English, Colors: palette(g.dark), Theme: theme, Motion: &motion}
			paint.Fill(gtx.Ops, view.Colors.PageBackground)
			layout.Inset{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return g.layout(gtx, view) })
			motion.EndFrame()
			e.Frame(gtx.Ops)
		}
	}
}
