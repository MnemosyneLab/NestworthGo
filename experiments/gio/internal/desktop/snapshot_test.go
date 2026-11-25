package desktop

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/font"
	"gioui.org/io/input"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/snapshot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// Visual snapshots render real frames on the GPU. They are an opt-in review
// tool, not an assertion: set NESTWORTH_GIO_SNAPSHOT_DIR to an absolute output
// directory to write PNGs. Ordinary test runs skip them.
//
//	NESTWORTH_GIO_SNAPSHOT_DIR=/tmp/gio-shots go -C experiments/gio test \
//	  -mod=readonly -run TestVisualSnapshots ./internal/desktop
const snapshotEnv = "NESTWORTH_GIO_SNAPSHOT_DIR"

// snapshotScale renders at the density of a Retina display.
const snapshotScale = 2

func snapshotDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv(snapshotEnv)
	if dir == "" {
		t.Skipf("set %s to write visual snapshots", snapshotEnv)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("%s must be an absolute path, got %q", snapshotEnv, dir)
	}
	return dir
}

func writeSnapshot(t *testing.T, dir, name string, size image.Point, ops *op.Ops) {
	t.Helper()
	img, err := snapshot.Render(size, ops)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.WritePNG(filepath.Join(dir, name+".png"), img); err != nil {
		t.Fatal(err)
	}
}

func TestVisualSnapshots(t *testing.T) {
	dir := snapshotDir(t)
	accents := []settings.Accent{
		settings.AccentIndigo, settings.AccentLavender, settings.AccentMint,
		settings.AccentSky, settings.AccentPeach,
	}
	modes := []struct {
		name       string
		appearance settings.Appearance
	}{{"light", settings.AppearanceLight}, {"dark", settings.AppearanceDark}}

	t.Run("shell", func(t *testing.T) {
		for _, accent := range accents {
			for _, mode := range modes {
				h := newPreviewHarness(t)
				h.shell.prefs.saved.Accent, h.shell.prefs.saved.Appearance = accent, mode.appearance
				h.snapshot(t, dir, fmt.Sprintf("preview-%s-%s", accent, mode.name))
			}
		}
	})
	t.Run("languages", func(t *testing.T) {
		for _, language := range []settings.Language{
			settings.LanguageEnglish, settings.LanguageZhCN, settings.LanguageZhTW, settings.LanguageJa,
		} {
			h := newPreviewHarness(t)
			h.shell.prefs.saved.Language = language
			h.snapshot(t, dir, fmt.Sprintf("preview-language-%s", language))
		}
	})
	t.Run("settings", func(t *testing.T) {
		for _, mode := range modes {
			h := newPreviewHarness(t)
			h.shell.trail = newNavigationTrail(settingsRoute)
			h.shell.prefs.saved.Appearance = mode.appearance
			h.snapshot(t, dir, "settings-"+mode.name)
		}
	})
	t.Run("gallery", func(t *testing.T) {
		for _, accent := range []struct {
			name  string
			index int
		}{{"indigo", 0}, {"mint", 2}} {
			for _, mode := range modes {
				for _, scenario := range galleryScenarios {
					h := newGalleryHarness(t, accent.index, mode.appearance == settings.AppearanceDark)
					h.run(scenario)
					h.snapshot(t, dir, fmt.Sprintf("gallery-%s-%s-%s", accent.name, mode.name, scenario.name))
				}
			}
		}
	})
}

func (h *previewHarness) snapshot(t *testing.T, dir, name string) {
	t.Helper()
	h.scale = snapshotScale
	size := h.size
	h.size = size.Mul(snapshotScale)
	defer func() { h.size, h.scale = size, 0 }()
	h.frame()
	// A second frame lets one-frame state, such as focus and layout-derived
	// positions, settle before the capture.
	h.frame()
	writeSnapshot(t, dir, name, h.size, &h.ops)
}

// controlGallery lays out the shared controls in every state a reviewer needs
// to compare. It is the visual contract for later control work: change a
// control, regenerate the snapshots and review the matrix.
type controlGallery struct {
	secondary, secondaryDisabled, primary, primaryDisabled widget.Clickable
	tabOff, tabOn                                          widget.Clickable
	rest, filled, focused, invalid, disabled, readOnly     ui.TextField
	search                                                 ui.SearchField
	scroll                                                 layout.List
}

