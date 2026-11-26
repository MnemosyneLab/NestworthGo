package ui

import (
	"fmt"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"image"
	"time"

	"gioui.org/io/key"
	"gioui.org/io/semantic"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/accessibility"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

func ParseCivilDay(value string) (time.Time, bool) {
	day, err := time.Parse(time.DateOnly, value)
	return day, err == nil && day.Year() > 0 && day.Format(time.DateOnly) == value
}

type DateOptions struct {
	Today, Min, Max string
	AllowFuture     bool
	Clearable       bool
	WeekStart       time.Weekday
	Format          string
}

func (o DateOptions) limits() (string, string) {
	lo, hi := "0001-01-01", "9999-12-31"
	if _, valid := ParseCivilDay(o.Min); valid {
		lo = o.Min
	}
	if _, valid := ParseCivilDay(o.Max); valid {
		hi = o.Max
	} else if !o.AllowFuture {
		if _, valid := ParseCivilDay(o.Today); valid {
			hi = o.Today
		} else {
			hi = ""
		}
	}
	return lo, hi
}

func (o DateOptions) permits(day string) bool {
	lo, hi := o.limits()
	_, valid := ParseCivilDay(day)
	return valid && lo <= day && day <= hi
}

func (o DateOptions) start(value string) time.Time {
	if o.permits(value) {
		day, _ := ParseCivilDay(value)
		return day
	}
	lo, hi := o.limits()
	start := lo
	if _, valid := ParseCivilDay(o.Today); valid {
		start = max(o.Today, lo)
	}
	if hi != "" {
		start = min(start, hi)
	}
	day, _ := ParseCivilDay(start)
	return day
}

func displayCivil(day time.Time, language i18n.Language) string {
	return day.Format(language.Text(i18n.DateDayLayout))
}

func FormatDay(day time.Time, language i18n.Language, format string) string {
	switch format {
	case "localized":
		return displayCivil(day, language)
	case "day-first":
		return day.Format("02/01/2006")
	case "month-first":
		return day.Format("01/02/2006")
	default:
		return day.Format(time.DateOnly)
	}
}

func monthStart(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func leadingDays(month time.Time, weekStart time.Weekday) int {
	return (int(month.Weekday()) - int(weekStart) + 7) % 7
}

type DatePicker struct {
	Trigger widget.Clickable
	month   time.Time
	day     time.Time
	cells   [42]widget.Clickable
	moves   [4]widget.Clickable
	clear   widget.Clickable
}

func (p *DatePicker) Layout(gtx layout.Context, view View, field PickerField, options DateOptions, value string, change func(string)) {
	title := view.Language.Text(i18n.PickerEmpty)
	invalid := value != "" && !options.permits(value)
	if invalid {
		title = view.Language.Text(i18n.PickerInvalidDate)
	} else if value != "" {
		day, _ := ParseCivilDay(value)
		title = FormatDay(day, view.Language, options.Format)
	}

	pane := Popover{width: 340, focus: p, Body: func(gtx layout.Context) layout.Dimensions {
		return p.popup(gtx, view, options, value, change)
	}}
	if pickerTrigger(gtx, view, field, &p.Trigger, title, invalid, pane) {
		p.day = options.start(value)
		p.month = monthStart(p.day)
	}
}

func (p *DatePicker) moveDay(options DateOptions, delta int) {
	day := p.day.AddDate(0, 0, delta)
	if options.permits(day.Format(time.DateOnly)) {
		p.day, p.month = day, monthStart(day)
	}
}

func (p *DatePicker) targetMonth(options DateOptions, delta int) (time.Time, bool) {
	month := p.month.AddDate(0, delta, 0)
	first, last := month.Format(time.DateOnly), month.AddDate(0, 1, -1).Format(time.DateOnly)
	lo, hi := options.limits()
	return month, month.Year() > 0 && month.Year() <= 9999 && first <= hi && last >= lo
}

func (p *DatePicker) moveMonth(options DateOptions, delta int) {
	if month, ok := p.targetMonth(options, delta); ok {
		lo, hi := options.limits()
		p.month = month
		day, _ := ParseCivilDay(min(max(month.Format(time.DateOnly), lo), hi))
		p.day = day
	}
}

func (p *DatePicker) popup(gtx layout.Context, view View, options DateOptions, value string, change func(string)) layout.Dimensions {
	registerPickerFocus(gtx, p)
	choose := func(day string) {
		if (day == "" && options.Clearable) || options.permits(day) {
			change(day)
			view.Overlay.Close()
		}
	}
	for _, e := range PressedKeys(gtx, p, key.NameLeftArrow, key.NameRightArrow, key.NameUpArrow, key.NameDownArrow,
		key.NamePageUp, key.NamePageDown, key.NameReturn, key.NameEnter, key.NameSpace, key.NameDeleteBackward, key.NameDeleteForward) {
		page := 1
		if e.Modifiers.Contain(key.ModShift) {
			page = 12
		}
		switch e.Name {
		case key.NameLeftArrow:
			p.moveDay(options, -1)
		case key.NameRightArrow:
			p.moveDay(options, 1)
		case key.NameUpArrow:
			p.moveDay(options, -7)
		case key.NameDownArrow:
			p.moveDay(options, 7)
		case key.NamePageUp:
			p.moveMonth(options, -page)
		case key.NamePageDown:
			p.moveMonth(options, page)
		case key.NameDeleteBackward, key.NameDeleteForward:
			choose("")
		default:
			choose(p.day.Format(time.DateOnly))
		}
	}

	rowHeight := min(gtx.Dp(34), max(0, (gtx.Constraints.Max.Y-gtx.Dp(86))/6))
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return p.caption(gtx, view, options) }),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return weekdayRow(gtx, view, options.WeekStart) }),
	}
	for row := range 6 {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.dayRow(gtx, view, options, value, row, rowHeight, choose)
		}))
	}
	if options.Clearable && value != "" {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if p.clear.Clicked(gtx) {
				choose("")
			}
			return view.Button(&p.clear, view.Language.Text(i18n.PickerClear))(gtx)
		}))
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(2)}.Layout(gtx, children...)
}

