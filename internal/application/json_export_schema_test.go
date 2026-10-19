package application

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func exportSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := os.ReadFile("../../docs/design/json-export-v2.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	resolved, err := schema.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
func validateExportSchema(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if err := exportSchema(t).Validate(doc); err != nil {
		t.Fatalf("actual export violates v2 schema: %v", err)
	}
	return doc
}

func TestJSONExportReconciliationSchemaCompatibility(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	ctx := t.Context()
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}, Note: "Separate opening note"})
	id, _ := domain.ParseAccountID(aid)
	target, _ := domain.ParseMoney("120", domain.CurrencyCode("USD"))
	var commands []any
	var err error
	previews, token, err := s.PreviewChangesGuardedWith(ctx, func(ctx context.Context) ([]any, error) {
		commands, err = s.BuildReconciliationCommands(ctx, []ReconciliationTarget{{AccountID: &id, TargetBalance: &target}})
		return commands, err
	})
	if err != nil {
		t.Fatal(err)
	}
	link := &domain.ReconciliationLink{SessionID: v.ID, AccountID: aid, ItemID: "balance", ExpectedRevision: v.Revision, FinancialRevision: v.Accounts[0].FinancialRevision}
	if err := s.ValidateReconciliationLink(ctx, link, previews); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, domain.NewMutationID().String(), strings.Repeat("b", 64), token); err != nil {
		t.Fatal(err)
	}
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range v.Accounts[0].Items {
		v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic source", AsOfDate: "2026-09-01"}})
	}
	v = recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision, Note: "Separate closing explanation"})
	data, err := s.ExportJSONBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	doc := validateExportSchema(t, data)
	history := doc["facts"].(map[string]any)["history"].(map[string]any)
	optional := []string{"reconciliationSessions", "reconciliationAccounts", "reconciliationItems", "reconciliationLinks", "accountFinancialRevisions", "reconciliationFinancialChanges"}
	for _, name := range optional {
		if len(history[name].([]any)) == 0 {
			t.Fatalf("fixture omitted real %s rows", name)
		}
	}
	session := history["reconciliationSessions"].([]any)[0].(map[string]any)
	if session["closingNote"] != v.ClosingNote || session["note"] != v.Note || v.ClosingNote == v.Note {
		t.Fatal("notes not exported separately")
	}
	schema := exportSchema(t)
	// The new field is additive: accept older session rows without closingNote.
	delete(session, "closingNote")
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("older v2 session row rejected: %v", err)
	}
	session["closingNote"] = 7
	if err := schema.Validate(doc); err == nil {
		t.Fatal("invalid closingNote type accepted")
	}
	delete(session, "closingNote")
	// Old v2 producers had none of these datasets. They remain optional, while
	// required original datasets and strict row envelopes remain enforced.
	for _, name := range optional {
		delete(history, name)
	}
	market := doc["facts"].(map[string]any)["marketData"].(map[string]any)
	delete(market, "agentQuoteBatches")
	delete(market, "agentQuoteRecords")
	if err := schema.Validate(doc); err != nil {
		t.Fatalf("older v2 export rejected: %v", err)
	}
	delete(history, "activities")
	if err := schema.Validate(doc); err == nil {
		t.Fatal("original required dataset became optional")
	}
	history["activities"] = []any{}
	history["reconciliationLinks"] = []any{map[string]any{"sessionId": v.ID}}
	if err := schema.Validate(doc); err == nil {
		t.Fatal("incomplete reconciliation envelope accepted")
	}
}

func TestJSONExportExcludesSourceJournalAndDoesNotAdvanceIt(t *testing.T) {
	f := newSeriesZeroFixture(t, false, "UTC", "2026-10-01", "1")
	f.day(t, "2026-10-03")
	saveSyntheticReviewRule(t, f, "Synthetic export certificate", syntheticHistoricalReviewSpec())
	if _, err := f.s.ReconcileInvestmentReviewsV2(t.Context()); err != nil {
		t.Fatal(err)
	}
	capture := temporalCapture(t, f)
	if capture.PublishedTemporal == nil {
		t.Fatal("export fixture omitted temporal checkpoint")
	}
	var epoch string
	var before, after int64
	if err := f.db.SQL.QueryRow(`SELECT tracking_epoch,revision FROM financial_source_revision`).Scan(&epoch, &before); err != nil {
		t.Fatal(err)
	}
	data, err := f.s.ExportJSONBytes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	validateExportSchema(t, data)
	if strings.Contains(string(data), "financial_source_") || strings.Contains(string(data), epoch) || strings.Contains(string(data), "review_temporal") || strings.Contains(string(data), capture.PublishedTemporal.ClockEpoch) || strings.Contains(string(data), domain.ReviewTemporalPolicyVersion) {
		t.Fatal("derived source metadata leaked into JSON contract")
	}
	if err = f.db.SQL.QueryRow(`SELECT revision FROM financial_source_revision`).Scan(&after); err != nil || after != before {
		t.Fatalf("export advanced tracking: %d -> %d %v", before, after, err)
	}
}
