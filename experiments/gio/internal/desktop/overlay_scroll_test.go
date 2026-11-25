package desktop

import (
	"image"
	"testing"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/settings"
)

func TestShellMenusStayAnchoredAfterPageScroll(t *testing.T) {
	for _, name := range []string{"Language", "Recent"} {
		t.Run(name, func(t *testing.T) {
			h, _ := newOverlayScrollHarness(t)
			s := h.shell
			trigger := &s.languagePicker.Trigger
			if name == "Recent" {
				trigger = &s.recentPicker.Trigger
			}

			button := semanticBounds(t, &h.router, name, false)
			h.click(name, false)
			anchor, panel := s.overlay.Anchor, s.overlay.Panel
			if s.overlay.Trigger != trigger || panel.Empty() || anchor != button {
				t.Fatalf("%s did not open at its button: anchor %v, button %v, panel %v", name, anchor, button, panel)
			}
			h.press(key.NameEscape)

			scrollOverlayTestPage(h, 120)
			if s.stage.Position.Offset <= 0 {
				t.Fatal("the outer page did not scroll")
			}
			h.click(name, false)
			h.frame()
			if s.overlay.Trigger != trigger || s.overlay.Anchor != anchor || s.overlay.Panel != panel {
				t.Fatalf("page scroll displaced or closed %s: anchor %v, panel %v; want %v and %v", name, s.overlay.Anchor, s.overlay.Panel, anchor, panel)
			}
			h.press(key.NameEscape)
			h.focused(trigger)
		})
	}
}

func TestPageMenuFollowsTriggerUntilItScrollsOut(t *testing.T) {
	h, picker := newOverlayScrollHarness(t)
	host := &h.shell.overlay
	name := i18n.English.Text(i18n.PreviewAccountType)
	h.click(name, true)
	initial, panel := host.Anchor, host.Panel
	if host.Trigger != &picker.Trigger || panel.Empty() || initial != semanticBounds(t, &h.router, name, true) {
		t.Fatalf("page menu did not open at its trigger: anchor %v, panel %v", initial, panel)
	}

	scrollOverlayTestPage(h, 50)
	button := semanticBounds(t, &h.router, name, true)
	if button.Min.Y >= initial.Min.Y || host.Trigger != &picker.Trigger || host.Anchor != button || host.Panel != panel.Add(button.Min.Sub(initial.Min)) {
		t.Fatalf("page menu did not follow the visible trigger: anchor %v, trigger %v, original %v, panel %v", host.Anchor, button, initial, host.Panel)
	}

	scrollOverlayTestPage(h, 120)
	pageTop := h.metric().Dp(headerHeight)
	if host.Trigger != &picker.Trigger || host.Anchor.Min.Y >= pageTop || host.Anchor.Max.Y <= pageTop {
		t.Fatalf("page menu should stay open while its trigger is partially visible: %v", host.Anchor)
	}
	scrollOverlayTestPage(h, 40)
	if initial.Max.Y-h.shell.stage.Position.Offset > pageTop {
		t.Fatal("fixture did not scroll the entire trigger above the page viewport")
	}
	if host.Trigger != nil || !host.Panel.Empty() {
		t.Fatalf("page menu stayed open after the trigger left the viewport: %v", host.Panel)
	}
}

func newOverlayScrollHarness(t *testing.T) (*previewHarness, *ui.Select) {
	t.Helper()
	h := newShellHarness(t)
	h.size = image.Pt(960, 460)
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	picker := new(ui.Select)
	h.shell.routes[historyRoute].body = func(gtx layout.Context, view ui.View) layout.Dimensions {
		ui.Place(gtx, view, image.Pt(0, gtx.Dp(100)), gtx.Dp(230), func(gtx layout.Context, view ui.View) layout.Dimensions {
			picker.Layout(gtx, view, ui.PickerField{Label: i18n.PreviewAccountType, Origin: view.Origin},
				ui.SelectOptions{Items: []ui.Choice{{ID: "sample", Label: "Sample account type"}}}, "sample", func(string) {})
			return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(24+ui.ControlDefault))}
		})
		return layout.Dimensions{Size: image.Pt(gtx.Constraints.Max.X, gtx.Dp(800))}
	}
	h.frame()
	h.click("Ledger activity", false)
	return h, picker
}

func scrollOverlayTestPage(h *previewHarness, distance float32) {
	h.router.Queue(pointer.Event{Kind: pointer.Scroll, Source: pointer.Mouse,
		Position: f32.Pt(float32(h.size.X-25), float32(h.size.Y/2)), Scroll: f32.Pt(0, distance)})
	h.frame()
}
