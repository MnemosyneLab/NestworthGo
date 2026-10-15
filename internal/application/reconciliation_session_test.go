package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func reconciliationFixture(t *testing.T) (*Service, *sqlite.DB, string) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "synthetic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s := NewService(sqlite.NewRepository(db))
	s.SetClock(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	ctx := context.Background()
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Tester"}}); err != nil {
		t.Fatal(err)
	}
	b, _ := s.Bootstrap(ctx)
	a, err := s.CreateAccount(ctx, AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: "USD", InitialAmount: "100", OwnerIDs: []domain.MemberID{b.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	return s, db, a.Account.ID.String()
}
func recAction(t *testing.T, s *Service, a ReconciliationAction) domain.ReconciliationSession {
	t.Helper()
	p, err := s.PreviewReconciliationSession(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CommitReconciliationSession(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func TestReconciliationSessionEvidenceDurabilityPartialAndImmutable(t *testing.T) {
	s, db, aid := reconciliationFixture(t)
	ctx := context.Background()
	for _, ids := range [][]string{{aid, aid}, {"00000000-0000-4000-8000-000000000099"}} {
		if _, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "start", AccountIDs: ids}); err == nil {
			t.Fatal("invalid scope accepted")
		}
	}
	p, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.CommitReconciliationSession(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	restarted := NewService(sqlite.NewRepository(db))
	again, err := restarted.CommitReconciliationSession(ctx, p)
	if err != nil || again.ID != v.ID {
		t.Fatalf("receipt restart: %+v %v", again, err)
	}
	changed := p
	changed.Action.Note = "changed"
	if _, err := s.CommitReconciliationSession(ctx, changed); err == nil {
		t.Fatal("changed receipt accepted")
	}
	if v.Counts.Pending != 2 || v.Counts.Confirmed != 0 {
		t.Fatalf("equality must start pending: %+v", v)
	}
	v = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: v.ID, ExpectedRevision: v.Revision})
	v, err = restarted.GetReconciliationSession(ctx, v.ID)
	if err != nil || v.Status != "paused" {
		t.Fatalf("pause: %+v %v", v, err)
	}
	v = recAction(t, s, ReconciliationAction{Action: "resume", SessionID: v.ID, ExpectedRevision: v.Revision})
	a := ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: "balance", FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed"}
	if _, err := s.PreviewReconciliationSession(ctx, a); err == nil {
		t.Fatal("fabricated source accepted")
	}
	a.Source = domain.ReconciliationSource{Reference: "Synthetic statement", AsOfDate: "2026-09-01"}
	v = recAction(t, s, a)
	if v.Accounts[0].Items[0].Source.AsOfTime != "" {
		t.Fatal("invented time")
	}
	if _, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision}); err == nil {
		t.Fatal("implicit partial finish accepted")
	}
	v = recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision, AllowPartial: true, Note: "Scope still unchecked"})
	if !v.Partial || v.Counts.Pending != 1 {
		t.Fatalf("partial counts: %+v", v)
	}
	last, err := s.ReconciliationLastSuccess(ctx, aid)
	if err != nil || last != nil {
		t.Fatalf("partial advanced success: %+v %v", last, err)
	}
	if _, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "resume", SessionID: v.ID, ExpectedRevision: v.Revision}); err == nil {
		t.Fatal("completed session mutable")
	}
}
func TestReconciliationSessionRevisionAndSuccess(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	ctx := context.Background()
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	for _, item := range v.Accounts[0].Items {
		v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic bank", AsOfDate: "2026-09-01"}})
	}
	finish, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision})
	if err != nil {
		t.Fatal(err)
	}
	id, _ := domain.ParseAccountID(aid)
	money, _ := domain.ParseMoney("1", domain.CurrencyCode("USD"))
	h, _ := s.requireHousehold(ctx)
	_, err = s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: h.ID, AccountID: id, Amount: money, Reason: domain.ReasonReconciliation, EffectiveAt: s.clock()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitReconciliationSession(ctx, finish); err == nil {
		t.Fatal("stale finish accepted")
	}
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Counts.Pending != 2 || v.Counts.Confirmed != 0 {
		t.Fatalf("changed facts retained confirmation: %+v", v)
	}
	for _, item := range v.Accounts[0].Items {
		v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic bank", AsOfDate: "2026-09-01"}})
	}
	v = recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision})
	last, err := s.ReconciliationLastSuccess(ctx, aid)
	if err != nil || last == nil || last.ID != v.ID {
		t.Fatalf("success not retained: %+v %v", last, err)
	}
	_, err = s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: h.ID, AccountID: id, Amount: money, Reason: domain.ReasonReconciliation, EffectiveAt: s.clock()})
	if err != nil {
		t.Fatal(err)
	}
	original, err := s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !original.Accounts[0].RecheckRequired || original.Counts.Confirmed != 2 || original.CompletedAt != v.CompletedAt {
		t.Fatalf("history overwritten: %+v", original)
	}
}

func TestReconciliationLinkedCorrectionAtomicReceiptAndExplicitRecheck(t *testing.T) {
	s, db, aid := reconciliationFixture(t)
	ctx := context.Background()
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	link := &domain.ReconciliationLink{SessionID: v.ID, AccountID: aid, ItemID: "balance", ExpectedRevision: v.Revision, FinancialRevision: v.Accounts[0].FinancialRevision}
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
	if err = s.ValidateReconciliationLink(ctx, link, previews); err != nil {
		t.Fatal(err)
	}
	mutation := domain.NewMutationID().String()
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	// An injected failure after the link is inserted must roll back link, money and receipt together.
	if _, err = db.SQL.Exec(`CREATE TRIGGER synthetic_link_failure BEFORE INSERT ON activities BEGIN SELECT RAISE(ABORT,'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, mutation, hash, token); err == nil {
		t.Fatal("injected failure ignored")
	}
	links, err := s.ReconciliationLinkedChanges(ctx, v.ID)
	if err != nil || len(links) != 0 {
		t.Fatalf("failed financial write linked: %+v %v", links, err)
	}
	db.SQL.Exec("DROP TRIGGER synthetic_link_failure")
	previews, token, err = s.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
	if err != nil {
		t.Fatal(err)
	}
	committed, err := s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, mutation, hash, token)
	if err != nil {
		t.Fatal(err)
	}
	links, err = s.ReconciliationLinkedChanges(ctx, v.ID)
	if err != nil || len(links) != 1 {
		t.Fatalf("link missing: %+v %v", links, err)
	}
	again, err := NewService(sqlite.NewRepository(db)).RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, mutation, hash, "expired")
	if err != nil || again[0].Activity.ID != committed[0].Activity.ID {
		t.Fatalf("receipt failed: %+v %v", again, err)
	}
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Counts.Confirmed != 0 || v.Counts.Pending != 2 {
		t.Fatalf("correction auto-confirmed: %+v", v)
	}
	v = recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision, AllowPartial: true, Note: "Still reviewing"})
	if _, err = s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, domain.NewMutationID().String(), hash, s.writes.previewToken()); err == nil {
		t.Fatal("completed session accepted a new correction")
	}
}
