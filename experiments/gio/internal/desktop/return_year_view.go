package desktop

import (
	"time"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

func (p *returnAnalysisPage) periodDirections(earliest, latest time.Time) (previous, next bool) {
	if p.year {
		return p.month.Year() > earliest.Year(), p.month.Year() < latest.Year()
	}
	return p.month.After(earliest), p.month.Before(latest)
}

func (p *returnAnalysisPage) updateYearSelection(gtx layout.Context, earliest, latest time.Time) {
	if !p.year || p.current == nil {
		return
	}
	first := time.Date(p.month.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	for index := range p.yearCells {
		month := first.AddDate(0, index, 0)
		if !month.Before(earliest) && !month.After(latest) && p.yearCells[index].Clicked(gtx) {
			p.year, p.current = false, nil
			p.readMonth(month, p.sunday)
			gtx.Execute(key.FocusCmd{Tag: &p.monthMode})
			gtx.Execute(op.InvalidateCmd{})
			return
		}
	}
}

func (p *returnAnalysisPage) layoutYear(gtx layout.Context, view ui.View, earliest, latest time.Time) layout.Dimensions {
	l := view.Language
	first := time.Date(p.month.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
	data := make(map[string]application.ReturnCalendarMonthResult, len(p.current.yearMonths))
	for _, row := range p.current.yearMonths {
		data[row.Month] = row
	}
	return (ui.Grid{MinColumn: 140, Gap: 10, MaxColumns: 4, Balanced: true}).Layout(gtx, view, len(p.yearCells),
		func(gtx layout.Context, view ui.View, index int) layout.Dimensions {
			month := first.AddDate(0, index, 0)
			button := &p.yearCells[index]
			if month.Before(earliest) || month.After(latest) {
				gtx = gtx.Disabled()
			}
			name := month.Format(l.Text(i18n.ReturnMonthLayout))
			return returnYearMonth(gtx, view, button, name, data[month.Format("2006-01")])
		})
}

func returnYearMonth(gtx layout.Context, view ui.View, button *widget.Clickable, name string, data application.ReturnCalendarMonthResult) layout.Dimensions {
	l := view.Language
	coverage := l.Text(i18n.ReturnNoData)
	status := i18n.NoMessage
	if data.Coverage.TotalDays > 0 {
		coverage = returnCoverage(l, data.Coverage, application.ReturnDayCoverage{})
		switch data.AmountStatus {
		case application.ReturnAmountPartial:
			status = i18n.OverviewIncomplete
		case application.ReturnAmountUnavailable:
			status = i18n.OverviewNoValue
		}
	}
	value := ""
	if data.ReturnAmount != nil || data.ReturnRate != nil || data.Coverage.TotalDays > 0 {
		value = returnSignedMoney(view, data.ReturnAmount) + " · " + returnRate(view, data.ReturnRate)
	}
	if status != i18n.NoMessage {
		coverage += " · " + l.Text(status)
	}
	description := coverage
	if value != "" {
		description = value + " · " + coverage
	}
	view.Focus.Watch(gtx, button)
	return accessibility.Button(gtx, button, name, description, false,
		func(gtx layout.Context) layout.Dimensions {
			return button.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				semantic.Button.Add(gtx.Ops)
				semantic.LabelOp(name).Add(gtx.Ops)
				border := view.Colors.Divider
				if gtx.Enabled() && (gtx.Focused(button) || button.Hovered()) {
					border = view.Colors.Accent
				}
				return view.Panel(gtx, border, func(gtx layout.Context) layout.Dimensions {
					return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						nameInk := view.Colors.Ink
						if !gtx.Enabled() {
							nameInk = view.Colors.SecondaryInk
						}
						rows := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, name, nameInk))}
						if value != "" {
							rows = append(rows, ui.Rigid(view.Message(value, view.Colors.Ink)))
						}
						rows = append(rows, ui.Rigid(view.Message(coverage, view.Colors.SecondaryInk)))
						return (ui.Column{Gap: 6}).Layout(gtx, rows...)
					})
				})
			})
		})
}
