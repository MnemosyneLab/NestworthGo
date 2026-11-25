package desktop

import (
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
)

type analysisDateWindow struct{ from, to string }

type analysisDatePreset struct {
	short, label i18n.MessageID
	days, months int
}

var analysisDatePresets = [...]analysisDatePreset{
	{short: i18n.AnalysisDayShort, label: i18n.AnalysisLastDay, days: 1},
	{short: i18n.AnalysisWeekShort, label: i18n.AnalysisLastWeek, days: 7},
	{short: i18n.AnalysisMonthShort, label: i18n.AnalysisLastMonth, months: 1},
	{short: i18n.AnalysisQuarterShort, label: i18n.AnalysisLastQuarter, months: 3},
	{short: i18n.AnalysisHalfYearShort, label: i18n.AnalysisLastHalfYear, months: 6},
	{short: i18n.AnalysisYearShort, label: i18n.AnalysisLastYear, months: 12},
	{short: i18n.AnalysisAllDates, label: i18n.AnalysisWholeHistory},
}

const analysisAllDates = len(analysisDatePresets) - 1

type analysisDateShortcuts struct {
	presets        [len(analysisDatePresets)]widget.Clickable
	previous, next widget.Clickable
	chosen         int
	at             analysisDateWindow
}

func analysisPresetRange(index int, to, earliest string) analysisDateWindow {
	end, ok := ui.ParseCivilDay(to)
	if !ok {
		return analysisDateWindow{}
	}
	from := earliest
	preset := analysisDatePresets[index]
	switch {
	case preset.days > 0:
		from = end.AddDate(0, 0, 1-preset.days).Format(time.DateOnly)
	case preset.months > 0:
		from = analysisShiftMonth(end, -preset.months).AddDate(0, 0, 1).Format(time.DateOnly)
	}
	return analysisDateWindow{from: max(earliest, from), to: to}
}

func analysisShiftMonth(day time.Time, months int) time.Time {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, months, 0)
	return first.AddDate(0, 0, min(day.Day(), first.AddDate(0, 1, -1).Day())-1)
}

func analysisShiftRange(window analysisDateWindow, direction, preset int) analysisDateWindow {
	from, fromOK := ui.ParseCivilDay(window.from)
	to, toOK := ui.ParseCivilDay(window.to)
	if !fromOK || !toOK || from.After(to) {
		return analysisDateWindow{}
	}
	if preset >= 0 && preset != analysisAllDates {
		adjacent := analysisDatePresets[preset]
		if direction < 0 {
			end := from.AddDate(0, 0, -1)
			if adjacent.days > 0 {
				return analysisPresetRange(preset, end.Format(time.DateOnly), "")
			}
			return analysisDateWindow{analysisShiftMonth(from, -adjacent.months).Format(time.DateOnly), end.Format(time.DateOnly)}
		}
		start := to.AddDate(0, 0, 1)
		end := start.AddDate(0, 0, adjacent.days-1)
		if adjacent.months > 0 {
			end = analysisShiftMonth(start, adjacent.months).AddDate(0, 0, -1)
		}
		return analysisDateWindow{start.Format(time.DateOnly), end.Format(time.DateOnly)}
	}
	days := int((to.Unix()-from.Unix())/86400+1) * direction
	return analysisDateWindow{from.AddDate(0, 0, days).Format(time.DateOnly), to.AddDate(0, 0, days).Format(time.DateOnly)}
}

func (p *analysisDateShortcuts) active(current analysisDateWindow, earliest string) int {
	if p.at.from != "" && p.at == current {
		return p.chosen
	}
	if current.to != "" {
		for index := range analysisDatePresets {
			if analysisPresetRange(index, current.to, earliest) == current {
				return index
			}
		}
	}
	return -1
}

func (p *analysisDateShortcuts) layout(gtx layout.Context, view ui.View, current analysisDateWindow, earliest, latest string, change func(analysisDateWindow)) layout.Dimensions {
	active := p.active(current, earliest)
	back := analysisShiftRange(current, -1, active)
	forward := analysisShiftRange(current, 1, active)
	canBack, canForward := active != analysisAllDates && back.to >= earliest && back.to != "",
		active != analysisAllDates && forward.from <= latest && forward.from != ""
	selectRange := func(next analysisDateWindow, preset int) {
		next.from, next.to = max(next.from, earliest), min(next.to, latest)
		p.chosen, p.at = preset, next
		change(next)
	}
	for index := range p.presets {
		if p.presets[index].Clicked(gtx) {
			selectRange(analysisPresetRange(index, latest, earliest), index)
			active = index
		}
	}
	if p.previous.Clicked(gtx) && canBack {
		selectRange(back, active)
	}
	if p.next.Clicked(gtx) && canForward {
		selectRange(forward, active)
	}
	l := view.Language
	widgets := make([]layout.Widget, 0, len(p.presets)+2)
	for index, preset := range analysisDatePresets {
		widgets = append(widgets, view.ButtonOf(&p.presets[index], ui.ButtonSpec{
			Variant: ui.ButtonOutline, Size: ui.ButtonSmall, Title: l.Text(preset.short), Name: l.Text(preset.label),
			Toggle: true, Selected: index == active,
		}))
	}
	widgets = append(widgets,
		returnMonthButton(view, &p.previous, i18n.AnalysisPreviousRange, ui.Glyph(lucide.ChevronLeft), canBack),
		returnMonthButton(view, &p.next, i18n.AnalysisNextRange, ui.Glyph(lucide.ChevronRight), canForward))
	return (ui.Wrap{Gap: 7}).Layout(gtx, len(widgets), func(gtx layout.Context, index int) layout.Dimensions {
		return widgets[index](gtx)
	})
}
