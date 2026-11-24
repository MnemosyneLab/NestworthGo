package ui

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

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type FieldOptions struct {
	Label, Hint, Help  i18n.MessageID
	AccessibleName     string
	MaxBytes           int
	Disabled, ReadOnly bool
	Problem            func(string) string
}

type TextField struct {
	Editor widget.Editor
}

func NewTextField(singleLine bool) TextField {
	// Form actions are buttons. Never enable the editor's submit mode.
	return TextField{Editor: widget.Editor{
		SingleLine: singleLine, PaintSelectionWhenUnfocused: true,
	}}
}

func (f *TextField) Text() string { return f.Editor.Text() }

func (f *TextField) Focus(gtx layout.Context) {
	gtx.Execute(key.FocusCmd{Tag: &f.Editor})
}

func (f *TextField) Layout(gtx layout.Context, view View, options FieldOptions) layout.Dimensions {
	return f.layoutWithAccessories(gtx, view, options, nil, nil)
}

func (f *TextField) layoutWithAccessories(gtx layout.Context, view View, options FieldOptions, leading, trailing layout.Widget) layout.Dimensions {
	f.Editor.ReadOnly = options.ReadOnly
	title := view.Language.Text(options.Label)
	accessibleName := options.AccessibleName
	if accessibleName == "" {
		accessibleName = title
	}
	input := gtx
	if options.Disabled {
		input = gtx.Disabled()
	}
	accessibility.ApplyEditor(input, &f.Editor)
	if f.Editor.SingleLine {
		// Gio turns an unhandled Return in a single-line editor into a space.
		// Consume those key events after the platform IME has had its turn.
		for {
			_, ok := input.Event(
				key.Filter{Focus: &f.Editor, Name: key.NameReturn, Optional: key.ModShift},
				key.Filter{Focus: &f.Editor, Name: key.NameEnter, Optional: key.ModShift},
			)
			if !ok {
				break
			}
		}
	}
	for {
		if _, ok := f.Editor.Update(input); !ok {
			break
		}
	}

	// A byte budget constrains submission. Keep the original draft editable.
	value := f.Text()
	used := len(value)
	overLimit := options.MaxBytes > 0 && used > options.MaxBytes
	problem := ""
	if options.Problem != nil {
		problem = options.Problem(value)
	}
	colors := view.Colors
	border := colors.Divider
	switch {
	case overLimit || problem != "":
		border = colors.ErrorInk
	case input.Focused(&f.Editor):
		border = colors.Accent
	}
	style := material.Editor(view.Theme, &f.Editor, view.Language.Text(options.Hint))
	style.TextSize, style.HintColor = 15, colors.SecondaryInk
	style.SelectionColor = colors.EditorSelection()
	if options.Disabled {
		style.Color = colors.SecondaryInk
	}

	height := unit.Dp(40)
	if !f.Editor.SingleLine {
		height = 120
	}
	layoutEditor := func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, min(gtx.Constraints.Max.Y, gtx.Dp(height)))
		gtx.Constraints = layout.Exact(size)
		defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
		semantic.DescriptionOp(accessibleName).Add(gtx.Ops)
		semantic.EnabledOp(!options.Disabled).Add(gtx.Ops)
		return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 12, Right: 8}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				if leading == nil && trailing == nil {
					return style.Layout(gtx)
				}
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(8)}.Layout(gtx,
					layout.Rigid(leading), layout.Flexed(1, style.Layout), layout.Rigid(trailing))
			})
		})
	}
	field := func(gtx layout.Context) layout.Dimensions {
		if options.Disabled {
			gtx = gtx.Disabled()
		}
		view.Focus.Watch(gtx, &f.Editor)
		return accessibility.Editor(gtx, &f.Editor, accessibleName, view.Language.Text(options.Help), layoutEditor)
	}
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, title, 14, font.Medium, colors.Ink)
		}),
		layout.Rigid(field),
	}
	if options.Help != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Message(view.Language.Text(options.Help), colors.SecondaryInk)))
	}
	if problem != "" {
		children = append(children, layout.Rigid(view.Message(problem, colors.ErrorInk)))
	}
	if options.MaxBytes > 0 {
		message, fg := i18n.FieldByteCount, colors.SecondaryInk
		if overLimit {
			message, fg = i18n.FieldByteExceeded, colors.ErrorInk
		}
		children = append(children, layout.Rigid(view.Message(fmt.Sprintf(view.Language.Text(message), used, options.MaxBytes), fg)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(5)}.Layout(gtx, children...)
}

type SearchField struct {
	Field TextField
	Clear widget.Clickable
}

var searchIcon = MustIcon(icons.ActionSearch)
var clearIcon = MustIcon(icons.NavigationClose)

func (s *SearchField) Layout(gtx layout.Context, view View, label, hint i18n.MessageID, onChange func(string)) layout.Dimensions {
	previous := s.Field.Text()
	if s.Clear.Clicked(gtx) && previous != "" {
		s.Field.Editor.SetText("")
		s.Field.Focus(gtx)
	}
	leading := func(gtx layout.Context) layout.Dimensions {
		size := gtx.Dp(16)
		gtx.Constraints = layout.Exact(image.Pt(size, size))
		return searchIcon.Layout(gtx, view.Colors.SecondaryInk)
	}
	trailing := func(gtx layout.Context) layout.Dimensions {
		if s.Field.Text() == "" {
			return layout.Dimensions{}
		}
		name := view.Language.Text(i18n.FieldClearSearch)
		clear := material.IconButton(view.Theme, &s.Clear, clearIcon, name)
		clear.Background, clear.Color = view.Colors.Surface, view.Colors.Ink
		clear.Size, clear.Inset = 16, layout.UniformInset(4)
		view.Focus.Watch(gtx, &s.Clear)
		return accessibility.Button(gtx, &s.Clear, name, "", false, clear.Layout)
	}
	dims := s.Field.layoutWithAccessories(gtx, view, FieldOptions{Label: label, Hint: hint}, leading, trailing)
	if query := s.Field.Text(); query != previous {
		onChange(query)
	}
	return dims
}
