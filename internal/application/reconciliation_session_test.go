package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
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
	for _, revision := range []int64{v.Revision, v.Revision - 1} {
		_, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: revision})
		var domainErr *domain.Error
		if !errors.As(err, &domainErr) || domainErr.Code != domain.ErrConflict || domainErr.Field != "expectedRevision" || !strings.Contains(domainErr.Message, "completed and read-only") {
			t.Fatalf("completed session refusal: %v", err)
		}
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
	household, _ := s.requireHousehold(ctx)
	if _, err := s.RecordChange(domain.WithReconciliationLink(ctx, link), domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: id, Amount: target, Reason: domain.ReasonReconciliation, EffectiveAt: s.clock()}); err == nil {
		t.Fatal("linked write without mutation identity accepted")
	}

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
	terms.Terms.MaturityInterest = strPtr("15")
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
	headers, err := next.ListReconciliationSessions(ctx)
	if err != nil || len(headers) != 1 || len(headers[0].AccountIDs) != 1 || headers[0].AccountIDs[0] != aid || headers[0].AccountNames[0] != "Synthetic bank" {
		t.Fatalf("persisted scope header: %+v %v", headers, err)
	}
	current, err := next.GetReconciliationSession(ctx, v.ID)
	if err != nil || current.Status != "paused" {
		t.Fatalf("persisted pause: %+v %v", current, err)
	}
	current = recAction(t, next, ReconciliationAction{Action: "resume", SessionID: v.ID, ExpectedRevision: current.Revision})
	for _, item := range current.Accounts[0].Items {
		current = recAction(t, next, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: current.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: current.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic reopened statement", AsOfDate: "2026-09-01"}})
	}
	current = recAction(t, next, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: current.Revision})
	if current.Accounts[0].LastSuccessfulAt != current.CompletedAt {
		t.Fatal("completed account did not advance its successful time")
	}
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

func TestReconciliationCompletionOrderingPreservesFractionalSeconds(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	ctx := context.Background()
	now := s.clock()
	s.SetClock(func() time.Time { return now })
	var ids []string
	for i := 0; i < 2; i++ {
		if i == 1 {
			now = now.Add(100 * time.Millisecond)
		}
		v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
		ids = append(ids, v.ID)
		for _, item := range v.Accounts[0].Items {
			v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic precise statement", AsOfDate: "2026-09-01"}})
		}
		recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision})
	}
	rows, err := s.ListReconciliationSessions(ctx)
	if err != nil || rows[0].ID != ids[1] {
		t.Fatalf("start ordering: %+v %v", rows, err)
	}
	last, err := s.ReconciliationLastSuccess(ctx, aid)
	if err != nil || last == nil || last.ID != ids[1] {
		t.Fatalf("completion ordering: %+v %v", last, err)
	}
}

func TestReconciliationRemovedItemsHaveStablePreviewAndCommitOrder(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	aid := seedHoldingsCash(t, s, ctx, "1000")
	var holdings []domain.HoldingID
	for _, name := range []string{"Synthetic first ETF", "Synthetic second ETF"} {
		instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "0"})
		if err != nil {
			t.Fatal(err)
		}
		holdings = append(holdings, holding.ID)
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	for _, item := range v.Accounts[0].Items {
		v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid.String(), ItemID: item.ID, FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic holdings statement", AsOfDate: "2026-09-20"}})
	}
	for _, id := range holdings {
		if err := s.ArchiveHolding(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}
	// Repeated refreshes must use the same canonical order as stored evidence.
	for i := 0; i < 20; i++ {
		current, err := s.GetReconciliationSession(ctx, v.ID)
		if err != nil {
			t.Fatal(err)
		}
		items := current.Accounts[0].Items
		if !sort.SliceIsSorted(items, func(i, j int) bool { return items[i].ID < items[j].ID }) {
			t.Fatalf("refreshed items are not in stable ID order: %+v", items)
		}
		removed := 0
		for _, item := range items {
			if item.Kind == "removed" {
				removed++
				if item.Outcome != "pending" || item.CheckedAt != "" || !item.RecheckRequired || len(item.Attempts) != 1 || item.FinancialRevision != current.Accounts[0].FinancialRevision || item.Attempts[0].FinancialRevision >= item.FinancialRevision {
					t.Fatalf("removed evidence changed semantics: %+v", item)
				}
			}
		}
		if removed != 2 {
			t.Fatalf("removed holdings = %d, want 2", removed)
		}
	}
	v = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: v.ID, ExpectedRevision: v.Revision})
	if v.Status != "paused" || v.Counts.Pending != 4 {
		t.Fatalf("unchanged preview did not commit refreshed evidence: %+v", v)
	}
}

