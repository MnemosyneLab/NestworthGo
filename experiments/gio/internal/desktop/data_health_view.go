package desktop

import (
	"fmt"
	"strconv"
	"strings"

	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui/lucide"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

var healthSections = [...]i18n.MessageID{i18n.HealthExecutable, i18n.HealthPrerequisites, i18n.HealthReported, i18n.HealthSnapshotDependencies}

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
	if p.manualSaved.saved() {
		rows = append(rows, ui.Rigid(manualQuoteNotice(view, p.manualSaved)))
	}
	if p.focus.active() {
		var fields []string
		if p.focus.label != "" {
			fields = append(fields, p.focus.label)
		}
		if p.focus.from != "" {
			fields = append(fields, healthDateRange(view, p.focus.from, p.focus.to))
		}
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HealthFocused), strings.Join(fields, " · ")), view.Colors.Ink)))
		if p.clearFocus != nil {
			if p.clear.Clicked(gtx) {
				p.clearFocus(&p.clear)
				gtx.Execute(op.InvalidateCmd{})
			}
			rows = append(rows, ui.Rigid(returnMonthButton(view, &p.clear, i18n.HealthShowAll, nil, true)))
		}
	}
	recheck := i18n.HealthRecheck
	switch {
	case p.err != nil:
		recheck = i18n.HistoryRetry
	}
	recheckButton := returnMonthButton(view, &p.recheck, recheck, nil, p.request == nil)
	inSummary := p.report != nil && !p.focus.active()
	if report := p.report; inSummary {
		rows = append(rows, ui.Rigid(healthSummary(view, *report, recheckButton)))
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
	switch {
	case p.request != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthLoading), view.Colors.SecondaryInk)))
	case p.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthFailed), view.Colors.ErrorInk)))
	}
	if !inSummary {
		rows = append(rows, ui.Rigid(recheckButton))
	}
	if p.report != nil {
		issues := p.findings
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
	dims := (ui.Column{Gap: 12, MinFill: 250}).Layout(gtx, rows...)
	if p.snapshots.restore != nil && p.request == nil && gtx.Enabled() {
		p.snapshots.restore = nil
		gtx.Execute(key.FocusCmd{Tag: &p.recheck})
	}
	return dims
}

