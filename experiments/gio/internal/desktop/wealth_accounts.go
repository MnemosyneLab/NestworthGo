package desktop

import (
	"context"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

type wealthAccountMode int

const (
	wealthSingleDay wealthAccountMode = iota
	wealthCompareDate
	wealthCompareCurrent
)

type wealthAccountDates struct {
	mode     wealthAccountMode
	from, to string
}

type wealthAccountCapture struct {
	draft, requested, captured wealthAccountDates
	request                    <-chan historyResult[application.HistoricalOverviewResult]
	current                    *application.HistoricalOverviewResult
	err                        error
	modes                      [3]widget.Clickable
	changes, reload, swap      widget.Clickable
	changesOnly                bool
	category                   wealthAllocationFilter
	earlier, later             ui.DatePicker
	list                       widget.List
	expanded                   map[string]bool
	toggles                    map[string]*widget.Clickable
	details                    map[string]*widget.Clickable
}

func (p *wealthHistoryPage) validAccountDates(dates wealthAccountDates, now time.Time) bool {
	return p.origin.permits(wealthDateRange{dates.from, dates.from}, now) &&
		(dates.mode != wealthCompareDate || p.origin.permits(wealthDateRange{dates.to, dates.to}, now))
}

func (p *wealthHistoryPage) readAccounts(dates wealthAccountDates) {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.accounts.requested, p.accounts.err = cancel, dates, nil
	p.accounts.request = runHistoryRead(&p.workers, p.invalidate, func() (application.HistoricalOverviewResult, error) {
		switch dates.mode {
		case wealthCompareDate:
			return p.reader.HistoricalOverview(ctx, dates.from, dates.to)
		case wealthCompareCurrent:
			return p.reader.HistoricalOverview(ctx, dates.from, "current")
		default:
			return p.reader.HistoricalOverview(ctx, dates.from, "")
		}
	})
}

func (p *wealthHistoryPage) updateAccounts() {
	a := &p.accounts
	select {
	case result := <-a.request:
		p.leave()
		a.err = result.err
		if result.err == nil {
			switch {
			case p.lens == wealthAllocationLens && result.value.Right != nil && (a.current == nil || a.current.Right == nil):
				p.lens = wealthChangeLens
			case p.lens == wealthChangeLens && result.value.Right == nil:
				p.lens = wealthAllocationLens
			}
			if a.captured != a.requested {
				a.list.Position = layout.Position{}
				a.category = wealthAllocationFilter{}
				p.allocation.list.Position = layout.Position{}
				p.allocation.focus, p.allocation.restore = nil, nil
				p.allocation.focusAccounts = false
				p.change.list.Position = layout.Position{}
				p.change.all = false
			}
			a.current, a.captured = &result.value, a.requested
		}
	default:
	}
}

func (p *wealthHistoryPage) selectEndpointLens(lens wealthHistoryLens) {
	if p.lens == wealthTrendLens {
		p.leave()
	}
	p.lens = lens
	if p.accounts.draft.from == "" {
		now := p.clock()
		local := now.In(p.origin.location)
		monthEnd := time.Date(local.Year(), local.Month(), 0, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
		chosen := max(monthEnd, p.origin.first)
		if !p.validAccountDates(wealthAccountDates{from: chosen}, now) {
			chosen = p.draft.to
		}
		p.accounts.draft = wealthAccountDates{from: chosen, to: p.draft.to}
	}
	if p.accounts.current == nil && p.accounts.request == nil && p.accounts.err == nil {
		p.readAccounts(p.accounts.draft)
	}
}

func (p *wealthHistoryPage) selectTrend() {
	p.lens = wealthTrendLens
	p.leave()
	if p.current == nil {
		p.readSeries(p.draft)
	}
}
