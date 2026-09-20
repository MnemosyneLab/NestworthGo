package application

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

func TestFixtureBOpeningReceiptUndoAndIdempotency(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	openID := domain.NewProductOperationID()
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("1000")},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatalf("preview open: %v", err)
	}
	openReceipt, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatalf("record open: %v", err)
	}
	if len(openReceipt.ProductIDs) != 1 {
		t.Fatalf("open products = %v", openReceipt.ProductIDs)
	}
	assertCash(t, service, ctx, account, "50000")
	product, err := service.Product(ctx, openReceipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if product.CurrentValue == nil || product.CurrentValue.CanonicalAmount() != "100000" {
		t.Fatalf("product value = %+v", product.CurrentValue)
	}
	replay, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil || !replay.Replayed || replay.OperationID != openReceipt.OperationID {
		t.Fatalf("idempotent replay = %+v err=%v", replay, err)
	}
	changed := openCmd
	changed.Open.Principal = "90000"
	if _, err := service.RecordProductOperation(ctx, changed, openID.String(), preview.ReviewedStateHash); err == nil || err.(*domain.Error).Code != domain.ErrConflict {
		t.Fatalf("changed mutation error = %v", err)
	}
	settleID := domain.NewProductOperationID()
	settleCmd := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{
		ProductID: product.Contract.ID.String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), Fee: strPtr("10"), EffectiveAt: "2026-09-20T04:00:00Z",
	}}
	settlePreview, err := service.PreviewProductOperation(ctx, settleCmd)
	if err != nil {
		t.Fatalf("preview settle: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, settleCmd, settleID.String(), settlePreview.ReviewedStateHash); err != nil {
		t.Fatalf("record settle: %v", err)
	}
	assertCash(t, service, ctx, account, "150990")
	settled, err := service.Product(ctx, product.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if settled.Contract.State != domain.ProductStateSettled {
		t.Fatalf("state = %s", settled.Contract.State)
	}
	undoID := domain.NewProductOperationID()
	undoCmd := ProductCommand{Kind: domain.ProductOpUndo, Undo: &UndoProductCommand{OperationID: settleID.String()}}
	undoPreview, err := service.PreviewProductOperation(ctx, undoCmd)
	if err != nil {
		t.Fatalf("preview undo: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, undoCmd, undoID.String(), undoPreview.ReviewedStateHash); err != nil {
		t.Fatalf("undo settle: %v", err)
	}
	assertCash(t, service, ctx, account, "50000")
	reopened, err := service.Product(ctx, product.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Contract.State != domain.ProductStateOpen {
		t.Fatalf("reopened state = %s", reopened.Contract.State)
	}
}

func TestFixtureCRenewalAtomicAndRetry(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	openID := domain.NewProductOperationID()
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatal(err)
	}
	openReceipt, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	renewID := domain.NewProductOperationID()
	renewCmd := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{
		Settle:    SettleProductCommand{ProductID: openReceipt.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), EffectiveAt: "2026-09-20T04:00:00Z"},
		Principal: "100500",
		Terms:     ProductTermsInput{Kind: "term_deposit", Name: "Deposit D2", StartOn: "2026-09-20", MaturityOn: strPtr("2027-03-20"), InterestMode: "none"},
		Policy:    depositPolicy(),
	}}
	renewPreview, err := service.PreviewProductOperation(ctx, renewCmd)
	if err != nil {
		t.Fatalf("preview renew: %v", err)
	}
	receipt, err := service.RecordProductOperation(ctx, renewCmd, renewID.String(), renewPreview.ReviewedStateHash)
	if err != nil {
		t.Fatalf("record renew: %v", err)
	}
	if len(receipt.ProductIDs) != 2 {
		t.Fatalf("renew products = %v", receipt.ProductIDs)
	}
	assertCash(t, service, ctx, account, "50500")
	oldProduct, _ := service.Product(ctx, receipt.ProductIDs[0])
	newProduct, _ := service.Product(ctx, receipt.ProductIDs[1])
	if oldProduct.Contract.State != domain.ProductStateSettled || newProduct.Contract.State != domain.ProductStateOpen {
		t.Fatalf("old=%s new=%s", oldProduct.Contract.State, newProduct.Contract.State)
	}
	if newProduct.Contract.RenewedFromID == nil || *newProduct.Contract.RenewedFromID != oldProduct.Contract.ID {
		t.Fatalf("renewed_from = %+v", newProduct.Contract.RenewedFromID)
	}
	if newProduct.CurrentValue == nil || newProduct.CurrentValue.CanonicalAmount() != "100500" {
		t.Fatalf("new value = %+v", newProduct.CurrentValue)
	}
	replay, err := service.RecordProductOperation(ctx, renewCmd, renewID.String(), renewPreview.ReviewedStateHash)
	if err != nil || !replay.Replayed {
		t.Fatalf("renew replay = %+v %v", replay, err)
	}
}