func healthSummary(view ui.View, report application.MarketDataHealthReport, recheck layout.Widget) layout.Widget {
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
		colors := view.Colors
		icon, fill, ink := lucide.TriangleAlert, ui.Mix(colors.Surface, colors.Warning, 0.18), colors.WarningInk
		if report.Healthy {
			icon, fill, ink = lucide.CircleCheck, ui.Mix(colors.Surface, colors.Success, 0.16), colors.Success
		}
		return view.Card(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				tile := func(gtx layout.Context) layout.Dimensions { return view.IconTile(gtx, icon, 36, fill, ink) }
				text := func(gtx layout.Context) layout.Dimensions {
					items := []ui.ColumnChild{ui.Rigid(view.Body(ui.TextHeading, title, colors.Ink))}
					if len(details) != 0 {
						items = append(items, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
							return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(details), func(gtx layout.Context, i int) layout.Dimensions {
								return view.Badge(gtx, ui.BadgeNeutral, details[i])
							})
						}))
					}
					return (ui.Column{Gap: ui.Space8}).Layout(gtx, items...)
				}
				if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 640) {
					return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space12)}.Layout(gtx,
						layout.Rigid(tile), layout.Flexed(1, text), layout.Rigid(recheck))
				}
				return (ui.Column{Gap: ui.Space12}).Layout(gtx,
					ui.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Start, Gap: gtx.Dp(ui.Space12)}.Layout(gtx, layout.Rigid(tile), layout.Flexed(1, text))
					}), ui.Rigid(recheck))
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
		heading := view.Body(ui.TextHeading, l.Text(healthSections[row.section]), view.Colors.Ink)
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space4, Bottom: ui.Space4}.Layout(gtx, heading)
		}))
	}
	if row.startGroup {
		title := view.Body(ui.TextLabel, l.Text(kind), view.Colors.Ink)
		rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: ui.Space4, Bottom: ui.Space4}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space8)}.Layout(gtx,
					layout.Rigid(title),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return view.Badge(gtx, ui.BadgeNeutral, strconv.Itoa(row.groupCount))
					}))
			})
		}))
	}
	name := healthIssueName(issue)
	if name == "" {
		name = l.Text(kind)
	}
	if isSnapshotFinding(issue) && issue.AccountID != "" {
		account := p.accountNames[issue.AccountID]
		if account == "" {
			account = l.Text(i18n.OverviewAllocationUnknownAccount)
		}
		name = account + " · " + name
	}
	var actions []layout.Widget
	if action := p.issueAction(gtx, view, issue, name); action != nil {
		actions = append(actions, action)
	}
	if p.quoteHistory != nil {
		if target, ok := quoteHistoryFinding(issue, name); ok {
			if p.historyActions == nil {
				p.historyActions = make(map[string]*widget.Clickable)
			}
			actions = append(actions, p.quoteHistory(gtx, view, target, portfolioButton(p.historyActions, issue.ID), p.request == nil && p.err == nil))
		}
	}
	actions = append(actions, p.snapshotActions(gtx, view, row, name)...)
	rows = append(rows, ui.Rigid(func(gtx layout.Context) layout.Dimensions {
		colors := view.Colors
		title := func(gtx layout.Context) layout.Dimensions {
			tone := ui.BadgeNeutral
			switch issue.Severity {
			case application.HealthSeverityBlocking:
				tone = ui.BadgeDestructive
			case application.HealthSeverityWarning:
				tone = ui.BadgeWarning
			}
			severity := healthSeverities[issue.Severity]
			return (ui.Wrap{Gap: ui.Space8, Middle: true}).Layout(gtx, 1+boolInt(severity != i18n.NoMessage), func(gtx layout.Context, i int) layout.Dimensions {
				if i == 0 {
					return view.Body(ui.TextLabel, name, colors.Ink)(gtx)
				}
				return view.Badge(gtx, tone, l.Text(severity))
			})
		}
		details := []ui.ColumnChild{ui.Rigid(title)}
		if evidence := healthIssueEvidenceText(view, issue, false); evidence != "" {
			details = append(details, ui.Rigid(view.Message(evidence, colors.SecondaryInk)))
		}
		if row.combined {
			details = append(details, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.HealthSnapshotAffected), issue.RangeCount), colors.SecondaryInk)))
		}
		if isSnapshotFinding(issue) && issue.Collapsed {
			details = append(details, ui.Rigid(view.Message(l.Text(i18n.HealthSnapshotBlocked), colors.WarningInk)))
		}
		if message := healthIssueAction(issue); message != i18n.NoMessage {
			details = append(details, ui.Rigid(view.Message(l.Text(message), colors.SecondaryInk)))
		}
		info := func(gtx layout.Context) layout.Dimensions { return (ui.Column{Gap: ui.Space4}).Layout(gtx, details...) }
		if len(actions) == 0 {
			return info(gtx)
		}
		buttons := func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space6}).Layout(gtx, len(actions), func(gtx layout.Context, i int) layout.Dimensions { return actions[i](gtx) })
		}
		if ui.WidthAtLeast(gtx, gtx.Constraints.Max.X, 720) {
			return layout.Flex{Alignment: layout.Middle, Gap: gtx.Dp(ui.Space16)}.Layout(gtx,
				layout.Flexed(1, info),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(340))
					return layout.E.Layout(gtx, buttons)
				}))
		}
		return (ui.Column{Gap: ui.Space8}).Layout(gtx, ui.Rigid(info), ui.Rigid(buttons))
	}))
	return (ui.Column{Gap: 7}).Layout(gtx, rows...)
}

