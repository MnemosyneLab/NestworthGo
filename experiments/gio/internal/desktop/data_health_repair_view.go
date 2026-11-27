package desktop

import (
	"fmt"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

func (p *dataHealthPage) layoutRepair(gtx layout.Context, view ui.View) {
	repair := p.repair
	if repair.sheet.dismissed(gtx) {
		repair.retirePreview()
		repair.returnFocus = true
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if repair.sheet.action.Clicked(gtx) {
		p.confirmRepair()
		gtx.Execute(key.FocusCmd{Tag: &p.recheck})
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	plan := *repair.plan
	action := i18n.HealthRepairConfirm
	if plan.EstimatedRequestCount == 0 && plan.SnapshotWorkEstimate == 0 && plan.LatestInstrumentCount == 0 && plan.LatestFXCount == 0 {
		action = i18n.NoMessage
	}
	l := view.Language
	repair.sheet.layout(gtx, view, l.Text(i18n.HealthRepairTitle), func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &repair.sheet.list, l.Text(i18n.HealthRepairTitle), 1+len(plan.Items)+len(plan.Unresolved), func(gtx layout.Context, index int) layout.Dimensions {
			if index == 0 {
				labels := []string{
					l.Text(i18n.HealthRepairDescription),
					fmt.Sprintf(l.Text(i18n.HealthRepairRequestCount), plan.EstimatedRequestCount),
					fmt.Sprintf(l.Text(i18n.HealthRepairTargetCount), plan.InstrumentTargets, plan.FXTargets),
					fmt.Sprintf(l.Text(i18n.HealthRepairLatestCount), plan.LatestInstrumentCount, plan.LatestFXCount),
					fmt.Sprintf(l.Text(i18n.HealthRepairSnapshotCount), plan.SnapshotWorkEstimate),
				}
				if len(plan.Unresolved) > 0 {
					labels = append(labels, fmt.Sprintf(l.Text(i18n.HealthRepairBlockedCount), len(plan.Unresolved)))
				}
				if action == i18n.NoMessage {
					labels = append(labels, l.Text(i18n.HealthRepairNoWork))
				}
				return healthRepairLines(gtx, view, labels)
			}
			if index <= len(plan.Items) {
				return healthRepairLines(gtx, view, syncItemLines(view, plan.Items[index-1]))
			}
			blocker := plan.Unresolved[index-1-len(plan.Items)]
			message := healthReason(blocker.Reason)
			if message == i18n.NoMessage {
				message = i18n.HealthRepairBlocked
			}
			return view.Message(l.Text(message), view.Colors.SecondaryInk)(gtx)
		})
	}, action)
}

func (p *dataHealthPage) repairProgress(gtx layout.Context, view ui.View) layout.Dimensions {
	r := p.repair
	l := view.Language
	job := r.job
	var rows []ui.ColumnChild
	if r.problem != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Message(l.Text(r.problem), view.Colors.ErrorInk)))
	}
	if r.problem == i18n.HealthRepairStale && r.owner == dataHealthRoute {
		ready := !r.inProgress()
		if ready && p.retryRepair.Clicked(gtx) {
			if r.lastScope.Scope == application.SyncScopeRepairAll {
				p.previewRepair()
			} else if p.previewQuoteRepair != nil {
				p.previewQuoteRepair(r.lastScope, &p.retryRepair)
			}
			gtx.Execute(op.InvalidateCmd{})
		}
		rows = append(rows, ui.Rigid(returnMonthButton(view, &p.retryRepair, i18n.MarketRetrySync, nil, ready)))
		return (ui.Column{Gap: 6}).Layout(gtx, rows...)
	}
	if r.startRead != nil && job.Outcome != application.SyncOutcomeRunning {
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthRepairStarting), view.Colors.SecondaryInk)))
	}
	if job.JobID == "" {
		return (ui.Column{Gap: 6}).Layout(gtx, rows...)
	}
	if status := syncOutcomeLabel(job.Outcome); status != i18n.NoMessage {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(status), view.Colors.Ink)))
	}
	progress := syncJobCounts(view, job)
	rows = append(rows, ui.Rigid(view.Message(progress, view.Colors.SecondaryInk)))
	var actions []layout.Widget
	if p.openRepairDetails != nil {
		if p.repairDetails.Clicked(gtx) {
			p.openRepairDetails(&p.repairDetails)
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, returnMonthButton(view, &p.repairDetails, i18n.MarketSyncDetails, nil, true))
	}
	if p.previewQuoteRepair != nil && marketSyncRetryable(job.Outcome) &&
		(job.Scope.Scope == application.SyncScopeInstrument || job.Scope.Scope == application.SyncScopeFX) {
		ready := !r.inProgress()
		if ready && p.retryRepair.Clicked(gtx) {
			p.previewQuoteRepair(job.Scope, &p.retryRepair)
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, returnMonthButton(view, &p.retryRepair, i18n.MarketRetrySync, nil, ready))
	}
	if job.Outcome == application.SyncOutcomeRunning {
		if r.cancelRead == nil && r.cancel.Clicked(gtx) {
			p.cancelRepair()
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, returnMonthButton(view, &r.cancel, i18n.HealthRepairCancel, nil, r.cancelRead == nil))
	}
	if len(actions) > 0 {
		rows = append(rows, ui.Rigid(marketActionRow(view, actions)))
	}
	if job.Outcome == application.SyncOutcomeRunning {
		if job.Current != nil {
			lines := syncItemLines(view, *job.Current)
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions { return healthRepairLines(gtx, view, lines) }))
		}
	}
	return (ui.Column{Gap: 6}).Layout(gtx, rows...)
}

func syncOutcomeLabel(outcome string) i18n.MessageID {
	switch outcome {
	case application.SyncOutcomeRunning:
		return i18n.HealthRepairRunning
	case application.SyncOutcomeSucceeded:
		return i18n.HealthRepairSucceeded
	case application.SyncOutcomePartial:
		return i18n.HealthRepairPartial
	case application.SyncOutcomeFailed:
		return i18n.HealthRepairFailed
	case application.SyncOutcomeCancelled:
		return i18n.HealthRepairCancelled
	default:
		return i18n.NoMessage
	}
}

func syncItemLines(view ui.View, item application.SyncItemProgress) []string {
	label := item.Label
	if label == "" {
		label = view.Language.Text(i18n.HealthFinding)
	}
	lines := []string{label}
	var evidence []string
	if item.Provider != "" {
		evidence = append(evidence, item.Provider)
	}
	if start := item.StartDate; start != "" {
		date := wealthDay(view, start)
		if item.EndDate != "" && item.EndDate != start {
			date += " – " + wealthDay(view, item.EndDate)
		}
		evidence = append(evidence, date)
	}
	if len(evidence) > 0 {
		lines = append(lines, strings.Join(evidence, " · "))
	}
	return lines
}

func healthRepairLines(gtx layout.Context, view ui.View, lines []string) layout.Dimensions {
	rows := make([]ui.ColumnChild, 0, len(lines))
	for _, line := range lines {
		rows = append(rows, ui.Rigid(view.Message(line, view.Colors.Ink)))
	}
	return (ui.Column{Gap: 6}).Layout(gtx, rows...)
}
