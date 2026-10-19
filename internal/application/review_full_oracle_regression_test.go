package application

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func reviewOracleRegressionPublications(t *testing.T, f reviewFullPipelineFixture) (domain.ReviewPublication, domain.ReviewPublication) {
	t.Helper()
	repository := sqlite.NewRepository(f.db)
	candidateCapture, err := repository.WithBoundedReviewWindow().CaptureReviewProjection(t.Context(), f.householdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	fullCapture, err := repository.WithFullReviewRecompute().CaptureReviewProjection(t.Context(), f.householdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := PrepareReviewPublication(t.Context(), candidateCapture, *f.now, f.service.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	full, err := PrepareReviewPublication(t.Context(), fullCapture, *f.now, f.service.FXProviderKey())
	if err != nil {
		t.Fatal(err)
	}
	if err := compareReviewFullOracle(candidate, full, f.service.FXProviderKey()); err != nil {
		t.Fatalf("legitimate publication rejected: %v", err)
	}
	return candidate, full
}

func cloneReviewOraclePublication(source domain.ReviewPublication) domain.ReviewPublication {
	source.Rules = slices.Clone(source.Rules)
	for i := range source.Rules {
		source.Rules[i].Days = slices.Clone(source.Rules[i].Days)
	}
	source.Capture.ReusableDailyEvidence = maps.Clone(source.Capture.ReusableDailyEvidence)
	return source
}

func TestReviewFullOracleRejectsDaySetMutations(t *testing.T) {
	f := newReviewPerfFixture(t, reviewPerfConfig{Accounts: 1, Instruments: 1, Rules: 2, HistoryDays: 10, Quotes: 10})
	// The second rule uses a three-day consecutive window, allowing the gap
	// repair case below to distinguish stored-slot count from date coverage.
	fullRebuild, full := reviewOracleRegressionPublications(t, f)
	if len(fullRebuild.Rules[0].Days) != 10 {
		t.Fatal("fixture did not require a full rebuild")
	}
	checkMutation := func(t *testing.T, candidate, oracle domain.ReviewPublication, mutate func(*domain.ReviewPublication), message string) {
		t.Helper()
		corrupted := cloneReviewOraclePublication(candidate)
		mutate(&corrupted)
		err := compareReviewFullOracle(corrupted, oracle, f.service.FXProviderKey())
		if err == nil || !strings.Contains(err.Error(), message) {
			t.Fatalf("mutation escaped expected %q rejection: %v", message, err)
		}
		t.Logf("rejected mutation: %v", err)
	}
	t.Run("full-rebuild-missing-middle-day", func(t *testing.T) {
		checkMutation(t, fullRebuild, full, func(p *domain.ReviewPublication) { p.Rules[0].Days = slices.Delete(p.Rules[0].Days, 4, 5) }, "emitted day count differs")
	})
	t.Run("full-rebuild-empty-days", func(t *testing.T) {
		checkMutation(t, fullRebuild, full, func(p *domain.ReviewPublication) { p.Rules[0].Days = nil }, "emitted day count differs")
	})
	t.Run("full-rebuild-duplicate-replaces-required-day", func(t *testing.T) {
		checkMutation(t, fullRebuild, full, func(p *domain.ReviewPublication) { p.Rules[0].Days[4] = p.Rules[0].Days[5] }, "duplicate emitted day")
	})
	if _, err := f.service.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	noOp, noOpFull := reviewOracleRegressionPublications(t, f)
	for _, rule := range noOp.Rules {
		if len(rule.Days) != 0 {
			t.Fatal("same-day continuation should emit no days")
		}
	}
	t.Run("no-op-unexpected-prefix", func(t *testing.T) {
		checkMutation(t, noOp, noOpFull, func(p *domain.ReviewPublication) {
			p.Rules[0].Days = []domain.ReviewPublicationDay{noOpFull.Rules[0].Days[0]}
		}, "emitted day count differs")
	})
	*f.now = f.now.AddDate(0, 0, 2)
	suffix, suffixFull := reviewOracleRegressionPublications(t, f)
	for _, rule := range suffix.Rules {
		if len(rule.Days) != 2 {
			t.Fatalf("two-day continuation emitted %d days", len(rule.Days))
		}
	}
	t.Run("incremental-missing-first-new-day", func(t *testing.T) {
		// Keep the final day and every summary unchanged: this mutation was
		// accepted by the original one-way candidate.Days comparison.
		checkMutation(t, suffix, suffixFull, func(p *domain.ReviewPublication) { p.Rules[0].Days = p.Rules[0].Days[1:] }, "emitted day count differs")
	})
	t.Run("incremental-empty-days", func(t *testing.T) {
		checkMutation(t, suffix, suffixFull, func(p *domain.ReviewPublication) { p.Rules[0].Days = nil }, "emitted day count differs")
	})
	t.Run("incremental-duplicate-replaces-new-day", func(t *testing.T) {
		checkMutation(t, suffix, suffixFull, func(p *domain.ReviewPublication) { p.Rules[0].Days[0] = p.Rules[0].Days[1] }, "duplicate emitted day")
	})
	t.Run("incremental-wrong-date-same-count", func(t *testing.T) {
		checkMutation(t, suffix, suffixFull, func(p *domain.ReviewPublication) { p.Rules[0].Days[0] = suffixFull.Rules[0].Days[0] }, "emitted date differs")
	})
	for _, kind := range []string{"date", "namespace"} {
		t.Run("cached-key-without-full-day-"+kind, func(t *testing.T) {
			checkMutation(t, suffix, suffixFull, func(p *domain.ReviewPublication) {
				for key, item := range p.Capture.ReusableDailyEvidence {
					if kind == "date" {
						key.On = "2099-01-01"
						item.Input.On = key.On
						item.Result.On = key.On
					} else {
						key.ConditionVersion++
					}
					p.Capture.ReusableDailyEvidence[key] = item
					break
				}
			}, "cached key has no full day")
		})
	}
	// Retain W stored slots but introduce a hole in the second rule's window.
	// Both planner and independent expected-set oracle must require all 12 days.
	if _, err := f.db.SQL.Exec(`DELETE FROM investment_review_daily_inputs WHERE input_policy=? AND on_date=?`, "rule:"+string(suffix.Rules[1].RuleID)+"/v:1", f.start.AddDate(0, 0, 8).Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	repaired, repairedFull := reviewOracleRegressionPublications(t, f)
	if len(repaired.Rules[1].Days) != 12 {
		t.Fatalf("gap repair emitted %d days, want full 12", len(repaired.Rules[1].Days))
	}
	t.Run("gap-repair-missing-required-day", func(t *testing.T) {
		checkMutation(t, repaired, repairedFull, func(p *domain.ReviewPublication) { p.Rules[1].Days = slices.Delete(p.Rules[1].Days, 4, 5) }, "emitted day count differs")
	})
}