func TestFixtureCRenewalFaultInjectionRollsBack(t *testing.T) {
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "renew-fail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	service := NewService(sqlite.NewRepository(database))
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Funds", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	account := seedHoldingsCash(t, service, ctx, "150000")
	openID := domain.NewProductOperationID()
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, _ := service.PreviewProductOperation(ctx, openCmd)
	openReceipt, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	sqlite.SetProductCommitFailAfter("activity:1")
	t.Cleanup(func() { sqlite.SetProductCommitFailAfter("") })
	renewID := domain.NewProductOperationID()
	renewCmd := ProductCommand{Kind: domain.ProductOpRenew, Renew: &RenewProductCommand{
		Settle:    SettleProductCommand{ProductID: openReceipt.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), EffectiveAt: "2026-09-20T04:00:00Z"},
		Principal: "100500",
		Terms:     ProductTermsInput{Kind: "term_deposit", Name: "Deposit D2", StartOn: "2026-09-20", MaturityOn: strPtr("2027-03-20"), InterestMode: "none"},
		Policy:    depositPolicy(),
	}}
	renewPreview, err := service.PreviewProductOperation(ctx, renewCmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordProductOperation(ctx, renewCmd, renewID.String(), renewPreview.ReviewedStateHash); err == nil {
		t.Fatal("expected injected failure")
	}
	assertCash(t, service, ctx, account, "50000")
	oldProduct, err := service.Product(ctx, openReceipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if oldProduct.Contract.State != domain.ProductStateOpen {
		t.Fatalf("partial renewal leaked state %s", oldProduct.Contract.State)
	}
}

func TestFixtureELockedProductValuationAndLoss(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "20000")
	openID := domain.NewProductOperationID()
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "10000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "locked_product", Name: "Locked L1", StartOn: "2026-09-20", InterestMode: "none"},
		Policy: ProductPolicyInput{AccessKind: "on_date", UnlockOn: strPtr("2026-10-05"), EarlyKind: "not_allowed", SettlementDays: intPtr(0), DayBasis: strPtr("calendar")},
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatalf("preview open: %v", err)
	}
	openReceipt, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatalf("open locked: %v", err)
	}
	productID := openReceipt.ProductIDs[0]
	if _, err := service.AppendProductValuation(ctx, AppendProductValuationInput{ProductID: productID, Amount: "9800", ObservedAt: "2026-09-20T04:00:00Z", MutationID: domain.NewProductOperationID().String()}); err != nil {
		t.Fatalf("valuation: %v", err)
	}
	detail, err := service.Product(ctx, productID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.CurrentValue == nil || detail.CurrentValue.CanonicalAmount() != "9800" {
		t.Fatalf("valued = %+v", detail.CurrentValue)
	}
	settleID := domain.NewProductOperationID()
	settleCmd := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{
		ProductID: productID.String(), GrossProceeds: strPtr("9700"), Fee: strPtr("5"), EffectiveAt: "2026-09-20T04:00:00Z",
	}}
	settlePreview, err := service.PreviewProductOperation(ctx, settleCmd)
	if err != nil {
		t.Fatalf("preview settle: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, settleCmd, settleID.String(), settlePreview.ReviewedStateHash); err != nil {
		t.Fatalf("settle locked: %v", err)
	}
	assertCash(t, service, ctx, account, "19695")
}

