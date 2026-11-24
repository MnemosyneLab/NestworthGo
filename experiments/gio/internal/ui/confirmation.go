package ui

import (
	"image/color"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// Confirmation draws a window-level dialog over disabled page content.
// The caller owns the operation, the open state, and focus after dismissal.
type Confirmation struct {
	Cancel, Confirm widget.Clickable
	focusOnOpen     bool
}

type ConfirmationOptions struct {
	Title, Body, Cancel, Confirm, Problem string
	Busy                                  bool
}

func (c *Confirmation) Open() { c.focusOnOpen = true }

func (c *Confirmation) Layout(gtx layout.Context, view View, options ConfirmationOptions, cancel, confirm func()) {
	if c.focusOnOpen {
		c.focusOnOpen = false
		gtx.Execute(key.FocusCmd{Tag: &c.Cancel})
	}
	for _, button := range []*widget.Clickable{&c.Cancel, &c.Confirm} {
		for _, e := range PressedKeys(gtx, button, key.NameEscape, key.NameTab) {
			if e.Name == key.NameEscape && !options.Busy {
				cancel()
				return
			}
			if e.Name == key.NameTab {
				target := &c.Cancel
				if button == &c.Cancel {
					target = &c.Confirm
				}
				gtx.Execute(key.FocusCmd{Tag: target})
			}
		}
	}
	if !options.Busy && c.Cancel.Clicked(gtx) {
		cancel()
		return
	}
	if !options.Busy && c.Confirm.Clicked(gtx) {
		confirm()
	}
	paint.Fill(gtx.Ops, color.NRGBA{A: 0x68})
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx = LimitWidth(gtx, 420)
		return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(22).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if options.Busy {
					gtx = gtx.Disabled()
				}
				return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Label(gtx, options.Title, 17, font.SemiBold, view.Colors.Ink)
					}),
					layout.Rigid(view.Message(options.Body, view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if options.Problem == "" {
							return layout.Dimensions{}
						}
						return view.Message(options.Problem, view.Colors.ErrorInk)(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Gap: gtx.Dp(12)}.Layout(gtx,
							layout.Rigid(view.Button(&c.Cancel, options.Cancel)),
							layout.Rigid(view.PrimaryButton(&c.Confirm, options.Confirm)),
						)
					}),
				)
			})
		})
	})
}
