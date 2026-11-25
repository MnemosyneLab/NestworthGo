package ui

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

// SurfaceSpec describes a bordered, optionally elevated container. Zero values
// pick the card defaults: the Surface color, a Divider hairline, RadiusXL and
// no shadow.
type SurfaceSpec struct {
	Fill      color.NRGBA
	Border    color.NRGBA
	Radius    unit.Dp
	Elevation Elevation
}

// Surface lays content out inside a container drawn to the content's size.
// Its shadow, fill and border sit behind the content.
func (v View) Surface(gtx layout.Context, spec SurfaceSpec, content layout.Widget) layout.Dimensions {
	p := v.Colors
	if spec.Fill == (color.NRGBA{}) {
		spec.Fill = p.Surface
	}
	if spec.Border == (color.NRGBA{}) {
		spec.Border = p.Divider
	}
	if spec.Radius == 0 {
		spec.Radius = RadiusXL
	}
	return layout.Background{}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		p.DrawShadow(gtx, bounds, spec.Radius, spec.Elevation)
		FillRoundRect(gtx, bounds, spec.Radius, spec.Fill)
		StrokeRoundRect(gtx, bounds, spec.Radius, 1, spec.Border)
		return layout.Dimensions{Size: bounds.Size()}
	}, content)
}

// Card is the standard content container: card surface, hairline border,
// RadiusXL and the xs shadow, as Card does in Wails.
func (v View) Card(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return v.Surface(gtx, SurfaceSpec{Elevation: ElevationXS}, content)
}

// Floating is a layer above the page, such as a popover, menu or tooltip. It
// carries the md shadow so it separates from what is behind it.
func (v View) Floating(gtx layout.Context, content layout.Widget) layout.Dimensions {
	return v.Surface(gtx, SurfaceSpec{Elevation: ElevationMD}, content)
}

// Panel is a Card with a caller-chosen border color.
func (v View) Panel(gtx layout.Context, border color.NRGBA, content layout.Widget) layout.Dimensions {
	return v.Surface(gtx, SurfaceSpec{Border: border, Elevation: ElevationXS}, content)
}

// fieldChrome is the shared frame of text-like inputs and select triggers.
type fieldChrome struct {
	// Tag identifies the control in the Motion registry.
	Tag event.Tag
	// Active is true while the control has keyboard focus or its popup is open.
	Active, Invalid, Disabled bool
}

// fieldFrame draws the input frame around content: an xs shadow, the card
// fill, a hairline that turns accent while active, and the focus ring that
// sits flush against the border. The ring fades with the Motion registry.
func (v View) fieldFrame(gtx layout.Context, chrome fieldChrome, content layout.Widget) layout.Dimensions {
	p := v.Colors
	radius := RadiusLG
	state := v.Motion.For(chrome.Tag)
	state.Focus.Set(gtx, flag(chrome.Active && !chrome.Disabled), DurationBase, EaseOut)
	focus := state.Focus.Value(gtx)

	border := lerp(p.Divider, Mix(p.Surface, p.Accent, 0.5), focus)
	ring := p.Accent
	if chrome.Invalid {
		border, ring = p.Destructive, p.Destructive
	}
	opacity := 1.0
	if chrome.Disabled {
		opacity = DisabledOpacity
	}

	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()

	bounds := image.Rectangle{Max: dims.Size}
	if !chrome.Disabled {
		p.DrawShadow(gtx, bounds, radius, ElevationXS)
	}
	FillRoundRect(gtx, bounds, radius, Fade(p.Surface, opacity))
	StrokeRoundRect(gtx, bounds, radius, 1, Fade(border, opacity))
	call.Add(gtx.Ops)
	if focus > 0 || (chrome.Invalid && chrome.Active) {
		amount := focus
		if chrome.Invalid {
			amount = max(amount, flag(chrome.Active))
		}
		strokeRing(gtx, bounds, radius, ring, amount)
	}
	return dims
}

// strokeRing draws the 2dp ring flush outside bounds.
func strokeRing(gtx layout.Context, bounds image.Rectangle, radius unit.Dp, ink color.NRGBA, amount float64) {
	width := gtx.Dp(2)
	StrokeRoundRect(gtx, bounds.Inset(-width), radius+2, 2, WithAlpha(ink, 0.5*amount))
}