func TestManagedPositionGuardsRejectGenericEdits(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	openID := domain.NewProductOperationID()
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, _ := service.PreviewProductOperation(ctx, openCmd)
	receipt, err := service.RecordProductOperation(ctx, openCmd, openID.String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	detail, _ := service.Product(ctx, receipt.ProductIDs[0])
	quantity, _ := domain.ParseQuantity("1")
	gross, _ := domain.ParseMoney("1000", "USD")
	bootstrap, _ := service.Bootstrap(ctx)
	if _, err := service.PreviewChange(ctx, domain.TradeInput{HouseholdID: bootstrap.Household.ID, Side: domain.TradeSell, SettlementAccountID: account, HoldingID: detail.Contract.HoldingID, InstrumentID: detail.Contract.InstrumentID, Quantity: quantity, Gross: gross}); err == nil || err.(*domain.Error).Code != domain.ErrManagedPosition {
		t.Fatalf("generic trade error = %v", err)
	}
	if _, err := service.UndoChange(ctx, receipt.ActivityIDs[0]); err == nil || err.(*domain.Error).Code != domain.ErrManagedPosition {
		t.Fatalf("generic undo error = %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, detail.Contract.InstrumentID, "999", "2026-09-20T04:00:00Z", false); err == nil || err.(*domain.Error).Code != domain.ErrManagedPosition {
		t.Fatalf("generic quote error = %v", err)
	}
}

func TestLiquidityOverviewUsesValuationSnapshot(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "10000")
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.LocalDate != "2026-09-20" {
		t.Fatalf("localDate = %s", overview.LocalDate)
	}
	found := false
	for _, source := range overview.Sources {
		if source.AccountID == account && source.Ref.Kind == domain.SourceAccountCash {
			found = true
		}
	}
	if !found {
		t.Fatalf("cash source missing: %+v", overview.Sources)
	}
}

func TestRecordExistingRequiresAcknowledgementAndCost(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	cmd := ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "20000", CurrentValue: "20000",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Existing", StartOn: "2025-01-01", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	if _, err := service.PreviewProductOperation(ctx, cmd); err == nil {
		t.Fatal("missing acknowledgement accepted")
	}
	cmd.RecordExisting.CashExcludesProduct = true
	if _, err := service.PreviewProductOperation(ctx, cmd); err == nil {
		t.Fatal("missing cost basis accepted")
	}
	cmd.RecordExisting.TotalCostBasis = "20000"
	preview, err := service.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatalf("preview existing: %v", err)
	}
	receipt, err := service.RecordProductOperation(ctx, cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatalf("record existing: %v", err)
	}
	assertCash(t, service, ctx, account, "150000")
	detail, _ := service.Product(ctx, receipt.ProductIDs[0])
	if detail.CurrentValue == nil || detail.CurrentValue.CanonicalAmount() != "20000" {
		t.Fatalf("existing value = %+v", detail.CurrentValue)
	}
}

func newProductTestService(t *testing.T, clock time.Time) (*Service, context.Context) {
	t.Helper()
	return newProductTestServiceTZ(t, clock, "UTC")
}

func newProductTestServiceTZ(t *testing.T, clock time.Time, timezone string) (*Service, context.Context) {
	t.Helper()
	database, err := sqlite.Open(filepath.Join(t.TempDir(), "products.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	service := NewService(sqlite.NewRepository(database))
	service.setClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := service.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Funds", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartHistory(ctx, timezone); err != nil {
		t.Fatal(err)
	}
	return service, ctx
}

func seedHoldingsCash(t *testing.T, service *Service, ctx context.Context, amount string) domain.AccountID {
	t.Helper()
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, amount, "USD", ""); err != nil {
		t.Fatal(err)
	}
	return account.Account.ID
}

func assertCash(t *testing.T, service *Service, ctx context.Context, accountID domain.AccountID, want string) {
	t.Helper()
	valuation, err := service.AccountValuation(ctx, accountID)
	if err != nil {
		t.Fatal(err)
	}
	for _, component := range valuation.Components {
		if component.HoldingID != nil || component.NativeCurrency.String() != "USD" {
			continue
		}
		if component.NativeAmount != want {
			t.Fatalf("cash = %s, want %s", component.NativeAmount, want)
		}
		return
	}
	t.Fatalf("USD cash component missing: %+v", valuation.Components)
}

func depositPolicy() ProductPolicyInput {
	zero := 0
	calendar := "calendar"
	maturity := "2026-12-20"
	return ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar, NormalExitFee: strPtr("0")}
}

