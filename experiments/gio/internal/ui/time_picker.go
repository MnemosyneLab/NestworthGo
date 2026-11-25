package ui

import (
	"fmt"
	"strconv"
	"time"

	"gioui.org/font"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func clockParts(value, format string) ([3]int, bool) {
	t, err := time.Parse(format, value)
	if err != nil || t.Format(format) != value {
		return [3]int{}, false
	}
	return [3]int{t.Hour(), t.Minute(), t.Second()}, true
}

func chooseClockPart(value, format string, column, part int) string {
	parts, _ := clockParts(value, format)
	parts[column] = part
	return time.Date(0, 1, 1, parts[0], parts[1], parts[2], 0, time.UTC).Format(format)
}

func clockPeriod(view View, hour int) string {
	if hour < 12 {
		return view.Language.Text(i18n.TimeAM)
	}
	return view.Language.Text(i18n.TimePM)
}

func clockDisplayHour(hour int) int {
	return (hour+11)%12 + 1
}

func displayClock(view View, value, format string, twelveHour bool) string {
	parts, valid := clockParts(value, format)
	if !valid {
		if value == "" {
			return view.Language.Text(i18n.PickerEmpty)
		}
		return view.Language.Text(i18n.PickerInvalidTime)
	}
	if !twelveHour {
		return value
	}
	hour, minute, second := parts[0], parts[1], parts[2]
	if format == time.TimeOnly {
		return fmt.Sprintf(view.Language.Text(i18n.TimeTwelveSecondsDisplay), clockDisplayHour(hour), minute, second, clockPeriod(view, hour))
	}
	return fmt.Sprintf(view.Language.Text(i18n.TimeTwelveDisplay), clockDisplayHour(hour), minute, clockPeriod(view, hour))
}

type TimePicker struct {
	Trigger widget.Clickable
	columns [3]optionList
	active  int
	clear   widget.Clickable
}

func (p *TimePicker) Layout(gtx layout.Context, view View, field PickerField, value string, twelveHour, clearable bool, change func(string)) {
	p.layout(gtx, view, field, value, "15:04", twelveHour, clearable, change)
}

func (p *TimePicker) LayoutSeconds(gtx layout.Context, view View, field PickerField, value string, twelveHour bool, change func(string)) {
	p.layout(gtx, view, field, value, time.TimeOnly, twelveHour, false, change)
}

func (p *TimePicker) layout(gtx layout.Context, view View, field PickerField, value, format string, twelveHour, clearable bool, change func(string)) {
	parts, valid := clockParts(value, format)
	pane := Popover{focus: p, Body: func(gtx layout.Context) layout.Dimensions {
		return p.popup(gtx, view, value, format, timeChoices(view, format, twelveHour), clearable, change)
	}}
	if pickerTrigger(gtx, view, field, &p.Trigger, displayClock(view, value, format, twelveHour), value != "" && !valid, pane) {
		p.active = 0
		for i, items := range timeChoices(view, format, twelveHour) {
			p.columns[i].scroll.Position = layout.Position{}
			p.columns[i].setActive(items, clockPartID(parts[i]), view.Overlay.ByKeyboard)
		}
	}
}

func timeChoices(view View, format string, twelveHour bool) [][]Choice {
	minutes := clockChoices(view, 60, false)
	choices := [][]Choice{clockChoices(view, 24, twelveHour), minutes}
	if format == time.TimeOnly {
		choices = append(choices, minutes)
	}
	return choices
}

func clockPartID(part int) string {
	return fmt.Sprintf("%02d", part)
}

func clockChoices(view View, count int, twelveHour bool) []Choice {
	items := make([]Choice, count)
	for i := range items {
		id := clockPartID(i)
		label := id
		if twelveHour {
			label = fmt.Sprintf(view.Language.Text(i18n.TimeHourDisplay), clockDisplayHour(i), clockPeriod(view, i))
		}
		items[i] = Choice{ID: id, Label: label}
	}
	return items
}

func (p *TimePicker) popup(gtx layout.Context, view View, value, format string, choices [][]Choice, clearable bool, change func(string)) layout.Dimensions {
	registerPickerFocus(gtx, p)
	pick := func(column int, item Choice) {
		part, _ := strconv.Atoi(item.ID)
		value = chooseClockPart(value, format, column, part)
		p.active = column
		change(value)
		gtx.Execute(op.InvalidateCmd{})
	}
	clear := func() {
		if clearable && value != "" {
			change("")
			view.Overlay.Close()
		}
	}
	for _, e := range PressedKeys(gtx, p, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow,
		key.NameReturn, key.NameEnter, key.NameSpace, key.NameDeleteBackward, key.NameDeleteForward) {
		active, items := &p.columns[p.active], choices[p.active]
		switch e.Name {
		case key.NameLeftArrow:
			p.active = max(0, p.active-1)
		case key.NameRightArrow:
			p.active = min(len(choices)-1, p.active+1)
		case key.NameUpArrow:
			active.move(items, -1)
		case key.NameDownArrow:
			active.move(items, 1)
		case key.NameDeleteBackward, key.NameDeleteForward:
			clear()
		default:
			active.accept(items, func(item Choice) { pick(p.active, item) })
		}
	}

	parts, valid := clockParts(value, format)
	titles := [...]i18n.MessageID{i18n.TimeHours, i18n.TimeMinutes, i18n.TimeSeconds}
	columns := make([]layout.FlexChild, 0, len(choices))
	for i, items := range choices {
		selected := ""
		if valid {
			selected = clockPartID(parts[i])
		}
		columns = append(columns, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return clockColumn(gtx, view, titles[i], &p.columns[i], items, selected, gtx.Focused(p) && p.active == i,
				func(item Choice) { pick(i, item) })
		}))
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(10)}.Layout(gtx, columns...)
	})}
	if clearable && value != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.clear.Clicked(gtx) {
				clear()
			}
			return view.Button(&p.clear, view.Language.Text(i18n.PickerClear))(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx, children...)
}

func clockColumn(gtx layout.Context, view View, title i18n.MessageID, list *optionList, items []Choice, selected string, active bool, pick func(Choice)) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(4)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return view.Label(gtx, view.Language.Text(title), 13, font.Medium, view.Colors.Ink)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.Y = max(0, gtx.Constraints.Max.Y-gtx.Dp(36))
			return list.layout(gtx, view, items, selected, active, pick)
		}),
	)
}
