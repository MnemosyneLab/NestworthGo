//go:build linked_valuation_reopen_repro

package reconciliation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

// Opt-in regression for the existing failure at the normal SQLite open boundary.
// The linked case is expected to fail until the shared verifier is corrected.
func TestValuationReceiptReopen(t *testing.T) {
	for _, mode := range []string{"legacy", "standalone", "linked"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			path := filepath.Join(t.TempDir(), "synthetic.db")
			db, err := sqlite.Open(path)
			requireValuationRepro(t, err)
			t.Cleanup(func() { _ = db.Close() })
			app := application.NewService(sqlite.NewRepository(db))
			now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
			app.SetClock(func() time.Time { return now })
			accountID, productID := seedValuationRepro(t, ctx, app)
			now = now.Add(24 * time.Hour)

			var schema int
			requireValuationRepro(t, db.SQL.QueryRowContext(ctx, "PRAGMA user_version").Scan(&schema))
			const amount = "131.2345"
			command := application.ProductValuationCommand{ProductID: productID, Amount: amount, ObservedAt: now.Format(time.RFC3339Nano)}
			mutationID := domain.NewProductOperationID().String()
			switch mode {
			case "legacy":
				_, err = app.AppendProductValuation(ctx, application.AppendProductValuationInput{
					ProductID: productID, Amount: amount, ObservedAt: command.ObservedAt, MutationID: mutationID,
				})
				requireValuationRepro(t, err)
			case "standalone":
				preview, token, err := app.PreviewProductValuationGuarded(ctx, command)
				requireValuationRepro(t, err)
				command = preview.Command
				_, err = app.CommitProductValuationGuarded(ctx, command, mutationID, preview.ReviewedStateHash, token)
				requireValuationRepro(t, err)
			case "linked":
				api := NewService(app)
				link := valuationReproLink(t, ctx, api, accountID, productID.String())
				preview, err := api.PreviewProductValuation(ctx, link, productID.String(), amount, command.ObservedAt)
				requireValuationRepro(t, err)
				command, mutationID = preview.Valuation.Command, preview.PlanID
				requireValuationRepro(t, api.CommitProduct(ctx, mutationID))
				// The same acknowledged mutation resolves through the saved receipt.
				requireValuationRepro(t, NewService(app).CommitProduct(ctx, mutationID))
			}

			var digest, request, unitPrice, currency string
			var linkedRows, observations int
			requireValuationRepro(t, db.SQL.QueryRowContext(ctx, `
				SELECT o.payload_sha256, o.request_json, q.unit_price, q.currency,
				       (SELECT count(*) FROM reconciliation_links l WHERE l.operation_id = o.id),
				       (SELECT count(*) FROM product_operations WHERE kind = 'value_observation')
				FROM product_operations o JOIN instrument_quotes q ON q.id = json_extract(o.result_json, '$.quoteId')
				WHERE o.kind = 'value_observation' AND o.id = ?`, mutationID).Scan(&digest, &request, &unitPrice, &currency, &linkedRows, &observations))
			wantLinks := 0
			if mode == "linked" {
				wantLinks = 1
			}
			if unitPrice != amount || currency != "USD" || linkedRows != wantLinks || observations != 1 {
				t.Fatalf("committed amount %s %s, links %d, observations %d", currency, unitPrice, linkedRows, observations)
			}
			var saved struct {
				Command *application.ProductValuationCommand `json:"command"`
			}
			requireValuationRepro(t, jsoncodec.Unmarshal([]byte(request), &saved))
			if (saved.Command == nil) != (mode == "legacy") {
				t.Fatalf("unexpected stored request shape: %s", request)
			}
			if saved.Command != nil {
				fullDigest := valuationCommandDigest(t, *saved.Command)
				if fullDigest != valuationCommandDigest(t, command) {
					t.Fatal("saved command differs from the reviewed command")
				}
				// Omitting only the link gives the same ordered three-field JSON
				// that verifyProductValuationReceipts currently reconstructs.
				saved.Command.ReconciliationLink = nil
				unlinkedDigest := valuationCommandDigest(t, *saved.Command)
				if fullDigest != digest || (unlinkedDigest == digest) != (mode == "standalone") {
					t.Fatalf("unexpected digest evidence: full match %t, three-field match %t", fullDigest == digest, unlinkedDigest == digest)
				}
				t.Logf("schema %d: exact quote and receipt saved, full digest matches, three-field digest matches: %t", schema, unlinkedDigest == digest)
			}
			var afterWriteSchema int
			requireValuationRepro(t, db.SQL.QueryRowContext(ctx, "PRAGMA user_version").Scan(&afterWriteSchema))
			if afterWriteSchema != schema {
				t.Fatalf("schema changed during post: %d to %d", schema, afterWriteSchema)
			}
			requireValuationRepro(t, db.CheckpointWAL(ctx))
			requireValuationRepro(t, db.Close())

			reopened, err := sqlite.Open(path)
			var bootstrap *sqlite.BootstrapError
			if mode == "linked" && errors.As(err, &bootstrap) && bootstrap.Status == sqlite.StatusIntegrityFailed && strings.Contains(err.Error(), "product: valuation receipt disagrees with recorded facts") {
				t.Fatalf("valid linked observation rejected by startup verification: %v", err)
			}
			requireValuationRepro(t, err)
			defer reopened.Close()
			reopenedApp := application.NewService(sqlite.NewRepository(reopened))
			reopenedApp.SetClock(func() time.Time { return now })
			detail, err := reopenedApp.Product(ctx, productID)
			requireValuationRepro(t, err)
			if detail.CurrentValue == nil || detail.CurrentValue.Amount().String() != amount || detail.CurrentValue.Currency() != "USD" {
				t.Fatalf("reopened product value: %v", detail.CurrentValue)
			}
		})
	}
}

