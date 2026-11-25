package ui

import (
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"image"
	"image/color"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type PickerField struct {
	Label          i18n.MessageID
	Glyph          Glyph
	Origin         image.Point
	Next, Previous event.Tag
	Disabled       bool
}

var ExpandIcon Glyph = lucide.ChevronDown

func pickerTrigger(gtx layout.Context, view View, field PickerField, trigger *widget.Clickable, value string, invalid bool, pane Popover) bool {
	title := view.Language.Text(field.Label)
	view.Line(gtx, TextLabel, title, view.Colors.Ink)
	y := gtx.Dp(24)
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	defer offset.Pop()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(ControlDefault))
	gtx.Constraints = layout.Exact(size)

	opened := false
	if !field.Disabled {
		anchor := field.Origin.Add(image.Pt(0, y))
		pane.next, pane.previous = field.Next, field.Previous
		opened = view.Overlay.Register(gtx, trigger, image.Rectangle{Min: anchor, Max: anchor.Add(size)}, pane)
	} else {
		gtx = gtx.Disabled()
	}

	view.Focus.Watch(gtx, trigger)
	accessibility.Button(gtx, trigger, title, value, true, func(gtx layout.Context) layout.Dimensions {
		return layoutPickerButton(gtx, view, trigger, title, value, invalid, field.Glyph)
	})
	return opened
}

func layoutPickerButton(gtx layout.Context, view View, trigger *widget.Clickable, title, value string, invalid bool, glyph Glyph) layout.Dimensions {
	colors := view.Colors
	open := view.Overlay != nil && view.Overlay.Trigger == trigger
	chrome := fieldChrome{Tag: trigger, Active: gtx.Focused(trigger) || open, Ring: gtx.Focused(trigger), Invalid: invalid, Disabled: !gtx.Enabled()}
	return view.fieldFrame(gtx, chrome, func(gtx layout.Context) layout.Dimensions {
		return trigger.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			semantic.Button.Add(gtx.Ops)
			semantic.DescriptionOp(title).Add(gtx.Ops)
			if gtx.Enabled() {
				pointer.CursorPointer.Add(gtx.Ops)
			}
			size := image.Pt(gtx.Constraints.Max.X, max(gtx.Constraints.Min.Y, gtx.Dp(ControlDefault)))
			gtx.Constraints = layout.Exact(size)
			layout.Inset{Left: 12, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.Y = size.Y
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return glyphLabel(gtx, view, glyph, value, colors.Ink)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return LayoutGlyph(gtx, ExpandIcon, 16, colors.SecondaryInk)
					}),
				)
			})
			return layout.Dimensions{Size: size}
		})
	})
}

func registerPickerFocus(gtx layout.Context, tag event.Tag) {
	defer clip.Rect{Max: image.Pt(1, 1)}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)
}

func PressedKeys(gtx layout.Context, tag event.Tag, names ...key.Name) []key.Event {
	filters := []event.Filter{key.FocusFilter{Target: tag}}
	for _, name := range names {
		filters = append(filters, key.Filter{Focus: tag, Name: name, Optional: key.ModShift})
	}
	var pressed []key.Event
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			return pressed
		}
		if pressedKey, ok := e.(key.Event); ok && pressedKey.State == key.Press {
			pressed = append(pressed, pressedKey)
		}
	}
}

type Choice struct {
	ID         string
	Label      string
	Glyph      Glyph
	SearchText string
	Disabled   bool
}

// Glyph is a reusable picker or row icon. Colored illustrations may ignore ink.
type Glyph interface {
	Layout(layout.Context, color.NRGBA) layout.Dimensions
}

func LayoutGlyph(gtx layout.Context, glyph Glyph, size unit.Dp, ink color.NRGBA) layout.Dimensions {
	side := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(side, side))
	return glyph.Layout(gtx, ink)
}

func glyphLabel(gtx layout.Context, view View, glyph Glyph, label string, ink color.NRGBA) layout.Dimensions {
	return glyphLabelStyled(gtx, view, glyph, label, TextBody, ink)
}

