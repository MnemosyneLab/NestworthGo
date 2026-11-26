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
	Kinds      []domain.ActivityKind
	From, To   string
}

func (f Filters) Equal(other Filters) bool {
	return f.Account == other.Account && f.Instrument == other.Instrument &&
		f.Kind == other.Kind && slices.Equal(f.Kinds, other.Kinds) &&
		f.From == other.From && f.To == other.To
}

func (f Filters) Empty() bool {
	return f.Equal(Filters{})
}

func (f Filters) query(after *domain.ActivityCursor) domain.ActivityQuery {
	q := domain.ActivityQuery{FromLocalDate: f.From, ToLocalDate: f.To, After: after, Limit: pageSize}
	if f.Account != "" {
		q.AccountID = &f.Account
	}
	if f.Instrument != "" {
		q.InstrumentID = &f.Instrument
	}
	if len(f.Kinds) != 0 {
		q.Kinds = slices.Clone(f.Kinds)
	} else if f.Kind != "" {
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
	query  pageQuery
	result chan pageResult
}

type pageQuery struct {
	after   *domain.ActivityCursor
	through domain.ActivityID
	minimum int
}

type Visit struct {
	filters     Filters
	rows        []Row
	next        *domain.ActivityCursor
	started     bool
	err         error
	retry       pageQuery
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
		visit.retry = p.pending.query
	}
	return visit
}

func (p *Model) Restore(visit Visit) {
	p.cancel()
	p.visit = visit
	p.visit.rows = slices.Clone(visit.rows)
	if visit.interrupted {
		p.read(visit.retry)
	}
}

func (p *Model) SetFilters(filters Filters) {
	if p.visit.filters.Equal(filters) {
		return
	}
	p.cancel()
	filters.Kinds = slices.Clone(filters.Kinds)
	p.visit = Visit{filters: filters}
	p.Reload()
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
		p.visit.retry = request.query
		if result.err != nil {
			return false
		}
		if request.query.after == nil {
			p.visit.rows = result.rows
		} else {
			p.visit.rows = append(p.visit.rows, result.rows...)
		}
		p.visit.next = result.next
		return request.query.after == nil
	default:
		return false
	}
}

func (p *Model) Reload() {
	p.read(pageQuery{})
}

// Refresh keeps the visited timeline range when a write invalidates it.
// Start at the head so no continuation cursor comes from an earlier version.
func (p *Model) Refresh() {
	p.cancel()
	reload := pageQuery{}
	if rows := p.visit.rows; len(rows) != 0 {
		reload.through = rows[len(rows)-1].ID
		reload.minimum = len(rows)
	}
	p.read(reload)
}

func (p *Model) LoadMore() {
	if p.HasMore() && p.visit.err == nil {
		p.read(pageQuery{after: p.visit.next})
	}
}

func (p *Model) Retry() {
	if p.visit.err != nil {
		p.read(p.visit.retry)
	}
}

func (p *Model) Wait()         { p.workers.Wait() }
func (p *Model) Rows() []Row   { return p.visit.rows }
func (p *Model) Loading() bool { return p.pending != nil }
func (p *Model) HasMore() bool { return p.visit.next != nil }
func (p *Model) Err() error    { return p.visit.err }
func (p *Model) Filters() Filters {
	filters := p.visit.filters
	filters.Kinds = slices.Clone(filters.Kinds)
	return filters
}

func (p *Model) cancel() {
	if p.pending != nil {
		p.pending.cancel()
		p.pending = nil
	}
}

func (p *Model) read(page pageQuery) {
	if p.pending != nil || p.ctx.Err() != nil {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	request := &pageRead{cancel: cancel, query: page, result: make(chan pageResult, 1)}
	query := p.visit.filters.query(page.after)
	p.pending = request
	p.visit.started, p.visit.interrupted, p.visit.err = true, false, nil
	p.workers.Go(func() {
		result := p.readRange(ctx, query, page)
		if ctx.Err() != nil {
			return
		}
		request.result <- result
		p.invalidate()
	})
}

func (p *Model) readRange(ctx context.Context, query domain.ActivityQuery, window pageQuery) pageResult {
	var result pageResult
	for {
		page, err := p.service.ListActivityPage(ctx, query)
		if err != nil {
			return pageResult{err: err}
		}
		for i, activity := range page.Activities {
			result.rows = append(result.rows, Row{
				ID: activity.ID, Kind: activity.Kind, Reason: activity.Reason,
				EffectiveLocalDate: activity.EffectiveLocalDate, Note: activity.Note,
			})
			if window.through != "" && activity.ID == window.through {
				if len(result.rows) < pageSize {
					window.through = ""
					continue
				}
				if i+1 < len(page.Activities) || page.HasMore {
					result.next = &domain.ActivityCursor{ID: activity.ID, EffectiveAt: activity.EffectiveAt, CreatedAt: activity.CreatedAt}
				}
				return result
			}
		}
		if !page.HasMore {
			return result
		}
		// A missing anchor may follow a restore. Bound the work to the
		// former window and one page for newly inserted rows.
		if window.through == "" || page.Next == nil || len(result.rows) >= window.minimum+pageSize || ctx.Err() != nil {
			result.next = page.Next
			return result
		}
		query.After = page.Next
	}
}
