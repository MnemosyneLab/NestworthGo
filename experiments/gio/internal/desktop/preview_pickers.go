package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type previewPickers struct {
	account, institution                     selectControl
	date                                     datePicker
	clock                                    timePicker
	accountID, institutionID, day, timeValue string
	sundayFirst, twelveHour                  bool
	weekButton, clockButton                  widget.Clickable
}

func previewAccountChoices(view presentation) []choice {
	return []choice{
		{id: "cash", label: view.language.Text(i18n.PreviewCash)},
		{id: "credit", label: view.language.Text(i18n.PreviewCredit), disabled: true},
		{id: "brokerage", label: view.language.Text(i18n.PreviewBrokerage)},
	}
}

func previewInstitutionChoices(view presentation) []choice {
	items := []choice{
		{id: "harbor", label: view.language.Text(i18n.PreviewHarbor)},
		{id: "evergreen", label: view.language.Text(i18n.PreviewEvergreen), disabled: true},
		{id: "westlake", label: view.language.Text(i18n.PreviewWestlake)},
	}
	for i := 1; i <= 36; i++ {
		items = append(items, choice{
			id:    fmt.Sprintf("harbor-branch-%02d", i),
			label: fmt.Sprintf(view.language.Text(i18n.PreviewHarborBranch), i),
		})
	}
	return items
}

func (p *previewPickers) dateOptions() dateOptions {
	weekStart := time.Monday
	if p.sundayFirst {
		weekStart = time.Sunday
	}
	return dateOptions{
		today: "2024-02-28", min: "2024-02-10", max: "2025-03-03",
		clearable: true, weekStart: weekStart, format: "localized",
	}
}

func (p *componentPreview) pickerFields(gtx layout.Context, view presentation, width, y, columns int) {
	c := &p.pickers
	tabs := [...]event.Tag{&p.readOnly.editor, &c.account.trigger, &c.institution.trigger, &c.date.trigger, &c.clock.trigger, &p.submit}
	fields := [...]struct {
		label i18n.MessageID
		draw  func(layout.Context, pickerField)
	}{
		{i18n.PreviewAccountType, func(gtx layout.Context, field pickerField) {
			options := selectOptions{items: previewAccountChoices(view), clearable: true}
			c.account.layout(gtx, view, field, options, c.accountID, func(id string) { c.accountID = id })
		}},
		{i18n.PreviewInstitutionChoice, func(gtx layout.Context, field pickerField) {
			options := selectOptions{items: previewInstitutionChoices(view), clearable: true, searchable: true}
			c.institution.layout(gtx, view, field, options, c.institutionID, func(id string) { c.institutionID = id })
		}},
		{i18n.PreviewEffectiveDate, func(gtx layout.Context, field pickerField) {
			c.date.layout(gtx, view, field, c.dateOptions(), c.day, func(day string) { c.day = day })
		}},
		{i18n.PreviewEffectiveTime, func(gtx layout.Context, field pickerField) {
			c.clock.layout(gtx, view, field, c.timeValue, c.twelveHour, true, func(value string) { c.timeValue = value })
		}},
	}
	for i, field := range fields {
		position, size := pickerSlot(gtx, width, y, columns, i)
		offset := op.Offset(position).Push(gtx.Ops)
		gtx.Constraints = layout.Exact(size)
		field.draw(gtx, pickerField{
			label: field.label, origin: view.origin.Add(position), previous: tabs[i], next: tabs[i+2],
		})
		offset.Pop()
	}
}

func pickerSlot(gtx layout.Context, width, y, columns, index int) (image.Point, image.Point) {
	gap := gtx.Dp(10)
	column := index % columns
	cellWidth := max(0, (width-(columns-1)*gap)/columns)
	x := column * (cellWidth + gap)
	return image.Pt(x, y+gtx.Dp(68)*(index/columns)), image.Pt(cellWidth, gtx.Dp(40))
}

func (p *previewPickers) resultLines(view presentation) []string {
	absent := view.language.Text(i18n.PickerEmpty)
	day, clock := p.day, p.timeValue
	if day == "" {
		day = absent
	}
	if clock == "" {
		clock = absent
	}
	return []string{
		fmt.Sprintf(view.language.Text(i18n.PreviewAccountResult), choiceLabel(view, previewAccountChoices(view), p.accountID)),
		fmt.Sprintf(view.language.Text(i18n.PreviewInstitutionResult), choiceLabel(view, previewInstitutionChoices(view), p.institutionID)),
		fmt.Sprintf(view.language.Text(i18n.PreviewDateResult), day),
		fmt.Sprintf(view.language.Text(i18n.PreviewTimeResult), clock),
	}
}