func glyphLabelStyled(gtx layout.Context, view View, glyph Glyph, label string, style TextStyle, ink color.NRGBA) layout.Dimensions {
	text := func(gtx layout.Context) layout.Dimensions { return view.Line(gtx, style, label, ink) }
	if glyph == nil {
		return text(gtx)
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return LayoutGlyph(gtx, glyph, 16, ink) }),
		layout.Flexed(1, text),
	)
}

type optionList struct {
	scroll    layout.List
	buttons   map[string]*widget.Clickable
	highlight string
	active    bool
	// keyboard shows the highlight. It starts on a keyboard open and after
	// arrow navigation; a mouse open leaves only the check mark and hover.
	keyboard bool
}

func (l *optionList) setActive(items []Choice, preferred string, keyboard bool) {
	l.active, l.keyboard = false, keyboard
	for i, item := range items {
		if !item.Disabled && item.ID == preferred {
			l.highlightAt(items, i)
			return
		}
	}
	l.move(items, 1)
	l.keyboard = keyboard
}

func (l *optionList) move(items []Choice, step int) {
	l.keyboard = true
	index := -1
	if step < 0 {
		index = len(items)
	}
	for i, item := range items {
		if l.active && item.ID == l.highlight && !item.Disabled {
			index = i
			break
		}
	}
	for i := index + step; i >= 0 && i < len(items); i += step {
		if !items[i].Disabled {
			l.highlightAt(items, i)
			return
		}
	}
}

func (l *optionList) highlightAt(items []Choice, index int) {
	l.highlight, l.active = items[index].ID, true
	position := l.scroll.Position
	end := position.First + position.Count
	if position.OffsetLast < 0 {
		end--
	}
	if index < position.First || index >= end || (index == position.First && position.Offset > 0) {
		l.scroll.ScrollTo(index)
	}
}

func (l *optionList) accept(items []Choice, choose func(Choice)) {
	for _, item := range items {
		if l.active && item.ID == l.highlight && !item.Disabled {
			choose(item)
			return
		}
	}
}

func (l *optionList) layout(gtx layout.Context, view View, items []Choice, selected string, showHighlight bool, choose func(Choice)) layout.Dimensions {
	if l.buttons == nil {
		l.buttons = make(map[string]*widget.Clickable)
	}
	l.scroll.Axis = layout.Vertical
	height := min(gtx.Constraints.Max.Y, gtx.Dp(OptionRowHeight)*min(len(items), 6))
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return l.scroll.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		item := items[i]
		button := l.buttons[item.ID]
		if button == nil {
			button = new(widget.Clickable)
			l.buttons[item.ID] = button
		}
		if !item.Disabled && button.Clicked(gtx) {
			l.highlight, l.active = item.ID, true
			choose(item)
		}
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(OptionRowHeight))
		gtx.Constraints = layout.Exact(size)
		return optionRow(gtx, view, button, item, item.ID == selected, showHighlight && l.keyboard && l.active && item.ID == l.highlight)
	})
}

// widestOption measures the popup width the options need: label, row padding,
// check mark and the popup padding and border. Long lists skip measuring and
// keep the anchor width, which is what Wails does with a fixed-width trigger.
func widestOption(gtx layout.Context, view View, items []Choice, extra ...string) int {
	const measureLimit = 64
	if len(items) > measureLimit {
		return 0
	}
	widest := 0
	measure := func(label string, glyph Glyph) {
		probe := gtx
		probe.Constraints = layout.Constraints{Max: image.Pt(1<<20, gtx.Dp(OptionRowHeight))}
		macro := op.Record(gtx.Ops)
		dims := glyphLabelStyled(probe, view, glyph, label, TextBody, color.NRGBA{})
		macro.Stop()
		widest = max(widest, dims.Size.X)
	}
	for _, item := range items {
		measure(item.Label, item.Glyph)
	}
	for _, label := range extra {
		measure(label, nil)
	}
	if widest == 0 {
		return 0
	}
	// Row inset 8+8, gap and check 8+16, popup inset 8+8 and a 1dp border each side.
	return widest + gtx.Dp(8+8+8+16+8+8+2)
}

