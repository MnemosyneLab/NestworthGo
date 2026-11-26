package desktop

import (
	"fmt"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

func healthDateRange(view ui.View, from, to string) string {
	if from == to || to == "" {
		return wealthDay(view, from)
	}
	return wealthDay(view, from) + " – " + wealthDay(view, to)
}

func (p *dataHealthPage) snapshotActions(gtx layout.Context, view ui.View, row healthIssueRow, label string) []layout.Widget {
	issue := row.issue
	state := &p.snapshots
	var actions []layout.Widget
	if selection, ok := snapshotInspection(row, label, p.focus); p.inspect != nil && ok {
		if state.inspect == nil {
			state.inspect = make(map[string]*widget.Clickable)
		}
		button := portfolioButton(state.inspect, issue.ID)
		if button.Clicked(gtx) {
			p.inspect(selection, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		actions = append(actions, returnMonthButton(view, button, i18n.HealthInspectSnapshot, nil, true))
	}
	from, to, valid := snapshotWindow(issue, p.focus)
	if !valid || !isSnapshotFinding(issue) || !issue.Executable || issue.Collapsed || issue.Action != application.HealthActionRepair || state.service == nil {
		return actions
	}
	if state.preview == nil {
		state.preview = make(map[string]*widget.Clickable)
	}
	button := portfolioButton(state.preview, issue.ID)
	enabled := p.request == nil && p.err == nil && state.active == nil && (p.repair == nil || !p.repair.inProgress())
	if enabled && gtx.Enabled() && state.restore == button {
		state.restore = nil
		gtx.Execute(key.FocusCmd{Tag: button})
	}
	if button.Clicked(gtx) && enabled {
		state.active = &snapshotRebuild{sheet: newAnalysisSheet(), trigger: button, from: from, to: to, next: from}
		gtx.Execute(op.InvalidateCmd{})
	}
	return append(actions, returnMonthButton(view, button, i18n.HealthPreviewSnapshot, nil, enabled))
}

func (p *dataHealthPage) layoutSnapshotRebuild(gtx layout.Context, view ui.View) {
	r := p.snapshots.active
	if r.sheet.dismissed(gtx) && r.request == nil {
		p.snapshots.active, p.snapshots.restore = nil, r.trigger
		gtx.Execute(op.InvalidateCmd{})
		return
	}
	if r.sheet.action.Clicked(gtx) {
		switch {
		case r.request != nil:
			r.stopping = true
		case r.next <= r.to:
			p.rebuildSnapshotBatch()
		}
		gtx.Execute(op.InvalidateCmd{})
	}
	wasRunning := r.request != nil
	p.updateSnapshotRebuild()
	if wasRunning && r.request == nil {
		gtx.Execute(key.FocusCmd{Tag: &r.sheet.close})
	}
	l := view.Language
	action := i18n.HealthConfirmSnapshot
	status := i18n.NoMessage
	detail := ""
	switch {
	case r.request != nil:
		action, status = i18n.HealthStopSnapshot, i18n.HealthSnapshotWorking
		detail = healthDateRange(view, r.next, r.batchLast)
		if r.stopping {
			action, status = i18n.NoMessage, i18n.HealthSnapshotStopping
		}
	case r.err != nil:
		action, status = i18n.HistoryRetry, i18n.HealthSnapshotFailed
		detail = healthDateRange(view, r.next, r.batchLast)
	case r.paused:
		action, status = i18n.HealthResumeSnapshot, i18n.HealthSnapshotPaused
		detail = healthDateRange(view, r.next, r.to)
	case r.next > r.to:
		action, status = i18n.NoMessage, i18n.HealthSnapshotDone
	}
	title := l.Text(i18n.HealthSnapshotRepairTitle)
	r.sheet.layoutActions(gtx, view, title, func(gtx layout.Context) layout.Dimensions {
		return analysisResultList(gtx, view, &r.sheet.list, title, 1, func(gtx layout.Context, _ int) layout.Dimensions {
			rows := []ui.ColumnChild{
				ui.Rigid(view.Body(ui.TextLabel, healthDateRange(view, r.from, r.to), view.Colors.Ink)),
				ui.Rigid(view.Message(l.Text(i18n.HealthSnapshotRepairNote), view.Colors.SecondaryInk)),
			}
			if status != i18n.NoMessage {
				message := l.Text(status)
				if detail != "" {
					message = fmt.Sprintf(message, detail)
				}
				ink := view.Colors.Ink
				if r.err != nil {
					ink = view.Colors.ErrorInk
				}
				rows = append(rows,
					ui.Rigid(view.Message(message, ink)),
					ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HealthSnapshotRevisions), r.revisions), view.Colors.SecondaryInk)))
			}
			return (ui.Column{Gap: 8}).Layout(gtx, rows...)
		})
	}, action, r.request == nil)
}