func strPtr(value string) *string { return &value }
func intPtr(value int) *int       { return &value }

func TestFixtureALiquidityOverviewTotals(t *testing.T) {
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	service, ctx := newProductTestServiceTZ(t, clock, "Asia/Singapore")
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bank := seedHoldingsCash(t, service, ctx, "10000")
	usd, err := domain.ParseCurrency("USD")
	if err != nil {
		t.Fatal(err)
	}
	saveExplicitAccessPolicy(t, service, ctx, domain.AccountCashSourceRef(bank, usd), 0)
	if _, err := service.SaveLiquidityReservation(ctx, SaveReservationInput{
		Source: domain.AccountCashSourceRef(bank, usd), Label: "Bills", Amount: "2000",
	}); err != nil {
		t.Fatalf("cash reservation: %v", err)
	}
	maturity := "2026-09-25"
	zero := 0
	calendar := "calendar"
	earlyFee := "50"
	earlyMode := "fixed_gross"
	earlyGross := "20000"
	depositCmd := ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{
		AccountID: bank.String(), Currency: "USD", Principal: "20000", TotalCostBasis: "20000", CurrentValue: "20000",
		CashExcludesProduct: true, EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-01-01", MaturityOn: &maturity, InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("200")},
		Policy: ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "allowed", SettlementDays: &zero, DayBasis: &calendar, EarlySettlementDays: &zero, EarlyDayBasis: &calendar, EarlyFee: &earlyFee, EarlyAmountMode: &earlyMode, EarlyGrossAmount: &earlyGross, NormalExitFee: strPtr("0")},
	}}
	preview, err := service.PreviewProductOperation(ctx, depositCmd)
	if err != nil {
		t.Fatalf("preview deposit: %v", err)
	}
	openReceipt, err := service.RecordProductOperation(ctx, depositCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatalf("record deposit: %v", err)
	}
	deposit, err := service.Product(ctx, openReceipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.SaveLiquidityReservation(ctx, SaveReservationInput{
		Source: domain.HoldingSourceRef(bank, deposit.Contract.HoldingID), Label: "Reserve D1", Amount: "5000",
	}); err != nil {
		t.Fatalf("deposit reservation: %v", err)
	}
	lockOn := "2026-10-05"
	lockedCmd := ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{
		AccountID: bank.String(), Currency: "USD", Principal: "5000", TotalCostBasis: "5000", CurrentValue: "5000",
		CashExcludesProduct: true, EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "locked_product", Name: "Locked L1", StartOn: "2026-01-01", MaturityOn: &lockOn, InterestMode: "none"},
		Policy: ProductPolicyInput{AccessKind: "on_date", UnlockOn: &lockOn, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar, NormalExitFee: strPtr("10")},
	}}
	lockedPreview, err := service.PreviewProductOperation(ctx, lockedCmd)
	if err != nil {
		t.Fatalf("preview locked: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, lockedCmd, domain.NewProductOperationID().String(), lockedPreview.ReviewedStateHash); err != nil {
		t.Fatalf("open locked: %v", err)
	}
	stock, err := service.CreateInstrument(ctx, InstrumentInput{Name: "H1", Type: "stock", QuoteCurrency: "USD", QuoteSource: "manual"})
	if err != nil {
		t.Fatalf("stock instrument: %v", err)
	}
	holding, err := service.CreateHolding(ctx, HoldingInput{AccountID: bank.String(), InstrumentID: stock.ID.String(), Quantity: "1", UnitCost: "3000"})
	if err != nil {
		t.Fatalf("stock holding: %v", err)
	}
	if _, err := service.AppendManualInstrumentQuote(ctx, stock.ID, "3000", "2026-09-20T04:00:00Z", false); err != nil {
		t.Fatalf("stock quote: %v", err)
	}
	saveExplicitAccessPolicy(t, service, ctx, domain.HoldingSourceRef(bank, holding.ID), 2)
	if _, err := service.CreateAccount(ctx, AccountInput{
		Name: "Home", AccountType: "property", BalanceSheetRole: "asset", TrackingMode: "manual_value",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}, InitialAmount: "100000",
	}); err != nil {
		t.Fatalf("property: %v", err)
	}
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.LocalDate != "2026-09-20" || overview.Timezone != "Asia/Singapore" {
		t.Fatalf("localDate/tz = %s %s", overview.LocalDate, overview.Timezone)
	}
	assertBucket(t, overview, "2026-09-20", "10000", "2000", "8000")
	assertBucket(t, overview, "2026-09-27", "33200", "7000", "26200")
	assertBucket(t, overview, "2026-10-20", "38190", "7000", "31190")
	overviewEarly, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{IncludeEarlyWithdrawal: true})
	if err != nil {
		t.Fatal(err)
	}
	assertBucket(t, overviewEarly, "2026-09-20", "29950", "7000", "22950")
	assertBucket(t, overviewEarly, "2026-09-27", "33200", "7000", "26200")

	if _, err := service.AppendAccountCashValue(ctx, bank, "100", "EUR", ""); err != nil {
		t.Fatalf("eur cash: %v", err)
	}
	eur, err := domain.ParseCurrency("EUR")
	if err != nil {
		t.Fatal(err)
	}
	saveExplicitAccessPolicy(t, service, ctx, domain.AccountCashSourceRef(bank, eur), 0)
	partial, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if partial.Buckets[0].FullAvailable != nil || (partial.Buckets[0].Status != domain.StatusPartial && partial.Buckets[0].Status != domain.StatusUnavailable) {
		t.Fatalf("missing EUR/USD FX must leave base full total null, got %+v", partial.Buckets[0])
	}
	usdKnown := ""
	eurKnown := ""
	for _, group := range partial.Buckets[0].NativeCurrencyGroups {
		if group.Currency == "USD" && group.KnownAvailableSubtotal != nil {
			usdKnown = group.KnownAvailableSubtotal.CanonicalAmount()
		}
		if group.Currency == "EUR" && group.KnownAvailableSubtotal != nil {
			eurKnown = group.KnownAvailableSubtotal.CanonicalAmount()
		}
	}
	if usdKnown != "10000" || eurKnown != "100" {
		t.Fatalf("native groups USD=%s EUR=%s, want 10000 and 100", usdKnown, eurKnown)
	}
}

