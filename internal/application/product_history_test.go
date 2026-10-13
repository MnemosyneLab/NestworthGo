package application

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"testing"
	"time"
)

func assertHistoryUndo(t *testing.T, s *Service, ctx context.Context, productID domain.ProductContractID, target domain.ProductOperationID, reason ProductUndoDisabledReason) ProductOperationPage {
	t.Helper()
	page, err := s.ListProductOperations(ctx, productID, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Undo.Operation == nil || page.Undo.Operation.ID != target {
		t.Fatalf("target = %+v, want %s", page.Undo, target)
	}
	allowed := reason == ""
	if page.Undo.Allowed != allowed || (allowed && page.Undo.Reason != nil) || (!allowed && (page.Undo.Reason == nil || *page.Undo.Reason != reason)) {
		t.Fatalf("undo = %+v, want reason %s", page.Undo, reason)
	}
	_, err = s.PreviewProductOperation(ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: target.String()}})
	if (err == nil) != allowed {
		t.Fatalf("history/preview disagreement: %+v, %v", page.Undo, err)
	}
	return page
}

type productHistoryReadCounter struct {
	Repository
	snapshots, independent int
}

func (r *productHistoryReadCounter) ReadProductOperationHistorySnapshot(ctx context.Context, id domain.ProductContractID, limit int, cursor string) (domain.ProductOperationHistorySnapshot, error) {
	r.snapshots++
	return r.Repository.ReadProductOperationHistorySnapshot(ctx, id, limit, cursor)
}
func (r *productHistoryReadCounter) ListProductOperations(ctx context.Context, h domain.HouseholdID, p domain.ProductContractID, n int, c string) ([]domain.ProductOperation, error) {
	r.independent++
	return r.Repository.ListProductOperations(ctx, h, p, n, c)
}
func (r *productHistoryReadCounter) ReadPortfolioSnapshot(ctx context.Context, f domain.AccountFilter) (domain.PortfolioSnapshot, error) {
	r.independent++
	return r.Repository.ReadPortfolioSnapshot(ctx, f)
}
func (r *productHistoryReadCounter) ReadProductUndoEvidence(ctx context.Context, h domain.HouseholdID, op domain.ProductOperationID) (domain.ProductUndoEvidence, error) {
	r.independent++
	return r.Repository.ReadProductUndoEvidence(ctx, h, op)
}

func TestProductHistoryOffPageOpeningAndRestorationBeyond100(t *testing.T) {
	t.Parallel()
	s, ctx, _, opened := openRegressionProduct(t)
	id := opened.ProductIDs[0]
	now := s.clock()
	for i := 0; i < 51; i++ {
		now = now.Add(time.Minute)
		s.setClock(func() time.Time { return now })
		interest := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: id.String(), Amount: "0.0001"}})
		if i == 0 {
			assertHistoryUndo(t, s, ctx, id, interest.OperationID, "")
		}
		now = now.Add(time.Minute)
		recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: interest.OperationID.String()}})
	}
	page := assertHistoryUndo(t, s, ctx, id, opened.OperationID, "")
	if page.Operations[0].ID == opened.OperationID || page.Next == nil {
		t.Fatal("opening must be off-page")
	}
	next, err := s.ListProductOperations(ctx, id, *page.Next, 1)
	if err != nil {
		t.Fatal(err)
	}
	if next.Operations[0].ReversedByOperationID == nil || *next.Operations[0].ReversedByOperationID != page.Operations[0].ID {
		t.Fatal("reverse link must be global")
	}
	counter := &productHistoryReadCounter{Repository: s.repository}
	s.repository = counter
	for _, n := range []int{1, 25, 100} {
		if _, err := s.ListProductOperations(ctx, id, "", n); err != nil {
			t.Fatal(err)
		}
	}
	if counter.snapshots != 3 || counter.independent != 0 {
		t.Fatalf("reads=%+v", counter)
	}
	s.repository = counter.Repository
	detail, err := s.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	terms := ProductTermsInput{Kind: "term_deposit", Name: detail.Contract.Name, StartOn: detail.Contract.StartOn, MaturityOn: detail.Contract.MaturityOn, InterestMode: "none"}
	changed, err := s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: id, ExpectedRevision: detail.Contract.Revision, Terms: terms, Policy: depositPolicy()})
	if err != nil {
		t.Fatal(err)
	}
	// Editing identical values is still a revision edge that receipts cannot restore.
	if changed.Contract.Revision == detail.Contract.Revision {
		t.Fatal("terms edit did not advance revision")
	}
	assertHistoryUndo(t, s, ctx, id, opened.OperationID, UndoContractChanged)
}

