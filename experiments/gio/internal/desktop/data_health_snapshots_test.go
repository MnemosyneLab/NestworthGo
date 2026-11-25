package desktop

import (
	"context"
	"errors"
	"image"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gioui.org/io/key"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/settings"
	"github.com/waltwang/nestworth-go/internal/wailsapi/marketdata"
)

func TestDataHealthSnapshotGroupingAndBlockingDependencies(t *testing.T) {
	component := application.HealthIssue{Kind: application.HealthKindSnapshotIncomplete, Label: "Synthetic fund", AccountID: "account-a",
		InstrumentID: "fund", Reason: "snapshot_incomplete", Collapsed: true}
	issue := func(id, date, account string) application.HealthIssue {
		r := component
		r.ID, r.RangeStart, r.RangeEnd, r.AccountID = id, date, date, account
		return r
	}
	items := []application.HealthIssue{
		issue("two", "2026-11-02", "account-a"), issue("other", "2026-11-02", "account-b"),
		issue("one", "2026-11-01", "account-a"), issue("four", "2026-11-04", "account-a"),
	}
	rows := snapshotFindingRows(items)
	if len(rows) != 3 || !rows[0].combined || rows[0].issue.ID != "one" || rows[0].issue.RangeEnd != "2026-11-02" || rows[0].issue.RangeCount != 2 || rows[1].issue.ID != "four" {
		t.Fatalf("a healthy date or distinct account was merged: %+v", rows)
	}
	selection, ok := snapshotInspection(rows[0], "Synthetic fund", healthFindingFocus{from: "2026-11-02", to: "2026-11-03"})
	if !ok || selection.id != "" || selection.from != "2026-11-02" || selection.to != "2026-11-02" {
		t.Fatalf("inspection lost focused date and grouping: %+v", selection)
	}
	provider := application.HealthIssue{ID: "provider", Kind: application.HealthKindMissingProviderKey, Severity: application.HealthSeverityBlocking, RangeEnd: "2026-10-31"}
	items = append(items, provider)
	matches := focusedHealthFindings(items, selection)
	dependencies := snapshotDependencies(items, matches)
	if len(matches) != 1 || matches[0].ID != "two" || !reflect.DeepEqual(dependencies, []application.HealthIssue{provider}) {
		t.Fatalf("component focus or root prerequisite = %+v, %+v", matches, dependencies)
	}
}

