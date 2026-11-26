package desktop

import (
	"context"
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type dataHealthReader interface {
	ScanMarketDataHealth(context.Context) (application.MarketDataHealthReport, error)
}

type healthAccountReader interface {
	ListAccounts(context.Context, domain.AccountFilter) ([]domain.AccountRecord, error)
}

type healthRead struct {
	report   application.MarketDataHealthReport
	accounts map[string]string
}

type dataHealthPage struct {
	ctx                        context.Context
	reader                     dataHealthReader
	invalidate                 func()
	workers                    sync.WaitGroup
	cancel                     context.CancelFunc
	request                    <-chan historyResult[healthRead]
	report                     *application.MarketDataHealthReport
	accountNames               map[string]string
	findings                   []healthIssueRow
	err                        error
	recheck                    widget.Clickable
	list                       widget.List
	issueActions               map[string]*widget.Clickable
	historyActions             map[string]*widget.Clickable
	openSettings               func(string, *widget.Clickable)
	openAccount                func(domain.AccountID, string, *widget.Clickable)
	openManual                 func(marketManualTarget, event.Tag)
	openInstrumentSource       func(domain.InstrumentID, string, event.Tag)
	openAgentEvidence          func(marketQuoteHistoryTarget, event.Tag)
	quoteHistory               func(layout.Context, ui.View, marketQuoteHistoryTarget, *widget.Clickable, bool) layout.Widget
	previewQuoteRepair         func(application.SyncRequest, event.Tag)
	openRepairDetails          func(event.Tag)
	repairDetails, retryRepair widget.Clickable
	manualSaved                marketManualReceipt
	clearFocus                 func(event.Tag)
	inspect                    func(healthFindingFocus, event.Tag)
	clear                      widget.Clickable
	focus                      healthFindingFocus
	repair                     *dataHealthRepair
	snapshots                  dataHealthSnapshots
}

type healthFindingFocus struct {
	from, to, account, instrument, currencyA, currencyB, label string
	id                                                         string
	snapshot                                                   snapshotFindingKey
}

func (f healthFindingFocus) active() bool { return f != (healthFindingFocus{}) }

func healthManualTarget(issue application.HealthIssue, focus healthFindingFocus) (marketManualTarget, bool) {
	target := marketManualTarget{date: issue.RangeStart}
	// A scoped date may narrow this finding, but an undated or related
	// prerequisite keeps its own date default.
	if target.date != "" && focus.from > target.date && focus.from <= snapshotRangeEnd(issue) {
		target.date = focus.from
	}
	if issue.InstrumentID != "" {
		id, err := domain.ParseInstrumentID(issue.InstrumentID)
		target.instrumentID = id
		return target, err == nil
	}
	base, baseErr := domain.ParseSupportedCurrency(issue.CurrencyA)
	quote, quoteErr := domain.ParseSupportedCurrency(issue.CurrencyB)
	target.base, target.quote = base, quote
	return target, baseErr == nil && quoteErr == nil && base != quote
}

func focusedHealthFindings(issues []application.HealthIssue, focus healthFindingFocus) []application.HealthIssue {
	var matches []application.HealthIssue
	for _, issue := range issues {
		if !focus.active() && issue.Collapsed {
			continue
		}
		if focus.instrument != "" && focus.instrument != issue.InstrumentID || focus.account != "" && focus.account != issue.AccountID {
			continue
		}
		if focus.id != "" && focus.id != issue.ID || focus.snapshot != (snapshotFindingKey{}) && focus.snapshot != snapshotKey(issue) {
			continue
		}
		if focus.currencyA != "" &&
			!(focus.currencyA == issue.CurrencyA && focus.currencyB == issue.CurrencyB) &&
			!(focus.currencyA == issue.CurrencyB && focus.currencyB == issue.CurrencyA) {
			continue
		}
		if focus.from != "" && issue.RangeEnd != "" && issue.RangeEnd < focus.from ||
			focus.to != "" && issue.RangeStart != "" && issue.RangeStart > focus.to {
			continue
		}
		matches = append(matches, issue)
	}
	return matches
}

func (p *dataHealthPage) enter(position layout.Position, focus healthFindingFocus) {
	p.manualSaved = marketManualReceipt{}
	p.list.Position = position
	p.focus = focus
	p.refreshFindings()
	p.read()
}

func (p *dataHealthPage) leave() {
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel, p.request = nil, nil
}

func (p *dataHealthPage) read() {
	p.leave()
	ctx, cancel := context.WithCancel(p.ctx)
	p.cancel, p.err = cancel, nil
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (healthRead, error) {
		report, err := p.reader.ScanMarketDataHealth(ctx)
		result := healthRead{report: report}
		if reader, ok := p.reader.(healthAccountReader); err == nil && ok {
			result.accounts = snapshotAccountNames(ctx, reader, report.Issues)
		}
		return result, err
	})
}

