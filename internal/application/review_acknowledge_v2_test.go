package application

import (
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewAcknowledgementV2MutatesOnlyEpisodeHandling(t *testing.T) {
	s, db, _, _ := overviewFixture(t)
	instrument, err := s.CreateInstrument(t.Context(), InstrumentInput{Name: "Synthetic stock", Type: "stock", QuoteCurrency: "CNY", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Synthetic", Enabled: true, Severity: "notice", Spec: syntheticHistoricalReviewSpec()})
	if err != nil {
		t.Fatal(err)
	}
	epoch, episode := string(domain.NewReviewEpisodeID()), string(domain.NewReviewEpisodeID())
	if _, err := db.SQL.Exec(`INSERT INTO investment_review_position_epochs(id,household_id,instrument_id,start_unknown,source_fingerprint) VALUES(?,?,?,1,'synthetic')`, epoch, rule.HouseholdID.String(), instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec(`INSERT INTO investment_review_episodes(id,rule_id,instrument_id,position_epoch_id,opened_condition_version,first_hit_on,triggered_on,first_detected_at,window_start,window_end,latest_closed_on,hit_count,false_count,unknown_count,consecutive_count,evidence_json,evidence_fingerprint,revision)
VALUES(?,?,?,?,1,'2026-08-01','2026-08-01','2026-08-02T00:00:00Z','2026-08-01','2026-08-01','2026-08-01',1,0,0,1,'{}','synthetic-fingerprint',1)`, episode, string(rule.ID), instrument.ID.String(), epoch); err != nil {
		t.Fatal(err)
	}
	mutation := string(domain.NewMutationID())
	first, err := s.AcknowledgeReviewEpisodeV2(t.Context(), episode, mutation, 1)
	if err != nil || first.Revision != 2 || first.AcknowledgedAt.IsZero() {
		t.Fatalf("first ack: %+v %v", first, err)
	}
	again, err := s.AcknowledgeReviewEpisodeV2(t.Context(), episode, mutation, 1)
	if err != nil || again != first {
		t.Fatalf("retry duplicated ack: %+v %v", again, err)
	}
	if _, err := s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 1); err == nil {
		t.Fatal("stale revision was accepted")
	}
	noOp, err := s.AcknowledgeReviewEpisodeV2(t.Context(), episode, string(domain.NewMutationID()), 2)
	if err != nil || noOp != first {
		t.Fatalf("already understood must be no-op: %+v %v", noOp, err)
	}
	var revision, stateRevision int
	var evidence string
	if err := db.SQL.QueryRow(`SELECT revision,evidence_fingerprint FROM investment_review_episodes WHERE id=?`, episode).Scan(&revision, &evidence); err != nil || revision != 2 || evidence != "synthetic-fingerprint" {
		t.Fatalf("ack overwrote evidence: rev=%d evidence=%q %v", revision, evidence, err)
	}
	if err := db.SQL.QueryRow(`SELECT review_state_revision FROM investment_review_control WHERE household_id=?`, rule.HouseholdID.String()).Scan(&stateRevision); err != nil || stateRevision != 1 {
		t.Fatalf("handling revision=%d %v", stateRevision, err)
	}
}
