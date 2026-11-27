package accessibility_test

import (
	"image"
	"testing"
	"time"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

func TestTabsPublishSelectionThroughGioAndNativeBridge(t *testing.T) {
	for _, fixture := range []struct {
		name string
		make func(ui.View, *widget.Clickable, string, bool) layout.Widget
	}{{"tab button", ui.View.TabButton}, {"tab segment", ui.View.TabSegment}} {
		t.Run(fixture.name, func(t *testing.T) {
			var bridge accessibility.TestFrame
			var router input.Router
			var ops op.Ops
			var tabs [2]widget.Clickable
			names := [2]string{"Activity", "Positions"}
			palette := ui.Palette{}.WithSemantics(false)
			view := ui.View{Colors: &palette, Theme: ui.NewTheme(), Motion: &ui.Motion{}}
			selected := 0
			render := func(id uint64, action accessibility.TestAction) []accessibility.TestElement {
				ops.Reset()
				gtx := bridge.Begin(layout.Context{Source: router.Source(), Now: time.Unix(1, 0), Ops: &ops,
					Constraints: layout.Constraints{Max: image.Pt(400, 100)}}, id, action)
				for i := range tabs {
					if tabs[i].Clicked(gtx) {
						selected = i
					}
				}
				layout.Flex{Gap: 6}.Layout(gtx,
					layout.Rigid(fixture.make(view, &tabs[0], names[0], selected == 0)),
					layout.Rigid(fixture.make(view, &tabs[1], names[1], selected == 1)),
				)
				view.Motion.EndFrame()
				router.Frame(gtx.Ops)
				return bridge.Snapshot(gtx)
			}
			assertSelected := func(elements []accessibility.TestElement, index int) {
				t.Helper()
				if len(elements) != 2 {
					t.Fatalf("expected two native controls, got %+v", elements)
				}
				for i, element := range elements {
					if element.Name != names[i] || !element.Pressable || element.Selected != (i == index) {
						t.Fatalf("native button %d = %+v, selected index %d", i, element, index)
					}
				}
				seen := 0
				for _, node := range router.AppendSemantics(nil) {
					if node.Desc.Class == semantic.Button && node.Desc.Selected {
						seen++
						if node.Desc.Label != names[index] {
							t.Fatalf("selected Gio label = %q, want %q", node.Desc.Label, names[index])
						}
					}
				}
				if seen != 1 {
					t.Fatalf("Gio reported %d selected tabs", seen)
				}
			}
			first := render(0, 0)
			assertSelected(first, 0)
			assertSelected(render(first[1].ID, accessibility.TestPress), 1)
			render(first[0].ID, accessibility.TestFocus)
			router.Queue(key.Event{Name: key.NameReturn, State: key.Press}, key.Event{Name: key.NameReturn, State: key.Release})
			last := render(0, 0)
			assertSelected(last, 0)
			if last[0].ID != first[0].ID || last[1].ID != first[1].ID {
				t.Fatalf("switching the selection changed the controls' native identity: %+v", last)
			}
		})
	}
}
