package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func importReviewDailyClose(t *testing.T, f seriesZeroFixture, on, value string) string {
	t.Helper()
	result, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		Kind: "close", InstrumentID: f.instrument.ID.String(), Currency: f.instrument.QuoteCurrency.String(),
		Date: on, Timezone: f.location.String(), Value: value, SourceTitle: "Synthetic review regression close",
	}}})
	if err != nil || len(result.QuoteIDs) != 1 {
		t.Fatalf("import synthetic close: %+v %v", result, err)
	}
	return result.QuoteIDs[0]
}

func importReviewDailyFX(t *testing.T, f seriesZeroFixture, on, rate string) string {
	t.Helper()
	result, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{{
		Kind: "daily_reference", BaseCurrency: "USD", QuoteCurrency: "CNY", Date: on,
		Timezone: f.location.String(), Value: rate, SourceTitle: "Synthetic review regression FX",
	}}})
	if err != nil || len(result.QuoteIDs) != 1 {
		t.Fatalf("import synthetic FX reference: %+v %v", result, err)
	}
	return result.QuoteIDs[0]
}

func saveSyntheticReviewRule(t *testing.T, f seriesZeroFixture, name string, spec domain.ReviewRuleSpec) domain.ReviewRule {
	t.Helper()
	rule, err := f.s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{
		RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()),
		Name: name, Enabled: true, Severity: "warning", Spec: spec,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

func reviewTruthAt(t *testing.T, f seriesZeroFixture, rule domain.ReviewRule, on string) string {
	t.Helper()
	var truth string
	if err := f.db.SQL.QueryRow(`SELECT predicate_truth FROM investment_review_daily_results
		WHERE rule_id=? AND instrument_id=? AND on_date=? ORDER BY condition_version DESC LIMIT 1`,
		string(rule.ID), f.instrument.ID.String(), on).Scan(&truth); err != nil {
		t.Fatalf("read daily truth on %s: %v", on, err)
	}
	return truth
}

func assertReviewSourceInvalidated(t *testing.T, f seriesZeroFixture) {
	t.Helper()
	capture, err := sqlite.NewRepository(f.db).CaptureReviewProjection(t.Context(), f.account.Account.HouseholdID, *f.now)
	if err != nil {
		t.Fatal(err)
	}
	if capture.Control.PublishedFingerprint == capture.HistoricalFingerprint || len(capture.ReusableDailyEvidence) != 0 {
		t.Fatalf("corrected source retained reusable daily evidence: published=%s current=%s reusable=%d", capture.Control.PublishedFingerprint, capture.HistoricalFingerprint, len(capture.ReusableDailyEvidence))
	}
}

func TestReviewHistoricalQuoteActivityAndFXCorrectionsInvalidateAndRecompute(t *testing.T) {
	t.Run("quote correction at its original date", func(t *testing.T) {
		f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
		importReviewDailyClose(t, f, "2026-08-01", "100")
		f.day(t, "2026-08-08")
		quoteID := importReviewDailyClose(t, f, "2026-08-08", "120")
		*f.now = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
		spec := syntheticHistoricalReviewSpec()
		spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
			{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
			{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}},
		}}
		rule := saveSyntheticReviewRule(t, f, "Synthetic quote correction", spec)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-08"); got != "true" {
			t.Fatalf("initial corrected-date quote truth = %s, want true", got)
		}
		correction := AgentMarketDataItem{Operation: "correct", TargetQuoteID: quoteID, Kind: "close", InstrumentID: f.instrument.ID.String(), Currency: "CNY", Date: "2026-08-08", Timezone: "UTC", Value: "105", SourceTitle: "Synthetic original-date quote correction"}
		if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
			t.Fatal(err)
		}
		assertReviewSourceInvalidated(t, f)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-08"); got != "false" {
			t.Fatalf("replayed corrected-date quote truth = %s, want false", got)
		}
	})

	t.Run("activity correction changes a value at its original effective date", func(t *testing.T) {
		f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
		*f.now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
		unitCost, err := domain.ParseUnitPrice("100")
		if err != nil {
			t.Fatal(err)
		}
		original, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{
			HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID,
			Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &unitCost, EffectiveAt: *f.now,
		})
		if err != nil {
			t.Fatal(err)
		}
		*f.now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
		spec := syntheticHistoricalReviewSpec()
		spec.Predicate.All[1].Leaf.Value = "150"
		rule := saveSyntheticReviewRule(t, f, "Synthetic activity correction", spec)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-02"); got != "false" {
			t.Fatalf("initial corrected-date activity truth = %s, want false", got)
		}
		replacement := domain.PositionAdjustmentInput{
			HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID,
			Quantity: mustQuantity(t, "0.25"), Added: true, UnitCost: &unitCost, EffectiveAt: original.Activity.EffectiveAt,
		}
		if _, err := f.s.FixChange(t.Context(), original.Activity.ID, replacement); err != nil {
			t.Fatal(err)
		}
		assertReviewSourceInvalidated(t, f)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		var correctedCost string
		if err := f.db.SQL.QueryRow(`SELECT cost_exact FROM investment_review_daily_inputs WHERE household_id=? AND instrument_id=? AND on_date='2026-08-02' AND input_policy=? ORDER BY computed_at DESC LIMIT 1`, f.account.Account.HouseholdID.String(), f.instrument.ID.String(), "rule:"+string(rule.ID)+"/v:1").Scan(&correctedCost); err != nil || correctedCost != "125" {
			var detail string
			_ = f.db.SQL.QueryRow(`SELECT evidence_json FROM investment_review_daily_results WHERE rule_id=? AND instrument_id=? AND on_date='2026-08-02' ORDER BY condition_version DESC LIMIT 1`, string(rule.ID), f.instrument.ID.String()).Scan(&detail)
			rows, _ := f.db.SQL.Query(`SELECT a.kind,a.effective_local_date,COALESCE(a.reverses_activity_id,''),e.direction,e.target,COALESCE(e.quantity,'') FROM activities a JOIN activity_effects e ON e.activity_id=a.id WHERE e.holding_id=? ORDER BY a.effective_at,a.created_at`, f.holding.ID.String())
			if rows != nil {
				defer rows.Close()
				for rows.Next() {
					var kind, date, reverses, direction, target, qty string
					if rows.Scan(&kind, &date, &reverses, &direction, &target, &qty) == nil {
						t.Logf("activity timeline: kind=%s date=%s reverses=%s direction=%s target=%s quantity=%s", kind, date, reverses, direction, target, qty)
					}
				}
			}
			t.Fatalf("corrected historical cost = %q, want 125: %v; evidence=%s", correctedCost, err, detail)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-02"); got != "true" {
			t.Fatalf("replayed corrected-date activity truth = %s, want true (cost=%s)", got, correctedCost)
		}
	})

	t.Run("FX correction at its original economic date", func(t *testing.T) {
		f := newSeriesZeroFixture(t, true, "UTC", "2026-08-01", "1")
		if _, err := f.s.SetFXPreference(t.Context(), "USD", "CNY", "agent"); err != nil {
			t.Fatal(err)
		}
		importReviewDailyClose(t, f, "2026-08-01", "100")
		importReviewDailyFX(t, f, "2026-08-01", "2")
		f.day(t, "2026-08-02")
		importReviewDailyClose(t, f, "2026-08-02", "100")
		fxID := importReviewDailyFX(t, f, "2026-08-02", "2")
		*f.now = time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
		spec := syntheticHistoricalReviewSpec()
		spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
			{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
			{Leaf: &domain.ReviewRuleLeaf{Metric: "amount", Operator: ">", Value: "150", Currency: "CNY", FXMode: "convert_on_day"}},
		}}
		rule := saveSyntheticReviewRule(t, f, "Synthetic FX correction", spec)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-02"); got != "true" {
			t.Fatalf("initial corrected-date FX truth = %s, want true", got)
		}
		correction := AgentMarketDataItem{Operation: "correct", TargetQuoteID: fxID, Kind: "daily_reference", BaseCurrency: "USD", QuoteCurrency: "CNY", Date: "2026-08-02", Timezone: "UTC", Value: "1", SourceTitle: "Synthetic original-date FX correction"}
		if _, err := f.s.ImportAgentMarketData(t.Context(), domain.NewActivityID().String(), AgentMarketDataInput{Items: []AgentMarketDataItem{correction}}); err != nil {
			t.Fatal(err)
		}
		assertReviewSourceInvalidated(t, f)
		if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := reviewTruthAt(t, f, rule, "2026-08-02"); got != "false" {
			t.Fatalf("replayed corrected-date FX truth = %s, want false", got)
		}
	})
}

