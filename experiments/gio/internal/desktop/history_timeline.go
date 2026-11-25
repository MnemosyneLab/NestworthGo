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
	cashMovementService
	historyStartLedger
}

type historyTimelineVisit struct {
	model    history.Visit
	scroll   layout.Position
	revision uint64
	focused  domain.ActivityID
}

type historyDetailState struct {
	scroll     layout.Position
	account    domain.AccountID
	instrument domain.InstrumentID
}

type historyDetailVisit struct {
	id               domain.ActivityID
	pending          <-chan historyResult[history.Detail]
	cancel           context.CancelFunc
	record           *history.Detail
	err              error
	dirty            bool
	back             widget.Clickable
	retry            widget.Clickable
	scroll           widget.List
	amendment        *historyAmendmentDraft
	correct, reverse widget.Clickable
	repeat           widget.Clickable
	repeatDraft      *historyRepeatDraft
	linked           [4]widget.Clickable
	accounts         map[domain.AccountID]*widget.Clickable
	instruments      map[domain.InstrumentID]*widget.Clickable
}

func (v *historyDetailVisit) accountButton(id domain.AccountID) *widget.Clickable {
	if v.accounts[id] == nil {
		v.accounts[id] = new(widget.Clickable)
	}
	return v.accounts[id]
}

func (v *historyDetailVisit) state(focus event.Tag) historyDetailState {
	state := historyDetailState{scroll: v.scroll.Position}
	for id, button := range v.accounts {
		if focus == button {
			state.account = id
			return state
		}
	}
	for id, button := range v.instruments {
		if focus == button {
			state.instrument = id
			break
		}
	}
	return state
}

type historyReads struct {
	ctx        context.Context
	service    history.AmendmentLedger
	repeatAPI  historyRepeatAPI
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup

	references     *history.References
	refsRead       <-chan historyResult[history.References]
	refsCancel     context.CancelFunc
	refsErr        error
	detail         *historyDetailVisit
	buttons        map[domain.ActivityID]*widget.Clickable
	posted         domain.Activity
	postedNotice   i18n.MessageID
	open           func(id domain.ActivityID, focus event.Tag)
	openAccount    func(id domain.AccountID, name string, focus event.Tag)
	openInstrument func(id domain.InstrumentID, name string, focus event.Tag)
	openProduct    func(id domain.ProductContractID, name string, focus event.Tag)
	back           func()
	backToAccount  func() bool
	rename         func(id domain.ActivityID, name string)
}

type historyPageReader struct{ service history.AmendmentLedger }

func (r historyPageReader) ListActivityPage(ctx context.Context, query domain.ActivityQuery) (out domain.ActivityPage, err error) {
	err = r.service.WithRead(ctx, func(ctx context.Context) error {
		out, err = r.service.ListActivityPage(ctx, query)
		return err
	})
	return out, err
}

func newLedgerHistory(ctx context.Context, service historyService, clock func() time.Time, invalidate func()) *historyPage {
	page := newHistoryPage(history.New(ctx, historyPageReader{service}, invalidate))
	page.cashMovements = newHistoryCashMovements(ctx, service, clock, invalidate)
	page.startPoint = newHistoryStart(service)
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
	ctx, cancel := context.WithCancel(r.ctx)
	r.refsCancel = cancel
	r.refsRead = runHistoryRead(&r.workers, r.invalidate, func() (history.References, error) {
		var refs history.References
		err := r.service.WithRead(ctx, func(ctx context.Context) error {
			var err error
			refs, err = history.LoadReferences(ctx, r.service)
			return err
		})
		return refs, err
	})
}

func (r *historyReads) retireReferences() {
	if r.refsCancel != nil {
		r.refsCancel()
	}
	r.refsCancel, r.refsRead = nil, nil
}

func (v *historyDetailVisit) retire() {
	if v.cancel != nil {
		v.cancel()
	}
	v.cancel, v.pending = nil, nil
}

func (r *historyReads) leaveDetail() {
	if r.detail != nil {
		r.detail.retire()
		r.detail = nil
	}
}

func (r *historyReads) enterDetail(id domain.ActivityID, state historyDetailState) event.Tag {
	r.detail = &historyDetailVisit{id: id,
		scroll:      widget.List{List: layout.List{Axis: layout.Vertical, Position: state.scroll}},
		accounts:    make(map[domain.AccountID]*widget.Clickable),
		instruments: make(map[domain.InstrumentID]*widget.Clickable)}
	r.loadDetail()
	if state.account != "" {
		return r.detail.accountButton(state.account)
	}
	if state.instrument != "" {
		return portfolioButton(r.detail.instruments, state.instrument)
	}
	return nil
}

func (r *historyReads) loadDetail() {
	visit := r.detail
	visit.retire()
	ctx, cancel := context.WithCancel(r.ctx)
	visit.cancel, visit.err, visit.dirty = cancel, nil, false
	id := visit.id
	visit.pending = runHistoryRead(&r.workers, r.invalidate, func() (history.Detail, error) {
		var detail history.Detail
		err := r.service.WithRead(ctx, func(ctx context.Context) error {
			var err error
			detail, err = history.LoadDetail(ctx, r.service, id)
			return err
		})
		return detail, err
	})
}

func (r *historyReads) update() {
	select {
	case result := <-r.refsRead:
		r.retireReferences()
		r.refsErr = result.err
		if result.err == nil {
			r.references = &result.value
		}
	default:
	}
	if visit := r.detail; visit != nil {
		if visit.dirty {
			r.loadDetail()
		}
		select {
		case result := <-visit.pending:
			visit.retire()
			visit.err = result.err
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

func (p *historyPage) enterTimeline(id uint64, visited bool, visit historyTimelineVisit, filters history.Filters) {
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
		if !p.model.Filters().Empty() {
			p.model.Restore(history.Visit{})
		}
		if !filters.Empty() {
			p.model.SetFilters(filters)
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
	p.pollStartingPoint(p.startPoint)
	if p.reads != nil {
		p.pollAmendment()
		p.pollRepeat()
	}
	if p.cashMovements == nil {
		return
	}
	var focus event.Tag = &p.list
	if d := p.cashMovements.draft; d != nil {
		focus = &p.cashMovements.actions[d.movement]
	}
	activity := p.cashMovements.update()
	if activity == nil {
		return
	}
	p.showPosted(*activity, i18n.NoMessage, focus)
}

func (p *historyPage) invalidateLedgerReads() {
	p.revision++
	p.model.Refresh()
	if r := p.reads; r != nil {
		if r.refsRead != nil || r.references != nil || r.refsErr != nil {
			r.retireReferences()
			r.loadReferences()
		}
		if r.detail != nil {
			r.detail.retire()
			r.detail.dirty = true
		}
	}
}

func (p *historyPage) ledgerChanged() {
	if p.onPosted != nil {
		p.onPosted()
	} else {
		p.invalidateLedgerReads()
	}
}

func (p *historyPage) showPosted(activity domain.Activity, notice i18n.MessageID, focus event.Tag) {
	p.ledgerChanged()
	p.reads.posted, p.reads.postedNotice = activity, notice
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
	if kind == domain.ActivityCashIn && reason == domain.ReasonCardPurchase {
		return i18n.LiabilityCardPurchase
	}
	if kind == domain.ActivityCashOut && reason == domain.ReasonCardRefund {
		return i18n.LiabilityCardRefund
	}
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
