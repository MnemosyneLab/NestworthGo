package application

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func openRegressionProduct(t *testing.T) (*Service, context.Context, domain.AccountID, ProductOperationReceipt) {
	t.Helper()
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	a := seedHoldingsCash(t, s, ctx, "150000")
	c := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "USD", Principal: "100000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Review", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	r := recordRegressionProduct(t, s, ctx, c)
	s.setClock(func() time.Time { return time.Date(2026, 9, 20, 5, 0, 0, 0, time.UTC) })
	return s, ctx, a, r
}
func recordRegressionProduct(t *testing.T, s *Service, ctx context.Context, c ProductCommand) ProductOperationReceipt {
	t.Helper()
	p, e := s.PreviewProductOperation(ctx, c)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.RecordProductOperation(ctx, c, domain.NewProductOperationID().String(), p.ReviewedStateHash)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func TestProductRegressionUndoRejectsEditedReservation(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	src := domain.HoldingSourceRef(a, p.Contract.HoldingID)
	reserve, e := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: src, Label: "Reserve", Amount: "1000"})
	if e != nil {
		t.Fatal(e)
	}
	settled := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: p.Contract.ID.String(), ReturnedPrincipal: strPtr("100000"), ReleaseReservationIDs: []string{reserve.ID.String()}}})
	_, e = s.SaveLiquidityReservation(ctx, SaveReservationInput{ID: &reserve.ID, ExpectedRevision: 2, Source: src, Label: "Changed after release", Amount: "5000"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = s.PreviewProductOperation(ctx, ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settled.OperationID.String()}})
	if e == nil {
		t.Fatal("undo accepts later reservation edit; reservation revision is never checked")
	}
}

