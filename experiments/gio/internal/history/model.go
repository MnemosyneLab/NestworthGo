package history

import (
	"context"
	"sync"

	"github.com/waltwang/nestworth-go/internal/domain"
)

const pageSize = 50

type Reader interface {
	ListActivityPage(context.Context, domain.ActivityQuery) (domain.ActivityPage, error)
}

// Row retains the fields needed to display an activity in the list. A detail
// view can read effects, trades and endpoint values by activity ID.
type Row struct {
	ID                 domain.ActivityID
	Kind               domain.ActivityKind
	EffectiveLocalDate string
	Note               *string
}

type pageResult struct {
	rows []Row
	next *domain.ActivityCursor
	err  error
}

// Model owns one sequential traversal of the household ledger. The reader
// delivers completed pages by channel for the UI goroutine to apply.
type Model struct {
	ctx        context.Context
	service    Reader
	invalidate func()
	workers    sync.WaitGroup
	results    chan pageResult

	started      bool
	loading      bool
	err          error
	rows         []Row
	next         *domain.ActivityCursor
	requestAfter *domain.ActivityCursor
}

func New(ctx context.Context, service Reader, invalidate func()) *Model {
	return &Model{
		ctx: ctx, service: service, invalidate: invalidate,
		results: make(chan pageResult, 1),
	}
}

// Update starts the first read on demand or applies a completed read. A true
// result tells the view to reset its scroll position to the new first page.
func (p *Model) Update() (replaced bool) {
	if !p.started {
		p.Reload()
		return false
	}
	select {
	case result := <-p.results:
		p.loading = false
		p.err = result.err
		if p.err != nil {
			return false
		}
		if p.requestAfter == nil {
			p.rows = result.rows
		} else {
			p.rows = append(p.rows, result.rows...)
		}
		p.next = result.next
		return p.requestAfter == nil
	default:
		return false
	}
}

func (p *Model) Reload() {
	p.read(nil)
}

func (p *Model) LoadMore() {
	if p.HasMore() && p.err == nil {
		p.read(p.next)
	}
}

func (p *Model) Retry() {
	if p.err != nil {
		p.read(p.requestAfter)
	}
}

func (p *Model) Wait()         { p.workers.Wait() }
func (p *Model) Rows() []Row   { return p.rows }
func (p *Model) Loading() bool { return p.loading }
func (p *Model) HasMore() bool { return p.next != nil }
func (p *Model) Err() error    { return p.err }

func (p *Model) read(after *domain.ActivityCursor) {
	if p.loading || p.ctx.Err() != nil {
		return
	}
	p.started = true
	p.loading = true
	p.err = nil
	p.requestAfter = after
	p.workers.Go(func() {
		page, err := p.service.ListActivityPage(p.ctx, domain.ActivityQuery{Limit: pageSize, After: after})
		if p.ctx.Err() != nil {
			return
		}
		result := pageResult{err: err}
		if err == nil {
			result.rows = make([]Row, len(page.Activities))
			for i, activity := range page.Activities {
				result.rows[i] = Row{
					ID: activity.ID, Kind: activity.Kind,
					EffectiveLocalDate: activity.EffectiveLocalDate, Note: activity.Note,
				}
			}
			if page.HasMore {
				result.next = page.Next
			}
		}
		// Single flight and one buffered slot let the worker finish even if
		// the user leaves this route before the next frame.
		p.results <- result
		p.invalidate()
	})
}