func TestReconciliationExpiredPreviewRefusalAndReceiptFirstRecovery(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	ctx := context.Background()
	plan, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	if err != nil {
		t.Fatal(err)
	}
	plan.ExpiresAt = time.Now().Add(-time.Minute)
	_, err = s.CommitReconciliationSession(ctx, plan)
	var refusal *domain.Error
	if !errors.As(err, &refusal) || refusal.Code != domain.ErrStalePreview || refusal.Field != "expiresAt" {
		t.Fatalf("expired uncommitted preview did not give a definite refusal: %v", err)
	}
	rows, err := s.ListReconciliationSessions(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("expired preview wrote a session: %+v %v", rows, err)
	}
	// Model a durable commit made before this saved plan expired, without a
	// wall-clock sleep: the original hash and outcome are already persisted.
	raw, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	repo, err := s.reconciliationRepo()
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveReconciliation(ctx, plan.Result, 0, plan.Revisions, plan.ID, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
	recovered, err := s.CommitReconciliationSession(ctx, plan)
	if err != nil || recovered.ID != plan.Result.ID || recovered.Revision != plan.Result.Revision {
		t.Fatalf("expiry blocked original durable outcome: %+v %v", recovered, err)
	}
}

// Override only cost reads; reconciliation transactions stay on the isolated SQLite repository.
type reconciliationCostFault struct {
	*sqlite.Repository
	eventsErr, startingErr error
	overrideStarting       bool
	events                 []domain.CostBasisEvent
	overrideEvents         bool
}

func (r *reconciliationCostFault) ListCostBasisEvents(ctx context.Context, id domain.HoldingID, filter domain.CostBasisReadFilter) ([]domain.CostBasisEvent, error) {
	if r.eventsErr != nil || r.overrideEvents {
		return r.events, r.eventsErr
	}
	return r.Repository.ListCostBasisEvents(ctx, id, filter)
}
func (r *reconciliationCostFault) StartingPointCost(ctx context.Context, id domain.HoldingID, filter domain.CostBasisReadFilter) (*domain.UnitPrice, error) {
	if r.startingErr != nil || r.overrideStarting {
		return nil, r.startingErr
	}
	return r.Repository.StartingPointCost(ctx, id, filter)
}
func TestReconciliationCostReadFailuresNeverPersist(t *testing.T) {
	for _, kind := range []string{"events", "starting", "integrity", "invalid_history"} {
		t.Run(kind, func(t *testing.T) {
			s, db, _ := reconciliationFixture(t)
			ctx := context.Background()
			aid := seedHoldingsCash(t, s, ctx, "1")
			instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "0"}); err != nil {
				t.Fatal(err)
			}
			plan, err := s.PreviewReconciliationSession(ctx, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
			if err != nil {
				t.Fatal(err)
			}
			fault := &reconciliationCostFault{Repository: sqlite.NewRepository(db)}
			injected := errors.New("synthetic cost repository failure")
			switch kind {
			case "events":
				fault.eventsErr = injected
			case "starting":
				fault.startingErr = injected
			case "integrity":
				fault.eventsErr = &domain.Error{Code: domain.ErrIntegrity, Message: "synthetic invalid cost history"}
			case "invalid_history":
				fault.overrideEvents = true
				quantity, _ := domain.ParseQuantity("2")
				zero, _ := domain.ParseUnitPrice("0")
				fault.events = []domain.CostBasisEvent{{Kind: domain.CostBasisStartingPoint, Quantity: quantity, UnitCost: &zero}, {Kind: domain.CostBasisStartingPoint, Quantity: quantity, UnitCost: &zero}}
			}
			s.repository = fault
			if _, err = s.PreviewReconciliationSession(ctx, plan.Action); err == nil {
				t.Fatal("cost failure ignored by preview")
			}
			if _, err = s.CommitReconciliationSession(ctx, plan); err == nil {
				t.Fatal("cost failure ignored by commit revalidation")
			}
			for _, table := range []string{"reconciliation_sessions", "reconciliation_receipts"} {
				var count int
				if err = db.SQL.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("%s writes: %d %v", table, count, err)
				}
			}
		})
	}
}
func TestReconciliationKnownZeroAndUnknownCost(t *testing.T) {
	s, db, _ := reconciliationFixture(t)
	ctx := context.Background()
	aid := seedHoldingsCash(t, s, ctx, "1")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic zero cost", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "0"})
	if err != nil {
		t.Fatal(err)
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	action := ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid.String(), ItemID: "holding:" + holding.ID.String(), FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic statement", AsOfDate: "2026-09-01"}}
	checked := recAction(t, s, action)
	for _, item := range checked.Accounts[0].Items {
		if item.Kind == "holding" && (item.Cost != "0" || item.Outcome != "confirmed") {
			t.Fatalf("zero cost rejected: %+v", item)
		}
	}
	quantity, _ := domain.ParseQuantity("2")
	for _, events := range [][]domain.CostBasisEvent{nil, {{Kind: domain.CostBasisStartingPoint, Quantity: quantity}}} {
		s.repository = &reconciliationCostFault{Repository: sqlite.NewRepository(db), overrideEvents: true, events: events}
		// No starting cost and no incoming cost facts: this is unavailable, never known zero.
		fault := s.repository.(*reconciliationCostFault)
		fault.overrideStarting = true
		fresh := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
		action.SessionID, action.ExpectedRevision = fresh.ID, fresh.Revision
		if _, err := s.PreviewReconciliationSession(ctx, action); err == nil {
			t.Fatal("unknown cost confirmed")
		}
	}
}
func TestReconciliationUnchangedRebuildPreservesEvidence(t *testing.T) {
	s, db, balance := reconciliationFixture(t)
	ctx := context.Background()
	cash := seedHoldingsCash(t, s, ctx, "20")
	for _, id := range []string{balance, cash.String()} {
		aid, _ := domain.ParseAccountID(id)
		household, _ := s.requireHousehold(ctx)
		amount, _ := domain.ParseMoney("1", "USD")
		if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: household.ID, AccountID: aid, Amount: amount, EffectiveAt: s.clock(), Reason: domain.ReasonReconciliation}); err != nil {
			t.Fatal(err)
		}
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{balance, cash.String()}})
	for _, account := range v.Accounts {
		for _, item := range account.Items {
			v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: account.AccountID, ItemID: item.ID, FinancialRevision: account.FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic", AsOfDate: "2026-09-01"}})
		}
	}
	completed := recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision})
	repo := sqlite.NewRepository(db)
	before, err := repo.ReconciliationRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	readChanges := func() map[string][]domain.ReconciliationFinancialChange {
		rows := map[string][]domain.ReconciliationFinancialChange{}
		for _, id := range []string{balance, cash.String()} {
			changes, err := repo.ReconciliationFinancialChanges(ctx, id, 0)
			if err != nil {
				t.Fatal(err)
			}
			rows[id] = changes
		}
		return rows
	}
	changes := readChanges()
	if len(changes[balance]) == 0 || len(changes[cash.String()]) == 0 {
		t.Fatal("fixture needs authoritative change evidence")
	}
	for i := 0; i < 2; i++ {
		if _, err := s.RebuildDerivedData(ctx); err != nil {
			t.Fatal(err)
		}
		after, err := repo.ReconciliationRevisions(ctx)
		if err != nil || !reflect.DeepEqual(before, after) {
			t.Fatalf("replay changed revisions: %v -> %v: %v", before, after, err)
		}
		current, err := s.GetReconciliationSession(ctx, completed.ID)
		if err != nil || !reflect.DeepEqual(completed, current) {
			t.Fatalf("replay changed outcome: %+v %v", current, err)
		}
		if !reflect.DeepEqual(changes, readChanges()) {
			t.Fatal("replay added financial changes")
		}
	}
	aid, _ := domain.ParseAccountID(balance)
	h, _ := s.requireHousehold(ctx)
	money, _ := domain.ParseMoney("1", "USD")
	if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: h.ID, AccountID: aid, Amount: money, EffectiveAt: s.clock(), Reason: domain.ReasonReconciliation}); err != nil {
		t.Fatal(err)
	}
	after, _ := repo.ReconciliationRevisions(ctx)
	if after[balance] <= before[balance] || after[cash.String()] != before[cash.String()] {
		t.Fatalf("real correction revisions: %v", after)
	}
	current, err := s.GetReconciliationSession(ctx, completed.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, account := range current.Accounts {
		if account.RecheckRequired != (account.AccountID == balance) {
			t.Fatal("incorrect correction recheck", account)
		}
	}
}
func TestReconciliationTextUTF8BoundsIdentifyField(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	for _, text := range []string{"a", "中", "😀"} {
		for _, field := range []struct {
			name  string
			limit int
		}{{"note", 2000}, {"referenceValue", 256}, {"source.reference", 512}} {
			action := ReconciliationAction{Action: "start", AccountIDs: []string{aid}}
			set := func(value string) {
				switch field.name {
				case "note":
					action.Note = value
				case "referenceValue":
					action.ReferenceValue = value
				default:
					action.Source.Reference = value
				}
			}
			set(strings.Repeat(text, field.limit/len(text)))
			if _, err := s.PreviewReconciliationSession(context.Background(), action); err != nil {
				t.Fatal(err)
			}
			set(strings.Repeat(text, field.limit/len(text)+1))
			_, err := s.PreviewReconciliationSession(context.Background(), action)
			var de *domain.Error
			if !errors.As(err, &de) || de.Field != field.name {
				t.Fatalf("%s: %v", field.name, err)
			}
		}
	}
}

