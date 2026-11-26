package desktop

import (
	"context"
	"errors"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/charts"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type wealthHistoryReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	HistoricalOverviewSeries(context.Context, string, string) (application.HistoricalOverviewSeriesResult, error)
	HistoricalOverview(context.Context, string, string) (application.HistoricalOverviewResult, error)
}

type wealthDateRange struct{ from, to string }

type wealthHistoryLens uint8

const (
	wealthAllocationLens wealthHistoryLens = iota
	wealthAccountsLens
	wealthTrendLens
	wealthChangeLens
)

type wealthHistoryOrigin struct {
	first    string
	location *time.Location
}

func (o wealthHistoryOrigin) lastClosed(now time.Time) time.Time {
	year, month, day := now.In(o.location).Date()
	return time.Date(year, month, day-1, 0, 0, 0, 0, time.UTC)
}

func (o wealthHistoryOrigin) permits(r wealthDateRange, now time.Time) bool {
	from, fromOK := ui.ParseCivilDay(r.from)
	to, toOK := ui.ParseCivilDay(r.to)
	if !fromOK || !toOK || r.from < o.first || to.After(o.lastClosed(now)) ||
		to.Before(from) || to.After(from.AddDate(0, 0, 30)) {
		return false
	}
	_, fromErr := domain.ResolveCivilDateCutoff(from, o.location)
	_, toErr := domain.ResolveCivilDateCutoff(to, o.location)
	return fromErr == nil && toErr == nil
}

type wealthHistoryCapture struct {
	series    application.HistoricalOverviewSeriesResult
	rangeRead wealthDateRange
	rows      []wealthHistoryRow
}

type wealthHistoryRow struct {
	point   *application.HistoricalSeriesPoint
	missing *application.HistoricalSeriesMissing
}

type wealthHistoryPage struct {
	ctx        context.Context
	reader     wealthHistoryReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	cancel     context.CancelFunc

	originRead       <-chan historyResult[*wealthHistoryOrigin]
	seriesRead       <-chan historyResult[application.HistoricalOverviewSeriesResult]
	origin           *wealthHistoryOrigin
	err              error
	draft, requested wealthDateRange
	current          *wealthHistoryCapture

	from, to                             ui.DatePicker
	load                                 widget.Clickable
	list                                 widget.List
	accounts                             wealthAccountCapture
	allocation                           wealthAllocation
	trend                                wealthTrend
	change                               wealthChange
	detail                               *wealthDetail
	returnDetail                         *wealthDetail
	detailFocus                          event.Tag
	openHistory                          func(history.Filters, event.Tag)
	changeTab                            widget.Clickable
	allocationTab, accountsTab, trendTab widget.Clickable
	lens                                 wealthHistoryLens
}

func newWealthHistoryPage(ctx context.Context, reader wealthHistoryReader, clock func() time.Time, invalidate func()) *wealthHistoryPage {
	return &wealthHistoryPage{
		ctx: ctx, reader: reader, clock: clock, invalidate: invalidate,
		list:  widget.List{List: layout.List{Axis: layout.Vertical}},
		trend: wealthTrend{days: make(map[string]*widget.Clickable)},
	}
}

func (p *wealthHistoryPage) enter() {
	if p.returnDetail != nil {
		p.detail, p.returnDetail = p.returnDetail, nil
	}
	if p.origin == nil {
		p.readOrigin()
	} else if p.lens != wealthTrendLens {
		if p.accounts.current == nil && p.accounts.err == nil && p.validAccountDates(p.accounts.draft, p.clock()) {
			p.readAccounts(p.accounts.draft)
		}
	} else if p.current == nil && p.err == nil && p.origin.permits(p.requested, p.clock()) {
		p.readSeries(p.requested)
	}
}

func (p *wealthHistoryPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.originRead, p.seriesRead = nil, nil, nil
	p.accounts.request = nil
}

func (p *wealthHistoryPage) readOrigin() {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err = cancel, nil
	p.originRead = runHistoryRead(&p.workers, p.invalidate, func() (*wealthHistoryOrigin, error) {
		origin, err := p.reader.HistoryOrigin(ctx)
		if err != nil || origin == nil {
			return nil, err
		}
		location, err := time.LoadLocation(origin.Timezone)
		if err != nil {
			return nil, err
		}
		return &wealthHistoryOrigin{first: origin.StartedAt.In(location).Format(time.DateOnly), location: location}, nil
	})
}

func (p *wealthHistoryPage) readSeries(r wealthDateRange) {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err, p.requested = cancel, nil, r
	p.seriesRead = runHistoryRead(&p.workers, p.invalidate, func() (application.HistoricalOverviewSeriesResult, error) {
		return p.reader.HistoricalOverviewSeries(ctx, r.from, r.to)
	})
}

func (p *wealthHistoryPage) update() {
	select {
	case result := <-p.originRead:
		p.leave()
		p.origin, p.err = result.value, result.err
	case result := <-p.seriesRead:
		p.leave()
		p.err = result.err
		if result.err != nil {
			return
		}
		if p.current == nil || p.current.rangeRead != p.requested {
			p.list.Position = layout.Position{}
			clear(p.trend.days)
			if day := string(p.trend.selected.Item); day < p.requested.from || day > p.requested.to {
				p.trend.selected = charts.Selection{}
			}
		}
		capture := &wealthHistoryCapture{series: result.value, rangeRead: p.requested}
		for i := range capture.series.Points {
			point := &capture.series.Points[i]
			capture.rows = append(capture.rows, wealthHistoryRow{point: point})
			for j := range point.Missing {
				capture.rows = append(capture.rows, wealthHistoryRow{point: point, missing: &point.Missing[j]})
			}
		}
		p.current = capture
		p.list.Position = restoredPosition(p.list.Position, len(capture.rows))
	default:
	}
	if p.origin == nil || p.draft != (wealthDateRange{}) {
		return
	}
	now := p.clock()
	for day := p.origin.lastClosed(now); day.Format(time.DateOnly) >= p.origin.first; day = day.AddDate(0, 0, -1) {
		date := day.Format(time.DateOnly)
		initial := wealthDateRange{date, date}
		if p.origin.permits(initial, now) {
			p.draft = initial
			p.requested = initial
			if p.lens == wealthTrendLens {
				p.readSeries(initial)
			} else {
				p.selectEndpointLens(p.lens)
			}
			return
		}
	}
}

func wealthHistoryBudgetExceeded(err error) bool {
	var problem *domain.Error
	return errors.As(err, &problem) && problem.Code == domain.ErrHistoricalSeriesBudget
}
