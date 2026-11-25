package desktop

import (
	"context"
	"sync"
	"time"

	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type assetChangesReader interface {
	analysisContextReader
	AssetChange(context.Context, domain.AnalysisQuery) (application.AssetChangeResult, error)
	AssetTrend(context.Context, domain.AnalysisQuery, application.AssetTrendGranularity, application.AssetTrendMetric) (application.AssetTrendResult, error)
	Categories(context.Context, domain.AnalysisQuery, application.AnalysisCategoryType) (application.CategoriesResult, error)
}

type assetChangesTab int

const (
	assetTrendTab assetChangesTab = iota
	assetDriversTab
	assetCategoriesTab
)

type assetChangesVisit struct {
	selection analysisSelection
	tab       assetChangesTab
	metric    application.AssetTrendMetric
	interval  application.AssetTrendGranularity
	category  application.AnalysisCategoryType
	list      layout.Position
	expanded  bool
}

type assetChangesResult struct {
	query      domain.AnalysisQuery
	trend      *application.AssetTrendResult
	drivers    *application.AssetChangeResult
	categories *application.CategoriesResult
}

type assetChangesPage struct {
	ctx        context.Context
	reader     assetChangesReader
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup
	cancel     context.CancelFunc
	refsRead   <-chan historyResult[analysisContext]
	request    <-chan historyResult[assetChangesResult]
	refs       analysisContext
	err        error
	current    *assetChangesResult

	state                      assetChangesVisit
	filters                    analysisFilterControls
	tabs                       [3]widget.Clickable
	reload                     widget.Clickable
	metric, interval, category ui.Select
	list                       widget.List
}

func (p *assetChangesPage) enter(state assetChangesVisit) {
	p.state = state
	p.filters.selection = state.selection
	p.filters.expanded = state.expanded
	p.list.Position = state.list
	p.current = nil
	if p.refs.origin == nil {
		p.readRefs()
	} else {
		p.read()
	}
}

func (p *assetChangesPage) visit() assetChangesVisit {
	state := p.state
	state.selection, state.list = p.filters.selection, p.list.Position
	state.expanded = p.filters.expanded
	return state
}

func (p *assetChangesPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request, p.refsRead = nil, nil, nil
}

func (p *assetChangesPage) readRefs() {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err = cancel, nil
	p.refsRead = runHistoryRead(&p.workers, p.invalidate, func() (analysisContext, error) {
		return readAnalysisContext(ctx, p.reader)
	})
}

func (p *assetChangesPage) read() {
	p.leave()
	p.err = nil
	selection := p.filters.selection
	if !selection.ready() {
		p.current = nil
		return
	}
	now := p.clock()
	month := returnMonth(now.In(p.refs.origin.location))
	query := selection.query(returnCalendarQuery(month, month.AddDate(0, 1, -1), p.refs.origin, now))
	if selection.from != "" {
		query.From = selection.from
	}
	if selection.to != "" {
		query.To = selection.to
	}
	if query.From > query.To {
		p.current = &assetChangesResult{query: query}
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel = cancel
	tab, metric, interval, category := p.state.tab, p.trendMetric(), p.trendInterval(), p.categoryType()
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (assetChangesResult, error) {
		out := assetChangesResult{query: query}
		switch tab {
		case assetDriversTab:
			data, err := p.reader.AssetChange(ctx, query)
			out.drivers = &data
			return out, err
		case assetCategoriesTab:
			data, err := p.reader.Categories(ctx, query, category)
			out.categories = &data
			return out, err
		default:
			data, err := p.reader.AssetTrend(ctx, query, interval, metric)
			out.trend = &data
			return out, err
		}
	})
}

func (p *assetChangesPage) update() {
	select {
	case result := <-p.refsRead:
		p.leave()
		p.refs, p.err = result.value, result.err
		if p.refs.origin != nil {
			p.read()
		}
	case result := <-p.request:
		p.leave()
		p.err = result.err
		if p.err == nil {
			p.current = &result.value
		}
	default:
	}
}

func (p *assetChangesPage) change() {
	p.current = nil
	p.list.Position = layout.Position{}
	if p.refs.origin != nil {
		p.read()
	}
}

func (p *assetChangesPage) trendMetric() application.AssetTrendMetric {
	if p.state.metric == "" {
		return application.TrendNetWorth
	}
	return p.state.metric
}

func (p *assetChangesPage) trendInterval() application.AssetTrendGranularity {
	if p.state.interval == "" {
		return application.TrendDay
	}
	return p.state.interval
}

func (p *assetChangesPage) categoryType() application.AnalysisCategoryType {
	if p.state.category == "" {
		return application.CategoryIncome
	}
	return p.state.category
}
