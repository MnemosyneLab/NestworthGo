package accessibility

import (
	"image"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf16"

	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
)

const frameKey = "nestworth.accessibility"
const bindingPrefix = "\x00"

// Roles are shared with bridge_darwin.m.
type role uint8

const (
	staticText role = iota + 1
	group
	button
	popupButton
	textField
	textArea
	list
)

type action uint8

const (
	press action = iota + 1
	focus
	setValue
	setSelection
)

type request struct {
	id            uint64
	action        action
	value         string
	start, length int
}

type element struct {
	ID, Parent                      uint64
	Role                            role
	Name, Value, Help               string
	X, Y, Width, Height             int
	Enabled, Selected               bool
	Focused, Pressable              bool
	Focusable, Editable             bool
	SelectionStart, SelectionLength int
}

type binding struct {
	tag               any
	role              role
	name, value, help string
	button            *widget.Clickable
	editor            *widget.Editor
	enabled, focused  bool
}

type controlKey struct {
	tag  any
	name string
}

type frame struct {
	router   input.Router
	semantic []input.SemanticNode
	ids      map[controlKey]uint64
	nextID   uint64
	bindings map[uint64]binding
	targets  map[uint64]binding
	last     []element
	pending  request
}

func (f *frame) start(gtx layout.Context, req request) layout.Context {
	f.pending = request{}
	target, ok := f.targets[req.id]
	if ok && target.enabled {
		switch req.action {
		case press:
			if target.button != nil {
				target.button.Click()
			}
		case focus:
			gtx.Execute(key.FocusCmd{Tag: target.tag})
		case setValue, setSelection:
			if target.editor != nil && (req.action == setSelection || !target.editor.ReadOnly) {
				f.pending = req
				gtx.Execute(key.FocusCmd{Tag: target.tag})
			}
		}
	}

	clear(f.bindings)
	if gtx.Values == nil {
		gtx.Values = make(map[string]any, 1)
	}
	gtx.Values[frameKey] = f
	return gtx
}

func (f *frame) bind(gtx layout.Context, b binding) {
	if f.ids == nil {
		f.ids = make(map[controlKey]uint64)
		f.bindings = make(map[uint64]binding)
	}
	key := controlKey{tag: b.tag}
	if b.button != nil {
		// A calendar cell can reuse the same widget for a different date.
		key.name = b.name
	}
	id, exists := f.ids[key]
	if !exists {
		f.nextID++
		id = f.nextID<<1 | 1
		f.ids[key] = id
	}
	f.bindings[id] = b
	// Gio's public semantic snapshot omits the input tag. A private description
	// on this bounding clip associates the visible node with its Go widget.
	semantic.DescriptionOp(bindingPrefix + strconv.FormatUint(id, 10)).Add(gtx.Ops)
}

func forContext(gtx layout.Context) *frame {
	f, _ := gtx.Values[frameKey].(*frame)
	return f
}

func track(gtx layout.Context, b binding, widget layout.Widget) layout.Dimensions {
	f := forContext(gtx)
	if f == nil {
		return widget(gtx)
	}
	if b.button != nil || b.editor != nil || b.role == list {
		b.enabled = gtx.Enabled()
		b.focused = gtx.Focused(b.tag)
	}
	recording := op.Record(gtx.Ops)
	dims := widget(gtx)
	content := recording.Stop()
	defer clip.Rect{Max: dims.Size}.Push(gtx.Ops).Pop()
	f.bind(gtx, b)
	content.Add(gtx.Ops)
	return dims
}

// Button attaches native actions to an existing Gio Clickable. A picker exposes
// its current value along with its label.
func Button(gtx layout.Context, click *widget.Clickable, name, value string, picker bool, widget layout.Widget) layout.Dimensions {
	role := button
	if picker {
		role = popupButton
	}
	return track(gtx, binding{tag: click, role: role, name: name, value: value, button: click}, widget)
}

func Editor(gtx layout.Context, editor *widget.Editor, name, help string, widget layout.Widget) layout.Dimensions {
	role := textField
	if !editor.SingleLine {
		role = textArea
	}
	return track(gtx, binding{tag: editor, role: role, name: name, help: help, editor: editor}, widget)
}

// ApplyEditor runs at the same point as ordinary editor input. Consumers such as
// SearchField can then observe the change in the same frame.
func ApplyEditor(gtx layout.Context, editor *widget.Editor) {
	f := forContext(gtx)
	if f == nil || f.pending.id == 0 || f.ids[controlKey{tag: editor}] != f.pending.id || !gtx.Enabled() {
		return
	}
	req := f.pending
	f.pending = request{}
	switch req.action {
	case setValue:
		if !editor.ReadOnly {
			editor.SetText(req.value)
			editor.SetCaret(editor.Len(), editor.Len())
		}
	case setSelection:
		start, end := runeRange(editor.Text(), req.start, req.length)
		editor.SetCaret(start, end)
	}
}

