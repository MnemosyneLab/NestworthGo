package main

import (
	"image"

	"gioui.org/font"
	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type pickerField struct {
	label          messageID
	origin         image.Point
	next, previous event.Tag
	disabled       bool
}

var expandIcon = mustIcon(icons.NavigationArrowDropDown)

func pickerTrigger(gtx layout.Context, view presentation, field pickerField, trigger *widget.Clickable, value string, invalid bool, pane popoverPane) bool {
	title := view.language.text(field.label)
	view.label(gtx, title, 13, font.Medium, view.colors.ink)
	y := gtx.Dp(22)
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	defer offset.Pop()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(38))
	gtx.Constraints = layout.Exact(size)

	opened := false
	if !field.disabled {
		anchor := field.origin.Add(image.Pt(0, y))
		pane.next, pane.previous = field.next, field.previous
		opened = view.overlay.register(gtx, trigger, image.Rectangle{Min: anchor, Max: anchor.Add(size)}, pane)
	} else {
		gtx = gtx.Disabled()
	}

	trigger.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(title).Add(gtx.Ops)
		colors := view.colors
		border := colors.divider
		switch {
		case invalid:
			border = colors.errorInk
		case gtx.Focused(trigger) || view.overlay.trigger == trigger:
			border = colors.accent
		}
		return view.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.W.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return view.label(gtx, value, 13, font.Normal, colors.ink)
						})
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						side := gtx.Dp(16)
						gtx.Constraints = layout.Exact(image.Pt(side, side))
						return expandIcon.Layout(gtx, colors.secondaryInk)
					}),
				)
			})
		})
	})
	return opened
}

func registerPickerFocus(gtx layout.Context, tag event.Tag) {
	defer clip.Rect{Max: image.Pt(1, 1)}.Push(gtx.Ops).Pop()
	event.Op(gtx.Ops, tag)
}

func pickerKeys(gtx layout.Context, tag event.Tag, names ...key.Name) []key.Event {
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

type choice struct {
	id       string
	label    string
	disabled bool
}

type optionList struct {
	scroll    layout.List
	buttons   map[string]*widget.Clickable
	highlight string
	active    bool
}

func (l *optionList) setActive(items []choice, preferred string) {
	l.active = false
	for i, item := range items {
		if !item.disabled && item.id == preferred {
			l.highlightAt(items, i)
			return
		}
	}
	l.move(items, 1)
}

func (l *optionList) move(items []choice, step int) {
	index := -1
	if step < 0 {
		index = len(items)
	}
	for i, item := range items {
		if l.active && item.id == l.highlight && !item.disabled {
			index = i
			break
		}
	}
	for i := index + step; i >= 0 && i < len(items); i += step {
		if !items[i].disabled {
			l.highlightAt(items, i)
			return
		}
	}
}

func (l *optionList) highlightAt(items []choice, index int) {
	l.highlight, l.active = items[index].id, true
	position := l.scroll.Position
	end := position.First + position.Count
	if position.OffsetLast < 0 {
		end--
	}
	if index < position.First || index >= end || (index == position.First && position.Offset > 0) {
		l.scroll.ScrollTo(index)
	}
}

func (l *optionList) accept(items []choice, choose func(choice)) {
	for _, item := range items {
		if l.active && item.id == l.highlight && !item.disabled {
			choose(item)
			return
		}
	}
}

func (l *optionList) layout(gtx layout.Context, view presentation, items []choice, selected string, showHighlight bool, choose func(choice)) layout.Dimensions {
	if l.buttons == nil {
		l.buttons = make(map[string]*widget.Clickable)
	}
	l.scroll.Axis = layout.Vertical
	height := min(gtx.Constraints.Max.Y, gtx.Dp(30)*min(len(items), 6))
	gtx.Constraints = layout.Exact(image.Pt(gtx.Constraints.Max.X, height))
	return l.scroll.Layout(gtx, len(items), func(gtx layout.Context, i int) layout.Dimensions {
		item := items[i]
		button := l.buttons[item.id]
		if button == nil {
			button = new(widget.Clickable)
			l.buttons[item.id] = button
		}
		if !item.disabled && button.Clicked(gtx) {
			l.highlight, l.active = item.id, true
			choose(item)
		}
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(30))
		gtx.Constraints = layout.Exact(size)
		return choiceButton(gtx, view, button, item.label, "", !item.disabled, item.id == selected, showHighlight && l.active && item.id == l.highlight)
	})
}

func choiceButton(gtx layout.Context, view presentation, button *widget.Clickable, title, description string, enabled, selected, highlighted bool) layout.Dimensions {
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(selected).Add(gtx.Ops)
		if description != "" {
			semantic.DescriptionOp(description).Add(gtx.Ops)
		}
		fg := view.colors.ink
		bounds := image.Rectangle{Max: gtx.Constraints.Min}
		switch {
		case !enabled:
			fg = view.colors.secondaryInk
		case selected:
			paint.FillShape(gtx.Ops, view.colors.accent, clip.UniformRRect(bounds, gtx.Dp(5)).Op(gtx.Ops))
			fg = view.colors.pageBackground
		case button.Hovered() || highlighted:
			paint.FillShape(gtx.Ops, view.colors.hoverSurface, clip.UniformRRect(bounds, gtx.Dp(5)).Op(gtx.Ops))
		}
		label := layout.Inset{Left: 5, Right: 5}
		if highlighted && enabled {
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: clip.UniformRRect(bounds.Inset(1), gtx.Dp(4)).Path(gtx.Ops), Width: 1}.Op())
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return label.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.label(gtx, title, 13, font.Normal, fg)
			})
		})
	}
	if enabled {
		return button.Layout(gtx, content)
	}
	defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
	semantic.EnabledOp(false).Add(gtx.Ops)
	return content(gtx)
}
