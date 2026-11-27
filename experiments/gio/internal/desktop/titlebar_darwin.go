//go:build darwin && !ios && cgo

package desktop

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework AppKit -framework QuartzCore
#include "titlebar_darwin.h"
*/
import "C"

import (
	"image"
	"log/slog"
	"sync"
	"sync/atomic"

	"gioui.org/app"
	"gioui.org/unit"
)

var titlebarHandles sync.Map
var nextTitlebarHandle atomic.Uint64

type nativeTitlebar struct {
	window                            *app.Window
	view, token                       uintptr
	attachment                        *titlebarAttachment
	want, last                        titlebarUpdate
	owned, rejected, verified, logged bool
	verifiedSize                      image.Point
	mismatches                        int
}

// Attachments have separate lifetimes so a late callback for an old NSView
// cannot publish stale geometry to a newly mounted view.
type titlebarAttachment struct {
	window  *app.Window
	current atomic.Pointer[titlebarHostState]
}

type titlebarHostState struct {
	status  titlebarHostStatus
	metrics titlebarMetrics
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
	// A replacement NSView must begin with normal native decorations.
	// Never leave Gio's decoration request disabled after an old attachment.
	if n.owned {
		n.window.Option(app.Decorated(true))
	}
	n.view = view.View
	n.token = uintptr(nextTitlebarHandle.Add(1))
	n.want, n.last = titlebarUpdate{}, titlebarUpdate{}
	n.owned, n.rejected, n.verified, n.logged = false, false, false, false
	n.verifiedSize, n.mismatches = image.Point{}, 0
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
	// On DestroyEvent Gio no longer has a driver; do not call Option or
	// Window.Run here. View restores ownership if there is a new attachment.
}

func (n *nativeTitlebar) host() titlebarHostState {
	if n.attachment != nil {
		if s := n.attachment.current.Load(); s != nil {
			return *s
		}
	}
	return titlebarHostState{}
}

// Config is intentionally asynchronous: Gio delivers events while AppKit
// may be waiting for the application to finish processing the event.
func (n *nativeTitlebar) Config() {
	if n.token != 0 {
		n.verified = false
		C.nestworthTitlebarRefresh(C.uintptr_t(n.token))
	}
}

func (n *nativeTitlebar) Metrics(size image.Point) titlebarMetrics {
	state := n.host()
	if !n.owned || n.rejected || state.status != titlebarHostIntegrated {
		return titlebarMetrics{}
	}
	if !n.verified || n.verifiedSize != size {
		// Inspect live AppKit geometry on Gio's native event thread, not a
		// cached rectangle or an inferred decoration height. Window.Run is
		// supported by the public API and does not dispatch_sync to AppKit.
		var native C.NestworthTitlebarProbe
		n.window.Run(func() {
			native = C.nestworthTitlebarProbe(C.uintptr_t(n.token))
		})
		p := titlebarHostProbe{
			viewSize: image.Pt(int(native.widthPx), int(native.heightPx)),
			fullSize: native.fullSize != 0, fullscreen: native.fullscreen != 0,
			transition: native.transition != 0, visibleButtons: int(native.visibleButtons),
		}
		if p.transition {
			n.verified = false
			return state.metrics
		}
		if !p.matches(size) {
			n.verified = false
			n.mismatches++
			// A configure/resize can generate one old-size frame before the
			// native reconciliation completes. Do not install drag regions
			// for it. Reject a persistent mismatch and restore system chrome.
			if n.mismatches >= 2 {
				n.rejected = true
				slog.Warn("Gio titlebar native handshake rejected; restoring system decorations",
					"gio_frame_px", size, "appkit_view_px", p.viewSize,
					"native_buttons", p.visibleButtons, "native_full_size", p.fullSize)
				return titlebarMetrics{}
			}
			n.window.Invalidate()
			return state.metrics
		}
		n.verified, n.verifiedSize, n.mismatches = true, size, 0
		if !n.logged && !p.fullscreen {
			n.logged = true
			slog.Info("Gio titlebar native handshake verified",
				"gio_frame_px", size, "appkit_view_px", p.viewSize,
				"outer_frame_pt", image.Pt(int(native.outerWidth+.5), int(native.outerHeight+.5)),
				"native_buttons", p.visibleButtons, "native_full_size", p.fullSize,
				"gio_decorated_requested", false)
		}
	}
	return state.metrics
}

