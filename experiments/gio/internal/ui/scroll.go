package ui

import (
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// List is a vertical or horizontal scrolling list with the shared thin,
// rounded scrollbar. Pages use it instead of material.List so a scrollbar
// change reaches every list.
func (v View) List(state *widget.List) material.ListStyle {
	style := material.List(v.Theme, state)
	p := v.Colors
	style.Track.Color = WithAlpha(p.Ink, 0)
	style.Track.MajorPadding, style.Track.MinorPadding = 2, 2
	style.Indicator.MinorWidth = unit.Dp(6)
	style.Indicator.CornerRadius = unit.Dp(3)
	style.Indicator.Color = WithAlpha(p.Ink, 0.22)
	style.Indicator.HoverColor = WithAlpha(p.Ink, 0.42)
	return style
}
