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
	earlier, later             ui.DatePicker
	list                       widget.List
	expanded                   map[string]bool
	toggles                    map[string]*widget.Clickable
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
			if a.captured != a.requested {
				a.list.Position = layout.Position{}
			}
			a.current, a.captured = &result.value, a.requested
		}
	default:
	}
}

func (p *wealthHistoryPage) selectAccounts() {
	p.showAccounts = true
	if p.accounts.draft.from == "" {
		day := p.origin.lastClosed(p.clock()).Format(time.DateOnly)
		p.accounts.draft = wealthAccountDates{from: day, to: day}
	}
	if p.accounts.current == nil {
		p.readAccounts(p.accounts.draft)
	} else {
		p.leave()
	}
}

func (p *wealthHistoryPage) selectTrend() {
	p.showAccounts = false
	p.leave()
	if p.current == nil {
		p.readSeries(p.draft)
	}
}
