package ui

import (
	"gioui.org/io/event"
	"gioui.org/layout"
)

// PageFocus observes the controls drawn by the active page for one frame.
// It lets navigation restore a control only when that control is available.
type PageFocus struct {
	Target, Current event.Tag
	TargetDrawn     bool
}

func (f *PageFocus) Watch(gtx layout.Context, tag event.Tag) {
	if f == nil || !gtx.Enabled() || gtx.Constraints.Max.X <= 0 || gtx.Constraints.Max.Y <= 0 {
		return
	}
	if tag == f.Target {
		f.TargetDrawn = true
	}
	if gtx.Focused(tag) {
		f.Current = tag
	}
}
