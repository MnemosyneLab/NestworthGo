package desktop

import (
	"time"

	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
)

const wealthCaptureShortcutCount = 7

func (p *wealthHistoryPage) layoutWealthCaptureShortcuts(gtx layout.Context, view ui.View, now time.Time, actions ...layout.Widget) layout.Dimensions {
	a := &p.accounts
	draft := a.draft
	monthEnd := p.origin.previousMonthEnd(now)
	latest := p.origin.lastClosed(now).Format(time.DateOnly)
	withDate := func(day string) wealthAccountDates {
		return wealthAccountDates{mode: draft.mode, from: day, to: draft.to}
	}
	previous, next := "", ""
	if date, ok := ui.ParseCivilDay(draft.from); ok && p.origin.permits(wealthDateRange{draft.from, draft.from}, now) {
		previous = date.AddDate(0, 0, -1).Format(time.DateOnly)
		next = date.AddDate(0, 0, 1).Format(time.DateOnly)
	}
	shortcuts := [wealthCaptureShortcutCount]struct {
		label i18n.MessageID
		dates wealthAccountDates
		icon  *lucide.Icon
	}{
		{i18n.WealthCapturePrevious, withDate(previous), lucide.ChevronLeft},
		{i18n.WealthCaptureNext, withDate(next), lucide.ChevronRight},
		{i18n.WealthCaptureLastMonth, withDate(monthEnd), nil},
		{i18n.WealthCaptureLastClosed, withDate(latest), nil},
		{i18n.WealthCaptureStartingPoint, withDate(p.origin.first), nil},
		{i18n.WealthCaptureMonthToLatest, wealthAccountDates{mode: wealthCompareDate, from: monthEnd, to: latest}, nil},
		{i18n.WealthCaptureCurrent, wealthAccountDates{mode: wealthCompareCurrent, from: draft.from, to: draft.to}, nil},
	}
	count := len(shortcuts)
	if draft.mode == wealthSingleDay {
		count -= 2
	}
	return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, count+len(actions), func(gtx layout.Context, i int) layout.Dimensions {
		if i >= count {
			return actions[i-count](gtx)
		}
		shortcut, button := shortcuts[i], &a.shortcuts[i]
		dates := shortcut.dates
		enabled := p.validAccountDates(dates, now) &&
			(a.request == nil || a.requested != dates) &&
			(a.draft != dates || a.captured != dates || a.err != nil)
		if enabled && button.Clicked(gtx) {
			a.draft = dates
			p.readAccounts(dates)
			gtx.Execute(op.InvalidateCmd{})
		}
		if shortcut.icon != nil {
			return analysisDateArrow(view, button, shortcut.label, shortcut.icon, enabled)(gtx)
		}
		return returnMonthButton(view, button, shortcut.label, nil, enabled)(gtx)
	})
}
