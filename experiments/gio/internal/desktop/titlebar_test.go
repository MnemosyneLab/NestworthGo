package desktop

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/snapshot"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/testledger"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/settings"
)

// These are representative native point geometries, NOT rendered native buttons.
// The same production shell is used on all platforms in this test.
var normalTitlebar = titlebarMetrics{active: true, buttonRight: 72, buttonBottom: 24}

func syntheticTitlebarHarness(t *testing.T) *previewHarness {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "synthetic.db")
	if err := testledger.Seed(t.Context(), path, filepath.Join(root, "settings.json")); err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return testledger.HistoryStart().Add(48 * time.Hour) }
	_, _, service := openStage6Database(t, path, clock, application.NewMarketDataRegistry(&unavailableOverviewProvider{}))
	overview := newOverviewPage(t.Context(), service, clock, func() {})
	timeline := newLedgerHistory(t.Context(), service, clock, func() {})
	t.Cleanup(func() {
		overview.workers.Wait()
		timeline.model.Wait()
		timeline.reads.workers.Wait()
		timeline.cashMovements.workers.Wait()
	})
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, history: timeline})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	readBrand := func(path string) paint.ImageOp {
		f, err := os.Open(filepath.Join("..", "..", "..", "..", path))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			t.Fatal(err)
		}
		return paint.NewImageOp(img)
	}
	h.shell.wordmark.Src = readBrand("frontend/src/assets/brand/wordmark.png")
	h.shell.mark.Src = readBrand("assets/brand/favicon.png")
	h.shell.titlebar = normalTitlebar
	h.frame()
	overview.workers.Wait()
	h.frame()
	if overview.current == nil || overview.err != nil {
		t.Fatalf("synthetic overview did not load: %v", overview.err)
	}
	return h
}

func checkTitlebarControls(t *testing.T, h *previewHarness, record bool) {
	t.Helper()
	s := h.shell
	g := s.chrome
	toggle := semanticBounds(t, &h.router, "Toggle sidebar", false)
	if toggle != g.toggle {
		t.Fatalf("toggle bounds %v != native-relative layout %v", toggle, g.toggle)
	}
	if toggle.Min.X < h.metric().Dp(s.titlebar.buttonRight)+h.metric().Dp(8) || toggle.Min.Y < g.top || toggle.Max.Y > g.height {
		t.Fatalf("toggle %v overlaps the native reserved area (geometry %+v)", toggle, g)
	}
	labels := []string{"Back", "Forward", "Recent"}
	if record {
		labels = append(labels, "Record change")
	}
	last := toggle
	for _, label := range labels {
		b := semanticBounds(t, &h.router, label, false)
		if b.Min.X < g.split || b.Min.X < last.Max.X || b.Min.Y < g.top || b.Max.Y > g.height || b.Max.X > h.size.X-g.rightInset {
			t.Fatalf("%s (%v) overlaps chrome or another control (%v), geometry %+v", label, b, last, g)
		}
		for _, drag := range g.drag {
			if b.Overlaps(drag) || toggle.Overlaps(drag) {
				t.Fatalf("drag zone %v overlaps control %s %v or toggle %v", drag, label, b, toggle)
			}
		}
		last = b
	}
}

func TestIntegratedTitlebar(t *testing.T) {
	h := syntheticTitlebarHarness(t)
	s := h.shell
	// Create a real navigation visit so Recent and Forward are enabled.
	h.click("Settings", false)
	h.click("Back", false)
	// Runtime changes, without remounting the shell or replacing the ledger.
	for _, accent := range settings.AllAccents() {
		for _, mode := range []settings.Appearance{settings.AppearanceLight, settings.AppearanceDark} {
			before := s.prefs.saved
			after := before
			after.Accent, after.Appearance = accent, mode
			s.prefs.save(before, after, nil)
			s.prefs.wait()
			h.frame()
			if got := s.preferences; got.dark != (mode == settings.AppearanceDark) || settings.AllAccents()[got.accentIndex] != accent {
				t.Fatalf("did not apply %s/%s: %+v", accent, mode, got)
			}
			for _, scale := range []float32{1, 1.5, 2} {
				h.scale = scale
				for _, width := range []int{640, 1120} {
					h.size = image.Pt(int(float32(width)*scale), int(840*scale))
					for _, collapsed := range []bool{false, true} {
						s.wideCollapsed, s.narrowExpanded = collapsed, !collapsed
						h.frame()
						checkTitlebarControls(t, h, false)
						if s.collapsed() != collapsed {
							t.Fatal("sidebar state changed during appearance/resize")
						}
						// Fake full-screen safe area and no traffic lights. No
						// phantom reservation should remain after hiding lights.
						s.titlebar = titlebarMetrics{active: true, safeTop: 32, safeLeft: 4, safeRight: 4}
						h.frame()
						checkTitlebarControls(t, h, false)
						if s.chrome.toggle.Min.X != h.metric().Dp(16) {
							t.Fatalf("fullscreen retained traffic light gap: %v", s.chrome.toggle)
						}
						s.titlebar = normalTitlebar
					}
				}
			}
		}
	}
	// Follow-system appearance changes publish through the same live palette.
	s.prefs.saved.Appearance = settings.AppearanceSystem
	for _, dark := range []bool{false, true} {
		s.prefs.systemDark = dark
		h.frame()
		if s.preferences.dark != dark {
			t.Fatal("system appearance change was not applied")
		}
	}
	// Exercise navigation and popup anchoring through the real Gio router.
	h.scale, h.size = 1, image.Pt(640, 840)
	s.narrowExpanded = false
	h.frame()
	h.click("Toggle sidebar", false)
	if s.collapsed() {
		t.Fatal("toggle did not expand the narrow rail")
	}
	h.click("Ledger activity", false)
	s.historyPage.model.Wait()
	h.frame()
	if !s.collapsed() {
		t.Fatal("narrow route visit did not collapse sidebar")
	}
	h.click("Toggle sidebar", false)
	checkTitlebarControls(t, h, true)
	h.click("Back", false)
	if s.activeRoute() != overviewRoute {
		t.Fatal("Back did not return to Overview")
	}
	h.click("Forward", false)
	if s.activeRoute() != historyRoute {
		t.Fatal("Forward did not restore History")
	}
	h.click("Recent", false)
	if s.overlay.Trigger != &s.recentPicker.Trigger || s.overlay.Panel.Empty() || s.titlebarDrags() != ([2]image.Rectangle{}) {
		t.Fatal("Recent did not open or native drag was not disabled while menu is open")
	}
	h.press(key.NameEscape)
	h.focused(&s.recentPicker.Trigger)
	h.click("Toggle sidebar", false)
	if s.collapsed() {
		t.Fatal("toggle did not expand sidebar after menu dismiss")
	}
	checkTitlebarControls(t, h, true)
	h.click("Toggle sidebar", false)
	if !s.collapsed() {
		t.Fatal("toggle did not collapse sidebar")
	}
	checkTitlebarControls(t, h, true)
	// Failed/missing native attachment returns to the ordinary brand toggle.
	s.titlebar = titlebarMetrics{}
	h.frame()
	if s.chrome.split != s.chrome.sidebar || s.chrome.height != headerHeight || s.titlebarDrags() != ([2]image.Rectangle{}) {
		t.Fatalf("fallback retained integrated chrome: %+v", s.chrome)
	}
	h.click("Toggle sidebar", false)
	if s.collapsed() {
		t.Fatal("fallback toggle was not clickable")
	}
}

