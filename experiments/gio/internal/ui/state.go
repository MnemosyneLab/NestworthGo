package ui

import "gioui.org/io/event"

// Motion keeps the transient animation state of controls drawn through View.
// Pages own their widgets (a Clickable, an Editor) and pass them in each
// frame; the visual state that follows from interaction, such as the hover
// blend or focus ring fade, lives here keyed by the same tag. That keeps call
// sites unchanged and lets one owner prune state for controls that stopped
// being drawn.
//
// A nil *Motion is valid. Controls then render their target state without
// animation, which tests and one-off windows rely on.
type Motion struct {
	items map[event.Tag]*ControlMotion
	frame uint64
}

// ControlMotion is the animated state shared by buttons and similar controls.
type ControlMotion struct {
	Hover, Press, Focus Tween
	used                uint64
}

// For returns the state for tag, creating it on first use. It marks the entry
// as drawn in the current frame.
func (m *Motion) For(tag event.Tag) *ControlMotion {
	if m == nil {
		return new(ControlMotion)
	}
	if m.items == nil {
		m.items = make(map[event.Tag]*ControlMotion)
	}
	item := m.items[tag]
	if item == nil {
		item = new(ControlMotion)
		m.items[tag] = item
	}
	item.used = m.frame
	return item
}

// EndFrame discards the state of controls that were not drawn this frame, so
// a list that recreates its rows does not grow the registry without bound.
func (m *Motion) EndFrame() {
	if m == nil {
		return
	}
	for tag, item := range m.items {
		if item.used != m.frame {
			delete(m.items, tag)
		}
	}
	m.frame++
}

// Len reports the number of tracked controls, for tests.
func (m *Motion) Len() int {
	if m == nil {
		return 0
	}
	return len(m.items)
}
