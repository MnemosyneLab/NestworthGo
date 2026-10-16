package application

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewPublicationIsAtomicAndSameDayIdempotent(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic daily cost", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()})
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(f.db)
	var firstChanges int
	for attempt := 0; attempt < 2; attempt++ {
		capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
		if err != nil {
			t.Fatal(err)
		}
		stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
		if err != nil || len(stage.Rules) != 1 || len(stage.Rules[0].Days) != 1 {
			t.Fatalf("stage %d: %+v %v", attempt, stage, err)
		}
		if err := repo.PublishReviewProjection(t.Context(), stage, *f.now); err != nil {
			t.Fatalf("publish %d: %v", attempt, err)
		}
		var changes int
		if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&changes); err != nil {
			t.Fatal(err)
		}
		if attempt == 0 {
			firstChanges = changes
		} else if changes != firstChanges {
			t.Fatalf("identical same-day publication wrote %d rows", changes-firstChanges)
		}
	}
	for table, expected := range map[string]int{"investment_review_daily_inputs": 1, "investment_review_daily_results": 1, "investment_review_position_epochs": 1, "investment_review_episodes": 1, "investment_review_episode_events": 1, "investment_review_evaluation_state": 1} {
		var count int
		if err := f.db.SQL.QueryRow(`SELECT count(*) FROM ` + table).Scan(&count); err != nil || count != expected {
			t.Fatalf("%s has %d rows, want %d: %v", table, count, expected, err)
		}
	}
	var triggered, state string
	if err := f.db.SQL.QueryRow(`SELECT triggered_on FROM investment_review_episodes WHERE rule_id=?`, string(rule.ID)).Scan(&triggered); err != nil || triggered != "2026-08-01" {
		t.Fatalf("episode: %s %v", triggered, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT policy_state FROM investment_review_evaluation_state WHERE rule_id=?`, string(rule.ID)).Scan(&state); err != nil || state != "triggered" {
		t.Fatalf("state: %s %v", state, err)
	}
	var generation int
	if err := f.db.SQL.QueryRow(`SELECT input_generation FROM investment_review_control`).Scan(&generation); err != nil || generation != 1 {
		t.Fatalf("generation: %d %v", generation, err)
	}
	var beforeRead, afterRead int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&beforeRead); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "pending"}, "", 10)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Rules) != 1 || page.Items[0].Rules[0].EpisodeID == "" || !page.Items[0].Rules[0].Actionable || page.ProjectionStatus != "complete" {
		t.Fatalf("published alert report: %+v %v", page, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&afterRead); err != nil || afterRead != beforeRead {
		t.Fatalf("alert read wrote %d rows: %v", afterRead-beforeRead, err)
	}
	alert := page.Items[0].Rules[0]
	coverage, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{Status: "all"}, "", 10)
	if err != nil || len(coverage.Items) != 1 || coverage.Items[0].Status != "triggered" || len(coverage.Items[0].Rules) != 1 || coverage.Items[0].Rules[0].Window.HitCount != 1 {
		t.Fatalf("published positive-holding coverage: %+v %v", coverage, err)
	}
	detail, err := f.s.GetReviewAlertV2(t.Context(), string(alert.EpisodeID))
	if err != nil || detail.InstrumentID != f.instrument.ID || len(detail.Rules) != 1 || detail.Rules[0].EpisodeID != alert.EpisodeID {
		t.Fatalf("current alert detail: %+v %v", detail, err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), string(alert.EpisodeID), string(domain.NewMutationID()), alert.EpisodeRevision); err != nil {
		t.Fatal(err)
	}
	acknowledgedPage, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "acknowledged"}, "", 10)
	if err != nil || len(acknowledgedPage.Items) != 1 || acknowledgedPage.Items[0].Rules[0].Actionable {
		t.Fatalf("acknowledgement did not change read handling: %+v %v", acknowledgedPage, err)
	}
	unignoredPage, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "unignored"}, "", 10)
	if err != nil || len(unignoredPage.Items) != 1 || unignoredPage.Items[0].Rules[0].Status != "acknowledged" {
		t.Fatalf("acknowledged alert disappeared from unignored view: %+v %v", unignoredPage, err)
	}
	if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "permanent"}); err != nil {
		t.Fatal(err)
	}
	ignoredPage, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "ignored"}, "", 10)
	if err != nil || len(ignoredPage.Items) != 1 || ignoredPage.Items[0].Rules[0].SuppressionID == "" || ignoredPage.Items[0].Rules[0].Actionable {
		t.Fatalf("permanent ignore did not change read handling: %+v %v", ignoredPage, err)
	}
	unignoredPage, err = f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "unignored"}, "", 10)
	if err != nil || len(unignoredPage.Items) != 0 {
		t.Fatalf("ignored alert remains in unignored view: %+v %v", unignoredPage, err)
	}
}

func TestReviewPublicationRejectsDeletedFinancialSource(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic daily cost", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()}); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(f.db)
	capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`DELETE FROM instrument_quotes WHERE instrument_id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishReviewProjection(t.Context(), stage, *f.now); err == nil {
		t.Fatal("published stale financial evidence")
	}
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_results`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale publication leaked %d daily results: %v", count, err)
	}
}

func TestReviewPublicationPreservesAcknowledgementAcrossUnknownDay(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "50", Currency: "CNY", FXMode: "native"}}}}
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic priced day", Enabled: true, Severity: "warning", Spec: spec}); err != nil {
		t.Fatal(err)
	}
	f.agent(t) // real agent close labelled 8/1
	f.day(t, "2026-08-02")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episode string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&episode); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-03") // 8/2 has no labelled close
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var policyState string
	if err := f.db.SQL.QueryRow(`SELECT policy_state FROM investment_review_evaluation_state`).Scan(&policyState); err != nil || policyState != "unknown" {
		t.Fatalf("missing close should pause an alert: %q %v", policyState, err)
	}
	f.agent(t) // labelled 8/3
	f.day(t, "2026-08-04")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var current, acknowledged string
	if err := f.db.SQL.QueryRow(`SELECT id,acknowledged_at FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&current, &acknowledged); err != nil || current != episode || acknowledged == "" {
		t.Fatalf("T-U-T lost original acknowledged episode: %q %q %v", current, acknowledged, err)
	}
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episode_events`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("T-U-T emitted another event: %d %v", count, err)
	}
}

func TestReviewPublicationRejectsConcurrentAcknowledgement(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic daily cost", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(f.db)
	capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	var episode string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&episode); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishReviewProjection(t.Context(), stage, *f.now); err == nil {
		t.Fatal("stale evaluation overwrote concurrent acknowledgement")
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var acknowledged string
	if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, episode).Scan(&acknowledged); err != nil || acknowledged == "" {
		t.Fatalf("reconciliation lost acknowledgement: %q %v", acknowledged, err)
	}
}

func TestReviewPublicationRejectsConcurrentIgnoreAndMidnight(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic daily cost", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()}); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(f.db)
	capture, err := repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "permanent"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishReviewProjection(t.Context(), stage, *f.now); err == nil {
		t.Fatal("stale evaluation ignored a concurrent suppression")
	}
	capture, err = repo.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	stage, err = PrepareReviewPublication(t.Context(), capture, *f.now, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.PublishReviewProjection(t.Context(), stage, time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("published prior day's stage after local midnight")
	}
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_results`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("conflicting publication leaked %d results: %v", count, err)
	}
}

