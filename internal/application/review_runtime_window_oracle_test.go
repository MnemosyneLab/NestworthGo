package application

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func cloneWindowOracle(t *testing.T, p domain.ReviewPublication) domain.ReviewPublication {
	t.Helper()
	raw, err := jsoncodec.Marshal(p.WindowOracle)
	if err != nil {
		t.Fatal(err)
	}
	p.WindowOracle = nil
	if err := jsoncodec.Unmarshal(raw, &p.WindowOracle); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReviewWindowOracleCompleteOutputs(t *testing.T) {
	f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 3})
	f.day(t, "2026-10-14")
	p := lineagePublication(t, f, r)
	if p.WindowOracle == nil || p.WindowOracle.PreparedVerdict != "window_oracle_matched" || len(p.Rules[0].Days) != 13 || len(p.WindowOracle.Rules[0].Days) != 2 {
		t.Fatal("runtime did not compare a separate bounded plan with the full publication")
	}
	hypothetical := p
	hypothetical.Rules = p.WindowOracle.Rules
	hypothetical.Capture.TemporalFullRecompute = false
	hypothetical.Capture.ReusableDailyEvidence = p.Capture.CompletenessLineage.Window
	// Keep the independent test oracle: it is not implemented by the new
	// production comparator and checks all emitted dates/lifecycle fields.
	if err := compareReviewFullOracle(hypothetical, p, f.s.FXProviderKey()); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"missing", "unknown", "prepare-failed", "missing-rule", "namespace", "missing-new-day", "empty-days", "duplicate-day", "extra-prefix", "input", "coverage", "truth", "policy", "latest-result", "latest-coverage", "first-hit", "trigger", "epoch", "ack", "closure", "excluded", "revision"} {
		t.Run(scenario, func(t *testing.T) {
			candidate := cloneWindowOracle(t, p)
			want := "window_oracle_lifecycle_changed"
			probe := candidate.WindowOracle
			switch scenario {
			case "missing":
				candidate.WindowOracle, want = nil, "window_oracle_missing"
			case "unknown":
				probe.Version, want = "future", "window_oracle_unknown"
			case "prepare-failed":
				probe.Failed, want = true, "window_prepare_failed"
			case "missing-rule":
				probe.Rules, want = nil, "window_oracle_rule_set_changed"
			case "namespace":
				probe.Rules[0].RuleID, want = domain.NewReviewRuleID(), "window_oracle_rule_set_changed"
			case "missing-new-day":
				probe.Rules[0].Days, want = probe.Rules[0].Days[1:], "window_oracle_day_set_changed"
			case "empty-days":
				probe.Rules[0].Days, want = nil, "window_oracle_day_set_changed"
			case "duplicate-day":
				probe.Rules[0].Days[0], want = probe.Rules[0].Days[1], "window_oracle_day_set_changed"
			case "extra-prefix":
				probe.Rules[0].Days, want = append(probe.Rules[0].Days, p.Rules[0].Days[0]), "window_oracle_day_set_changed"
			case "input":
				value := "999"
				probe.Rules[0].Days[0].Input.Quantity, want = &value, "window_oracle_day_changed"
			case "coverage":
				probe.Rules[0].Days[0].CoverageMissing, want = []string{"synthetic"}, "window_oracle_day_changed"
			case "truth":
				probe.Rules[0].Days[0].Result.Truth, want = domain.ReviewFalse, "window_oracle_day_changed"
			case "policy":
				probe.Rules[0].Days[0].PolicyWindow.ConsecutiveCount++
				want = "window_oracle_day_changed"
			case "latest-result":
				probe.Rules[0].LatestResult.Truth = domain.ReviewFalse
			case "latest-coverage":
				probe.Rules[0].LatestCoverage = []string{"synthetic"}
			case "first-hit":
				probe.Rules[0].FirstHitOn = "2026-09-01"
			case "trigger":
				probe.Rules[0].TriggerResult.Truth = domain.ReviewFalse
			case "epoch":
				probe.Rules[0].Epochs[0].StartUnknown = !probe.Rules[0].Epochs[0].StartUnknown
			case "ack":
				at := f.now.Add(-time.Hour)
				probe.Rules[0].CurrentEpisode.AcknowledgedAt = &at
			case "closure":
				probe.Rules[0].ClosedEpisodes = append(probe.Rules[0].ClosedEpisodes, *probe.Rules[0].CurrentEpisode)
			case "excluded":
				probe.Rules[0].TargetExcluded = true
			case "revision":
				probe.Rules[0].RuleRevision++
			}
			// A forged successful preparation stamp must not bypass rechecking.
			if candidate.WindowOracle != nil {
				candidate.WindowOracle.PreparedVerdict = "window_oracle_matched"
			}
			if got := domain.ValidateReviewWindowOracle(candidate); got != want {
				t.Fatalf("got %s want %s", got, want)
			}
		})
	}
	if got := domain.ValidateReviewWindowOracle(p); got != "window_oracle_matched" {
		t.Fatal("comparison mutated original", got)
	}
}

