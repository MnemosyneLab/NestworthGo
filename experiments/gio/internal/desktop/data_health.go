package desktop

import (
	"context"
	"strings"
	"sync"

	"gioui.org/io/event"
	"gioui.org/layout"
	"gioui.org/widget"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

type dataHealthReader interface {
	ScanMarketDataHealth(context.Context) (application.MarketDataHealthReport, error)
}

type dataHealthPage struct {
	ctx          context.Context
	reader       dataHealthReader
	invalidate   func()
	workers      sync.WaitGroup
	cancel       context.CancelFunc
	request      <-chan historyResult[application.MarketDataHealthReport]
	report       *application.MarketDataHealthReport
	err          error
	recheck      widget.Clickable
	list         widget.List
	issueActions map[string]*widget.Clickable
	openSettings func(*widget.Clickable)
	openAccount  func(domain.AccountID, string, *widget.Clickable)
	clearFocus   func(event.Tag)
	clear        widget.Clickable
	focus        healthFindingFocus
	repair       *dataHealthRepair
}

type healthFindingFocus struct {
	from, to, account, instrument, currencyA, currencyB, label string
}

func (f healthFindingFocus) active() bool { return f != (healthFindingFocus{}) }

func focusedHealthFindings(issues []application.HealthIssue, focus healthFindingFocus) []application.HealthIssue {
	var matches []application.HealthIssue
	for _, issue := range issues {
		if !focus.active() && issue.Collapsed {
			continue
		}
		if focus.instrument != "" && focus.instrument != issue.InstrumentID || focus.account != "" && focus.account != issue.AccountID {
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
	p.list.Position = position
	p.focus = focus
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
	p.request = runHistoryRead(&p.workers, p.invalidate, func() (application.MarketDataHealthReport, error) {
		return p.reader.ScanMarketDataHealth(ctx)
	})
}

func (p *dataHealthPage) update() {
	select {
	case response := <-p.request:
		p.leave()
		p.err = response.err
		if p.err == nil {
			p.report = &response.value
		}
	default:
	}
}

type healthIssueSection int

const (
	healthRepairable healthIssueSection = iota
	healthPrerequisite
	healthReported
)

type healthIssueRow struct {
	section      healthIssueSection
	startSection bool
	startGroup   bool
	groupCount   int
	issue        application.HealthIssue
}

func healthIssueRows(issues []application.HealthIssue) []healthIssueRow {
	var sections [3][]application.HealthIssue
	for _, issue := range issues {
		section := healthReported
		switch {
		case issue.Executable:
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
			for index := start; index < end; index++ {
				rows = append(rows, healthIssueRow{
					section: healthIssueSection(section), startSection: index == 0, startGroup: index == start,
					groupCount: end - start, issue: issues[index],
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
