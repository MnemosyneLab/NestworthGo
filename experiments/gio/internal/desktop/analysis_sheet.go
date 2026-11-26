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
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type analysisSheet struct {
	close, retry, action widget.Clickable
	list                 widget.List
	focus                bool
	width                unit.Dp
	modal                bool
}

func newAnalysisSheet() analysisSheet {
	return analysisSheet{focus: true, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
}

func (s *analysisSheet) dismissed(gtx layout.Context, extra ...event.Tag) bool {
	if s.focus {
		s.focus = false
		gtx.Execute(key.FocusCmd{Tag: &s.close})
	}
	for _, tag := range append([]event.Tag{&s.close, &s.retry, &s.action, &s.list}, extra...) {
		for {
			// A FocusFilter also matches edit and IME events. Leave those
			// to the focused editor when an action sheet contains a form.
			e, ok := gtx.Event(key.Filter{Focus: tag, Name: key.NameEscape, Optional: key.ModShift},
				key.Filter{Focus: tag, Name: key.NamePageUp, Optional: key.ModShift},
				key.Filter{Focus: tag, Name: key.NamePageDown, Optional: key.ModShift})
			if !ok {
				break
			}
			press := e.(key.Event)
			if press.State != key.Press {
				continue
			}
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
	var footer layout.Widget
	if action != i18n.NoMessage {
		footer = view.Button(&s.action, view.Language.Text(action))
	}
	s.layoutPanel(gtx, view, title, func(gtx layout.Context, _ ui.View) layout.Dimensions { return body(gtx) }, footer, canClose)
}

func (s *analysisSheet) layoutPanel(gtx layout.Context, view ui.View, title string, body func(layout.Context, ui.View) layout.Dimensions, footer layout.Widget, canClose bool) {
	panelWidth := s.width
	if panelWidth == 0 {
		panelWidth = 450
	}
	width := min(gtx.Constraints.Max.X, gtx.Dp(panelWidth))
	backdrop := image.Rect(0, 0, gtx.Constraints.Max.X-width, gtx.Constraints.Max.Y)
	paint.Fill(gtx.Ops, color.NRGBA{A: 0x65})
	mask := clip.Rect(backdrop).Push(gtx.Ops)
	event.Op(gtx.Ops, s)
	mask.Pop()
	view.Origin = image.Pt(backdrop.Max.X+gtx.Dp(16), gtx.Dp(16))
	layout.E.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints = layout.Exact(image.Pt(width, gtx.Constraints.Max.Y))
		panel := func(gtx layout.Context) layout.Dimensions {
			return view.Floating(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					var headHeight int
					rows := []ui.ColumnChild{
						ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							dims := layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(12)}.Layout(gtx,
								layout.Flexed(1, view.Body(ui.TextHeading, title, view.Colors.Ink)),
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									if !canClose {
										gtx = gtx.Disabled()
									}
									return view.Button(&s.close, view.Language.Text(i18n.FieldDismiss))(gtx)
								}))
							headHeight = dims.Size.Y
							return dims
						}),
						ui.Fill(func(gtx layout.Context) layout.Dimensions {
							bodyView := view
							bodyView.Origin.Y += headHeight + gtx.Dp(12)
							return body(gtx, bodyView)
						}),
					}
					if footer != nil {
						rows = append(rows, ui.Rigid(footer))
					}
					return (ui.Column{Gap: 12}).Layout(gtx, rows...)
				})
			})
		}
		if s.modal {
			return accessibility.Modal(gtx, s, title, panel)
		}
		return panel(gtx)
	})
}
