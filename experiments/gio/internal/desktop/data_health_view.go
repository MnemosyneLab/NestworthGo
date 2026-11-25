package desktop

import (
	"fmt"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var healthSections = [...]i18n.MessageID{i18n.HealthExecutable, i18n.HealthPrerequisites, i18n.HealthReported}

var healthKinds = map[string]i18n.MessageID{
	application.HealthKindMissingInstrumentHistory: i18n.HealthInstrumentHistory,
	application.HealthKindMissingFXHistory:         i18n.HealthFXHistory,
	application.HealthKindMissingManualPrice:       i18n.HealthManualPrice,
	application.HealthKindMissingManualFX:          i18n.HealthManualFX,
	application.HealthKindMissingAgentPrice:        i18n.HealthAgentPrice,
	application.HealthKindMissingAgentFX:           i18n.HealthAgentFX,
	application.HealthKindMissingProviderKey:       i18n.HealthProviderKey,
	application.HealthKindMissingBinding:           i18n.HealthBinding,
	application.HealthKindInitialAnchorMissing:     i18n.HealthAnchor,
	application.HealthKindUnsupportedCoverage:      i18n.HealthUnsupported,
	application.HealthKindStaleAccountValue:        i18n.HealthStaleValue,
	application.HealthKindMissingAccountValue:      i18n.HealthMissingValue,
	application.HealthKindIncompleteValuation:      i18n.HealthIncompleteValuation,
	application.HealthKindHistoryPending:           i18n.HealthPending,
	application.HealthKindValuationProvisional:     i18n.HealthProvisional,
	application.HealthKindSnapshotIncomplete:       i18n.HealthSnapshotIncomplete,
	application.HealthKindSnapshotMissing:          i18n.HealthSnapshotMissing,
	application.HealthKindSnapshotOutdated:         i18n.HealthSnapshotOutdated,
	application.HealthKindSyncFailure:              i18n.HealthSyncFailure,
	application.HealthKindHistoryNotStarted:        i18n.HealthHistoryNotStarted,
}

var healthSeverities = map[string]i18n.MessageID{
	application.HealthSeverityBlocking: i18n.HealthBlocking,
	application.HealthSeverityWarning:  i18n.HealthWarning,
	application.HealthSeverityInfo:     i18n.HealthInfo,
}

func (p *dataHealthPage) layout(gtx layout.Context, view ui.View) layout.Dimensions {
	p.update()
	p.updateRepair()
	l := view.Language
	if p.request == nil && p.recheck.Clicked(gtx) {
		p.read()
		gtx.Execute(op.InvalidateCmd{})
	}
	rows := []ui.ColumnChild{ui.Rigid(view.Message(l.Text(i18n.HealthDescription), view.Colors.SecondaryInk))}
	if p.focus.active() {
		scope := p.focus.label
		if p.focus.from != "" {
			scope = wealthDay(view, p.focus.from)
			if p.focus.to != "" && p.focus.from != p.focus.to {
				scope += " – " + wealthDay(view, p.focus.to)
			}
		}
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HealthFocused), scope), view.Colors.Ink)))
		if p.clearFocus != nil {
			if p.clear.Clicked(gtx) {
				p.clearFocus(&p.clear)
				gtx.Execute(op.InvalidateCmd{})
			}
			rows = append(rows, ui.Rigid(returnMonthButton(view, &p.clear, i18n.HealthShowAll, nil, true)))
		}
	}
	if report := p.report; report != nil && !p.focus.active() {
		rows = append(rows, ui.Rigid(healthSummary(view, *report)))
		if p.repair != nil && !report.Healthy {
			rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				return p.repairAction(gtx, view)
			}))
		}
	}
	if repair := p.repair; repair != nil && (repair.job.JobID != "" || repair.problem != i18n.NoMessage || repair.startRead != nil) {
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return p.repairProgress(gtx, view)
		}))
	}
	recheck := i18n.HealthRecheck
	switch {
	case p.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthLoading), view.Colors.SecondaryInk)))
	case p.err != nil:
		recheck = i18n.HistoryRetry
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthFailed), view.Colors.ErrorInk)))
	}
	rows = append(rows, ui.Rigid(returnMonthButton(view, &p.recheck, recheck, nil, p.request == nil)))
	if report := p.report; report != nil {
		issues := healthIssueRows(focusedHealthFindings(report.Issues, p.focus))
		if len(issues) != 0 {
			rows = append(rows, ui.Fill(func(gtx layout.Context) layout.Dimensions {
				return analysisResultList(gtx, view, &p.list, l.Text(i18n.HealthFindings), len(issues), func(gtx layout.Context, index int) layout.Dimensions {
					return p.issueRow(gtx, view, issues[index])
				})
			}))
		} else if p.focus.active() && p.request == nil && p.err == nil {
			rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthFocusedEmpty), view.Colors.SecondaryInk)))
		}
	}
	return (ui.Column{Gap: 12, MinFill: 250}).Layout(gtx, rows...)
}

