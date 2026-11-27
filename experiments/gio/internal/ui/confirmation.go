package ui

import (
	"image/color"

	"gioui.org/f32"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/widget"
)

// Confirmation draws a window-level dialog over disabled page content.
// The caller owns the operation, the open state, and focus after dismissal.
type Confirmation struct {
	Cancel, Confirm widget.Clickable
	focusOnOpen     bool
	enter           Tween
}

type ConfirmationOptions struct {
	Title, Body, Cancel, Confirm, Problem string
	Busy, Destructive                     bool
}

func (c *Confirmation) Open() {
	c.focusOnOpen = true
	c.enter.Snap(0)
}

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
	c.enter.Set(gtx, 1, DurationBase, EaseOut)
	progress := c.enter.Value(gtx)
	paint.Fill(gtx.Ops, color.NRGBA{A: uint8(0x68 * progress)})
	layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx = LimitWidth(gtx, 420)
		// The dialog scales up from 96% while it fades in.
		macro := op.Record(gtx.Ops)
		dims := view.Surface(gtx, SurfaceSpec{Elevation: ElevationLG}, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(Space24).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if options.Busy {
					gtx = gtx.Disabled()
				}
				// Title and body read as one block; the actions sit apart at
				// the trailing edge, as in the Wails alert dialog.
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Line(gtx, TextHeading, options.Title, view.Colors.Ink)
					}),
					layout.Rigid(layout.Spacer{Height: Space8}.Layout),
					layout.Rigid(view.Body(TextBody, options.Body, view.Colors.SecondaryInk)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if options.Problem == "" {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: Space12}.Layout(gtx, view.Message(options.Problem, view.Colors.ErrorInk))
					}),
					layout.Rigid(layout.Spacer{Height: Space24}.Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						return layout.Flex{Spacing: layout.SpaceStart, Gap: gtx.Dp(Space8)}.Layout(gtx,
							layout.Rigid(view.Button(&c.Cancel, options.Cancel)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								variant := ButtonPrimary
								if options.Destructive {
									variant = ButtonDestructive
								}
								return view.ButtonOf(&c.Confirm, ButtonSpec{Variant: variant, Title: options.Confirm})(gtx)
							}),
						)
					}),
				)
			})
		})
		call := macro.Stop()
		scale := float32(0.96 + 0.04*progress)
		if ReducedMotion() {
			scale = 1
		}
		center := f32.Pt(float32(dims.Size.X)/2, float32(dims.Size.Y)/2)
		defer op.Affine(f32.Affine2D{}.Scale(center, f32.Pt(scale, scale))).Push(gtx.Ops).Pop()
		defer paint.PushOpacity(gtx.Ops, float32(progress)).Pop()
		call.Add(gtx.Ops)
		return dims
	})
}
