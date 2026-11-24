package main

import (
	"fmt"
	"image"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

type fieldOptions struct {
	label, hint, help  messageID
	maxBytes           int
	disabled, readOnly bool
	problem            func(string) string
}

type textField struct {
	editor widget.Editor
}

func newTextField(singleLine bool) textField {
	// Form actions are buttons. Never enable the editor's submit mode.
	return textField{editor: widget.Editor{
		SingleLine: singleLine, PaintSelectionWhenUnfocused: true,
	}}
}

func (f *textField) text() string { return f.editor.Text() }

func (f *textField) focus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &f.editor})
}

func (f *textField) layout(gtx layout.Context, view presentation, options fieldOptions) layout.Dimensions {
	return f.layoutWithAccessories(gtx, view, options, nil, nil)
}

func (f *textField) layoutWithAccessories(gtx layout.Context, view presentation, options fieldOptions, leading, trailing layout.Widget) layout.Dimensions {
	f.editor.ReadOnly = options.readOnly
	title := view.language.text(options.label)
	input := gtx
	if options.disabled {
		input = gtx.Disabled()
	}
	if f.editor.SingleLine {
		// Gio turns an unhandled Return in a single-line editor into a space.
		// Consume those key events after the platform IME has had its turn.
		for {
			_, ok := input.Event(
				key.Filter{Focus: &f.editor, Name: key.NameReturn, Optional: key.ModShift},
				key.Filter{Focus: &f.editor, Name: key.NameEnter, Optional: key.ModShift},
			)
			if !ok {
				break
			}
		}
	}
	for {
		if _, ok := f.editor.Update(input); !ok {
			break
		}
	}

	// A byte budget constrains submission. Keep the original draft editable.
	value := f.text()
	used := len(value)
	overLimit := options.maxBytes > 0 && used > options.maxBytes
	problem := ""
	if options.problem != nil {
		problem = options.problem(value)
	}
	colors := view.colors
	border := colors.divider
	switch {
	case overLimit || problem != "":
		border = colors.errorInk
	case input.Focused(&f.editor):
		border = colors.accent
	}
	style := material.Editor(view.theme, &f.editor, view.language.text(options.hint))
	style.TextSize, style.HintColor = 15, colors.secondaryInk
	style.SelectionColor = colors.editorSelection()
	if options.disabled {
		style.Color = colors.secondaryInk
	}

	height := unit.Dp(40)
	if !f.editor.SingleLine {
		height = 120
	}
	field := func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, min(gtx.Constraints.Max.Y, gtx.Dp(height)))
		gtx.Constraints = layout.Exact(size)
		defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
		semantic.DescriptionOp(title).Add(gtx.Ops)
		semantic.EnabledOp(!options.disabled).Add(gtx.Ops)
		return view.panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
			if options.disabled {
				gtx = gtx.Disabled()
			}
			return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if leading == nil && trailing == nil {
					return style.Layout(gtx)
				}
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Rigid(leading), layout.Flexed(1, style.Layout), layout.Rigid(trailing))
			})
		})
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.label(gtx, title, 14, font.Medium, colors.ink)
		}),
		layout.Rigid(field),
	}
	if options.help != noMessage {
		children = append(children, layout.Rigid(view.message(view.language.text(options.help), colors.secondaryInk)))
	}
	if problem != "" {
		children = append(children, layout.Rigid(view.message(problem, colors.errorInk)))
	}
	if options.maxBytes > 0 {
		message, fg := fieldByteCount, colors.secondaryInk
		if overLimit {
			message, fg = fieldByteExceeded, colors.errorInk
		}
		children = append(children, layout.Rigid(view.message(fmt.Sprintf(view.language.text(message), used, options.maxBytes), fg)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx, children...)
}

type searchField struct {
	field textField
	clear widget.Clickable
}

var searchIcon = mustIcon(icons.ActionSearch)
var clearIcon = mustIcon(icons.NavigationClose)

func (s *searchField) layout(gtx layout.Context, view presentation, label, hint messageID, onChange func(string)) layout.Dimensions {
	previous := s.field.text()
	if s.clear.Clicked(gtx) && previous != "" {
		s.field.editor.SetText("")
		s.field.focus(gtx)
	}
	leading := func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(16)
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		return searchIcon.Layout(gtx, view.colors.secondaryInk)
	}
	trailing := func(gtx layout.Context) layout.Dimensions {
		if s.field.text() == "" {
			return layout.Dimensions{}
		}
		clear := material.IconButton(view.theme, &s.clear, clearIcon, view.language.text(fieldClearSearch))
		clear.Background, clear.Color = view.colors.surface, view.colors.ink
		clear.Size, clear.Inset = 16, layout.UniformInset(4)
		return clear.Layout(gtx)
	}
	dims := s.field.layoutWithAccessories(gtx, view, fieldOptions{label: label, hint: hint}, leading, trailing)
	if query := s.field.text(); query != previous {
		onChange(query)
	}
	return dims
}
