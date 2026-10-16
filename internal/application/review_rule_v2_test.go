package application

import (
	"errors"
	"testing"

	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReviewRuleV2MutationCASCanonicalVersionAndReceipt(t *testing.T) {
	s, db, _, _ := overviewFixture(t)
	ruleID := string(domain.NewReviewRuleID())
	input := SaveReviewRuleV2Input{RuleID: ruleID, MutationID: string(domain.NewMutationID()), Name: " Synthetic cost ", Enabled: true, Severity: "notice", Spec: syntheticHistoricalReviewSpec()}
	created, err := s.CreateReviewRuleV2(t.Context(), input)
	if err != nil || created.Name != "Synthetic cost" || created.RowRevision != 1 || created.CurrentConditionVersion != 1 {
		t.Fatalf("create: %+v %v", created, err)
	}
	again, err := s.CreateReviewRuleV2(t.Context(), input)
	if err != nil || again.ID != created.ID || again.RowRevision != 1 {
		t.Fatalf("retry duplicate: %+v %v", again, err)
	}
	var receipts int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM investment_review_mutation_receipts`).Scan(&receipts); err != nil || receipts != 1 {
		t.Fatalf("receipt count=%d %v", receipts, err)
	}
	input.Name = "Different request"
	if _, err := s.CreateReviewRuleV2(t.Context(), input); err == nil {
		t.Fatal("same mutation ID accepted a different request")
	}
	input.MutationID = string(domain.NewMutationID())
	input.ExpectedRevision = 1
	input.Name = "Renamed"
	input.Spec.Predicate.All[1].Leaf.Value = "250.00"
	renamed, err := s.UpdateReviewRuleV2(t.Context(), input)
	if err != nil || renamed.RowRevision != 2 || renamed.CurrentConditionVersion != 1 || renamed.SemanticHash != created.SemanticHash {
		t.Fatalf("semantic no-op edit: %+v %v", renamed, err)
	}
	input.Spec.Predicate.All[1].Leaf.Value = "260"
	input.MutationID = string(domain.NewMutationID())
	input.ExpectedRevision = 2
	updated, err := s.UpdateReviewRuleV2(t.Context(), input)
	if err != nil || updated.RowRevision != 3 || updated.CurrentConditionVersion != 2 || updated.SemanticHash == created.SemanticHash {
		t.Fatalf("condition change: %+v %v", updated, err)
	}
	var versions int
	if err := db.SQL.QueryRow(`SELECT count(*) FROM investment_review_rule_versions WHERE rule_id=?`, ruleID).Scan(&versions); err != nil || versions != 2 {
		t.Fatalf("condition versions=%d %v", versions, err)
	}
	input.MutationID = string(domain.NewMutationID())
	input.ExpectedRevision = 2
	if _, err := s.UpdateReviewRuleV2(t.Context(), input); err == nil {
		t.Fatal("stale revision overwrote a rule")
	} else {
		var appErr *domain.Error
		if !errors.As(err, &appErr) || appErr.Code != domain.ErrConflict {
			t.Fatalf("stale revision: %v", err)
		}
	}
	archived, err := s.ArchiveReviewRuleV2(t.Context(), ruleID, string(domain.NewMutationID()), 3)
	if err != nil || archived.ArchivedAt == nil || archived.RowRevision != 4 || archived.CurrentConditionVersion != 2 {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	page, err := s.ListReviewRulesV2(t.Context(), "", 100)
	if err != nil || len(page) != 1 || page[0].ID != created.ID || page[0].ArchivedAt == nil {
		t.Fatalf("bounded read: %+v %v", page, err)
	}
	empty, err := s.ListReviewRulesV2(t.Context(), ruleID, 100)
	if err != nil || len(empty) != 0 {
		t.Fatalf("cursor: %+v %v", empty, err)
	}
}

func TestReviewRuleV2RejectsForeignOrUnknownTarget(t *testing.T) {
	s, _, _, _ := overviewFixture(t)
	spec := syntheticHistoricalReviewSpec()
	spec.Target = domain.ReviewRuleTarget{Kind: "instruments", InstrumentIDs: []domain.InstrumentID{domain.NewInstrumentID()}}
	_, err := s.CreateReviewRuleV2(t.Context(), SaveReviewRuleV2Input{RuleID: string(domain.NewReviewRuleID()), MutationID: string(domain.NewMutationID()), Name: "Invalid target", Enabled: true, Severity: "notice", Spec: spec})
	if err == nil {
		t.Fatal("accepted instrument outside household")
	}
}
