package desktop

import (
	"context"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type returnAnalysisReader interface {
	analysisContextReader
	ReturnCalendar(context.Context, domain.AnalysisQuery, string, string) (application.ReturnCalendarResult, error)
	ReturnDay(context.Context, domain.AnalysisQuery, domain.LocalDate) (application.ReturnDayResult, error)
	ReturnTrend(context.Context, domain.AnalysisQuery, application.ReturnTrendDisplay) (application.ReturnTrendResult, error)
	Contribution(context.Context, domain.AnalysisQuery, application.ContributionReturnType, application.ContributionGroupBy, application.ContributionSort) (application.ContributionResult, error)
	ContributionItem(context.Context, domain.AnalysisQuery, application.ContributionReturnType, application.ContributionGroupBy, string) (application.ContributionItemResult, error)
}

type returnAnalysisTab int

const (
	returnCalendarTab returnAnalysisTab = iota
	returnTrendTab
	returnContributionTab
)

type returnCalendarFrame struct {
	summary      application.ReturnCalendarResult
	days         map[domain.LocalDate]application.ReturnDayResult
	gridQuery    domain.AnalysisQuery
	query        domain.AnalysisQuery
	summaryFrom  domain.LocalDate
	summaryTo    domain.LocalDate
	trend        *application.ReturnTrendResult
	contribution *application.ContributionResult
	yearMonths   []application.ReturnCalendarMonthResult
}

type returnDaySheet struct {
	analysisSheet
	date     domain.LocalDate
	button   *widget.Clickable
	request  <-chan historyResult[application.ReturnDayResult]
	cancel   context.CancelFunc
	value    application.ReturnDayResult
	err      error
	hasValue bool
}

type returnAnalysisPage struct {
	ctx        context.Context
	reader     returnAnalysisReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	cancel     context.CancelFunc
	refsRead   <-chan historyResult[analysisContext]
	request    <-chan historyResult[returnCalendarFrame]

	refs    analysisContext
	month   time.Time
	year    bool
	sunday  bool
	err     error
	current *returnCalendarFrame
	filters analysisFilterControls
	tab     returnAnalysisTab
	tabs    [3]widget.Clickable
	output  returnProjectionControls

	previous, next, today, reload widget.Clickable
	monthMode, yearMode           widget.Clickable
	cells                         [42]widget.Clickable
	yearCells                     [12]widget.Clickable
	day                           *returnDaySheet
	item                          *contributionSheet
	returnFocus                   *widget.Clickable
	openAssets                    func(analysisSelection, *widget.Clickable)
	openHistory                   func(history.Filters, *widget.Clickable)
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
	p.refsRead = runHistoryRead(&p.workers, p.invalidate, func() (analysisContext, error) {
		return readAnalysisContext(ctx, p.reader)
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

func (p *returnAnalysisPage) periodQuery(month, now time.Time) domain.AnalysisQuery {
	start, end := month, month.AddDate(0, 1, -1)
	if p.year && p.tab == returnCalendarTab {
		start = time.Date(month.Year(), time.January, 1, 0, 0, 0, 0, time.UTC)
		end = start.AddDate(1, 0, -1)
	}
	return returnCalendarQuery(start, end, p.refs.origin, now)
}

func (p *returnAnalysisPage) readMonth(month time.Time, sunday bool) {
	p.leave()
	if p.month != month || p.sunday != sunday {
		p.current = nil
	}
	p.month, p.sunday, p.err = month, sunday, nil
	selected := p.filters.selection
	if !selected.ready() {
		p.current = nil
		return
	}
	now := p.clock()
	origin := p.refs.origin
	grid, cells := returnGrid(month, sunday)
	periodQuery := selected.query(p.periodQuery(month, now))
	monthQuery := periodQuery
	year := p.year && p.tab == returnCalendarTab
	monthCursor := month.Format("2006-01")
	if year {
		monthCursor = ""
	}
	if selected.from != "" && selected.to != "" {
		monthQuery.From, monthQuery.To, monthCursor = selected.from, selected.to, ""
	}
	gridRange := returnCalendarQuery(grid, grid.AddDate(0, 0, cells-1), origin, now)
	gridQuery := monthQuery
	gridQuery.From, gridQuery.To = gridRange.From, gridRange.To
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	tab, display, metric, group, sort := p.tab, p.output.trendDisplay(), p.output.contributionMetric(), p.output.contributionGroup(), p.output.contributionSort()
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (returnCalendarFrame, error) {
		out := returnCalendarFrame{summaryFrom: monthQuery.From, summaryTo: monthQuery.To, gridQuery: gridQuery, query: monthQuery}
		if tab != returnCalendarTab && monthQuery.From <= monthQuery.To {
			switch tab {
			case returnTrendTab:
				result, err := p.reader.ReturnTrend(ctx, monthQuery, display)
				out.trend = &result
				return out, err
			case returnContributionTab:
				result, err := p.reader.Contribution(ctx, monthQuery, metric, group, sort)
				out.contribution = &result
				return out, err
			}
		}
		if tab != returnCalendarTab {
			return out, nil
		}
		if monthQuery.From <= monthQuery.To {
			result, err := p.reader.ReturnCalendar(ctx, monthQuery, monthCursor, "")
			if err != nil {
				return returnCalendarFrame{}, err
			}
			out.summary = result
		}
		if year {
			if periodQuery.From <= periodQuery.To {
				months := out.summary.Months
				if periodQuery != monthQuery {
					result, err := p.reader.ReturnCalendar(ctx, periodQuery, "", "")
					if err != nil {
						return returnCalendarFrame{}, err
					}
					months = result.Months
				}
				out.yearMonths = months
			}
			return out, nil
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

func (p *returnAnalysisPage) changeTab(tab returnAnalysisTab) {
	if tab == p.tab {
		return
	}
	if p.tab == returnCalendarTab && p.refs.origin != nil {
		selection := &p.filters.selection
		window := p.periodQuery(p.month, p.clock())
		if selection.from != "" {
			window.From = selection.from
		}
		if selection.to != "" {
			window.To = selection.to
		}
		if window.From <= window.To {
			selection.from, selection.to = window.From, window.To
		}
	}
	p.tab, p.current = tab, nil
	p.output.list.Position = layout.Position{}
	if p.refs.origin != nil {
		p.readMonth(p.month, p.sunday)
	}
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
			if ranks := p.current.contribution; ranks != nil {
				present := make(map[string]bool, len(ranks.Rows))
				for _, row := range ranks.Rows {
					present[row.Key] = true
				}
				for key := range p.output.rankButtons {
					if !present[key] {
						delete(p.output.rankButtons, key)
					}
				}
				if !present[p.output.selectedKey] {
					p.output.selectedKey = ""
				}
			}
		}
	default:
	}
	if p.refs.origin != nil && p.filters.selection.ready() && (p.sunday != sunday || p.request == nil && p.current == nil && p.err == nil) {
		p.readMonth(p.month, sunday)
	}
}

func (p *returnAnalysisPage) changeFilters(previous, selection analysisSelection) {
	month := p.month
	if previous.to != selection.to {
		if date, valid := ui.ParseCivilDay(selection.to); valid {
			month = returnMonth(date)
		}
	}
	p.current = nil
	p.output.list.Position = layout.Position{}
	p.readMonth(month, p.sunday)
}

func (p *returnAnalysisPage) openDay(date domain.LocalDate, trigger *widget.Clickable) {
	p.day = &returnDaySheet{analysisSheet: newAnalysisSheet(), date: date, button: trigger}
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