var (
	firstPageIcon    = Glyph(lucide.ChevronsLeft)
	chevronLeftIcon  = Glyph(lucide.ChevronLeft)
	chevronRightIcon = Glyph(lucide.ChevronRight)
	lastPageIcon     = Glyph(lucide.ChevronsRight)
)

var monthNavigation = [...]struct {
	icon  Glyph
	label i18n.MessageID
	step  int
}{
	{firstPageIcon, i18n.DatePreviousYear, -12},
	{chevronLeftIcon, i18n.DatePreviousMonth, -1},
	{chevronRightIcon, i18n.DateNextMonth, 1},
	{lastPageIcon, i18n.DateNextYear, 12},
}

func (p *DatePicker) caption(gtx layout.Context, view View, options DateOptions) layout.Dimensions {
	nav := func(i int) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			action := monthNavigation[i]
			_, enabled := p.targetMonth(options, action.step)
			if enabled && p.moves[i].Clicked(gtx) {
				p.moveMonth(options, action.step)
			}
			if !enabled {
				gtx = gtx.Disabled()
			}
			return view.ButtonOf(&p.moves[i], ButtonSpec{
				Variant: ButtonGhost, Size: ButtonCompact, Name: view.Language.Text(action.label), Icon: action.icon,
			})(gtx)
		})
	}
	return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(2)}.Layout(gtx,
		nav(0), nav(1),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			title := p.month.Format(view.Language.Text(i18n.DateMonthLayout))
			return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return view.Line(gtx, TextLabel, title, view.Colors.Ink)
			})
		}),
		nav(2), nav(3),
	)
}

func weekdayRow(gtx layout.Context, view View, start time.Weekday) layout.Dimensions {
	labels := [...]i18n.MessageID{i18n.WeekdaySunday, i18n.WeekdayMonday, i18n.WeekdayTuesday, i18n.WeekdayWednesday, i18n.WeekdayThursday, i18n.WeekdayFriday, i18n.WeekdaySaturday}
	return calendarRow(gtx, gtx.Dp(20), func(gtx layout.Context, column int) layout.Dimensions {
		return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, TextCaption, view.Language.Text(labels[(int(start)+column)%7]), view.Colors.SecondaryInk)
		})
	})
}

