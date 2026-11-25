package charts

import (
	"image"
	"testing"
	"time"

	"gioui.org/f32"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// rig drives one chart through real Gio frames and input routing.
type rig struct {
	router input.Router
	ops    op.Ops
	now    time.Time
	width  int
	view   ui.View
}

func newRig(width int, dark bool) *rig {
	return &rig{now: time.Unix(1, 0), width: width,
		view: ui.View{Language: i18n.English, Colors: testPalette(dark), Theme: ui.NewTheme(), Motion: &ui.Motion{}}}
}

type layoutFunc func(gtx layout.Context, view ui.View, selected Selection) (layout.Dimensions, []Event)

// frame lays a chart out once and returns its size and the requests it made.
func (r *rig) frame(draw layoutFunc, selected Selection) (image.Point, []Event) {
	r.ops.Reset()
	r.now = r.now.Add(16 * time.Millisecond)
	gtx := layout.Context{
		Ops: &r.ops, Source: r.router.Source(), Now: r.now,
		Metric:      unit.Metric{PxPerDp: 1, PxPerSp: 1},
		Constraints: layout.Constraints{Max: image.Pt(r.width, 4000)},
	}
	dims, events := draw(gtx, r.view, selected)
	r.router.Frame(&r.ops)
	return dims.Size, events
}

func (r *rig) press(at image.Point) {
	r.router.Queue(pointer.Event{Kind: pointer.Press, Source: pointer.Mouse, Buttons: pointer.ButtonPrimary, Position: f32.Pt(float32(at.X), float32(at.Y))})
}

func (r *rig) key(name key.Name) {
	r.router.Queue(key.Event{Name: name, State: key.Press})
}

func (r *rig) focus(tag event.Tag) {
	r.router.Source().Execute(key.FocusCmd{Tag: tag})
}

// subject adapts one chart to the shared interaction tests.
type subject struct {
	name string
	// items are the stable IDs in the order a reader meets them.
	items []Selection
	tag   event.Tag
	// end is where End moves the cursor from the first item.
	end  Selection
	draw layoutFunc
	// scan lists pixels that sweep across the items in reading order, for a
	// chart of the given laid-out size.
	scan func(size image.Point) []image.Point
}

func downTheLeftColumn(size image.Point) []image.Point {
	var pts []image.Point
	for y := 0; y < size.Y; y += 3 {
		pts = append(pts, image.Pt(size.X/2, y))
	}
	return pts
}

// hits presses every scan point once, with nothing selected, and returns the
// distinct items they reach in the order met.
// registry lists the charts the shared interaction tests run against. Each
// component's test file registers its subject.
var registry []subject

// fitOnly lists extra configurations that only the layout-fit test uses.
var fitOnly []subject

func subjects() []subject { return registry }

func hits(r *rig, s subject) []Selection {
	size, _ := r.frame(s.draw, Selection{})
	r.frame(s.draw, Selection{})
	var seen []Selection
	for _, at := range s.scan(size) {
		r.press(at)
		_, events := r.frame(s.draw, Selection{})
		for _, e := range events {
			if e.Kind == EventSelect && indexOf(seen, e.Selection) < 0 {
				seen = append(seen, e.Selection)
			}
		}
	}
	return seen
}

func TestPointerReachesEveryItemInReadingOrder(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, s := range subjects() {
			got := hits(newRig(520, dark), s)
			// The scan may miss a tiny or missing item, but whatever it reaches
			// must be in caller order, without repeats; and a chart must reach
			// at least all items that have geometry.
			last := -1
			for _, sel := range got {
				i := indexOf(s.items, sel)
				if i < 0 {
					t.Fatalf("%s dark=%v: reached unknown item %v", s.name, dark, sel)
				}
				// Line points are met in time order, which interleaves series, and
				// a flow diagram is met column by column with its links between;
				// the plan tests cover that picking.
				if i < last && s.name != "line" && s.name != "sankey" {
					t.Fatalf("%s dark=%v: items reached out of caller order: %v", s.name, dark, got)
				}
				last = i
			}
			if len(got) < len(s.items)-1 {
				t.Fatalf("%s dark=%v: pointer reached %v of %d items", s.name, dark, got, len(s.items))
			}
		}
	}
}

