package ui

import (
	"image"
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
)

type Select struct {
	Trigger widget.Clickable
	list    optionList
	search  SearchField
}

type SelectOptions struct {
	Invalid    bool
	Items      []Choice
	Searchable bool
	Clearable  bool
}

func (s *Select) Layout(gtx layout.Context, view View, field PickerField, options SelectOptions, value string, change func(string)) {
	pane := Popover{focus: s}
	if options.Searchable {
		pane.focus = &s.search.Field.Editor
		pane.otherFocus = []event.Tag{s, &s.search.Clear}
		pane.onClose = s.resetSearch
	}
	pane.Body = func(gtx layout.Context) layout.Dimensions {
		return s.popup(gtx, view, options, value, change)
	}
	if pickerTrigger(gtx, view, field, &s.Trigger, ChoiceLabel(view, options.Items, value), options.Invalid, pane) {
		if options.Searchable {
			s.resetSearch()
		}
		s.list.scroll.Position = layout.Position{}
		s.list.setActive(s.choices(view, options), value)
	}
}

func (s *Select) resetSearch() {
	s.search.Field = NewTextField(true)
}

func ChoiceLabel(view View, items []Choice, value string) string {
	for _, item := range items {
		if item.ID == value {
			return item.Label
		}
	}
	return view.Language.Text(i18n.PickerEmpty)
}

func (s *Select) choices(view View, options SelectOptions) []Choice {
	var query string
	if options.Searchable {
		query = strings.ToLower(strings.TrimSpace(s.search.Field.Text()))
	}
	if query == "" && !options.Clearable {
		return options.Items
	}
	items := make([]Choice, 0, len(options.Items)+1)
	if query == "" {
		items = append(items, Choice{Label: view.Language.Text(i18n.PickerEmptyOption)})
	}
	for _, item := range options.Items {
		if strings.Contains(strings.ToLower(item.Label+" "+item.SearchText), query) {
			items = append(items, item)
		}
	}
	return items
}

func (s *Select) popup(gtx layout.Context, view View, options SelectOptions, value string, change func(string)) layout.Dimensions {
	registerPickerFocus(gtx, s)
	choose := func(item Choice) {
		change(item.ID)
		view.Overlay.Close()
	}
	rows := func(gtx layout.Context) layout.Dimensions {
		items := s.choices(view, options)
		for _, e := range PressedKeys(gtx, s, key.NameUpArrow, key.NameDownArrow, key.NameReturn, key.NameEnter, key.NameSpace) {
			switch e.Name {
			case key.NameUpArrow:
				s.list.move(items, -1)
			case key.NameDownArrow:
				s.list.move(items, 1)
			default:
				s.list.accept(items, choose)
			}
		}
		if len(items) == 0 {
			return view.Message(view.Language.Text(i18n.PickerNoMatches), view.Colors.SecondaryInk)(gtx)
		}
		return s.list.layout(gtx, view, items, value, gtx.Focused(s), choose)
	}

	if !options.Searchable {
		return rows(gtx)
	}

	// Keep candidate navigation and Return in the native editor. The explicit
	// shortcut moves focus to the option list, which then owns navigation.
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &s.search.Field.Editor, Name: key.NameDownArrow, Required: key.ModAlt},
			key.Filter{Focus: s, Name: key.NameUpArrow, Required: key.ModAlt},
		)
		if !ok {
			break
		}
		if e.(key.Event).State != key.Press {
			continue
		}
		if e.(key.Event).Name == key.NameUpArrow {
			s.search.Field.Focus(gtx)
		} else if s.list.active {
			gtx.Execute(key.FocusCmd{Tag: s})
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.search.Layout(gtx, view, i18n.PickerFilter, i18n.PickerFilterHint, func(string) {
				s.list.scroll.Position = layout.Position{}
				s.list.setActive(s.choices(view, options), value)
			})
		}),
		layout.Rigid(view.Message(view.Language.Text(i18n.PickerResultsKeys), view.Colors.SecondaryInk)),
		layout.Rigid(rows),
	)
}

// menu anchors the same single-select popup to a caller-drawn compact trigger.
func (s *Select) Menu(gtx layout.Context, view View, bounds image.Rectangle, items []Choice, value string, previous, next event.Tag, change func(string)) {
	if !gtx.Enabled() {
		return
	}
	pane := Popover{width: max(150, gtx.Metric.PxToDp(bounds.Dx())), menu: true, focus: s, previous: previous, next: next}
	pane.Body = func(gtx layout.Context) layout.Dimensions {
		return s.popup(gtx, view, SelectOptions{Items: items}, value, change)
	}
	if view.Overlay.Register(gtx, &s.Trigger, bounds, pane) {
		s.list.scroll.Position = layout.Position{}
		s.list.setActive(items, value)
	}
}
