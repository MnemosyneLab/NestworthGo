package reconciliation

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/application"
	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
)

func TestLinkedValuationVerificationCompatibility(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "source.db")
	db, err := sqlite.Open(path)
	requireValuationTest(t, err)
	t.Cleanup(func() { _ = db.Close() })
	app := application.NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	app.SetClock(func() time.Time { return now })
	accountID, productID := seedValuationReceipt(t, ctx, app)
	boot, err := app.Bootstrap(ctx)
	requireValuationTest(t, err)
	other, err := app.CreateAccount(ctx, application.AccountInput{
		Name: "Other synthetic bank", AccountType: "bank_account", BalanceSheetRole: "asset", TrackingMode: "holdings",
		DefaultCurrency: "USD", IncludeInNetWorth: true, OwnerIDs: []domain.MemberID{boot.Members[0].ID},
	})
	requireValuationTest(t, err)

	now = now.Add(24 * time.Hour)
	legacyID := domain.NewProductOperationID().String()
	_, err = app.AppendProductValuation(ctx, application.AppendProductValuationInput{
		ProductID: productID, Amount: "129.0011", ObservedAt: now.Format(time.RFC3339Nano), MutationID: legacyID,
	})
	requireValuationTest(t, err)

	now = now.Add(time.Hour)
	standalone, token, err := app.PreviewProductValuationGuarded(ctx, application.ProductValuationCommand{ProductID: productID, Amount: "130.0022"})
	requireValuationTest(t, err)
	standaloneID := domain.NewProductOperationID().String()
	_, err = app.CommitProductValuationGuarded(ctx, standalone.Command, standaloneID, standalone.ReviewedStateHash, token)
	requireValuationTest(t, err)

	api := NewService(app)
	start, err := api.Preview(ctx, application.ReconciliationAction{Action: "start", AccountIDs: []string{accountID, other.Account.ID.String()}})
	requireValuationTest(t, err)
	session, err := api.Commit(ctx, start.PlanID)
	requireValuationTest(t, err)
	findLink := func(accountID, kind string) domain.ReconciliationLink {
		t.Helper()
		for _, account := range session.Accounts {
			if account.AccountID != accountID {
				continue
			}
			for _, item := range account.Items {
				if item.Kind == kind {
					return domain.ReconciliationLink{SessionID: session.ID, AccountID: accountID, ItemID: item.ID,
						ExpectedRevision: session.Revision, FinancialRevision: account.FinancialRevision}
				}
			}
		}
		t.Fatalf("missing synthetic %s item for %s", kind, accountID)
		return domain.ReconciliationLink{}
	}
	link := findLink(accountID, "product")
	scopeLink := findLink(accountID, "scope")
	otherLink := findLink(other.Account.ID.String(), "scope")

	now = now.Add(time.Hour)
	const literalAmount = " 131.2300 "
	const enteredTime = "2026-09-21T09:59:00.123456789-04:00"
	const observedAt = "2026-09-21T13:59:00.123Z"
	preview := func(target domain.ReconciliationLink) ProductPreview {
		t.Helper()
		p, err := api.PreviewProductValuation(ctx, target, productID.String(), literalAmount, enteredTime)
		requireValuationTest(t, err)
		if p.Valuation.Command.Amount != literalAmount || p.Valuation.Command.ObservedAt != observedAt {
			t.Fatalf("unexpected normalized valuation command: %+v", p.Valuation.Command)
		}
		return p
	}

	// The product repository inserts the reconciliation link before product facts.
	// Force the subsequent operation insert to fail and exercise the real rollback.
	rejected := preview(link)
	_, err = db.SQL.ExecContext(ctx, `CREATE TEMP TRIGGER synthetic_reject_value
		BEFORE INSERT ON product_operations WHEN NEW.kind = 'value_observation'
		BEGIN SELECT RAISE(ABORT, 'synthetic valuation rollback'); END`)
	requireValuationTest(t, err)
	if err := api.CommitProduct(ctx, rejected.PlanID); err == nil {
		t.Fatal("injected failure did not abort the transaction")
	}
	_, err = db.SQL.ExecContext(ctx, "DROP TRIGGER synthetic_reject_value")
	requireValuationTest(t, err)
	var counts [3]int
	requireValuationTest(t, db.SQL.QueryRowContext(ctx, `SELECT
		(SELECT count(*) FROM reconciliation_links), (SELECT count(*) FROM product_operations WHERE kind='value_observation'),
		(SELECT count(*) FROM instrument_quotes)`).Scan(&counts[0], &counts[1], &counts[2]))
	if counts != [3]int{0, 2, 3} {
		t.Fatalf("failed write affected link, observations or quotes: %v", counts)
	}

	linked := preview(link)
	requireValuationTest(t, api.CommitProduct(ctx, linked.PlanID))
	requireValuationTest(t, NewService(app).CommitProduct(ctx, linked.PlanID))
	var request, digest string
	requireValuationTest(t, db.SQL.QueryRowContext(ctx, "SELECT request_json,payload_sha256 FROM product_operations WHERE id=?", linked.PlanID).Scan(&request, &digest))
	var stored struct {
		Command json.RawMessage `json:"command"`
	}
	requireValuationTest(t, jsoncodec.Unmarshal([]byte(request), &stored))
	canonical, err := jsoncodec.Marshal(linked.Valuation.Command)
	requireValuationTest(t, err)
	if string(stored.Command) != string(canonical) || digest != valuationCommandDigest(t, linked.Valuation.Command) {
		t.Fatal("the operation did not preserve the reviewed command bytes and digest")
	}

	// A product may also match the owning account's completeness checkpoint.
	// Re-read its new financial revision after the previous recorded observation.
	session, err = api.Get(ctx, session.ID)
	requireValuationTest(t, err)
	now = now.Add(time.Hour)
	scoped, err := api.PreviewProductValuation(ctx, findLink(accountID, "scope"), productID.String(), "132.1234", now.Format(time.RFC3339Nano))
	requireValuationTest(t, err)
	requireValuationTest(t, api.CommitProduct(ctx, scoped.PlanID))

	session, err = api.Get(ctx, session.ID)
	requireValuationTest(t, err)
	pause, err := api.Preview(ctx, application.ReconciliationAction{
		Action: "pause", SessionID: session.ID, ExpectedRevision: session.Revision,
	})
	requireValuationTest(t, err)
	paused, err := api.Commit(ctx, pause.PlanID)
	requireValuationTest(t, err)
	if paused.Status != "paused" || paused.Revision <= link.ExpectedRevision {
		t.Fatalf("session did not advance beyond the recorded link: %+v", paused)
	}

	original := captureValuationFacts(t, db.SQL)
	requireValuationTest(t, db.CheckpointWAL(ctx))
	requireValuationTest(t, db.Close())
	source, err := os.ReadFile(path)
	requireValuationTest(t, err)

	t.Run("valid writable and backup verification", func(t *testing.T) {
		for _, open := range []func(string) (*sqlite.DB, error){sqlite.Open, sqlite.OpenReadOnlyForVerify} {
			copyPath := filepath.Join(t.TempDir(), "copy.db")
			requireValuationTest(t, os.WriteFile(copyPath, source, 0o600))
			opened, err := open(copyPath)
			requireValuationTest(t, err)
			t.Cleanup(func() { _ = opened.Close() })
			if got := captureValuationFacts(t, opened.SQL); !reflect.DeepEqual(original, got) {
				t.Fatal("verification changed schema, hashes, receipt bytes, links or quote facts")
			}
			var amount, when string
			requireValuationTest(t, opened.SQL.QueryRowContext(t.Context(), `SELECT
				json_extract(evidence,'$.committed.result.receipt.amount.amount'),
				json_extract(evidence,'$.committed.result.receipt.observedAt')
				FROM reconciliation_links WHERE operation_id=? AND session_id=? AND account_id=? AND item_id=?`,
				linked.PlanID, link.SessionID, link.AccountID, link.ItemID).Scan(&amount, &when))
			if amount != "131.23" || when != observedAt {
				t.Fatalf("reopened linked evidence changed: %s at %s", amount, when)
			}
		}
	})

	cases := []struct {
		name, query string
		args        []any
	}{
		{"linked receipt amount", "UPDATE product_operations SET result_json=json_set(result_json,'$.receipt.amount.amount','1') WHERE id=?", []any{linked.PlanID}},
		{"linked receipt date", "UPDATE product_operations SET result_json=json_set(result_json,'$.receipt.observedAt','2026-09-21T12:39:00Z') WHERE id=?", []any{linked.PlanID}},
		{"removed persisted link", "DELETE FROM reconciliation_links WHERE operation_id=?", []any{linked.PlanID}},
		{"wrong link item with valid foreign key", "UPDATE reconciliation_links SET item_id=? WHERE operation_id=?", []any{scopeLink.ItemID, linked.PlanID}},
		{"wrong link account with valid foreign key", "UPDATE reconciliation_links SET account_id=?,item_id=? WHERE operation_id=?", []any{otherLink.AccountID, otherLink.ItemID, linked.PlanID}},
		{"competing product link with valid foreign key", "INSERT INTO reconciliation_links(session_id,account_id,item_id,operation_id,family,evidence) SELECT session_id,?,?,operation_id,family,evidence FROM reconciliation_links WHERE operation_id=?", []any{otherLink.AccountID, otherLink.ItemID, linked.PlanID}},
		{"unexpected product link on standalone guarded receipt", "INSERT INTO reconciliation_links(session_id,account_id,item_id,operation_id,family,evidence) SELECT session_id,account_id,item_id,?,family,evidence FROM reconciliation_links WHERE operation_id=?", []any{standaloneID, linked.PlanID}},
		{"unexpected product link on legacy observation", "INSERT INTO reconciliation_links(session_id,account_id,item_id,operation_id,family,evidence) SELECT session_id,account_id,item_id,?,family,evidence FROM reconciliation_links WHERE operation_id=?", []any{legacyID, linked.PlanID}},
		{"wrong link family", "UPDATE reconciliation_links SET family='activity' WHERE operation_id=?", []any{linked.PlanID}},
		{"linked row stripped to legacy shape", "UPDATE product_operations SET request_json=json_remove(request_json,'$.command'), result_json=json_remove(result_json,'$.receipt') WHERE id=?", []any{linked.PlanID}},
		{"removed sealed command link", "UPDATE product_operations SET request_json=json_remove(request_json,'$.command.reconciliationLink') WHERE id=?", []any{linked.PlanID}},
		{"equivalent amount with altered literal", "UPDATE product_operations SET request_json=json_set(request_json,'$.command.amount','131.23') WHERE id=?", []any{linked.PlanID}},
		{"changed committed link evidence", "UPDATE reconciliation_links SET evidence=json_set(evidence,'$.reviewedBefore.financialRevision',100) WHERE operation_id=?", []any{linked.PlanID}},
		{"changed linked session revision", "UPDATE reconciliation_links SET evidence=json_set(evidence,'$.sessionRevision',100) WHERE operation_id=?", []any{linked.PlanID}},
		{"linked target no longer matches", "UPDATE reconciliation_links SET evidence=json_set(evidence,'$.targetMatched',json('false')) WHERE operation_id=?", []any{linked.PlanID}},
		{"changed duplicated committed result", "UPDATE reconciliation_links SET evidence=json_set(evidence,'$.committed.result.receipt.amount.amount','5') WHERE operation_id=?", []any{linked.PlanID}},
		{"unrelated guarded amount", "UPDATE product_operations SET result_json=json_set(result_json,'$.receipt.amount.amount','2') WHERE id=?", []any{standaloneID}},
		{"unrelated guarded digest", "UPDATE product_operations SET payload_sha256=? WHERE id=?", []any{strings.Repeat("0", 64), standaloneID}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			copyPath := filepath.Join(t.TempDir(), "tampered.db")
			requireValuationTest(t, os.WriteFile(copyPath, source, 0o600))
			raw, err := sql.Open("sqlite", "file:"+copyPath+"?_pragma=foreign_keys(1)")
			requireValuationTest(t, err)
			res, err := raw.ExecContext(t.Context(), tc.query, tc.args...)
			requireValuationTest(t, err)
			n, err := res.RowsAffected()
			requireValuationTest(t, err)
			if n != 1 {
				t.Fatalf("changed %d rows", n)
			}
			requireValuationTest(t, raw.Close())
			expectValuationIntegrityFailure(t, copyPath)
		})
	}
	t.Run("same ambiguous command stored in both records", func(t *testing.T) {
		copyPath := filepath.Join(t.TempDir(), "ambiguous.db")
		requireValuationTest(t, os.WriteFile(copyPath, source, 0o600))
		raw, err := sql.Open("sqlite", "file:"+copyPath+"?_pragma=foreign_keys(1)")
		requireValuationTest(t, err)
		t.Cleanup(func() { _ = raw.Close() })
		// An extra field decodes to the same typed command and digest. Even
		// when both copies agree, it is outside the normalized wire shape.
		_, err = raw.ExecContext(t.Context(), "UPDATE product_operations SET request_json=json_set(request_json,'$.command.conflictingEvidence',1) WHERE id=?", linked.PlanID)
		requireValuationTest(t, err)
		_, err = raw.ExecContext(t.Context(), `UPDATE reconciliation_links SET evidence=json_set(evidence,'$.committed.request',
			json((SELECT request_json FROM product_operations WHERE id=?))) WHERE operation_id=?`, linked.PlanID, linked.PlanID)
		requireValuationTest(t, err)
		requireValuationTest(t, raw.Close())
		expectValuationIntegrityFailure(t, copyPath)
	})
}

