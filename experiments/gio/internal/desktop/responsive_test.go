package desktop

import (
	"fmt"
	"image"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
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

// openLedgerActivity builds the real Ledger activity page, with its filters and
// rows, in a window of the given size.
func openLedgerActivity(t *testing.T, size image.Point) (*previewHarness, *historyPage) {
	t.Helper()
	service, _ := openBankFixture(t, createFixtureRoot(t))
	page := newLedgerHistory(t.Context(), service, func() time.Time { return accountTestClock }, func() {})
	h := newShellHarnessWithPages(t, ledgerPages{history: page})
	h.size = size
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	wait := func() {
		page.reads.workers.Wait()
		page.cashMovements.workers.Wait()
		page.model.Wait()
	}
	t.Cleanup(wait)
	h.frame()
	h.click("Ledger activity", false)
	for range 3 {
		h.frame()
		wait()
	}
	return h, page
}

// A short window scrolls the page instead of squeezing the activity list; a
// tall one leaves the list to fill the space.
func TestLedgerActivityScrollsOnlyWhenTheWindowIsShort(t *testing.T) {
	_, short := openLedgerActivity(t, image.Pt(700, 460))
	if short.pageScroll.Position.BeforeEnd == false {
		t.Error("700x460: the page should be taller than the window and scroll")
	}
	_, tall := openLedgerActivity(t, image.Pt(1600, 1000))
	if tall.pageScroll.Position.BeforeEnd {
		t.Error("1600x1000: the page should fit without scrolling")
	}
}

// TestVisualSnapshotsLedgerActivity renders the Ledger activity page at each
// window size. Opt in with NESTWORTH_GIO_SNAPSHOT_DIR.
func TestVisualSnapshotsLedgerActivity(t *testing.T) {
	dir := snapshotDir(t)
	for _, size := range responsiveSizes {
		h, _ := openLedgerActivity(t, size)
		h.snapshot(t, dir, fmt.Sprintf("responsive-ledger-activity-%dx%d", size.X, size.Y))
	}
}
