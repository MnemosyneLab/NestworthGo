package main

import (
	"gioui.org/font"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type componentPreview struct {
	name  widget.Editor
	notes widget.Editor
}

func newComponentPreview() *componentPreview {
	return &componentPreview{
		name:  widget.Editor{SingleLine: true, PaintSelectionWhenUnfocused: true},
		notes: widget.Editor{PaintSelectionWhenUnfocused: true},
	}
}

func (p *componentPreview) layout(gtx layout.Context, view presentation) layout.Dimensions {
	help := material.Label(view.theme, 13, view.language.text(previewFormHelp))
	help.Color = view.colors.secondaryInk
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(help.Layout),
		layout.Rigid(layout.Spacer{Height: 20}.Layout),
		layout.Rigid(previewField(view, &p.name, previewInstitutionName, previewInstitutionHint, 40)),
		layout.Rigid(layout.Spacer{Height: 16}.Layout),
		layout.Rigid(previewField(view, &p.notes, previewNotes, previewNotesHint, 120)),
	)
}

func previewField(view presentation, editor *widget.Editor, label, hint messageID, height unit.Dp) layout.Widget {
	title := view.language.text(label)
	style := material.Editor(view.theme, editor, view.language.text(hint))
	style.TextSize, style.HintColor = 15, view.colors.secondaryInk
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.label(gtx, title, 14, font.Medium, view.colors.ink)
			}),
			layout.Rigid(layout.Spacer{Height: 7}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				border := view.colors.divider
				if gtx.Focused(editor) {
					border = view.colors.accent
				}
				size := gtx.Constraints.Max
				size.Y = min(size.Y, gtx.Dp(height))
				gtx.Constraints = layout.Exact(size)
				defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
				semantic.DescriptionOp(title).Add(gtx.Ops)
				return view.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, style.Layout)
				})
			}),
		)
	}
}