// OptionRowHeight is one row of a select list.
const OptionRowHeight unit.Dp = 32

var checkIcon Glyph = lucide.Check

// optionRow is a select list row: left aligned, a quiet hover or keyboard
// highlight, and a check mark on the chosen row.
func optionRow(gtx layout.Context, view View, button *widget.Clickable, item Choice, selected, highlighted bool) layout.Dimensions {
	p := view.Colors
	enabled := !item.Disabled
	if !enabled {
		gtx = gtx.Disabled()
	}
	state := view.Motion.For(button)
	state.Hover.Set(gtx, flag(enabled && (button.Hovered() || highlighted)), DurationFast, EaseOut)
	hover := state.Hover.Value(gtx)
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		if enabled {
			pointer.CursorPointer.Add(gtx.Ops)
		}
		size := gtx.Constraints.Min
		bounds := image.Rectangle{Max: size}
		FillRoundRect(gtx, bounds, RadiusMD, lerp(WithAlpha(p.HoverSurface, 0), p.HoverSurface, hover))
		ink := p.Ink
		if !enabled {
			ink = Fade(p.Ink, DisabledOpacity)
		}
		style := TextBody
		if selected {
			style.Weight = font.Medium
		}
		layout.Inset{Left: 8, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y = size.Y
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.Y = 0
					return glyphLabelStyled(gtx, view, item.Glyph, item.Label, style, ink)
				}),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					side := gtx.Dp(16)
					gtx.Constraints = layout.Exact(image.Pt(side, side))
					if !selected {
						return layout.Dimensions{Size: gtx.Constraints.Min}
					}
					return checkIcon.Layout(gtx, ink)
				}),
			)
		})
		return layout.Dimensions{Size: size}
	}
	return accessibility.Button(gtx, button, item.Label, "", false, func(gtx layout.Context) layout.Dimensions {
		if enabled {
			return button.Layout(gtx, content)
		}
		defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
		semantic.EnabledOp(false).Add(gtx.Ops)
		return content(gtx)
	})
}

func choiceButton(gtx layout.Context, view View, button *widget.Clickable, title, description string, enabled, selected, highlighted bool) layout.Dimensions {
	return choiceButtonWithGlyph(gtx, view, button, title, description, enabled, selected, highlighted, nil)
}

func choiceButtonWithGlyph(gtx layout.Context, view View, button *widget.Clickable, title, description string, enabled, selected, highlighted bool, glyph Glyph) layout.Dimensions {
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		if description != "" {
			semantic.DescriptionOp(description).Add(gtx.Ops)
		}
		fg := view.Colors.Ink
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		switch {
		case !enabled:
			fg = view.Colors.SecondaryInk
		case selected:
			paint.FillShape(gtx.Ops, view.Colors.Accent, clip.UniformRRect(bounds, gtx.Dp(8)).Op(gtx.Ops))
			fg = view.Colors.OnAccent
		case button.Hovered() || highlighted:
			paint.FillShape(gtx.Ops, view.Colors.HoverSurface, clip.UniformRRect(bounds, gtx.Dp(8)).Op(gtx.Ops))
		}
		label := layout.Inset{Left: 5, Right: 5}
		if highlighted && enabled {
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: clip.UniformRRect(bounds.Inset(1), gtx.Dp(4)).Path(gtx.Ops), Width: 1}.Op())
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return label.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return glyphLabel(gtx, view, glyph, title, fg)
			})
		})
	}
	name := title
	if description != "" {
		name = description
	}
	if !enabled {
		gtx = gtx.Disabled()
	}
	return accessibility.Button(gtx, button, name, "", false, func(gtx layout.Context) layout.Dimensions {
		if enabled {
			return button.Layout(gtx, content)
		}
		defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
		semantic.EnabledOp(false).Add(gtx.Ops)
		return content(gtx)
	})
}