func TestReviewTimezoneEditThroughMutationAndPublicationKeepsAcknowledgement(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-05", "1")
	importReviewDailyClose(t, f, "2026-10-05", "100")
	f.day(t, "2026-10-12")
	importReviewDailyClose(t, f, "2026-10-12", "120")
	*f.now = time.Date(2026, 10, 13, 2, 0, 0, 0, time.UTC)
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate = domain.ReviewPredicateNode{All: []domain.ReviewPredicateNode{
		{Leaf: &domain.ReviewRuleLeaf{Metric: "type", Operator: "in", Types: []domain.InstrumentType{domain.InstrumentStock}}},
		{Leaf: &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}},
	}}
	rule := saveSyntheticReviewRule(t, f, "Synthetic timezone review", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID, triggeredOn string
	var episodeRevision int
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,revision FROM investment_review_episodes WHERE rule_id=? AND instrument_id=? AND resolved_at IS NULL`, string(rule.ID), f.instrument.ID.String()).Scan(&episodeID, &triggeredOn, &episodeRevision); err != nil {
		t.Fatal(err)
	}
	if triggeredOn != "2026-10-12" || reviewTruthAt(t, f, rule, triggeredOn) != "true" {
		t.Fatalf("fixture did not publish the Oct 12 trigger: triggered=%s", triggeredOn)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episodeID, string(domain.NewMutationID()), episodeRevision); err != nil {
		t.Fatal(err)
	}
	var acknowledgedAt string
	if err := f.db.SQL.QueryRow(`SELECT acknowledged_at FROM investment_review_episodes WHERE id=?`, episodeID).Scan(&acknowledgedAt); err != nil || acknowledgedAt == "" {
		t.Fatalf("acknowledgement was not stored: %q %v", acknowledgedAt, err)
	}
	updatedSpec := rule.Spec
	updatedSpec.Schedule.Timezone = "America/Los_Angeles"
	if _, err := f.s.UpdateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{
		RuleID: string(rule.ID), MutationID: string(domain.NewMutationID()), ExpectedRevision: rule.RowRevision,
		Name: rule.Name, Enabled: rule.Enabled, Severity: rule.Severity, Spec: updatedSpec,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var afterID, afterTriggered, afterAcknowledged string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,acknowledged_at FROM investment_review_episodes WHERE rule_id=? AND instrument_id=? AND resolved_at IS NULL`, string(rule.ID), f.instrument.ID.String()).Scan(&afterID, &afterTriggered, &afterAcknowledged); err != nil {
		t.Fatal(err)
	}
	if afterID != episodeID || afterTriggered != triggeredOn || afterAcknowledged != acknowledgedAt {
		t.Fatalf("timezone mutation/publication lost the existing episode or ack: before=%s/%s/%s after=%s/%s/%s", episodeID, triggeredOn, acknowledgedAt, afterID, afterTriggered, afterAcknowledged)
	}
}

