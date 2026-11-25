package desktop

import (
	"fmt"
	"strings"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

func marketWorkStatus(status string) i18n.MessageID {
	switch status {
	case "planned", "pending":
		return i18n.MarketWorkPlanned
	case "running":
		return i18n.MarketWorkRunning
	case "fetched", "committed":
		return i18n.MarketWorkFetched
	case "cached":
		return i18n.MarketWorkCached
	case "skipped":
		return i18n.MarketWorkSkipped
	case "failed":
		return i18n.MarketWorkFailed
	case "rate_limited":
		return i18n.MarketWorkRateLimited
	case "cancelled":
		return i18n.MarketWorkCancelled
	case "partial":
		return i18n.HealthRepairPartial
	}
	return i18n.NoMessage
}

func marketWorkLines(view ui.View, item application.SyncItemProgress) []string {
	lines := syncItemLines(view, item)
	if status := marketWorkStatus(item.Status); status != i18n.NoMessage {
		lines = append(lines, view.Language.Text(status))
	}
	if reason := healthReason(item.Detail); reason != i18n.NoMessage {
		lines = append(lines, view.Language.Text(reason))
	}
	return lines
}

func (p *marketDataPage) marketOperations(gtx layout.Context, view ui.View, reload i18n.MessageID) layout.Dimensions {
	r, job := &p.latest, p.sync
	busy := p.providerBusy()
	button := func(control *widget.Clickable, label i18n.MessageID, active bool, action func()) layout.Widget {
		if active && control.Clicked(gtx) {
			action()
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, control, label, nil, active)
	}
	var actions []layout.Widget
	if r.service != nil {
		if !busy && r.update.Clicked(gtx) {
			p.startLatest(marketLatestTarget{})
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, func(gtx layout.Context) layout.Dimensions {
			if busy {
				gtx = gtx.Disabled()
			}
			view.Focus.Watch(gtx, &r.update)
			title := view.Language.Text(i18n.MarketUpdateLatest)
			return view.ButtonOf(&r.update, ui.ButtonSpec{Title: title, Name: title, Size: ui.ButtonSmall, Variant: ui.ButtonPrimary})(gtx)
		})
	}
	actions = append(actions, button(&r.more, i18n.MarketMoreActions, !busy, func() { r.moreOpen = !r.moreOpen }))
	if r.request != nil {
		actions = append(actions, button(&r.cancel, i18n.MarketCancelLatest, !r.stopping, func() { r.stopping = true; r.stop() }))
	}
	if r.err != nil || r.result != nil && (r.result.cancelled || latestFailures(r.result.result) != 0) {
		actions = append(actions, button(&r.retry, i18n.MarketRetryLatest, !busy, func() { p.startLatest(r.last) }))
	}
	if r.progress != nil {
		actions = append(actions, button(&r.details, i18n.MarketLatestDetails, true, func() { p.openMarketOperation(marketLatestDetails, &r.details) }))
	}
	if job != nil {
		if job.job.JobID != "" {
			actions = append(actions, button(&p.syncDetails, i18n.MarketSyncDetails, true, func() { p.openMarketOperation(marketSyncDetails, &p.syncDetails) }))
		}
		if job.job.Outcome == application.SyncOutcomeRunning {
			actions = append(actions, button(&job.cancel, i18n.HealthRepairCancel, job.cancelRead == nil, func() { job.cancelJob(&p.workers, p.invalidate) }))
		}
		if (job.owner == marketDataRoute && job.problem != i18n.NoMessage && p.lastSync.Scope != "") || marketSyncRetryable(job.job.Outcome) {
			actions = append(actions, button(&p.retrySync, i18n.MarketRetrySync, !busy, func() {
				request := job.job.Scope
				if job.problem != i18n.NoMessage && p.lastSync.Scope != "" {
					request = p.lastSync
				}
				p.previewMarketSync(request, &p.retrySync)
			}))
		}
	}
	actions = append(actions, returnMonthButton(view, &p.reload, reload, nil, p.request == nil))
	rows := []ui.ColumnChild{ui.Rigid(marketActionRow(view, actions))}
	if r.moreOpen {
		var extra []layout.Widget
		if job != nil {
			extra = append(extra, button(&p.previewSync, i18n.MarketSyncRepair, !busy, func() {
				p.previewMarketSync(application.SyncRequest{Scope: application.SyncScopeRepairAll}, &p.previewSync)
			}))
		}
		if r.service != nil {
			extra = append(extra, button(&r.force, i18n.MarketForceLatest, !busy, func() { p.startLatest(marketLatestTarget{force: true}) }))
		}
		if p.openProviderSettings != nil {
			extra = append(extra, button(&p.providerSettings, i18n.HealthOpenSettings, true, func() { p.openProviderSettings(&p.providerSettings) }))
		}
		rows = append(rows, ui.Rigid(marketActionRow(view, extra)))
	}
	for _, line := range append(p.marketLatestStatus(view), p.marketHistoricalStatus(view)...) {
		rows = append(rows, ui.Rigid(view.Message(line, view.Colors.SecondaryInk)))
	}
	return (ui.Column{Gap: 6}).Layout(gtx, rows...)
}

func marketActionRow(view ui.View, actions []layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Wrap{Gap: 6}).Layout(gtx, len(actions), func(gtx layout.Context, index int) layout.Dimensions { return actions[index](gtx) })
	}
}

