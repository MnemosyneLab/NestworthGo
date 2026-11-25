package desktop

import (
	"fmt"
	"image"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
)

type previewPickers struct {
	account, institution                     ui.Select
	date                                     ui.DatePicker
	clock                                    ui.TimePicker
	accountID, institutionID, day, timeValue string
	sundayFirst, twelveHour                  bool
	lastWeekStart, lastTimeFormat            string
	weekButton, clockButton                  widget.Clickable
}

func previewAccountChoices(view ui.View) []ui.Choice {
	return []ui.Choice{
		{ID: "cash", Label: view.Language.Text(i18n.PreviewCash)},
		{ID: "credit", Label: view.Language.Text(i18n.PreviewCredit), Disabled: true},
		{ID: "brokerage", Label: view.Language.Text(i18n.PreviewBrokerage)},
	}
}

func previewInstitutionChoices(view ui.View) []ui.Choice {
	items := []ui.Choice{
		{ID: "harbor", Label: view.Language.Text(i18n.PreviewHarbor)},
		{ID: "evergreen", Label: view.Language.Text(i18n.PreviewEvergreen), Disabled: true},
		{ID: "westlake", Label: view.Language.Text(i18n.PreviewWestlake)},
	}
	for i := 1; i <= 36; i++ {
		items = append(items, ui.Choice{
			ID:    fmt.Sprintf("harbor-branch-%02d", i),
			Label: fmt.Sprintf(view.Language.Text(i18n.PreviewHarborBranch), i),
		})
	}
	return items
}

func (p *previewPickers) dateOptions(format string) ui.DateOptions {
	weekStart := time.Monday
	if p.sundayFirst {
		weekStart = time.Sunday
	}
	return ui.DateOptions{
		Today: "2024-02-28", Min: "2024-02-10", Max: "2025-03-03",
		Clearable: true, WeekStart: weekStart, Format: format,
	}
}

func (p *componentPreview) pickerFields(gtx layout.Context, view ui.View) layout.Dimensions {
	c := &p.pickers
	if view.TimeFormat != c.lastTimeFormat {
		c.twelveHour, c.lastTimeFormat = view.TimeFormat == "12h", view.TimeFormat
	}
	if view.WeekStart != c.lastWeekStart {
		c.sundayFirst, c.lastWeekStart = view.WeekStart == "sunday", view.WeekStart
	}
	tabs := [...]event.Tag{&p.readOnly.Editor, &c.account.Trigger, &c.institution.Trigger, &c.date.Trigger, &c.clock.Trigger, &p.submit}
	fields := [...]struct {
		label i18n.MessageID
		draw  func(layout.Context, ui.PickerField)
	}{
		{i18n.PreviewAccountType, func(gtx layout.Context, field ui.PickerField) {
			options := ui.SelectOptions{Items: previewAccountChoices(view), Clearable: true}
			c.account.Layout(gtx, view, field, options, c.accountID, func(id string) { c.accountID = id })
		}},
		{i18n.PreviewInstitutionChoice, func(gtx layout.Context, field ui.PickerField) {
			options := ui.SelectOptions{Items: previewInstitutionChoices(view), Clearable: true, Searchable: true}
			c.institution.Layout(gtx, view, field, options, c.institutionID, func(id string) { c.institutionID = id })
		}},
		{i18n.PreviewEffectiveDate, func(gtx layout.Context, field ui.PickerField) {
			c.date.Layout(gtx, view, field, c.dateOptions(view.DateFormat), c.day, func(day string) { c.day = day })
		}},
		{i18n.PreviewEffectiveTime, func(gtx layout.Context, field ui.PickerField) {
			c.clock.Layout(gtx, view, field, c.timeValue, c.twelveHour, true, func(value string) { c.timeValue = value })
		}},
	}
	return previewGrid.Layout(gtx, view, len(fields), func(gtx layout.Context, view ui.View, i int) layout.Dimensions {
		size := image.Pt(gtx.Constraints.Max.X, gtx.Dp(ui.PickerHeight))
		gtx.Constraints = layout.Exact(size)
		fields[i].draw(gtx, ui.PickerField{
			Label: fields[i].label, Origin: view.Origin, Previous: tabs[i], Next: tabs[i+2],
		})
		return layout.Dimensions{Size: size}
	})
}

// triggers lists the picker triggers whose popups follow the scrolled page.
func (p *previewPickers) triggers() []*widget.Clickable {
	return []*widget.Clickable{&p.account.Trigger, &p.institution.Trigger, &p.date.Trigger, &p.clock.Trigger}
}

func (p *previewPickers) resultLines(view ui.View) []string {
	absent := view.Language.Text(i18n.PickerEmpty)
	day, clock := p.day, p.timeValue
	if day == "" {
		day = absent
	}
	if clock == "" {
		clock = absent
	}
	return []string{
		fmt.Sprintf(view.Language.Text(i18n.PreviewAccountResult), ui.ChoiceLabel(view, previewAccountChoices(view), p.accountID)),
		fmt.Sprintf(view.Language.Text(i18n.PreviewInstitutionResult), ui.ChoiceLabel(view, previewInstitutionChoices(view), p.institutionID)),
		fmt.Sprintf(view.Language.Text(i18n.PreviewDateResult), day),
		fmt.Sprintf(view.Language.Text(i18n.PreviewTimeResult), clock),
	}
}