func healthSummary(view ui.View, report application.MarketDataHealthReport) layout.Widget {
	l := view.Language
	title := l.Text(i18n.HealthHealthy)
	var details []string
	if report.Healthy {
		if day := report.CoverageThrough; day != "" {
			details = append(details, fmt.Sprintf(l.Text(i18n.HealthCompleteThrough), wealthDay(view, day)))
		}
	} else {
		title = l.Text(i18n.HealthIncomplete)
		if day := report.IncompleteSince; day != "" {
			title = fmt.Sprintf(l.Text(i18n.HealthIncompleteSince), wealthDay(view, day))
		}
		details = append(details, fmt.Sprintf(l.Text(i18n.HealthGapCount), report.IssueCount))
		if report.SnapshotDays != 0 {
			details = append(details, fmt.Sprintf(l.Text(i18n.HealthSnapshotCount), report.SnapshotDays))
		}
		if report.PrerequisiteCount != 0 {
			details = append(details, fmt.Sprintf(l.Text(i18n.HealthPrerequisiteCount), report.PrerequisiteCount))
		}
	}
	for _, issue := range report.Issues {
		if issue.Collapsed {
			details = append(details, l.Text(i18n.HealthGrouped))
			break
		}
	}
	return func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(12).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				items := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, title, view.Colors.Ink))}
				for _, detail := range details {
					items = append(items, ui.Rigid(view.Message(detail, view.Colors.SecondaryInk)))
				}
				return (ui.Column{Gap: 5}).Layout(gtx, items...)
			})
		})
	}
}

func (p *dataHealthPage) issueRow(gtx layout.Context, view ui.View, row healthIssueRow) layout.Dimensions {
	l, issue := view.Language, row.issue
	var rows []ui.ColumnChild
	kind := healthKinds[issue.Kind]
	if kind == i18n.NoMessage {
		kind = i18n.HealthFinding
	}
	if row.startSection {
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, l.Text(healthSections[row.section]), view.Colors.Ink)))
	}
	if row.startGroup {
		heading := fmt.Sprintf("%s · %d", l.Text(kind), row.groupCount)
		rows = append(rows, ui.Rigid(view.Body(ui.TextLabel, heading, view.Colors.Ink)))
	}
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				name := healthIssueName(issue)
				if name == "" {
					name = l.Text(kind)
				}
				items := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextLabel, name, view.Colors.Ink))}
				if evidence := healthIssueEvidence(view, issue); evidence != "" {
					items = append(items, ui.Rigid(view.Message(evidence, view.Colors.SecondaryInk)))
				}
				if message := healthIssueAction(issue); message != i18n.NoMessage {
					items = append(items, ui.Rigid(view.Message(l.Text(message), view.Colors.SecondaryInk)))
				}
				if action := p.issueAction(gtx, view, issue, name); action != nil {
					items = append(items, ui.Rigid(action))
				}
				return (ui.Column{Gap: 6}).Layout(gtx, items...)
			})
		})
	}))
	return (ui.Column{Gap: 7}).Layout(gtx, rows...)
}

