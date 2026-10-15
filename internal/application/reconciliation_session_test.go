package application

import (
	"context"
	"encoding/json"
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

func TestReconciliationMixedAccountProductLinksAndMarketDataIsolation(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	aid := seedHoldingsCash(t, s, ctx, "10000")
	create := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: aid.String(), Currency: "USD", Principal: "1000", EffectiveAt: "2026-09-20T04:00:00Z", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("10")}, Policy: depositPolicy()}}
	p, err := s.PreviewProductOperation(ctx, create)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.RecordProductOperation(ctx, create, domain.NewProductOperationID().String(), p.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "25"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendAccountCashValue(ctx, aid, "10", "EUR", ""); err != nil {
		t.Fatal(err)
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	kinds := map[string]int{}
	var productItem string
	for _, item := range v.Accounts[0].Items {
		kinds[item.Kind]++
		if item.Kind == "product" {
			productItem = item.ID
		}
	}
	if kinds["scope"] != 1 || kinds["cash"] != 2 || kinds["holding"] != 1 || kinds["product"] != 1 {
		t.Fatalf("mixed contents lost: %v", kinds)
	}
	rev := v.Accounts[0].FinancialRevision
	// Ordinary manual quotes affect valuation, not a cash/quantity attestation.
	if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, "30", "", false); err != nil {
		t.Fatal(err)
	}
	current, err := s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Accounts[0].FinancialRevision != rev {
		t.Fatal("quote invalidated financial evidence")
	}
	product, err := s.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	link := &domain.ReconciliationLink{SessionID: v.ID, AccountID: aid.String(), ItemID: productItem, ExpectedRevision: v.Revision, FinancialRevision: rev}
	terms := UpdateProductTermsInput{ReconciliationLink: link, ProductID: product.Contract.ID, ExpectedRevision: product.Contract.Revision, Terms: create.Open.Terms, Policy: create.Open.Policy}
	terms.Terms.Name = "Updated synthetic deposit"
	preview, token, err := s.PreviewProductTermsGuarded(ctx, terms)
	if err != nil {
		t.Fatal(err)
	}
	mutation := domain.NewProductOperationID().String()
	saved, err := s.CommitProductTermsGuarded(ctx, terms, mutation, preview.ReviewedStateHash, token)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CommitProductTermsGuarded(ctx, terms, mutation, preview.ReviewedStateHash, "expired")
	if err != nil || !replay.Replayed || replay.Contract.ID != saved.Contract.ID {
		t.Fatalf("product terms receipt %+v %v", replay, err)
	}
	links, err := s.ReconciliationLinkedChanges(ctx, v.ID)
	if err != nil || len(links) != 1 || links[0].Family != "product_terms" {
		t.Fatalf("terms link %+v %v", links, err)
	}
	current, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Counts.Confirmed != 0 || current.Accounts[0].FinancialRevision == rev {
		t.Fatal("terms did not require renewed review")
	}
}

