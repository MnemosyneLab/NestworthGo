package desktop

import (
	"image"
	"image/color"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type analysisSheet struct {
	close, retry, action widget.Clickable
	list                 widget.List
	focus                bool
}

func newAnalysisSheet() analysisSheet {
	return analysisSheet{focus: true, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

func (s *analysisSheet) dismissed(gtx layout.Context) bool {
	if s.focus {
		s.focus = false
		gtx.Execute(key.FocusCmd{Tag: &s.close})
	}
	for _, tag := range []*widget.Clickable{&s.close, &s.retry, &s.action} {
		for _, press := range ui.PressedKeys(gtx, tag, key.NameEscape, key.NamePageUp, key.NamePageDown) {
			switch press.Name {
			case key.NameEscape:
				return true
			case key.NamePageUp:
				s.list.Position.Offset -= gtx.Dp(180)
			case key.NamePageDown:
				s.list.Position.Offset += gtx.Dp(180)
			}
		}
	}
	_, outside := gtx.Event(pointer.Filter{Target: s, Kinds: pointer.Press})
	return outside || s.close.Clicked(gtx)
}

func (s *analysisSheet) layout(gtx layout.Context, view ui.View, title string, body layout.Widget, action i18n.MessageID) {
	s.layoutActions(gtx, view, title, body, action, true)
}

func (s *analysisSheet) layoutActions(gtx layout.Context, view ui.View, title string, body layout.Widget, action i18n.MessageID, canClose bool) {
	width := min(gtx.Constraints.Max.X, gtx.Dp(450))
	backdrop := image.Rect(0, 0, gtx.Constraints.Max.X-width, gtx.Constraints.Max.Y)
	paint.Fill(gtx.Ops, color.NRGBA{A: 0x65})
	mask := clip.Rect(backdrop).Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	mask.Pop()
	layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y))
		return view.Floating(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				rows := []ui.ColumnChild{
					ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
							layout.Flexed(1, view.Body(ui.TextHeading, title, view.Colors.Ink)),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								if !canClose {
									gtx = gtx.Disabled()
								}
								return view.Button(&s.close, view.Language.Text(i18n.FieldDismiss))(gtx)
							}))
					}),
					ui.Fill(body),
				}
				if action != i18n.NoMessage {
					rows = append(rows, ui.Rigid(view.Button(&s.action, view.Language.Text(action))))
				}
				return (ui.Column{Gap: 12}).Layout(gtx, rows...)
			})
		})
	})
}