func healthIssueEvidence(view ui.View, issue application.HealthIssue) string {
	l := view.Language
	var evidence []string
	if severity := healthSeverities[issue.Severity]; severity != i18n.NoMessage {
		evidence = append(evidence, l.Text(severity))
	}
	if issue.RangeStart != "" {
		date := wealthDay(view, issue.RangeStart)
		if end := issue.RangeEnd; end != "" && end != issue.RangeStart {
			date += " – " + wealthDay(view, end)
		}
		evidence = append(evidence, date)
	}
	if issue.Provider != "" && issue.Provider != issue.Label {
		evidence = append(evidence, issue.Provider)
	}
	reason := healthReason(issue.Reason)
	if reason != i18n.NoMessage && reason != healthKinds[issue.Kind] {
		evidence = append(evidence, l.Text(reason))
	}
	return strings.Join(evidence, " · ")
}

func healthReason(reason string) i18n.MessageID {
	switch reason {
	case "opening_anchor_missing", application.HealthKindInitialAnchorMissing:
		return i18n.HealthAnchor
	case "missing_current_input":
		return i18n.HealthCurrentMissing
	case "agent_data_required":
		return i18n.HealthAgentNeeded
	case "provider_not_configured", application.ProviderConfigMissingKey:
		return i18n.HealthProviderKey
	case "valuation_coverage_pending", "daily_reference_pending":
		return i18n.HealthPending
	case "snapshot_inputs_ready":
		return i18n.HealthInputsReady
	case "holding_amount_out_of_range":
		return i18n.HealthHoldingOutOfRange
	case "manual_value_older_than_180_days":
		return i18n.HealthStaleValue
	case "instrument_type_unsupported", "history_capability_unavailable", "market_not_supported_for_session_policy", "coingecko_history_limit_365_days":
		return i18n.HealthUnsupported
	}
	return i18n.NoMessage
}

func healthIssueAction(issue application.HealthIssue) i18n.MessageID {
	switch issue.Action {
	case application.HealthActionRepair:
		if issue.Executable {
			return i18n.HealthRepairAvailable
		}
	case application.HealthActionInstrumentEditor:
		return i18n.HealthCheckInstrument
	case application.HealthActionManualEntry:
		return i18n.HealthEnterManual
	case application.HealthActionNone:
		if issue.Kind == application.HealthKindMissingAgentFX || issue.Kind == application.HealthKindMissingAgentPrice {
			return i18n.HealthAgentNeeded
		}
	}
	return i18n.NoMessage
}

func (p *dataHealthPage) issueAction(gtx layout.Context, view ui.View, issue application.HealthIssue, name string) layout.Widget {
	l := view.Language
	switch {
	case issue.Action == application.HealthActionProviderSettings && p.openSettings != nil:
		button := p.issueButton(issue.ID)
		if button.Clicked(gtx) {
			p.openSettings(button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.HealthOpenSettings, nil, true)
	case issue.Action == "account_value" && p.openAccount != nil:
		id, err := domain.ParseAccountID(issue.AccountID)
		if err != nil {
			return nil
		}
		button := p.issueButton(issue.ID)
		if button.Clicked(gtx) {
			p.openAccount(id, name, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return func(gtx layout.Context) layout.Dimensions {
			view.Focus.Watch(gtx, button)
			return view.Button(button, fmt.Sprintf(l.Text(i18n.HistoryOpenAccount), name))(gtx)
		}
	}
	return nil
}

func (p *dataHealthPage) issueButton(id string) *widget.Clickable {
	if p.issueActions == nil {
		p.issueActions = make(map[string]*widget.Clickable)
	}
	return portfolioButton(p.issueActions, id)
}