func expectValuationIntegrityFailure(t *testing.T, path string) {
	t.Helper()
	opened, err := sqlite.Open(path)
	if opened != nil {
		_ = opened.Close()
	}
	var bootstrap *sqlite.BootstrapError
	if !errors.As(err, &bootstrap) || bootstrap.Status != sqlite.StatusIntegrityFailed ||
		!strings.Contains(err.Error(), "product: valuation receipt disagrees with recorded facts") {
		t.Fatalf("tampered receipt got %v", err)
	}
}

func captureValuationFacts(t *testing.T, db *sql.DB) []string {
	t.Helper()
	var result []string
	for _, query := range []string{
		"SELECT CAST(user_version AS TEXT) FROM pragma_user_version",
		"SELECT sql FROM sqlite_schema WHERE sql IS NOT NULL ORDER BY name",
		"SELECT json_array(id,payload_sha256,request_json,result_json,effective_at,created_at) FROM product_operations ORDER BY id",
		"SELECT json_array(session_id,account_id,item_id,operation_id,family,evidence) FROM reconciliation_links ORDER BY operation_id",
		"SELECT json_array(id,unit_price,currency,quoted_at,created_at) FROM instrument_quotes ORDER BY id",
	} {
		rows, err := db.QueryContext(t.Context(), query)
		requireValuationTest(t, err)
		for rows.Next() {
			var row string
			requireValuationTest(t, rows.Scan(&row))
			result = append(result, row)
		}
		requireValuationTest(t, rows.Err())
		requireValuationTest(t, rows.Close())
	}
	return result
}
