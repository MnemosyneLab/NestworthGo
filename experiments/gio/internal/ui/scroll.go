package ui

import (
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// List is a vertical or horizontal scrolling list with the shared thin,
// rounded scrollbar. Pages use it instead of material.List so a scrollbar
// change reaches every list.
func (v View) List(state *widget.List) ScrollList {
	style := material.List(v.Theme, state)
	p := v.Colors
	style.Track.Color = WithAlpha(p.Ink, 0)
	style.Track.MajorPadding, style.Track.MinorPadding = 2, 2
	style.Indicator.MinorWidth = unit.Dp(6)
	style.Indicator.CornerRadius = unit.Dp(3)
	style.Indicator.Color = WithAlpha(p.Ink, 0.22)
	style.Indicator.HoverColor = WithAlpha(p.Ink, 0.42)
	// The thin bar floats over the content edge instead of taking width, so a
	// fixed toolbar above a list lines up with the rows below it.
	style.AnchorStrategy = material.Overlay
	return ScrollList{style}
}

// ScrollList is a material list that stays inside its window. A page on the
// shell stage can be unbounded in height; a list there takes the viewport
// height instead of laying out every row.
type ScrollList struct{ material.ListStyle }

// Layout lays out length rows. When the height is unbounded the list is as tall
// as the minimum height the stage passes down, or as its content when the stage
// passes none.
func (l ScrollList) Layout(gtx layout.Context, length int, row layout.ListElement) layout.Dimensions {
	if gtx.Constraints.Max.Y >= unbounded && gtx.Constraints.Min.Y > 0 {
		gtx.Constraints.Max.Y = gtx.Constraints.Min.Y
	}
	return l.ListStyle.Layout(gtx, length, row)
}
