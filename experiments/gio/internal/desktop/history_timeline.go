package desktop

import (
	"context"
	"slices"
	"sync"
	"time"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/history"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type historyService interface {
	history.AmendmentLedger
	history.CashMovementLedger
	historyStartLedger
}

type historyTimelineVisit struct {
	model    history.Visit
	scroll   layout.Position
	revision uint64
	focused  domain.ActivityID
}

type historyDetailVisit struct {
	id               domain.ActivityID
	pending          <-chan historyResult[history.Detail]
	cancel           context.CancelFunc
	record           *history.Detail
	err              error
	back             widget.Clickable
	retry            widget.Clickable
	scroll           widget.List
	amendment        *historyAmendmentDraft
	correct, reverse widget.Clickable
	repeat           widget.Clickable
	repeatDraft      *historyRepeatDraft
	linked           [4]widget.Clickable
}

type historyReads struct {
	ctx        context.Context
	service    history.AmendmentLedger
	repeatAPI  historyRepeatAPI
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup

	references   *history.References
	refsRead     <-chan historyResult[history.References]
	refsErr      error
	detail       *historyDetailVisit
	buttons      map[domain.ActivityID]*widget.Clickable
	posted       domain.Activity
	postedNotice i18n.MessageID
	open         func(id domain.ActivityID, focus event.Tag)
	back         func()
	rename       func(id domain.ActivityID, name string)
}

func newLedgerHistory(ctx context.Context, service historyService, clock func() time.Time, invalidate func()) *historyPage {
	page := newHistoryPage(history.New(ctx, service, invalidate))
	page.cashMovements = newHistoryCashMovements(ctx, service, clock, invalidate)
	page.startPoint = &historyStart{service: service, balances: widget.List{List: layout.List{Axis: layout.Vertical}}}
	page.reads = &historyReads{
		ctx: ctx, service: service, clock: clock, invalidate: invalidate,
		buttons: make(map[domain.ActivityID]*widget.Clickable),
	}
	return page
}

type historyResult[T any] struct {
	value T
	err   error
}

func runHistoryRead[T any](workers *sync.WaitGroup, invalidate func(), load func() (T, error)) <-chan historyResult[T] {
	result := make(chan historyResult[T], 1)
	workers.Go(func() {
		value, err := load()
		result <- historyResult[T]{value: value, err: err}
		invalidate()
	})
	return result
}

func (r *historyReads) loadReferences() {
	if r.refsRead != nil {
		return
	}
	r.refsErr = nil
	r.refsRead = runHistoryRead(&r.workers, r.invalidate, func() (history.References, error) {
		return history.LoadReferences(r.ctx, r.service)
	})
}

func (r *historyReads) leaveDetail() {
	if r.detail != nil {
		r.detail.cancel()
		r.detail = nil
	}
}

func (r *historyReads) enterDetail(id domain.ActivityID) {
	r.detail = &historyDetailVisit{id: id, scroll: widget.List{List: layout.List{Axis: layout.Vertical}}}
	r.loadDetail()
}

func (r *historyReads) loadDetail() {
	visit := r.detail
	if visit.cancel != nil {
		visit.cancel()
	}
	ctx, cancel := context.WithCancel(r.ctx)
	visit.cancel, visit.err = cancel, nil
	id := visit.id
	visit.pending = runHistoryRead(&r.workers, r.invalidate, func() (history.Detail, error) {
		return history.LoadDetail(ctx, r.service, id)
	})
}

func (r *historyReads) update() {
	select {
	case result := <-r.refsRead:
		r.refsRead, r.refsErr = nil, result.err
		if result.err == nil {
			r.references = &result.value
		}
	default:
	}
	if visit := r.detail; visit != nil {
		select {
		case result := <-visit.pending:
			visit.pending, visit.err = nil, result.err
			if result.err == nil {
				visit.record = &result.value
			}
		default:
		}
	}
}

func (p *historyPage) timelineVisit(focus event.Tag) historyTimelineVisit {
	visit := historyTimelineVisit{model: p.model.Visit(), scroll: p.list.Position, revision: p.revision}
	if p.reads != nil {
		for id, button := range p.reads.buttons {
			if focus == button {
				visit.focused = id
				break
			}
		}
	}
	return visit
}

func (p *historyPage) enterTimeline(id uint64, visited bool, visit historyTimelineVisit) {
	if p.bound == id {
		return
	}
	p.bound = id
	if visited {
		p.model.Restore(visit.model)
		p.list.Position = restoredPosition(visit.scroll, len(p.model.Rows()))
		if visit.revision != p.revision {
			p.model.Refresh()
		}
	} else {
		if p.model.Filters() != (history.Filters{}) {
			p.model.Restore(history.Visit{})
		}
		p.list.Position = layout.Position{}
	}
	if p.reads != nil {
		p.pruneDetailButtons()
		p.reads.loadReferences()
	}
}

// Poll before the shell handles navigation so acknowledgement can open the exact
// saved ID, even when the active query excludes it.
func (p *historyPage) pollPosted() {
	p.pollStartingPoint()
	if p.reads != nil {
		p.pollAmendment()
		p.pollRepeat()
	}
	if p.cashMovements == nil {
		return
	}
	activity := p.cashMovements.update()
	if activity == nil {
		return
	}
	p.showPosted(*activity, i18n.NoMessage, &p.list)
}

func (p *historyPage) showPosted(activity domain.Activity, notice i18n.MessageID, focus event.Tag) {
	p.revision++
	p.model.Refresh()
	p.reads.posted, p.reads.postedNotice = activity, notice
	if p.onPosted != nil {
		p.onPosted()
	}
	p.reads.open(activity.ID, focus)
}

func (p *historyPage) detailButton(id domain.ActivityID) *widget.Clickable {
	if p.reads.buttons[id] == nil {
		p.reads.buttons[id] = new(widget.Clickable)
	}
	return p.reads.buttons[id]
}

func (p *historyPage) pruneDetailButtons() {
	rows := p.model.Rows()
	for id := range p.reads.buttons {
		if !slices.ContainsFunc(rows, func(row history.Row) bool { return row.ID == id }) {
			delete(p.reads.buttons, id)
		}
	}
}

func historyKindLabel(kind domain.ActivityKind, reason domain.ActivityReason) i18n.MessageID {
	if (kind == domain.ActivityCashIn || kind == domain.ActivityCashOut) && reason == domain.ReasonReconciliation {
		return i18n.HistoryCashReconciled
	}
	if kind == domain.ActivityCashIn && reason == domain.ReasonContribution {
		return i18n.ContributionTitle
	}
	if label, ok := historyKindLabels[kind]; ok {
		return label
	}
	return i18n.HistoryActivity
}