func (p *marketDataPage) marketTargetActions(gtx layout.Context, view ui.View, latest marketLatestTarget, history application.SyncRequest) []layout.Widget {
	if p.latest.service == nil && p.sync == nil {
		return nil
	}
	if p.quoteActions == nil {
		p.quoteActions = make(map[string]*widget.Clickable)
	}
	key := history.InstrumentID + "/" + history.CurrencyA + "/" + history.CurrencyB
	ready := p.request == nil && p.err == nil && !p.providerBusy()
	var buttons []layout.Widget
	if p.latest.service != nil {
		control := portfolioButton(p.quoteActions, key+"/1")
		if ready && control.Clicked(gtx) {
			p.startLatest(latest)
			gtx.Execute(op.InvalidateCmd{})
		}
		buttons = append(buttons, returnMonthButton(view, control, i18n.MarketUpdateTarget, nil, ready))
	}
	if p.sync != nil {
		control := portfolioButton(p.quoteActions, key+"/2")
		if ready && control.Clicked(gtx) {
			p.previewMarketSync(history, control)
			gtx.Execute(op.InvalidateCmd{})
		}
		buttons = append(buttons, returnMonthButton(view, control, i18n.MarketPreviewTarget, nil, ready))
	}
	return buttons
}

func marketSyncRetryable(outcome string) bool {
	return outcome == application.SyncOutcomeFailed || outcome == application.SyncOutcomePartial || outcome == application.SyncOutcomeCancelled
}

func latestFailures(result application.RefreshResult) int {
	n := 0
	for _, item := range result.Items {
		if item.Status == application.RefreshFailed || item.Status == application.RefreshRateLimited {
			n++
		}
	}
	return n
}

func (p *marketDataPage) marketLatestStatus(view ui.View) []string {
	l, r := view.Language, &p.latest
	var lines []string
	switch {
	case r.stopping:
		lines = append(lines, l.Text(i18n.MarketStoppingLatest))
	case r.request != nil:
		lines = append(lines, l.Text(i18n.MarketUpdatingLatest))
	case r.result != nil && r.result.cancelled:
		lines = append(lines, l.Text(i18n.MarketLatestCancelled))
	case r.err != nil:
		lines = append(lines, l.Text(i18n.MarketFailedLatest))
	case r.result != nil:
		lines = append(lines, latestSummary(view, r.result.result))
	}
	if r.result != nil && r.result.result.RateLimited {
		lines = append(lines, l.Text(i18n.MarketWorkRateLimited))
	}
	return lines
}