func List(gtx layout.Context, tag any, name string, widget layout.Widget) layout.Dimensions {
	return track(gtx, binding{tag: tag, role: list, name: name, enabled: true}, widget)
}

func Row(gtx layout.Context, tag any, widget layout.Widget) layout.Dimensions {
	return track(gtx, binding{tag: tag, role: group, enabled: true}, widget)
}

func (f *frame) snapshot(gtx layout.Context) ([]element, bool) {
	f.router.Frame(gtx.Ops)
	f.semantic = f.router.AppendSemantics(f.semantic[:0])
	if f.targets == nil {
		f.targets = make(map[uint64]binding)
	}
	clear(f.targets)
	elements := f.collect(nil, f.semantic[0].Children, 0, image.Rectangle{Max: gtx.Constraints.Max}, false)
	for tag, id := range f.ids {
		if _, exists := f.targets[id]; !exists {
			delete(f.ids, tag)
		}
	}

	if reflect.DeepEqual(f.last, elements) {
		return nil, false
	}
	f.last = elements
	return elements, true
}

func (f *frame) collect(out []element, nodes []input.SemanticNode, parent uint64, viewport image.Rectangle, hideLabels bool) []element {
	for _, node := range nodes {
		desc := node.Desc
		bounds := desc.Bounds.Intersect(viewport)
		id, tracked := bindingID(desc.Description)
		if tracked {
			b, ok := f.bindings[id]
			if !ok || bounds.Empty() {
				continue
			}
			f.targets[id] = b
			e := makeElement(id, parent, b.role, b.name, bounds)
			e.Enabled, e.Focused, e.Value, e.Help = b.enabled, b.focused, b.value, b.help
			e.Pressable = b.button != nil && b.enabled
			e.Focusable = (b.button != nil || b.editor != nil || b.role == list) && b.enabled
			if b.button != nil {
				e.Selected = desc.Selected || hasSelection(node.Children)
			}
			if b.editor != nil {
				e.Editable = b.enabled && !b.editor.ReadOnly
				e.Value = b.editor.Text()
				e.SelectionStart, e.SelectionLength = utf16Range(e.Value, b.editor.Selection)
			}
			out = append(out, e)
			if b.role == group || b.role == list {
				out = f.collect(out, node.Children, e.ID, bounds, false)
			} else {
				out = f.collect(out, node.Children, parent, bounds, true)
			}
			continue
		}

		if !hideLabels && !bounds.Empty() && desc.Label != "" {
			id := uint64(node.ID) << 1
			e := makeElement(id, parent, staticText, desc.Label, bounds)
			e.Value = desc.Label
			out = append(out, e)
		}
		out = f.collect(out, node.Children, parent, viewport, hideLabels)
	}
	return out
}

func bindingID(description string) (uint64, bool) {
	raw, ok := strings.CutPrefix(description, bindingPrefix)
	if !ok {
		return 0, false
	}
	id, err := strconv.ParseUint(raw, 10, 64)
	return id, err == nil
}

func hasSelection(nodes []input.SemanticNode) bool {
	for _, node := range nodes {
		if node.Desc.Selected {
			return true
		}
		if hasSelection(node.Children) {
			return true
		}
	}
	return false
}

func makeElement(id, parent uint64, role role, name string, bounds image.Rectangle) element {
	return element{
		ID: id, Parent: parent, Role: role, Name: name, Enabled: true,
		X: bounds.Min.X, Y: bounds.Min.Y, Width: bounds.Dx(), Height: bounds.Dy(),
	}
}

// AppKit text ranges are UTF-16 offsets. Gio editors count Unicode code points.
func utf16Range(value string, selection func() (int, int)) (start, length int) {
	a, b := selection()
	a, b = min(a, b), max(a, b)
	offset := 0
	for _, r := range value {
		if b <= 0 {
			break
		}
		offset += utf16.RuneLen(r)
		if a > 0 {
			start = offset
		}
		a--
		b--
	}
	return start, offset - start
}

func runeRange(value string, start, length int) (int, int) {
	start = max(0, start)
	end := start + min(max(0, length), int(^uint(0)>>1)-start)
	a, b, offset := 0, 0, 0
	for _, r := range value {
		offset += utf16.RuneLen(r)
		if offset <= start {
			a++
		}
		if offset <= end {
			b++
		}
	}
	return a, b
}
