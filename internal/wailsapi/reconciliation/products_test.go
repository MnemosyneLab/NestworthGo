package reconciliation

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/wailsapi/apierror"
)

// Exercise the desktop expiry substitution and application receipt lookup together.
// Every fixture is a new synthetic ledger; no provider or user data is used.
func TestProductRefusalAndReceiptRecovery(t *testing.T) {
	for _, kind := range []string{"terms", "valuation", "operation"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			db, err := sqlite.Open(filepath.Join(t.TempDir(), "products.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			app := application.NewService(sqlite.NewRepository(db))
			now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
			app.SetClock(func() time.Time { return now })
			if err := app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
				t.Fatal(err)
			}
			if _, err := app.StartHistory(ctx, "UTC"); err != nil {
				t.Fatal(err)
			}
			bootstrap, err := app.Bootstrap(ctx)
			if err != nil {
				t.Fatal(err)
			}
			account, err := app.CreateAccount(ctx, application.AccountInput{Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID}})
			if err != nil {
				t.Fatal(err)
			}
			command := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{
				AccountID: account.Account.ID.String(), Currency: "USD", Principal: "1000", TotalCostBasis: "950", CurrentValue: "1000", CashExcludesProduct: true,
				Terms:  application.ProductTermsInput{Kind: "locked_product", Name: "Synthetic product", StartOn: "2026-09-01", InterestMode: "none"},
				Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: productString("2026-12-20"), EarlyKind: "not_allowed", SettlementDays: productInt(0), DayBasis: productString("calendar"), NormalExitFee: productString("0")},
			}}
			opened, err := app.PreviewProductOperation(ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := app.RecordProductOperation(ctx, command, uuid.NewString(), opened.ReviewedStateHash)
			if err != nil {
				t.Fatal(err)
			}
			productID := receipt.ProductIDs[0]
			detail, err := app.Product(ctx, productID)
			if err != nil {
				t.Fatal(err)
			}
			p := productPlan{kind: kind, id: uuid.NewString(), expires: time.Now().Add(time.Hour)}
			switch kind {
			case "terms":
				p.terms = application.UpdateProductTermsInput{ProductID: productID, ExpectedRevision: detail.Contract.Revision, Terms: command.RecordExisting.Terms, Policy: command.RecordExisting.Policy}
				p.terms.Terms.Name = "Reviewed name"
				preview, token, err := app.PreviewProductTermsGuarded(ctx, p.terms)
				if err != nil {
					t.Fatal(err)
				}
				p.token, p.state = token, preview.ReviewedStateHash
			case "valuation":
				preview, token, err := app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ProductID: productID, Amount: "1100"})
				if err != nil {
					t.Fatal(err)
				}
				p.valuation, p.token, p.state = preview.Command, token, preview.ReviewedStateHash
			case "operation":
				preview, token, err := app.PreviewProductOperationGuarded(ctx, application.ProductCommand{Kind: domain.ProductOpSettle, Settle: &application.SettleProductCommand{ProductID: productID.String(), GrossProceeds: productString("1100")}})
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(preview.NormalizedJSON), &p.operation); err != nil {
					t.Fatal(err)
				}
				p.token, p.state = token, preview.ReviewedStateHash
			}
			service := NewService(app)
			assertStale := func(plan productPlan, field string) {
				t.Helper()
				err := service.CommitProduct(ctx, service.products.Put(plan))
				if err == nil {
					t.Fatal("stale plan committed")
				}
				wire, ok := apierror.Parse(err.Error())
				if !ok || wire.Code != "stale_preview" || wire.Field != field {
					t.Fatalf("got %v; want stale_preview/%s", err, field)
				}
			}
			expired := p
			expired.expires = time.Now().Add(-time.Minute)
			assertStale(expired, "previewToken")
			invalidated := p
			invalidated.token = "invalidated"
			assertStale(invalidated, "previewToken")
			// Failed write attempts also fence old tokens. Refresh only the token so
			// the original immutable reviewed state remains under test.
			refreshToken := func() string {
				t.Helper()
				_, token, err := app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ProductID: productID, Amount: "1100"})
				if err != nil {
					t.Fatal(err)
				}
				return token
			}
			p.token = refreshToken()
			// A new ledger day changes the reviewed facts without changing the process token.
			now = now.Add(24 * time.Hour)
			assertStale(p, "reviewedStateHash")
			now = now.Add(-24 * time.Hour)
			current, err := app.Product(ctx, productID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current.Contract, detail.Contract) || !reflect.DeepEqual(current.CurrentValue, detail.CurrentValue) {
				t.Fatal("refused corrections changed product facts")
			}
			// The same mutation still commits with its original facts: refusals wrote no receipt.
			p.token = refreshToken()
			registered, err := service.saveProductPlan(ctx, p)
			if err != nil {
				t.Fatal(err)
			}
			if err := NewService(app).CommitProduct(ctx, registered); err != nil {
				t.Fatal(err)
			}
			if _, err := db.SQL.Exec("DELETE FROM app_configuration WHERE key LIKE 'reconciliation.desktop.%'"); err != nil {
				t.Fatal(err)
			}
			if err := NewService(app).CommitProduct(ctx, registered); err != nil {
				t.Fatalf("missing product body receipt: %v", err)
			}
			// Lost-response retries must recover before expiry, token or state checks.
			expired.token, expired.state = "invalidated", "stale"
			if err := service.CommitProduct(ctx, service.products.Put(expired)); err != nil {
				t.Fatalf("receipt recovery: %v", err)
			}
			changed := expired
			switch kind {
			case "terms":
				changed.terms.Terms.Name = "Different request"
			case "valuation":
				changed.valuation.Amount = "1200"
			case "operation":
				copy := *changed.operation.Settle
				copy.GrossProceeds = productString("1200")
				changed.operation.Settle = &copy
			}
			err = service.CommitProduct(ctx, service.products.Put(changed))
			if err == nil {
				t.Fatal("changed receipt payload accepted")
			}
			wire, ok := apierror.Parse(err.Error())
			if !ok || wire.Code != "conflict" || (wire.Field != "" && wire.Field != "mutationId") {
				t.Fatalf("receipt conflict misclassified: %v", err)
			}
		})
	}
}

func TestMissingProductPlanRemainsUnknown(t *testing.T) {
	err := NewService(nil).CommitProduct(context.Background(), uuid.NewString())
	if err == nil {
		t.Fatal("missing plan accepted")
	}
	wire, ok := apierror.Parse(err.Error())
	if !ok || wire.Code != "stale_preview" || wire.Field != "planId" {
		t.Fatal(err)
	}
}

func productString(value string) *string { return &value }

func productInt(value int) *int { return &value }
