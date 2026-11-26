package ui

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Checkbox shares the toolkit's toggle and keyboard behavior across pages.
func (v View) Checkbox(box *widget.Bool, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, box)
		return material.CheckBox(v.Theme, box, label).Layout(gtx)
	}
}
