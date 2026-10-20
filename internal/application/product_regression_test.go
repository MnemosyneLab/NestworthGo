package application

import (
	"context"
	"errors"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
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

func TestProductContractDatesNormalizeAcrossCreateAndTermsUpdate(t *testing.T) {
	t.Parallel()
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, s, ctx, "150000")
	command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000",
		Terms: ProductTermsInput{
			Kind: "term_deposit", Name: "Whitespace dates", StartOn: " 2026-09-20 ", MaturityOn: strPtr(" 2026-12-20 "),
			InterestMode: "simple_act_365", AnnualRatePercent: strPtr("2.5"), InterestPaidThroughOn: strPtr(" 2026-09-20 "),
		},
		Policy: depositPolicy(),
	}}
	receipt := recordRegressionProduct(t, s, ctx, command)
	product, err := s.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if product.Contract.StartOn != "2026-09-20" || product.Contract.MaturityOn == nil || *product.Contract.MaturityOn != "2026-12-20" || product.Contract.InterestPaidThroughOn == nil || *product.Contract.InterestPaidThroughOn != "2026-09-20" {
		t.Fatalf("created contract dates = start %q maturity %v paid-through %v", product.Contract.StartOn, product.Contract.MaturityOn, product.Contract.InterestPaidThroughOn)
	}

	updated, err := s.UpdateProductTerms(ctx, UpdateProductTermsInput{
		ProductID: product.Contract.ID, ExpectedRevision: product.Contract.Revision,
		Terms: ProductTermsInput{
			Kind: "term_deposit", Name: product.Contract.Name, StartOn: " 2026-09-20 ", MaturityOn: strPtr(" 2026-12-20 "),
			InterestMode: "simple_act_365", AnnualRatePercent: strPtr("2.5"), InterestPaidThroughOn: strPtr(" 2026-10-01 "),
		},
		Policy: depositPolicy(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Contract.StartOn != "2026-09-20" || updated.Contract.MaturityOn == nil || *updated.Contract.MaturityOn != "2026-12-20" || updated.Contract.InterestPaidThroughOn == nil || *updated.Contract.InterestPaidThroughOn != "2026-10-01" {
		t.Fatalf("updated contract dates = start %q maturity %v paid-through %v", updated.Contract.StartOn, updated.Contract.MaturityOn, updated.Contract.InterestPaidThroughOn)
	}
	interest, err := updated.Contract.EstimatedUnpaidMaturityInterest()
	if err != nil || interest == nil || !interest.Amount().IsPositive() {
		t.Fatalf("normalized dates were not usable by interest accrual: amount=%v err=%v", interest, err)
	}

	badTerms := ProductTermsInput{Kind: "term_deposit", Name: updated.Contract.Name, StartOn: updated.Contract.StartOn, MaturityOn: strPtr(" 2026-02-30 "), InterestMode: "none"}
	if _, err := s.UpdateProductTerms(ctx, UpdateProductTermsInput{ProductID: updated.Contract.ID, ExpectedRevision: updated.Contract.Revision, Terms: badTerms, Policy: depositPolicy()}); err == nil {
		t.Fatal("invalid date update unexpectedly succeeded")
	}
	unchanged, err := s.Product(ctx, updated.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Contract.Revision != updated.Contract.Revision || unchanged.Contract.InterestPaidThroughOn == nil || *unchanged.Contract.InterestPaidThroughOn != "2026-10-01" {
		t.Fatalf("failed date update changed contract: %+v", unchanged.Contract)
	}
}

func TestProductOperationRejectsInvalidWhitespaceDateBeforeWrite(t *testing.T) {
	t.Parallel()
	s, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, s, ctx, "150000")
	command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Invalid date", StartOn: " 2026-02-30 ", MaturityOn: strPtr(" 2026-12-20 "), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	if _, err := s.PreviewProductOperation(ctx, command); err == nil {
		t.Fatal("invalid contract date unexpectedly accepted")
	}
}

func TestGuardedProductTermsPreserveLegacyStartDateAndVerifyOnReopen(t *testing.T) {
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "legacy-product.db")
	database, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if database != nil {
			_ = database.Close()
		}
	})
	service := NewService(sqlite.NewRepository(database))
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Legacy dates", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	account := seedHoldingsCash(t, service, ctx, "150000")
	command := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Legacy padded start", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	created := recordRegressionProduct(t, service, ctx, command)
	product, err := service.Product(ctx, created.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE product_contracts SET start_on=? WHERE id=?`, " 2026-09-20 ", product.Contract.ID.String()); err != nil {
		t.Fatal(err)
	}

	input := UpdateProductTermsInput{
		ProductID: product.Contract.ID, ExpectedRevision: product.Contract.Revision,
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Legacy start updated", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}
	preview, token, err := service.PreviewProductTermsGuarded(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitProductTermsGuarded(ctx, input, domain.NewProductOperationID().String(), preview.ReviewedStateHash, token); err != nil {
		t.Fatal(err)
	}
	var storedStart string
	if err := database.SQL.QueryRowContext(ctx, `SELECT start_on FROM product_contracts WHERE id=?`, product.Contract.ID.String()).Scan(&storedStart); err != nil {
		t.Fatal(err)
	}
	if storedStart != " 2026-09-20 " {
		t.Fatalf("terms update rewrote immutable legacy start date as %q", storedStart)
	}
	var mutationKey string
	var rawMutation []byte
	if err := database.SQL.QueryRowContext(ctx, `SELECT key,value FROM app_configuration WHERE key LIKE 'product.terms-mutation.%'`).Scan(&mutationKey, &rawMutation); err != nil {
		t.Fatal(err)
	}
	var mutation domain.ProductTermsMutation
	if err := jsoncodec.Unmarshal(rawMutation, &mutation); err != nil {
		t.Fatal(err)
	}
	if mutation.Receipt.Contract.StartOn != "2026-09-20" {
		t.Fatalf("new guarded receipt StartOn = %q, want canonical date", mutation.Receipt.Contract.StartOn)
	}

	// Exercise the original mismatch directly: the live row still contains a
	// valid padded legacy value while its newly sealed receipt is canonical.
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen padded legacy row with canonical receipt: %v", err)
	}
	database = reopened
	legacyRead, err := sqlite.NewRepository(reopened).Product(ctx, product.Contract.HouseholdID, product.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if legacyRead.StartOn != " 2026-09-20 " {
		t.Fatalf("ordinary legacy row read = %q, want stored spelling to remain padded", legacyRead.StartOn)
	}
	var preservedCanonical []byte
	if err := reopened.SQL.QueryRowContext(ctx, `SELECT value FROM app_configuration WHERE key=?`, mutationKey).Scan(&preservedCanonical); err != nil || string(preservedCanonical) != string(rawMutation) {
		t.Fatalf("startup verification rewrote the canonical sealed receipt: %v", err)
	}
	canonicalVerifier, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("read-only verification of padded legacy row and canonical receipt: %v", err)
	}
	if err := canonicalVerifier.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate a previously sealed receipt whose command and result both kept
	// the valid padded spelling; verification must accept it without rewriting
	// the original record or its digest.
	var legacyCommand UpdateProductTermsInput
	if err := jsoncodec.Unmarshal(mutation.CommandJSON, &legacyCommand); err != nil {
		t.Fatal(err)
	}
	legacyCommand.Terms.StartOn = " 2026-09-20 "
	legacyCommandJSON, err := jsoncodec.Marshal(legacyCommand)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceipt := mutation.Receipt
	legacyReceipt.Contract.StartOn = " 2026-09-20 "
	legacyMutation, err := domain.NewProductTermsMutation(mutation.ID, mutation.HouseholdID, legacyCommandJSON, legacyReceipt, mutation.CreatedAt)
	if err != nil {
		t.Fatal(err)
	}
	legacyRaw, err := jsoncodec.Marshal(legacyMutation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE app_configuration SET value=? WHERE key=?`, string(legacyRaw), mutationKey); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database = nil

	legacyReopened, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen padded legacy receipt: %v", err)
	}
	database = legacyReopened
	var preservedMutation []byte
	if err := legacyReopened.SQL.QueryRowContext(ctx, `SELECT value FROM app_configuration WHERE key=?`, mutationKey).Scan(&preservedMutation); err != nil || string(preservedMutation) != string(legacyRaw) {
		t.Fatalf("startup verification rewrote the sealed legacy receipt: %v", err)
	}
	verified, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatalf("read-only verification of padded legacy receipt: %v", err)
	}
	if err := verified.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	if err := verified.Close(); err != nil {
		t.Fatal(err)
	}

	// Canonicalization must not make a genuinely different valid date compare
	// equal. Reject the altered live fact on writable and read-only paths.
	if _, err := legacyReopened.SQL.ExecContext(ctx, `UPDATE product_contracts SET start_on=? WHERE id=?`, "2026-09-21", product.Contract.ID.String()); err != nil {
		t.Fatal(err)
	}
	if err := legacyReopened.Close(); err != nil {
		t.Fatal(err)
	}
	database = nil
	if unexpected, err := sqlite.OpenReadOnlyForVerify(path); err == nil {
		_ = unexpected.Close()
		t.Fatal("read-only verifier accepted a genuinely different product start date")
	}
	if unexpected, err := sqlite.Open(path); err == nil {
		_ = unexpected.Close()
		t.Fatal("startup verifier accepted a genuinely different product start date")
	}
}

func TestProductRegressionUndoRejectsEditedReservation(t *testing.T) {
	t.Parallel()
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
	assertHistoryUndo(t, s, ctx, p.Contract.ID, settled.OperationID, UndoReservationChanged)
}

func TestProductRegressionNonUSDPolicyRoundTrip(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