func TestReviewPublicationRebuyOpensNewEpisodeWithoutDiscardingRollingVotes(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	spec := syntheticHistoricalReviewSpec()
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 6, MinHits: 5}
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic rolling cost", Enabled: true, Severity: "warning", Spec: spec}); err != nil {
		t.Fatal(err)
	}
	ignore, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "permanent"})
	if err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-06") // closed days 1–5 are all T
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var old string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&old); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), old, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	f.quantity(t, false) // 10/6 position zero, but rolling still has five earlier T days
	f.day(t, "2026-10-07")
	f.quantity(t, true) // 10/7 re-buy; rolling window 2–7 still has five T days
	f.day(t, "2026-10-08")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var current, triggered string
	var acknowledged *string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,acknowledged_at FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&current, &triggered, &acknowledged); err != nil || current == old || triggered != "2026-10-07" || acknowledged != nil {
		t.Fatalf("new epoch inherited old episode or lost rolling votes: %q %q %v %v", current, triggered, acknowledged, err)
	}
	var reason, oldAcknowledged string
	if err := f.db.SQL.QueryRow(`SELECT resolved_reason,acknowledged_at FROM investment_review_episodes WHERE id=?`, old).Scan(&reason, &oldAcknowledged); err != nil || reason != "position_closed" || oldAcknowledged == "" {
		t.Fatalf("old acknowledged episode not retained as closed: %q %q %v", reason, oldAcknowledged, err)
	}
	var hitCount int
	if err := f.db.SQL.QueryRow(`SELECT hit_count FROM investment_review_evaluation_state WHERE episode_id=?`, current).Scan(&hitCount); err != nil || hitCount != 5 {
		t.Fatalf("historical rolling T days were reset at re-buy: %d %v", hitCount, err)
	}
	repo := sqlite.NewRepository(f.db)
	listed, err := repo.ListReviewSuppressionsV2(t.Context(), f.account.Account.HouseholdID, "", 10)
	if err != nil || len(listed) != 1 || listed[0].ID != ignore.ID {
		t.Fatalf("permanent ignore was overwritten: %+v %v", listed, err)
	}
	active, err := domain.ActiveReviewSuppression(listed, f.instrument.ID, "new-rule", *f.now)
	if err != nil || active == nil || active.ID != ignore.ID {
		t.Fatalf("permanent ignore stopped covering a new episode: %+v %v", active, err)
	}
}