func TestDataHealthSnapshotRebuildRequiresConfirmationAndAppendsOnlySelectedRevision(t *testing.T) {
	ctx := t.Context()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "snapshots.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo := sqlite.NewRepository(db)
	provider := &unavailableOverviewProvider{}
	svc := application.NewService(repo, application.NewMarketDataRegistry(provider))
	now := time.Date(2026, 11, 21, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	svc.SetClock(clock)
	if err := svc.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic snapshots", BaseCurrency: "USD", MemberNames: []string{"Sample owner"}}); err != nil {
		t.Fatal(err)
	}
	bs, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAccount(ctx, application.AccountInput{Name: "Synthetic cash", AccountType: "cash_on_hand", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD",
		InitialAmount: "125.25", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bs.Members[0].ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	now = now.AddDate(0, 0, 4)
	if n, err := svc.RebuildHistoricalSnapshots(ctx, "2026-11-21", "2026-11-24"); err != nil || n != 4 {
		t.Fatalf("synthetic seed rebuilt %d days: %v", n, err)
	}
	// Model old derived data with otherwise unchanged authoritative facts.
	if _, err := db.SQL.Exec(`UPDATE daily_valuation_snapshots SET content_hash = 'v2:synthetic-old', resolver_policy_version = 'synthetic-old' WHERE local_date IN ('2026-11-22', '2026-11-23')`); err != nil {
		t.Fatal(err)
	}
	snapshotRows := func() []domain.DailyValuationSnapshot {
		t.Helper()
		rows, err := repo.ListDailyValuationSnapshots(ctx, bs.Household.ID, time.Time{}, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		return rows
	}
	initial := snapshotRows()
	fixture := overviewHistoryFixture{overviewReader: svc, trend: domain.NetWorthTrend{Currency: "USD", StartDate: "2026-11-22", EndDate: "2026-11-22",
		Points: []domain.NetWorthTrendPoint{{LocalDate: "2026-11-22"}}}}
	overview := newOverviewPage(ctx, fixture, clock, func() {})
	p := &dataHealthPage{ctx: ctx, reader: svc, invalidate: func() {}, snapshots: dataHealthSnapshots{service: svc}}
	t.Cleanup(overview.workers.Wait)
	t.Cleanup(p.workers.Wait)
	h := newShellHarnessWithPages(t, ledgerPages{overview: overview, dataHealth: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.size = image.Pt(1130, 970)
	h.frame()
	overview.workers.Wait()
	h.frame()
	overview.workers.Wait()
	h.frame()
	h.click("View period coverage gaps", false)
	h.frame()
	p.workers.Wait()
	h.frame()
	if p.focus.from != "2026-11-22" || p.focus.to != "2026-11-22" {
		t.Fatalf("Overview selected %+v", p.focus)
	}
	ref, err := marketdata.NewService(svc, nil).ScanMarketDataHealth()
	if err != nil || !reflect.DeepEqual(marketdata.HealthReportDTO(*p.report), ref) {
		t.Fatalf("local health scan diverged from Wails: %v", err)
	}
	h.click("Inspect snapshot finding", false)
	h.frame()
	p.workers.Wait()
	h.frame()
	issueID := p.focus.id
	if issueID == "" {
		t.Fatal("inspection did not isolate the exact issue")
	}
	h.click("Back", false)
	h.frame()
	p.workers.Wait()
	h.frame()
	h.focused(p.snapshots.inspect[issueID])
	h.press(key.NameReturn)
	h.frame()
	p.workers.Wait()
	h.frame()
	h.size = image.Pt(700, 480)
	h.frame()
	h.click("Preview snapshot repair", false)
	h.frame()
	if r := p.snapshots.active; r == nil || r.from != "2026-11-22" || r.to != "2026-11-22" || r.request != nil {
		t.Fatalf("preview scheduled a write or widened the range: %+v", r)
	}
	h.focused(&p.snapshots.active.sheet.close)
	h.press(key.NameEscape)
	h.frame()
	h.focused(p.snapshots.preview[issueID])
	if !reflect.DeepEqual(snapshotRows(), initial) {
		t.Fatal("scan, inspection, preview or dismissal changed snapshot rows")
	}
	h.press(key.NameReturn)
	h.frame()
	h.click("Confirm and rebuild", false)
	p.workers.Wait()
	h.frame()
	p.workers.Wait()
	h.frame()
	assertPageLabel(t, &h.router, "Snapshot rebuild finished.")
	assertPageLabel(t, &h.router, "Snapshot revisions added · 1")
	latest := snapshotRows()
	for i, row := range latest {
		if row.LocalDate == "2026-11-22" {
			if row.ID == initial[i].ID || row.SupersedesID == nil || *row.SupersedesID != initial[i].ID || !row.Complete || row.AssetsAmount.CanonicalAmount() != "125.25" {
				t.Fatalf("rebuilt row = %+v", row)
			}
		} else if !reflect.DeepEqual(row, initial[i]) {
			t.Fatalf("rebuild escaped selected date to %s", row.LocalDate)
		}
	}
	if len(latest) != len(initial) || provider.requests != 0 {
		t.Fatalf("repair widened snapshot dates or requested network data: %d days, %d requests", len(latest), provider.requests)
	}
	h.click("Close", false)
	h.frame()
	h.focused(&p.recheck)
	assertPageLabel(t, &h.router, "No saved findings match this focus.")
	h.click("Back", false)
	h.frame()
	p.workers.Wait()
	h.frame()
	h.click("Back", false)
	h.frame()
	h.focused(&overview.trend.gaps)
}

type scriptedSnapshotBatches struct {
	called chan [2]string
	result chan historyResult[int]
}

func (s scriptedSnapshotBatches) RebuildHistoricalSnapshots(ctx context.Context, from, to string) (int, error) {
	s.called <- [2]string{from, to}
	select {
	case r := <-s.result:
		return r.value, r.err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func TestDataHealthSnapshotStopAndRetryKeepCompletedBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	batches := scriptedSnapshotBatches{make(chan [2]string, 8), make(chan historyResult[int], 1)}
	p := &dataHealthPage{ctx: ctx, invalidate: func() {}, snapshots: dataHealthSnapshots{service: batches},
		reader: fixedDataHealthRead{Issues: []application.HealthIssue{{ID: "ready", Kind: application.HealthKindSnapshotOutdated,
			Action: application.HealthActionRepair, Executable: true, RangeStart: "2026-01-01", RangeEnd: "2026-03-05"}}}}
	t.Cleanup(func() { cancel(); p.workers.Wait() })
	h := newShellHarnessWithPages(t, ledgerPages{marketData: &marketDataPage{}, dataHealth: p})
	h.shell.prefs.saved.Language = settings.LanguageEnglish
	h.shell.trail = newNavigationTrail(dataHealthRoute)
	h.size = image.Pt(700, 480)
	p.read()
	p.workers.Wait()
	h.frame()
	expectCall := func(from, to string) {
		t.Helper()
		select {
		case got := <-batches.called:
			if got != [2]string{from, to} {
				t.Fatalf("batch = %v, want %s through %s", got, from, to)
			}
		case <-time.After(time.Second):
			t.Fatalf("no batch started for %s through %s", from, to)
		}
	}
	finish := func(count int, err error) {
		t.Helper()
		batches.result <- historyResult[int]{value: count, err: err}
		p.workers.Wait()
		h.frame()
	}
	h.click("Preview snapshot repair", false)
	h.frame()
	h.click("Confirm and rebuild", false)
	expectCall("2026-01-01", "2026-01-31")
	h.press(key.NameEscape)
	if p.snapshots.active == nil || p.snapshots.active.request == nil {
		t.Fatal("Escape dismissed a running rebuild")
	}
	h.click("Stop after current batch", false)
	finish(31, nil)
	assertPageLabel(t, &h.router, "Paused · 2026-02-01 – 2026-03-05 remains")
	if len(batches.called) != 0 || p.snapshots.active.request != nil {
		t.Fatal("stop dispatched a second batch")
	}
	h.click("Resume rebuild", false)
	expectCall("2026-02-01", "2026-03-03")
	finish(2, errors.New("synthetic interruption after two days"))
	assertPageLabel(t, &h.router, "Could not finish the batch · 2026-02-01 – 2026-03-03")
	h.click("Retry", false)
	expectCall("2026-02-01", "2026-03-03")
	finish(29, nil)
	expectCall("2026-03-04", "2026-03-05")
	finish(2, nil)
	assertPageLabel(t, &h.router, "Snapshot revisions added · 64")
	assertPageLabel(t, &h.router, "Snapshot rebuild finished.")
}