func TestReconciliationArchiveGuardAndLegacyVisibility(t *testing.T) {
	s, db, _ := reconciliationFixture(t)
	ctx := t.Context()
	aid := seedHoldingsCash(t, s, ctx, "100")
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "Synthetic archived ETF", Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: instrument.ID.String(), Quantity: "2", UnitCost: "10"})
	if err != nil {
		t.Fatal(err)
	}
	repo := sqlite.NewRepository(db)
	b, _ := s.Bootstrap(ctx)
	if err := repo.SetInstrumentArchive(ctx, b.Household.ID, instrument.ID, true, s.clock()); err == nil {
		t.Fatal("instrument with nonzero holdings archived")
	}
	if err := repo.SetHoldingArchive(ctx, b.Household.ID, holding.ID, true, s.clock()); err == nil {
		t.Fatal("nonzero holding archived")
	}
	// Simulate an existing legacy database, bypassing the repository write guard.
	if _, err := db.SQL.Exec("UPDATE instruments SET archived_at=? WHERE id=?", s.clock().Format(time.RFC3339), instrument.ID.String()); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SQL.Exec("UPDATE holdings SET archived_at=? WHERE id=?", s.clock().Format(time.RFC3339), holding.ID.String()); err != nil {
		t.Fatal(err)
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	for _, item := range v.Accounts[0].Items {
		if item.HoldingID == holding.ID.String() {
			if item.Value != "2" || !item.ArchivedNonzero {
				t.Fatalf("missing archive warning: %+v", item)
			}
			return
		}
	}
	t.Fatal("legacy nonzero position omitted")
}

