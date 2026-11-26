package desktop

import (
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/domain"
)

const taskUpcomingPageSize = 50

type taskUpcomingFilter int

const (
	taskUpcomingAll taskUpcomingFilter = iota
	taskUpcomingDue
	taskUpcomingReservations
	taskUpcomingScheduled
)

var taskUpcomingFilters = [...]i18n.MessageID{
	i18n.TasksAllActions, i18n.TasksReceiptsDue, i18n.ReserveTitle, i18n.TasksScheduled,
}

func (f taskUpcomingFilter) matches(row domain.UpcomingAction) bool {
	if row.Group != "attention" && row.Group != "scheduled" {
		return false
	}
	switch f {
	case taskUpcomingDue:
		return row.Phase == "receipt_due"
	case taskUpcomingReservations:
		return row.Kind == "reservation_shortfall" || row.Kind == "reservation_unresolved"
	case taskUpcomingScheduled:
		return row.Group == "scheduled"
	default:
		return true
	}
}

func (p *tasksPage) upcomingFilterTabs(view ui.View) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return view.TabTrack(gtx, len(taskUpcomingFilters), func(gtx layout.Context, i int) layout.Dimensions {
			filter, button := taskUpcomingFilter(i), &p.upcomingTabs[i]
			if button.Clicked(gtx) && filter != p.upcomingFilter {
				p.upcomingFilter, p.upcomingLimit = filter, taskUpcomingPageSize
				gtx.Execute(op.InvalidateCmd{})
			}
			view.Focus.Watch(gtx, button)
			return view.TabSegment(button, view.Language.Text(taskUpcomingFilters[i]), filter == p.upcomingFilter)(gtx)
		})
	}
}

func (p *tasksPage) upcomingRows(view ui.View, rows []layout.Widget, items []domain.UpcomingAction) []layout.Widget {
	before := len(rows)
	more, attention, scheduled := false, 0, 0
	limit := max(p.upcomingLimit, taskUpcomingPageSize)
	for _, row := range items {
		if !p.upcomingFilter.matches(row) {
			continue
		}
		if p.upcomingFilter == taskUpcomingAll {
			n, heading := &scheduled, i18n.TasksScheduled
			if row.Group == "attention" {
				n, heading = &attention, i18n.TasksAttention
			}
			if *n == 0 {
				rows = append(rows, tradeTitle(view, heading))
			}
			if *n >= limit {
				more = true
				continue
			}
			*n += 1
		} else if len(rows)-before >= limit {
			more = true
			break
		}
		rows = append(rows, p.card(view, row))
	}
	if len(rows) == before {
		rows = append(rows, emptyState(view, view.Language.Text(i18n.TasksEmpty)))
	}
	if more {
		rows = append(rows, func(gtx layout.Context) layout.Dimensions {
			if p.upcomingMore.Clicked(gtx) {
				p.upcomingLimit = limit + taskUpcomingPageSize
				gtx.Execute(op.InvalidateCmd{})
			}
			return returnMonthButton(view, &p.upcomingMore, i18n.TasksShowMoreActions, nil, true)(gtx)
		})
	}
	return rows
}
