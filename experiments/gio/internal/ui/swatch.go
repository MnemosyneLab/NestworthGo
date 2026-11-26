package ui

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
)

// SwatchSize is the box of one color swatch, including its selection ring.
const SwatchSize unit.Dp = 28

// SwatchSpec describes one color swatch in a single-choice group.
type SwatchSpec struct {
	Fill     color.NRGBA
	Name     string
	Selected bool
}

// Swatch is a round color option. The selected swatch wears a ring in its own
// color, a hovered one previews the ring, and keyboard focus shows it at
// strength. Pressing shrinks the dot slightly. It is focusable and reports a
// selected button to assistive technology.
func (v View) Swatch(gtx layout.Context, click *widget.Clickable, spec SwatchSpec) layout.Dimensions {
	v.Focus.Watch(gtx, click)
	return accessibility.SelectableButton(gtx, click, spec.Name, spec.Selected, func(gtx layout.Context) layout.Dimensions {
		return v.layoutSwatch(gtx, click, spec)
	})
}

func (v View) layoutSwatch(gtx layout.Context, click *widget.Clickable, spec SwatchSpec) layout.Dimensions {
	enabled := gtx.Enabled()
	state := v.Motion.For(click)
	state.Hover.Set(gtx, flag(enabled && click.Hovered()), DurationBase, EaseOut)
	state.Press.Set(gtx, flag(enabled && click.Pressed()), DurationFast, EaseOut)
	state.Select.Set(gtx, flag(spec.Selected), DurationBase, EaseOut)
	state.Focus.Set(gtx, flag(enabled && gtx.Focused(click)), DurationBase, EaseOut)
	hover, press := state.Hover.Value(gtx), state.Press.Value(gtx)
	selected, focus := state.Select.Value(gtx), state.Focus.Value(gtx)

	side := gtx.Dp(SwatchSize)
	size := image.Pt(side, side)
	gtx.Constraints = layout.Exact(size)
	return click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(spec.Selected).Add(gtx.Ops)
		semantic.DescriptionOp(spec.Name).Add(gtx.Ops)
		semantic.EnabledOp(enabled).Add(gtx.Ops)
		if enabled {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		area := clip.Rect{Max: size}.Push(gtx.Ops)
		defer area.Pop()

		ring := max(selected, focus*0.6, hover*0.35)
		if ring > 0 {
			StrokeRoundRect(gtx, image.Rectangle{Max: size}, RadiusFull, 2, WithAlpha(spec.Fill, ring))
		}
		// The dot sits 4dp inside the box, which leaves a 2dp gap to the ring,
		// and shrinks a little while pressed.
		inset := gtx.Dp(4) + int(float64(gtx.Dp(1))*press*1.5)
		FillRoundRect(gtx, image.Rectangle{Max: size}.Inset(inset), RadiusFull, Fade(spec.Fill, map[bool]float64{true: 1, false: DisabledOpacity}[enabled]))
		return layout.Dimensions{Size: size}
	})
}