func TestReviewSuppressionExpirySurvivesRestoredProjection(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic rule", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()}); err != nil {
		t.Fatal(err)
	}
	ignore, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", Days: 7})
	if err != nil || ignore.UntilOn != "2026-10-08" {
		t.Fatalf("committed resume date: %+v %v", ignore, err)
	}
	copyPath := filepath.Join(t.TempDir(), "synthetic-restored.db")
	if err := f.db.SnapshotTo(t.Context(), copyPath); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	service := NewService(sqlite.NewRepository(restored))
	repo := sqlite.NewRepository(restored)
	for _, tc := range []struct {
		at     time.Time
		active bool
	}{
		{time.Date(2026, 10, 7, 23, 59, 0, 0, time.UTC), true},
		{time.Date(2026, 10, 8, 0, 0, 0, 0, time.UTC), false},
	} {
		service.SetClock(func() time.Time { return tc.at })
		if _, err := service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		entries, err := repo.ListReviewSuppressionsV2(t.Context(), f.account.Account.HouseholdID, "", 10)
		if err != nil || len(entries) != 1 || entries[0].UntilOn != ignore.UntilOn {
			t.Fatalf("restore/reconcile recalculated committed expiry: %+v %v", entries, err)
		}
		selected, err := domain.ActiveReviewSuppression(entries, f.instrument.ID, "future-rule", tc.at)
		if err != nil || (selected != nil) != tc.active {
			t.Fatalf("restored expiry at %s: %+v %v", tc.at, selected, err)
		}
	}
}

func TestReviewReportRevisionChangesAtSuppressionDayNotEveryMinute(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	if _, err := f.s.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: f.instrument.ID.String(), Mode: "until_date", Days: 7}); err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(f.db)
	read := func(at time.Time) string {
		page, err := repo.ListReviewAlertGroupsV2(t.Context(), f.account.Account.HouseholdID, domain.ReviewAlertFilter{}, "", 1, at)
		if err != nil {
			t.Fatal(err)
		}
		return page.ReportRevision
	}
	first := read(time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC))
	if later := read(time.Date(2026, 8, 2, 9, 45, 0, 0, time.UTC)); later != first {
		t.Fatal("report cursor invalidated within an unchanged civil day")
	}
	if tomorrow := read(time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)); tomorrow == first {
		t.Fatal("report revision did not change at a suppression civil day")
	}
}

