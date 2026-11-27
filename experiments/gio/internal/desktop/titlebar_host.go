package desktop

import "image"

// AppKit readiness is separate from Gio's ownership of client decorations.
// FullSizeContentView makes Gio v0.10.3 report native Decorated=false even
// when the three native buttons exist. Setting the public Decorated(false)
// option suppresses Gio's fallback, but AppKit must then restore those
// buttons. Only a successfully attached host may request that handoff.
type titlebarHostStatus uint8

const (
	titlebarHostWaiting titlebarHostStatus = iota
	titlebarHostReady
	titlebarHostIntegrated
	titlebarHostUnavailable
)

type titlebarOwnershipAction uint8

const (
	titlebarOwnershipKeep titlebarOwnershipAction = iota
	titlebarOwnershipAcquire
	titlebarOwnershipRestore
)

func titlebarOwnership(wanted, owned, rejected bool, status titlebarHostStatus) titlebarOwnershipAction {
	if owned {
		if !wanted || rejected || status == titlebarHostUnavailable {
			return titlebarOwnershipRestore
		}
	} else if wanted && !rejected && status == titlebarHostReady {
		return titlebarOwnershipAcquire
	}
	return titlebarOwnershipKeep
}

// AppKit view dimensions are physical backing pixels, not dp. The native
// window must actually cover the complete Gio frame; a client decoration
// introduces an offset and invalidates both layout and native drag regions.
type titlebarHostProbe struct {
	viewSize                         image.Point
	fullSize, fullscreen, transition bool
	visibleButtons                   int
}

func (p titlebarHostProbe) matches(frame image.Point) bool {
	return p.fullSize && !p.transition && p.viewSize == frame && frame.X > 0 && frame.Y > 0 &&
		(p.fullscreen || p.visibleButtons == 3)
}
