package ui

import (
	"gioui.org/layout"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
)

// Checkbox shares the toolkit's toggle and keyboard behavior across pages.
func (v View) Checkbox(box *widget.Bool, label string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		v.Focus.Watch(gtx, box)
		return accessibility.Checkbox(gtx, box, label, material.CheckBox(v.Theme, box, label).Layout)
	}
}
