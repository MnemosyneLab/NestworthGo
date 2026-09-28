package application

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestLiquidityReviewUnknownSettlementPreserved(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	policy := depositPolicy()
	policy.SettlementDays = nil
	policy.DayBasis = nil
	got, e := s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: p.Contract.Name, StartOn: p.Contract.StartOn, MaturityOn: p.Contract.MaturityOn, InterestMode: "none"}, Policy: policy})
	if e != nil {
		t.Fatal(e)
	}
	if got.Policy.SettlementDays != nil {
		t.Fatalf("unknown settlement saved as %d", *got.Policy.SettlementDays)
	}
}
func TestLiquidityReviewRejectInconsistentLock(t *testing.T) {
	s, ctx, a, _ := openRegressionProduct(t)
	policy := depositPolicy()
	policy.UnlockOn = strPtr("2027-01-20")
	c := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "USD", Principal: "1000", Terms: ProductTermsInput{Kind: "locked_product", Name: "Locked", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: policy}}
	if _, err := s.PreviewProductOperation(ctx, c); err == nil {
		t.Fatal("preview accepted normal maturity before mandatory unlock")
	}
}

func TestLiquidityReviewUndoPreviousAfterUndoInterest(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	interest := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: r.ProductIDs[0].String(), Amount: "100"}})
	s.setClock(func() time.Time { return time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC) })
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: interest.OperationID.String()}})
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: r.OperationID.String()}})
	product, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if product.Contract.State != domain.ProductStateCancelled {
		t.Fatal("opening was not cancelled")
	}
	path := filepath.Join(t.TempDir(), "undo-chain.db")
	if err := s.SnapshotTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	db, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
func TestLiquidityReviewRejectPolicyBeforeEligibility(t *testing.T) {
	s, ctx, _, r := openRegressionProduct(t)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	policy := depositPolicy()
	policy.ReceiptOnOverride = strPtr("2026-11-20")
	_, e = s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: p.Contract.Name, StartOn: p.Contract.StartOn, MaturityOn: p.Contract.MaturityOn, InterestMode: "none"}, Policy: policy})
	if e == nil {
		t.Fatal("receipt before maturity saved; valid funds become unknown instead of rejecting invalid rule")
	}
}
func TestLiquidityReviewBackupRequiresProductEvidence(t *testing.T) {
	s, ctx, _, _ := openRegressionProduct(t)
	path := filepath.Join(t.TempDir(), "db")
	if e := s.SnapshotTo(ctx, path); e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.SQL.ExecContext(ctx, `DELETE FROM product_operation_activities`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = sqlite.OpenReadOnlyForVerify(path)
	if db != nil {
		db.Close()
	}
	if e == nil {
		t.Fatal("backup accepted product with financial child links missing; generic child undo no longer guarded")
	}
}
func TestLiquidityReviewBackupRequiresProductPolicy(t *testing.T) {
	s, ctx, _, _ := openRegressionProduct(t)
	path := filepath.Join(t.TempDir(), "db")
	if e := s.SnapshotTo(ctx, path); e != nil {
		t.Fatal(e)
	}
	db, e := sqlite.Open(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.SQL.ExecContext(ctx, `DELETE FROM liquidity_policies`); e != nil {
		t.Fatal(e)
	}
	db.Close()
	db, e = sqlite.OpenReadOnlyForVerify(path)
	if db != nil {
		db.Close()
	}
	if e == nil {
		t.Fatal("backup accepted managed product without mandatory policy")
	}
}

func TestLiquidityReviewUndoDoesNotCrossContractEdit(t *testing.T) {
	for _, when := range []string{"before_income", "after_reversal"} {
		t.Run(when, func(t *testing.T) {
			s, ctx, _, r := openRegressionProduct(t)
			edit := func() {
				p, err := s.Product(ctx, r.ProductIDs[0])
				if err != nil {
					t.Fatal(err)
				}
				_, err = s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: p.Contract.ID, ExpectedRevision: p.Contract.Revision, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Edited", StartOn: p.Contract.StartOn, MaturityOn: p.Contract.MaturityOn, InterestMode: "none"}, Policy: depositPolicy()})
				if err != nil {
					t.Fatal(err)
				}
			}
			if when == "before_income" {
				edit()
			}
			income := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: r.ProductIDs[0].String(), Amount: "100"}})
			s.setClock(func() time.Time { return time.Date(2026, 9, 20, 6, 0, 0, 0, time.UTC) })
			recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: income.OperationID.String()}})
			if when == "after_reversal" {
				edit()
			}
			_, err := s.PreviewProductOperation(ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: r.OperationID.String()}})
			if err == nil {
				t.Fatal("contract edit was bypassed through undo chain")
			}
		})
	}
}

func TestLiquidityReviewDepositRejectsMismatchedUnlock(t *testing.T) {
	s, ctx, account, _ := openRegressionProduct(t)
	policy := depositPolicyForMaturity("2026-11-20")
	_, err := s.PreviewProductOperation(ctx, ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "1000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Invalid date", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: policy}})
	if err == nil {
		t.Fatal("deposit accepted an unlock date different from maturity")
	}
}

func TestLiquidityReviewMultipleUndoRestoresOriginalCash(t *testing.T) {
	s, ctx, account, opened := openRegressionProduct(t)
	var incomes []ProductOperationReceipt
	for i := 0; i < 3; i++ {
		s.setClock(func() time.Time { return time.Date(2026, 9, 20, 5+i, 0, 0, 0, time.UTC) })
		incomes = append(incomes, recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: opened.ProductIDs[0].String(), Amount: "100"}}))
	}
	for i := len(incomes) - 1; i >= 0; i-- {
		s.setClock(func() time.Time { return time.Date(2026, 9, 20, 12-i, 0, 0, 0, time.UTC) })
		recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: incomes[i].OperationID.String()}})
	}
	s.setClock(func() time.Time { return time.Date(2026, 9, 20, 15, 0, 0, 0, time.UTC) })
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: opened.OperationID.String()}})
	assertCash(t, s, ctx, account, "150000")
}
