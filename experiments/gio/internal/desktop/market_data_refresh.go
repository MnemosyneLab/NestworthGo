package desktop

import (
	"context"
	"sync"

	"gioui.org/io/event"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type marketLatestService interface {
	RefreshMissingOrStale(context.Context) (application.RefreshResult, error)
	RefreshAll(context.Context) (application.RefreshResult, error)
	RefreshInstrument(context.Context, domain.InstrumentID) (application.RefreshResult, error)
	RefreshFX(context.Context, string, string) (application.RefreshResult, error)
}

type marketLatestTarget struct {
	force      bool
	instrument domain.InstrumentID
	pair       marketFXPair
}

type marketLatestResult struct {
	result    application.RefreshResult
	cancelled bool
}

type marketLatest struct {
	service               marketLatestService
	request               <-chan historyResult[marketLatestResult]
	stop                  context.CancelFunc
	stopping              bool
	progress              *marketLatestProgress
	last                  marketLatestTarget
	result                *marketLatestResult
	err                   error
	update, force, cancel widget.Clickable
	retry, details, more  widget.Clickable
	moreOpen              bool
}

type marketLatestProgress struct {
	mu      sync.Mutex
	current *application.SyncItemProgress
	items   []application.SyncItemProgress
}

func (p *marketLatestProgress) record(item application.SyncItemProgress) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if item.Status == application.SyncOutcomeRunning {
		p.current = &item
		return
	}
	p.current = nil
	p.items = append(p.items, item)
}

func (p *marketLatestProgress) itemsSnapshot() []application.SyncItemProgress {
	p.mu.Lock()
	defer p.mu.Unlock()
	var rows []application.SyncItemProgress
	if p.current != nil {
		rows = append(rows, *p.current)
	}
	return append(rows, p.items...)
}

func (p *marketDataPage) startLatest(target marketLatestTarget) {
	r := &p.latest
	if r.service == nil || p.providerBusy() {
		return
	}
	ctx, stop := context.WithCancel(p.ctx)
	progress := &marketLatestProgress{}
	r.stop, r.progress, r.last, r.result, r.err = stop, progress, target, nil, nil
	r.stopping = false
	tracked := application.WithRefreshProgress(ctx, func(item application.SyncItemProgress) {
		progress.record(item)
		p.invalidate()
	})
	r.request = runHistoryRead(&p.workers, p.invalidate, func() (marketLatestResult, error) {
		var data application.RefreshResult
		var err error
		switch {
		case target.instrument != "":
			data, err = r.service.RefreshInstrument(tracked, target.instrument)
		case target.pair != (marketFXPair{}):
			data, err = r.service.RefreshFX(tracked, target.pair.first.String(), target.pair.second.String())
		case target.force:
			data, err = r.service.RefreshAll(tracked)
		default:
			data, err = r.service.RefreshMissingOrStale(tracked)
		}
		return marketLatestResult{result: data, cancelled: ctx.Err() != nil}, err
	})
}

func (p *marketDataPage) updateLatest() bool {
	r := &p.latest
	select {
	case response := <-r.request:
		r.stop()
		r.request, r.stop, r.stopping = nil, nil, false
		r.result, r.err = &response.value, response.err
		return true
	default:
		return false
	}
}

func (p *marketDataPage) providerBusy() bool {
	return p.latest.request != nil || p.sync != nil && p.sync.inProgress()
}

type marketOperationKind uint8

const (
	marketSyncPreview marketOperationKind = iota
	marketSyncDetails
	marketLatestDetails
)

type marketOperationSheet struct {
	kind    marketOperationKind
	sheet   analysisSheet
	source  event.Tag
	request application.SyncRequest
	force   widget.Clickable
}

func (p *marketDataPage) openMarketOperation(kind marketOperationKind, from event.Tag) {
	p.operation = &marketOperationSheet{kind: kind, sheet: newAnalysisSheet(), source: from}
}

func (p *marketDataPage) closeMarketOperation() {
	if p.operation != nil && p.operation.kind == marketSyncPreview {
		p.sync.retirePreview()
		p.sync.problem = i18n.NoMessage
	}
	p.operation = nil
}

func (p *marketDataPage) previewMarketSync(request application.SyncRequest, from event.Tag) {
	if p.providerBusy() {
		return
	}
	p.openMarketOperation(marketSyncPreview, from)
	p.operation.request = request
	p.loadMarketSyncPreview()
}

func (p *marketDataPage) loadMarketSyncPreview() {
	r := p.sync
	r.owner = marketDataRoute
	r.preview(p.ctx, &p.workers, p.invalidate, p.operation.request)
}