func (p *dataHealthPage) update() {
	select {
	case response := <-p.request:
		p.leave()
		p.err = response.err
		if p.err == nil {
			p.report, p.accountNames = &response.value.report, response.value.accounts
			p.refreshFindings()
		}
	default:
	}
}

func snapshotAccountNames(ctx context.Context, reader healthAccountReader, issues []application.HealthIssue) map[string]string {
	filter := domain.AccountFilter{IncludeArchived: true}
	seen := map[domain.AccountID]bool{}
	for _, issue := range issues {
		if !isSnapshotFinding(issue) || issue.AccountID == "" {
			continue
		}
		if id, err := domain.ParseAccountID(issue.AccountID); err == nil && !seen[id] {
			filter.AccountIDs = append(filter.AccountIDs, id)
			seen[id] = true
		}
	}
	if len(filter.AccountIDs) == 0 {
		return nil
	}
	accounts, err := reader.ListAccounts(ctx, filter)
	if err != nil {
		return nil // Names are supplemental; keep the diagnostic report usable.
	}
	names := make(map[string]string, len(accounts))
	for _, account := range accounts {
		names[account.Account.ID.String()] = account.Account.Name
	}
	return names
}

func (p *dataHealthPage) refreshFindings() {
	if p.report == nil {
		return
	}
	matches := focusedHealthFindings(p.report.Issues, p.focus)
	p.findings = healthIssueRows(matches)
	for i, dependency := range healthIssueRows(snapshotDependencies(p.report.Issues, matches)) {
		dependency.section, dependency.startSection = healthDependencies, i == 0
		p.findings = append(p.findings, dependency)
	}
	p.list.Position = restoredPosition(p.list.Position, len(p.findings))
}

type healthIssueSection int

const (
	healthRepairable healthIssueSection = iota
	healthPrerequisite
	healthReported
	healthDependencies
)

type healthIssueRow struct {
	section      healthIssueSection
	startSection bool
	startGroup   bool
	groupCount   int
	issue        application.HealthIssue
	combined     bool
}

func healthIssueRows(issues []application.HealthIssue) []healthIssueRow {
	var sections [3][]application.HealthIssue
	for _, issue := range issues {
		section := healthReported
		switch {
		case issue.Executable && !issue.Collapsed:
			section = healthRepairable
		case issue.Action != application.HealthActionNone && issue.Action != application.HealthActionRepair:
			section = healthPrerequisite
		}
		sections[section] = append(sections[section], issue)
	}
	var rows []healthIssueRow
	for section, issues := range sections {
		for start := 0; start < len(issues); {
			end := start + 1
			for end < len(issues) && issues[end].Kind == issues[start].Kind {
				end++
			}
			group := issues[start:end]
			findings := snapshotFindingRows(group)
			for index, finding := range findings {
				rows = append(rows, healthIssueRow{
					section: healthIssueSection(section), startSection: start == 0 && index == 0, startGroup: index == 0,
					groupCount: len(group), issue: finding.issue, combined: finding.combined,
				})
			}
			start = end
		}
	}
	return rows
}

func healthIssueName(issue application.HealthIssue) string {
	if name := strings.TrimSpace(issue.Label); name != "" && name != issue.InstrumentID && name != issue.AccountID {
		return name
	}
	if issue.CurrencyA != "" && issue.CurrencyB != "" {
		return issue.CurrencyA + "/" + issue.CurrencyB
	}
	return ""
}