func newControlGallery() *controlGallery {
	g := &controlGallery{
		rest: ui.NewTextField(true), filled: ui.NewTextField(true), focused: ui.NewTextField(true),
		invalid: ui.NewTextField(true), disabled: ui.NewTextField(true), readOnly: ui.NewTextField(true),
		search: ui.SearchField{Field: ui.NewTextField(true)},
		scroll: layout.List{Axis: layout.Vertical},
	}
	g.filled.Editor.SetText("Harbor Savings")
	g.focused.Editor.SetText("Harbor Savings")
	g.invalid.Editor.SetText("")
	g.disabled.Editor.SetText("Evergreen Credit Union")
	g.readOnly.Editor.SetText("Westlake Bank")
	g.search.Field.Editor.SetText("harbor")
	return g
}

func (g *controlGallery) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	section := func(title string, rows ...layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, title, 12, font.Medium, view.Colors.SecondaryInk)
			})}
			for _, row := range rows {
				children = append(children, layout.Rigid(row))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(10)}.Layout(gtx, children...)
		}
	}
	row := func(cells ...layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, len(cells))
			for i, cell := range cells {
				children[i] = layout.Flexed(1, cell)
			}
			return layout.Flex{Gap: gtx.Dp(14)}.Layout(gtx, children...)
		}
	}
	field := func(f *ui.TextField, options ui.FieldOptions) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions { return f.Layout(gtx, view, options) }
	}
	left := func(w layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = 0
			return layout.W.Layout(gtx, w)
		}
	}
	buttons := section("Buttons",
		layout.Widget(func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Gap: gtx.Dp(12), Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(view.NamedButton(&g.secondary, "Secondary", "secondary")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx = gtx.Disabled()
					return view.NamedButton(&g.secondaryDisabled, "Disabled", "secondary disabled")(gtx)
				}),
				layout.Rigid(view.PrimaryButton(&g.primary, "Primary")),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx = gtx.Disabled()
					return view.PrimaryButton(&g.primaryDisabled, "Disabled")(gtx)
				}),
				layout.Rigid(view.TabButton(&g.tabOff, "Tab", false)),
				layout.Rigid(view.TabButton(&g.tabOn, "Selected tab", true)),
			)
		}),
	)
	fields := section("Text fields",
		row(
			field(&g.rest, ui.FieldOptions{Label: i18n.PreviewInstitutionName, Hint: i18n.PreviewInstitutionHint}),
			field(&g.filled, ui.FieldOptions{Label: i18n.PreviewInstitutionName, Hint: i18n.PreviewInstitutionHint, MaxBytes: institutionNameBudget}),
		),
		row(
			field(&g.focused, ui.FieldOptions{Label: i18n.PreviewInstitutionName, Hint: i18n.PreviewInstitutionHint}),
			field(&g.invalid, ui.FieldOptions{
				Label: i18n.PreviewInstitutionName, Hint: i18n.PreviewInstitutionHint,
				Problem: func(string) string { return view.Language.Text(i18n.PreviewNameRequired) },
			}),
		),
		row(
			field(&g.disabled, ui.FieldOptions{Label: i18n.PreviewDisabled, Disabled: true}),
			field(&g.readOnly, ui.FieldOptions{Label: i18n.PreviewReadOnly, ReadOnly: true}),
		),
		func(gtx layout.Context) layout.Dimensions {
			return g.search.Layout(ui.LimitWidth(gtx, 420), view, i18n.PreviewSearch, i18n.PreviewSearchHint, func(string) {})
		},
	)
	surfaces := section("Surfaces and text",
		func(gtx layout.Context) layout.Dimensions {
			return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return view.Label(gtx, "Net worth", 13, font.Normal, view.Colors.SecondaryInk)
						}),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							label := material.Label(view.Theme, 32, "CNY 1,234,567.89")
							label.Color, label.Font.Weight = view.Colors.Ink, font.Medium
							return label.Layout(gtx)
						}),
						layout.Rigid(view.Message("Secondary text sits under the figure.", view.Colors.SecondaryInk)),
						layout.Rigid(view.Message("Something needs attention.", view.Colors.ErrorInk)),
					)
				})
			})
		},
	)
	return g.scroll.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(26)}.Layout(gtx,
			layout.Rigid(left(buttons)), layout.Rigid(fields), layout.Rigid(surfaces),
		)
	})
}