func (p *marketDataPage) marketHistoricalStatus(view ui.View) []string {
	l := view.Language
	var lines []string
	if sync := p.sync; sync != nil {
		if sync.problem != i18n.NoMessage && sync.owner == marketDataRoute && p.operation == nil {
			lines = append(lines, l.Text(sync.problem))
		}
		if sync.startRead != nil && sync.job.Outcome != application.SyncOutcomeRunning {
			lines = append(lines, l.Text(i18n.HealthRepairStarting))
		} else if status := syncOutcomeLabel(sync.job.Outcome); status != i18n.NoMessage {
			lines = append(lines, l.Text(status), syncJobCounts(view, sync.job))
		}
	}
	return lines
}

func latestSummary(view ui.View, result application.RefreshResult) string {
	if len(result.Items) == 0 {
		return view.Language.Text(i18n.MarketNoTargets)
	}
	var fetched, cached, skipped int
	for _, item := range result.Items {
		switch item.Status {
		case application.RefreshFetched:
			fetched++
		case application.RefreshCached:
			cached++
		case application.RefreshSkipped:
			skipped++
		}
	}
	return fmt.Sprintf(view.Language.Text(i18n.MarketLatestSummary), fetched, cached, skipped, latestFailures(result))
}

func syncJobCounts(view ui.View, job application.SyncJobSnapshot) string {
	return fmt.Sprintf(view.Language.Text(i18n.HealthRepairProgress), job.CompletedRequests, job.EstimatedRequests,
		job.CompletedTargets, job.TargetCount, job.SnapshotDaysRebuilt, job.SnapshotDaysPlanned)
}

func (p *marketDataPage) layoutMarketOperation(gtx layout.Context, view ui.View) event.Tag {
	sheet := p.operation
	if sheet.sheet.dismissed(gtx, &sheet.force) {
		p.closeMarketOperation()
		gtx.Execute(op.InvalidateCmd{})
		return sheet.source
	}
	if sheet.kind == marketSyncPreview {
		return p.layoutMarketSyncPreview(gtx, view, sheet)
	}
	l := view.Language
	title := l.Text(i18n.MarketLatestDetails)
	lines := p.marketLatestStatus(view)
	items := p.marketLatestItems(view)
	var blockers []application.SyncBlocker
	if sheet.kind == marketSyncDetails {
		title = l.Text(i18n.MarketSyncDetails)
		lines = p.marketHistoricalStatus(view)
		job := p.sync.job
		items = append([]application.SyncItemProgress(nil), job.Items...)
		if job.Current != nil {
			items = append([]application.SyncItemProgress{*job.Current}, items...)
		}
		blockers = job.Blockers
	}
	sheet.sheet.layout(gtx, view, title, marketOperationList(view, sheet, title, lines, items, blockers, nil), i18n.NoMessage)
	return nil
}