func TestReconciliationNotesReferenceBasisAndFrozenLaterChanges(t *testing.T) {
	s, _, aid := reconciliationFixture(t)
	ctx := t.Context()
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}, Note: "Keep opening evidence"})
	check := ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: "balance", FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic statement", AsOfDate: "2026-09-01"}, ReferenceValue: "99", ReferenceComparable: true}
	if _, err := s.PreviewReconciliationSession(ctx, check); err == nil {
		t.Fatal("comparable unexplained mismatch confirmed")
	}
	check.Note = "One unit is still settling"
	v = recAction(t, s, check)
	check.ExpectedRevision = v.Revision
	check.ReferenceValue = "100.0000"
	check.Note = ""
	v = recAction(t, s, check)
	check.ExpectedRevision = v.Revision
	check.Source.AsOfDate = "2026-08-01"
	if _, err := s.PreviewReconciliationSession(ctx, check); err == nil {
		t.Fatal("old statement compared to current checkpoint")
	}
	check.ReferenceComparable = false
	check.ReferenceValue = "statement text"
	v = recAction(t, s, check)
	v = recAction(t, s, ReconciliationAction{Action: "check", SessionID: v.ID, ExpectedRevision: v.Revision, AccountID: aid, ItemID: "scope", FinancialRevision: v.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic contents", AsOfDate: "2026-09-01"}})
	v = recAction(t, s, ReconciliationAction{Action: "pause", SessionID: v.ID, ExpectedRevision: v.Revision})
	completed := recAction(t, s, ReconciliationAction{Action: "finish", SessionID: v.ID, ExpectedRevision: v.Revision})
	if completed.Note != "Keep opening evidence" || completed.ClosingNote != "" {
		t.Fatalf("blank finish erased opening: %+v", completed)
	}
	id, _ := domain.ParseAccountID(aid)
	money, _ := domain.ParseMoney("1", domain.CurrencyCode("USD"))
	h, _ := s.requireHousehold(ctx)
	if _, err := s.RecordChange(ctx, domain.MoneyAddedInput{HouseholdID: h.ID, AccountID: id, Amount: money, Reason: domain.ReasonIncome, EffectiveAt: s.clock()}); err != nil {
		t.Fatal(err)
	}
	last, err := s.ReconciliationLastSuccess(ctx, aid)
	if err != nil || last.CompletedAt != completed.CompletedAt || !last.Accounts[0].RecheckRequired {
		t.Fatalf("historical success lost: %+v %v", last, err)
	}
	changes, err := s.ReconciliationChanges(ctx, v.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) == 0 {
		t.Fatal("later changes omitted from first completed baseline")
	}
	for _, change := range changes {
		if !change.AfterCompletion || change.Revision <= completed.Accounts[0].FinancialRevision {
			t.Fatal("later change included in original frozen interval")
		}
	}
	next := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid}})
	if next.Accounts[0].PriorSuccessfulAt == "" {
		t.Fatal("per-account baseline missing")
	}
	exported, err := s.ExportJSONBytes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"closingNote", "accountFinancialRevisions", "reconciliationFinancialChanges"} {
		if !strings.Contains(string(exported), key) {
			t.Fatalf("export missing %s", key)
		}
	}
	for _, key := range []string{"reconciliation.desktop.", "reconciliation_receipts", "mcp.plan."} {
		if strings.Contains(string(exported), key) {
			t.Fatalf("private recovery material exported: %s", key)
		}
	}
}

