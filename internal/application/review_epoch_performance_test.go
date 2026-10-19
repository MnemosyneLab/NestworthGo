package application

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Real command-path cycles, rather than fabricated evaluation-state rows. Both
// the flat-position boundary and re-entry are published and independently
// checked. Intraday cycles keep closed daily votes unchanged.
func seedReviewPerfEpochCycles(t *testing.T, source reviewFullPipelineFixture, cycles int) {
	t.Helper()
	var holding domain.HoldingID
	var quantityText string
	if err := source.db.SQL.QueryRow(`SELECT id,quantity FROM holdings WHERE instrument_id=? AND archived_at IS NULL ORDER BY id LIMIT 1`, source.instruments[0].ID.String()).Scan(&holding, &quantityText); err != nil {
		t.Fatal(err)
	}
	quantity := mustQuantity(t, quantityText)
	cost, err := domain.ParseUnitPrice("10")
	if err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < cycles; cycle++ {
		for _, added := range []bool{false, true} {
			*source.now = source.now.Add(time.Second)
			if _, err := source.service.RecordChange(t.Context(), domain.PositionAdjustmentInput{HouseholdID: source.householdID, HoldingID: holding, Quantity: quantity, Added: added, UnitCost: &cost, EffectiveAt: *source.now}); err != nil {
				t.Fatal(err)
			}
			var captures [2]domain.ReviewProjectionCapture
			for path, repo := range []*sqlite.Repository{sqlite.NewRepository(source.db).WithReviewWindowBaseline(), sqlite.NewRepository(source.db).WithBoundedReviewWindow()} {
				capture, err := repo.CaptureReviewProjection(t.Context(), source.householdID, *source.now)
				if err != nil {
					t.Fatal(err)
				}
				captures[path] = capture
				publication, err := PrepareReviewPublication(t.Context(), capture, *source.now, source.service.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				assertReviewFullOracle(t, source, publication)
			}
			if !reflect.DeepEqual(captures[0], captures[1]) {
				t.Fatalf("epoch cycle=%d added=%t capture differs", cycle, added)
			}
			var durable [2]string
			for path := range durable {
				t.Run("epoch-publish", func(t *testing.T) {
					run := cloneReviewPerfFixture(t, source)
					repo := sqlite.NewRepository(run.db).WithBoundedReviewWindow()
					if path == 0 {
						repo = repo.WithReviewWindowBaseline()
					}
					service := NewService(repo)
					service.setClock(func() time.Time { return *run.now })
					if _, err := service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
						t.Fatal(err)
					}
					durable[path] = reviewPerfPublishedRows(t, run, source)
				})
			}
			if durable[0] != durable[1] {
				t.Fatalf("epoch cycle=%d added=%t durable lifecycle differs", cycle, added)
			}
			if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}
	var epochs, states int
	if err := source.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_position_epochs WHERE instrument_id=?`, source.instruments[0].ID.String()).Scan(&epochs); err != nil {
		t.Fatal(err)
	}
	if err := source.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_evaluation_state WHERE instrument_id=?`, source.instruments[0].ID.String()).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if epochs != cycles+1 || states != len(capturesRules(t, source))*(cycles+1) {
		t.Fatalf("full exits/re-entry not materialized: cycles=%d epochs=%d states=%d", cycles, epochs, states)
	}
	t.Logf("real epoch history cycles=%d positionEpochs=%d evaluationStates=%d; both exit and re-entry checked against full oracle and durable baseline", cycles, epochs, states)
}

func capturesRules(t *testing.T, source reviewFullPipelineFixture) []domain.ReviewRule {
	t.Helper()
	capture, err := sqlite.NewRepository(source.db).WithFullReviewRecompute().CaptureReviewProjection(t.Context(), source.householdID, *source.now)
	if err != nil {
		t.Fatal(err)
	}
	return capture.Rules
}

func TestReviewRepeatedEpochWindowOracle(t *testing.T) {
	// One exit/re-entry cycle hits the same oracle and durable-lifecycle branches
	// as repeating the pair. Eight cycles stay in the weekly perf workflow.
	runReviewPerfEpochPairs(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 10, Quotes: 10}, 1, 1)
}

func TestReviewRepeatedEpochWindowOracleScale(t *testing.T) {
	if os.Getenv("NESTWORTH_PERF") != "1" {
		t.Skip("eight epoch cycles run in the weekly perf workflow (NESTWORTH_PERF=1)")
	}
	runReviewPerfEpochPairs(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Holdings: 1, Rules: 4, HistoryDays: 10, Quotes: 10}, 1, 8)
}