func TestReviewPublicationFailureAfterFinancialCommitDoesNotHalfWrite(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	rule := saveSyntheticReviewRule(t, f, "Synthetic S13 atomicity", syntheticHistoricalReviewSpec())
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var beforeInputs, beforeResults, beforeEpisodes, beforeEvents, beforeState int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_inputs`).Scan(&beforeInputs); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_daily_results`).Scan(&beforeResults); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes`).Scan(&beforeEpisodes); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episode_events`).Scan(&beforeEvents); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_evaluation_state`).Scan(&beforeState); err != nil {
		t.Fatal(err)
	}
	var beforeGeneration int
	var beforeFingerprint string
	if err := f.db.SQL.QueryRow(`SELECT input_generation,published_input_fingerprint FROM investment_review_control WHERE household_id=?`, f.account.Account.HouseholdID.String()).Scan(&beforeGeneration, &beforeFingerprint); err != nil {
		t.Fatal(err)
	}

	*f.now = time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	unitCost, err := domain.ParseUnitPrice("100")
	if err != nil {
		t.Fatal(err)
	}
	financial, err := f.s.RecordChange(t.Context(), domain.PositionAdjustmentInput{
		HouseholdID: f.account.Account.HouseholdID, HoldingID: f.holding.ID,
		Quantity: mustQuantity(t, "1"), Added: true, UnitCost: &unitCost, EffectiveAt: *f.now,
	})
	if err != nil {
		t.Fatalf("financial commit failed before review publication: %v", err)
	}
	var persistedActivity int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM activities WHERE id=?`, financial.Activity.ID.String()).Scan(&persistedActivity); err != nil || persistedActivity != 1 {
		t.Fatalf("committed financial activity missing: %d %v", persistedActivity, err)
	}
	var persistedQuantity string
	if err := f.db.SQL.QueryRow(`SELECT quantity FROM holdings WHERE id=?`, f.holding.ID.String()).Scan(&persistedQuantity); err != nil || persistedQuantity != "2" {
		t.Fatalf("financial holding update missing: quantity=%q err=%v", persistedQuantity, err)
	}

	if _, err := f.db.SQL.Exec(`CREATE TRIGGER force_review_publication_failure
		BEFORE UPDATE OF input_generation ON investment_review_control
		WHEN NEW.input_generation<>OLD.input_generation
		BEGIN SELECT RAISE(ABORT,'synthetic publication failure after staged rows'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err == nil {
		t.Fatal("expected forced review publication failure")
	}

	var inputs, results, episodes, events, states int
	for query, target := range map[string]*int{
		`SELECT count(*) FROM investment_review_daily_inputs`:     &inputs,
		`SELECT count(*) FROM investment_review_daily_results`:    &results,
		`SELECT count(*) FROM investment_review_episodes`:         &episodes,
		`SELECT count(*) FROM investment_review_episode_events`:   &events,
		`SELECT count(*) FROM investment_review_evaluation_state`: &states,
	} {
		if err := f.db.SQL.QueryRow(query).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	var afterGeneration int
	var afterFingerprint string
	if err := f.db.SQL.QueryRow(`SELECT input_generation,published_input_fingerprint FROM investment_review_control WHERE household_id=?`, f.account.Account.HouseholdID.String()).Scan(&afterGeneration, &afterFingerprint); err != nil {
		t.Fatal(err)
	}
	if inputs != beforeInputs || results != beforeResults || episodes != beforeEpisodes || events != beforeEvents || states != beforeState || afterGeneration != beforeGeneration || afterFingerprint != beforeFingerprint {
		t.Fatalf("failed publication leaked review writes: inputs=%d/%d results=%d/%d episodes=%d/%d events=%d/%d state=%d/%d generation=%d/%d fingerprint=%s/%s", inputs, beforeInputs, results, beforeResults, episodes, beforeEpisodes, events, beforeEvents, states, beforeState, afterGeneration, beforeGeneration, afterFingerprint, beforeFingerprint)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM activities WHERE id=?`, financial.Activity.ID.String()).Scan(&persistedActivity); err != nil || persistedActivity != 1 {
		t.Fatalf("review failure rolled back the committed financial activity: %d %v", persistedActivity, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT quantity FROM holdings WHERE id=?`, f.holding.ID.String()).Scan(&persistedQuantity); err != nil || persistedQuantity != "2" {
		t.Fatalf("review failure rolled back the committed holding: quantity=%q err=%v", persistedQuantity, err)
	}
	if rule.ID == "" {
		t.Fatal("test rule was not created")
	}
}
