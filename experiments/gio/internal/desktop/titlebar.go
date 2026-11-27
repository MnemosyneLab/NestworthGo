package desktop

import (
	"image"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

const (
	titlebarCompactPageWidth unit.Dp = 500
	titlebarWideDragPad      unit.Dp = 44
	titlebarCompactDragPad   unit.Dp = 24
	titlebarRightPadding     unit.Dp = 16
)

// titlebarMetrics is the public-AppKit geometry in view points (Gio dp), not
// screen pixels. A zero value retains the ordinary platform decorations.
type titlebarMetrics struct {
	active                                                                bool
	buttonRight, buttonBottom, buttonCenter, safeTop, safeLeft, safeRight unit.Dp
}

// titlebarLayout is computed once for the whole shell. The collapsed rail
// stays narrow below the toolbar; only its top background extends across
// the native buttons and toggle when necessary.
type titlebarLayout struct {
	height, top, center, sidebar, split, rightInset int
	toggle                                          image.Rectangle
	drag                                            [2]image.Rectangle
}

func calculateTitlebar(gtx layout.Context, metrics titlebarMetrics, sidebar unit.Dp) titlebarLayout {
	g := titlebarLayout{height: gtx.Dp(headerHeight), sidebar: gtx.Dp(sidebar)}
	g.split = g.sidebar
	if !metrics.active {
		return g
	}
	g.top = gtx.Dp(metrics.safeTop)
	g.height = max(g.top+gtx.Dp(headerHeight), gtx.Dp(metrics.buttonBottom)+gtx.Dp(8))
	side := gtx.Dp(30)
	x := max(gtx.Dp(metrics.safeLeft)+gtx.Dp(12), gtx.Dp(metrics.buttonRight)+gtx.Dp(12))
	// Never split the actual control across independently laid out columns.
	if x < g.sidebar && x+side+gtx.Dp(8) > g.sidebar {
		x = g.sidebar + gtx.Dp(8)
	}
	g.center = g.top + (g.height-g.top)/2
	if metrics.buttonCenter > 0 {
		halfControl := (gtx.Dp(ui.ControlSmall) + 1) / 2
		g.center = min(g.height-halfControl, max(g.top+halfControl, gtx.Dp(metrics.buttonCenter)))
	}
	y := g.center - side/2
	g.toggle = image.Rect(x, y, x+side, y+side)
	g.split = max(g.sidebar, g.toggle.Max.X+gtx.Dp(8))
	// Only explicitly empty parts of the toolbar are native drag targets.
	// Do not include the heading (keyboard focus), nav buttons or a popover.
	g.drag[0] = image.Rect(g.toggle.Max.X+gtx.Dp(4), g.top, max(g.toggle.Max.X+gtx.Dp(4), g.split-1), g.height-1)
	right := gtx.Constraints.Max.X - gtx.Dp(metrics.safeRight)
	dragWidth := gtx.Dp(titlebarWideDragPad)
	if right-g.split < gtx.Dp(titlebarCompactPageWidth) && g.drag[0].Dx() >= gtx.Dp(titlebarWideDragPad) {
		// On the narrow expanded shell the left segment is already a large
		// drag target. Keep more space for the route name and record action.
		dragWidth = gtx.Dp(titlebarCompactDragPad)
	}
	// Include the existing 16dp toolbar padding in the reserved drag pad.
	g.rightInset = gtx.Dp(metrics.safeRight) + dragWidth - gtx.Dp(titlebarRightPadding)
	g.drag[1] = image.Rect(right-dragWidth, g.top, right, g.height-1)
	return g
}

func (s *shell) titlebarToggle(gtx layout.Context, view ui.View, origin image.Point) layout.Dimensions {
	bounds := s.chrome.toggle.Sub(origin)
	offset := op.Offset(bounds.Min).Push(gtx.Ops)
	gtx.Constraints = layout.Exact(bounds.Size())
	icon := collapseIcon
	if s.collapsed() {
		icon = expandRailIcon
	}
	sidebarButton(gtx, view, &s.railButton, view.Language.Text(i18n.SidebarToggle), "", false,
		sidebarIcon(icon, view.Colors.SecondaryInk, 18))
	offset.Pop()
	return layout.Dimensions{Size: image.Pt(s.chrome.sidebar, s.chrome.height)}
}

func (s *shell) sidebarTitlebar(gtx layout.Context, view ui.View) layout.Dimensions {
	if !s.titlebar.active {
		return layout.Dimensions{}
	}
	if s.chrome.toggle.Max.X <= s.chrome.sidebar {
		s.titlebarToggle(gtx, view, image.Point{})
	}
	return layout.Dimensions{Size: image.Pt(s.chrome.sidebar, s.chrome.height)}
}

// layoutHeaderChrome draws the extended sidebar segment, if any, then
// translates only the page toolbar. Body geometry and overlay anchors use
// absolute window coordinates.
func (s *shell) layoutHeaderChrome(gtx layout.Context, view ui.View, body layout.Widget) layout.Dimensions {
	if !s.titlebar.active {
		return body(gtx)
	}
	size := gtx.Constraints.Max
	prefix := s.chrome.split - s.chrome.sidebar
	if prefix > 0 {
		paint.FillShape(gtx.Ops, view.Colors.SidebarBackground, clip.Rect(image.Rect(0, 0, prefix, size.Y)).Op())
		paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(prefix-1, 0, prefix, size.Y)).Op())
		paint.FillShape(gtx.Ops, view.Colors.Divider, clip.Rect(image.Rect(0, size.Y-1, prefix, size.Y)).Op())
		s.titlebarToggle(gtx, view, image.Pt(s.chrome.sidebar, 0))
	}
	// Center the toolbar row on the AppKit controls.
	top := max(s.chrome.top, 2*s.chrome.center-size.Y)
	height := min(size.Y-top, 2*(s.chrome.center-top))
	offset := op.Offset(image.Pt(prefix, top)).Push(gtx.Ops)
	gtx.Constraints = layout.Exact(image.Pt(max(0, size.X-prefix-s.chrome.rightInset), max(0, height)))
	body(gtx)
	offset.Pop()
	return layout.Dimensions{Size: size}
}

// Native drag zones correspond to the rendered frame. Disable them for
// overlays/sheets, onboarding and before geometry has been established.
func (s *shell) titlebarDrags() [2]image.Rectangle {
	if !s.titlebar.active || s.onboarding != nil || s.overlay.Trigger != nil {
		return [2]image.Rectangle{}
	}
	return s.chrome.drag
}
