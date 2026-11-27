//go:build !darwin || ios || !cgo

package desktop

import (
	"gioui.org/app"
	"gioui.org/unit"
	"image"
)

// Keep ordinary Gio platform chrome (and the existing sidebar control).
type nativeTitlebar struct{}

func newNativeTitlebar(*app.Window) *nativeTitlebar                                      { return &nativeTitlebar{} }
func (*nativeTitlebar) View(app.ViewEvent)                                               {}
func (*nativeTitlebar) Close()                                                           {}
func (*nativeTitlebar) Config()                                                          {}
func (*nativeTitlebar) Metrics(image.Point) titlebarMetrics                              { return titlebarMetrics{} }
func (*nativeTitlebar) Publish(bool, bool, image.Point, unit.Metric, [2]image.Rectangle) {}

func (*nativeTitlebar) AfterFrame()          {}
func (*nativeTitlebar) FrameSizeValid() bool { return true }