func TestProductRegressionNonUSDPolicyRoundTrip(t *testing.T) {
	s, ctx, a, _ := openRegressionProduct(t)
	if _, e := s.AppendAccountCashValue(ctx, a, "2000", "EUR", ""); e != nil {
		t.Fatal(e)
	}
	c := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "EUR", Principal: "1000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "EUR deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	r := recordRegressionProduct(t, s, ctx, c)
	p, e := s.Product(ctx, r.ProductIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	if p.Policy.NormalExitFee == nil || p.Policy.NormalExitFee.Currency() != domain.CurrencyCode("EUR") {
		t.Fatalf("EUR product policy reloaded fee=%v; expected EUR 0", p.Policy.NormalExitFee)
	}
}

func TestProductRegressionTermsWriteRollbackAndCAS(t *testing.T) {
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "terms.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	repo := sqlite.NewRepository(db)
	s := NewService(repo)
	s.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err = s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Terms", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	a := seedHoldingsCash(t, s, ctx, "150000")
	cmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: a.String(), Currency: "USD", Principal: "100000", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Atomic", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"}, Policy: depositPolicy()}}
	r := recordRegressionProduct(t, s, ctx, cmd)
	before, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	input := UpdateProductTermsInput{ProductID: before.Contract.ID, ExpectedRevision: before.Contract.Revision, Terms: cmd.Open.Terms, Policy: cmd.Open.Policy}
	input.Terms.MaturityOn = strPtr("2027-01-20")
	input.Policy.UnlockOn = input.Terms.MaturityOn
	if _, err = db.SQL.ExecContext(ctx, `CREATE TRIGGER fail_policy BEFORE UPDATE ON liquidity_policies BEGIN SELECT RAISE(ABORT, 'policy write failed'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateProductTerms(ctx, input); err == nil {
		t.Fatal("expected trigger failure")
	}
	got, err := s.Product(ctx, before.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.Revision != before.Contract.Revision || *got.Contract.MaturityOn != *before.Contract.MaturityOn || got.Policy.Revision != before.Policy.Revision || *got.Policy.UnlockOn != *before.Policy.UnlockOn {
		t.Fatalf("partial terms update: %+v", got)
	}
	if _, err = db.SQL.ExecContext(ctx, `DROP TRIGGER fail_policy`); err != nil {
		t.Fatal(err)
	}
	got, err = s.UpdateProductTerms(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.Revision != 2 || got.Policy.Revision != 2 || *got.Contract.MaturityOn != "2027-01-20" || *got.Policy.UnlockOn != "2027-01-20" {
		t.Fatalf("update=%+v", got)
	}
	// Application and transaction checks both reject stale writes.
	if _, err = s.UpdateProductTerms(ctx, input); err == nil {
		t.Fatal("stale application revision accepted")
	}
	if err = repo.SaveProductTerms(ctx, got.Contract, got.Policy, 1); err == nil {
		t.Fatal("stale persisted contract accepted")
	}
	next := got.Contract
	next.Revision++
	next.Name = "Should not persist"
	if err = repo.SaveProductTerms(ctx, next, got.Policy, 1); err == nil {
		t.Fatal("stale persisted policy accepted")
	}
	current, err := s.Product(ctx, got.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Contract.Name != got.Contract.Name {
		t.Fatal("CAS conflict leaked contract")
	}
}

type reservationRaceRepository struct {
	Repository
	beforeCommit func(context.Context, domain.ProductBundle) error
}

func (r reservationRaceRepository) CommitProductBundle(ctx context.Context, bundle domain.ProductBundle) error {
	if err := r.beforeCommit(ctx, bundle); err != nil {
		return err
	}
	return r.Repository.CommitProductBundle(ctx, bundle)
}

func TestProductRegressionReservationCASRollsBackUndo(t *testing.T) {
	s, ctx, a, r := openRegressionProduct(t)
	p, err := s.Product(ctx, r.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	src := domain.HoldingSourceRef(a, p.Contract.HoldingID)
	reserve, err := s.SaveLiquidityReservation(ctx, SaveReservationInput{Source: src, Label: "Reserve", Amount: "1000"})
	if err != nil {
		t.Fatal(err)
	}
	settled := recordRegressionProduct(t, s, ctx, ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{ProductID: p.Contract.ID.String(), ReturnedPrincipal: strPtr("100000"), ReleaseReservationIDs: []string{reserve.ID.String()}}})
	cmd := ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settled.OperationID.String()}}
	preview, err := s.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	original := s.repository
	s.repository = reservationRaceRepository{Repository: original, beforeCommit: func(ctx context.Context, b domain.ProductBundle) error {
		reservations, err := original.ListLiquidityReservations(ctx, p.Contract.HouseholdID, true)
		if err != nil {
			return err
		}
		current := reservations[0]
		current.Revision++
		current.Label = "Concurrent edit"
		return original.SaveLiquidityReservation(ctx, current)
	}}
	mutation := domain.NewProductOperationID()
	_, err = s.RecordProductOperation(ctx, cmd, mutation.String(), preview.ReviewedStateHash)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.ErrRevisionConflict {
		t.Fatalf("conflict=%v", err)
	}
	stored, err := original.LookupProductOperation(ctx, p.Contract.HouseholdID, mutation)
	if err != nil || stored != nil {
		t.Fatalf("failed operation leaked: %v %v", stored, err)
	}
	got, err := s.Product(ctx, p.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Contract.State != domain.ProductStateSettled {
		t.Fatal("undo partially reopened product")
	}
	assertCash(t, s, ctx, a, "150000")
}

func TestProductRegressionPolicyCurrenciesForOrdinarySources(t *testing.T) {
	for _, currency := range []string{"EUR", "SGD"} {
		t.Run(currency, func(t *testing.T) {
			s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
			bootstrap, err := s.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			a, err := s.CreateAccount(ctx, AccountInput{Name: "Simple", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "balance", DefaultCurrency: currency, InitialAmount: "1000", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			src := domain.AccountValueSourceRef(a.Account.ID)
			policy := depositPolicy()
			policy.AccessKind = "on_request"
			policy.UnlockOn = nil
			policy.NormalExitFee = strPtr("10")
			policy.EarlyKind = "allowed"
			policy.EarlySettlementDays = intPtr(0)
			policy.EarlyDayBasis = strPtr("calendar")
			policy.EarlyFee = strPtr("20")
			policy.EarlyAmountMode = strPtr("fixed_gross")
			policy.EarlyGrossAmount = strPtr("900")
			if _, err = s.SaveLiquidityPolicy(ctx, SavePolicyInput{Source: src, Policy: policy}); err != nil {
				t.Fatal(err)
			}
			overview, err := s.LiquidityOverview(ctx, LiquidityOverviewQuery{IncludeEarlyWithdrawal: true})
			if err != nil {
				t.Fatal(err)
			}
			source := overview.Sources[0]
			if source.Ref.Currency != nil {
				t.Fatal("account_value source shape changed")
			}
			if source.NormalRoute.NetNative == nil || source.NormalRoute.NetNative.Currency().String() != currency || source.NormalRoute.NetNative.CanonicalAmount() != "990" {
				t.Fatalf("normal=%+v", source.NormalRoute)
			}
			if source.EarlyRoute.NetNative == nil || source.EarlyRoute.NetNative.Currency().String() != currency || source.EarlyRoute.NetNative.CanonicalAmount() != "880" {
				t.Fatalf("early=%+v", source.EarlyRoute)
			}
		})
	}
}

func TestProductRegressionRenewWithInterestTerms(t *testing.T) {
	for _, mode := range []string{"simple_act_365", "simple_act_360", "manual_maturity_amount"} {
		t.Run(mode, func(t *testing.T) {
			s, ctx, a, r := openRegressionProduct(t)
			terms := ProductTermsInput{Kind: "term_deposit", Name: "Renewed", StartOn: "2026-09-20", MaturityOn: strPtr("2027-09-20"), InterestMode: mode}
			if mode == "manual_maturity_amount" {
				terms.MaturityInterest = strPtr("2500")
			} else {
				terms.AnnualRatePercent = strPtr("2.5")
			}
			policy := depositPolicy()
			policy.UnlockOn = terms.MaturityOn
			cmd := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{Settle: SettleProductCommand{ProductID: r.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), Fee: strPtr("0")}, Principal: "100000", OpeningFee: strPtr("10"), Terms: terms, Policy: policy}}
			receipt := recordRegressionProduct(t, s, ctx, cmd)
			assertCash(t, s, ctx, a, "50990")
			old, err := s.Product(ctx, r.ProductIDs[0])
			if err != nil {
				t.Fatal(err)
			}
			if old.Contract.State != domain.ProductStateSettled {
				t.Fatal("predecessor not settled")
			}
			if len(receipt.ProductIDs) != 2 {
				t.Fatalf("products=%v", receipt.ProductIDs)
			}
			next, err := s.Product(ctx, receipt.ProductIDs[1])
			if err != nil {
				t.Fatal(err)
			}
			if next.Contract.State != domain.ProductStateOpen || string(next.Contract.InterestMode) != mode || next.CurrentValue == nil || next.CurrentValue.CanonicalAmount() != "100000" || next.Contract.StartOn != "2026-09-20" {
				t.Fatalf("successor=%+v", next)
			}
			interest, err := next.Contract.EstimatedUnpaidMaturityInterest()
			if err != nil {
				t.Fatal(err)
			}
			want := "2500"
			if mode == "simple_act_360" {
				want = "2534.7222"
			}
			if interest == nil || interest.CanonicalAmount() != want {
				t.Fatalf("forecast=%v want %s", interest, want)
			}
		})
	}
}

func TestProductRegressionOrdinaryHoldingPolicyAmountsRetainQuoteCurrency(t *testing.T) {
	s, ctx, a, _ := openRegressionProduct(t)
	instrument, err := s.CreateInstrument(ctx, InstrumentInput{Name: "SGD fund", Type: "etf", QuoteCurrency: "SGD", QuoteSource: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendManualInstrumentQuote(ctx, instrument.ID, "1000", "", false); err != nil {
		t.Fatal(err)
	}
	holding, err := s.CreateHolding(ctx, HoldingInput{AccountID: a.String(), InstrumentID: instrument.ID.String(), Quantity: "1"})
	if err != nil {
		t.Fatal(err)
	}
	input := depositPolicy()
	input.AccessKind = "on_request"
	input.UnlockOn = nil
	input.NormalExitFee = strPtr("10")
	input.EarlyKind = "allowed"
	input.EarlySettlementDays = intPtr(0)
	input.EarlyDayBasis = strPtr("calendar")
	input.EarlyFee = strPtr("20")
	input.EarlyAmountMode = strPtr("fixed_gross")
	input.EarlyGrossAmount = strPtr("900")
	policy, err := s.SaveLiquidityPolicy(ctx, SavePolicyInput{Source: domain.HoldingSourceRef(a, holding.ID), Policy: input})
	if err != nil {
		t.Fatal(err)
	}
	cap, err := domain.ParseMoney("500", "SGD")
	if err != nil {
		t.Fatal(err)
	}
	policy.AccessibleAmountCap = &cap
	policy.Revision++
	if err = s.repository.SaveLiquidityPolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	overview, err := s.LiquidityOverview(ctx, LiquidityOverviewQuery{IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range overview.Sources {
		if source.Ref.HoldingID != nil && *source.Ref.HoldingID == holding.ID {
			if source.Policy.AccessibleAmountCap == nil || source.Policy.AccessibleAmountCap.Currency() != "SGD" {
				t.Fatalf("cap=%v", source.Policy.AccessibleAmountCap)
			}
			if source.NormalRoute.NetNative == nil || source.NormalRoute.NetNative.Currency() != "SGD" || source.NormalRoute.NetNative.CanonicalAmount() != "490" {
				t.Fatalf("normal=%+v", source.NormalRoute)
			}
			if source.EarlyRoute.NetNative == nil || source.EarlyRoute.NetNative.Currency() != "SGD" || source.EarlyRoute.NetNative.CanonicalAmount() != "480" {
				t.Fatalf("early=%+v", source.EarlyRoute)
			}
			return
		}
	}
	t.Fatal("ordinary holding source missing")
}
