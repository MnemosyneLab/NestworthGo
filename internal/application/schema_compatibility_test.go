package application

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/waltwang/nestworth-go/internal/domain"
	"github.com/waltwang/nestworth-go/internal/infrastructure/sqlite"
	"github.com/waltwang/nestworth-go/internal/jsoncodec"
	"github.com/waltwang/nestworth-go/internal/testutil"
)

func TestSchema21UpgradePreservesFinancialRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-financial.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewService(sqlite.NewRepository(db))
	now := time.Date(2026, 10, 1, 4, 0, 0, 0, time.UTC)
	s.setClock(func() time.Time { return now })
	ctx := t.Context()
	if err := s.CompleteOnboarding(ctx, OnboardingInput{HouseholdName: "Synthetic migration", BaseCurrency: "USD", MemberNames: []string{"Owner"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartHistory(ctx, "UTC"); err != nil {
		t.Fatal(err)
	}
	account := seedHoldingsCash(t, s, ctx, "150000.01")
	cmd := ProductCommand{Kind: domain.ProductOpOpen, Open: &OpenProductCommand{AccountID: account.String(), Currency: "USD", Principal: "100000.01", EffectiveAt: "2026-10-01T04:00:00Z", Terms: ProductTermsInput{Kind: "term_deposit", Name: "Synthetic deposit", StartOn: "2026-10-01", MaturityOn: strPtr("2026-12-20"), InterestMode: "manual_maturity_amount", MaturityInterest: strPtr("123.45")}, Policy: depositPolicy()}}
	preview, err := s.PreviewProductOperation(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordProductOperation(ctx, cmd, domain.NewProductOperationID().String(), preview.ReviewedStateHash); err != nil {
		t.Fatal(err)
	}
	testutil.Schema21(t, db.SQL)
	// Include immutable facts, exact decimals, cost basis, ownership, provenance,
	// idempotency receipts, financial revisions and journal in the comparison.
	before := supportedFinancialRows(t, db.SQL)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	ro, err := sqlite.OpenReadOnlyForVerify(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := supportedFinancialRows(t, ro.SQL); !reflect.DeepEqual(before, got) {
		t.Fatal("readonly21 changed financial rows")
	}
	ro.Close()
	upgraded, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if got := supportedFinancialRows(t, upgraded.SQL); !reflect.DeepEqual(before, got) {
		t.Fatal("21 to22 changed financial rows")
	}
	if err := upgraded.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func supportedFinancialRows(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	result := map[string][]string{}
	for _, table := range []string{"households", "members", "accounts", "account_ownership", "account_cash_values", "instruments", "holdings", "product_contracts", "liquidity_policies", "activities", "activity_effects", "product_operations", "activity_mutation_keys", "product_operation_products", "product_operation_activities", "financial_source_revision", "financial_source_changes"} {
		rows, err := db.Query(`SELECT * FROM ` + table + ` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		columns, err := rows.Columns()
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			values := make([]any, len(columns))
			destinations := make([]any, len(columns))
			for i := range values {
				destinations[i] = &values[i]
			}
			if err := rows.Scan(destinations...); err != nil {
				t.Fatal(err)
			}
			raw, err := jsoncodec.Marshal(values)
			if err != nil {
				t.Fatal(err)
			}
			result[table] = append(result[table], string(raw))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		rows.Close()
	}
	return result
}
