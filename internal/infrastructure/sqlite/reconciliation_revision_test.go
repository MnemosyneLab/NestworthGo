package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
)

func TestReconciliationProductValuationPreservesChecksInFreshAndUpgradedSchemas(t *testing.T) {
	for _, upgraded := range []bool{false, true} {
		name := "fresh"
		if upgraded {
			name = "v16-upgrade"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "synthetic.db")
			if upgraded {
				path = syntheticV16(t)
			}
			db, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			s := application.NewService(NewRepository(db))
			now := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
			s.SetClock(func() time.Time { return now })
			ctx := t.Context()
			if err := s.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Tester"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := s.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			account, err := s.CreateAccount(ctx, application.AccountInput{Name: "Synthetic holdings", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			aid := account.Account.ID
			if _, err := s.AppendAccountCashValue(ctx, aid, "100", "USD", ""); err != nil {
				t.Fatal(err)
			}
			maturity, zero, calendar := "2026-12-20", 0, "calendar"
			terms := application.ProductTermsInput{Kind: "locked_product", Name: "Synthetic locked product", StartOn: "2026-09-20", MaturityOn: &maturity, InterestMode: "none"}
			policy := application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar}
			command := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{AccountID: aid.String(), Currency: "USD", Principal: "1000", TotalCostBasis: "1000", CurrentValue: "1000", CashExcludesProduct: true, Terms: terms, Policy: policy}}
			preview, err := s.PreviewProductOperation(ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			product, err := s.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
			if err != nil {
				t.Fatal(err)
			}
			act := func(action application.ReconciliationAction) domain.ReconciliationSession {
				t.Helper()
				p, err := s.PreviewReconciliationSession(ctx, action)
				if err != nil {
					t.Fatal(err)
				}
				v, err := s.CommitReconciliationSession(ctx, p)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}
			session := act(application.ReconciliationAction{Action: "start", AccountIDs: []string{aid.String()}})
			for _, item := range session.Accounts[0].Items {
				session = act(application.ReconciliationAction{Action: "check", SessionID: session.ID, ExpectedRevision: session.Revision, AccountID: aid.String(), ItemID: item.ID, FinancialRevision: session.Accounts[0].FinancialRevision, Outcome: "confirmed", Source: domain.ReconciliationSource{Reference: "Synthetic statement", AsOfDate: "2026-09-20"}})
			}
			finish, err := s.PreviewReconciliationSession(ctx, application.ReconciliationAction{Action: "finish", SessionID: session.ID, ExpectedRevision: session.Revision})
			if err != nil {
				t.Fatal(err)
			}
			valuation, token, err := s.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ProductID: product.ProductIDs[0], Amount: "1050"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.CommitProductValuationGuarded(ctx, valuation.Command, domain.NewProductOperationID().String(), valuation.ReviewedStateHash, token); err != nil {
				t.Fatal(err)
			}
			current, err := s.GetReconciliationSession(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Accounts[0].FinancialRevision != session.Accounts[0].FinancialRevision || current.Accounts[0].RecheckRequired || current.Counts != session.Counts {
				t.Fatalf("valuation invalidated checks: revision %d→%d, counts %+v→%+v, recheck=%v", session.Accounts[0].FinancialRevision, current.Accounts[0].FinancialRevision, session.Counts, current.Counts, current.Accounts[0].RecheckRequired)
			}
			var valuationChanges int
			if err := db.SQL.QueryRow(`SELECT count(*) FROM reconciliation_financial_changes WHERE account_id=? AND family='product_operation' AND json_extract(evidence,'$.kind')='value_observation'`, aid.String()).Scan(&valuationChanges); err != nil || valuationChanges != 0 {
				t.Fatalf("valuation recorded as a financial change: %d %v", valuationChanges, err)
			}
			completed, err := s.CommitReconciliationSession(ctx, finish)
			if err != nil || completed.Partial {
				t.Fatalf("valuation invalidated unchanged finish preview: %+v %v", completed, err)
			}
			// Terms and actual cash changes still advance the financial revision.
			detail, err := s.Product(ctx, product.ProductIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			terms.Name = "Updated synthetic terms"
			input := application.UpdateProductTermsInput{ProductID: detail.Contract.ID, ExpectedRevision: detail.Contract.Revision, Terms: terms, Policy: policy}
			tp, token, err := s.PreviewProductTermsGuarded(ctx, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.CommitProductTermsGuarded(ctx, input, domain.NewProductOperationID().String(), tp.ReviewedStateHash, token); err != nil {
				t.Fatal(err)
			}
			current, err = s.GetReconciliationSession(ctx, session.ID)
			if err != nil || !current.Accounts[0].RecheckRequired || current.Accounts[0].FinancialRevision != completed.Accounts[0].FinancialRevision {
				t.Fatalf("terms failed to flag frozen evidence for recheck: %+v %v", current, err)
			}
			repo := NewRepository(db)
			before, err := repo.ReconciliationRevisions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.AppendAccountCashValue(ctx, aid, "110", "USD", ""); err != nil {
				t.Fatal(err)
			}
			after, err := repo.ReconciliationRevisions(ctx)
			if err != nil || after[aid.String()] <= before[aid.String()] {
				t.Fatalf("cash failed to advance revision: before=%v after=%v err=%v", before, after, err)
			}
			if err := db.Verify(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}