func saveExplicitAccessPolicy(t *testing.T, service *Service, ctx context.Context, source domain.LiquiditySourceRef, settlementDays int) {
	t.Helper()
	calendar := "calendar"
	if _, err := service.SaveLiquidityPolicy(ctx, SavePolicyInput{
		Source: source,
		Policy: ProductPolicyInput{
			AccessKind: "on_request", SettlementDays: &settlementDays, DayBasis: &calendar,
			NormalExitFee: strPtr("0"), EarlyKind: "not_allowed",
		},
	}); err != nil {
		t.Fatalf("save policy: %v", err)
	}
}

func assertBucket(t *testing.T, overview domain.LiquidityOverview, horizon, available, reserved, unreserved string) {
	t.Helper()
	for _, bucket := range overview.Buckets {
		if bucket.HorizonOn != horizon {
			continue
		}
		gotAvailable := moneyOrEmpty(bucket.KnownAvailableSubtotal)
		gotReserved := moneyOrEmpty(bucket.AppliedReserveSubtotal)
		gotUnreserved := moneyOrEmpty(bucket.KnownUnreservedSubtotal)
		if gotAvailable != available || gotReserved != reserved || gotUnreserved != unreserved {
			t.Fatalf("%s available/reserve/unreserved = %s/%s/%s, want %s/%s/%s status=%s", horizon, gotAvailable, gotReserved, gotUnreserved, available, reserved, unreserved, bucket.Status)
		}
		return
	}
	t.Fatalf("missing horizon %s in %+v", horizon, overview.Buckets)
}

func moneyOrEmpty(value *domain.Money) string {
	if value == nil {
		return ""
	}
	return value.CanonicalAmount()
}

