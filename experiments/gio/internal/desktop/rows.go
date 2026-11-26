package desktop

import (
	"fmt"
	"image/color"
	"strings"

	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

// valueRow is a name on the left with an amount and an optional caption on the
// right, the shape of a list row in the Wails cards. The amount aligns to the
// trailing edge so a column of rows reads as figures.
func valueRow(view ui.View, name, amount, caption string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextBody, name, view.Colors.Ink)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextLabel, amount, view.Colors.Ink)
			}),
		}
		if caption != "" {
			// A flex gap sits between children, so an absent caption must not
			// be a child or the amount would stop short of the trailing edge.
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Dp(52)
				return view.LineEnd(gtx, ui.TextCaption, caption, view.Colors.SecondaryInk)
			}))
		}
		return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, children...)
	}
}

// labeledAmount shows "name ... figure" for a message whose pattern ends with
// the amount, such as "Cost basis · %s". The whole formatted message stays the
// accessible label, so the split is purely visual; a translation that puts the
// amount mid-sentence falls back to plain text.
func labeledAmount(view ui.View, pattern i18n.MessageID, value string, ink color.NRGBA) layout.Widget {
	text := view.Language.Text(pattern)
	const marker = "\x00"
	name, ok := strings.CutSuffix(fmt.Sprintf(text, marker), marker)
	if !ok {
		return view.Message(fmt.Sprintf(text, value), ink)
	}
	return labeledRow(view, fmt.Sprintf(text, value), strings.TrimRight(name, " ·:："), value, ink)
}

// labeledRow draws a muted name on the left and a figure on the right. full is
// the accessible label of the whole row.
func labeledRow(view ui.View, full, name, value string, ink color.NRGBA) layout.Widget {
	return styledRow(view, full, name, ui.TextBody, view.Colors.SecondaryInk, value, ink)
}

// headingRow is labeledRow with the name in ink and label weight, the title of
// a card section whose total sits on the right.
func headingRow(view ui.View, full, name, value string, ink color.NRGBA) layout.Widget {
	return styledRow(view, full, name, ui.TextLabel, view.Colors.Ink, value, ink)
}

func styledRow(view ui.View, full, name string, style ui.TextStyle, nameInk color.NRGBA, value string, ink color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		macro := op.Record(gtx.Ops)
		dims := layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return view.Body(style, name, nameInk)(gtx)
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, ui.TextLabel, value, ink)
			}),
		)
		call := macro.Stop()
		area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
		semantic.LabelOp(full).Add(gtx.Ops)
		call.Add(gtx.Ops)
		area.Pop()
		return dims
	}
}

// semanticRow draws content under one accessible label, for a row whose parts
// read as a single sentence.
func semanticRow(gtx layout.Context, label string, content layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := content(gtx)
	call := macro.Stop()
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	semantic.LabelOp(label).Add(gtx.Ops)
	call.Add(gtx.Ops)
	area.Pop()
	return dims
}

// emptyState is a centered note in a card, for lists that have nothing to show.
func emptyState(view ui.View, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return view.CardPadded(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Top: ui.Space16, Bottom: ui.Space16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return view.Body(ui.TextBody, text, view.Colors.SecondaryInk)(gtx)
				})
			})
		})
	}
}

// summaryFigure is a headline figure in a summary card. A missing value is
// shown as quiet body text so a row of "no value" notes does not shout.
func summaryFigure(view ui.View, text string, ink color.NRGBA) layout.Widget {
	if text == view.Language.Text(i18n.OverviewNoValue) {
		return view.Body(ui.TextBody, text, view.Colors.SecondaryInk)
	}
	return view.Body(ui.TextMoneyMD, text, ink)
}
