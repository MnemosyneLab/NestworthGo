package ui

import (
	"bytes"
	"image"
	"io"

	"gioui.org/font"
	"gioui.org/io/clipboard"
	"gioui.org/io/event"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// TextSelection keeps the window's active selection and the input tags for
// labels drawn this frame. The tags distinguish repeated text such as amounts.
type TextSelection struct {
	labels    map[textLabel][]*textTarget
	counts    map[textLabel]int
	active    *textTarget
	editor    widget.Selectable
	input     input.Router
	ops       op.Ops
	suspended int
	listRow   textListRow
}

type textLabel struct {
	row       textListRow
	origin    image.Point
	value     string
	font      font.Font
	size      unit.Sp
	maxLines  int
	alignment text.Alignment
}

type textListRow struct {
	list  *widget.List
	index int
}

// Input tags must be unique pointers even when the labels have the same text.
type textTarget struct{ _ byte }

func (s *TextSelection) EndFrame() {
	for id, labels := range s.labels {
		count := s.counts[id]
		for _, target := range labels[count:] {
			if s.active == target {
				s.clear()
			}
		}
		if count == 0 {
			delete(s.labels, id)
			continue
		}
		clear(labels[count:])
		s.labels[id] = labels[:count]
	}
	clear(s.counts)
}

func (s *TextSelection) Reset() {
	clear(s.labels)
	clear(s.counts)
	s.clear()
}

func (s *TextSelection) clear() {
	s.active = nil
	s.editor = widget.Selectable{}
	s.input = input.Router{}
}

func (v View) WithTextSelection(selection *TextSelection) View {
	v.textSelection = selection
	return v
}

// WithoutTextSelection keeps a label that is part of a control from receiving
// text gestures in place of the control.
func (v View) WithoutTextSelection() View {
	return v.WithTextSelection(nil)
}

// suspend also covers content closures that captured the original view.
func (s *TextSelection) suspend() func() {
	if s == nil {
		return func() {}
	}
	s.suspended++
	return func() { s.suspended-- }
}

func (v View) layoutText(gtx layout.Context, label material.LabelStyle) layout.Dimensions {
	s := v.textSelection
	if s == nil || s.suspended > 0 || !gtx.Enabled() || label.Text == "" || gtx.Constraints.Max.X <= 0 || gtx.Constraints.Max.Y <= 0 {
		return label.Layout(gtx)
	}
	id := textLabel{s.listRow, v.Origin, label.Text, label.Font, label.TextSize, label.MaxLines, label.Alignment}
	target := s.target(id)
	label.SelectionColor = v.Colors.EditorSelection()
	return s.layout(gtx, target, label)
}

func (s *TextSelection) target(id textLabel) *textTarget {
	if s.labels == nil {
		s.labels = make(map[textLabel][]*textTarget)
		s.counts = make(map[textLabel]int)
	}
	i := s.counts[id]
	s.counts[id] = i + 1
	if i == len(s.labels[id]) {
		s.labels[id] = append(s.labels[id], new(textTarget))
	}
	return s.labels[id][i]
}

func (s *TextSelection) layout(gtx layout.Context, target *textTarget, label material.LabelStyle) layout.Dimensions {
	pressed := false
	filter := pointer.Filter{Target: target, Kinds: pointer.Press | pointer.Drag | pointer.Release | pointer.Cancel}
	for {
		e, ok := gtx.Event(filter)
		if !ok {
			break
		}
		p := e.(pointer.Event)
		if p.Kind == pointer.Press && p.Source == pointer.Mouse && p.Buttons == pointer.ButtonPrimary {
			pressed = true
			if s.active != target {
				s.clear()
				s.active = target
				// The selectable needs its size and handlers before it can
				// receive the first press, including a press with no hover.
				s.drawSelection(gtx, label)
			}
			gtx.Execute(key.FocusCmd{Tag: target})
		}
		if s.active == target {
			// Routers turn raw moves into drags while a pointer is pressed.
			if p.Kind == pointer.Drag {
				p.Kind = pointer.Move
			}
			s.input.Queue(p)
		}
	}

	if s.active == target {
		s.updateKeys(gtx, target, pressed)
	}
	var (
		dims layout.Dimensions
		draw op.CallOp
	)
	selected := s.active == target
	if selected {
		dims, draw = s.drawSelection(gtx, label)
	} else {
		dims = label.Layout(gtx)
	}
	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	if selected {
		// The selectable draws the highlight but omits the static semantic label.
		semantic.LabelOp(label.Text).Add(gtx.Ops)
	}
	pointer.CursorText.Add(gtx.Ops)
	event.Op(gtx.Ops, target)
	draw.Add(gtx.Ops)
	return dims
}

func (s *TextSelection) drawSelection(gtx layout.Context, label material.LabelStyle) (layout.Dimensions, op.CallOp) {
	// Route Gio's selection gestures locally so static labels never add a
	// tab stop. Only the label selected by the mouse receives keyboard focus.
	label.State = &s.editor
	local := gtx
	local.Source = s.input.Source()
	macro := op.Record(gtx.Ops)
	dims := label.Layout(local)
	draw := macro.Stop()
	s.ops.Reset()
	draw.Add(&s.ops)
	s.input.Frame(&s.ops)
	if mime, data, ok := s.input.WriteClipboard(); ok {
		gtx.Execute(clipboard.WriteCmd{Type: mime, Data: io.NopCloser(bytes.NewReader(data))})
	}
	return dims, draw
}

func (s *TextSelection) updateKeys(gtx layout.Context, target *textTarget, pressed bool) {
	for {
		e, ok := gtx.Event(key.FocusFilter{Target: target},
			key.Filter{Focus: target, Name: key.NameEscape},
			key.Filter{Focus: target, Name: "A", Required: key.ModShortcut},
			key.Filter{Focus: target, Name: "C", Required: key.ModShortcut},
			key.Filter{Focus: target, Name: key.NameLeftArrow, Optional: key.ModShortcutAlt | key.ModShift},
			key.Filter{Focus: target, Name: key.NameRightArrow, Optional: key.ModShortcutAlt | key.ModShift},
			key.Filter{Focus: target, Name: key.NameUpArrow, Optional: key.ModShortcutAlt | key.ModShift},
			key.Filter{Focus: target, Name: key.NameDownArrow, Optional: key.ModShortcutAlt | key.ModShift},
			key.Filter{Focus: target, Name: key.NameHome, Optional: key.ModShift},
			key.Filter{Focus: target, Name: key.NameEnd, Optional: key.ModShift},
		)
		if !ok {
			return
		}
		if changed, isFocus := e.(key.FocusEvent); isFocus {
			if !changed.Focus && !pressed && !gtx.Focused(target) {
				s.clear()
				return
			}
			continue
		}
		if k := e.(key.Event); k.Name == key.NameEscape && k.State == key.Press {
			s.clear()
			return
		}
		s.input.Queue(e)
	}
}
