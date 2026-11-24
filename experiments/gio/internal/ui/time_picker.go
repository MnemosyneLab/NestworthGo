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

const clockLayout = "15:04"

func clockParts(value string) (hour, minute int, valid bool) {
	t, err := time.Parse(clockLayout, value)
	if err != nil || t.Format(clockLayout) != value {
		return 0, 0, false
	}
	return t.Hour(), t.Minute(), true
}

func chooseClockPart(value string, isMinute bool, part int) string {
	hour, minute, _ := clockParts(value)
	if isMinute {
		minute = part
	} else {
		hour = part
	}
	return fmt.Sprintf("%02d:%02d", hour, minute)
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

func displayClock(view View, value string, twelveHour bool) string {
	hour, minute, valid := clockParts(value)
	if !valid {
		if value == "" {
			return view.Language.Text(i18n.PickerEmpty)
		}
		return view.Language.Text(i18n.PickerInvalidTime)
	}
	if !twelveHour {
		return value
	}
	return fmt.Sprintf(view.Language.Text(i18n.TimeTwelveDisplay), clockDisplayHour(hour), minute, clockPeriod(view, hour))
}

type TimePicker struct {
	Trigger        widget.Clickable
	hours, minutes optionList
	minuteActive   bool
	clear          widget.Clickable
}

func (p *TimePicker) Layout(gtx layout.Context, view View, field PickerField, value string, twelveHour, clearable bool, change func(string)) {
	_, _, valid := clockParts(value)
	pane := Popover{focus: p, Body: func(gtx layout.Context) layout.Dimensions {
		return p.popup(gtx, view, value, twelveHour, clearable, change)
	}}
	if pickerTrigger(gtx, view, field, &p.Trigger, displayClock(view, value, twelveHour), value != "" && !valid, pane) {
		p.minuteActive = false
		hour, minute, _ := clockParts(value)
		p.hours.scroll.Position, p.minutes.scroll.Position = layout.Position{}, layout.Position{}
		p.hours.setActive(clockChoices(view, 24, twelveHour), clockPartID(hour))
		p.minutes.setActive(clockChoices(view, 60, false), clockPartID(minute))
	}
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

func (p *TimePicker) popup(gtx layout.Context, view View, value string, twelveHour, clearable bool, change func(string)) layout.Dimensions {
	registerPickerFocus(gtx, p)
	hours := clockChoices(view, 24, twelveHour)
	minutes := clockChoices(view, 60, false)
	pick := func(isMinute bool, item Choice) {
		part, _ := strconv.Atoi(item.ID)
		value = chooseClockPart(value, isMinute, part)
		p.minuteActive = isMinute
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
		active, items := &p.hours, hours
		if p.minuteActive {
			active, items = &p.minutes, minutes
		}
		switch e.Name {
		case key.NameLeftArrow:
			p.minuteActive = false
		case key.NameRightArrow:
			p.minuteActive = true
		case key.NameUpArrow:
			active.move(items, -1)
		case key.NameDownArrow:
			active.move(items, 1)
		case key.NameDeleteBackward, key.NameDeleteForward:
			clear()
		default:
			active.accept(items, func(item Choice) { pick(p.minuteActive, item) })
		}
	}

	selectedHour, selectedMinute := "", ""
	if hour, minute, valid := clockParts(value); valid {
		selectedHour, selectedMinute = clockPartID(hour), clockPartID(minute)
	}
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Gap: gtx.Dp(10)}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return clockColumn(gtx, view, i18n.TimeHours, &p.hours, hours, selectedHour, gtx.Focused(p) && !p.minuteActive,
					func(item Choice) { pick(false, item) })
			}),
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return clockColumn(gtx, view, i18n.TimeMinutes, &p.minutes, minutes, selectedMinute, gtx.Focused(p) && p.minuteActive,
					func(item Choice) { pick(true, item) })
			}),
		)
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
