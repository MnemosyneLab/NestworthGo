package application

import (
	"database/sql"
	"fmt"

	"path/filepath"
	"reflect"

	"slices"

	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Each correctness scenario starts from this identical, checkpointed synthetic
// snapshot. VACUUM INTO includes committed WAL contents without copying a live
// database file. UUIDs are generated once in the source and shared by clones.
func cloneReviewPerfFixture(t *testing.T, source reviewFullPipelineFixture) reviewFullPipelineFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-clone.db")
	if _, err := source.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := *source.now
	service := NewService(sqlite.NewRepository(db))
	service.setClock(func() time.Time { return now })
	return reviewFullPipelineFixture{service: service, db: db, now: &now, householdID: source.householdID, dbPath: path, start: source.start, instruments: source.instruments}
}

// Full recomputation is captured independently from the same authoritative DB,
// retaining handling history but supplying no persisted daily projection inputs
// to the independent computation.
// Compare every emitted day's exact typed financial evidence and the complete
// lifecycle/coverage/window summary. Only freshly generated episode IDs are
// mapped; persisted IDs and acknowledgement times must remain identical.
func assertReviewFullOracle(t *testing.T, f reviewFullPipelineFixture, candidate domain.ReviewPublication) {
	t.Helper()
	capture, err := fullReviewOracle(sqlite.NewRepository(f.db)).CaptureReviewProjection(t.Context(), f.householdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(capture.ReusableDailyEvidence) != 0 {
		t.Fatal("full oracle reused evidence")
	}
	full, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.service.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := compareReviewFullOracle(candidate, full, f.service.FXProviderKey()); err != nil {
		t.Fatal(err)
	}
}

// Compare against independently rebuilt inputs. Derive the emitted suffix from
// authoritative full dates and the complete captured anchor window, never from
// whichever days the candidate happened to emit or the incremental planner.
func compareReviewFullOracle(candidate, full domain.ReviewPublication, providerKey string) error {
	if len(candidate.Rules) != len(full.Rules) {
		return fmt.Errorf("oracle rule count differs")
	}
	fullDays := map[domain.ReviewDailyEvidenceKey]domain.ReviewPublicationDay{}
	for _, rule := range full.Rules {
		for _, day := range rule.Days {
			key := domain.ReviewDailyEvidenceKey{RuleID: rule.RuleID, ConditionVersion: rule.ConditionVersion, InstrumentID: rule.InstrumentID, TimezoneVersion: rule.TimezoneVersion, On: day.On}
			if _, exists := fullDays[key]; exists {
				return fmt.Errorf("oracle duplicate full day: %+v", key)
			}
			fullDays[key] = day
		}
	}
	omitted := map[domain.ReviewDailyEvidenceKey]bool{}
	if candidate.Capture.Suffix != nil {
		for _, p := range candidate.Capture.Suffix.Prefix {
			day, ok := fullDays[p.Key]
			if !ok || !reflect.DeepEqual(p.Evidence.Input, day.Input) || !reflect.DeepEqual(p.Evidence.CoverageMissing, day.CoverageMissing) || !reflect.DeepEqual(p.Evidence.Result, day.Result) || p.Evidence.InputFingerprint != day.InputFingerprint {
				return fmt.Errorf("suffix evidence differs from independent full day: %+v", p.Key)
			}
			if p.OutputAttested {
				if omitted[p.Key] {
					return fmt.Errorf("duplicate omitted oracle key")
				}
				omitted[p.Key] = true
			}
		}
	}
	for key, cached := range candidate.Capture.ReusableDailyEvidence {
		day, exists := fullDays[key]
		if !exists {
			return fmt.Errorf("oracle cached key has no full day: %+v", key)
		}
		if !reflect.DeepEqual(cached.Input, day.Input) || !reflect.DeepEqual(cached.CoverageMissing, day.CoverageMissing) || cached.InputFingerprint != day.InputFingerprint || (cached.HasResult && !reflect.DeepEqual(cached.Result, day.Result)) {
			return fmt.Errorf("cached evidence differs from full recompute: %+v", key)
		}
	}
	for i, got := range candidate.Rules {
		want := full.Rules[i]
		expected, err := expectedReviewOracleDays(candidate.Capture.ReusableDailyEvidence, full.Capture, want, providerKey)
		if err != nil {
			return err
		}
		if len(omitted) > 0 {
			expected = nil
			for _, d := range want.Days {
				key := domain.ReviewDailyEvidenceKey{RuleID: want.RuleID, ConditionVersion: want.ConditionVersion, InstrumentID: want.InstrumentID, TimezoneVersion: want.TimezoneVersion, On: d.On}
				if !omitted[key] {
					expected = append(expected, d)
				}
			}
		}
		seen := map[string]bool{}
		for _, day := range got.Days {
			if seen[day.On] {
				return fmt.Errorf("oracle duplicate emitted day rule=%s instrument=%s on=%s", got.RuleID, got.InstrumentID, day.On)
			}
			seen[day.On] = true
		}
		if len(got.Days) != len(expected) {
			return fmt.Errorf("oracle emitted day count differs rule=%s instrument=%s: got=%d want=%d", got.RuleID, got.InstrumentID, len(got.Days), len(expected))
		}
		for index, day := range got.Days {
			if day.On != expected[index].On {
				return fmt.Errorf("oracle emitted date differs rule=%s instrument=%s: got=%s want=%s", got.RuleID, got.InstrumentID, day.On, expected[index].On)
			}
			if !reflect.DeepEqual(day, expected[index]) {
				return fmt.Errorf("oracle day differs rule=%s instrument=%s on=%s\ngot=%+v\nwant=%+v", got.RuleID, got.InstrumentID, day.On, day, expected[index])
			}
		}
		got.Days, want.Days = nil, nil
		if got.OpenedEpisode && want.OpenedEpisode && got.CurrentEpisode != nil && want.CurrentEpisode != nil {
			copied := *got.CurrentEpisode
			copied.ID = want.CurrentEpisode.ID
			got.CurrentEpisode = &copied
		}
		if !reflect.DeepEqual(got, want) {
			return fmt.Errorf("oracle lifecycle differs rule=%s instrument=%s\ngot=%+v\nwant=%+v", got.RuleID, got.InstrumentID, got, want)
		}
	}
	return nil
}

func expectedReviewOracleDays(cached map[domain.ReviewDailyEvidenceKey]domain.ReviewReusableDailyEvidence, authoritative domain.ReviewProjectionCapture, rule domain.ReviewRulePublication, providerKey string) ([]domain.ReviewPublicationDay, error) {
	// Without a complete, source-equal projection every full-oracle date is due.
	if len(rule.Days) == 0 || authoritative.Control.ProjectionStatus != "complete" || authoritative.Control.PublishedFingerprint != authoritative.HistoricalFingerprint {
		return rule.Days, nil
	}
	width := 0
	for _, spec := range authoritative.Rules {
		if spec.ID != rule.RuleID {
			continue
		}
		switch spec.Spec.TriggerPolicy.Kind {
		case "consecutive":
			width = spec.Spec.TriggerPolicy.Count
		case "rolling":
			width = spec.Spec.TriggerPolicy.WindowDays
		default:
			width = 1
		}
		break
	}
	if width < 1 {
		return nil, fmt.Errorf("oracle missing/invalid policy for rule %s", rule.RuleID)
	}
	for _, episode := range authoritative.OpenEpisodes {
		if episode.RuleID == rule.RuleID && episode.InstrumentID == rule.InstrumentID && episode.TriggerWindow.WindowStart == "" {
			return rule.Days, nil
		}
	}
	anchors := map[string]bool{}
	for key, item := range cached {
		if key.RuleID != rule.RuleID || key.ConditionVersion != rule.ConditionVersion || key.InstrumentID != rule.InstrumentID || key.TimezoneVersion != rule.TimezoneVersion {
			continue
		}
		if !item.HasResult || item.Input.FXProviderKey != providerKey {
			return rule.Days, nil
		}
		anchors[key.On] = true
	}
	if len(anchors) == 0 {
		return rule.Days, nil
	}
	last := -1
	for index, day := range rule.Days {
		if anchors[day.On] {
			last = index
		}
	}
	first := last - width + 1
	if first < 0 {
		first = 0
	}
	// A hole, truncated window, or extra anchor forces the full reconstruction,
	// matching the contract's contiguous date coverage gate independently.
	if last < 0 || len(anchors) != last-first+1 {
		return rule.Days, nil
	}
	for _, day := range rule.Days[first : last+1] {
		if !anchors[day.On] {
			return rule.Days, nil
		}
	}
	return rule.Days[last+1:], nil
}

func TestReviewPerformanceFullOracle(t *testing.T) {
	checkReviewEpochReplayOracle(t, reviewPerfConfig{Accounts: 2, Instruments: 3, Holdings: 4, Rules: 4, HistoryDays: 10, Quotes: 30, MixedActivities: 8}, 0)
}

// Small correctness scenarios compare supported C2 publication with a separate
// authoritative full replay. No timing/RSS, rollout arms or sample controls.
func checkReviewEpochReplayOracle(t *testing.T, config reviewPerfConfig, cycles int) {
	t.Helper()
	source := newReviewPerfFixture(t, config)
	if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episode string
	var revision int
	err := source.db.SQL.QueryRow(`SELECT id,revision FROM investment_review_episodes WHERE resolved_at IS NULL ORDER BY id LIMIT 1`).Scan(&episode, &revision)
	if err == nil {
		if _, err := source.service.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), revision); err != nil {
			t.Fatal(err)
		}
	} else if err != sql.ErrNoRows {
		t.Fatal(err)
	}
	if cycles > 0 {
		seedReviewPerfEpochCycles(t, source, cycles)
	}
	if _, err := source.service.SetReviewSuppressionV2(t.Context(), SetReviewSuppressionV2Input{SuppressionID: string(domain.NewReviewSuppressionID()), MutationID: string(domain.NewMutationID()), InstrumentID: source.instruments[0].ID.String(), Mode: "permanent"}); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"no-op", "clock-only", "append", "correction"} {
		t.Run(scenario, func(t *testing.T) {
			fixture := cloneReviewPerfFixture(t, source)
			switch scenario {
			case "clock-only":
				*fixture.now = fixture.now.AddDate(0, 0, 1)
			case "append":
				*fixture.now = fixture.now.AddDate(0, 0, 1)
				if err := appendReviewPerfDailyMarketData(t.Context(), fixture, fixture.now.AddDate(0, 0, -1)); err != nil {
					t.Fatal(err)
				}
			case "correction":
				if err := correctReviewPerfHistoricalQuote(t.Context(), fixture, 0); err != nil {
					t.Fatal(err)
				}
			}
			capture, err := sqlite.NewRepository(fixture.db).CaptureReviewProjection(t.Context(), fixture.householdID, *fixture.now)
			if err != nil {
				t.Fatal(err)
			}
			publication, err := PrepareReviewPublication(t.Context(), capture, *fixture.now, fixture.service.FXProviderKey())
			if err != nil {
				t.Fatal(err)
			}
			assertReviewFullOracle(t, fixture, publication)
			var published [2]string
			for path := range published {
				run := cloneReviewPerfFixture(t, fixture)
				var repo reviewTestProjectionRepository = sqlite.NewRepository(run.db)
				if path == 0 {
					repo = fullReviewOracle(repo)
				}
				service := NewService(repo)
				service.setClock(func() time.Time { return *run.now })
				if _, err := service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
					t.Fatal(err)
				}
				published[path] = reviewPerfPublishedRows(t, run, source)
			}
			if published[0] != published[1] {
				t.Fatal("C2 and full durable projection/lifecycle differ")
			}
		})
	}
}

