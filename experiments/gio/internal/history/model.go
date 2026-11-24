package history

import (
	"context"
	"slices"
	"sync"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const pageSize = 50

type Reader interface {
	ListActivityPage(context.Context, domain.ActivityQuery) (domain.ActivityPage, error)
}

type Filters struct {
	Account    domain.AccountID
	Instrument domain.InstrumentID
	Kind       domain.ActivityKind
	From, To   string
}

func (f Filters) query(after *domain.ActivityCursor) domain.ActivityQuery {
	q := domain.ActivityQuery{FromLocalDate: f.From, ToLocalDate: f.To, After: after, Limit: pageSize}
	if f.Account != "" {
		q.AccountID = &f.Account
	}
	if f.Instrument != "" {
		q.InstrumentID = &f.Instrument
	}
	if f.Kind != "" {
		q.Kinds = []domain.ActivityKind{f.Kind}
	}
	return q
}

// Row retains the fields needed to display an activity in the list. Detail
// reads the underlying activity separately by its stable ID.
type Row struct {
	ID                 domain.ActivityID
	Kind               domain.ActivityKind
	Reason             domain.ActivityReason
	EffectiveLocalDate string
	Note               *string
}

type pageResult struct {
	rows []Row
	next *domain.ActivityCursor
	err  error
}

type pageRead struct {
	cancel context.CancelFunc
	after  *domain.ActivityCursor
	result chan pageResult
}

type Visit struct {
	filters     Filters
	rows        []Row
	next        *domain.ActivityCursor
	started     bool
	err         error
	retryAfter  *domain.ActivityCursor
	interrupted bool
}

// Model owns one cursor traversal. Replacing a query retires its response
// channel as well as cancelling its context, so a late result cannot append
// rows or overwrite the active query's status.
type Model struct {
	ctx        context.Context
	service    Reader
	invalidate func()
	workers    sync.WaitGroup
	visit      Visit
	pending    *pageRead
}

func New(ctx context.Context, service Reader, invalidate func()) *Model {
	return &Model{ctx: ctx, service: service, invalidate: invalidate}
}

func (p *Model) Visit() Visit {
	visit := p.visit
	visit.rows = slices.Clone(visit.rows)
	if p.pending != nil {
		visit.interrupted = true
		visit.retryAfter = p.pending.after
	}
	return visit
}

func (p *Model) Restore(visit Visit) {
	p.cancel()
	p.visit = visit
	p.visit.rows = slices.Clone(visit.rows)
	if visit.interrupted {
		p.read(visit.retryAfter)
	}
}

func (p *Model) SetFilters(filters Filters) {
	if p.visit.filters == filters {
		return
	}
	p.cancel()
	p.visit = Visit{filters: filters}
	p.read(nil)
}

// Update starts the first read on demand or applies a completed read.
// A true result reports a replacement of the first page.
func (p *Model) Update() bool {
	if !p.visit.started {
		p.Reload()
		return false
	}
	request := p.pending
	if request == nil {
		return false
	}
	select {
	case result := <-request.result:
		p.cancel()
		p.visit.err = result.err
		p.visit.retryAfter = request.after
		if result.err != nil {
			return false
		}
		if request.after == nil {
			p.visit.rows = result.rows
		} else {
			p.visit.rows = append(p.visit.rows, result.rows...)
		}
		p.visit.next = result.next
		return request.after == nil
	default:
		return false
	}
}

func (p *Model) Reload() {
	p.read(nil)
}

func (p *Model) Refresh() {
	p.cancel()
	p.Reload()
}

func (p *Model) LoadMore() {
	if p.HasMore() && p.visit.err == nil {
		p.read(p.visit.next)
	}
}

func (p *Model) Retry() {
	if p.visit.err != nil {
		p.read(p.visit.retryAfter)
	}
}

func (p *Model) Wait()            { p.workers.Wait() }
func (p *Model) Rows() []Row      { return p.visit.rows }
func (p *Model) Loading() bool    { return p.pending != nil }
func (p *Model) HasMore() bool    { return p.visit.next != nil }
func (p *Model) Err() error       { return p.visit.err }
func (p *Model) Filters() Filters { return p.visit.filters }

func (p *Model) cancel() {
	if p.pending != nil {
		p.pending.cancel()
		p.pending = nil
	}
}

func (p *Model) read(after *domain.ActivityCursor) {
	if p.pending != nil || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	request := &pageRead{cancel: cancel, after: after, result: make(chan pageResult, 1)}
	query := p.visit.filters.query(after)
	p.pending = request
	p.visit.started, p.visit.interrupted, p.visit.err = true, false, nil
	p.workers.Go(func() {
		page, err := p.service.ListActivityPage(ctx, query)
		if ctx.Err() != nil {
			return
		}
		result := pageResult{err: err}
		if err == nil {
			result.rows = make([]Row, len(page.Activities))
			for i, activity := range page.Activities {
				result.rows[i] = Row{
					ID: activity.ID, Kind: activity.Kind, Reason: activity.Reason,
					EffectiveLocalDate: activity.EffectiveLocalDate, Note: activity.Note,
				}
			}
			if page.HasMore {
				result.next = page.Next
			}
		}
		request.result <- result
		p.invalidate()
	})
}