func TestReviewWindowOracleFreshEpisodeAndFullEscape(t *testing.T) {
	f, r, _ := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "consecutive", Count: 12})
	p := lineagePublication(t, f, r)
	if !p.Rules[0].OpenedEpisode || !p.WindowOracle.Rules[0].OpenedEpisode || p.Rules[0].CurrentEpisode.ID == p.WindowOracle.Rules[0].CurrentEpisode.ID || domain.ValidateReviewWindowOracle(p) != "window_oracle_matched" {
		t.Fatal("fresh UUID normalization or runtime comparison failed")
	}
	bad := cloneWindowOracle(t, p)
	bad.WindowOracle.Rules[0].CurrentEpisode.ID = ""
	if got := domain.ValidateReviewWindowOracle(bad); got != "window_oracle_episode_identity_changed" {
		t.Fatal(got)
	}
	for _, mode := range []*sqlite.Repository{sqlite.NewRepository(f.db), r.WithFullReviewRecompute()} {
		capture, err := mode.CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
		if err != nil {
			t.Fatal(err)
		}
		full, err := PrepareReviewPublication(t.Context(), capture, *f.now, f.s.FXProviderKey())
		if err != nil || full.WindowOracle != nil {
			t.Fatal("default/full path enabled oracle shadow", err)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PrepareReviewPublication(ctx, p.Capture, *f.now, f.s.FXProviderKey()); err == nil {
		t.Fatal("cancelled preparation succeeded")
	}
}

// Clone before publication/damage: a clone taken after a conditional full write
// can inherit unrepaired corruption and cannot prove restoration of clean data.
func windowOracleReference(t *testing.T, f seriesZeroFixture) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synthetic-pre-publication.db")
	if _, err := f.db.SQL.Exec(`VACUUM INTO ?`, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db).WithFullReviewRecompute())
	s.fxProviderKey = f.s.FXProviderKey()
	s.setClock(func() time.Time { return *f.now })
	if _, err := s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestReviewWindowOraclePublicationRechecksOutput(t *testing.T) {
	for _, scenario := range []string{"missing", "day", "lifecycle", "honest"} {
		t.Run(scenario, func(t *testing.T) {
			f, r, o := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			clean := windowOracleReference(t, f)
			p := lineagePublication(t, f, r)
			want := "lineage_validated_shadow"
			switch scenario {
			case "missing":
				p.WindowOracle = nil
				want = "window_oracle_missing"
			case "day":
				p.WindowOracle.Rules[0].Days = nil
				want = "window_oracle_day_set_changed"
			case "lifecycle":
				p.WindowOracle.Rules[0].LatestWindow.ConsecutiveCount++
				want = "window_oracle_lifecycle_changed"
			}
			if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
				t.Fatal(err)
			}
			if got := o.last().Verdict; got != want {
				t.Fatalf("got %s want %s", got, want)
			}
			if !reflect.DeepEqual(noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, clean)) {
				t.Fatal("diagnostic result altered full publication")
			}
		})
	}
}

func TestReviewWindowOracleDoesNotAuthenticatePreexistingArchive(t *testing.T) {
	for _, kind := range []string{"historical-json", "frozen-lifecycle"} {
		t.Run(kind, func(t *testing.T) {
			f, r, o := lineageFixture(t, domain.ReviewTriggerPolicy{Kind: "single"})
			clean := windowOracleReference(t, f)
			statement := `UPDATE investment_review_daily_inputs SET evidence_json=json_set(evidence_json,'$.Quantity','999') WHERE on_date='2026-10-01'`
			table := "investment_review_daily_inputs"
			if kind == "frozen-lifecycle" {
				statement = `UPDATE investment_review_episodes SET evidence_json=json_set(evidence_json,'$.TriggerResult.truth','false')`
				table = "investment_review_episodes"
			}
			if _, err := f.db.SQL.Exec(statement); err != nil {
				t.Fatal(err)
			}
			p := lineagePublication(t, f, r)
			if domain.ValidateReviewWindowOracle(p) != "window_oracle_matched" || len(p.Rules[0].Days) != 12 || len(p.Capture.ReusableDailyEvidence) != 0 {
				t.Fatal("full computation/snapshot comparison contract changed")
			}
			if kind == "historical-json" && (p.Rules[0].Days[0].Input.Quantity == nil || *p.Rules[0].Days[0].Input.Quantity == "999") {
				t.Fatal("full calculation consumed corrupt archived JSON")
			}
			if kind == "frozen-lifecycle" && (p.Rules[0].TriggerResult == nil || p.Rules[0].TriggerResult.Truth != domain.ReviewFalse) {
				t.Fatal("fixture did not expose shared frozen-metadata trust")
			}
			if err := r.PublishReviewProjection(t.Context(), p, *f.now); err != nil {
				t.Fatal(err)
			}
			if o.last().Verdict != "lineage_validated_shadow" {
				t.Fatal(o.last())
			}
			actual, reference := noChangeProjectionRows(t, f.db), noChangeProjectionRows(t, clean)
			if reflect.DeepEqual(actual[table], reference[table]) {
				t.Fatal("fixture did not expose the unclosed preexisting integrity gate")
			}
			for name := range actual {
				if name != table && !(kind == "frozen-lifecycle" && name == "investment_review_episode_events") && !reflect.DeepEqual(actual[name], reference[name]) {
					t.Fatalf("unexpected difference outside damaged table: %s", name)
				}
			}
			// Never use a clone-after-publication oracle to assert repair: it would
			// inherit this manually damaged row. Matching plans is not authentication.
		})
	}
}
