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
	history.PositionSourceReader
	cashMovementService
	historyStartLedger
}

type historyTimelineVisit struct {
	model        history.Visit
	scroll       layout.Position
	revision     uint64
	focused      domain.ActivityID
	focusActions bool
	dateAt       analysisDateWindow
	dateMode     int
}

type historyDetailState struct {
	scroll     layout.Position
	account    domain.AccountID
	instrument domain.InstrumentID
	linked     int
}

type historyDetailVisit struct {
	id                domain.ActivityID
	pending           <-chan historyResult[history.Detail]
	cancel            context.CancelFunc
	record            *history.Detail
	err               error
	dirty             bool
	back              widget.Clickable
	retry             widget.Clickable
	scroll            widget.List
	amendment         *historyAmendmentDraft
	amendmentFocus    *widget.Clickable
	correct, reverse  widget.Clickable
	repeat            widget.Clickable
	repeatDraft       *historyRepeatDraft
	requested         historyRowAction
	fromRowAction     bool
	actionUnavailable bool
	linked            [4]widget.Clickable
	accounts          map[domain.AccountID]*widget.Clickable
	instruments       map[domain.InstrumentID]*widget.Clickable
}

func (v *historyDetailVisit) accountButton(id domain.AccountID) *widget.Clickable {
	if v.accounts[id] == nil {
		v.accounts[id] = new(widget.Clickable)
	}
	return v.accounts[id]
}

func (v *historyDetailVisit) state(focus event.Tag) historyDetailState {
	state := historyDetailState{scroll: v.scroll.Position}
	for i := range v.linked {
		if focus == &v.linked[i] {
			state.linked = i + 1
			return state
		}
	}
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

type historyTimelineReferences struct {
	history.References
	holdingAccounts map[domain.HoldingID]domain.AccountID
}

type historyReads struct {
	ctx        context.Context
	service    historyService
	repeatAPI  historyRepeatAPI
	clock      func() time.Time
	invalidate func()
	workers    sync.WaitGroup

	references      *history.References
	rowNames        historyRowNames
	refsRead        <-chan historyResult[historyTimelineReferences]
	refsCancel      context.CancelFunc
	refsErr         error
	detail          *historyDetailVisit
	buttons         map[domain.ActivityID]*widget.Clickable
	actionButtons   map[domain.ActivityID]*widget.Clickable
	rowActions      *historyRowActions
	posted          domain.Activity
	postedNotice    i18n.MessageID
	open            func(id domain.ActivityID, focus event.Tag)
	openAction      func(id domain.ActivityID, action historyRowAction, focus event.Tag)
	openAccount     func(id domain.AccountID, name string, focus event.Tag)
	openInstrument  func(id domain.InstrumentID, name string, focus event.Tag)
	openProduct     func(id domain.ProductContractID, name string, focus event.Tag)
	back            func()
	postedBackLabel func() i18n.MessageID
	rename          func(id domain.ActivityID, name string)
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
		buttons: make(map[domain.ActivityID]*widget.Clickable), actionButtons: make(map[domain.ActivityID]*widget.Clickable),
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
	r.refsRead = runHistoryRead(&r.workers, r.invalidate, func() (historyTimelineReferences, error) {
		var refs historyTimelineReferences
		err := r.service.WithRead(ctx, func(ctx context.Context) error {
			references, err := history.LoadReferences(ctx, r.service)
			if err != nil {
				return err
			}
			refs.References = references
			// Holding names enrich the timeline. If the optional read fails,
			// keep the exact quantities and let the normal Reload retry it.
			holdings, err := history.LoadHoldingReferences(ctx, r.service, references.Accounts)
			if err == nil {
				refs.holdingAccounts = make(map[domain.HoldingID]domain.AccountID)
				for account, rows := range holdings {
					for _, holding := range rows {
						refs.holdingAccounts[holding.ID] = account
					}
				}
			}
			return nil
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
	if state.linked > 0 {
		return &r.detail.linked[state.linked-1]
	}
	return nil
}

func (r *historyReads) loadDetail() {
	visit := r.detail
	visit.retire()
	ctx, cancel := context.WithCancel(r.ctx)
	visit.cancel, visit.err, visit.dirty = cancel, nil, false
	visit.pending = r.readDetail(ctx, visit.id)
}

func (r *historyReads) readDetail(ctx context.Context, id domain.ActivityID) <-chan historyResult[history.Detail] {
	return runHistoryRead(&r.workers, r.invalidate, func() (history.Detail, error) {
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
			r.references = &result.value.References
			r.rowNames = newHistoryRowNames(result.value.References)
			r.rowNames.holdingAccounts = result.value.holdingAccounts
		}
	default:
	}
	r.updateRowActions()
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
				r.startRequestedRowAction(visit)
			}
		default:
		}
	}
}

func (p *historyPage) timelineVisit(focus event.Tag) historyTimelineVisit {
	visit := historyTimelineVisit{
		model: p.model.Visit(), scroll: p.list.Position, revision: p.revision,
		dateAt: p.filter.dates.at, dateMode: p.filter.dates.chosen,
	}
	if p.reads != nil {
		for id, button := range p.reads.actionButtons {
			if focus == button {
				visit.focused, visit.focusActions = id, true
				return visit
			}
		}
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
	p.filter.dates.at = analysisDateWindow{}
	if visited {
		p.filter.dates.at, p.filter.dates.chosen = visit.dateAt, visit.dateMode
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
	if p.investment != nil {
		p.investment.poll(p.investmentRuntime())
	}
	if p.cashMovements != nil {
		if activity := p.cashMovements.update(); activity != nil {
			p.recordPosted(*activity, i18n.NoMessage, &p.record.trigger)
		}
	}
}

func (p *historyPage) invalidateLedgerReads() {
	p.revision++
	p.model.Refresh()
	if r := p.reads; r != nil {
		r.closeRowActions()
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
	for id := range p.reads.actionButtons {
		if !slices.ContainsFunc(rows, func(row history.Row) bool { return row.ID == id }) {
			delete(p.reads.actionButtons, id)
		}
	}
	if menu := p.reads.rowActions; menu != nil && p.reads.actionButtons[menu.id] == nil {
		p.reads.closeRowActions()
	}
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

func liabilityKindLabel(kind domain.ActivityKind, reason domain.ActivityReason) i18n.MessageID {
	switch kind {
	case domain.ActivityCashIn:
		if reason != domain.ReasonCardPurchase {
			return i18n.LiabilityDebtIncrease
		}
	case domain.ActivityCashOut:
		if reason != domain.ReasonCardRefund {
			return i18n.LiabilityDebtDecrease
		}
	}
	return historyKindLabel(kind, reason)
}