func TestReconciliationMissingPositionsAndManagedProductUseExistingAtomicOperations(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	aid := seedHoldingsCash(t, s, ctx, "10000")
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Missing synthetic fund", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	link := &domain.ReconciliationLink{SessionID: v.ID, AccountID: aid.String(), ItemID: "scope", ExpectedRevision: v.Revision, FinancialRevision: v.Accounts[0].FinancialRevision}
	h, _ := s.requireHousehold(ctx)
	quantity, _ := domain.ParseQuantity("2")
	cost, _ := domain.ParseUnitPrice("25")
	command := domain.PositionImportInput{HouseholdID: h.ID, AccountID: aid, InstrumentID: instrument.ID, Quantity: quantity, UnitCost: &cost, Currency: domain.CurrencyCode("USD"), EffectiveAt: s.clock()}
	preview, token, err := s.PreviewChangeGuarded(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateReconciliationCommandLink(ctx, link, command, preview); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChangeGuarded(domain.WithReconciliationLink(ctx, link), command, domain.NewMutationID().String(), "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", token); err != nil {
		t.Fatal(err)
	}
	assertCash(t, s, ctx, aid, "10000")
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	link.ExpectedRevision = v.Revision
	link.FinancialRevision = v.Accounts[0].FinancialRevision
	productCommand := ProductCommand{ReconciliationLink: link, Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{AccountID: aid.String(), Currency: "USD", Principal: "1000", TotalCostBasis: "1000", CurrentValue: "1000", CashExcludesProduct: true, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Missing synthetic deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("10")}, Policy: depositPolicy()}}
	p, token, err := s.PreviewProductOperationGuarded(ctx, productCommand)
	if err != nil {
		t.Fatal(err)
	}
	var frozen ProductCommand
	if err = json.Unmarshal([]byte(p.NormalizedJSON), &frozen); err != nil {
		t.Fatal(err)
	}
	r, err := s.RecordProductOperationGuarded(ctx, frozen, domain.NewProductOperationID().String(), p.ReviewedStateHash, token)
	if err != nil {
		t.Fatal(err)
	}
	assertCash(t, s, ctx, aid, "10000")
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if v.Counts.Confirmed != 0 {
		t.Fatal("new position/product auto-confirmed")
	}
	link.ExpectedRevision = v.Revision
	link.FinancialRevision = v.Accounts[0].FinancialRevision
	link.ItemID = "product:" + r.ProductIDs[0].String()
	renewal := ProductCommand{ReconciliationLink: link, Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{Settle: SettleProductCommand{ProductID: r.ProductIDs[0].String(), ReturnedPrincipal: strPtr("1000"), Interest: strPtr("10"), Fee: strPtr("0"), EffectiveAt: "2026-09-20T04:00:00Z"}, Principal: "1000", OpeningFee: strPtr("0"), Terms: ProductTermsInput{Kind: "term_deposit", Name: "Renewed synthetic deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	p, token, err = s.PreviewProductOperationGuarded(ctx, renewal)
	if err != nil {
		t.Fatal(err)
	}
	frozen = ProductCommand{}
	if err = json.Unmarshal([]byte(p.NormalizedJSON), &frozen); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordProductOperationGuarded(ctx, frozen, domain.NewProductOperationID().String(), p.ReviewedStateHash, token); err != nil {
		t.Fatal(err)
	}
	links, err := s.ReconciliationLinkedChanges(ctx, v.ID)
	if err != nil || len(links) < 3 {
		t.Fatalf("explicit links: %+v %v", links, err)
	}
}

func TestReconciliationDatabaseReopenAndCompletedEvidenceVerification(t *testing.T) {
	s, db, aid := reconciliationFixture(t)
	ctx := context.Background()
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	v = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: v.ID, ExpectedRevision: v.Revision})
	path := db.Path
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reopened.Close() })
	next := NewService(sqlite.NewRepository(reopened))
	next.SetClock(s.clock)
	current, err := next.GetReconciliationSession(ctx, v.ID)
	if err != nil || current.Status != "paused" {
		t.Fatalf("persisted pause: %+v %v", current, err)
	}
	current = recAction(t, next, ReconciliationAction{Action: "resume", SessionID: v.ID, ExpectedRevision: current.Revision})
	for _, item := range current.Accounts[0].Items {
		current = recAction(t, next, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: current.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: current.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic reopened statement", AsOfDate: "2026-09-01"}})
	}
	current = recAction(t, next, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: current.Revision})
	if err := reopened.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatal(err)
	}
	finalDB, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer finalDB.Close()
	last, err := NewService(sqlite.NewRepository(finalDB)).ReconciliationLastSuccess(ctx, aid)
	if err != nil || last == nil || last.ID != current.ID || last.CompletedAt != current.CompletedAt {
		t.Fatalf("immutable success after reopen: %+v %v", last, err)
	}
}