func TestReviewCoverageIncludesUnknownWithoutAnAlertAndExcludesZeroHolding(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: "<", Value: "-1", LookbackDays: 30}}}}
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic missing price", Enabled: true, Severity: "notice", Spec: spec}); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-02")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	page, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{Status: "unknown"}, "", 10)
	if err != nil || len(page.Items) != 1 || len(page.Items[0].Rules) != 1 || page.Items[0].Rules[0].State != "unknown" {
		t.Fatalf("missing data coverage: %+v %v", page, err)
	}
	alerts, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "all"}, "", 10)
	if err != nil || len(alerts.Items) != 0 {
		t.Fatalf("unknown produced an alert: %+v %v", alerts, err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE instruments SET archived_at='2026-08-02T12:00:00Z' WHERE id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	archived, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{Status: "unknown"}, "", 10)
	if err != nil || len(archived.Items) != 1 {
		t.Fatalf("archived positive holding disappeared: %+v %v", archived, err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE instruments SET archived_at=NULL WHERE id=?`, f.instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	f.quantity(t, false)
	zero, err := f.s.ListReviewCoverageV2(t.Context(), domain.ReviewCoverageFilter{Status: "all"}, "", 10)
	if err != nil || len(zero.Items) != 0 {
		t.Fatalf("zero holding remained current coverage: %+v %v", zero, err)
	}
}

func TestArchivedReviewRuleClosesEpisodeWithoutReplayingHistory(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic archive", Enabled: true, Severity: "notice", Spec: syntheticHistoricalReviewSpec()})
	if err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-02")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ArchiveReviewRuleV2(t.Context(), string(rule.ID), string(domain.NewMutationID()), rule.RowRevision); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-08-04")
	result, err := f.s.ReconcileInvestmentReviewsV2(t.Context())
	if err != nil || result.DayCount != 0 {
		t.Fatalf("archived rule replayed days: %+v %v", result, err)
	}
	var reason string
	if err := f.db.SQL.QueryRow(`SELECT resolved_reason FROM investment_review_episodes WHERE id=?`, episodeID).Scan(&reason); err != nil || reason != "rule_archived" {
		t.Fatalf("archived episode: %q %v", reason, err)
	}
}

func TestReviewAlertPageCursorExpiresOnHandlingChange(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic rule", Enabled: true, Severity: "warning", Spec: syntheticHistoricalReviewSpec()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := f.s.CreateInstrument(t.Context(), InstrumentInput{Name: "Second synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	secondHolding, err := f.s.CreateHolding(t.Context(), HoldingInput{AccountID: f.account.Account.ID.String(), InstrumentID: second.ID.String(), Quantity: "0"})
	if err != nil {
		t.Fatal(err)
	}
	unitCost, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: f.account.Account.HouseholdID, HoldingID: secondHolding.ID, Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &unitCost, EffectiveAt: *f.now}); err != nil {
		t.Fatal(err)
	}
	epoch, episode := string(domain.NewReviewEpisodeID()), string(domain.NewReviewEpisodeID())
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_position_epochs(id,household_id,instrument_id,start_unknown,source_fingerprint) VALUES(?,?,?,1,'synthetic')`, epoch, rule.HouseholdID.String(), second.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_episodes(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,evidence_json,evidence_fingerprint,revision) VALUES(?,?,?,?,1,'2026-08-01','2026-08-01','2026-08-02T00:00:00Z','2026-08-01','2026-08-01','2026-08-01',1,0,0,1,'{"Coverage":[]}','synthetic',1)`, episode, string(rule.ID), second.ID.String(), epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`INSERT INTO investment_review_evaluation_state(rule_id,instrument_id,position_epoch_id,latest_closed_on,policy_state,window_start,window_end,hit_count,false_count,unknown_count,consecutive_count,episode_id,coverage_json,publication_revision) VALUES(?,?,?,'2026-08-01','triggered','2026-08-01','2026-08-01',1,0,0,1,?,'[]',1)`, string(rule.ID), second.ID.String(), epoch, episode); err != nil {
		t.Fatal(err)
	}
	first, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, "", 1)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatalf("first page: %+v %v", first, err)
	}
	next, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, first.NextCursor, 1)
	if err != nil || len(next.Items) != 1 || next.Items[0].InstrumentID == first.Items[0].InstrumentID {
		t.Fatalf("next page: %+v %v", next, err)
	}
	alert := first.Items[0].Rules[0]
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), string(alert.EpisodeID), string(domain.NewMutationID()), alert.EpisodeRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{}, first.NextCursor, 1); err == nil {
		t.Fatal("cursor survived a report handling revision")
	}
}