func TestProductHistoryReadAndFinalRevalidation(t *testing.T) {
	t.Parallel()
	s, ctx, account, opened := openRegressionProduct(t)
	id := opened.ProductIDs[0]
	assertHistoryUndo(t, s, ctx, id, opened.OperationID, "")
	detail, err := s.Product(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	reserve, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: domain.HoldingSourceRef(account, detail.Contract.HoldingID), Label: "Synthetic reserve", Amount: "1"})
	if err != nil {
		t.Fatal(err)
	}
	assertHistoryUndo(t, s, ctx, id, opened.OperationID, UndoActiveReservation)
	settled := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: id.String(), ReturnedPrincipal: strPtr("100000"), ReleaseReservationIDs: []string{reserve.ID.String()}}})
	assertHistoryUndo(t, s, ctx, id, settled.OperationID, "")
	command := ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settled.OperationID.String()}}
	preview, err := s.PreviewProductOperation(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	household, err := s.requireHousehold(ctx)
	if err != nil {
		t.Fatal(err)
	}
	amount, _ := domain.ParseMoney("50000.0001", domain.CurrencyCode("USD"))
	if _, err := s.RecordChange(ctx, domain.MoneyRemovedInput{HouseholdID: household.ID, AccountID: account, Amount: amount, Reason: domain.ReasonOther, EffectiveAt: s.clock()}); err != nil {
		t.Fatal(err)
	}
	assertHistoryUndo(t, s, ctx, id, settled.OperationID, UndoInsufficientBalance)
	if _, err := s.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err == nil {
		t.Fatal("stale allowed metadata bypassed final validation")
	}
	assertCash(t, s, ctx, account, "99999.9999")
	if err := s.ArchiveAccount(ctx, account, true); err != nil {
		t.Fatal(err)
	}
	assertHistoryUndo(t, s, ctx, id, settled.OperationID, UndoArchivedTarget)
}

func TestProductHistoryRenewalLinkedSuccessorAndReservations(t *testing.T) {
	t.Parallel()
	s, ctx, _, opened := openRegressionProduct(t)
	id := opened.ProductIDs[0]
	renewed := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{Settle: SettleProductCommand{ProductID: id.String(), ReturnedPrincipal: strPtr("100000")}, Principal: "100000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic successor", StartOn: "2026-09-20", MaturityOn: strPtr("2027-03-20"), InterestMode: "none"}, Policy: depositPolicyForMaturity("2027-03-20")}})
	successor := renewed.ProductIDs[1]
	assertHistoryUndo(t, s, ctx, id, renewed.OperationID, "")
	now := s.clock().Add(time.Minute)
	s.setClock(func() time.Time { return now })
	income := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: successor.String(), Amount: "1"}})
	assertHistoryUndo(t, s, ctx, id, renewed.OperationID, UndoLaterProductOperation)
	now = now.Add(time.Minute)
	recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: income.OperationID.String()}})
	assertHistoryUndo(t, s, ctx, id, renewed.OperationID, "")
	detail, err := s.Product(ctx, successor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: domain.HoldingSourceRef(detail.Contract.AccountID, detail.Contract.HoldingID), Label: "Synthetic successor reserve", Amount: "1"}); err != nil {
		t.Fatal(err)
	}
	assertHistoryUndo(t, s, ctx, id, renewed.OperationID, UndoActiveReservation)
}

func TestProductHistoryMissingAndCancellationFailClosed(t *testing.T) {
	t.Parallel()
	s, ctx, _, opened := openRegressionProduct(t)
	_, err := s.ListProductOperations(ctx, domain.NewProductContractID(), "", 25)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.ErrNotFound {
		t.Fatalf("missing product=%v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	page, err := s.ListProductOperations(cancelled, opened.ProductIDs[0], "", 25)
	if err == nil || page.Undo.Allowed || page.Operations != nil {
		t.Fatalf("partial cancelled response: %+v %v", page, err)
	}
}

func TestProductHistoryRecordExistingAndArchivedHoldingInstrument(t *testing.T) {
	for _, target := range []string{"holding", "instrument"} {
		t.Run(target, func(t *testing.T) {
			s, ctx, account, _ := openRegressionProduct(t)
			now := s.clock().Add(time.Minute)
			s.setClock(func() time.Time { return now })
			existing := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{AccountID: account.String(), Currency: "USD", Principal: "100.0001", CurrentValue: "100.0001", TotalCostBasis: "100.0001", CashExcludesProduct: true, Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic existing", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}})
			id := existing.ProductIDs[0]
			assertHistoryUndo(t, s, ctx, id, existing.OperationID, "")
			now = now.Add(time.Minute)
			settled := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: id.String(), ReturnedPrincipal: strPtr("100.0001")}})
			assertHistoryUndo(t, s, ctx, id, settled.OperationID, "")
			detail, err := s.Product(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			// Generic GUI archive routes prohibit managed holdings/instruments. Seed
			// archived persistence facts directly to test the final target restriction.
			if target == "holding" {
				err = s.repository.SetHoldingArchive(ctx, detail.Contract.HouseholdID, detail.Contract.HoldingID, true, now)
			} else {
				err = s.repository.SetInstrumentArchive(ctx, detail.Contract.HouseholdID, detail.Contract.InstrumentID, true, now)
			}
			if err != nil {
				t.Fatal(err)
			}
			assertHistoryUndo(t, s, ctx, id, settled.OperationID, UndoArchivedTarget)
		})
	}
}

func TestProductHistoryCorruptRestorationFailsReadInsteadOfBusinessReason(t *testing.T) {
	t.Parallel()
	s, ctx, _, opened := openRegressionProduct(t)
	id := opened.ProductIDs[0]
	interest := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpReceiveInterest, ReceiveInterest: &ReceiveInterestCommand{ProductID: id.String(), Amount: "1"}})
	now := s.clock().Add(time.Minute)
	s.setClock(func() time.Time { return now })
	reversed := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: interest.OperationID.String()}})
	snapshot, err := s.repository.ReadProductOperationHistorySnapshot(ctx, id, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	history := snapshot.UndoEvidence.Histories[id]
	for i := range history {
		if history[i].ID == reversed.OperationID {
			history[i].ResultJSON = "invalid synthetic receipt"
		}
	}
	result, err := s.productUndoAvailability(snapshot)
	if err == nil || result.Allowed || result.Reason != nil {
		t.Fatalf("integrity error disguised as availability %+v %v", result, err)
	}
}
