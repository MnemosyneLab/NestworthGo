package main

import (
	"fmt"
	"image"
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
)

const institutionNameBudget = 48

type componentPreview struct {
	name, notes        textField
	readOnly, disabled textField
	search             searchField
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
	p := &componentPreview{
		name:     newTextField(true),
		notes:    newTextField(false),
		readOnly: newTextField(true),
		disabled: newTextField(true),
		search:   searchField{field: newTextField(true)},
		scroll:   layout.List{Axis: layout.Vertical},
	}
	p.readOnly.editor.SetText("Harbor Bank")
	p.disabled.editor.SetText("Evergreen Credit Union")
	return p
}

func (p *componentPreview) layout(gtx layout.Context, view presentation) layout.Dimensions {
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

func (p *componentPreview) content(gtx layout.Context, view presentation) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx,
		layout.Rigid(view.message(view.language.text(previewFormHelp), view.colors.secondaryInk)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			form := func(gtx layout.Context) layout.Dimensions { return p.form(gtx, view) }
			results := func(gtx layout.Context) layout.Dimensions { return p.results(gtx, view) }
			if gtx.Constraints.Max.X >= gtx.Dp(680) {
				return layout.Flex{Gap: gtx.Dp(18)}.Layout(gtx, layout.Flexed(0.6, form), layout.Flexed(0.4, results))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(16)}.Layout(gtx, layout.Rigid(form), layout.Rigid(results))
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.readOnly.layout(limitWidth(gtx, 460), view, fieldOptions{label: previewReadOnly, readOnly: true})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.disabled.layout(limitWidth(gtx, 460), view, fieldOptions{label: previewDisabled, disabled: true})
		}),
	)
}

func (p *componentPreview) form(gtx layout.Context, view presentation) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(14)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.name.layout(gtx, view, fieldOptions{
				label: previewInstitutionName, hint: previewInstitutionHint,
				maxBytes: institutionNameBudget, validate: p.nameProblem,
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.notes.layout(gtx, view, fieldOptions{label: previewNotes, hint: previewNotesHint, help: previewNotesHelp})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.search.layout(gtx, view, previewSearch, previewSearchHint, func(query string) { p.query = query })
		}),
	)
}

func (p *componentPreview) nameProblem(text string) messageID {
	if p.checkName && strings.TrimSpace(text) == "" {
		return previewNameRequired
	}
	return noMessage
}

func (p *componentPreview) saveSample(gtx layout.Context) {
	p.checkName = true
	name := p.name.text()
	if p.nameProblem(name) != noMessage || len(name) > institutionNameBudget {
		p.name.focus(gtx)
	} else {
		p.submissions++
		p.submittedName, p.submittedNotes = name, p.notes.text()
	}
	gtx.Execute(op.InvalidateCmd{})
}

func (p *componentPreview) results(gtx layout.Context, view presentation) layout.Dimensions {
	return view.panel(gtx, view.colors.divider, func(gtx layout.Context) layout.Dimensions {
		return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			message := func(text string) layout.FlexChild {
				return layout.Rigid(view.message(text, view.colors.ink))
			}
			children := []layout.FlexChild{
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return view.label(gtx, view.language.text(previewResults), 14, font.Medium, view.colors.ink)
				}),
				message(fmt.Sprintf(view.language.text(previewSubmissions), p.submissions)),
			}
			if p.submissions > 0 {
				children = append(children,
					message(fmt.Sprintf(view.language.text(previewSavedName), p.submittedName)),
					message(fmt.Sprintf(view.language.text(previewSavedNotes), p.submittedNotes)),
				)
			}
			for _, line := range p.pickers.resultLines(view) {
				children = append(children, message(line))
			}
			query := strings.ToLower(strings.TrimSpace(p.query))
			if query != "" {
				children = append(children, message(fmt.Sprintf(view.language.text(previewQuery), p.query)))
			}
			found := false
			for _, id := range [...]messageID{previewHarbor, previewEvergreen, previewWestlake} {
				name := view.language.text(id)
				if strings.Contains(strings.ToLower(name), query) {
					children = append(children, message(name))
					found = true
				}
			}
			if !found {
				children = append(children, message(view.language.text(previewNoMatches)))
			}
			return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx, children...)
		})
	})
}

func (p *componentPreview) footer(gtx layout.Context, view presentation, width, y int) {
	if p.pickers.weekButton.Clicked(gtx) {
		p.pickers.sundayFirst = !p.pickers.sundayFirst
		gtx.Execute(op.InvalidateCmd{})
	}
	if p.pickers.clockButton.Clicked(gtx) {
		p.pickers.twelveHour = !p.pickers.twelveHour
		gtx.Execute(op.InvalidateCmd{})
	}
	weekTitle := previewMondayFirst
	if p.pickers.sundayFirst {
		weekTitle = previewSundayFirst
	}
	timeTitle := preview24Hour
	if p.pickers.twelveHour {
		timeTitle = preview12Hour
	}
	buttons := [...]struct {
		button *widget.Clickable
		title  messageID
	}{
		{&p.submit, previewSubmit},
		{&p.pickers.weekButton, weekTitle},
		{&p.pickers.clockButton, timeTitle},
		{&p.info, previewInfo},
	}
	for i, action := range buttons {
		position, size := pickerSlot(gtx, width, y, len(buttons), i)
		offset := op.Offset(position).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(size)
		if action.button == &p.info {
			anchor := view.origin.Add(position)
			view.overlay.register(gtx, action.button, image.Rectangle{Min: anchor, Max: anchor.Add(size)}, popoverPane{
				body: view.message(view.language.text(previewInfoBody), view.colors.ink),
			})
		}
		view.button(action.button, view.language.text(action.title))(gtx)
		offset.Pop()
	}
}