// A same-day re-entry can trigger on the last closed day before its opening
// civil date. A later full replay must keep that established interval and ack.
func TestReviewReentryEpisodeFullReplayAcrossMidnight(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	spec := syntheticHistoricalReviewSpec()
	// All six votes are required, so a corrected false vote on October 6
	// independently proves recovery (five T plus one F), without an oracle.
	spec.TriggerPolicy = domain.ReviewTriggerPolicy{Kind: "rolling", WindowDays: 6, MinHits: 6}
	if _, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic re-entry interval", Enabled: true, Severity: "warning", Spec: spec}); err != nil {
		t.Fatal(err)
	}
	f.day(t, "2026-10-07")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var oldID string
	if err := f.db.SQL.QueryRow(`SELECT id FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&oldID); err != nil {
		t.Fatal(err)
	}
	f.quantity(t, false)
	*f.now = f.now.Add(time.Second)
	f.quantity(t, true)
	detectedAt := *f.now
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var id string
	var revision int
	if err := f.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&id, &revision); err != nil {
		t.Fatal(err)
	}
	if id == oldID || revision != 1 {
		t.Fatalf("re-entry did not open a fresh episode: id=%s old=%s revision=%d", id, oldID, revision)
	}
	*f.now = f.now.Add(2*time.Minute + 345*time.Nanosecond)
	acknowledgedAt := *f.now
	ack, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), id, string(domain.NewMutationID()), revision)
	if err != nil || ack.Revision != 2 || !ack.AcknowledgedAt.Equal(acknowledgedAt) {
		t.Fatalf("acknowledgement: %+v %v", ack, err)
	}
	type episodeRecord struct {
		id, triggered, detected, acknowledged, latest, resolved, reason string
		revision                                                        int
	}
	readEpisode := func(t *testing.T, db *sqlite.DB) episodeRecord {
		t.Helper()
		var row episodeRecord
		if err := db.SQL.QueryRow(`SELECT id,triggered_on,first_detected_at,acknowledged_at,latest_closed_on,COALESCE(resolved_at,''),COALESCE(resolved_reason,''),revision FROM investment_review_episodes WHERE id=?`, id).Scan(&row.id, &row.triggered, &row.detected, &row.acknowledged, &row.latest, &row.resolved, &row.reason, &row.revision); err != nil {
			t.Fatal(err)
		}
		return row
	}
	type eventRecord struct{ id, episode, key, kind, payload, created string }
	readEvents := func(t *testing.T, db *sqlite.DB) []eventRecord {
		t.Helper()
		rows, err := db.SQL.Query(`SELECT id,episode_id,event_key,kind,payload_json,created_at FROM investment_review_episode_events WHERE episode_id=? ORDER BY event_key`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var events []eventRecord
		for rows.Next() {
			var e eventRecord
			if err := rows.Scan(&e.id, &e.episode, &e.key, &e.kind, &e.payload, &e.created); err != nil {
				t.Fatal(err)
			}
			events = append(events, e)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return events
	}
	want := episodeRecord{id: id, triggered: "2026-10-06", detected: detectedAt.UTC().Format(time.RFC3339Nano), acknowledged: acknowledgedAt.UTC().Format(time.RFC3339Nano), latest: "2026-10-06", revision: 2}
	if got := readEpisode(t, f.db); got != want {
		t.Fatalf("stored re-entry/ack: got=%+v want=%+v", got, want)
	}
	wantEvents := readEvents(t, f.db)
	if len(wantEvents) != 2 || wantEvents[0].kind != "acknowledged" || wantEvents[0].key != "ack/"+id || wantEvents[0].created != want.acknowledged || wantEvents[1].kind != "opened" || wantEvents[1].key != "open/"+id || wantEvents[1].created != want.detected {
		t.Fatalf("unexpected initial audit events: %+v", wantEvents)
	}
	f.day(t, "2026-10-08")
	source := reviewFullPipelineFixture{service: f.s, db: f.db, now: f.now, householdID: f.account.Account.HouseholdID, instruments: []domain.Instrument{f.instrument}}
	for _, mode := range []string{"full", "baseline", "bounded"} {
		t.Run(mode, func(t *testing.T) {
			run := cloneReviewPerfFixture(t, source)
			repo := sqlite.NewRepository(run.db)
			switch mode {
			case "full":
				repo = repo.WithFullReviewRecompute()
			case "baseline":
				repo = repo.WithReviewWindowBaseline()
			case "bounded":
				repo = repo.WithBoundedReviewWindow()
			}
			prepare := func() domain.ReviewPublication {
				t.Helper()
				capture, err := repo.CaptureReviewProjection(t.Context(), run.householdID, *run.now)
				if err != nil {
					t.Fatal(err)
				}
				p, err := PrepareReviewPublication(t.Context(), capture, *run.now, run.service.FXProviderKey())
				if err != nil {
					t.Fatal(err)
				}
				if len(p.Rules) != 1 {
					t.Fatalf("missing rule: %+v", p.Rules)
				}
				return p
			}
			p := prepare()
			current := p.Rules[0].CurrentEpisode
			if current == nil || current.ID != id || current.TriggeredOn != "2026-10-06" || !current.FirstDetectedAt.Equal(detectedAt) || current.AcknowledgedAt == nil || !current.AcknowledgedAt.Equal(acknowledgedAt) || p.Rules[0].OpenedEpisode || len(p.Rules[0].ClosedEpisodes) != 0 {
				t.Fatalf("midnight changed re-entry identity/times: %+v", p.Rules[0])
			}
			if err := repo.PublishReviewProjection(t.Context(), p, *run.now); err != nil {
				t.Fatal(err)
			}
			midnightWant := want
			midnightWant.latest = "2026-10-07"
			midnightWant.revision = 3
			if got := readEpisode(t, run.db); got != midnightWant {
				t.Fatalf("midnight publication: got=%+v want=%+v", got, midnightWant)
			}
			if got := readEvents(t, run.db); !reflect.DeepEqual(got, wantEvents) {
				t.Fatalf("midnight changed audit events: got=%+v want=%+v", got, wantEvents)
			}
			// Repeating publication must not bump revision or rewrite/add audit events.
			if err := repo.PublishReviewProjection(t.Context(), prepare(), *run.now); err != nil {
				t.Fatal(err)
			}
			if got := readEpisode(t, run.db); got != midnightWant {
				t.Fatalf("repeat changed episode: %+v", got)
			}
			if got := readEvents(t, run.db); !reflect.DeepEqual(got, wantEvents) {
				t.Fatalf("repeat changed events: %+v", got)
			}
			cost, err := domain.ParseUnitPrice("300")
			if err != nil {
				t.Fatal(err)
			}
			// Record the correction after re-entry, effective on the old trigger
			// date before this epoch opened. October 6 becomes false; re-entry
			// resets cost to 100 on October 7, whose predicate remains true.
			// The six-slot policy is nevertheless false (five T plus one F).
			if _, err := run.service.RecordChange(t.Context(), domain.PositionCostAdjustmentInput{HouseholdID: run.householdID, HoldingID: f.holding.ID, UnitCost: cost, EffectiveAt: time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)}); err != nil {
				t.Fatal(err)
			}
			corrected := prepare()
			item := corrected.Rules[0]
			if item.LatestResult == nil || item.LatestResult.Truth != domain.ReviewTrue || item.LatestWindow.Truth != domain.ReviewFalse || item.LatestWindow.HitCount != 5 || item.LatestWindow.FalseCount != 1 || item.LatestWindow.UnknownCount != 0 || item.CurrentEpisode != nil || item.OpenedEpisode || len(item.ClosedEpisodes) != 1 {
				t.Fatalf("false correction must close the episode: %+v", item)
			}
			correctedTrigger := false
			for _, day := range item.Days {
				if day.On == "2026-10-06" {
					correctedTrigger = day.Result.Truth == domain.ReviewFalse
				}
			}
			if !correctedTrigger {
				t.Fatal("the historical trigger vote was not corrected to false")
			}
			closed := item.ClosedEpisodes[0]
			if closed.ID != id || closed.TriggeredOn != "2026-10-06" || !closed.FirstDetectedAt.Equal(detectedAt) || closed.AcknowledgedAt == nil || !closed.AcknowledgedAt.Equal(acknowledgedAt) || closed.ResolvedAt == nil || !closed.ResolvedAt.Equal(*run.now) || closed.ResolvedReason != "recovered" {
				t.Fatalf("closure changed original identity/times: %+v", closed)
			}
			if err := repo.PublishReviewProjection(t.Context(), corrected, *run.now); err != nil {
				t.Fatal(err)
			}
			closedWant := midnightWant
			closedWant.resolved = run.now.UTC().Format(time.RFC3339Nano)
			closedWant.reason = "recovered"
			closedWant.revision = 4
			if got := readEpisode(t, run.db); got != closedWant {
				t.Fatalf("stored closure: got=%+v want=%+v", got, closedWant)
			}
			gotEvents := readEvents(t, run.db)
			if len(gotEvents) != 3 || gotEvents[1].kind != "resolved" || gotEvents[1].key != "close/"+id || gotEvents[1].created != closedWant.resolved || !reflect.DeepEqual([]eventRecord{gotEvents[0], gotEvents[2]}, wantEvents) {
				t.Fatalf("closure must append one event and preserve history: %+v", gotEvents)
			}
			var resolution domain.ReviewEpisodeIdentity
			if err := json.Unmarshal([]byte(gotEvents[1].payload), &resolution); err != nil {
				t.Fatal(err)
			}
			if resolution.ID != id || resolution.TriggeredOn != "2026-10-06" || !resolution.FirstDetectedAt.Equal(detectedAt) || resolution.AcknowledgedAt == nil || !resolution.AcknowledgedAt.Equal(acknowledgedAt) || resolution.ResolvedAt == nil || !resolution.ResolvedAt.Equal(*run.now) || resolution.ResolvedReason != "recovered" {
				t.Fatalf("resolution event lost exact identity/times: %+v", resolution)
			}
			if err := repo.PublishReviewProjection(t.Context(), prepare(), *run.now); err != nil {
				t.Fatal(err)
			}
			if got := readEpisode(t, run.db); got != closedWant {
				t.Fatalf("repeat closure changed episode: %+v", got)
			}
			if got := readEvents(t, run.db); !reflect.DeepEqual(got, gotEvents) {
				t.Fatalf("repeat closure changed audit history: %+v", got)
			}
			var open int
			if err := run.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes WHERE resolved_at IS NULL`).Scan(&open); err != nil || open != 0 {
				t.Fatalf("false correction reopened an episode: count=%d err=%v", open, err)
			}
		})
	}
}
