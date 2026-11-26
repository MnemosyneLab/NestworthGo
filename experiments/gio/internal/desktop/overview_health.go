package desktop

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/i18n"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

type overviewHealthScan struct {
	reader      dataHealthReader
	request     <-chan historyResult[application.MarketDataHealthReport]
	cancel      context.CancelFunc
	report      *application.MarketDataHealthReport
	err         error
	dirty       bool
	provisional bool
	findings    []application.HealthIssue
	links       map[string]*widget.Clickable
	retry       widget.Clickable
}

func (s *overviewHealthScan) retire() {
	if s.cancel != nil {
		s.cancel()
	}
	s.cancel, s.request = nil, nil
}

func (s *overviewHealthScan) invalidate() {
	s.retire()
	s.dirty = true
}

func (p *overviewPage) updateOverviewHealth() {
	scan := &p.healthScan
	if scan.reader == nil || p.openHealth == nil || p.current == nil || p.current.value.AccountCount == 0 {
		return
	}
	select {
	case result := <-scan.request:
		scan.retire()
		scan.err = result.err
		if result.err == nil {
			scan.report = &result.value
			scan.findings = overviewHealthFindings(result.value.Issues)
			scan.provisional = slices.ContainsFunc(result.value.Issues, func(issue application.HealthIssue) bool {
				return issue.Kind == application.HealthKindValuationProvisional
			})
			links := make(map[string]*widget.Clickable, len(scan.findings))
			for _, issue := range scan.findings {
				if link := scan.links[issue.ID]; link != nil {
					links[issue.ID] = link
				}
			}
			scan.links = links
		}
	default:
	}
	if scan.request != nil || p.ctx.Err() != nil || !scan.dirty && (scan.report != nil || scan.err != nil) {
		return
	}
	ctx, cancel := context.WithCancel(p.ctx)
	scan.cancel, scan.dirty, scan.err = cancel, false, nil
	scan.request = runHistoryRead(&p.workers, p.invalidate, func() (application.MarketDataHealthReport, error) {
		return scan.reader.ScanMarketDataHealth(ctx)
	})
}

func overviewHealthFindings(issues []application.HealthIssue) []application.HealthIssue {
	var rows []application.HealthIssue
	// Keep the scan's order within each tier, with user actions ahead of
	// informational findings or findings covered by the repair flow.
	for _, actionable := range []bool{true, false} {
		for _, issue := range issues {
			if issue.Collapsed || issue.Kind == application.HealthKindHistoryNotStarted || strings.HasPrefix(issue.Kind, "snapshot") {
				continue
			}
			hasAction := issue.Action != application.HealthActionNone && issue.Action != application.HealthActionRepair
			if hasAction == actionable {
				rows = append(rows, issue)
				if len(rows) == overviewAttentionLimit {
					return rows
				}
			}
		}
	}
	return rows
}

func (p *overviewPage) overviewHealthRows(view ui.View) []ui.ColumnChild {
	scan, l := &p.healthScan, view.Language
	if scan.reader == nil || p.openHealth == nil {
		return nil
	}
	var rows []ui.ColumnChild
	switch {
	case scan.err != nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthFailed), view.Colors.ErrorInk)),
			ui.Rigid(func(gtx layout.Context) layout.Dimensions {
				if scan.retry.Clicked(gtx) {
					scan.invalidate()
					gtx.Execute(op.InvalidateCmd{})
				}
				return returnMonthButton(view, &scan.retry, i18n.HistoryRetry, nil, true)(gtx)
			}))
	case scan.report == nil:
		rows = append(rows, ui.Rigid(view.Message(l.Text(i18n.HealthLoading), view.Colors.SecondaryInk)))
	}
	for _, issue := range scan.findings {
		rows = append(rows, ui.Rigid(p.overviewHealthFinding(view, issue)))
	}
	if scan.report != nil && scan.report.IssueCount > 0 && len(scan.findings) == 0 {
		rows = append(rows, ui.Rigid(view.Message(fmt.Sprintf(l.Text(i18n.OverviewHealthIssues), scan.report.IssueCount), view.Colors.SecondaryInk)))
	}
	return rows
}

func (p *overviewPage) overviewHealthFinding(view ui.View, issue application.HealthIssue) layout.Widget {
	l := view.Language
	kind := healthKinds[issue.Kind]
	if kind == i18n.NoMessage {
		kind = i18n.HealthFinding
	}
	label := l.Text(kind)
	subject := healthIssueName(issue)
	if subject == "" && issue.AccountID != "" {
		for _, account := range p.current.value.AccountLabels {
			if account.ID == issue.AccountID && account.Name != account.ID {
				subject = account.Name
				break
			}
		}
	}
	if subject != "" {
		label = subject + " · " + label
	}
	button := portfolioButton(p.healthScan.links, issue.ID)
	tone := ui.BadgeNeutral
	switch issue.Severity {
	case application.HealthSeverityBlocking:
		tone = ui.BadgeDestructive
	case application.HealthSeverityWarning:
		tone = ui.BadgeWarning
	}
	details := []layout.Widget{view.Message(label, view.Colors.Ink)}
	if severity := healthSeverities[issue.Severity]; severity != i18n.NoMessage {
		details = append(details, func(gtx layout.Context) layout.Dimensions { return view.Badge(gtx, tone, l.Text(severity)) })
	}
	details = append(details, func(gtx layout.Context) layout.Dimensions {
		if button.Clicked(gtx) {
			p.openHealth(healthFindingFocus{id: issue.ID, label: label, from: issue.RangeStart, to: issue.RangeEnd}, button)
			gtx.Execute(op.InvalidateCmd{})
		}
		view.Focus.Watch(gtx, button)
		text := l.Text(i18n.OverviewViewGap)
		return view.ButtonOf(button, ui.ButtonSpec{Variant: ui.ButtonLink, Size: ui.ButtonSmall, Title: text, Name: text + " · " + label})(gtx)
	})
	return func(gtx layout.Context) layout.Dimensions {
		return (ui.Column{Gap: ui.Space4}).Layout(gtx, ui.Rigid(view.Hairline), ui.Rigid(func(gtx layout.Context) layout.Dimensions {
			return (ui.Wrap{Gap: ui.Space6, Middle: true}).Layout(gtx, len(details), func(gtx layout.Context, i int) layout.Dimensions {
				return details[i](gtx)
			})
		}))
	}
}
