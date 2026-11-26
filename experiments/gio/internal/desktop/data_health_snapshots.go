package desktop

import (
	"context"
	"slices"
	"strings"
	"time"

	"gioui.org/widget"
	"github.com/waltwang/nestworth-go/experiments/gio/internal/ui"
	"github.com/waltwang/nestworth-go/internal/application"
)

type snapshotFindingKey struct {
	kind, account, instrument, label, base, quote, target, provider, reason, action, code, severity string
	executable, collapsed                                                                           bool
}

func snapshotKey(issue application.HealthIssue) snapshotFindingKey {
	return snapshotFindingKey{
		issue.Kind, issue.AccountID, issue.InstrumentID, issue.Label, issue.CurrencyA, issue.CurrencyB,
		issue.TargetKey, issue.Provider, issue.Reason, issue.Action, issue.Code, issue.Severity, issue.Executable, issue.Collapsed,
	}
}

func isSnapshotFinding(issue application.HealthIssue) bool {
	switch issue.Kind {
	case application.HealthKindSnapshotMissing, application.HealthKindSnapshotOutdated, application.HealthKindSnapshotIncomplete:
		return true
	default:
		return false
	}
}

// Group only adjacent observations of the same component and diagnosis, as the
// Wails list does. A healthy day stays a visible break between two findings.
func snapshotFindingRows(issues []application.HealthIssue) []healthIssueRow {
	rows := make([]healthIssueRow, 0, len(issues))
	if len(issues) == 0 || issues[0].Kind != application.HealthKindSnapshotIncomplete {
		for _, issue := range issues {
			rows = append(rows, healthIssueRow{issue: issue})
		}
		return rows
	}
	groups := make(map[snapshotFindingKey][]application.HealthIssue)
	var keys []snapshotFindingKey
	for _, issue := range issues {
		key := snapshotKey(issue)
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], issue)
	}
	for _, key := range keys {
		group := groups[key]
		slices.SortStableFunc(group, func(a, b application.HealthIssue) int { return strings.Compare(a.RangeStart, b.RangeStart) })
		start := len(rows)
		for _, issue := range group {
			if len(rows) > start {
				previous := &rows[len(rows)-1]
				end, valid := ui.ParseCivilDay(snapshotRangeEnd(previous.issue))
				if valid && end.AddDate(0, 0, 1).Format(time.DateOnly) == issue.RangeStart {
					previous.issue.RangeEnd = snapshotRangeEnd(issue)
					previous.issue.RangeCount = max(1, previous.issue.RangeCount) + max(1, issue.RangeCount)
					previous.combined = true
					continue
				}
			}
			rows = append(rows, healthIssueRow{issue: issue})
		}
	}
	return rows
}

func snapshotRangeEnd(issue application.HealthIssue) string {
	if issue.RangeEnd == "" {
		return issue.RangeStart
	}
	return issue.RangeEnd
}

func snapshotWindow(issue application.HealthIssue, focus healthFindingFocus) (from, to string, valid bool) {
	if issue.RangeStart == "" {
		return "", "", false
	}
	from, to = max(issue.RangeStart, focus.from), snapshotRangeEnd(issue)
	if focus.to != "" {
		to = min(to, focus.to)
	}
	_, fromValid := ui.ParseCivilDay(from)
	_, toValid := ui.ParseCivilDay(to)
	return from, to, fromValid && toValid && from <= to
}

func snapshotInspection(row healthIssueRow, label string, focus healthFindingFocus) (healthFindingFocus, bool) {
	issue := row.issue
	from, to, valid := snapshotWindow(issue, focus)
	if !valid || !isSnapshotFinding(issue) {
		return healthFindingFocus{}, false
	}
	selection := healthFindingFocus{from: from, to: to, label: label, id: issue.ID}
	if row.combined {
		selection.id = ""
		selection.snapshot = snapshotKey(issue)
	}
	return selection, selection != focus
}

func snapshotDependencies(issues, matches []application.HealthIssue) []application.HealthIssue {
	if !slices.ContainsFunc(matches, func(issue application.HealthIssue) bool { return isSnapshotFinding(issue) && issue.Collapsed }) {
		return nil
	}
	shown := make(map[string]bool, len(matches))
	for _, issue := range matches {
		shown[issue.ID] = true
	}
	var dependencies []application.HealthIssue
	for _, issue := range issues {
		if !shown[issue.ID] && !issue.Collapsed && !isSnapshotFinding(issue) && issue.Severity == application.HealthSeverityBlocking {
			dependencies = append(dependencies, issue)
		}
	}
	return dependencies
}

type snapshotRebuilder interface {
	RebuildHistoricalSnapshots(context.Context, string, string) (int, error)
}

type dataHealthSnapshots struct {
	service snapshotRebuilder
	active  *snapshotRebuild
	restore *widget.Clickable
	rebuilt func()
	inspect map[string]*widget.Clickable
	preview map[string]*widget.Clickable
}

type snapshotRebuild struct {
	sheet                     analysisSheet
	trigger                   *widget.Clickable
	from, to, next, batchLast string
	request                   <-chan historyResult[int]
	revisions                 int
	err                       error
	stopping, paused          bool
}

func (p *dataHealthPage) rebuildSnapshotBatch() {
	r := p.snapshots.active
	next, _ := ui.ParseCivilDay(r.next)
	// The shared API accepts at most 31 inclusive civil dates per call.
	from, to := r.next, min(r.to, next.AddDate(0, 0, 30).Format(time.DateOnly))
	r.batchLast, r.err, r.paused = to, nil, false
	r.request = runHistoryRead(&p.workers, p.invalidate, func() (int, error) {
		return p.snapshots.service.RebuildHistoricalSnapshots(p.ctx, from, to)
	})
}

func (p *dataHealthPage) updateSnapshotRebuild() {
	r := p.snapshots.active
	select {
	case result := <-r.request:
		r.request, r.err = nil, result.err
		r.revisions += result.value
		if result.err == nil {
			last, _ := ui.ParseCivilDay(r.batchLast)
			r.next = last.AddDate(0, 0, 1).Format(time.DateOnly)
			if !r.stopping && r.next <= r.to {
				p.rebuildSnapshotBatch()
				return
			}
		}
		r.paused, r.stopping = r.stopping && r.next <= r.to, false
		p.read()
		if p.snapshots.rebuilt != nil {
			p.snapshots.rebuilt()
		}
	default:
	}
}