// Do not persist a transient frame during the ownership/geometry handoff.
func (n *nativeTitlebar) FrameSizeValid() bool {
	return !n.owned || (n.verified && !n.rejected && n.host().status == titlebarHostIntegrated)
}

func (n *nativeTitlebar) Publish(enabled, dark bool, size image.Point, metric unit.Metric, drag [2]image.Rectangle) {
	n.want = titlebarUpdate{enabled: enabled, dark: dark, size: size, metric: metric, drag: drag}
	if !n.owned || n.token == 0 {
		return
	}
	u := n.want
	if !n.verified || n.rejected || n.host().status != titlebarHostIntegrated {
		u.drag = [2]image.Rectangle{}
	}
	if u == n.last {
		return
	}
	n.last = u
	dp := func(v int) C.double { return C.double(metric.PxToDp(v)) }
	a, b := u.drag[0], u.drag[1]
	C.nestworthTitlebarUpdate(C.uintptr_t(n.token), cBool(u.dark),
		dp(size.X), dp(size.Y), dp(a.Min.X), dp(a.Min.Y), dp(a.Dx()), dp(a.Dy()),
		dp(b.Min.X), dp(b.Min.Y), dp(b.Dx()), dp(b.Dy()))
}

// Change decoration ownership only after submitting the frame built under
// the previous mode. Option(false) suppresses Gio's fallback material
// titlebar AND hides the native buttons. SetMode immediately reconciles
// the real AppKit controls using public APIs after Configure has run.
func (n *nativeTitlebar) AfterFrame() {
	if n.token == 0 {
		return
	}
	u := n.want
	dp := func(v int) C.double { return C.double(u.metric.PxToDp(v)) }
	switch titlebarOwnership(u.enabled, n.owned, n.rejected, n.host().status) {
	case titlebarOwnershipAcquire:
		n.window.Option(app.Decorated(false))
		n.owned, n.verified = true, false
		n.last = titlebarUpdate{}
		n.window.Run(func() {
			C.nestworthTitlebarSetMode(C.uintptr_t(n.token), 1, cBool(u.dark), dp(u.size.X), dp(u.size.Y))
		})
		// If AppKit rejected the install, immediately roll back instead
		// of waiting on a future frame with hidden native buttons.
		if n.host().status == titlebarHostUnavailable {
			n.restoreOwnership()
		}
		n.window.Invalidate()
	case titlebarOwnershipRestore:
		n.restoreOwnership()
	}
}

func (n *nativeTitlebar) restoreOwnership() {
	n.rejected = n.rejected || n.host().status == titlebarHostUnavailable
	if n.rejected {
		slog.Warn("Gio integrated titlebar unavailable; keeping native system decorations")
	}
	// Stop the native observer's integration before Gio configures
	// ordinary chrome; otherwise the two layers would fight again.
	n.window.Run(func() {
		C.nestworthTitlebarSetMode(C.uintptr_t(n.token), 0, cBool(n.want.dark), 0, 0)
	})
	n.window.Option(app.Decorated(true))
	n.owned, n.verified = false, false
	n.mismatches = 0
	n.last = titlebarUpdate{}
	n.window.Invalidate()
}

func cBool(v bool) C.int {
	if v {
		return 1
	}
	return 0
}

//export nestworthTitlebarChanged
func nestworthTitlebarChanged(token C.uintptr_t, status C.int, right, bottom, center, top, left, safeRight C.double) {
	value, ok := titlebarHandles.Load(uintptr(token))
	if !ok {
		return
	}
	n := value.(*titlebarAttachment)
	m := titlebarHostState{status: titlebarHostStatus(status), metrics: titlebarMetrics{
		active: status == C.NestworthTitlebarIntegrated, buttonRight: unit.Dp(right), buttonBottom: unit.Dp(bottom), buttonCenter: unit.Dp(center),
		safeTop: unit.Dp(top), safeLeft: unit.Dp(left), safeRight: unit.Dp(safeRight)}}
	if old := n.current.Load(); old == nil || *old != m {
		n.current.Store(&m)
		// Do not block AppKit on Gio's event loop.
		go n.window.Invalidate()
	}
}
