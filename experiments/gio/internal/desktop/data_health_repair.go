package desktop

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type healthRepairService interface {
	ReviewMarketDataSync(context.Context, application.SyncRequest) (application.SyncPlanPreview, error)
	ConfirmMarketDataSync(context.Context, application.SyncRequest, string) (application.SyncStartResult, error)
	GetCurrentSyncJob() (application.SyncJobSnapshot, bool)
	CancelSyncJob(string) (application.SyncJobSnapshot, bool)
}

type dataHealthRepair struct {
	service      healthRepairService
	previewRead  <-chan historyResult[application.SyncPlanPreview]
	startRead    <-chan historyResult[application.SyncStartResult]
	cancelRead   <-chan historyResult[bool]
	stopPreview  context.CancelFunc
	plan         *application.SyncPlanPreview
	job          application.SyncJobSnapshot
	problem      i18n.MessageID
	revision     atomic.Uint64
	seenRevision uint64
	sheet        analysisSheet
	open         widget.Clickable
	cancel       widget.Clickable
	returnFocus  bool
	owner        routeID
	lastScope    application.SyncRequest
	latestBusy   func() bool
}

func (p *dataHealthPage) previewRepair() {
	r := p.repair
	r.owner = dataHealthRoute
	r.preview(p.ctx, &p.workers, p.invalidate, application.SyncRequest{Scope: application.SyncScopeRepairAll})
}

func (r *dataHealthRepair) preview(ctx context.Context, workers *sync.WaitGroup, invalidate func(), request application.SyncRequest) {
	r.retirePreview()
	r.problem = i18n.NoMessage
	ctx, cancel := context.WithCancel(ctx)
	r.stopPreview = cancel
	r.previewRead = runHistoryRead(workers, invalidate, func() (application.SyncPlanPreview, error) {
		return r.service.ReviewMarketDataSync(ctx, request)
	})
}

func (p *dataHealthPage) confirmRepair() {
	r := p.repair
	r.start(p.ctx, &p.workers, p.invalidate, *r.plan)
}

func (r *dataHealthRepair) start(ctx context.Context, workers *sync.WaitGroup, invalidate func(), plan application.SyncPlanPreview) {
	r.retirePreview()
	r.problem = i18n.NoMessage
	r.lastScope = plan.Scope
	r.startRead = runHistoryRead(workers, invalidate, func() (application.SyncStartResult, error) {
		return r.service.ConfirmMarketDataSync(ctx, plan.Scope, plan.ReviewID)
	})
}

func (p *dataHealthPage) cancelRepair() {
	r := p.repair
	r.cancelJob(&p.workers, p.invalidate)
}

func (r *dataHealthRepair) cancelJob(workers *sync.WaitGroup, invalidate func()) {
	id := r.job.JobID
	r.problem = i18n.NoMessage
	r.cancelRead = runHistoryRead(workers, invalidate, func() (bool, error) {
		_, ok := r.service.CancelSyncJob(id)
		return ok, nil
	})
}

func (r *dataHealthRepair) retirePreview() {
	if r.stopPreview != nil {
		r.stopPreview()
	}
	r.stopPreview, r.previewRead, r.plan = nil, nil, nil
}

func (p *dataHealthPage) updateRepair() {
	if p.repair != nil && p.repair.update() {
		p.read()
	}
}

func (r *dataHealthRepair) update() (terminal bool) {
	select {
	case response := <-r.previewRead:
		r.retirePreview()
		if response.err != nil {
			r.problem = i18n.HealthRepairPlanFailed
			var problem *domain.Error
			if errors.As(response.err, &problem) && problem.Code == domain.ErrMissingHistoryOrigin {
				r.problem = i18n.HealthRepairHistoryRequired
			}
		} else {
			r.plan = &response.value
			r.sheet = newAnalysisSheet()
		}
	case response := <-r.startRead:
		r.startRead = nil
		var problem *domain.Error
		switch {
		case errors.As(response.err, &problem) && problem.Code == domain.ErrStalePreview:
			r.problem = i18n.HealthRepairStale
		case response.err != nil:
			r.problem = i18n.HealthRepairStartFailed
		case response.value.Conflict:
			r.problem = i18n.HealthRepairConflict
		}
		r.revision.Add(1)
	case response := <-r.cancelRead:
		r.cancelRead = nil
		if !response.value {
			r.problem = i18n.HealthRepairCancelFailed
		}
		r.revision.Add(1)
	default:
	}
	if revision := r.revision.Load(); revision != r.seenRevision {
		r.seenRevision = revision
		job, exists := r.service.GetCurrentSyncJob()
		terminal = exists && job.JobID != "" && job.Outcome != application.SyncOutcomeRunning &&
			(job.JobID != r.job.JobID || r.job.Outcome == application.SyncOutcomeRunning)
		r.job = job
	}
	return terminal
}

func (r *dataHealthRepair) inProgress() bool {
	return r.previewRead != nil || r.startRead != nil || r.job.Outcome == application.SyncOutcomeRunning ||
		r.latestBusy != nil && r.latestBusy()
}

func (p *dataHealthPage) repairAction(gtx layout.Context, view ui.View) layout.Dimensions {
	r := p.repair
	if r.returnFocus {
		r.returnFocus = false
		gtx.Execute(key.FocusCmd{Tag: &r.open})
	}
	active := p.request == nil && p.err == nil && p.report.ExecutableCount > 0 && !r.inProgress()
	if r.open.Clicked(gtx) && active {
		p.previewRepair()
		gtx.Execute(op.InvalidateCmd{})
	}
	label := i18n.HealthRepairAll
	if r.previewRead != nil {
		label = i18n.HealthRepairPlanning
	}
	return returnMonthButton(view, &r.open, label, nil, active)(gtx)
}
