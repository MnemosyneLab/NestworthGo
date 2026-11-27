package ui

import (
	"image"
	"image/color"

	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
)

// ButtonVariant mirrors the Wails Button variants in
// frontend/src/components/ui/button.tsx.
type ButtonVariant uint8

const (
	// ButtonPrimary is the filled accent action. Use one per view.
	ButtonPrimary ButtonVariant = iota
	// ButtonSoft is a quiet tinted accent action.
	ButtonSoft
	// ButtonDestructive confirms an irreversible action.
	ButtonDestructive
	// ButtonOutline is the neutral action on a card surface.
	ButtonOutline
	// ButtonGhost has no chrome until hovered, for toolbars and row actions.
	ButtonGhost
	// ButtonLink reads as text with an underline on hover.
	ButtonLink
	// ButtonSegment is one option of a TabTrack: quiet until selected, then a
	// raised card, as the Wails pill tabs.
	ButtonSegment
)

// ButtonSize selects one of the shared control heights.
type ButtonSize uint8

const (
	ButtonDefault ButtonSize = iota // 36dp
	ButtonSmall                     // 32dp
	ButtonLarge                     // 40dp
	// ButtonCompact is 28dp, for icon buttons inside a field or a dense row.
	ButtonCompact
)

// ButtonSpec describes one button. Title is the visible text and may be empty
// for an icon-only button, which is then square and needs a Name.
type ButtonSpec struct {
	Variant ButtonVariant
	Size    ButtonSize
	Title   string
	Name    string // accessible name; defaults to Title
	// ShortName labels the icon of an icon-only button whose Name carries
	// context, such as "Back to Accounts". Assistive technology and tests can
	// then find the control by its plain action word.
	ShortName   string
	Description string
	// Toggle marks a button that reports Selected as its state, as a tab does.
	Toggle, Selected bool
	// Radio opts into a radio role. The caller manages the group and selection.
	Radio bool
	Icon  Glyph
	// Fill stretches the button across the available width. Text and custom
	// actions otherwise size to their content, even inside a full-width card.
	Fill bool
	// Content replaces the title with a custom layout, as in a clickable row.
	// The button grows to fit it and Name carries the accessible label.
	Content layout.Widget
}

type buttonMetrics struct {
	height, padding, radius unit.Dp
	text                    TextStyle
}

func (s ButtonSize) metrics() buttonMetrics {
	label := TextLabel
	switch s {
	case ButtonSmall:
		label.Size = 12
		return buttonMetrics{height: ControlSmall, padding: Space12, radius: RadiusMD, text: label}
	case ButtonCompact:
		label.Size = 12
		return buttonMetrics{height: ControlCompact, padding: Space8, radius: RadiusMD, text: label}
	case ButtonLarge:
		return buttonMetrics{height: ControlLarge, padding: Space24, radius: RadiusLG, text: label}
	}
	return buttonMetrics{height: ControlDefault, padding: Space16, radius: RadiusLG, text: label}
}

// buttonPalette resolves the colors of a variant for a hover amount from 0 to
// 1. Hover colors reproduce the Tailwind states in button.tsx.
type buttonPalette struct {
	fill, border, ink color.NRGBA
	shadow            Elevation
}