func healthIssueEvidence(view ui.View, issue application.HealthIssue) string {
	return healthIssueEvidenceText(view, issue, true)
}

// healthIssueEvidenceText joins the facts of a finding; a row that shows its
// severity as a badge leaves the severity word out.
func healthIssueEvidenceText(view ui.View, issue application.HealthIssue, withSeverity bool) string {
	l := view.Language
	var evidence []string
	if severity := healthSeverities[issue.Severity]; withSeverity && severity != i18n.NoMessage {
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
	case p.openAgentEvidence != nil && (issue.Kind == application.HealthKindMissingAgentPrice ||
		issue.Kind == application.HealthKindMissingAgentFX || issue.Reason == "agent_data_required"):
		target, valid := quoteHistoryFinding(issue, name)
		if !valid {
			return nil
		}
		button := p.issueButton(issue.ID)
		ready := p.request == nil && p.err == nil
		if ready && button.Clicked(gtx) {
			p.openAgentEvidence(target, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.AgentEvidenceOpen, nil, ready)
	case issue.Action == application.HealthActionInstrumentEditor && p.openInstrumentSource != nil:
		id, err := domain.ParseInstrumentID(issue.InstrumentID)
		if err != nil {
			return nil
		}
		button := p.issueButton(issue.ID)
		ready := p.request == nil && p.err == nil && (p.repair == nil || !p.repair.inProgress())
		if ready && button.Clicked(gtx) {
			p.openInstrumentSource(id, name, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.InstrumentBindingOpen, nil, ready)
	case issue.Executable && issue.Action == application.HealthActionRepair && p.previewQuoteRepair != nil:
		scope, valid := healthQuoteRepairScope(issue)
		if !valid {
			return nil
		}
		button := p.issueButton(issue.ID)
		ready := p.request == nil && p.err == nil && !p.repair.inProgress()
		if button.Clicked(gtx) && ready {
			p.previewQuoteRepair(scope, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.MarketPreviewTarget, nil, ready)
	case issue.Action == application.HealthActionManualEntry && p.openManual != nil:
		target, valid := healthManualTarget(issue, p.focus)
		if !valid {
			return nil
		}
		button := p.issueButton(issue.ID)
		ready := p.request == nil && p.err == nil && (p.repair == nil || !p.repair.inProgress())
		if button.Clicked(gtx) && ready {
			p.openManual(target, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.MarketManualAdd, nil, ready)
	case issue.Action == application.HealthActionProviderSettings && p.openSettings != nil:
		button := p.issueButton(issue.ID)
		ready := p.request == nil && p.err == nil
		if ready && button.Clicked(gtx) {
			p.openSettings(issue.Provider, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		return returnMonthButton(view, button, i18n.HealthOpenSettings, nil, ready)
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

func quoteHistoryFinding(issue application.HealthIssue, label string) (marketQuoteHistoryTarget, bool) {
	found, valid := healthManualTarget(issue, healthFindingFocus{})
	focus := healthFindingFocus{from: issue.RangeStart, to: issue.RangeEnd, id: issue.ID, label: label}
	return marketQuoteHistoryTarget{instrument: found.instrumentID, name: label,
		pair: marketFXPair{found.base, found.quote}, finding: focus}, valid
}

func healthQuoteRepairScope(issue application.HealthIssue) (application.SyncRequest, bool) {
	if isSnapshotFinding(issue) {
		return application.SyncRequest{}, false
	}
	target, valid := healthManualTarget(issue, healthFindingFocus{})
	if !valid {
		return application.SyncRequest{}, false
	}
	if target.instrumentID != "" {
		return application.SyncRequest{Scope: application.SyncScopeInstrument, InstrumentID: target.instrumentID.String()}, true
	}
	return application.SyncRequest{Scope: application.SyncScopeFX, CurrencyA: target.base.String(), CurrencyB: target.quote.String()}, true
}