// layoutTokens draws the semantic palette: status, provenance and category
// colors with their soft tints, plus the accent surface vocabulary.
func (g *controlGallery) layoutTokens(gtx layout.Context, view ui.View) layout.Dimensions {
	p := view.Colors
	swatch := func(name string, fill, edge color.NRGBA) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					size := image.Pt(gtx.Dp(76), gtx.Dp(44))
					bounds := image.Rectangle{Max: size}
					radius := gtx.Dp(ui.RadiusLG)
					paint.FillShape(gtx.Ops, edge, clip.UniformRRect(bounds, radius).Op(gtx.Ops))
					paint.FillShape(gtx.Ops, fill, clip.UniformRRect(bounds.Inset(1), radius-1).Op(gtx.Ops))
					return layout.Dimensions{Size: size}
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = gtx.Dp(84)
					return view.Label(gtx, name, 11, font.Normal, p.SecondaryInk)
				}),
			)
		}
	}
	group := func(title string, swatches ...layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			children := make([]layout.FlexChild, 0, len(swatches))
			for _, s := range swatches {
				children = append(children, layout.Rigid(s))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, title, 12, font.Medium, p.SecondaryInk)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(10)}.Layout(gtx, children...)
				}),
			)
		}
	}
	edge := p.Divider
	surfaces := group("Surfaces and ink",
		swatch("page", p.PageBackground, edge), swatch("sidebar", p.SidebarBackground, edge),
		swatch("surface", p.Surface, edge), swatch("hover", p.HoverSurface, edge),
		swatch("divider", p.Divider, edge), swatch("ink", p.Ink, edge),
		swatch("secondary", p.SecondaryInk, edge), swatch("accent", p.Accent, edge),
		swatch("on accent", p.OnAccent, edge),
	)
	status := group("Status, solid and tinted",
		swatch("success", p.Success, edge), swatch("success 14%", p.Tint(p.Success, 0.14), edge),
		swatch("warning", p.Warning, edge), swatch("warning 18%", p.Tint(p.Warning, 0.18), edge),
		swatch("warning ink", p.WarningInk, edge),
		swatch("destructive", p.Destructive, edge), swatch("destr. 12%", p.Tint(p.Destructive, 0.12), edge),
		swatch("accent 10%", p.Tint(p.Accent, 0.10), edge), swatch("error ink", p.ErrorInk, edge),
	)
	provenance := group("Data provenance",
		swatch("current", p.Provenance.Current, edge), swatch("manual", p.Provenance.Manual, edge),
		swatch("remote", p.Provenance.Remote, edge), swatch("stale", p.Provenance.Stale, edge),
		swatch("unavailable", p.Provenance.Unavailable, edge),
	)
	var categories, softs []layout.Widget
	for i := range p.Category {
		categories = append(categories, swatch(fmt.Sprintf("cat %d", i+1), p.Category[i], edge))
		softs = append(softs, swatch(fmt.Sprintf("soft %d", i+1), p.CategorySoft(i), edge))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(22)}.Layout(gtx,
		layout.Rigid(surfaces), layout.Rigid(status), layout.Rigid(provenance),
		layout.Rigid(group("Category", categories...)), layout.Rigid(group("Category, soft", softs...)),
	)
}

// layoutElevation compares the shadow scale, hairline borders and focus rings
// on the page background and on a tinted surface.
func (g *controlGallery) layoutElevation(gtx layout.Context, view ui.View) layout.Dimensions {
	p := view.Colors
	card := func(name string, level ui.Elevation) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			size := image.Pt(gtx.Dp(120), gtx.Dp(84))
			bounds := image.Rectangle{Max: size}
			p.DrawShadow(gtx, bounds, ui.RadiusXL, level)
			ui.FillRoundRect(gtx, bounds, ui.RadiusXL, p.Surface)
			ui.StrokeRoundRect(gtx, bounds, ui.RadiusXL, 1, p.Divider)
			gtx.Constraints = layout.Exact(size)
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, name, 12, font.Medium, p.Ink)
			})
			return layout.Dimensions{Size: size}
		}
	}
	row := func(title string, children ...layout.Widget) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			flex := make([]layout.FlexChild, len(children))
			for i, child := range children {
				flex[i] = layout.Rigid(child)
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, title, 12, font.Medium, p.SecondaryInk)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Gap: gtx.Dp(28)}.Layout(gtx, flex...)
				}),
			)
		}
	}
	rings := func(name string, radius unit.Dp, size image.Point, ring func(layout.Context, image.Rectangle)) layout.Widget {
		return func(gtx layout.Context) layout.Dimensions {
			s := image.Pt(gtx.Dp(unit.Dp(size.X)), gtx.Dp(unit.Dp(size.Y)))
			bounds := image.Rectangle{Max: s}
			ui.FillRoundRect(gtx, bounds, radius, p.Surface)
			ui.StrokeRoundRect(gtx, bounds, radius, 1, p.Divider)
			if ring != nil {
				ring(gtx, bounds)
			}
			gtx.Constraints = layout.Exact(s)
			layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, name, 12, font.Normal, p.Ink)
			})
			return layout.Dimensions{Size: s}
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(34)}.Layout(gtx,
		layout.Rigid(row("Elevation: none, xs, sm, md, lg",
			card("none", ui.ElevationNone), card("xs", ui.ElevationXS), card("sm", ui.ElevationSM),
			card("md", ui.ElevationMD), card("lg", ui.ElevationLG))),
		layout.Rigid(row("Focus ring (outside, 2dp gap) and input focus border",
			rings("button", ui.RadiusLG, image.Pt(110, 36), func(gtx layout.Context, b image.Rectangle) { p.FocusRing(gtx, b, ui.RadiusLG) }),
			rings("input", ui.RadiusLG, image.Pt(180, 36), func(gtx layout.Context, b image.Rectangle) { p.FocusBorder(gtx, b, ui.RadiusLG) }),
			rings("pill", ui.RadiusFull, image.Pt(90, 28), func(gtx layout.Context, b image.Rectangle) { p.FocusRing(gtx, b, ui.RadiusFull) }),
			rings("no ring", ui.RadiusLG, image.Pt(110, 36), nil))),
	)
}

