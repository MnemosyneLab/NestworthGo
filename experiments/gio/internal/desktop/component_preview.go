package desktop

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

const institutionNameBudget = 48

type componentPreview struct {
	name, notes        ui.TextField
	readOnly, disabled ui.TextField
	search             ui.SearchField
	query              string
	submit, info       widget.Clickable
	scroll             layout.List
	pickers            previewPickers
	checkName          bool
	submissions        int
	submittedName      string
	submittedNotes     string
}

func newComponentPreview() *componentPreview {
	return &componentPreview{
		name:     ui.NewTextField(true),
		notes:    ui.NewTextField(false),
		readOnly: ui.NewTextField(true),
		disabled: ui.NewTextField(true),
		search:   ui.SearchField{Field: ui.NewTextField(true)},
		scroll:   layout.List{Axis: layout.Vertical},
	}
}

func (p *componentPreview) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	setPreviewExample(&p.readOnly, view.Language.Text(i18n.PreviewHarbor))
	setPreviewExample(&p.disabled, view.Language.Text(i18n.PreviewEvergreen))

	size := gtx.Constraints.Max
	footerHeight := gtx.Dp(40)
	footerY := max(0, size.Y-footerHeight)
	columns := 4
	if size.X < gtx.Dp(640) {
		columns = 2
	}
	pickerY := max(0, footerY-gtx.Dp(68)*(4/columns))
	body := gtx
	body.Constraints = layout.Exact(image.Pt(size.X, max(0, pickerY-gtx.Dp(12))))
	p.scroll.Layout(body, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return p.content(gtx, view)
	})

	// Read the edited fields after the body has delivered this frame's input.
	if p.submit.Clicked(gtx) {
		p.saveSample(gtx)
	}
	p.pickerFields(gtx, view, size.X, pickerY, columns)
	p.footer(gtx, view, size.X, footerY)
	return layout.Dimensions{Size: size}
}

func setPreviewExample(field *ui.TextField, text string) {
	if field.Text() == text {
		return
	}
	start, end := field.Editor.Selection()
	field.Editor.SetText(text)
	length := field.Editor.Len()
	field.Editor.SetCaret(min(start, length), min(end, length))
}

func (p *componentPreview) content(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx,
		layout.Rigid(view.Message(view.Language.Text(i18n.PreviewFormHelp), view.Colors.SecondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			form := func(gtx layout.Context) layout.Dimensions { return p.form(gtx, view) }
			results := func(gtx layout.Context) layout.Dimensions { return p.results(gtx, view) }
			if gtx.Constraints.Max.X >= gtx.Dp(680) {
				return layout.Flex{Gap: gtx.Dp(18)}.Layout(gtx, layout.Flexed(0.6, form), layout.Flexed(0.4, results))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx, layout.Rigid(form), layout.Rigid(results))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.readOnly.Layout(ui.LimitWidth(gtx, 460), view, ui.FieldOptions{Label: i18n.PreviewReadOnly, ReadOnly: true})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.disabled.Layout(ui.LimitWidth(gtx, 460), view, ui.FieldOptions{Label: i18n.PreviewDisabled, Disabled: true})
		}),
	)
}

func (p *componentPreview) form(gtx layout.Context, view ui.View) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.name.Layout(gtx, view, ui.FieldOptions{
				Label: i18n.PreviewInstitutionName, Hint: i18n.PreviewInstitutionHint,
				MaxBytes: institutionNameBudget,
				Problem:  func(value string) string { return view.Language.Text(p.nameProblem(value)) },
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.notes.Layout(gtx, view, ui.FieldOptions{Label: i18n.PreviewNotes, Hint: i18n.PreviewNotesHint, Help: i18n.PreviewNotesHelp})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.search.Layout(gtx, view, i18n.PreviewSearch, i18n.PreviewSearchHint, func(query string) { p.query = query })
		}),
	)
}

func (p *componentPreview) nameProblem(text string) i18n.MessageID {
	if p.checkName && strings.TrimSpace(text) == "" {
		return i18n.PreviewNameRequired
	}
	return i18n.NoMessage
}

func (p *componentPreview) saveSample(gtx layout.Context) {
	p.checkName = true
	name := p.name.Text()
	if p.nameProblem(name) != i18n.NoMessage || len(name) > institutionNameBudget {
		p.name.Focus(gtx)
	} else {
		p.submissions++
		p.submittedName, p.submittedNotes = name, p.notes.Text()
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *componentPreview) results(gtx layout.Context, view ui.View) layout.Dimensions {
	return view.Panel(gtx, view.Colors.Divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message := func(text string) layout.FlexChild {
				return layout.Rigid(view.Message(text, view.Colors.Ink))
			}
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.Label(gtx, view.Language.Text(i18n.PreviewResults), 14, font.Medium, view.Colors.Ink)
				}),
				message(fmt.Sprintf(view.Language.Text(i18n.PreviewSubmissions), p.submissions)),
			}
			if p.submissions > 0 {
				children = append(children,
					message(fmt.Sprintf(view.Language.Text(i18n.PreviewSavedName), p.submittedName)),
					message(fmt.Sprintf(view.Language.Text(i18n.PreviewSavedNotes), p.submittedNotes)),
				)
			}
			for _, line := range p.pickers.resultLines(view) {
				children = append(children, message(line))
			}
			query := strings.ToLower(strings.TrimSpace(p.query))
			if query != "" {
				children = append(children, message(fmt.Sprintf(view.Language.Text(i18n.PreviewQuery), p.query)))
			}
			found := false
			for _, id := range [...]i18n.MessageID{i18n.PreviewHarbor, i18n.PreviewEvergreen, i18n.PreviewWestlake} {
				name := view.Language.Text(id)
				if strings.Contains(strings.ToLower(name), query) {
					children = append(children, message(name))
					found = true
				}
			}
			if !found {
				children = append(children, message(view.Language.Text(i18n.PreviewNoMatches)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx, children...)
		})
	})
}

func (p *componentPreview) footer(gtx layout.Context, view ui.View, width, y int) {
	if p.pickers.weekButton.Clicked(gtx) {
		p.pickers.sundayFirst = !p.pickers.sundayFirst
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.pickers.clockButton.Clicked(gtx) {
		p.pickers.twelveHour = !p.pickers.twelveHour
		gtx.Execute(op.InvalidateCmd{})
	}
	weekTitle := i18n.PreviewMondayFirst
	if p.pickers.sundayFirst {
		weekTitle = i18n.PreviewSundayFirst
	}
	timeTitle := i18n.Preview24Hour
	if p.pickers.twelveHour {
		timeTitle = i18n.Preview12Hour
	}
	buttons := [...]struct {
		button *widget.Clickable
		title  i18n.MessageID
	}{
		{&p.submit, i18n.PreviewSubmit},
		{&p.pickers.weekButton, weekTitle},
		{&p.pickers.clockButton, timeTitle},
		{&p.info, i18n.PreviewInfo},
	}
	for i, action := range buttons {
		position, size := pickerSlot(gtx, width, y, len(buttons), i)
		offset := op.Offset(position).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(size)
		if action.button == &p.info {
			anchor := view.Origin.Add(position)
			view.Overlay.Register(gtx, action.button, image.Rectangle{Min: anchor, Max: anchor.Add(size)}, ui.Popover{
				Body: view.Message(view.Language.Text(i18n.PreviewInfoBody), view.Colors.Ink),
			})
		}
		view.Button(action.button, view.Language.Text(action.title))(gtx)
		offset.Pop()
	}
}