func TestReconciliationConcreteHoldingAndAtomicCompanion(t *testing.T) {
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	aid := seedHoldingsCash(t, s, ctx, "1000")
	var holdings []domain.Holding
	for _, name := range []string{"Reviewed ETF", "Other ETF"} {
		i, err := s.CreateInstrument(ctx, InstrumentInput{Name: name, Type: "etf", QuoteCurrency: "USD", QuoteSource: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		h, err := s.CreateHolding(ctx, HoldingInput{AccountID: aid.String(), InstrumentID: i.ID.String(), Quantity: "2", UnitCost: "10"})
		if err != nil {
			t.Fatal(err)
		}
		holdings = append(holdings, h)
	}
	v := recAction(t, s, ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
	link := &domain.ReconciliationLink{SessionID: v.ID, AccountID: aid.String(), ItemID: "holding:" + holdings[0].ID.String(), ExpectedRevision: v.Revision, FinancialRevision: v.Accounts[0].FinancialRevision}
	h, _ := s.requireHousehold(ctx)
	money, _ := domain.ParseMoney("1", domain.CurrencyCode("USD"))
	q, _ := domain.ParseQuantity("1")
	cash := domain.MoneyAddedInput{HouseholdID: h.ID, AccountID: aid, Amount: money, Reason: domain.ReasonIncome, EffectiveAt: s.clock()}
	wrong := domain.PositionAdjustmentInput{HouseholdID: h.ID, HoldingID: holdings[1].ID, Quantity: q, Added: false, EffectiveAt: s.clock()}
	for _, commands := range [][]any{{cash}, {wrong}} {
		previews, token, err := s.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ValidateReconciliationLink(ctx, link, previews); err == nil {
			t.Fatal("wrong concrete target preview accepted")
		}
		if _, err = s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, domain.NewMutationID().String(), strings.Repeat("a", 64), token); err == nil {
			t.Fatal("wrong target committed")
		}
	}
	right := wrong
	right.HoldingID = holdings[0].ID
	commands := []any{cash, right}
	previews, token, err := s.PreviewChangesGuardedWith(ctx, func(context.Context) ([]any, error) { return commands, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ValidateReconciliationLink(ctx, link, previews); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordChangesGuarded(domain.WithReconciliationLink(ctx, link), commands, domain.NewMutationID().String(), strings.Repeat("b", 64), token); err != nil {
		t.Fatal(err)
	}
	links, err := s.ReconciliationLinkedChanges(ctx, v.ID)
	if err != nil || len(links) != 2 {
		t.Fatalf("companion links %+v %v", links, err)
	}
	v, err = s.GetReconciliationSession(ctx, v.ID)
	if err != nil || v.Counts.Confirmed != 0 {
		t.Fatal("correction implied attestation")
	}
}
