package desktop

import (
	"image"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
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
	// showRange writes the selected dates between the arrows. Leave it off
	// where date fields beside the control already show them.
	showRange bool
	// pinnedPreset, when pinned is set, stays highlighted even if the dates
	// also match an earlier shortcut. Overview pins All history. Leave pinned
	// unset and the shortcut is inferred from the dates.
	pinned       bool
	pinnedPreset int
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
	if p.pinned {
		return p.pinnedPreset
	}
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
	track := func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(analysisDatePresets), func(gtx layout.Context, index int) layout.Dimensions {
			preset := analysisDatePresets[index]
			return view.TabSegmentNamed(&p.presets[index], l.Text(preset.short), l.Text(preset.label), index == active)(gtx)
		})
	}
	nav := func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min = image.Point{}
		return view.Surface(gtx, ui.SurfaceSpec{Radius: ui.RadiusLG}, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = image.Point{}
			return layout.UniformInset(ui.Space2).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				children := []layout.FlexChild{layout.Rigid(analysisDateArrow(view, &p.previous, i18n.AnalysisPreviousRange, lucide.ChevronLeft, canBack))}
				if p.showRange {
					children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Left: ui.Space4, Right: ui.Space4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return view.Line(gtx, ui.TextCaption, analysisDateRangeText(view, current, active), view.Colors.SecondaryInk)
						})
					}))
				}
				children = append(children, layout.Rigid(analysisDateArrow(view, &p.next, i18n.AnalysisNextRange, lucide.ChevronRight, canForward)))
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
			})
		})
	}
	return analysisDateRow(gtx, track, nav)
}

// analysisDateRow places the shortcut track and the range navigator on one
// line, the navigator at the far end, as the Wails control does. When they do
// not fit the navigator wraps under the track.
func analysisDateRow(gtx layout.Context, track, nav layout.Widget) layout.Dimensions {
	width := gtx.Constraints.Max.X
	gap := gtx.Dp(ui.Space12)
	gtx.Constraints.Min = image.Point{}
	place := func(widget layout.Widget) (layout.Dimensions, op.CallOp) {
		macro := op.Record(gtx.Ops)
		dims := widget(gtx)
		return dims, macro.Stop()
	}
	trackDims, trackCall := place(track)
	navDims, navCall := place(nav)
	trackAt, navAt := image.Point{}, image.Pt(0, trackDims.Size.Y+gap)
	size := image.Pt(max(trackDims.Size.X, navDims.Size.X), navAt.Y+navDims.Size.Y)
	if trackDims.Size.X+gap+navDims.Size.X <= width {
		height := max(trackDims.Size.Y, navDims.Size.Y)
		size = image.Pt(width, height)
		trackAt = image.Pt(0, (height-trackDims.Size.Y)/2)
		navAt = image.Pt(width-navDims.Size.X, (height-navDims.Size.Y)/2)
	}
	stack := op.Offset(trackAt).Push(gtx.Ops)
	trackCall.Add(gtx.Ops)
	stack.Pop()
	stack = op.Offset(navAt).Push(gtx.Ops)
	navCall.Add(gtx.Ops)
	stack.Pop()
	return layout.Dimensions{Size: size}
}

// analysisDateArrow is one icon button of the range navigator.
func analysisDateArrow(view ui.View, button *widget.Clickable, label i18n.MessageID, icon *lucide.Icon, enabled bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if !enabled {
			gtx = gtx.Disabled()
		}
		text := view.Language.Text(label)
		return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonGhost, Size: ui.ButtonCompact, Name: text, ShortName: text, Icon: ui.Glyph(icon)})(gtx)
	}
}

// analysisDateRangeText writes the selected dates for the navigator.
func analysisDateRangeText(view ui.View, window analysisDateWindow, active int) string {
	if active == analysisAllDates {
		return view.Language.Text(i18n.AnalysisWholeHistory)
	}
	return wealthDay(view, window.from) + " – " + wealthDay(view, window.to)
}