func seedValuationRepro(t *testing.T, ctx context.Context, app *application.Service) (string, domain.ProductContractID) {
	t.Helper()
	requireValuationRepro(t, app.CompleteOnboarding(ctx, application.OnboardingInput{HouseholdName: "Synthetic household", BaseCurrency: "USD", MemberNames: []string{"Owner"}}))
	_, err := app.StartHistory(ctx, "UTC")
	requireValuationRepro(t, err)
	bootstrap, err := app.Bootstrap(ctx)
	requireValuationRepro(t, err)
	account, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings", DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{bootstrap.Members[0].ID},
	})
	requireValuationRepro(t, err)
	accountID := account.Account.ID.String()
	command := application.ProductCommand{Kind: domain.ProductOpRecordExisting, RecordExisting: &application.RecordExistingProductCommand{
		AccountID: accountID, Currency: "USD", Principal: "125.25", TotalCostBasis: "120", CurrentValue: "123.4567", CashExcludesProduct: true,
		Terms:  application.ProductTermsInput{Kind: "locked_product", Name: "Synthetic product", StartOn: "2026-09-01", InterestMode: "none"},
		Policy: application.ProductPolicyInput{AccessKind: "on_date", UnlockOn: productString("2026-12-20"), EarlyKind: "not_allowed", SettlementDays: productInt(0), DayBasis: productString("calendar"), NormalExitFee: productString("0")},
	}}
	preview, err := app.PreviewProductOperation(ctx, command)
	requireValuationRepro(t, err)
	receipt, err := app.RecordProductOperation(ctx, command, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	requireValuationRepro(t, err)
	return accountID, receipt.ProductIDs[0]
}

func valuationReproLink(t *testing.T, ctx context.Context, api *Service, accountID, productID string) domain.ReconciliationLink {
	t.Helper()
	plan, err := api.Preview(ctx, application.ReconciliationAction{Action: "start", AccountIDs: []string{accountID}})
	requireValuationRepro(t, err)
	session, err := api.Commit(ctx, plan.PlanID)
	requireValuationRepro(t, err)
	for _, account := range session.Accounts {
		for _, item := range account.Items {
			if account.AccountID == accountID && item.Kind == "product" && item.ProductID == productID {
				return domain.ReconciliationLink{SessionID: session.ID, AccountID: accountID, ItemID: item.ID, ExpectedRevision: session.Revision, FinancialRevision: account.FinancialRevision}
			}
		}
	}
	t.Fatal("started session without the synthetic product checkpoint")
	return domain.ReconciliationLink{}
}

func valuationCommandDigest(t *testing.T, command application.ProductValuationCommand) string {
	t.Helper()
	raw, err := jsoncodec.Marshal(command)
	requireValuationRepro(t, err)
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func requireValuationRepro(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