// Include every durable projection field. Map only newly allocated episode and
// event IDs to their logical identities; existing source IDs remain exact.
func reviewPerfPublishedRows(t *testing.T, f, source reviewFullPipelineFixture) string {
	t.Helper()
	known := map[string]bool{}
	rows, err := source.db.SQL.Query(`SELECT id FROM investment_review_episodes`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		known[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	knownEvents := map[string]bool{}
	rows, err = source.db.SQL.Query(`SELECT id FROM investment_review_episode_events`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		knownEvents[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	aliases := map[string]string{}
	rows, err = f.db.SQL.Query(`SELECT id,rule_id,instrument_id,position_epoch_id,triggered_on FROM investment_review_episodes`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id, rule, instrument, epoch, on string
		if err := rows.Scan(&id, &rule, &instrument, &epoch, &on); err != nil {
			t.Fatal(err)
		}
		if !known[id] {
			aliases[id] = "new-episode:" + rule + ":" + instrument + ":" + epoch + ":" + on
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	rows.Close()
	tables := map[string][]string{}
	for _, table := range []string{"investment_review_control", "investment_review_daily_inputs", "investment_review_daily_results", "investment_review_position_epochs", "investment_review_episodes", "investment_review_evaluation_state", "investment_review_episode_events", "review_temporal_checkpoints"} {
		rows, err := f.db.SQL.Query(`SELECT * FROM ` + table)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			pointers := make([]any, len(columns))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			for i, value := range values {
				if table == "review_temporal_checkpoints" && columns[i] == "certificate_json" {
					var certificate domain.ReviewTemporalCertificate
					if err := jsoncodec.Unmarshal([]byte(value.(string)), &certificate); err != nil {
						t.Fatal(err)
					}
					certificate.ClockEpoch = "independent-synthetic-session"
					raw, err := jsoncodec.Marshal(certificate)
					if err != nil {
						t.Fatal(err)
					}
					value = string(raw)
				}
				if text, ok := value.(string); ok {
					for id, alias := range aliases {
						text = strings.ReplaceAll(text, id, alias)
					}
					values[i] = text
				}
			}
			// Existing audit identities remain exact. Only a newly allocated event
			// UUID may differ across independently published clones; map it through
			// its normalized unique event key while retaining every other field.
			if table == "investment_review_episode_events" && !knownEvents[values[0].(string)] {
				for index, column := range columns {
					if column == "event_key" {
						values[0] = "new-event:" + values[index].(string)
					}
				}
			}
			encoded, err := jsoncodec.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			tables[table] = append(tables[table], string(encoded))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
		slices.Sort(tables[table])
	}
	encoded, err := jsoncodec.Marshal(tables)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestReviewDurableOraclePreservesExistingEventIdentity(t *testing.T) {
	source := newReviewPerfFixture(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Rules: 1, HistoryDays: 2, Quotes: 2})
	if _, err := source.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	clone := cloneReviewPerfFixture(t, source)
	before := reviewPerfPublishedRows(t, clone, source)
	result, err := clone.db.SQL.Exec(`UPDATE investment_review_episode_events SET id=? WHERE id=(SELECT id FROM investment_review_episode_events ORDER BY id LIMIT 1)`, string(domain.NewMutationID()))
	if err != nil {
		t.Fatal(err)
	}
	if count, err := result.RowsAffected(); err != nil || count != 1 {
		t.Fatalf("missing synthetic event: rows=%d err=%v", count, err)
	}
	if before == reviewPerfPublishedRows(t, clone, source) {
		t.Fatal("oracle ignored replacement of an existing event identity")
	}
}
