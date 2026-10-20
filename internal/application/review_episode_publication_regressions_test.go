package application

import (
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func openAcknowledgedCostEpisodeOnOctober10(t *testing.T) (seriesZeroFixture, domain.ReviewRule, string) {
	t.Helper()
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-05")
	importReviewDailyClose(t, f, "2026-10-05", "100")
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf.Operator = ">"
	spec.Predicate.All[1].Leaf.Value = "150"
	rule := saveSyntheticReviewRule(t, f, "Synthetic opening cost", spec)
	f.day(t, "2026-10-10")
	f.quantity(t, true) // Cost crosses 150 on October 10.
	f.day(t, "2026-10-11")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID, triggered string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episodeID, &triggered); err != nil || triggered != "2026-10-10" {
		t.Fatalf("synthetic cost episode trigger=%s err=%v", triggered, err)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episodeID, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	return f, rule, episodeID
}

func unknownThenTruePriceChangeSpec() domain.ReviewRuleSpec {
	spec := syntheticHistoricalReviewSpec()
	spec.Predicate.All[1].Leaf = &domain.ReviewRuleLeaf{Metric: "price_change", Operator: ">", Value: "10", LookbackDays: 7}
	return spec
}

func TestReviewPublicationMaterialEditKeepsOriginalTriggerSnapshotAcrossUnknownThenTrue(t *testing.T) {
	f, rule, episodeID := openAcknowledgedCostEpisodeOnOctober10(t)
	f.day(t, "2026-10-12")
	importReviewDailyClose(t, f, "2026-10-12", "120")
	f.day(t, "2026-10-13")
	updated, err := f.s.UpdateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{
		RuleID: string(rule.ID), MutationID: string(domain.NewMutationID()), ExpectedRevision: rule.RowRevision,
		Name: rule.Name, Enabled: rule.Enabled, Severity: rule.Severity, Spec: unknownThenTruePriceChangeSpec(),
	})
	if err != nil || updated.CurrentConditionVersion != 2 {
		t.Fatalf("material rule edit: %+v %v", updated, err)
	}
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ on, truth string }{
		{"2026-10-10", "unknown"}, {"2026-10-11", "unknown"}, {"2026-10-12", "true"},
	} {
		if got := reviewTruthAt(t, f, updated, check.on); got != check.truth {
			t.Fatalf("edited condition on %s = %s, want %s", check.on, got, check.truth)
		}
	}
	var gotID, triggered, firstHit, acknowledged, evidenceJSON string
	var openedVersion int
	if err := f.db.SQL.QueryRow(`SELECT id,opened_condition_version,triggered_on,first_hit_on,acknowledged_at,evidence_json
		FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&gotID, &openedVersion, &triggered, &firstHit, &acknowledged, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var evidence storedReviewEpisodeEvidence
	if err := jsoncodec.Unmarshal([]byte(evidenceJSON), &evidence); err != nil {
		t.Fatal(err)
	}
	if gotID != episodeID || openedVersion != 1 || triggered != "2026-10-10" || firstHit != "2026-10-10" || acknowledged == "" || evidence.TriggerWindow.WindowEnd != triggered || evidence.TriggerResult == nil || evidence.TriggerResult.On != triggered || evidence.TriggerResult.Truth != domain.ReviewTrue || evidence.Window.WindowEnd != "2026-10-12" {
		t.Fatalf("condition edit split the preserved opening metadata from its evidence: id=%s version=%d trigger=%s first=%s ack=%q evidence=%+v", gotID, openedVersion, triggered, firstHit, acknowledged, evidence)
	}
	var openedJSON string
	if err := f.db.SQL.QueryRow(`SELECT payload_json FROM investment_review_episode_events WHERE episode_id=? AND kind='opened'`, episodeID).Scan(&openedJSON); err != nil {
		t.Fatal(err)
	}
	var opened storedReviewEpisodeEvidence
	if err := jsoncodec.Unmarshal([]byte(openedJSON), &opened); err != nil || opened.TriggerWindow.WindowEnd != triggered || opened.TriggerResult == nil || opened.TriggerResult.On != triggered {
		t.Fatalf("opening event no longer matches preserved trigger date: %+v %v", opened, err)
	}
}

func TestReviewPublicationRetainsLegacyAcknowledgedEpisodeWithoutTriggerEvidence(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	requireDailyReviewPrices(t, &f)
	importReviewDailyClose(t, f, "2026-10-01", "100")
	f.day(t, "2026-10-05")
	importReviewDailyClose(t, f, "2026-10-05", "100")
	f.day(t, "2026-10-10")
	triggerQuoteID := importReviewDailyClose(t, f, "2026-10-10", "120")
	f.day(t, "2026-10-11")
	spec := unknownThenTruePriceChangeSpec()
	allowDailyReviewCrypto(&spec.Predicate)
	rule := saveSyntheticReviewRule(t, f, "Synthetic legacy trigger", spec)
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	var episodeID, triggered string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&episodeID, &triggered); err != nil || triggered != "2026-10-10" {
		var daily string
		_ = f.db.SQL.QueryRow(`SELECT group_concat(on_date||'='||predicate_truth, ',') FROM investment_review_daily_results WHERE rule_id=?`, string(rule.ID)).Scan(&daily)
		t.Fatalf("synthetic price episode trigger=%s err=%v daily=%s", triggered, err, daily)
	}
	if _, err := f.s.AcknowledgeReviewEpisodeV2(t.Context(), episodeID, string(domain.NewMutationID()), 1); err != nil {
		t.Fatal(err)
	}
	// Model the pre-trigger-snapshot projection: the current window and first
	// hit were stored, but there is no TriggerWindow or trigger-day result.
	legacyEvidence := `{"Window":{"truth":"true","windowStart":"2026-10-10","windowEnd":"2026-10-10","hitCount":1},"FirstHitOn":"2026-10-10","Coverage":[]}`
	if _, err := f.db.SQL.Exec(`UPDATE investment_review_episodes SET evidence_json=? WHERE id=?`, legacyEvidence, episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`UPDATE investment_review_episode_events SET payload_json=? WHERE episode_id=? AND kind='opened'`, legacyEvidence, episodeID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SQL.Exec(`DELETE FROM instrument_quotes WHERE id=?`, triggerQuoteID); err != nil {
		t.Fatal(err)
	}
	// The source correction removes the only T vote. The current same-version
	// policy is now U, which retains the unresolved episode and acknowledgement.
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatalf("publish should retain an acknowledged episode whose old trigger evidence is unavailable: %v", err)
	}
	var gotID, gotTriggered, firstHit, acknowledged, evidenceJSON string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,first_hit_on,acknowledged_at,evidence_json
		FROM investment_review_episodes WHERE rule_id=? AND resolved_at IS NULL`, string(rule.ID)).Scan(&gotID, &gotTriggered, &firstHit, &acknowledged, &evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var unavailable storedReviewEpisodeEvidence
	if err := jsoncodec.Unmarshal([]byte(evidenceJSON), &unavailable); err != nil {
		t.Fatal(err)
	}
	if gotID != episodeID || gotTriggered != "2026-10-10" || firstHit != "2026-10-10" || acknowledged == "" || unavailable.TriggerWindow.WindowStart != "" || unavailable.TriggerResult != nil || unavailable.Window.Truth != domain.ReviewUnknown {
		t.Fatalf("unavailable trigger was fabricated or episode handling was lost: id=%s trigger=%s first=%s ack=%q evidence=%+v", gotID, gotTriggered, firstHit, acknowledged, unavailable)
	}
	page, err := f.s.ListReviewDailyEvidenceV2(t.Context(), episodeID, "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].On != "2026-10-10" || page.Items[0].Truth != domain.ReviewUnknown {
		t.Fatalf("current corrected daily evidence should remain truthfully unknown: %+v %v", page, err)
	}

	// Reconcile a separate instrument/rule in the same household after the
	// unavailable episode was retained. Its group must still publish normally.
	f.day(t, "2026-10-12")
	other, err := f.s.CreateInstrument(t.Context(), InstrumentInput{Name: "Unrelated synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.AppendManualInstrumentQuote(t.Context(), other.ID, "100", f.now.Format(time.RFC3339), false); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.CreateHolding(t.Context(), HoldingInput{AccountID: f.account.Account.ID.String(), InstrumentID: other.ID.String(), Quantity: "1", UnitCost: "100"}); err != nil {
		t.Fatal(err)
	}
	otherSpec := syntheticHistoricalReviewSpec()
	otherSpec.Target = domain.ReviewRuleTarget{Kind: "instruments", InstrumentIDs: []domain.InstrumentID{other.ID}}
	otherSpec.Predicate.All[1].Leaf.Value = "50"
	otherSpec.Predicate.All[1].Leaf.Operator = ">"
	otherRule := saveSyntheticReviewRule(t, f, "Unrelated synthetic group", otherSpec)
	f.day(t, "2026-10-13")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatalf("unrelated group failed to reconcile beside legacy episode: %v", err)
	}
	alerts, err := f.s.ListReviewAlertGroupsV2(t.Context(), domain.ReviewAlertFilter{Status: "all"}, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	foundOther := false
	for _, group := range alerts.Items {
		if group.InstrumentID != other.ID {
			continue
		}
		for _, item := range group.Rules {
			foundOther = foundOther || item.RuleID == otherRule.ID && item.TriggeredOn == "2026-10-12"
		}
	}
	if !foundOther {
		t.Fatalf("separate synthetic instrument group did not publish: %+v", alerts.Items)
	}

	// Restoring the same-version source makes the original trigger reconstructable.
	importReviewDailyClose(t, f, "2026-10-10", "120")
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SQL.QueryRow(`SELECT evidence_json FROM investment_review_episodes WHERE id=?`, episodeID).Scan(&evidenceJSON); err != nil {
		t.Fatal(err)
	}
	var repaired storedReviewEpisodeEvidence
	if err := jsoncodec.Unmarshal([]byte(evidenceJSON), &repaired); err != nil || repaired.TriggerWindow.WindowStart != "2026-10-10" || repaired.TriggerWindow.WindowEnd != "2026-10-10" || repaired.TriggerResult == nil || repaired.TriggerResult.On != "2026-10-10" || repaired.TriggerResult.Truth != domain.ReviewTrue {
		t.Fatalf("later same-version evidence did not repair the legacy trigger snapshot: %+v %v", repaired, err)
	}
	var repairedID, repairedTrigger, repairedAck string
	if err := f.db.SQL.QueryRow(`SELECT id,triggered_on,acknowledged_at FROM investment_review_episodes WHERE id=?`, episodeID).Scan(&repairedID, &repairedTrigger, &repairedAck); err != nil || repairedID != episodeID || repairedTrigger != "2026-10-10" || repairedAck == "" {
		t.Fatalf("legacy evidence repair changed identity/trigger/ack: id=%s trigger=%s ack=%q err=%v", repairedID, repairedTrigger, repairedAck, err)
	}
}
