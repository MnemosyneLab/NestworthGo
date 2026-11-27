//go:build darwin && !ios && cgo

package desktop

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework QuartzCore
#include <stdint.h>
void nestworthTitlebarAttach(uintptr_t token, uintptr_t view);
void nestworthTitlebarDetach(uintptr_t token);
void nestworthTitlebarRefresh(uintptr_t token);
void nestworthTitlebarUpdate(uintptr_t token, int enabled, int dark,
	double width, double height,
	double x1, double y1, double w1, double h1,
	double x2, double y2, double w2, double h2);
*/
import "C"

import (
	"image"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/unit"
)

var titlebarHandles sync.Map
var nextTitlebarHandle atomic.Uint64

type nativeTitlebar struct {
	window      *app.Window
	view, token uintptr
	attachment  *titlebarAttachment
	last        titlebarUpdate
}

// Attachments have separate lifetimes so a late callback for an old NSView
// cannot publish stale geometry to a newly mounted view.
type titlebarAttachment struct {
	window  *app.Window
	current atomic.Pointer[titlebarMetrics]
}

type titlebarUpdate struct {
	enabled, dark bool
	size          image.Point
	metric        unit.Metric
	drag          [2]image.Rectangle
}

func newNativeTitlebar(window *app.Window) *nativeTitlebar {
	return &nativeTitlebar{window: window}
}

func (n *nativeTitlebar) View(event app.ViewEvent) {
	view, ok := event.(app.AppKitViewEvent)
	if !ok || view.View == n.view {
		return
	}
	n.Close()
	if view.View == 0 {
		return
	}
	n.view = view.View
	n.token = uintptr(nextTitlebarHandle.Add(1))
	n.last = titlebarUpdate{}
	n.attachment = &titlebarAttachment{window: n.window}
	titlebarHandles.Store(n.token, n.attachment)
	C.nestworthTitlebarAttach(C.uintptr_t(n.token), C.uintptr_t(n.view))
}

func (n *nativeTitlebar) Close() {
	if n.token == 0 {
		return
	}
	titlebarHandles.Delete(n.token)
	C.nestworthTitlebarDetach(C.uintptr_t(n.token))
	n.token, n.view = 0, 0
	n.attachment = nil
}

// Config is intentionally asynchronous: Gio delivers events while the AppKit
// thread may be waiting for the application to finish processing the event.
func (n *nativeTitlebar) Config() {
	if n.token != 0 {
		C.nestworthTitlebarRefresh(C.uintptr_t(n.token))
	}
}

func (n *nativeTitlebar) Metrics() titlebarMetrics {
	if n.attachment != nil {
		if m := n.attachment.current.Load(); m != nil {
			return *m
		}
	}
	return titlebarMetrics{}
}

func (n *nativeTitlebar) Publish(enabled, dark bool, size image.Point, metric unit.Metric, drag [2]image.Rectangle) {
	u := titlebarUpdate{enabled: enabled, dark: dark, size: size, metric: metric, drag: drag}
	if n.token == 0 || u == n.last {
		return
	}
	n.last = u
	dp := func(v int) C.double { return C.double(metric.PxToDp(v)) }
	a, b := drag[0], drag[1]
	C.nestworthTitlebarUpdate(C.uintptr_t(n.token), cBool(enabled), cBool(dark),
		dp(size.X), dp(size.Y), dp(a.Min.X), dp(a.Min.Y), dp(a.Dx()), dp(a.Dy()),
		dp(b.Min.X), dp(b.Min.Y), dp(b.Dx()), dp(b.Dy()))
}

func cBool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

//export nestworthTitlebarChanged
func nestworthTitlebarChanged(token C.uintptr_t, active C.int, right, bottom, top, left, safeRight C.double) {
	value, ok := titlebarHandles.Load(uintptr(token))
	if !ok {
		return
	}
	n := value.(*titlebarAttachment)
	m := titlebarMetrics{active: active != 0, buttonRight: unit.Dp(right), buttonBottom: unit.Dp(bottom),
		safeTop: unit.Dp(top), safeLeft: unit.Dp(left), safeRight: unit.Dp(safeRight)}
	if old := n.current.Load(); old == nil || *old != m {
		n.current.Store(&m)
		// Do not block AppKit on Gio's event loop.
		go n.window.Invalidate()
	}
}
