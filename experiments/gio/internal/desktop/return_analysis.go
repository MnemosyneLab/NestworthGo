package desktop

import (
	"context"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type returnAnalysisReader interface {
	HistoryOrigin(context.Context) (*domain.HistoryOrigin, error)
	ListInstruments(context.Context, bool) ([]domain.Instrument, error)
	ReturnCalendar(context.Context, domain.AnalysisQuery, string, string) (application.ReturnCalendarResult, error)
	ReturnDay(context.Context, domain.AnalysisQuery, domain.LocalDate) (application.ReturnDayResult, error)
}

type returnCalendarFrame struct {
	summary     application.ReturnCalendarResult
	days        map[domain.LocalDate]application.ReturnDayResult
	gridQuery   domain.AnalysisQuery
	summaryFrom domain.LocalDate
	summaryTo   domain.LocalDate
}

type returnCalendarContext struct {
	origin *wealthHistoryOrigin
	names  map[string]string
}

type returnDaySheet struct {
	date     domain.LocalDate
	button   *widget.Clickable
	close    widget.Clickable
	retry    widget.Clickable
	list     widget.List
	request  <-chan historyResult[application.ReturnDayResult]
	cancel   context.CancelFunc
	value    application.ReturnDayResult
	err      error
	hasValue bool
	focus    bool
}

type returnAnalysisPage struct {
	ctx        context.Context
	reader     returnAnalysisReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	cancel     context.CancelFunc
	refsRead   <-chan historyResult[returnCalendarContext]
	request    <-chan historyResult[returnCalendarFrame]

	refs    returnCalendarContext
	month   time.Time
	sunday  bool
	err     error
	current *returnCalendarFrame

	previous, next, today, reload widget.Clickable
	cells                         [42]widget.Clickable
	day                           *returnDaySheet
	returnFocus                   *widget.Clickable
}

func (p *returnAnalysisPage) enter() {
	if p.refs.origin == nil {
		p.readRefs()
	} else {
		p.readMonth(p.month, p.sunday)
	}
}

func (p *returnAnalysisPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.refsRead, p.request = nil, nil, nil
}

func (p *returnAnalysisPage) readRefs() {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err = cancel, nil
	p.refsRead = runHistoryRead(&p.workers, p.invalidate, func() (returnCalendarContext, error) {
		var out returnCalendarContext
		origin, err := p.reader.HistoryOrigin(ctx)
		if err != nil || origin == nil {
			return out, err
		}
		location, err := time.LoadLocation(origin.Timezone)
		if err != nil {
			return out, err
		}
		out.origin = &wealthHistoryOrigin{first: origin.StartedAt.In(location).Format(time.DateOnly), location: location}
		instruments, err := p.reader.ListInstruments(ctx, true)
		if err != nil {
			return returnCalendarContext{}, err
		}
		out.names = make(map[string]string, len(instruments))
		for _, instrument := range instruments {
			out.names[instrument.ID.String()] = instrument.DisplayLabel()
		}
		return out, nil
	})
}

func returnMonth(day time.Time) time.Time {
	year, month, _ := day.Date()
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

func returnGrid(month time.Time, sunday bool) (time.Time, int) {
	start := time.Monday
	if sunday {
		start = time.Sunday
	}
	leading := (int(month.Weekday()-start) + 7) % 7
	days := month.AddDate(0, 1, -1).Day()
	return month.AddDate(0, 0, -leading), ((leading + days + 6) / 7) * 7
}

func returnCalendarQuery(from, to time.Time, origin *wealthHistoryOrigin, now time.Time) domain.AnalysisQuery {
	return domain.AnalysisQuery{
		Scope:     domain.AnalysisScope{Kind: domain.ScopeHousehold},
		From:      domain.LocalDate(max(from.Format(time.DateOnly), origin.first)),
		To:        domain.LocalDate(min(to.Format(time.DateOnly), origin.lastClosed(now).Format(time.DateOnly))),
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	}
}

func (p *returnAnalysisPage) readMonth(month time.Time, sunday bool) {
	p.leave()
	if p.month != month || p.sunday != sunday {
		p.current = nil
	}
	p.month, p.sunday, p.err = month, sunday, nil
	now := p.clock()
	origin := p.refs.origin
	grid, cells := returnGrid(month, sunday)
	monthQuery := returnCalendarQuery(month, month.AddDate(0, 1, -1), origin, now)
	gridQuery := returnCalendarQuery(grid, grid.AddDate(0, 0, cells-1), origin, now)
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (returnCalendarFrame, error) {
		out := returnCalendarFrame{summaryFrom: monthQuery.From, summaryTo: monthQuery.To, gridQuery: gridQuery}
		if monthQuery.From <= monthQuery.To {
			result, err := p.reader.ReturnCalendar(ctx, monthQuery, month.Format("2006-01"), "")
			if err != nil {
				return returnCalendarFrame{}, err
			}
			out.summary = result
		}
		if gridQuery.From <= gridQuery.To {
			cells := out.summary.Days
			if gridQuery != monthQuery {
				result, err := p.reader.ReturnCalendar(ctx, gridQuery, "", "")
				if err != nil {
					return returnCalendarFrame{}, err
				}
				cells = result.Days
			}
			out.days = make(map[domain.LocalDate]application.ReturnDayResult, len(cells))
			for _, day := range cells {
				out.days[day.Date] = day
			}
		}
		return out, nil
	})
}

func (p *returnAnalysisPage) update(sunday bool) {
	select {
	case result := <-p.refsRead:
		p.leave()
		p.refs, p.err = result.value, result.err
		if p.refs.origin != nil {
			p.readMonth(returnMonth(p.clock().In(p.refs.origin.location)), sunday)
		}
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err == nil {
			p.current = &result.value
		}
	default:
	}
	if p.refs.origin != nil && (p.sunday != sunday || p.request == nil && p.current == nil && p.err == nil) {
		p.readMonth(p.month, sunday)
	}
}

func (p *returnAnalysisPage) openDay(date domain.LocalDate, trigger *widget.Clickable) {
	p.day = &returnDaySheet{date: date, button: trigger, focus: true, list: widget.List{List: layout.List{Axis: layout.Vertical}}}
	p.readDay()
}

func (p *returnAnalysisPage) readDay() {
	sheet := p.day
	query, date := p.current.gridQuery, sheet.date
	ctx, cancel := context.WithCancel(p.ctx)
	sheet.cancel, sheet.err = cancel, nil
	sheet.request = runHistoryRead(&p.workers, p.invalidate, func() (application.ReturnDayResult, error) {
		return p.reader.ReturnDay(ctx, query, date)
	})
}

func (p *returnAnalysisPage) closeDay() {
	p.day.cancel()
	p.returnFocus = p.day.button
	p.day = nil
}
