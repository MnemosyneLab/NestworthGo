//go:build !darwin || ios

package accessibility

import (
	"gioui.org/app"
	"gioui.org/layout"
)

type Bridge struct{}

func New(*app.Window) *Bridge                           { return &Bridge{} }
func (*Bridge) View(app.ViewEvent)                      {}
func (*Bridge) Begin(gtx layout.Context) layout.Context { return gtx }
func (*Bridge) Publish(layout.Context)                  {}
func (*Bridge) Close()                                  {}
