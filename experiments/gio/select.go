package main

import (
	"strings"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/widget"
)

type selectControl struct {
	trigger widget.Clickable
	list    optionList
	search  searchField
	query   string
}

type selectOptions struct {
	items      []choice
	searchable bool
	clearable  bool
}

func (s *selectControl) layout(gtx layout.Context, view presentation, field pickerField, options selectOptions, value string, change func(string)) {
	pane := popoverPane{focus: s}
	if options.searchable {
		pane.focus = &s.search.field.editor
		pane.otherFocus = []event.Tag{s, &s.search.clear}
		pane.onClose = s.resetSearch
	}
	pane.body = func(gtx layout.Context) layout.Dimensions {
		return s.popup(gtx, view, options, value, change)
	}
	if pickerTrigger(gtx, view, field, &s.trigger, choiceLabel(view, options.items, value), false, pane) {
		if options.searchable {
			s.resetSearch()
		}
		s.list.scroll.Position = layout.Position{}
		s.list.setActive(s.choices(view, options), value)
	}
}

func (s *selectControl) resetSearch() {
	s.query = ""
	s.search.field = newTextField(true)
}

func choiceLabel(view presentation, items []choice, value string) string {
	for _, item := range items {
		if item.id == value {
			return item.label
		}
	}
	return view.language.text(pickerEmpty)
}

func (s *selectControl) choices(view presentation, options selectOptions) []choice {
	query := strings.ToLower(strings.TrimSpace(s.query))
	if query == "" && !options.clearable {
		return options.items
	}
	items := make([]choice, 0, len(options.items)+1)
	if query == "" {
		items = append(items, choice{label: view.language.text(pickerEmptyOption)})
	}
	for _, item := range options.items {
		if strings.Contains(strings.ToLower(item.label), query) {
			items = append(items, item)
		}
	}
	return items
}

func (s *selectControl) popup(gtx layout.Context, view presentation, options selectOptions, value string, change func(string)) layout.Dimensions {
	registerPickerFocus(gtx, s)
	choose := func(item choice) {
		change(item.id)
		view.overlay.close()
	}
	rows := func(gtx layout.Context) layout.Dimensions {
		items := s.choices(view, options)
		for _, e := range pickerKeys(gtx, s, key.NameUpArrow, key.NameDownArrow, key.NameReturn, key.NameEnter, key.NameSpace) {
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
			return view.message(view.language.text(previewNoMatches), view.colors.secondaryInk)(gtx)
		}
		return s.list.layout(gtx, view, items, value, gtx.Focused(s), choose)
	}

	if !options.searchable {
		return rows(gtx)
	}

	// Keep candidate navigation and Return in the native editor. The explicit
	// shortcut moves focus to the option list, which then owns navigation.
	for {
		e, ok := gtx.Event(
			key.Filter{Focus: &s.search.field.editor, Name: key.NameDownArrow, Required: key.ModAlt},
			key.Filter{Focus: s, Name: key.NameUpArrow, Required: key.ModAlt},
		)
		if !ok {
			break
		}
		if e.(key.Event).State != key.Press {
			continue
		}
		if e.(key.Event).Name == key.NameUpArrow {
			s.search.field.focus(gtx)
		} else if s.list.active {
			gtx.Execute(key.FocusCmd{Tag: s})
		}
	}
	return layout.Flex{Axis: layout.Vertical, Gap: gtx.Dp(6)}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return s.search.layout(gtx, view, pickerFilter, pickerFilterHint, func(query string) {
				s.query = query
				s.list.scroll.Position = layout.Position{}
				s.list.setActive(s.choices(view, options), value)
			})
		}),
		layout.Rigid(view.message(view.language.text(pickerResultsKeys), view.colors.secondaryInk)),
		layout.Rigid(rows),
	)
}
