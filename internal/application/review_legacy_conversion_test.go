package application

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestReviewLegacyConversionPreviewsThenAtomicallyArchivesAndCreatesTwoRules(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-08-01", "1")
	f.day(t, "2026-08-02")
	store := sqlite.NewConfigurationRepository(f.db)
	key := "investment_review/" + f.account.Account.HouseholdID.String()
	old := domain.ReviewLegacyConfiguration{Policy: domain.DefaultInvestmentReviewPolicy(), Revision: 4,
		Enrolled: map[string]bool{f.instrument.ID.String(): true}, Acknowledged: map[string]bool{"unresolvable-old-ack": true},
		SnoozedUntil: map[string]string{f.instrument.ID.String(): "2026-08-30"}}
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveConfiguration(key, raw); err != nil {
		t.Fatal(err)
	}
	rules, err := f.s.ListReviewRulesV2(t.Context(), "", 10)
	if err != nil || len(rules) != 1 || rules[0].Evaluator != domain.ReviewLegacyEvaluator {
		t.Fatalf("legacy rule: %+v %v", rules, err)
	}
	draft := ReviewLegacyConversionDraft{LegacyRuleID: string(rules[0].ID), TargetKind: "instruments", Timezone: "UTC", Trigger: domain.ReviewTriggerPolicy{Kind: "single"}}
	var before, after int
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	preview, err := f.s.PreviewReviewLegacyConversionV2(t.Context(), draft)
	if err != nil || preview.LegacyRevision != rules[0].RowRevision || preview.RecommendedPolicy.Count != 7 || len(preview.LowYieldSpec.Target.InstrumentIDs) != 1 || preview.SourceToken == "" || preview.LowYieldAlerts != 0 || preview.LossAlerts != 0 {
		t.Fatalf("conversion preview: %+v %v", preview, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT total_changes()`).Scan(&after); err != nil || after != before {
		t.Fatalf("preview wrote %d rows: %v", after-before, err)
	}
	input := ConvertReviewLegacyRuleV2Input{ReviewLegacyConversionDraft: draft, ExpectedRevision: preview.LegacyRevision, ExpectedConfigRevision: preview.ConfigRevision, ExpectedStateRevision: preview.StateRevision, ExpectedSourceToken: strings.Repeat("0", 64), LowYieldRuleID: string(domain.NewReviewRuleID()), LossRuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID())}
	input.ExpectedStateRevision++
	if _, err := f.s.ConvertReviewLegacyRuleV2(t.Context(), input); err == nil {
		t.Fatal("accepted a stale suppression/handling preview")
	}
	input.ExpectedStateRevision = preview.StateRevision
	if _, err := f.s.ConvertReviewLegacyRuleV2(t.Context(), input); err == nil {
		t.Fatal("accepted a stale conversion preview")
	}
	var archived int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_rules WHERE id=? AND archived_at IS NOT NULL`, draft.LegacyRuleID).Scan(&archived); err != nil || archived != 0 {
		t.Fatalf("stale preview partly archived legacy rule: %d %v", archived, err)
	}
	input.ExpectedSourceToken = preview.SourceToken
	created, err := f.s.ConvertReviewLegacyRuleV2(t.Context(), input)
	if err != nil || len(created) != 2 || created[0].Evaluator != domain.ReviewEvaluator || created[1].Evaluator != domain.ReviewEvaluator || created[0].ID == created[1].ID || created[0].Spec.TriggerPolicy.Kind != "single" {
		t.Fatalf("conversion: %+v %v", created, err)
	}
	retried, err := f.s.ConvertReviewLegacyRuleV2(t.Context(), input)
	if err != nil || len(retried) != 2 || retried[0].ID != created[0].ID || retried[1].ID != created[1].ID {
		t.Fatalf("conversion receipt: %+v %v", retried, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_rules WHERE id=? AND archived_at IS NOT NULL`, draft.LegacyRuleID).Scan(&archived); err != nil || archived != 1 {
		t.Fatalf("legacy rule not archived atomically: %d %v", archived, err)
	}
	var count int
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_suppressions WHERE instrument_id=? AND until_on='2026-08-30' AND timezone='UTC'`, f.instrument.ID.String()).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy absolute snooze lost: %d %v", count, err)
	}
	if err := f.db.SQL.QueryRow(`SELECT count(*) FROM investment_review_episodes WHERE acknowledged_at IS NOT NULL`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("irreversible old ack was forged as daily ack: %d %v", count, err)
	}
	loaded, found, err := store.LoadConfiguration(key)
	if err != nil || !found {
		t.Fatalf("archived legacy state unavailable: %s %t %v", loaded, found, err)
	}
	state, err := domain.DecodeReviewLegacyConfiguration(loaded)
	if err != nil || state.Policy.Enabled || !state.Acknowledged["unresolvable-old-ack"] {
		t.Fatalf("archived v1 history changed: %+v %v", state, err)
	}
	if err := store.SaveConfiguration(key, raw); err == nil {
		t.Fatal("legacy v1 writer reactivated an archived compatibility rule")
	}
}
