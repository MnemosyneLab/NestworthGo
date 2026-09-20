package liquidity_test

import (
	"context"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/wailsapi/account"
	"github.com/waltwang/nestworth-go/internal/wailsapi/holding"
	"github.com/waltwang/nestworth-go/internal/wailsapi/household"
	"github.com/waltwang/nestworth-go/internal/wailsapi/liquidity"
	"github.com/waltwang/nestworth-go/internal/wailsapi/wailstest"
)

func TestLiquidityOverviewAndProductLifecycle(t *testing.T) {
	app := wailstest.NewService(t)
	clock := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return clock })
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "Funds", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "UTC",
	}); err != nil {
		t.Fatalf("onboarding: %v", err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []string{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holding.NewService(app).AppendAccountCashValue(ctx, record.Account.ID, "150000", "USD", ""); err != nil {
		t.Fatal(err)
	}
	service := liquidity.NewService(app)
	overview, err := service.Overview(ctx, liquidity.OverviewRequest{})
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	if overview.LocalDate != "2026-09-20" || overview.BaseCurrency != "USD" || len(overview.Buckets) != 3 {
		t.Fatalf("overview = %+v", overview)
	}
	maturity := "2026-12-20"
	zero := 0
	calendar := "calendar"
	preview, err := service.PreviewProductOperation(ctx, liquidity.ProductCommandRequest{
		Kind: "open",
		Open: &application.OpenProductCommand{
			AccountID: record.Account.ID, Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
			Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: &maturity, InterestMode: "none"},
			Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar},
		},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview.ReviewedStateHash == "" || len(preview.Activities) == 0 {
		t.Fatalf("preview incomplete: %+v", preview)
	}
	receipt, err := service.RecordProductOperation(ctx, liquidity.RecordProductOperationRequest{
		Command: liquidity.ProductCommandRequest{
			Kind: "open",
			Open: &application.OpenProductCommand{
				AccountID: record.Account.ID, Currency: "USD", Principal: "100000", EffectiveAt: "2026-09-20T04:00:00Z",
				Terms: application.ProductTermsInput{Kind: "term_deposit", Name: "Deposit D1", StartOn: "2026-09-20", MaturityOn: &maturity, InterestMode: "none"},
				Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: &maturity, EarlyKind: "not_allowed", SettlementDays: &zero, DayBasis: &calendar},
			},
		},
		MutationID:        domain.NewProductOperationID().String(),
		ReviewedStateHash: preview.ReviewedStateHash,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(receipt.ProductIDs) != 1 || receipt.Replayed {
		t.Fatalf("receipt = %+v", receipt)
	}
	detail, err := service.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	if detail.Product.CurrentValue == nil || detail.Product.CurrentValue.Amount != "100000" {
		t.Fatalf("product = %+v", detail.Product)
	}
	if _, err := service.PreviewProductOperation(ctx, liquidity.ProductCommandRequest{Kind: "open"}); err == nil {
		t.Fatal("empty union accepted")
	}
}

func TestLiquidityServiceRejectsInvalidCursor(t *testing.T) {
	app := wailstest.NewService(t)
	ctx := context.Background()
	if err := household.NewService(app).CompleteOnboarding(ctx, household.CompleteOnboardingRequest{
		HouseholdName: "Funds", BaseCurrency: "USD", MemberNames: []string{"Owner"}, Timezone: "UTC",
	}); err != nil {
		t.Fatal(err)
	}
	bootstrap, err := household.NewService(app).Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record, err := account.NewService(app).CreateAccount(ctx, account.CreateAccountRequest{
		Name: "Bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []string{bootstrap.Members[0].ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	service := liquidity.NewService(app)
	_, err = service.ListOperations(ctx, liquidity.ListOperationsRequest{ProductID: record.Account.ID, Cursor: "not-a-cursor"})
	if err == nil {
		t.Fatal("invalid product id accepted")
	}
}