func (p *Palette) buttonPalette(variant ButtonVariant, hover, selected float64) buttonPalette {
	switch variant {
	case ButtonSegment:
		colors := buttonPalette{
			fill: lerp(WithAlpha(p.Surface, 0), p.Surface, selected),
			ink:  lerp(p.SecondaryInk, p.Ink, max(selected, hover)),
		}
		if selected > 0.5 {
			colors.shadow = ElevationXS
		}
		return colors
	case ButtonSoft:
		return buttonPalette{
			fill:   lerp(p.Tint(p.Accent, 0.10), p.Tint(p.Accent, 0.16), hover),
			border: Mix(p.Surface, p.Accent, 0.28),
			ink:    p.Accent,
		}
	case ButtonDestructive:
		return buttonPalette{
			fill:   lerp(p.Destructive, Mix(p.PageBackground, p.Destructive, 0.9), hover),
			ink:    p.OnDestructive,
			shadow: ElevationXS,
		}
	case ButtonOutline:
		return buttonPalette{
			fill:   lerp(p.Surface, Mix(p.Surface, p.HoverSurface, 0.6), hover),
			border: lerp(p.Divider, Mix(p.Surface, p.Ink, 0.2), hover),
			ink:    p.Ink,
			shadow: ElevationXS,
		}
	case ButtonGhost:
		return buttonPalette{fill: lerp(WithAlpha(p.HoverSurface, 0), p.HoverSurface, hover), ink: p.Ink}
	case ButtonLink:
		return buttonPalette{ink: p.Accent}
	}
	return buttonPalette{
		fill:   lerp(p.Accent, Mix(p.PageBackground, p.Accent, 0.9), hover),
		ink:    p.OnAccent,
		shadow: ElevationXS,
	}
}

// lerp blends every channel, including alpha, so a transparent state can fade
// into a filled one.
func lerp(a, b color.NRGBA, t float64) color.NRGBA {
	t = max(0, min(1, t))
	mix := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.NRGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

func flag(on bool) float64 {
	if on {
		return 1
	}
	return 0
}

// Button is the neutral outline button at the default height.
func (v View) Button(button *widget.Clickable, title string) layout.Widget {
	return v.NamedButton(button, title, title)
}

// NamedButton is Button with an accessible name that differs from its text.
func (v View) NamedButton(button *widget.Clickable, title, name string) layout.Widget {
	return v.ButtonOf(button, ButtonSpec{Variant: ButtonOutline, Title: title, Name: name})
}

// PrimaryButton is the filled accent button at the default height.
func (v View) PrimaryButton(button *widget.Clickable, title string) layout.Widget {
	return v.ButtonOf(button, ButtonSpec{Variant: ButtonPrimary, Title: title, Name: title})
}

// ButtonOf lays out a button with a variant, size and optional icon.
func (v View) ButtonOf(button *widget.Clickable, spec ButtonSpec) layout.Widget {
	name := spec.Name
	if name == "" {
		name = spec.Title
	}
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, button)
		content := func(gtx layout.Context) layout.Dimensions {
			return v.layoutButton(gtx, button, spec, name)
		}
		if spec.Radio {
			return accessibility.RadioButton(gtx, button, name, spec.Description, spec.Selected, content)
		}
		return accessibility.ButtonWithSelection(gtx, button, name, spec.Description, spec.Selected, content)
	}
}

