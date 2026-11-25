package ui

import (
	"fmt"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"image"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/widget"
	"gioui.org/widget/material"

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

func (f *TextField) LayoutCurrency(gtx layout.Context, view View, options FieldOptions, currency string) layout.Dimensions {
	options.AccessibleName = view.Language.Text(options.Label) + " · " + currency
	return f.layoutWithAccessories(gtx, view, options, nil, view.Message(currency, view.Colors.SecondaryInk))
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
	invalid := overLimit || problem != ""
	style := material.Editor(view.Theme, &f.Editor, view.Language.Text(options.Hint))
	style.TextSize, style.HintColor = TextBody.Size, colors.SecondaryInk
	style.LineHeightScale = TextBody.LineHeight
	style.SelectionColor = colors.EditorSelection()
	if options.Disabled {
		style.Color = colors.SecondaryInk
	}

	height := ControlDefault
	if !f.Editor.SingleLine {
		height = 120
	}
	layoutEditor := func(gtx layout.Context) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, min(gtx.Constraints.Max.Y, gtx.Dp(height)))
		gtx.Constraints = layout.Exact(size)
		chrome := fieldChrome{Tag: &f.Editor, Active: input.Focused(&f.Editor), Ring: input.Focused(&f.Editor), Invalid: invalid, Disabled: options.Disabled}
		return view.fieldFrame(gtx, chrome, func(gtx layout.Context) layout.Dimensions {
			defer clip.Rect{Max: size}.Push(gtx.Ops).Pop()
			semantic.DescriptionOp(accessibleName).Add(gtx.Ops)
			semantic.EnabledOp(!options.Disabled).Add(gtx.Ops)
			inset := layout.Inset{Left: 12, Right: 8}
			if !f.Editor.SingleLine {
				inset.Top, inset.Bottom = 8, 8
			}
			inset.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				body := func(gtx layout.Context) layout.Dimensions {
					if leading == nil && trailing == nil {
						return style.Layout(gtx)
					}
					var children []layout.FlexChild
					if leading != nil {
						children = append(children, layout.Rigid(leading))
					}
					children = append(children, layout.Flexed(1, style.Layout))
					if trailing != nil {
						children = append(children, layout.Rigid(trailing))
					}
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(Space8)}.Layout(gtx, children...)
				}
				if f.Editor.SingleLine {
					// Centre one line vertically while the editor keeps the full
					// width, so the whole control stays a click and caret target.
					gtx.Constraints.Min = image.Pt(gtx.Constraints.Max.X, 0)
					macro := op.Record(gtx.Ops)
					dims := body(gtx)
					call := macro.Stop()
					defer op.Offset(image.Pt(0, (size.Y-dims.Size.Y)/2)).Push(gtx.Ops).Pop()
					call.Add(gtx.Ops)
					return dims
				}
				return body(gtx)
			})
			return layout.Dimensions{Size: size}
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
			return view.Line(gtx, TextLabel, title, colors.Ink)
		}),
		layout.Rigid(field),
	}
	if options.Help != i18n.NoMessage {
		children = append(children, layout.Rigid(view.Body(TextCaption, view.Language.Text(options.Help), colors.SecondaryInk)))
	}
	if problem != "" {
		children = append(children, layout.Rigid(view.Body(TextCaption, problem, colors.ErrorInk)))
	}
	if options.MaxBytes > 0 {
		message, fg := i18n.FieldByteCount, colors.SecondaryInk
		if overLimit {
			message, fg = i18n.FieldByteExceeded, colors.ErrorInk
		}
		children = append(children, layout.Rigid(view.Body(TextCaption, fmt.Sprintf(view.Language.Text(message), used, options.MaxBytes), fg)))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(Space6)}.Layout(gtx, children...)
}

type SearchField struct {
	Field TextField
	Clear widget.Clickable
}

var searchIcon Glyph = lucide.Search
var clearIcon Glyph = lucide.X

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
		return view.ButtonOf(&s.Clear, ButtonSpec{Variant: ButtonGhost, Size: ButtonCompact, Name: name, Icon: clearIcon})(gtx)
	}
	dims := s.Field.layoutWithAccessories(gtx, view, FieldOptions{Label: label, Hint: hint}, leading, trailing)
	if query := s.Field.Text(); query != previous {
		onChange(query)
	}
	return dims
}
