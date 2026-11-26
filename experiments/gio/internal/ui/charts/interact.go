package charts

import (
	"image"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
)

// interaction is the pointer and keyboard state every chart shares. A chart
// is one tab stop. Its items are reached with the arrow keys, the way the
// rows of a list box are, so a chart with hundreds of points does not add
// hundreds of tab stops.
type interaction struct {
	hover  Selection
	cursor Selection
	// at is the last pointer position in chart coordinates.
	at image.Point
}

// frameItems describes the items of one frame to the interaction.
type frameItems struct {
	// order lists every item in keyboard order.
	order []Selection
	// pick maps a point in chart coordinates to the item under it.
	pick func(image.Point) (Selection, bool)
	// move returns the item a navigation key reaches from the cursor. A nil
	// move steps through order: Left and Up go back, Right and Down advance.
	move     func(from Selection, name key.Name) (Selection, bool)
	selected Selection
	// activate requests the same action as pressing Return on an item.
	activate Selection
}

var navigationKeys = [...]key.Name{
	key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow,
	key.NameHome, key.NameEnd, key.NameReturn, key.NameEnter, key.NameSpace, key.NameEscape,
}

// update reads this frame's input for tag and returns the selection requests
// it produced. It must run before the chart draws, with items built from the
// same spec the chart is about to draw.
func (in *interaction) update(gtx layout.Context, tag event.Tag, items frameItems) []Event {
	if !gtx.Enabled() {
		in.hover = Selection{}
		return nil
	}
	if !contains(items.order, in.hover) {
		in.hover = Selection{}
	}
	filters := []event.Filter{
		pointer.Filter{Target: tag, Kinds: pointer.Press | pointer.Move | pointer.Enter | pointer.Leave | pointer.Cancel},
		key.FocusFilter{Target: tag},
	}
	for _, name := range navigationKeys {
		filters = append(filters, key.Filter{Focus: tag, Name: name})
	}
	var events []Event
	toggle := func(sel Selection) {
		if sel.IsZero() {
			return
		}
		if sel == items.selected {
			events = append(events, Event{Kind: EventClear, Selection: sel})
		} else {
			events = append(events, Event{Kind: EventSelect, Selection: sel})
		}
	}
	if !items.activate.IsZero() {
		in.cursor, in.hover = items.activate, Selection{}
		gtx.Execute(key.FocusCmd{Tag: tag})
		toggle(items.activate)
	}
	for {
		e, ok := gtx.Event(filters...)
		if !ok {
			break
		}
		switch e := e.(type) {
		case pointer.Event:
			at := image.Pt(int(e.Position.X+0.5), int(e.Position.Y+0.5))
			in.at = at
			switch e.Kind {
			case pointer.Move, pointer.Enter:
				in.hover, _ = pick(items, at)
			case pointer.Leave, pointer.Cancel:
				in.hover = Selection{}
			case pointer.Press:
				if e.Source == pointer.Mouse && !e.Buttons.Contain(pointer.ButtonPrimary) {
					continue
				}
				gtx.Execute(key.FocusCmd{Tag: tag})
				if sel, ok := pick(items, at); ok {
					in.cursor, in.hover = sel, sel
					toggle(sel)
				}
			}
		case key.FocusEvent:
			if e.Focus && !contains(items.order, in.cursor) {
				in.cursor = defaultCursor(items)
			}
		case key.Event:
			if e.State != key.Press {
				continue
			}
			if !contains(items.order, in.cursor) {
				in.cursor = defaultCursor(items)
			}
			switch e.Name {
			case key.NameReturn, key.NameEnter, key.NameSpace:
				toggle(in.cursor)
			case key.NameEscape:
				if !items.selected.IsZero() {
					events = append(events, Event{Kind: EventClear, Selection: items.selected})
				}
			default:
				if next, ok := in.step(items, e.Name); ok {
					in.cursor = next
					// Moving the cursor takes over from a stale pointer hover.
					in.hover = Selection{}
				}
			}
		}
	}
	if len(events) > 0 {
		gtx.Execute(op.InvalidateCmd{})
	}
	return events
}

func (in *interaction) step(items frameItems, name key.Name) (Selection, bool) {
	if items.move != nil {
		return items.move(in.cursor, name)
	}
	return linearMove(items.order, in.cursor, name)
}

// linearMove steps through order for the navigation keys.
func linearMove(order []Selection, from Selection, name key.Name) (Selection, bool) {
	if len(order) == 0 {
		return Selection{}, false
	}
	at := indexOf(order, from)
	switch name {
	case key.NameLeftArrow, key.NameUpArrow:
		return order[max(0, at-1)], true
	case key.NameRightArrow, key.NameDownArrow:
		return order[min(len(order)-1, at+1)], true
	case key.NameHome:
		return order[0], true
	case key.NameEnd:
		return order[len(order)-1], true
	}
	return Selection{}, false
}

func pick(items frameItems, at image.Point) (Selection, bool) {
	if items.pick == nil {
		return Selection{}, false
	}
	return items.pick(at)
}

func defaultCursor(items frameItems) Selection {
	if contains(items.order, items.selected) {
		return items.selected
	}
	if len(items.order) > 0 {
		return items.order[0]
	}
	return Selection{}
}

// active is the item that receives emphasis and the tooltip: the item under
// the pointer, otherwise the keyboard cursor while the chart has focus.
func (in *interaction) active(gtx layout.Context, tag event.Tag, order []Selection) Selection {
	if contains(order, in.hover) {
		return in.hover
	}
	if gtx.Focused(tag) && gtx.Enabled() {
		if contains(order, in.cursor) {
			return in.cursor
		}
		return defaultCursor(frameItems{order: order})
	}
	return Selection{}
}

// register makes the whole chart the input area of tag. Call it after the
// chart is drawn, inside the chart's own coordinate system.
func register(gtx layout.Context, tag event.Tag, size image.Point, hovering bool) {
	defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
	if hovering && gtx.Enabled() {
		pointer.CursorPointer.Add(gtx.Ops)
	}
	event.Op(gtx.Ops, tag)
}

func contains(order []Selection, sel Selection) bool {
	return indexOf(order, sel) >= 0 && !sel.IsZero()
}

func indexOf(order []Selection, sel Selection) int {
	for i, candidate := range order {
		if candidate == sel {
			return i
		}
	}
	return -1
}