func (v View) layoutButton(gtx layout.Context, button *widget.Clickable, spec ButtonSpec, name string) layout.Dimensions {
	p := v.Colors
	metrics := spec.Size.metrics()
	if spec.Variant == ButtonSegment {
		metrics = buttonMetrics{height: ControlSmall, padding: Space12, radius: RadiusMD, text: TextLabel}
	}
	if spec.Content != nil {
		metrics.padding = Space12
	}
	enabled := gtx.Enabled()

	state := v.Motion.For(button)
	state.Hover.Set(gtx, flag(enabled && button.Hovered()), DurationBase, EaseOut)
	state.Press.Set(gtx, flag(enabled && button.Pressed()), DurationFast, EaseOut)
	state.Focus.Set(gtx, flag(enabled && gtx.Focused(button)), DurationBase, EaseOut)
	state.Select.Set(gtx, flag(spec.Selected), DurationBase, EaseOut)
	hover, press, focus := state.Hover.Value(gtx), state.Press.Value(gtx), state.Focus.Value(gtx)

	colors := p.buttonPalette(spec.Variant, max(hover, press), state.Select.Value(gtx))
	opacity := 1.0
	if !enabled {
		opacity = DisabledOpacity
	}

	// Measure the content first; the control grows around it.
	padding := gtx.Dp(metrics.padding)
	height := gtx.Dp(metrics.height)
	inner := gtx
	inner.Constraints.Min = image.Point{}
	if spec.Title != "" || spec.Content != nil {
		inner.Constraints.Max.X = max(0, gtx.Constraints.Max.X-2*padding)
	}
	if spec.Content != nil && spec.Fill {
		inner.Constraints.Min.X = inner.Constraints.Max.X
	}
	macro := op.Record(gtx.Ops)
	content := v.buttonContent(inner, spec, metrics, Fade(colors.ink, opacity))
	drawContent := macro.Stop()

	width := content.Size.X + 2*padding
	if spec.Title == "" && spec.Content == nil {
		width = height
	}
	if spec.Content != nil {
		height = max(height, content.Size.Y+2*gtx.Dp(Space8))
	}
	if spec.Fill {
		width = gtx.Constraints.Max.X
	} else if spec.Title != "" || spec.Content != nil {
		gtx.Constraints.Min.X = 0
	}
	size := gtx.Constraints.Constrain(image.Pt(width, height))
	bounds := image.Rectangle{Max: size}

	if enabled {
		p.DrawShadow(gtx, bounds, metrics.radius, colors.shadow)
	}
	cgtx := gtx
	cgtx.Constraints = layout.Exact(size)
	dims := button.Layout(cgtx, func(gtx layout.Context) layout.Dimensions {
		role := semantic.Button
		if spec.Radio {
			role = semantic.RadioButton
		}
		role.Add(gtx.Ops)
		semantic.SelectedOp(spec.Selected).Add(gtx.Ops)
		semantic.LabelOp(name).Add(gtx.Ops)
		if spec.Title == "" || spec.Toggle {
			// An icon has no text, and a tab is found by its name, so the name
			// doubles as the description.
			semantic.DescriptionOp(name).Add(gtx.Ops)
		}
		if enabled {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		scale := 1.0
		if !ReducedMotion() {
			scale = 1 - (1-PressedScale)*press
		}
		defer PressTransform(gtx, size, scale).Pop()
		FillRoundRect(gtx, bounds, metrics.radius, Fade(colors.fill, opacity))
		StrokeRoundRect(gtx, bounds, metrics.radius, 1, Fade(colors.border, opacity))
		offset := op.Offset(image.Pt((size.X-content.Size.X)/2, (size.Y-content.Size.Y)/2)).Push(gtx.Ops)
		if spec.ShortName != "" {
			area := clip.Rect{Max: content.Size}.Push(gtx.Ops)
			semantic.LabelOp(spec.ShortName).Add(gtx.Ops)
			area.Pop()
		}
		drawContent.Add(gtx.Ops)
		if spec.Variant == ButtonLink && hover > 0 {
			underline := image.Rect(0, content.Size.Y-gtx.Dp(3), content.Size.X, content.Size.Y-gtx.Dp(2))
			FillRoundRect(gtx, underline, 0, Fade(colors.ink, hover*opacity))
		}
		offset.Pop()
		return layout.Dimensions{Size: size}
	})
	if focus > 0 {
		p.FocusRingAt(gtx, bounds, metrics.radius, focus)
	}
	return dims
}

func (v View) buttonContent(gtx layout.Context, spec ButtonSpec, metrics buttonMetrics, ink color.NRGBA) layout.Dimensions {
	defer v.textSelection.suspend()()
	if spec.Content != nil {
		return spec.Content(gtx)
	}
	icon := func(gtx layout.Context) layout.Dimensions {
		return LayoutGlyph(gtx, spec.Icon, 16, ink)
	}
	switch {
	case spec.Icon != nil && spec.Title == "":
		return icon(gtx)
	case spec.Icon == nil:
		return v.Line(gtx, metrics.text, spec.Title, ink)
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(Space8)}.Layout(gtx,
		layout.Rigid(icon),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return v.Line(gtx, metrics.text, spec.Title, ink)
		}),
	)
}