func TestSecondPressOnSelectedItemClears(t *testing.T) {
	for _, s := range subjects() {
		r := newRig(520, false)
		reached := hits(r, s)
		if len(reached) == 0 {
			t.Fatalf("%s: nothing reachable", s.name)
		}
		size, _ := r.frame(s.draw, Selection{})
		// Find a point for reached[0], then press it again while selected.
		var at image.Point
		for _, p := range s.scan(size) {
			r.press(p)
			if _, ev := r.frame(s.draw, Selection{}); len(ev) > 0 && ev[0].Selection == reached[0] {
				at = p
				break
			}
		}
		r.frame(s.draw, reached[0])
		r.press(at)
		_, events := r.frame(s.draw, reached[0])
		if len(events) != 1 || events[0].Kind != EventClear || events[0].Selection != reached[0] {
			t.Fatalf("%s: second press produced %+v, want one clear", s.name, events)
		}
	}
}

func TestKeyboardNavigatesSelectsAndClears(t *testing.T) {
	for _, s := range subjects() {
		r := newRig(520, false)
		r.frame(s.draw, Selection{})
		// Without focus the chart ignores the keyboard.
		r.key(key.NameReturn)
		if _, ev := r.frame(s.draw, Selection{}); len(ev) != 0 {
			t.Fatalf("%s: unfocused chart reacted to Return: %+v", s.name, ev)
		}
		r.focus(s.tag)
		r.frame(s.draw, Selection{})
		r.frame(s.draw, Selection{})

		// First arrow press keeps the default cursor moving to the next item.
		r.key(key.NameRightArrow)
		r.frame(s.draw, Selection{})
		r.key(key.NameSpace)
		_, ev := r.frame(s.draw, Selection{})
		if len(ev) != 1 || ev[0].Kind != EventSelect || ev[0].Selection != s.items[1] {
			t.Fatalf("%s: Right then Space = %+v, want select of %v", s.name, ev, s.items[1])
		}

		r.key(key.NameEnd)
		r.frame(s.draw, Selection{})
		r.key(key.NameEnter)
		_, ev = r.frame(s.draw, Selection{})
		if len(ev) != 1 || ev[0].Selection != s.end {
			t.Fatalf("%s: End then Enter = %+v, want %v", s.name, ev, s.end)
		}

		// Escape clears only when something is selected.
		r.key(key.NameEscape)
		if _, ev = r.frame(s.draw, Selection{}); len(ev) != 0 {
			t.Fatalf("%s: Escape with no selection produced %+v", s.name, ev)
		}
		sel := s.items[1]
		r.key(key.NameEscape)
		if _, ev = r.frame(s.draw, sel); len(ev) != 1 || ev[0].Kind != EventClear {
			t.Fatalf("%s: Escape with a selection produced %+v, want clear", s.name, ev)
		}
	}
}

func TestSelectionIsControlled(t *testing.T) {
	// A chart only draws the selection its spec carries. When the owner does
	// not apply a request, the same press requests the same selection again
	// instead of the chart remembering it.
	check := func(s subject) {
		r := newRig(520, false)
		reached := hits(r, s)
		if len(reached) == 0 {
			t.Fatalf("%s: no item reachable", s.name)
		}
		size, _ := r.frame(s.draw, Selection{})
		for _, at := range s.scan(size) {
			r.press(at)
			if _, ev := r.frame(s.draw, Selection{}); len(ev) > 0 && ev[0].Selection == reached[0] {
				r.press(at)
				_, again := r.frame(s.draw, Selection{})
				if len(again) != 1 || again[0].Kind != EventSelect || again[0].Selection != reached[0] {
					t.Fatalf("%s: repeated press with an unapplied request produced %+v", s.name, again)
				}
				return
			}
		}
		t.Fatalf("%s: found no point to press", s.name)
	}
	for _, s := range subjects() {
		check(s)
	}
}

func TestChartsFitNarrowAndWideWindowsInBothThemes(t *testing.T) {
	for _, width := range []int{240, 320, 390, 560, 1100} {
		for _, dark := range []bool{false, true} {
			all := append(append([]subject(nil), registry...), fitOnly...)
			for _, s := range all {
				r := newRig(width, dark)
				for _, sel := range []Selection{{}, s.items[0], s.items[len(s.items)-1]} {
					size, _ := r.frame(s.draw, sel)
					if size.X > width || size.X <= 0 || size.Y <= 0 {
						t.Fatalf("%s width=%d dark=%v: size %v does not fit", s.name, width, dark, size)
					}
				}
			}
		}
	}
}
