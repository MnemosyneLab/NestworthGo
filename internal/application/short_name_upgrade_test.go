package application

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
)

// Exact rows include all financial records, IDs, canonical decimals, seals,
// observations, history origin, mutation keys, product facts and configuration.
func financialRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	tables, err := db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE 'reconciliation_%' AND name NOT LIKE 'investment_review_%' AND name != 'account_financial_revisions' AND name NOT IN ('financial_source_revision','financial_source_changes','review_temporal_checkpoints','agent_instrument_closures') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for tables.Next() {
		var name string
		if err := tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	out := map[string][]string{}
	for _, name := range names {
		info, err := db.Query(`PRAGMA table_info("` + name + `")`)
		if err != nil {
			t.Fatal(err)
		}
		var columns []string
		for info.Next() {
			var cid, nn, pk int
			var n, typ string
			var d sql.NullString
			if err := info.Scan(&cid, &n, &typ, &nn, &d, &pk); err != nil {
				t.Fatal(err)
			}
			if n != "short_name" {
				columns = append(columns, `"`+n+`"`)
			}
		}
		info.Close()
		list := strings.Join(columns, ",")
		rows, err := db.Query(`SELECT ` + list + ` FROM "` + name + `" ORDER BY ` + list)
		if err != nil {
			t.Fatal(err)
		}
		out[name] = []string{}
		for rows.Next() {
			values := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range values {
				dest[i] = &values[i]
			}
			if err := rows.Scan(dest...); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			out[name] = append(out[name], string(raw))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return out
}

func TestPopulatedV15FinancialUpgradeAndAliasReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "synthetic-current.sqlite")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 9, 20, 4, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic migration", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	account := seedHoldingsCash(t, s, ctx, "150000.01")
	cmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "100000.01", EffectiveAt: "2026-09-20T04:00:00Z", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-09-20", MaturityOn: strPtr("2026-12-20"), InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("123.45")}, Policy: depositPolicy()}}
	preview, err := s.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := s.RecordProductOperation(ctx, cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash)
	if err != nil {
		t.Fatal(err)
	}
	product, err := s.Product(ctx, receipt.ProductIDs[0])
	if err != nil {
		t.Fatal(err)
	}
	// Include a sealed, idempotent terms receipt in the old-schema source.
	input := UpdateProductTermsInput{ProductID: product.Contract.ID, ExpectedRevision: product.Contract.Revision, Terms: cmd.Open.Terms, Policy: depositPolicy()}
	termsPreview, token, err := s.PreviewProductTermsGuarded(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CommitProductTermsGuarded(ctx, input, domain.NewProductOperationID().String(), termsPreview.ReviewedStateHash, token); err != nil {
		t.Fatal(err)
	}
	before := financialRows(t, db.SQL)
	for _, table := range []string{"holdings", "account_cash_values", "activities", "activity_effects", "product_operations", "product_contracts", "history_origins", "activity_trade_details"} {
		if len(before[table]) == 0 {
			t.Fatalf("fixture missing %s", table)
		}
	}
	snapshot := filepath.Join(dir, "current-snapshot.sqlite")
	if err := db.SnapshotTo(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	// Construct a populated v15 database from the immutable main schema, copying
	// every old column verbatim rather than changing a version on a new schema.
	oldPath := filepath.Join(dir, "synthetic-v15.sqlite")
	old, err := sql.Open("sqlite", oldPath+"?_pragma=foreign_keys%3d1")
	if err != nil {
		t.Fatal(err)
	}
	schema, err := os.ReadFile("../infrastructure/sqlite/testdata/schema_v15.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Exec(`ATTACH DATABASE ? AS source`, snapshot); err != nil {
		t.Fatal(err)
	}
	// Defer foreign-key checks for the table-copy transaction.
	tx, err := old.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(`PRAGMA defer_foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	for name := range before {
		info, err := tx.Query(`PRAGMA table_info("` + name + `")`)
		if err != nil {
			t.Fatal(err)
		}
		var cols []string
		for info.Next() {
			var cid, nn, pk int
			var n, typ string
			var d sql.NullString
			if err := info.Scan(&cid, &n, &typ, &nn, &d, &pk); err != nil {
				t.Fatal(err)
			}
			cols = append(cols, `"`+n+`"`)
		}
		info.Close()
		list := strings.Join(cols, ",")
		if _, err := tx.Exec(`INSERT INTO "` + name + `" (` + list + `) SELECT ` + list + ` FROM source."` + name + `"`); err != nil {
			t.Fatalf("copy %s: %v", name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	old.Close()
	readonly, err := sqlite.OpenReadOnlyForVerify(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, financialRows(t, readonly.SQL)) {
		t.Fatal("v15 fixture differs from source")
	}
	readonly.Close()
	upgraded, err := sqlite.Open(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, financialRows(t, upgraded.SQL)) {
		t.Fatal("v15 upgrade changed financial records")
	}
	if err := upgraded.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	upgradedService := NewService(sqlite.NewRepository(upgraded))
	upgradedService.setClock(func() time.Time { return now })
	loaded, err := upgradedService.Product(ctx, product.Contract.ID)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedRevision = loaded.Contract.Revision
	input.Terms.ShortName = strPtr("存款 🌿")
	p, token, err := upgradedService.PreviewProductTermsGuarded(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := upgradedService.CommitProductTermsGuarded(ctx, input, domain.NewProductOperationID().String(), p.ReviewedStateHash, token); err != nil {
		t.Fatal(err)
	}
	if err := upgraded.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	afterAlias := financialRows(t, upgraded.SQL)
	for _, table := range []string{"account_cash_values", "account_values", "holdings", "activities", "activity_effects", "activity_trade_details", "instrument_quotes", "history_origins", "history_origin_components", "holding_quantity_values", "daily_valuation_snapshots", "daily_valuation_snapshot_items", "product_operations"} {
		if !reflect.DeepEqual(before[table], afterAlias[table]) {
			t.Fatalf("alias changed financial history table %s", table)
		}
	}
	upgraded.Close()
	reopened, err := sqlite.Open(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	alias, err := sqlite.NewRepository(reopened).Instrument(ctx, product.Contract.HouseholdID, product.Contract.InstrumentID)
	if err != nil || alias.ShortName != "存款 🌿" {
		t.Fatalf("reload %+v %v", alias, err)
	}
	// A v16 snapshot carries the same alias into backup/restore destinations.
	backupPath := filepath.Join(dir, "alias-snapshot.sqlite")
	if err := reopened.SnapshotTo(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	restored, err := sqlite.Open(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredAlias, err := sqlite.NewRepository(restored).Instrument(ctx, product.Contract.HouseholdID, product.Contract.InstrumentID)
	if err != nil || restoredAlias.ShortName != alias.ShortName {
		t.Fatalf("restored alias %+v %v", restoredAlias, err)
	}
}