func TestIntegratedTitlebarSnapshots(t *testing.T) {
	dir := snapshotDir(t)
	h := syntheticTitlebarHarness(t)
	h.size = image.Pt(1120, 840)
	s := h.shell
	render := func(name string) {
		t.Helper()
		h.frame()
		img, err := snapshot.Render(h.size, &h.ops)
		if err != nil {
			t.Fatal(err)
		}
		p := s.preferences.colors()
		// Sample actual GPU-rendered top and sidebar surfaces, away from
		// controls. No fake lights or OS frame are drawn.
		for _, sample := range []struct {
			point image.Point
			want  color.NRGBA
		}{
			{image.Pt(2, 2), p.SidebarBackground},
			{image.Pt(s.chrome.split-2, 2), p.SidebarBackground},
			{image.Pt(s.chrome.split+2, 2), p.PageBackground},
			{image.Pt(2, s.chrome.height+2), p.SidebarBackground},
		} {
			got := color.NRGBAModel.Convert(img.At(sample.point.X, sample.point.Y)).(color.NRGBA)
			if got != sample.want {
				t.Fatalf("%s at %v is %v, want %v", name, sample.point, got, sample.want)
			}
		}
		if err := snapshot.WritePNG(filepath.Join(dir, name+".png"), img); err != nil {
			t.Fatal(err)
		}
	}
	for _, accent := range settings.AllAccents() {
		for _, mode := range []settings.Appearance{settings.AppearanceLight, settings.AppearanceDark} {
			s.prefs.saved.Accent, s.prefs.saved.Appearance = accent, mode
			for _, collapsed := range []bool{false, true} {
				s.wideCollapsed = collapsed
				render(fmt.Sprintf("titlebar-%s-%s-collapsed-%t", accent, mode, collapsed))
			}
		}
	}
	h.scale, h.size = 2, image.Pt(1280, 1680)
	s.narrowExpanded = false
	render("titlebar-narrow-2x")
	s.titlebar = titlebarMetrics{active: true, safeTop: 32, safeLeft: 4, safeRight: 4}
	render("titlebar-fullscreen-safe-area-2x")
}

func TestIntegratedTitlebarGeometry(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 2, PxPerSp: 2}, Constraints: layout.Exact(image.Pt(1280, 1000))}
	for _, left := range []unit.Dp{0, 50, 72, 100, 145} {
		for _, rail := range []unit.Dp{sidebarRailWidth, sidebarWidth} {
			g := calculateTitlebar(gtx, titlebarMetrics{active: true, buttonRight: left, buttonBottom: 50}, rail)
			if g.toggle.Min.X < gtx.Dp(left)+gtx.Dp(8) || g.toggle.Max.Y >= g.height ||
				(g.toggle.Min.X < g.sidebar && g.toggle.Max.X >= g.sidebar) || g.split < g.toggle.Max.X || g.height < gtx.Dp(58) {
				t.Fatalf("invalid geometry for native right %v and sidebar %v: %+v", left, rail, g)
			}
		}
	}
}

// TestExportIntegratedTitlebarFixture is an opt-in, test-only way to start the
// actual preview launcher with known synthetic data. Never touch a default
// profile or overwrite files in an existing directory.
func TestExportIntegratedTitlebarFixture(t *testing.T) {
	dir := os.Getenv("NESTWORTH_GIO_TITLEBAR_FIXTURE_DIR")
	if dir == "" {
		t.Skip("set NESTWORTH_GIO_TITLEBAR_FIXTURE_DIR to a new, empty absolute directory")
	}
	if !filepath.IsAbs(dir) {
		t.Fatal("fixture directory must be absolute")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("fixture directory must exist and be empty: %v (entries: %d)", err, len(entries))
	}
	if err := testledger.Seed(t.Context(), filepath.Join(dir, "nestworth.db"), filepath.Join(dir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	t.Log("Seeded Harbor Sample Household (synthetic only) in", dir)
}