func TestJSONExportIncludesLiquidityFacts(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	maturity := "2026-12-20"
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: &maturity, InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordProductOperation(ctx, openCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	doc := decodeExport(t, service)
	if doc.FormatVersion != 2 {
		t.Fatalf("formatVersion = %d", doc.FormatVersion)
	}
	if len(doc.Facts.Liquidity["contracts"]) != 1 || len(doc.Facts.Liquidity["operations"]) != 1 {
		t.Fatalf("liquidity facts = %+v", doc.Facts.Liquidity)
	}
	if _, ok := doc.Facts.Liquidity["policies"]; !ok {
		t.Fatal("policies dataset missing")
	}
}

func TestRestrictedAccountCashIsUnknownUntilReviewed(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	bootstrap, err := service.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(ctx, AccountInput{
		Name: "Brokerage", AccountType: "brokerage", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AppendAccountCashValue(ctx, account.Account.ID, "8000", "USD", ""); err != nil {
		t.Fatal(err)
	}
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range overview.Sources {
		if source.AccountID == account.Account.ID && source.Ref.Kind == domain.SourceAccountCash {
			found = true
			if source.PolicyOrigin != domain.PolicyOriginAssumed || source.NormalRoute == nil || source.NormalRoute.NetNative != nil {
				t.Fatalf("restricted cash must stay unknown, got %+v", source)
			}
		}
	}
	if !found {
		t.Fatal("brokerage cash source missing")
	}
	if overview.Buckets[0].FullAvailable != nil {
		t.Fatalf("unknown restricted cash must not yield a full total, got %v", overview.Buckets[0].FullAvailable)
	}
}

func TestDueUnconfirmedProductIsNotSpendable(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "10000")
	maturity := "2026-09-20"
	cmd := ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &RecordExistingProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "4000", TotalCostBasis: "4000", CurrentValue: "4000",
		CashExcludesProduct: true, EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Due D1", StartOn: "2026-01-01", MaturityOn: &maturity, InterestMode: "none"},
		Policy: ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: intPtr(0), DayBasis: strPtr("calendar"), NormalExitFee: strPtr("0")},
	}}
	preview, err := service.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := service.RecordProductOperation(ctx, cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range overview.Sources {
		if source.ProductID != nil && *source.ProductID == receipt.ProductIDs[0] {
			found = true
			if !source.DueUnconfirmed {
				t.Fatalf("expected due-unconfirmed, got %+v", source)
			}
			if len(source.BucketResults) == 0 || source.BucketResults[0].NetNative != nil {
				t.Fatalf("due product must not contribute spendable net, got %+v", source.BucketResults)
			}
		}
	}
	if !found {
		t.Fatal("due product source missing")
	}
	assertBucket(t, overview, "2026-09-20", "10000", "0", "10000")
}

func TestProductInterestAttributedOnceAcrossScopes(t *testing.T) {
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	service, ctx := newProductTestService(t, clock)
	account := seedHoldingsCash(t, service, ctx, "150000")
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatal(err)
	}
	openReceipt, err := service.RecordProductOperation(ctx, openCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	settleCmd := ProductCommand{Kind: domain.ProductOpSettle, Settle: &SettleProductCommand{
		ProductID: openReceipt.ProductIDs[0].String(), ReturnedPrincipal: strPtr("100000"), Interest: strPtr("1000"), Fee: strPtr("10"), EffectiveAt: "2026-09-20T04:00:00Z",
	}}
	settlePreview, err := service.PreviewProductOperation(ctx, settleCmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordProductOperation(ctx, settleCmd, domain.NewProductOperationID().String(), settlePreview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	product, err := service.Product(ctx, openReceipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	service.setClock(func() time.Time { return time.Date(2026, 9, 21, 4, 0, 0, 0, time.UTC) })
	householdQuery := domain.AnalysisQuery{
		Scope: domain.AnalysisScope{Kind: domain.ScopeHousehold}, From: "2026-09-20", To: "2026-09-20",
		Valuation: domain.ValuationBase, Basis: domain.ReturnBasisInvestment, IncludeCash: true,
	}
	household, err := service.Analyze(ctx, householdQuery)
	if err != nil {
		t.Fatalf("household analysis: %v", err)
	}
	interest := sumReturnComponent(household, domain.ReturnDividendInterest)
	if !interest.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("household product interest = %s, want 1000", interest)
	}
	withoutCash := householdQuery
	withoutCash.IncludeCash = false
	investment, err := service.Analyze(ctx, withoutCash)
	if err != nil {
		t.Fatal(err)
	}
	if got := sumReturnComponent(investment, domain.ReturnDividendInterest); !got.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("includeCash=false interest = %s, want 1000", got)
	}
	instrumentQuery := householdQuery
	instrumentQuery.Scope = domain.AnalysisScope{Kind: domain.ScopeInstrument, ID: product.Contract.InstrumentID.String()}
	instrument, err := service.Analyze(ctx, instrumentQuery)
	if err != nil {
		t.Fatal(err)
	}
	if got := sumReturnComponent(instrument, domain.ReturnDividendInterest); !got.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("instrument-scope interest = %s, want 1000", got)
	}
}