func (p *marketDataPage) layoutMarketSyncPreview(gtx layout.Context, view ui.View, sheet *marketOperationSheet) event.Tag {
	job, l := p.sync, view.Language
	plan := job.plan
	if sheet.request.Scope == application.SyncScopeRepairAll && sheet.force.Clicked(gtx) {
		sheet.request.ForceRecheck = !sheet.request.ForceRecheck
		p.loadMarketSyncPreview()
		plan = nil
		gtx.Execute(op.InvalidateCmd{})
	}
	var lines []string
	var items []application.SyncItemProgress
	var blockers []application.SyncBlocker
	action := i18n.NoMessage
	switch {
	case job.previewRead != nil:
		lines = append(lines, l.Text(i18n.HealthRepairPlanning))
	case job.problem != i18n.NoMessage:
		lines = append(lines, l.Text(job.problem))
		if job.problem != i18n.HealthRepairHistoryRequired {
			action = i18n.HistoryRetry
		}
	case plan != nil:
		lines = append(lines, l.Text(i18n.HealthRepairDescription),
			fmt.Sprintf(l.Text(i18n.HealthRepairRequestCount), plan.EstimatedRequestCount),
			fmt.Sprintf(l.Text(i18n.HealthRepairTargetCount), plan.InstrumentTargets, plan.FXTargets),
			fmt.Sprintf(l.Text(i18n.HealthRepairLatestCount), plan.LatestInstrumentCount, plan.LatestFXCount),
			fmt.Sprintf(l.Text(i18n.HealthRepairSnapshotCount), plan.SnapshotWorkEstimate))
		items, blockers = plan.Items, plan.Unresolved
		if plan.EstimatedRequestCount > 0 || plan.SnapshotWorkEstimate > 0 || plan.LatestInstrumentCount > 0 || plan.LatestFXCount > 0 {
			action = i18n.MarketStartSync
		} else {
			lines = append(lines, l.Text(i18n.HealthRepairNoWork))
		}
	}
	if sheet.sheet.action.Clicked(gtx) {
		switch action {
		case i18n.HistoryRetry:
			p.loadMarketSyncPreview()
		case i18n.MarketStartSync:
			p.lastSync = plan.Scope
			job.start(p.ctx, &p.workers, p.invalidate, plan.Scope)
			p.operation = nil
			gtx.Execute(op.InvalidateCmd{})
			return sheet.source
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	var force layout.Widget
	if sheet.request.Scope == application.SyncScopeRepairAll {
		force = analysisToggle(view, &sheet.force, l.Text(i18n.MarketForceHistory), sheet.request.ForceRecheck)
	}
	title := l.Text(i18n.MarketSyncPreview)
	sheet.sheet.layout(gtx, view, title, marketOperationList(view, sheet, title, lines, items, blockers, force), action)
	return nil
}

func marketOperationList(view ui.View, sheet *marketOperationSheet, title string, lines []string, items []application.SyncItemProgress, blockers []application.SyncBlocker, option layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &sheet.sheet.list, title, 1+len(items)+len(blockers), func(gtx layout.Context, i int) layout.Dimensions {
			switch {
			case i == 0:
				rows := []ui.ColumnChild{ui.Rigid(func(gtx layout.Context) layout.Dimensions { return healthRepairLines(gtx, view, lines) })}
				if option != nil {
					rows = append(rows, ui.Rigid(option))
				}
				return (ui.Column{Gap: 8}).Layout(gtx, rows...)
			case i <= len(items):
				return healthRepairLines(gtx, view, marketWorkLines(view, items[i-1]))
			default:
				blocker := blockers[i-1-len(items)]
				reason := healthReason(blocker.Reason)
				if reason == i18n.NoMessage {
					reason = i18n.HealthRepairBlocked
				}
				return view.Message(view.Language.Text(reason), view.Colors.WarningInk)(gtx)
			}
		})
	}
}

func (p *marketDataPage) marketLatestItems(view ui.View) []application.SyncItemProgress {
	if p.latest.progress == nil {
		return nil
	}
	items := p.latest.progress.itemsSnapshot()
	if p.latest.result == nil {
		return items
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		seen[item.TargetKey] = true
	}
	for _, result := range p.latest.result.result.Items {
		if seen[result.TargetKey] {
			continue
		}
		name := view.Language.Text(i18n.HealthFinding)
		if pair, ok := strings.CutPrefix(result.TargetKey, string(application.RefreshFXTarget)+":"); ok {
			name = pair
		}
		if id, ok := strings.CutPrefix(result.TargetKey, string(application.RefreshInstrumentTarget)+":"); ok && p.current[marketInstruments] != nil {
			for _, row := range p.current[marketInstruments].instruments {
				if row.instrument.ID.String() == id {
					name = row.instrument.DisplayLabel()
					break
				}
			}
		}
		items = append(items, application.SyncItemProgress{Label: name, TargetKey: result.TargetKey, Status: string(result.Status), Detail: string(result.ErrorCode)})
	}
	return items
}
