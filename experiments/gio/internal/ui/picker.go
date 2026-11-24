package ui

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

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type PickerField struct {
	Label          i18n.MessageID
	Origin         image.Point
	Next, Previous event.Tag
	Disabled       bool
}

var ExpandIcon = MustIcon(icons.NavigationArrowDropDown)

func pickerTrigger(gtx layout.Context, view View, field PickerField, trigger *widget.Clickable, value string, invalid bool, pane Popover) bool {
	title := view.Language.Text(field.Label)
	view.Label(gtx, title, 13, font.Medium, view.Colors.Ink)
	y := gtx.Dp(22)
	offset := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
	defer offset.Pop()
	size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(38))
	gtx.Constraints = layout.Exact(size)

	opened := false
	if !field.Disabled {
		anchor := field.Origin.Add(image.Pt(0, y))
		pane.next, pane.previous = field.Next, field.Previous
		opened = view.Overlay.Register(gtx, trigger, image.Rectangle{Min: anchor, Max: anchor.Add(size)}, pane)
	} else {
		gtx = gtx.Disabled()
	}

	accessibility.Button(gtx, trigger, title, value, true, func(gtx layout.Context) layout.Dimensions {
		return layoutPickerButton(gtx, view, trigger, title, value, invalid)
	})
	return opened
}

func layoutPickerButton(gtx layout.Context, view View, trigger *widget.Clickable, title, value string, invalid bool) layout.Dimensions {
	return trigger.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.DescriptionOp(title).Add(gtx.Ops)
		colors := view.Colors
		border := colors.Divider
		switch {
		case invalid:
			border = colors.ErrorInk
		case gtx.Focused(trigger) || view.Overlay.Trigger == trigger:
			border = colors.Accent
		}
		return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Left: 10, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.Y = 0
						return view.Label(gtx, value, 13, font.Normal, colors.Ink)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						side := gtx.Dp(16)
						gtx.Constraints = layout.Exact(image.Pt(side, side))
						return ExpandIcon.Layout(gtx, colors.SecondaryInk)
					}),
				)
			})
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
	ID       string
	Label    string
	Disabled bool
}

type optionList struct {
	scroll    layout.List
	buttons   map[string]*widget.Clickable
	highlight string
	active    bool
}

func (l *optionList) setActive(items []Choice, preferred string) {
	l.active = false
	for i, item := range items {
		if !item.Disabled && item.ID == preferred {
			l.highlightAt(items, i)
			return
		}
	}
	l.move(items, 1)
}

func (l *optionList) move(items []Choice, step int) {
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
	height := min(gtx.Constraints.Max.Y, gtx.Dp(30)*min(len(items), 6))
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
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(30))
		gtx.Constraints = layout.Exact(size)
		return choiceButton(gtx, view, button, item.Label, "", !item.Disabled, item.ID == selected, showHighlight && l.active && item.ID == l.highlight)
	})
}

func choiceButton(gtx layout.Context, view View, button *widget.Clickable, title, description string, enabled, selected, highlighted bool) layout.Dimensions {
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
			paint.FillShape(gtx.Ops, view.Colors.Accent, clip.UniformRRect(bounds, gtx.Dp(5)).Op(gtx.Ops))
			fg = view.Colors.OnAccent
		case button.Hovered() || highlighted:
			paint.FillShape(gtx.Ops, view.Colors.HoverSurface, clip.UniformRRect(bounds, gtx.Dp(5)).Op(gtx.Ops))
		}
		label := layout.Inset{Left: 5, Right: 5}
		if highlighted && enabled {
			paint.FillShape(gtx.Ops, fg, clip.Stroke{Path: clip.UniformRRect(bounds.Inset(1), gtx.Dp(4)).Path(gtx.Ops), Width: 1}.Op())
		}
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return label.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Label(gtx, title, 13, font.Normal, fg)
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