func sumReturnComponent(result domain.PeriodAnalysisResult, component domain.ReturnComponent) decimal.Decimal {
	total := decimal.Zero
	for _, day := range result.Days {
		if value, ok := day.ReturnComponents[component]; ok {
			total = total.Add(value.Amount())
		}
	}
	return total
}

func TestExclusiveBackupRejectsProductWrites(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	release := holdExclusive(t, service, ExclusiveBackup)
	defer release()
	usd, _ := domain.ParseCurrency("USD")
	if _, err := service.SaveLiquidityReservation(ctx, SaveReservationInput{
		Source: domain.AccountCashSourceRef(account, usd), Label: "Bills", Amount: "100",
	}); !isBackupRestoreBusy(err) {
		t.Fatalf("reservation write = %v, want backup_restore_busy", err)
	}
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "1000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Blocked", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatalf("preview should remain readable: %v", err)
	}
	if _, err := service.RecordProductOperation(ctx, openCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash); !isBackupRestoreBusy(err) {
		t.Fatalf("record write = %v, want backup_restore_busy", err)
	}
}

func TestLiquiditySnapshotRoundTripPreservesFacts(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "150000")
	openCmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{
		AccountID: account.String(), Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
		Terms:  ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "none"},
		Policy: depositPolicy(),
	}}
	preview, err := service.PreviewProductOperation(ctx, openCmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RecordProductOperation(ctx, openCmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	usd, _ := domain.ParseCurrency("USD")
	if _, err := service.SaveLiquidityReservation(ctx, SaveReservationInput{
		Source: domain.AccountCashSourceRef(account, usd), Label: "Bills", Amount: "2000",
	}); err != nil {
		t.Fatal(err)
	}
	original := decodeExport(t, service)
	path := filepath.Join(t.TempDir(), "liquidity-backup.sqlite")
	if err := service.SnapshotTo(ctx, path); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restoredDB.Close() })
	restored := NewService(sqlite.NewRepository(restoredDB))
	restored.setClock(func() time.Time { return time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC) })
	copyDoc := decodeExport(t, restored)
	if len(copyDoc.Facts.Liquidity["contracts"]) != 1 || len(copyDoc.Facts.Liquidity["operations"]) != 1 || len(copyDoc.Facts.Liquidity["reservations"]) != 1 {
		t.Fatalf("restored liquidity facts = %+v", copyDoc.Facts.Liquidity)
	}
	if original.Facts.Liquidity["contracts"][0]["id"] != copyDoc.Facts.Liquidity["contracts"][0]["id"] {
		t.Fatalf("contract id drifted: %v vs %v", original.Facts.Liquidity["contracts"][0]["id"], copyDoc.Facts.Liquidity["contracts"][0]["id"])
	}
}

func TestAssumedBankCashIsAvailableToday(t *testing.T) {
	service, ctx := newProductTestService(t, time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC))
	account := seedHoldingsCash(t, service, ctx, "10000")
	overview, err := service.LiquidityOverview(ctx, LiquidityOverviewQuery{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range overview.Sources {
		if source.AccountID == account && source.Ref.Kind == domain.SourceAccountCash {
			found = true
			if source.PolicyOrigin != domain.PolicyOriginAssumed {
				t.Fatalf("origin = %s, want assumed", source.PolicyOrigin)
			}
			if source.NormalRoute == nil || source.NormalRoute.NetNative == nil || source.NormalRoute.NetNative.CanonicalAmount() != "10000" {
				t.Fatalf("assumed bank cash net = %+v", source.NormalRoute)
			}
		}
	}
	if !found {
		t.Fatal("cash source missing")
	}
	assertBucket(t, overview, "2026-09-20", "10000", "0", "10000")
}