func (p *DatePicker) dayRow(gtx layout.Context, view View, options DateOptions, value string, row, height int, choose func(string)) layout.Dimensions {
	return calendarRow(gtx, height, func(gtx layout.Context, column int) layout.Dimensions {
		index := row*7 + column
		day := p.month.AddDate(0, 0, index-leadingDays(p.month, options.WeekStart))
		if day.Month() != p.month.Month() {
			return layout.Dimensions{Size: gtx.Constraints.Min}
		}
		id := day.Format(time.DateOnly)
		enabled := options.permits(id)
		if enabled && p.cells[index].Clicked(gtx) {
			choose(id)
		}
		title := displayCivil(day, view.Language)
		if id == options.Today {
			title = fmt.Sprintf(view.Language.Text(i18n.DateToday), title)
		}
		return dayCell(gtx, view, &p.cells[index], fmt.Sprint(day.Day()), title, dayState{
			enabled: enabled, selected: id == value, today: id == options.Today,
			highlighted: day.Equal(p.day) && gtx.Focused(p),
		})
	})
}

type dayState struct{ enabled, selected, today, highlighted bool }

// dayCell draws one calendar day as a disc centered in its cell, like the
// Wails calendar: accent for the chosen day, a soft accent disc for today, a
// wash on hover and a ring for the keyboard cursor.
func dayCell(gtx layout.Context, view View, button *widget.Clickable, label, description string, state dayState) layout.Dimensions {
	colors := view.Colors
	content := func(gtx layout.Context) layout.Dimensions {
		semantic.Button.Add(gtx.Ops)
		semantic.SelectedOp(state.selected).Add(gtx.Ops)
		semantic.DescriptionOp(description).Add(gtx.Ops)
		size := gtx.Constraints.Min
		// The disc leaves room for the keyboard ring inside the cell.
		side := max(0, min(size.X, size.Y)-2*gtx.Dp(2))
		disc := image.Rectangle{Min: image.Pt((size.X-side)/2, (size.Y-side)/2)}
		disc.Max = disc.Min.Add(image.Pt(side, side))
		ink, style := colors.Ink, TextBody
		switch {
		case !state.enabled:
			ink = WithAlpha(colors.SecondaryInk, 0.6)
		case state.selected:
			paint.FillShape(gtx.Ops, colors.Accent, clip.Ellipse(disc).Op(gtx.Ops))
			ink, style = colors.OnAccent, TextLabel
		case state.today:
			fill := Mix(colors.Surface, colors.Accent, 0.14)
			if button.Hovered() {
				fill = Mix(colors.Surface, colors.Accent, 0.22)
			}
			paint.FillShape(gtx.Ops, fill, clip.Ellipse(disc).Op(gtx.Ops))
			ink, style = colors.Accent, TextLabel
		case button.Hovered() || state.highlighted:
			paint.FillShape(gtx.Ops, colors.HoverSurface, clip.Ellipse(disc).Op(gtx.Ops))
		}
		if state.highlighted && state.enabled {
			ring := colors.Accent
			if state.selected {
				ring = colors.Ink
			}
			outer := disc.Inset(-gtx.Dp(1))
			paint.FillShape(gtx.Ops, ring, clip.Stroke{Path: clip.Ellipse(outer).Path(gtx.Ops), Width: float32(gtx.Dp(2))}.Op())
		}
		layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return view.Line(gtx, style, label, ink)
		})
		return layout.Dimensions{Size: size}
	}
	if !state.enabled {
		gtx = gtx.Disabled()
	}
	return accessibility.Button(gtx, button, description, "", false, func(gtx layout.Context) layout.Dimensions {
		if state.enabled {
			return button.Layout(gtx, content)
		}
		defer clip.Rect{Max: gtx.Constraints.Min}.Push(gtx.Ops).Pop()
		semantic.EnabledOp(false).Add(gtx.Ops)
		return content(gtx)
	})
}

func calendarRow(gtx layout.Context, height int, cell func(layout.Context, int) layout.Dimensions) layout.Dimensions {
	children := make([]layout.FlexChild, 7)
	for i := range children {
		children[i] = layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.Y, gtx.Constraints.Max.Y = height, height
			return cell(gtx, i)
		})
	}
	return layout.Flex{Gap: gtx.Dp(2)}.Layout(gtx, children...)
}