// galleryScenario drives pointer state. Only one pointer exists, so each
// interactive state gets its own snapshot.
type galleryScenario struct {
	name         string
	hover, press string
	focusField   bool
	page         string
}

var galleryScenarios = []galleryScenario{
	{name: "rest", focusField: true},
	{name: "hover-secondary", hover: "Secondary", focusField: true},
	{name: "hover-primary", hover: "Primary", focusField: true},
	{name: "press-primary", press: "Primary", focusField: true},
	{name: "tokens", page: "tokens"},
	{name: "elevation", page: "elevation"},
}

type galleryHarness struct {
	t       *testing.T
	gallery *controlGallery
	view    ui.View
	router  input.Router
	ops     op.Ops
	now     time.Time
	size    image.Point
	scale   float32
	page    string
}

func newGalleryHarness(t *testing.T, accent int, dark bool) *galleryHarness {
	t.Helper()
	prefs := sessionPreferences{language: i18n.English, dark: dark, accentIndex: accent}
	colors := prefs.colors()
	theme := material.NewTheme()
	theme.Palette = material.Palette{Bg: colors.PageBackground, Fg: colors.Ink, ContrastBg: colors.Accent, ContrastFg: colors.OnAccent}
	return &galleryHarness{
		t: t, gallery: newControlGallery(), now: time.Unix(1, 0),
		size: image.Pt(900, 740), scale: snapshotScale,
		view: ui.View{
			Language: prefs.language, Colors: colors, Theme: theme, Overlay: &ui.Overlay{},
			DateFormat: "YYYY-MM-DD", DecimalSeparator: ".", GroupingSeparator: ",",
			TimeLocation: time.UTC,
		},
	}
}

func (h *galleryHarness) frame(focus bool) {
	h.ops.Reset()
	h.now = h.now.Add(time.Second / 60)
	gtx := layout.Context{
		Ops: &h.ops, Source: h.router.Source(), Now: h.now,
		Metric:      unit.Metric{PxPerDp: h.scale, PxPerSp: h.scale},
		Constraints: layout.Exact(h.size.Mul(int(h.scale))),
	}
	gtx.Locale.Language = string(h.view.Language)
	paint.Fill(gtx.Ops, h.view.Colors.PageBackground)
	layout.UniformInset(28).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if focus {
			h.gallery.focused.Focus(gtx)
		}
		switch h.page {
		case "tokens":
			return h.gallery.layoutTokens(gtx, h.view)
		case "elevation":
			return h.gallery.layoutElevation(gtx, h.view)
		}
		return h.gallery.layout(gtx, h.view)
	})
	h.router.Frame(&h.ops)
}

func (h *galleryHarness) run(s galleryScenario) {
	h.page = s.page
	h.frame(s.focusField)
	h.frame(false)
	target := s.hover
	if s.press != "" {
		target = s.press
	}
	if target == "" {
		return
	}
	bounds := semanticBounds(h.t, &h.router, target, false)
	centre := bounds.Min.Add(bounds.Max).Div(2)
	pos := f32.Pt(float32(centre.X), float32(centre.Y))
	h.router.Queue(pointer.Event{Kind: pointer.Move, Source: pointer.Mouse, Position: pos})
	if s.press != "" {
		h.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: pos})
	}
	h.frame(false)
	h.frame(false)
}

func (h *galleryHarness) snapshot(t *testing.T, dir, name string) {
	t.Helper()
	writeSnapshot(t, dir, name, h.size.Mul(int(h.scale)), &h.ops)
}
